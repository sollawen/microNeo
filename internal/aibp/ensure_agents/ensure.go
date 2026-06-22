package ensure_agents

import (
	"errors"
	"fmt"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

var (
	errExtensionOutdated = errors.New("aibp 扩展协议版本过旧，请运行 `pi update npm:aibp-pi` 后重启 pi")
	errMicroNeoOutdated  = errors.New("aibp 扩展协议版本较新，请升级 microNeo")
)

// AgentEnsurer 描述一个 agent 的 aibp 扩展自举。
// 不同 agent 的扩展机制差异大，各自实现这五个方法吸收差异。
type AgentEnsurer interface {
	AgentName() string // "pi" / "opencode"——日志和报错用

	HasAgent() bool    // 本机有没有这个 agent 程序
	HasAIBP() bool     // 该 agent 装没装 aibp 扩展
	AIBPVersion() (string, error) // 已装扩展实现的协议（如 "aibp-1"）。
	                               //   注意：协议版本，非包版本。读静态声明（package.json），不启动 agent
	InstallAIBP() error // 装 aibp 扩展到该 agent
}

// Ensure 是统一编排逻辑，各 ensure_<agent>.go 共用。
// 返回的 error 由调用方（action/command_neo.go）决定如何交互（InfoBar 提示等）——
// 本函数不预设交互模式，不依赖 action 包。
func Ensure(e AgentEnsurer) error {
	if !e.HasAgent() {
		return fmt.Errorf("未检测到 %s，请先安装", e.AgentName())
	}
	if !e.HasAIBP() {
		return e.InstallAIBP()
	}
	ext, err := e.AIBPVersion()
	if err != nil {
		return e.InstallAIBP() // 读不到协议 = 没装，重装
	}
	switch extMajorVersion, mineMajorVersion := aibp.MajorVersion(ext), aibp.MajorVersion(aibp.Protocol); {
	case extMajorVersion == mineMajorVersion:
		return nil // 兼容
	case extMajorVersion < mineMajorVersion:
		return errExtensionOutdated // D9：只提示
	default:
		return errMicroNeoOutdated
	}
}
