package finder

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/micro-editor/micro/v2/internal/config"
)

// statusKind 是 porcelain 解析内部的文件状态分类，仅供 parsePorcelain 做优先级聚合
// （st < existing 取更严重者）。不进 state / entry / 返回值：外部边界是 entry.gitChar（rune）。
type statusKind uint8

const (
	stNone      statusKind = iota
	stModified             // M: 已修改
	stUntracked            // U: 未跟踪 (??)
	stAdded                // A: 已暂存
	stDeleted              // D: 已删除
	stRenamed              // R: 已重命名
	stIgnored              // I: 被 .gitignore 忽略（!!）
)

// dirState 描述「当前目录整体」的状态，区别于单文件的 statusKind。
// git 会对「整个 untracked 或 ignored 目录」做折叠报告（只有一条记录，路径恒等于 cwd）。
// parsePorcelain 据折叠记录判定这个状态，上层据它给本目录所有条目统一打标志。
// 枚举值互斥：一个目录不可能既全 ignored 又全 untracked。
type dirState uint8

const (
	dirNormal      dirState = iota // 正常：按 per-entry chars 映射
	dirAllIgnored                  // 当前目录整个被 ignore（git 折叠成 !! cwd/）
	dirAllUntracked                // 当前目录整个 untracked（git 折叠成 ?? cwd/）
)

// getGitStatus fork git 取某目录的状态。返回 (isRepo, branch, chars, state)：
//   - isRepo=true 时 branch 为分支名（可能空：detached/unborn），chars 为 name→状态字符；
//   - isRepo=false 时 branch="", chars=nil, state=dirNormal（非仓库/超时/diffgutter 关/git 不存在）。
//   - state=dirAllIgnored / dirAllUntracked 表示当前目录本身被 git 折叠报告
//     ("!! dir/" 或 "?? dir/")，内部所有条目统一打 'I' 或 'U'。
//
// 无缓存：每次 selector 打开/chdir 都重新 fork git——工作树可能被别的进程改，
// 缓存会陈旧；后台几十 ms 不卡首次渲染（fetchGit 在 goroutine 里调本函数）。
func getGitStatus(dir string) (isRepo bool, branch string, chars map[string]rune, state dirState) {
	if !config.GetGlobalOption("diffgutter").(bool) { // 总开关降级链
		return false, "", nil, dirNormal
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// -b：输出首条记录恒为 "## " 分支头（parsePorcelain 据此捕获分支名）。
	out, err := exec.CommandContext(ctx, "git", "-C", dir,
		"status", "--porcelain=v1", "-z", "-b", "--ignored=traditional", "--", ".").Output()
	if err != nil {
		return false, "", nil, dirNormal
	}
	// 复用 status 的 2s ctx 跑 rev-parse，防 goroutine 泄漏。
	// prefix 是「当前目录相对仓库根」的路径，用来把仓库根相对路径换算到当前目录相对。
	prefix := ""
	if p, e := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-prefix").Output(); e == nil {
		prefix = strings.TrimSpace(string(p))
	}
	chars, branch, state = parsePorcelain(out, prefix)
	return true, branch, chars, state
}

