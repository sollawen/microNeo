package action

import "github.com/micro-editor/micro/v2/internal/aibp/ensure_agents"

// RegisterCommands 注册 microNeo 所有自己的命令。
// 用 MakeCommand API（micro 为插件/扩展设计的注册入口），避免侵入 command.go 的 InitCommands map。
// 必须在 InitCommands() 之后调用（commands map 已初始化），由 InitGlobals 触发。
// 未来加新命令：在此函数体内追加 MakeCommand 行即可。
func RegisterCommands() {
	MakeCommand("check-agent", (*BufPane).CheckAibpCmd, nil)
}

// CheckAibpCmd 是 :check-agent 命令的处理函数。
// 检查 pi 是否装了 aibp-pi 扩展；没装则安装，装了则校验协议版本兼容性。
// 用户主动运行（非启动自动），可给明确反馈。
func (h *BufPane) CheckAibpCmd(args []string) {
	if err := ensure_agents.Ensure(ensure_agents.PiEnsurer{}); err != nil {
		InfoBar.Message("aibp-pi: " + err.Error())
		return
	}
	InfoBar.Message("aibp-pi: 就绪")
}
