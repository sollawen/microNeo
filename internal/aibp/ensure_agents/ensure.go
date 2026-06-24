package ensure_agents

import (
	"errors"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

var (
	errAgentNotFound     = errors.New("agent not found, please install it first")
	errExtensionOutdated = errors.New("aibp 扩展协议版本过旧，请运行 `pi update npm:aibp-pi` 后重启 pi")
	errMicroNeoOutdated  = errors.New("aibp 扩展协议版本较新，请升级 microNeo")
)

// Reporter 汇报进度 / 状态消息的回调。
// 签名的格式由调用方（action/command_neo.go InfoBarNow）定义。
type Reporter func(msg string)

// AgentEnsurer 描述一个 agent 的 aibp 扩展自举。
// 不同 agent 的扩展机制差异大，各自实现这五个方法吸收差异。
type AgentEnsurer interface {
	AgentName() string // "pi" / "opencode"——日志和报错用

	HasAgent() bool           // 本机有没有这个 agent 程序
	HasAIBP() bool            // 该 agent 装没装 aibp 扩展
	AIBPVersion() (string, error) // 已装扩展实现的协议（如 "aibp-1"）。
	                               //   注意：协议版本，非包版本。读静态声明（package.json），不启动 agent
	InstallAIBP() error       // 装 aibp 扩展到该 agent
}

// Ensure 是统一编排逻辑，各 ensure_<agent>.go 共用。
// 返回的 error 由调用方（action/command_neo.go）决定如何交互（InfoBar 提示等）——
// 本函数不预设交互模式，不依赖 action 包。
//
// report 会在每一步业务进展和每一种结果时被调用，调用方负责渲染到 UI。
// 如果 report 为 nil，则静默执行。
func Ensure(e AgentEnsurer, report Reporter) error {
	if report == nil {
		report = func(string) {}
	}
	prefix := "aibp-" + e.AgentName()
	report("checking " + e.AgentName() + " ...")
	if !e.HasAgent() {
		report(e.AgentName() + " not found, please install it first")
		return errAgentNotFound
	}
	if !e.HasAIBP() {
		report(prefix + " downloading.....")
		if err := e.InstallAIBP(); err != nil {
			report(prefix + " install failed: " + err.Error())
			return err
		}
		report(prefix + " installed")
	}
	report("checking " + prefix + " protocol version ...")
	ext, err := e.AIBPVersion()
	if err != nil {
		report(prefix + " reinstalling (corrupted) ...")
		if err := e.InstallAIBP(); err != nil {
			report(prefix + " install failed: " + err.Error())
			return err
		}
		ext, err = e.AIBPVersion()
		if err != nil {
			report(prefix + " version still invalid after reinstall: " + err.Error())
			return err
		}
	}
	switch extMajor, mine := aibp.MajorVersion(ext), aibp.MajorVersion(aibp.Protocol); {
	case extMajor == mine:
		report(prefix + " ready")
		return nil
	case extMajor < mine:
		report(e.AgentName() + " protocol outdated, please upgrade")
		return errExtensionOutdated
	default:
		report("microNeo protocol outdated, please upgrade")
		return errMicroNeoOutdated
	}
}
