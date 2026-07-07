package action

import (
	"context"
	"os/exec"
	"path/filepath"
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
	// stIgnored (I) 延后（F1 §10.5 / R4）
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
		"status", "--porcelain=v1", "-z", "--", ".")
	out, err := cmd.Output()
	if err != nil {
		// 非仓库 / 超时 / git 不存在 → 静默降级
		return nil, false
	}

	m = parsePorcelain(out)

	g.mu.Lock()
	g.cache[dir] = m
	g.mu.Unlock()
	return m, true
}

// parsePorcelain 解析 git status --porcelain=v1 -z 的输出。
// 输入是字节串，每条记录格式：XY path\0（F1 §10.2）。
// XY 两个字符表示索引状态和工作区状态，常见值：
//   - "??" → stUntracked（未跟踪）
//   - " M" / "M " / "MM" 等含 M → stModified
//   - "A " / "AM" 等含 A → stAdded
//   - "D " / " D" → stDeleted
//   - "R " → stRenamed
//
// 本函数只关心工作区状态（第二个字符），因为 FileSelector 只显示当前目录条目。
func parsePorcelain(out []byte) map[string]statusKind {
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

		// 取文件名（只显示当前目录条目名）
		name := filepath.Base(path)
		if name == "." || name == "" {
			continue
		}

		// 映射工作区状态
		var st statusKind
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
