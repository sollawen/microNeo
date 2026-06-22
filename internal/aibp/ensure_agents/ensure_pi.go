package ensure_agents

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// HasAIBP：检查 pi settings.json 的 packages 里是否已登记 aibp-pi。
// 兼容 "npm:aibp-pi"（unpinned，期望形态）和 "npm:aibp-pi@x.y.z"（pinned，历史遗留）。
func (PiEnsurer) HasAIBP() bool {
	b, err := os.ReadFile(piSettingsPath())
	if err != nil {
		return false // 读不到 → 视为未登记（首次安装）
	}
	var s struct{ Packages []string `json:"packages"` }
	if json.Unmarshal(b, &s) != nil {
		return false
	}
	for _, p := range s.Packages {
		if p == aibpPiSpec || strings.HasPrefix(p, aibpPiSpec+"@") {
			return true
		}
	}
	return false
}

// AIBPVersion：读已装扩展的 package.json 的 aibp.protocol 字段。
//   扩展位置 = <piAgentDir>/npm/node_modules/aibp-pi/package.json（与 settings.json 同源）
//   读不到（文件缺失/损坏/字段缺失）→ 返回 error，由 Ensure 视作「没装」触发 Install。
//   协议版本的单一事实来源是 package.json 的 aibp.protocol——aibp-pi 启动时也读它派生注册表
//   （见 aibp-agents/pi/index.ts:11-12），所以静态检测与运行时必然一致。
func (PiEnsurer) AIBPVersion() (string, error) {
	pkgPath := filepath.Join(piAgentDir(), "npm", "node_modules", "aibp-pi", "package.json")
	b, err := os.ReadFile(pkgPath)
	if err != nil {
		return "", err
	}
	var pkg struct{ AIBP struct{ Protocol string `json:"protocol"` } `json:"aibp"` }
	if err := json.Unmarshal(b, &pkg); err != nil {
		return "", err
	}
	// 字段缺失（合法 JSON 但无 aibp.protocol）也视为读不到——Ensure() 会据此重装自愈。
	// 否则 MajorVersion("") 返回 -1，Ensure() 会误报"扩展过旧"，用户跑了 pi update 也修复不了。
	if pkg.AIBP.Protocol == "" {
		return "", fmt.Errorf("package.json 缺少 aibp.protocol 字段（视为没装）")
	}
	return pkg.AIBP.Protocol, nil
}

func (PiEnsurer) InstallAIBP() error {
	cmd := exec.Command("pi", "install", aibpPiSpec) // 不带 @版本（D5）
	if err := cmd.Run(); err != nil {
		// D3：pi install 失败不兜底。唯一现实风险是 pi 太旧没有 install 子命令。
		return fmt.Errorf("pi install 失败（可能 pi 过旧，请升级 pi）: %w", err)
	}
	return nil
}
