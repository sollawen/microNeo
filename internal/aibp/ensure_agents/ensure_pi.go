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

const aibpPiSpec = "npm:aibp-pi" // 不 pin 版本（D5）

// PiEnsurer 是 pi 的 aibp 扩展自举实现。
type PiEnsurer struct{}

var _ AgentEnsurer = PiEnsurer{} // 编译期接口断言（同包可用）

func (PiEnsurer) AgentName() string { return "pi" }

func (PiEnsurer) HasAgent() bool {
	_, err := exec.LookPath("pi")
	return err == nil
}

// piAgentDir 返回 pi 的 agent 配置目录（镜像 pi 的 getAgentDir()，见 pi 源码 dist/config.js:412-419）。
// 优先读 PI_CODING_AGENT_DIR 环境变量（pi 实际的覆盖机制，见 config.js:413；变量名由 APP_NAME 派生，
// pi 的 APP_NAME="pi" 所以是 PI_CODING_AGENT_DIR）。
// ~ 展开规则镜像 pi 的 normalizePath（见 pi 源码 dist/utils/paths.js:48-52）："~"→$HOME，"~/x"→$HOME/x。
// 注意：设了环境变量时 pi 直接用展开结果作 agent 目录，不附加 .pi/agent（与未设时的 fallback 不同）。
//
// 派生路径（与 pi 读写一致）：
//   settings.json:                   <agentDir>/settings.json
//   aibp 扩展的 package.json:        <agentDir>/npm/node_modules/aibp-pi/package.json
//                                    （npm: spec 走 agentDir/npm，见 pi package-manager.js:1597）
func piAgentDir() string {
	if env := os.Getenv("PI_CODING_AGENT_DIR"); env != "" {
		if env == "~" {
			home, _ := os.UserHomeDir()
			return home
		}
		if strings.HasPrefix(env, "~/") {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, env[2:])
		}
		return env
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pi", "agent")
}

func piSettingsPath() string {
	return filepath.Join(piAgentDir(), "settings.json")
}

// piReadSetting 读取 pi settings.json 的 packages[]。
func piReadSetting() []string {
	b, err := os.ReadFile(piSettingsPath())
	if err != nil {
		return nil
	}
	var s struct{ Packages []string `json:"packages"` }
	if err := json.Unmarshal(b, &s); err != nil {
		return nil
	}
	return s.Packages
}

// AIBPVersion：识别本 agent 已装的 aibp。
//   - 包含 "aibp-agents" → 源码路径，返回 (0, 0, true)（不读盘；信任假设）
//   - 包含 "npm:aibp-pi" → npm 包，读取 <agentDir>/npm/node_modules/aibp-pi/package.json 的协议
//   - 未找到 / 损坏 → (0, 0, false)
func (PiEnsurer) AIBPVersion() (int, int, bool) {
	for _, entry := range piReadSetting() {
		if strings.Contains(entry, "aibp-agents") {
			return 0, 0, true // 源码路径：不读盘
		}
		if strings.Contains(entry, "npm:aibp-pi") {
			return piNpmAIBPVersion() // npm 包：读版本号
		}
	}
	return 0, 0, false // 没找到 aibp 条目
}

// piNpmAIBPVersion 读取 aibp-pi 的 npm 安装版本的协议。
// 协议来源：package.json 的 aibp.protocol 交 aibp.ParseProtocol 解析。
func piNpmAIBPVersion() (int, int, bool) {
	pkgPath := filepath.Join(piAgentDir(), "npm", "node_modules", "aibp-pi", "package.json")
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

func (PiEnsurer) InstallAIBP() error {
	cmd := exec.Command("pi", "install", aibpPiSpec) // 不带 @版本（D5）
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pi install 失败（可能 pi 过旧，请升级 pi）: %w", err)
	}
	return nil
}

func (PiEnsurer) UpdateAIBP() error {
	cmd := exec.Command("pi", "update", aibpPiSpec)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pi update 失败: %w", err)
	}
	return nil
}