// parsePorcelain 解析 git status --porcelain=v1 -z -b 的输出。
//
// 输出首条记录恒为 "## " 分支头（-b 产生）：本函数捕获分支名（detached/unborn→空）。
// 其余记录格式 "XY path"（\0 分隔）：剥 prefix、取顶层分量（子目录内变更→对应顶层目录名
// 命中），映射 indexSt/workSt 到 statusKind，按优先级聚合成每 name 一个赢家状态，
// 最后转成 name→rune（颜色归显示层 gitCharStyle）。
//
// ignored（!!）和 untracked（??）都可能触发「整目录折叠」：git 对整个 untracked 或
// ignored 的目录只报一条折叠记录，不枚举内部文件。cwd 落在折叠实体内时 git 只回这一条，
// chars 不含任何 per-entry 项，由 state=dirAllIgnored/dirAllUntracked 通知 fetchGit
// 给本目录所有条目统一打标志（'I' 或 'U'）。两种实体的折叠路径行为不同（untracked 报
// cwd 自己、ignored 报 ignored 树根），统一判据见循环内注释。
func parsePorcelain(out []byte, prefix string) (chars map[string]rune, branch string, state dirState) {
	chars = make(map[string]rune)
	agg := make(map[string]statusKind) // 解析内部：name → 赢家状态（优先级聚合，不外泄）
	records := strings.Split(string(out), "\x00")
	for _, rec := range records {
		// —— 分支头（## ...）——
		if strings.HasPrefix(rec, "## ") {
			b := strings.TrimPrefix(rec, "## ")
			if strings.HasPrefix(b, "HEAD (no branch)") {
				continue // detached：无分支名，留空
			}
			b = strings.TrimPrefix(b, "No commits yet on ") // unborn：去掉前缀
			if i := strings.Index(b, "..."); i >= 0 {       // "main...origin/main" → "main"
				b = b[:i]
			}
			if i := strings.IndexByte(b, ' '); i >= 0 { // "main [ahead 1]" → "main"
				b = b[:i]
			}
			branch = b
			continue
		}
		if len(rec) < 4 { // 至少 "XY " + 路径首字符
			continue
		}
		// rec[0] = 索引状态，rec[1] = 工作区状态
		indexSt, workSt := rec[0], rec[1]
		path := rec[3:]
		if path == "" {
			continue
		}
		// 剥掉「当前目录相对仓库根」前缀，得到「相对当前目录」的路径；再取顶层分量。
		rel := strings.TrimPrefix(path, prefix)
		var name string
		if i := strings.IndexByte(rel, '/'); i >= 0 {
			name = rel[:i] // 含 / → 变更在子目录内，取顶层目录名
		} else {
			name = rel // 当前目录内的文件
		}
		var st statusKind
		// —— 统一「整目录折叠」判据 ——
		// git 对 untracked（??）和 ignored（!!）整目录折叠的路径行为不同：
		//   untracked：cwd 在树内任意层，git 恒报 cwd 自己（"?? <cwd>/"，path==prefix）
		//   ignored：  cwd 在实体内任意层，git 恒报 ignored 树根（"!! <树根>/"，prefix 是 path 的祖先）
		// 两种情况 path 都是 prefix 的祖先或等于 prefix；git 只回这一条、不报内部文件，
		// 所以 cwd 整个落在折叠实体内——统一打 dirAllXxx 标志，chars 留空，由上层给本目录
		// 所有条目统一上标志。
		if strings.HasSuffix(path, "/") && strings.HasPrefix(prefix, path) {
			if indexSt == '!' && workSt == '!' {
				state = dirAllIgnored
			} else if indexSt == '?' && workSt == '?' {
				state = dirAllUntracked
			}
			continue
		}
		// ignored（!!）：'I' 只贴在被 ignore 的实体本身，不向上冒泡到祖先目录——
		// 否则父目录会被误读为「自己被 ignore」。用剥 prefix 后的 rel 形态区分：
		//   去掉尾斜杠后不含 / → 直接文件或直接子目录 → 照常冒泡（st=stIgnored）；
		//   其余 → ignored 实体在更深处（单文件或子目录的子目录）→ 丢弃（st 保持 stNone，不进 agg）。
		// 「当前目录本身被 ignore」的实体根情形已被上面的统一判据覆盖（continue），
		// 不在此处处理。
		if indexSt == '!' && workSt == '!' {
			core := strings.TrimSuffix(rel, "/")
			if !strings.ContainsRune(core, '/') {
				st = stIgnored
			}
		} else {
			switch workSt {
			case '?':
				// untracked（??）：照常冒泡到顶层目录名。「cwd 本身整个 untracked」的情形
				// 已被上面的统一判据覆盖（path==prefix → continue），不在此处处理。
				st = stUntracked
			case 'M', 'T': // M=修改，T=类型变更
				st = stModified
			case 'A':
				st = stAdded
			case 'D':
				st = stDeleted
			case 'R':
				st = stRenamed
			default:
				// 工作区干净但索引有变化（如 "M "）也检查索引状态。
				switch indexSt {
				case 'M':
					st = stModified
				case 'A':
					st = stAdded
				case 'D':
					st = stDeleted
				case 'R':
					st = stRenamed
				}
			}
		}
		// name 守卫放这里（而非更早）：rel=="" 的折叠记录需先在上面置好 state，
		// 再由这里的 name=="" 跳过；非折叠的 name=="" 同样在此安全跳过。
		if name == "." || name == "" {
			continue
		}
		if st != stNone {
			// 优先级聚合：同一 name 多记录取更严重者（st 数值越小越严重）。
			if existing, has := agg[name]; !has || st < existing {
				agg[name] = st
			}
		}
	}
	// agg → chars（statusKind→rune；颜色归显示层 gitCharStyle）。
	for name, st := range agg {
		ch := ' '
		switch st {
		case stModified:
			ch = 'M'
		case stUntracked:
			ch = 'U'
		case stAdded:
			ch = 'A'
		case stDeleted:
			ch = 'D'
		case stRenamed:
			ch = 'R'
		case stIgnored:
			ch = 'I'
		}
		chars[name] = ch
	}
	return chars, branch, state
}
