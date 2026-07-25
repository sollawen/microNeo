package finder

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/tcell/v2"
)

// ---- 预览常量 ----

const (
	previewMaxBytes      = 256 * 1024
	previewBinProbeBytes = 8 * 1024
	previewMinWidth      = 10
	previewScrollStep    = 3
)

// preview 是右侧文件预览 region，只实现 NoKeyboardRegion（不接键盘、不参与 focus）。
type preview struct {
	rect Rect // 预览区矩形；Open 注入，生命周期内不变

	path     string // 当前预览的文件路径；空 = 无可预览
	topLine  int    // 首行在 previewLines 中的索引
	readErr  bool   // 读取失败
	binary   bool   // 二进制文件
	truncated bool  // 文件超大被截断

	// previewLines 在 Load 内按需填充，用完即弃
	previewLines []string
}

// newPreview 构造 preview region。
func newPreview(rect Rect) *preview {
	return &preview{rect: rect}
}

// ---- NoKeyboardRegion ----

func (p *preview) Rect() Rect { return p.rect }

func (p *preview) Display() {
	r := p.rect
	if r.W < previewMinWidth {
		return
	}
	// 清屏（盖底层编辑区，避开上下边框行）
	clearRect(r.X, r.Y+1, r.W, r.H-2, config.DefStyle)
	x, y, w, h := r.X, r.Y+1, r.W, r.H-2
	switch {
	case p.readErr:
		p.drawCenteredText(x, y, w, h, "Unable to preview", config.DefStyle)
	case p.binary:
		p.drawCenteredText(x, y, w, h, "Binary file", config.DefStyle)
	case p.path == "" || len(p.previewLines) == 0:
		p.drawCenteredText(x, y, w, h, "Select a file", config.DefStyle)
	default:
		p.drawPreviewBody(x, y, w, h, config.DefStyle)
	}
}

func (p *preview) HandleMouse(ev *tcell.EventMouse) bool {
	switch ev.Buttons() {
	case tcell.WheelUp:
		p.scroll(-previewScrollStep)
		return true
	case tcell.WheelDown:
		p.scroll(previewScrollStep)
		return true
	}
	return false
}

// ---- 预览加载 ----

// Load 载入一个文本文件用于预览。path 与当前相同则 no-op（保留滚动位置）。
func (p *preview) Load(path string) {
	if p.path == path {
		return
	}
	p.path = path
	p.previewLines = nil
	p.truncated = false
	p.binary = false
	p.readErr = false
	p.topLine = 0

	if path == "" {
		return
	}

	f, err := os.Open(path)
	if err != nil {
		p.readErr = true
		return
	}
	defer f.Close()

	buf := make([]byte, previewMaxBytes+1)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	if n > previewMaxBytes {
		p.truncated = true
		buf = buf[:previewMaxBytes]
	}

	probeEnd := len(buf)
	if probeEnd > previewBinProbeBytes {
		probeEnd = previewBinProbeBytes
	}
	if bytes.ContainsRune(buf[:probeEnd], 0) {
		p.binary = true
		return
	}

	p.previewLines = strings.Split(string(buf), "\n")
	if p.truncated {
		p.previewLines = append(p.previewLines, " (truncated)")
	}
}

// Clear 重置预览状态（目录/面包屑等占位场景调用）。
func (p *preview) Clear() {
	p.path = ""
	p.truncated = false
	p.binary = false
	p.readErr = false
}

// ---- 绘制 ----

func (p *preview) drawPreviewBody(x, y, w, h int, style tcell.Style) {
	sy := y
	for i := p.topLine; i < len(p.previewLines) && sy < y+h; i++ {
		drawString(x, sy, w, p.previewLines[i], style)
		sy++
	}
	for ; sy < y+h; sy++ {
		clearRect(x, sy, w, 1, style)
	}
}

func (p *preview) drawCenteredText(x, y, w, h int, text string, style tcell.Style) {
	if w < 1 || h < 1 {
		return
	}
	tw := stringWidth(text)
	sx := x + (w-tw)/2
	if sx < x {
		sx = x
	}
	sy := y + h/2
	drawString(sx, sy, w-(sx-x), text, style)
}

func (p *preview) scroll(delta int) {
	if len(p.previewLines) == 0 {
		return
	}
	visible := p.rect.H - 2
	if visible < 1 {
		return
	}
	maxTop := len(p.previewLines) - visible
	if maxTop < 0 {
		maxTop = 0
	}
	top := p.topLine + delta
	if top < 0 {
		top = 0
	}
	if top > maxTop {
		top = maxTop
	}
	p.topLine = top
}

