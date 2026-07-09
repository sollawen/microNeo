package ensure_agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

// allEnsurers 注册所有已知 agent 的 aibp 扩展自举实现。
// 新加 agent 只需追加一行：把对应的 <Agent>Ensurer{} 加进 slice。
//
// 注意：allEnsurers 与 Pi/Opencode/... 文件的相对顺序无所谓，
// 但建议把"主要 / 优先 / 更普及"的 agent 放前面（D11 §4.1 名字池顺序逻辑类似）。
var allEnsurers = []AgentEnsurer{
	PiEnsurer{},
	OpencodeEnsurer{},
	ClaudeEnsurer{},
}

var (
	errAgentNotFound    = errors.New("agent not found, please install it first")
	errMicroNeoOutdated = errors.New("aibp 扩展协议版本较新，请升级 microNeo")
)

// EnsureAll 对所有已注册 agent 执行 Ensure 编排（跳过未安装的）。
// report 透传给各 agent 的 Ensure；nil 则静默。
// 返回 hadError：是否有 agent 出错，调用方据此决定退出码。
func EnsureAll(report Reporter) (hadError bool) {
	if report == nil {
		report = func(string) {}
	}
	for _, e := range allEnsurers {
		if !e.HasAgent() {
			report(e.AgentName() + ": not installed, skipping")
			continue
		}
		if err := Ensure(e, report); err != nil {
			hadError = true
		}
	}
	return
}

// Reporter 汇报进度 / 状态消息的回调。
// 签名格式由调用方（cmd/micro/micro_neo.go 的 fmt.Println）定义。
type Reporter func(msg string)

// AgentEnsurer 描述一个 agent 的 aibp 扩展自举。
// 不同 agent 的扩展机制差异大，各自实现这六个方法吸收差异。
type AgentEnsurer interface {
	AgentName() string // "pi" / "opencode"——日志和报错用

	HasAgent() bool // 本机有没有这个 agent 程序

	// AIBPVersion：识别本 agent 已装的 aibp。
	// 返回 (major, minor, isSourceInstall, version)。
	//   - 源码路径安装 → isSourceInstall=true，major/minor=0，version=""（不读盘；编排跳过；信任假设）
	//   - npm/cache/marketplace 安装 → 读到协议 + 版本号 → isSourceInstall=false，version="X.Y.Z"
	//   - 未装 / 损坏 / 读不到协议 → (0, 0, false, "")（编排触发 InstallAIBP）
	// 约定：合法协议大号从 1 起；major==0 即「无法识别」；version 仅 npm/marketplace 形态非空。
	// version 供 UpdateToLatest（D27）一次调用同时拿 isSource + installed，免得绕调内部 helper 重读 package.json。
	AIBPVersion() (major, minor int, isSourceInstall bool, version string)

	InstallAIBP() error // 装 aibp 扩展到该 agent（首次 / 损坏自愈）
	UpdateAIBP() error  // 升 aibp 扩展到最新 npm 包（过旧时）

	// UpdateToLatest 把该 agent 的 aibp 扩展升到最新发布版（三态分发，见 D27 §3.1）。
	// report 汇报进度 / 结果。各 agent 自行决定语义：
	//   - opencode/claude：真正干 source→npm / 装 / 查 registry 重装的活
	//   - pi：no-op + report "self-manages upgrades, skipping"（pi 有自己的 in-app 升级提示，
	//     microNeo 不应插手；返回 nil 表示「无需 microNeo 更新，非错误」）
	UpdateToLatest(report Reporter) error
}

// Ensure 是统一编排逻辑，各 ensure_<agent>.go 共用。
// 返回的 error 由调用方（cmd/micro/micro_neo.go 的 DoCheckAgent）决定如何处理（退出码等）——
// 本函数不预设交互模式，不依赖任何 UI / action 包。
//
// report 会在每一步业务进展和每一种结果时被调用，调用方负责渲染到 UI。
// 如果 report 为 nil，则静默执行。
func Ensure(e AgentEnsurer, report Reporter) error {
	if report == nil {
		report = func(string) {}
	}
	prefix := "aibp-" + e.AgentName()
	report("checking " + e.AgentName() + " ...")
	if !e.HasAgent() { // 兜底；EnsureAll 已先过滤
		report(e.AgentName() + " not found")
		return errAgentNotFound
	}

	major, minor, isSource, _ := e.AIBPVersion() // version 仅 UpdateToLatest 用，Ensure 丢弃
	mineMajor, _, _ := aibp.ParseProtocol(aibp.Protocol) // =2

	switch {
	case isSource:
		report(prefix + " source install, skipping")
		return nil
	case major == 0: // 无法识别 / 未装
		report(prefix + " not installed, installing ...")
		if err := e.InstallAIBP(); err != nil {
			report(prefix + " install failed: " + err.Error())
			return err
		}
		report(prefix + " installed")
		return nil
	case major < mineMajor:
		report(prefix + fmt.Sprintf(" outdated (aibp-%d < aibp-%d), updating ...", major, mineMajor))
		if err := e.UpdateAIBP(); err != nil {
			report(prefix + " update failed: " + err.Error())
			return err
		}
		report(prefix + " updated")
		return nil
	case major > mineMajor:
		report("microNeo protocol older than " + prefix + ", please upgrade microNeo")
		return errMicroNeoOutdated
	default: // major == mineMajor（此时一定是 npm 安装，源码已在前面的 case isSource 跳过）
		report(prefix + fmt.Sprintf(" ready (aibp-%d.%d)", major, minor))
		return nil
	}
}

// UpdateAll 对所有已注册 agent 执行「升到最新发布版」编排：
// 跳过未装的；其余调 UpdateToLatest（pi 在自己的 UpdateToLatest 里 no-op 跳过）。
// 返回 hadError：是否有 agent 出错，调用方据此决定退出码。
func UpdateAll(report Reporter) (hadError bool) {
	if report == nil {
		report = func(string) {}
	}
	for _, e := range allEnsurers {
		if !e.HasAgent() {
			report(e.AgentName() + ": not installed, skipping")
			continue
		}
		if err := e.UpdateToLatest(report); err != nil {
			hadError = true
		}
	}
	return
}

// latestNpmVersion 查 npm registry 上 pkg 的 latest 版本号。
// GET https://registry.npmjs.org/<pkg>/latest → JSON .version
// 仅 opencode 用（pi 跳过、claude 走 marketplace），放 ensure.go 作通用域工具。
func latestNpmVersion(pkg string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://registry.npmjs.org/" + pkg + "/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("npm registry returned %d", resp.StatusCode)
	}
	var body struct{ Version string `json:"version"` }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.Version == "" {
		return "", fmt.Errorf("npm registry: empty version")
	}
	return body.Version, nil
}

// semverLess 报 a < b（仅支持 X.Y.Z 纯数字；aibp 包约定无 pre-release tag）。
func semverLess(a, b string) bool {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return len(pa) < len(pb) // 1.0 < 1.0.0
}
