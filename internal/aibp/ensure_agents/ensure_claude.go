package ensure_agents

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

const (
	// 以下三个常量的取值已实测确认（2026-07-03，claude 2.1.199）：
	//   `claude plugin marketplace add sollawen/microNeo-plugins` 后，known_marketplaces.json
	//   存的 key 就是 "microNeo-plugins"；installed_plugins.json 的 plugin key 就是
	//   "aibp-claude@microNeo-plugins"。若未来 marketplace 改名，三常量需同步。
	claudeMarketplaceRepo = "sollawen/microNeo-plugins"                              // owner/repo，喂给 `claude plugin marketplace add`
	claudeMarketplaceName = "microNeo-plugins"                                      // marketplace 名（known_marketplaces.json 的 key）
	claudePluginKey       = "aibp-claude@" + claudeMarketplaceName                    // installed_plugins.json / enabledPlugins / install|enable|update 的 key
)

// ClaudeEnsurer 是 claude 的 aibp 扩展自举实现。
type ClaudeEnsurer struct{}

var _ AgentEnsurer = ClaudeEnsurer{} // 编译期接口断言

func (ClaudeEnsurer) AgentName() string { return "claude" }

func (ClaudeEnsurer) HasAgent() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// claudeConfigDir 返回 claude 的配置目录。
// claude 原生支持 CLAUDE_CONFIG_DIR 覆盖（已隔离测试：set 后 plugin install/list 全走该目录，
// 布局与默认 ~/.claude 一致）；未设 → ~/.claude。
func claudeConfigDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// claudeInstalledRecord 读 installed_plugins.json 里 aibp-claude 的条目。
// 返回 (installPath, version, ok)。文件缺失/损坏/无条目 → ok=false.
//
// installed_plugins.json 结构（claude 实际格式，已读真实文件确认）：
//   { "version": 2, "plugins": { "<plugin>@<market>": [ { scope, installPath, version, ... } ] } }
// 每个 key 映射到一个 slice（支持多 scope 同名）；取第一个（microNeo 装的就是 user scope）。
func claudeInstalledRecord() (installPath, version string, ok bool) {
	b, err := os.ReadFile(filepath.Join(claudeConfigDir(), "plugins", "installed_plugins.json"))
	if err != nil {
		return "", "", false
	}
	var doc struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
			Version     string `json:"version"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return "", "", false
	}
	entries := doc.Plugins[claudePluginKey]
	if len(entries) == 0 {
		return "", "", false
	}
	return entries[0].InstallPath, entries[0].Version, true
}

// claudeEnabled 读 settings.json 的 enabledPlugins[claudePluginKey]。
// 文件缺失/损坏/无 enabledPlugins 键 → false。
// 注：enabledPlugins 键缺失时 EnabledPlugins 是 nil map，nil map 查询返回零值 false（安全，不 panic）。
func claudeEnabled() bool {
	b, err := os.ReadFile(filepath.Join(claudeConfigDir(), "settings.json"))
	if err != nil {
		return false
	}
	var s struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return false
	}
	return s.EnabledPlugins[claudePluginKey] // nil map 查询安全
}

// AIBPVersion：识别 claude 已装的 aibp（仅 marketplace 形态，见 §0.3）。
//   - installed_plugins.json 有 aibp-claude 条目 + settings.json enabled + installPath 下 package.json 可读
//     → 读 aibp.protocol → (major, minor, false)
//   - 任一不满足 → (0, 0, false)（触发 InstallAIBP）
func (ClaudeEnsurer) AIBPVersion() (int, int, bool) {
	installPath, _, ok := claudeInstalledRecord()
	if !ok {
		return 0, 0, false
	}
	if !claudeEnabled() {
		return 0, 0, false // 装了但被 disable → 视为未就绪，让 InstallAIBP 自愈（install+enable）
	}
	return claudeReadProtocol(installPath)
}

// claudeReadProtocol 读 <installPath>/package.json 的 aibp.protocol。
// 与 piNpmAIBPVersion / opencodeNpmAIBPVersion 同构。
func claudeReadProtocol(installPath string) (int, int, bool) {
	b, err := os.ReadFile(filepath.Join(installPath, "package.json"))
	if err != nil {
		return 0, 0, false
	}
	var pkg struct {
		AIBP struct {
			Protocol string `json:"protocol"`
		} `json:"aibp"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return 0, 0, false
	}
	if pkg.AIBP.Protocol == "" {
		return 0, 0, false
	}
	maj, min, parseOK := aibp.ParseProtocol(pkg.AIBP.Protocol)
	if !parseOK {
		return 0, 0, false
	}
	return maj, min, false // marketplace 安装：isSource 恒为 false
}

// claudeMarketplaceAdded 读 known_marketplaces.json 判断 marketplace 是否已登记。
func claudeMarketplaceAdded() bool {
	b, err := os.ReadFile(filepath.Join(claudeConfigDir(), "plugins", "known_marketplaces.json"))
	if err != nil {
		return false
	}
	var doc map[string]json.RawMessage
	return json.Unmarshal(b, &doc) == nil && doc[claudeMarketplaceName] != nil
}

func (ClaudeEnsurer) InstallAIBP() error {
	// 1. marketplace 未登记则补登（已登记跳过，避免重复 add 报错）
	if !claudeMarketplaceAdded() {
		if err := exec.Command("claude", "plugin", "marketplace", "add", claudeMarketplaceRepo).Run(); err != nil {
			return fmt.Errorf("claude marketplace add 失败: %w", err)
		}
	}
	// 2. install（对"已装但 disabled"可能是 no-op，下一步 enable 兜底）
	_ = exec.Command("claude", "plugin", "install", claudePluginKey).Run()
	// 3. enable 兜底（install 默认会 enable；这行专治"已装但被 disable"的自愈场景）
	_ = exec.Command("claude", "plugin", "enable", claudePluginKey).Run()
	// 4. 权威校验：装完该能读到 record（不校验 enabled——enable 失败不阻塞，留给下次自愈）
	if _, _, ok := claudeInstalledRecord(); !ok {
		return fmt.Errorf("claude plugin install 失败：install 后 installed_plugins.json 仍无 aibp-claude 记录（可能 claude 过旧或网络问题）")
	}
	return nil
}

func (ClaudeEnsurer) UpdateAIBP() error {
	cmd := exec.Command("claude", "plugin", "update", claudePluginKey)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude plugin update 失败: %w", err)
	}
	return nil
}
