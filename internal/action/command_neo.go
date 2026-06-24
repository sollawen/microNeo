package action

import (
	"time"

	"github.com/micro-editor/micro/v2/internal/aibp/ensure_agents"
	"github.com/micro-editor/micro/v2/internal/screen"
)

func RegisterCommands() {
	MakeCommand("check-agent", (*BufPane).CheckAibpCmd, nil)
	MakeCommand("test-info", (*BufPane).TestInfoCmd, nil)
}

// InfoBarNow 设消息 + 同步刷 InfoBar 那 1 行到终端。
//
// 用于同步阻塞命令（如 :check-agent 安装 aibp-pi 联网几秒）执行期间——
// 主 goroutine 卡在命令里，DoEvent 循环的自动 redraw 不运行，
// backbuffer 变化刷不到终端。本函数手动触发 InfoBar 行刷新。
//
// 必须在主 goroutine 调用。签名匹配 ensure_agents.Reporter。
func InfoBarNow(msg string) {
	screen.Screen.HideCursor()
	InfoBar.Message(msg)
	InfoBar.Display()
	screen.Screen.Show()
}

// CheckAibpCmd 是 :check-agent 命令的处理函数。
// 检查 pi 是否装了 aibp-pi 扩展；没装则安装，装了则校验协议版本兼容性。
// 用户主动运行，可给明确反馈。
//
// Ensure 内部所有需要告诉用户的消息都通过 reporter 通知，
// reporter 直接用 InfoBarNow：签名匹配，无需闭包。
func (h *BufPane) CheckAibpCmd(args []string) {
	_ = ensure_agents.Ensure(ensure_agents.PiEnsurer{}, InfoBarNow)
}

// TestInfoCmd 是 :test-info 命令的处理函数。
// microNeo 内部使用，不在用户文档 / README / 帮助里出现。
func (h *BufPane) TestInfoCmd(args []string) {
	msgs := []string{
		"step 1/5: checking",
		"step 2/5: downloading",
		"step 3/5: installing",
		"step 4/5: verifying",
		"step 5/5: done",
	}
	for _, msg := range msgs {
		InfoBarNow(msg)
		time.Sleep(1 * time.Second)
	}
}