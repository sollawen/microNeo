package finder

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// ---- 预览常量 ----

const (
	previewMaxBytes      = 256 * 1024 // 单文件预览读取上限；超出截断
	previewBinProbeBytes = 8 * 1024   // 二进制探测窗口（前 N 字节含 NUL 即判二进制）
	previewMinWidth      = 10         // 预览区宽低于此则不绘预览
	previewScrollStep    = 3          // 每次滚轮滚动行数
)

// ---- previewState ----

// previewState 是文件预览的可变状态。仅主线程读写（reload / load 在 handleKey 链上，
// draw 在 Display 链上，均主线程；fetchGit 后台 goroutine 只碰 git 字段，不碰预览），
// 故无需 mu。
type previewState struct {
	path         string   // 当前预览文件的绝对路径；"" = 无预览
	previewLines []string // 按 '\n' 切的原始行（一原始行 = preview 一屏行）
	truncated    bool     // 文件超大被截断（仅读前 previewMaxBytes）
	binary       bool     // 判定为二进制（前 N 字节含 NUL）
	readErr      bool     // 读取失败（权限/IO 等）
	topLine      int      // 滚动位置：previewLines 数组下标（原始行号）
}

// ---- Session 预览方法 ----

// refreshPreview 按当前光标刷新预览（统一入口）。moveCursor / chdirTo / toggleHidden
// 末尾各调一次。同步读盘 < 10ms（256KB 本地盘），path 未变时 loadFile 内部 no-op
// 保留滚动位置。
func (fm *Session) refreshPreview() {
	s := fm.state

	// gate 1：cursor 不合法
	if s.cursor < 1 || s.cursor-1 >= len(s.showEntries) {
		fm.clearPreview()
		return
	}
	e := s.showEntries[s.cursor-1]

	// gate 2：当前光标是目录
	if e.isDir {
		fm.clearPreview()
		return
	}

	// 真加载
	fm.loadFile(filepath.Join(s.currentDir, e.name))
}

// loadFile 载入一个文本文件用于预览。path 与当前相同则 no-op（保留滚动位置）。
// 二进制（前 previewBinProbeBytes 含 NUL）/ 读失败 / 截断分别置标志。文本内容按
// '\n' 切原始行存进 previewLines，运行时逐行 drawString，超宽右半自然截断。
// 在主线程 handleKey 链上同步调用（256KB 本地盘 < 10ms，不卡）。
func (fm *Session) loadFile(path string) {
	pv := fm.state.preview

	// gate 3：缓存命中（光标停在同一文件不动 / 重复 trigger）
	if pv.path == path {
		return
	}
	pv.path = path
	pv.previewLines = nil
	pv.truncated = false
	pv.binary = false
	pv.readErr = false
	pv.topLine = 0

	if path == "" {
		return // 空 path 不读盘（drawPreview 走占位文案）
	}

	f, err := os.Open(path)
	if err != nil {
		pv.readErr = true
		return
	}
	defer f.Close()

	// 多读 1 字节探截断：读满 N+1 字节 = 文件 > N（未截断时 ReadFull 在 EOF 返回
	// ErrUnexpectedEOF + 全量字节数，忽略错误）。
	buf := make([]byte, previewMaxBytes+1)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	if n > previewMaxBytes {
		pv.truncated = true
		buf = buf[:previewMaxBytes]
	}

	probeEnd := len(buf)
	if probeEnd > previewBinProbeBytes {
		probeEnd = previewBinProbeBytes
	}
	if bytes.ContainsRune(buf[:probeEnd], 0) {
		pv.binary = true
		return // 不再 split：避免二进制内容被当文本一行超长
	}

	pv.previewLines = strings.Split(string(buf), "\n")
	if pv.truncated {
		pv.previewLines = append(pv.previewLines, " (truncated)") // 末行标记，画到屏自然出现
	}
}

// clearPreview 重置预览状态（目录/面包屑等占位场景调用）。path 置空、标志复位；
// previewLines / topLine 不清，由 loadFile 重建。
func (fm *Session) clearPreview() {
	pv := fm.state.preview
	pv.path = ""
	pv.truncated = false
	pv.binary = false
	pv.readErr = false
}

