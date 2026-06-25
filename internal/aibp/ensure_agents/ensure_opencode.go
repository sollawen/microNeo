package ensure_agents

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

const aibpOpencodeSpec = "aibp-opencode" // 不 pin 版本（D5）

// OpencodeEnsurer 是 opencode 的 aibp 扩展自举实现。
type OpencodeEnsurer struct{}

var _ AgentEnsurer = OpencodeEnsurer{} // 编译期接口断言

func (OpencodeEnsurer) AgentName() string { return "opencode" }

func (OpencodeEnsurer) HasAgent() bool {
	_, err := exec.LookPath("opencode")
	return err == nil
}

// opencodeConfigDir 返回 opencode 的全局配置目录。
//
// 派生路径（与 opencode 1.17.9 的 XDG 行为一致）：
//   tui.json:           <configDir>/tui.json        （TUI 插件登记表；D19 §源码结构）
//   opencode.json:      <configDir>/opencode.json   （server 插件 + MCP；本计划不读）
//   package.json:       <configDir>/../cache/opencode/packages/aibp-opencode@latest/node_modules/aibp-opencode/package.json
//
// 注意：opencode 无 PI_CODING_AGENT_DIR 类环境变量覆盖机制——D8 硬编码。
func opencodeConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opencode")
}

func opencodeTuiPath() string {
	return filepath.Join(opencodeConfigDir(), "tui.json")
}

// opencodeCacheDir 返回 opencode 的包缓存目录。
//
// opencode 使用 xdg-basedir npm 包确定缓存路径：
//   - XDG_CACHE_HOME 设了 → $XDG_CACHE_HOME/opencode
//   - macOS fallback → ~/.cache/opencode
//   - Linux fallback → ~/.cache/opencode
//   - Windows fallback → %LOCALAPPDATA%/opencode
//
// Go os.UserCacheDir() 在 macOS 返回 ~/Library/Caches，与 xdg-basedir 不一致。
// 所以不能用 os.UserCacheDir()，必须自己实现相同的 fallback 逻辑。
func opencodeCacheDir() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "opencode")
}

// opencodeReadTui 读取 opencode tui.json 的 plugin[]。
func opencodeReadTui() []string {
	b, err := os.ReadFile(opencodeTuiPath())
	if err != nil {
		return nil
	}
	var s struct{ Plugin []string `json:"plugin"` }
	if err := json.Unmarshal(b, &s); err != nil {
		return nil
	}
	return s.Plugin
}

// AIBPVersion：识别本 agent 已装的 aibp。
//   - 包含 "aibp-agents" → 源码路径，返回 (0, 0, true)（不读盘；信任假设）
//   - 包含 "aibp-opencode" → npm 包，读取缓存的 package.json 的协议
//   - 未找到 / 损坏 → (0, 0, false)
func (OpencodeEnsurer) AIBPVersion() (int, int, bool) {
	for _, entry := range opencodeReadTui() {
		if strings.Contains(entry, "aibp-agents") {
			return 0, 0, true // 源码路径：不读盘
		}
		if strings.Contains(entry, "aibp-opencode") {
			return opencodeNpmAIBPVersion() // npm 包：读版本号
		}
	}
	return 0, 0, false // 没找到 aibp 条目
}

// opencodeNpmAIBPVersion 读取 aibp-opencode 的 cache 安装版本的协议。
// 协议来源：package.json 的 aibp.protocol 交 aibp.ParseProtocol 解析。
func opencodeNpmAIBPVersion() (int, int, bool) {
	pkgPath := filepath.Join(
		opencodeCacheDir(), "packages", "aibp-opencode@latest",
		"node_modules", "aibp-opencode", "package.json",
	)
	b, err := os.ReadFile(pkgPath)
	if err != nil {
		return 0, 0, false
	}
	var pkg struct {
		AIBP struct{ Protocol string `json:"protocol"` } `json:"aibp"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return 0, 0, false
	}
	if pkg.AIBP.Protocol == "" {
		return 0, 0, false
	}
	return aibp.ParseProtocol(pkg.AIBP.Protocol)
}

// installOrUpdate 是 opencode 的安装/更新通用实现。
// opencode 无原生 update 命令，清 cache + 重装是唯一强制刷新路径（D24 调研结论）。
func (e OpencodeEnsurer) installOrUpdate() error {
	cacheDir := filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@latest")
	_ = os.RemoveAll(cacheDir)
	cmd := exec.Command("opencode", "plugin", aibpOpencodeSpec, "-g")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("opencode plugin 失败（可能 opencode 过旧，请升级 opencode）: %w", err)
	}
	return nil
}

func (OpencodeEnsurer) InstallAIBP() error {
	return OpencodeEnsurer{}.installOrUpdate()
}

func (OpencodeEnsurer) UpdateAIBP() error {
	return OpencodeEnsurer{}.installOrUpdate()
}