package action

import (
	"strconv"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/dialog"
	"github.com/micro-editor/micro/v2/internal/md"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/util"
	"github.com/micro-editor/tcell/v2"
)

// OutlineCmd 是 :outline 命令的 action：弹出当前 pane 的目录（OutlineDialog），
// Enter 跳到选中标题所在行。非 MD 文件静默忽略。
func (h *BufPane) OutlineCmd(args []string) {
	if !h.Buf.IsMD {
		return
	}
	v := h.GetView() // pane 矩形：总宽高 + 左缘/顶
	w := v.Width * 4 / 10
	if w < 20 {
		w = 20 // 下限：标题至少要 10 列才看得出内容
	}
	if w > v.Width {
		w = v.Width // 弹窗不能比 pane 还宽
	}
	textW := w - 3 // 左右边框各 1 列 + ▲▼ 指示器 1 列
	if textW < 12 {
		InfoBar.Message("pane too narrow for outline")
		return
	}
	outerH := h.BufView().Height // pane 编辑区高，已扣掉 pane 自己的 statusline 行
	scrH, _ := screen.Screen.Size()
	limit := scrH - config.GetInfoBarOffset() - 1 - v.Y // 从 pane 顶到屏幕底允许的外框高
	if outerH > limit {
		outerH = limit // 浮窗不能压到 infobar
	}
	if outerH < 3 {
		InfoBar.Message("pane too short for outline")
		return
	}
	items := md.CollectHeadings(h.Buf, textW)
	if len(items) == 0 {
		InfoBar.Message("No headings found")
		return
	}
	dialog.NewOutlineDialog().Open(items,
		"Outline",
		dialog.Pos{X: v.X + v.Width - w, Y: v.Y}, // 右缘贴 pane 右边，顶边对齐 pane 顶
		dialog.Size{W: w - 2, H: outerH - 2},     // 纯内容区尺寸，不含边框
		tcell.Style{}, tcell.Style{},
		func(picked *string) {
			if picked == nil {
				return // Esc / resize / 开窗失败
			}
			line := parseTrailingInt(*picked)
			if line < 1 {
				return // 解析失败当作取消处理
			}
			line = util.Clamp(line-1, 0, h.Buf.LinesNum()-1) // 1 基转回 buffer 的 0 基并夹住
			h.RemoveAllMultiCursors()
			h.Cursor.Deselect(true)
			h.GotoLoc(buffer.Loc{X: 0, Y: line})
		})
}

// parseTrailingInt 从条目串尾部解析行号。
// 行号 = 串尾连续十进制数字，且数字前一个字符必须是分隔符 ･
// （保证尾部数字不会和标题里的数字粘连）；不满足则返回 0。
func parseTrailingInt(s string) int {
	runes := []rune(s)
	i := len(runes)
	for i > 0 && runes[i-1] >= '0' && runes[i-1] <= '9' {
		i--
	}
	if i == len(runes) || i == 0 {
		return 0 // 无尾部数字 / 整串都是数字（无分隔符）
	}
	if runes[i-1] != '･' {
		return 0
	}
	n, err := strconv.Atoi(string(runes[i:]))
	if err != nil {
		return 0
	}
	return n
}