// ---- 绘制 ----

// drawPreview 在 Display 链调用：清屏 + 按状态分支绘制。
func (fm *Session) drawPreview() {
	pv := fm.state.preview
	r := fm.state.pvRect
	if r.W < previewMinWidth {
		return // 预览宽不够，整个预览区域不绘（让给 finder）
	}
	// 1) 清屏（盖底层编辑区，避开上下边框行）
	fm.clearRect(r.X, r.Y+1, r.W, r.H-2, config.DefStyle)
	// 2) 按状态分支，内容区在上下边框之间
	x, y, w, h := r.X, r.Y+1, r.W, r.H-2
	switch {
	case pv.readErr:
		fm.drawCenteredText(x, y, w, h, "Unable to preview", config.DefStyle)
	case pv.binary:
		fm.drawCenteredText(x, y, w, h, "Binary file", config.DefStyle)
	case pv.path == "" || len(pv.previewLines) == 0:
		fm.drawCenteredText(x, y, w, h, "Select a file", config.DefStyle)
	default:
		fm.drawPreviewBody(x, y, w, h, config.DefStyle)
	}
}

// drawPreviewBody 在内容区 [x,y,w,h] 内从 topLine 起逐行绘制。previewLines 是按
// '\n' 切的原始行，每条可能远超 w 列——由 drawString 限宽 col+rw > x+w 时 break
// 截断（右半不画、不切半 CJK），尾部 fill space 清底层编辑区。
func (fm *Session) drawPreviewBody(x, y, w, h int, style tcell.Style) {
	pv := fm.state.preview
	sy := y
	for i := pv.topLine; i < len(pv.previewLines) && sy < y+h; i++ {
		fm.drawString(x, sy, w, pv.previewLines[i], style)
		sy++
	}
	for ; sy < y+h; sy++ {
		fm.clearRect(x, sy, w, 1, style)
	}
}

// drawCenteredText 在 [x,y,w,h] 矩形内画一行居中文本。文本超出 w 时由 drawString 截断；
// h < 1 时 no-op。
func (fm *Session) drawCenteredText(x, y, w, h int, text string, style tcell.Style) {
	if w < 1 || h < 1 {
		return
	}
	tw := stringWidth(text)
	sx := x + (w-tw)/2
	if sx < x {
		sx = x
	}
	sy := y + h/2
	fm.drawString(sx, sy, w-(sx-x), text, style)
}

// scrollPreview 按原始行滚动预览。delta 正向下、负向上。可滚动的 topLine 受两端
// 边界约束：上界 0（首行不能被滚出区域），下界 = max(0, len(previewLines)-visible)
// （末行不能被滚出区域；文件完全装得下时 = 0，不能滚动）。
func (fm *Session) scrollPreview(delta int) {
	pv := fm.state.preview
	if len(pv.previewLines) == 0 {
		return // 空预览 / load 未就绪不滚动
	}
	visible := fm.state.pvRect.H - 2 // 预览正文可见行数 = 内容区高 = 外高 - 上下边框
	if visible < 1 {
		return // 区域太小没法滚（防御：drawPreview 也走这里）
	}
	maxTop := len(pv.previewLines) - visible
	if maxTop < 0 {
		maxTop = 0 // 文件短于区域 → 不可滚，等价于 fixed viewport
	}
	top := pv.topLine + delta
	if top < 0 {
		top = 0
	}
	if top > maxTop {
		top = maxTop
	}
	pv.topLine = top
}

// handleRightMouse 处理右栏内的全部鼠标事件（由入口 whereIsMouse 筛选后送达）。
// 滚轮上下 → 滚动 preview 正文；其余事件 no-op。
func (fm *Session) handleRightMouse(ev *tcell.EventMouse) {
	switch ev.Buttons() {
	case tcell.WheelUp:
		fm.scrollPreview(-previewScrollStep)
		screen.Redraw()
	case tcell.WheelDown:
		fm.scrollPreview(previewScrollStep)
		screen.Redraw()
	}
}
