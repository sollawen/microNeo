package action

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/micro-editor/micro/v2/internal/config"
)

// statusKind 表示 git 文件状态（F1 §10.2 状态码表）。
type statusKind uint8

const (
	stNone statusKind = iota
	stModified  // M: 已修改
	stUntracked // U: 未跟踪 (??)
	stAdded     // A: 已暂存
	stDeleted   // D: 已删除
	stRenamed   // R: 已重命名
	stIgnored   // I: 被 .gitignore 忽略（!!）— F7
)

// gitStatusCache 是 git 状态缓存的接口，用于解耦与 mock 测试（F1 §10.6）。
type gitStatusCache interface {
	// statusFor 返回指定目录的 git 状态映射。
	// 返回 (nil, false) 表示不可用（非仓库/超时/git 不存在），调用方降级为不显示。
	statusFor(dir string) (map[string]statusKind, bool)
}

// gitStatus 是 gitStatusCache 的阻塞实现（F1 §6.4 契约）。
type gitStatus struct {
	mu    sync.Mutex
	cache map[string]map[string]statusKind // key=dir → 文件名→状态
}

// NewGitStatus 返回一个 gitStatus 实例。
func NewGitStatus() *gitStatus {
	return &gitStatus{
		cache: make(map[string]map[string]statusKind),
	}
}

// statusFor 实现 gitStatusCache 接口。
func (g *gitStatus) statusFor(dir string) (map[string]statusKind, bool) {
	// 1. diffgutter 总开关（F1 §10.4 降级链）
	if !config.GetGlobalOption("diffgutter").(bool) {
		return nil, false
	}

	// 2. 缓存命中（F1 §10.6）
	g.mu.Lock()
	m, ok := g.cache[dir]
	g.mu.Unlock()
	if ok {
		// 命中即返回（空 map 也是有效结果：目录里无变更）
		return m, true
	}

	// 3. fork git（F1 §10.2）：pathspec 钉死当前目录，2s ctx（F1 §10.4）
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git",
		"-C", dir,
		"status", "--porcelain=v1", "-z",
		"--ignored=traditional", // F7：合并到 status，不开第二条进程
		"--", ".")
	out, err := cmd.Output()
	if err != nil {
		// 非仓库 / 超时 / git 不存在 → 静默降级
		return nil, false
	}

	// 多跑一次 rev-parse --show-prefix 拿「当前目录相对仓库根」的前缀
	// 用来对齐 porcelain 恒输出「仓库根相对」路径（见 F1c §2）
	prefix := ""
	prefixCmd := exec.Command("git", "-C", dir, "rev-parse", "--show-prefix")
	if out2, err2 := prefixCmd.Output(); err2 == nil {
		prefix = strings.TrimSpace(string(out2))
	}

	m = parsePorcelain(out, prefix)

	g.mu.Lock()
	g.cache[dir] = m
	g.mu.Unlock()
	return m, true
}

// parsePorcelain 解析 git status --porcelain=v1 -z 的输出。
// prefix 是「当前目录相对仓库根」的路径（带尾斜杠），由 rev-parse --show-prefix 提供。
// 输入是字节串，每条记录格式：XY path\0（F1 §10.2）。
// XY 两个字符表示索引状态和工作区状态，常见值：
//   - "??" → stUntracked（未跟踪）
//   - " M" / "M " / "MM" 等含 M → stModified
//   - "A " / "AM" 等含 A → stAdded
//   - "D " / " D" → stDeleted
//   - "R " → stRenamed
//   - "!!" → stIgnored（被 .gitignore 忽略，F7）
//
// 本函数只关心工作区状态（第二个字符），因为 FileSelector 只显示当前目录条目。
// 剥掉 prefix 后取顶层分量（F1c §3.1）：子目录内文件变更时，对应的子目录条目亮状态。
func parsePorcelain(out []byte, prefix string) map[string]statusKind {
	result := make(map[string]statusKind)
	if len(out) == 0 {
		return result
	}

	records := strings.Split(string(out), "\x00")
	for _, rec := range records {
		if len(rec) < 4 { // 至少 XY、空格、路径首字符（" M x"）
			continue
		}
		// rec[0] = 索引状态，rec[1] = 工作区状态
		indexSt := rec[0]
		workSt := rec[1]

		// 取路径（rec[3:] 开始，跳过 "XY " 三个字符）
		path := rec[3:]
		if path == "" {
			continue
		}

		// 剥掉「当前目录相对仓库根」的前缀，得到「相对当前目录」的路径；再取顶层分量
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

		// 映射工作区状态
		var st statusKind
		// F7: ignored 优先于其它判断（`!!` 前缀稳定，short-circuit 避免落到 default）
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
				// 也检查索引状态（如 "M " 表示暂存区已修改，工作区干净）
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
			// 优先级：已修改/已删除/已重命名 > 已添加 > 未跟踪
			// 这保证同一文件多种状态时取最重要的那个
			if existing, has := result[name]; !has || st < existing {
				result[name] = st
			}
		}
	}
	return result
}
