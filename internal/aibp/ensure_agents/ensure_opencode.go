package ensure_agents

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// HasAIBP：检查 opencode tui.json 的 plugin[] 里是否已登记 aibp-opencode。
//
// 形态（实测）：
//   - "aibp-opencode"           （unpinned，本计划期望形态）
//   - "aibp-opencode@1.0.1"     （pinned 到具体版本；用户手改或历史遗留）
//
// 兼容两种（D17 D5 同款策略）。
func (OpencodeEnsurer) HasAIBP() bool {
	b, err := os.ReadFile(opencodeTuiPath())
	if err != nil {
		return false // 读不到 → 视为未登记
	}
	var s struct {
		Plugin []string `json:"plugin"`
	}
	if json.Unmarshal(b, &s) != nil {
		return false
	}
	for _, p := range s.Plugin {
		if p == aibpOpencodeSpec || strings.HasPrefix(p, aibpOpencodeSpec+"@") {
			return true
		}
	}
	return false
}

// AIBPVersion：读已装扩展的 package.json 的 aibp.protocol 字段。
//
// 包位置（实测 opencode 1.17.9）：
//   ~/.cache/opencode/packages/aibp-opencode@latest/node_modules/aibp-opencode/package.json
//
// 注意：用 opencodeCacheDir() 而不是 os.UserCacheDir()——macOS 下两者不一致。
// 注意 @latest 缓存 key：D3 通过 InstallAIBP 先 rm -rf 缓存再安装，保证 cache 不陈旧。
// 若 cache 文件本身被外部破坏（用户手 rm -rf），Ensure() 会视作「没装」触发 Install 自愈。
func (OpencodeEnsurer) AIBPVersion() (string, error) {
	pkgPath := filepath.Join(
		opencodeCacheDir(), "packages", "aibp-opencode@latest",
		"node_modules", "aibp-opencode", "package.json",
	)
	b, err := os.ReadFile(pkgPath)
	if err != nil {
		return "", err
	}
	var pkg struct {
		AIBP struct {
			Protocol string `json:"protocol"`
		} `json:"aibp"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return "", err
	}
	if pkg.AIBP.Protocol == "" {
		return "", fmt.Errorf("package.json 缺少 aibp.protocol 字段（视为没装）")
	}
	return pkg.AIBP.Protocol, nil
}

func (OpencodeEnsurer) InstallAIBP() error {
	cacheDir := filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@latest")
	// D3：npm 缓存不自动刷新，必须先 rm -rf 缓存目录再安装
	if cacheErr := os.RemoveAll(cacheDir); cacheErr != nil {
		// 缓存目录不存在是正常的（首次安装），不阻断
	}
	cmd := exec.Command("opencode", "plugin", aibpOpencodeSpec, "-g")
	if err := cmd.Run(); err != nil {
		// D4：opencode plugin 失败不兜底。唯一现实风险是 opencode 太旧没有 plugin 子命令。
		return fmt.Errorf("opencode plugin 失败（可能 opencode 过旧，请升级 opencode）: %w", err)
	}
	return nil
}
