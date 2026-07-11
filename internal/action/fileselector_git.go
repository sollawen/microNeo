package action

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

// getGitStatus fork git 取某目录的状态。返回 (isRepo, branch, chars)：
//   - isRepo=true 时 branch 为分支名（可能空：detached/unborn），chars 为 name→状态字符；
//   - isRepo=false 时 branch=""、chars=nil（非仓库/超时/diffgutter 关/git 不存在）。
//
// 无缓存：每次 selector 打开/chdir 都重新 fork git——工作树可能被别的进程改，
// 缓存会陈旧；后台几十 ms 不卡首次渲染（fetchGit 在 goroutine 里调本函数）。
func getGitStatus(dir string) (isRepo bool, branch string, chars map[string]rune) {
	if !config.GetGlobalOption("diffgutter").(bool) { // 总开关降级链
		return false, "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// -b：输出首条记录恒为 "## " 分支头（parsePorcelain 据此捕获分支名）。
	out, err := exec.CommandContext(ctx, "git", "-C", dir,
		"status", "--porcelain=v1", "-z", "-b", "--ignored=traditional", "--", ".").Output()
	if err != nil {
		return false, "", nil
	}
	// 复用 status 的 2s ctx 跑 rev-parse，防 goroutine 泄漏。
	// prefix 是「当前目录相对仓库根」的路径，用来把仓库根相对路径换算到当前目录相对。
	prefix := ""
	if p, e := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-prefix").Output(); e == nil {
		prefix = strings.TrimSpace(string(p))
	}
	chars, branch = parsePorcelain(out, prefix)
	return true, branch, chars
}

// parsePorcelain 解析 git status --porcelain=v1 -z -b 的输出。
//
// 输出首条记录恒为 "## " 分支头（-b 产生）：本函数捕获分支名（detached/unborn→空）。
// 其余记录格式 "XY path"（\0 分隔）：剥 prefix、取顶层分量（子目录内变更→对应顶层目录名
// 命中），映射 indexSt/workSt 到 statusKind，按优先级聚合成每 name 一个赢家状态，
// 最后转成 name→rune（颜色归显示层 gitCharStyle）。
func parsePorcelain(out []byte, prefix string) (chars map[string]rune, branch string) {
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
		if name == "." || name == "" {
			continue
		}
		var st statusKind
		// ignored 优先于其它判断（!! 前缀稳定，short-circuit 避免落到 default）。
		if indexSt == '!' && workSt == '!' {
			st = stIgnored
		} else {
			switch workSt {
			case '?':
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
	return chars, branch
}
