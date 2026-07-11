package action

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// FileSelector 是文件选择器浮窗（具体浮窗，参见 docs/fileSelect/F1-架构设计方案.md）。
//
// 三层分离（F1 D3 / F1 §5）：
//   - Model：fileSelectorState（目录、条目、光标、git 状态）
//   - View  ：display(area) 三段布局（面包屑 / 文件列表 / hint）
//   - Controller：handleEvent(ev) 键位映射
//
// 生命周期对齐 SelectPane：调用方 new 一个 FileSelector，调 Open(...)，把 display /
// handleEvent 塞给 TheFloatFrame；运行期对象不再被主循环引用（用完即弃，靠 GC）；
// 关闭由 handleEvent 内部发起：先 TheFloatFrame.Close() 再 onSelect(...)。
//
// layout 由本类型自算（pane-local，F1 §7），传 AutoExpand=false 钉死 pane 左上角。

// pane-local 布局常量（F0 §4.3）。
const (
	fsMinWidth  = 20 // 极窄 pane 拒开阈值（列）
	fsMinHeight = 10 // 极矮 pane 拒开阈值（行，保证内容区≥8、文件列表≥6 可滚动）
)

// rowKind 分类光标所在行（F1 §8.2 Enter 上下文语义）。
type rowKind uint8

const (
	rowFile rowKind = iota
	rowBreadcrumb
	rowDir
)

// entry 是已排序、已过滤后的一个目录条目。
type entry struct {
	name  string
	isDir bool
}

// fileSelectorState 是 FileSelector 的可变状态（Model 层，F1 §5 / D3）。
//
// git 状态由后台 goroutine 写、主循环 Display 读，靠 mu（RWMutex）保护（F1 §10.7）。
// Display 读 git 一律走 gitOf()（RLock），绝不裸读 map。
type fileSelectorState struct {
	currentDir string
	entries    []entry // 已排序（目录优先 + 大小写不敏感字母序）、已按 showHidden 过滤
	cursor     int     // 0=面包屑行, 1..len(entries)=条目
	topIdx     int     // 文件列表视口顶（指向 entries 索引）
	showHidden bool
	pickerW    int // 内容区宽（截断用）
	listH      int // 文件列表可见行数（内容区高 - 面包屑 - hint）

	// git 状态（异步、并发保护，F1 §10.7）
	gitStatus map[string]statusKind
	gitOK     bool // false=无 git 列（降级）
	mu        sync.RWMutex

	// 元数据缓存（按条目名缓存，F1d §4.1）
	metaName  string    // 缓存对应的条目名（""=面包屑/空/越界，无缓存）
	metaSize  int64
	metaMtime time.Time
	metaOK    bool      // false=stat 失败（断链/无权限），字段降级显 —
	metaIsDir bool      // = info.IsDir()（跟随符号链接）
}

// setGitStatus 由后台 goroutine 写入 git 查询结果（F1 §10.7）。
func (s *fileSelectorState) setGitStatus(m map[string]statusKind, ok bool) {
	s.mu.Lock()
	s.gitStatus = m
	s.gitOK = ok
	s.mu.Unlock()
}

// gitOf 读取某条目的 git 状态（Display 用，RLock 保护）。
func (s *fileSelectorState) gitOf(name string) (statusKind, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.gitOK {
		return stNone, false
	}
	st, has := s.gitStatus[name]
	return st, has
}

// ---- SelectResult：FileSelector 回调契约（F3 §3）----

// ResultKind 是关闭原因的大类（F3 §3）。
type ResultKind uint8

const (
	// Picked 表示用户选中了文件（Path 有效）。
	Picked ResultKind = iota
	// Closed 表示没选就关了（Reason 有效）。
	Closed
)

// CloseReason 是关闭的具体原因（仅 Kind==Closed 时有效，F3 §3）。
type CloseReason uint8

const (
	ReasonEsc    CloseReason = iota // 用户按 Esc
	ReasonQuit                       // 用户按 Ctrl-q（或 welcome 态的 Quit）
	ReasonSize                       // 打开时窗口过小，selector 从未显示（F5）
	ReasonResize                     // 运行中窗口 resize，已显示后被打断（F5）
)

// SelectResult 承载 FileSelector 的关闭结果（F3 §3）。
// 原来 onSelect 签名是 func(*string)（nil=取消），现在换成带原因的 SelectResult，
// 让调用方能区分「选中文件」和「各种关掉」，welcome 调用方据此决定行为（退出/重开/no-op）。
type SelectResult struct {
	Kind   ResultKind   // Picked | Closed
	Path   string       // Kind==Picked 时：选中的文件绝对路径
	Reason CloseReason  // Kind==Closed 时：关闭原因
}

// FileSelector 是文件选择器浮窗本体。
type FileSelector struct {
	state    *fileSelectorState
	pane     *BufPane
	onSelect func(SelectResult)
	gitCache gitStatusCache
}

// NewFileSelector 返回一个未打开的 FileSelector（gitCache 注入真实实现，F1 §10.6）。
func NewFileSelector() *FileSelector {
	return &FileSelector{gitCache: NewGitStatus()}
}

// Open 打开文件选择器（F1 §3.2 / §6.2 / §10.7 异步时序）。
//
//   pane      发起 :file 的 pane（pane-local 布局 + 选中后开进此 pane）
//   startDir  起始目录（F1 §8.1 / R6）
//   onSelect  回调（SelectResult）；browse/birth 调用方只关心 Picked，quit 调用方按 Reason 分流
//
// 首次渲染绝不阻塞：State init（os.ReadDir，μs 级）→ 列表立即可见、无 git 标志；
// git 后台查询（带 2s ctx），回来后 screen.Redraw() 触发补画（F1 §10.7 第 1-5 步）。
func (fs *FileSelector) Open(pane *BufPane, startDir string, onSelect func(SelectResult)) {
	fs.onSelect = onSelect
	fs.pane = pane

	// 当前 buffer 文件名（光标起始定位用，F0 §5.3）
	currentFile := ""
	if pane.Buf != nil && pane.Buf.AbsPath != "" {
		currentFile = filepath.Base(pane.Buf.AbsPath)
	}

	// 1. State init（os.ReadDir 同步）→ git 空（F1 §10.7 第 1 步）
	fs.state = newState(startDir, currentFile)

	// 2. layout 预检 + 算 spec
	anchor, contentSize, ok := fs.computeLayout(pane)
	if !ok {
		if onSelect != nil {
			onSelect(SelectResult{Kind: Closed, Reason: ReasonSize}) // 预检拒开 → 透明返回（F1 §7.3 / F5 §5.1b）
		}
		return
	}
	fs.state.pickerW = contentSize.W
	fs.state.listH = contentSize.H - 2 // 减面包屑 + hint

	// 3. 打开（首次 Display：列表已可见、无 git 标志，F1 §10.7 第 2 步）
	spec := fs.buildSpec(anchor, contentSize)
	if !TheFloatFrame.Open(spec) {
		if onSelect != nil {
			onSelect(SelectResult{Kind: Closed, Reason: ReasonSize}) // F5 §5.1b：打开时放不下
		}
		return
	}

	// 4. 后台启 git（渐进显示，F1 §10.7 第 3-5 步）—— 绝不阻塞首次渲染
	go fs.fetchGit(fs.state.currentDir)
}

// fetchGit 后台查询某目录的 git 状态并触发重绘（F1 §10.7）。
// 仅当仍停留在该目录时才应用结果，避免快速导航下的竞态污染。
func (fs *FileSelector) fetchGit(dir string) {
	m, ok := fs.gitCache.statusFor(dir) // 阻塞查询，带 2s ctx
	if fs.state == nil || fs.state.currentDir != dir {
		return // 已导航离开，丢弃
	}
	fs.state.setGitStatus(m, ok)
	screen.Redraw() // 通知主循环重绘 → 列表补 git 标志
}

// newState 构造初始 State（F0 §5.3 光标起始 / §5.4 dotfile 自动显隐）。
func newState(dir, currentFile string) *fileSelectorState {
	dir = filepath.Clean(dir)
	s := &fileSelectorState{currentDir: dir}
	// 当前文件是 dotfile 且默认隐藏 → 自动置 showHidden=true（F0 §5.4）
	if isHiddenName(currentFile) {
		s.showHidden = true
	}
	s.entries = listDirEntries(dir, s.showHidden)
	s.locate(currentFile)
	return s
}

// locate 把光标停到当前文件上（F0 §5.3）；找不到 / 无路径 → 首条目。
func (s *fileSelectorState) locate(currentFile string) {
	s.cursor = 1 // 默认首条目（cursor 1..len）
	s.topIdx = 0
	if currentFile == "" {
		return
	}
	for i, e := range s.entries {
		if e.name == currentFile {
			s.cursor = i + 1
			return
		}
	}
}

// ---- listDirEntries / 过滤 / 排序（F0 §5.4 / §5.5）----

// listDirEntries 读目录并返回排好序的条目（目录优先、各自大小写不敏感字母序）。
// showHidden=false 时过滤 dotfile（F0 §5.4）。
func listDirEntries(dir string, showHidden bool) []entry {
	infos, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs, files []entry
	for _, d := range infos {
		name := d.Name()
		if !showHidden && isHiddenName(name) {
			continue
		}
		e := entry{name: name, isDir: d.IsDir()}
		if e.isDir {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	caseSort := func(es []entry) {
		sort.SliceStable(es, func(i, j int) bool {
			return strings.ToLower(es[i].name) < strings.ToLower(es[j].name)
		})
	}
	caseSort(dirs)
	caseSort(files)
	return append(dirs, files...)
}

// isHiddenName 判断 Unix dotfile（F0 §5.4）。
func isHiddenName(name string) bool {
	return len(name) > 0 && name[0] == '.'
}

// ---- layout（F1 §7）----

// computeLayout 算 pane-local 的锚点与内容尺寸（F1 §7.1–7.4），并做预检。
// 返回 ok=false 时已向 InfoBar 提示拒开原因（F1 §7.3）。
func (fs *FileSelector) computeLayout(pane *BufPane) (anchor Pos, contentSize Size, ok bool) {
	view := pane.BWindow.GetView() // *View{X,Y,Width,Height}（范式参照 notepane.go:238）
	avail := view.Height - 1       // 全高模型（F0 §4.1）：留 statusLine

	widthFrac := config.GetGlobalOption("fileselectwidth").(float64)
	pickerW := int(widthFrac * float64(view.Width))
	if pickerW < fsMinWidth {
		pickerW = fsMinWidth
	}
	if pickerW > view.Width {
		pickerW = view.Width // 防御：不溢出到相邻 pane（F0 §3.1）
	}

	// 预检（F1 §7.3 / D2）：拒开条件
	if view.Width < fsMinWidth {
		InfoBar.Message("pane too narrow for file selector (need ", fsMinWidth, " cols)")
		return Pos{}, Size{}, false
	}
	if avail < fsMinHeight {
		InfoBar.Message("pane too short for file selector (need ", fsMinHeight, " rows)")
		return Pos{}, Size{}, false
	}

	// contentSize = 外尺寸 - 边框 2；anchor = pane 左上角（AutoExpand=false 时即外矩形左上）
	return Pos{X: view.X, Y: view.Y}, Size{W: pickerW - 2, H: avail - 2}, true
}

// buildSpec 换算 FloatOpenSpec（F1 §6.1 / §7.4）。
func (fs *FileSelector) buildSpec(anchor Pos, contentSize Size) FloatOpenSpec {
	return FloatOpenSpec{
		Anchor:      anchor,
		ContentSize: contentSize,
		Title:       "Open File",
		FrameColor:  tcell.Style{}, // 零值 → FloatFrame 用 config.DefStyle
		Display:     fs.display,
		HandleEvent: fs.handleEvent,
		AutoExpand:  false, // F1 D2：钉死 pane 左上角，跳过 expandAnchor
		OnCancel: func() { // resize 即关时清理业务回调（F1a）；统一上报 ReasonResize（F3 §4.1f）
			fs.finish(SelectResult{Kind: Closed, Reason: ReasonResize})
		},
	}
}

// ---- View：display(area)（F1 §9 / §7.5）三段布局 ----
//
// 内容区高（area.H）由 minHeight=10 保证恒 ≥8：行 0 面包屑 / 行 1..H-2 文件列表 / 末行 hint。

func (fs *FileSelector) display(area Rect) {
	s := fs.state
	if s == nil {
		return
	}
	revStyle := config.DefStyle.Reverse(true)

	// 行 0：面包屑（目录路径，用 type 色，对齐子目录行，F0 §7.5）
	bcStyle := config.GetColor("type")
	if s.cursor == 0 {
		bcStyle = revStyle
	}
	fs.drawBreadcrumb(area.X, area.Y, area.W, bcStyle)

	// 行 1..H-2：文件列表视口
	listTop := area.Y + 1
	listH := area.H - 2 // 减面包屑 + hint
	if listH < 0 {
		listH = 0
	}
	visibleH := min(len(s.entries), listH)
	for vi := 0; vi < visibleH; vi++ {
		i := s.topIdx + vi
		if i >= len(s.entries) {
			break
		}
		fs.drawEntry(area.X, listTop+vi, area.W, s.entries[i], i+1 == s.cursor, revStyle)
	}

	// 滚动指示符（仅当内容溢出时）；style 跟随所在行（对齐 SelectPane 范式）
	if len(s.entries) > visibleH && visibleH > 0 {
		rightCol := area.X + area.W - 1
		topStyle := config.DefStyle
		if s.topIdx+1 == s.cursor {
			topStyle = revStyle
		}
		botStyle := config.DefStyle
		if s.topIdx+visibleH == s.cursor {
			botStyle = revStyle
		}
		if s.topIdx > 0 {
			screen.Screen.SetContent(rightCol, listTop, '▲', nil, topStyle)
		}
		if s.topIdx+visibleH < len(s.entries) {
			screen.Screen.SetContent(rightCol, listTop+visibleH-1, '▼', nil, botStyle)
		}
	}

	// 末行：光标条目元数据（F1d §4.3）
	hintRow := area.Y + area.H - 1
	fs.refreshMetaIfStale()
	text := ""
	if s.metaName != "" { // 面包屑行/空目录 → 留空
		text = fs.buildMetaLine()
	}
	fs.drawString(area.X, hintRow, area.W, text, config.DefStyle)
}

// drawBreadcrumb 画面包屑行（F0 §6.2 左截断全路径，恒保留"当前目录/"；根目录显 /）。
func (fs *FileSelector) drawBreadcrumb(x, y, w int, style tcell.Style) {
	s := fs.state
	var path string
	switch {
	case s.currentDir == "" || s.currentDir == string(filepath.Separator):
		path = string(filepath.Separator)
	default:
		path = s.currentDir
		if !strings.HasSuffix(path, string(filepath.Separator)) {
			path += string(filepath.Separator)
		}
	}
	disp := truncateLeftPath(path, w)
	col := x
	for _, r := range disp {
		rw := runeWidth(r)
		if col+rw > x+w {
			break // 双宽字符放不下完整两列，不写半截（避免覆盖）
		}
		screen.Screen.SetContent(col, y, r, nil, style)
		col += rw
	}
	for col < x+w { // 尾部填 style（Reverse 时视觉连续）
		screen.Screen.SetContent(col, y, ' ', nil, style)
		col++
	}
}

// drawEntry 画一个文件/目录条目行（F0 §7.5 格式 [标记] 名[/] [git]）。
func (fs *FileSelector) drawEntry(x, y, w int, e entry, selected bool, revStyle tcell.Style) {
	s := fs.state

	// git 状态列（仅 gitOK 且有该条目状态时显示）
	gst, ghas := s.gitOf(e.name)
	showGit := ghas

	// 颜色（F0 §7.5：目录 type 色，文件 default，git 状态色）；选中行统一 Reverse
	nameStyle := config.DefStyle
	if e.isDir {
		nameStyle = config.GetColor("type")
	}
	fillStyle := config.DefStyle
	gitStyle := gitStatusStyle(gst)
	if selected {
		nameStyle = revStyle
		fillStyle = revStyle
		gitStyle = revStyle
	}

	col := x
	// 标记：目录 ▸，文件空格（预留 1 字符位）
	marker := ' '
	if e.isDir {
		marker = '▸'
	}
	screen.Screen.SetContent(col, y, marker, nil, nameStyle)
	col += runeWidth(marker)

	// 名（目录带 / 后缀；超长左截断保留扩展名，F0 §4.2 / R3）
	dispName := e.name
	if e.isDir {
		dispName += string(filepath.Separator)
	}
	nameLimit := w - 1 // 去掉标记 1 列
	if showGit {
		nameLimit-- // 预留 git 1 列
	}
	if nameLimit < 0 {
		nameLimit = 0
	}
	dispName = truncateNameKeepExt(dispName, nameLimit, e.isDir)
	nameEnd := x + 1 + nameLimit
	if showGit {
		nameEnd = x + w - 1
	}
	for _, r := range dispName {
		rw := runeWidth(r)
		if col+rw > nameEnd {
			break // 放不下完整双宽字符则停（不写半截）
		}
		screen.Screen.SetContent(col, y, r, nil, nameStyle)
		col += rw
	}
	for col < nameEnd { // 名与 git 列之间填 style（空格固定 1 列宽）
		screen.Screen.SetContent(col, y, ' ', nil, fillStyle)
		col++
	}
	if showGit {
		screen.Screen.SetContent(x+w-1, y, gitStatusChar(gst), nil, gitStyle)
	}
}

// drawString 在 (x,y) 起、限宽 w 列内写文本，尾部填 style（hint 行用）。
// 按 CJK 显示列宽推进（中文占 2 列），放不下完整双宽字符则停。
func (fs *FileSelector) drawString(x, y, w int, text string, style tcell.Style) {
	col := x
	for _, r := range text {
		rw := runeWidth(r)
		if col+rw > x+w {
			break
		}
		screen.Screen.SetContent(col, y, r, nil, style)
		col += rw
	}
	for col < x+w {
		screen.Screen.SetContent(col, y, ' ', nil, style)
		col++
	}
}

// cursorEntryName 返回当前光标条目名（F1d §4.4）。
// 边界：cursor==0 或 idx 越界 → 返回 ""，绝不 panic。
func (s *fileSelectorState) cursorEntryName() string {
	if s.cursor == 0 {
		return ""
	}
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.entries) {
		return ""
	}
	return s.entries[idx].name
}

// refreshMetaIfStale 按需刷新元数据（F1d §4.2）。
// 在 display 内 guard：名字没变则命中缓存、零 stat；变了才 stat 一次。
// 调用位置在 s==nil return 之后、所有渲染之前，确保每条 display 路径都覆盖。
func (fs *FileSelector) refreshMetaIfStale() {
	s := fs.state
	name := s.cursorEntryName()
	if name == s.metaName {
		return // 命中缓存
	}
	s.metaName = name
	if name == "" { // 面包屑行 / 无条目
		s.metaOK = false
		return
	}
	info, err := os.Stat(filepath.Join(s.currentDir, name))
	s.metaOK = err == nil
	if err == nil {
		s.metaSize = info.Size()
		s.metaMtime = info.ModTime()
		s.metaIsDir = info.IsDir()
	}
}

// humanSize 把字节数格式化为人类可读字符串（F1d §3.2，输出恒 ≤5 列）。
// 对齐 ls -lh 的 1024 进制规则。
func humanSize(n int64) string {
	if n < 0 {
		n = 0
	}

	const unit1024 = int64(1024)
	units := []string{"B", "K", "M", "G", "T", "P"}

	if n < unit1024 {
		return fmt.Sprintf("%dB", n)
	}

	// 除到 mantissa < unit1024
	mantissa := float64(n)
	unitIdx := 0
	for mantissa >= float64(unit1024) && unitIdx < len(units)-1 {
		mantissa /= float64(unit1024)
		unitIdx++
	}

	unit := units[unitIdx]
	if mantissa < 100 {
		s := fmt.Sprintf("%.1f%s", mantissa, unit)
		if stringWidth(s) <= 5 {
			return s
		}
		// 否则 99.9x 进位到 100.0 → 落到下面的整数分支
	}
	// mantissa ≥ 100：整数，去小数（按四舍五入取最近整数）
	// 1023.6→1024K → 进位 1.0M；999.9→1000K；1023.0→1023K
	intPart := int64(mantissa + 0.5) // 四舍五入
	// 进位：进位后值 ≥ 1024（如 1024K → 1.0M）
	if intPart >= unit1024 && unitIdx < len(units)-1 {
		return fmt.Sprintf("1.0%s", units[unitIdx+1])
	}
	return fmt.Sprintf("%d%s", intPart, unit)
}

// formatMtime 把修改时间格式化为对齐 ls -lh 的字符串（F1d §3.3）。
// 同年 → "MM-DD HH:MM"（11 列）；跨年 → "YYYY-MM-DD"（10 列）。
func formatMtime(t time.Time) string {
	now := time.Now()
	year, _, _ := t.Date()
	nowYear, _, _ := now.Date()
	if year == nowYear {
		return t.Format("01-02 15:04") // 11 列
	}
	return t.Format("2006-01-02") // 10 列（跨年省略时分）
}

// buildMetaLine 组装光标条目的元数据行（F1d §3.1 / §3.4）。
// size 是固定 5 列（右对齐），目录行 size 段留空但保留宽度，mtime 紧随其后。
// metaOK=false 时缺失字段显 — 并补齐到字段定宽（— 的显示宽度是 2 列）。
func (fs *FileSelector) buildMetaLine() string {
	s := fs.state

	// size 段（固定 5 列）
	sizeStr := ""
	if s.metaOK {
		if !s.metaIsDir {
			sizeStr = humanSize(s.metaSize)
		}
	}
	if sizeStr == "" {
		if s.metaOK {
			// 目录行（metaOK && metaIsDir）：size 段留空 5 列
			sizeStr = "     " // 5 spaces
		} else {
			// stat 失败：显 — 占位（— 是 U+2014，2 列；3 空格 + — = 5 列）
			sizeStr = "   —"
		}
	}
	// pad sizeStr to exactly 5 display cols
	for stringWidth(sizeStr) < 5 {
		sizeStr = " " + sizeStr
	}

	// mtime 段（10/11 列，formatMtime 已对齐）
	mtimeStr := "          " // default 10 spaces
	if s.metaOK {
		mtimeStr = formatMtime(s.metaMtime)
		// pad to 11 cols if same year (formatMtime returns 11 cols)
		for stringWidth(mtimeStr) < 11 {
			mtimeStr += " "
		}
	} else {
		// stat 失败：— 占 2 列，右 pad 到 11 列（与正常 mtime 左对齐一致）
		mtimeStr = "—"
		for stringWidth(mtimeStr) < 11 {
			mtimeStr += " " // 右 pad（左对齐）
		}
	}

	return sizeStr + "  " + mtimeStr
}


// ---- Controller：handleEvent（F1 §8.2 键位表）----
//
// resize 不会到达这里（FloatFrame 已拦截 → Close + OnCancel）。

func (fs *FileSelector) handleEvent(event tcell.Event) {
	s := fs.state
	if s == nil {
		return
	}
	ev, ok := event.(*tcell.EventKey)
	if !ok {
		return
	}
	switch ev.Key() {
	case tcell.KeyDown:
		fs.moveCursor(+1)
	case tcell.KeyUp:
		fs.moveCursor(-1)
	case tcell.KeyEnter:
		fs.activate()
	case tcell.KeyLeft:
		fs.chdirParent()
	case tcell.KeyRight:
		if fs.cursorIsDir() {
			fs.chdir(s.entries[s.cursor-1].name)
		}
	case tcell.KeyEscape:
		// 两种态都收 Esc → 取消：关 selector，回到调起前的编辑状态（F4 §4）
		fs.finish(SelectResult{Kind: Closed, Reason: ReasonEsc})
	case tcell.KeyCtrlQ:
		// 三入口统一收 Ctrl-q → ReasonQuit，回调里统一 h.Quit()/pane.Quit()（F5 §5.2）
		fs.finish(SelectResult{Kind: Closed, Reason: ReasonQuit})
	default:
		// '.' 切 dotfile（浮窗模态拦截所有事件，绕过 micro 键位绑定，F1b §3.5）
		if ev.Key() == tcell.KeyRune && ev.Rune() == '.' {
			fs.toggleHidden()
		}
		// 其它键吞掉（modal）
	}
}

// moveCursor 上下移动光标（不循环，clamp [0, len]，F0 §5.1）。
func (fs *FileSelector) moveCursor(delta int) {
	s := fs.state
	max := len(s.entries) // cursor ∈ [0, len]
	s.cursor += delta
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor > max {
		s.cursor = max
	}
	fs.ensureVisible()
	screen.Redraw()
}

// activate 处理 Enter（上下文敏感，F0 §5.2）。
func (fs *FileSelector) activate() {
	s := fs.state
	switch fs.cursorRowKind() {
	case rowBreadcrumb:
		fs.chdirParent()
	case rowDir:
		if s.cursor >= 1 && s.cursor-1 < len(s.entries) {
			fs.chdir(s.entries[s.cursor-1].name)
		}
	case rowFile:
		fs.pick()
	}
}

// pick 选中当前文件：上报 Picked + 路径（F3 §4.1d，替换原来的 onSelect(&p)）。
func (fs *FileSelector) pick() {
	s := fs.state
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.entries) {
		return
	}
	e := s.entries[idx]
	if e.isDir { // 防御：仅文件可打开
		return
	}
	full := filepath.Join(s.currentDir, e.name)
	fs.finish(SelectResult{Kind: Picked, Path: full})
}

// cancel Esc：上报 ReasonEsc（F3 §4.1c；现由 handleEvent 直接调 finish，此方法保留作兼容）。
func (fs *FileSelector) cancel() {
	fs.finish(SelectResult{Kind: Closed, Reason: ReasonEsc})
}

// finish 统一收尾：关 FloatFrame + 调回调（F3 §4.1e）。
// 替换原来 pick/cancel 里重复的收尾逻辑，所有关闭路径走这里。
func (fs *FileSelector) finish(r SelectResult) {
	cb := fs.onSelect
	TheFloatFrame.Close()
	if cb != nil {
		cb(r)
	}
}

// chdirParent 回上级目录（Enter on 面包屑 / ← 快捷键）。
// 回上级时光标落在"刚离开的目录"上（ranger/lf 约定）。
func (fs *FileSelector) chdirParent() {
	s := fs.state
	parent := filepath.Dir(s.currentDir)
	if parent == s.currentDir {
		return // 已在根，no-op
	}
	fs.chdirTo(parent, filepath.Base(s.currentDir))
}

// chdir 进入光标所在子目录（→ 快捷键 / Enter on 目录条目）。
func (fs *FileSelector) chdir(sub string) {
	fs.chdirTo(filepath.Join(fs.state.currentDir, sub), "")
}

// chdirTo 切到目标目录并重定位光标 + 后台重查 git。
//   target     目标目录绝对路径
//   focusName  非空 → 落到该名称的条目上（回上级用）；空 → 首条目
func (fs *FileSelector) chdirTo(target, focusName string) {
	s := fs.state
	target = filepath.Clean(target)
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return // 防御：非目录不进
	}
	s.currentDir = target
	s.entries = listDirEntries(target, s.showHidden)
	s.topIdx = 0

	// 光标重定位
	s.cursor = 1
	if focusName != "" {
		for i, e := range s.entries {
			if e.name == focusName {
				s.cursor = i + 1
				break
			}
		}
	}
	if len(s.entries) == 0 {
		s.cursor = 0 // 仅面包屑
	}
	fs.ensureVisible()

	// 重置 git（新目录首帧无 git 列，F1 §10.6 老结果作废）
	s.setGitStatus(nil, false)
	screen.Redraw()

	go fs.fetchGit(target)
}

// toggleHidden 切换 dotfile 显隐（F0 §5.4）。
// 显隐翻转后尽量保持光标位置：同名优先，否则按索引 clamp 到最近可见条目。
func (fs *FileSelector) toggleHidden() {
	s := fs.state
	oldName := ""
	oldIdx := -1
	if s.cursor >= 1 && s.cursor-1 < len(s.entries) {
		oldName = s.entries[s.cursor-1].name
		oldIdx = s.cursor - 1
	}
	s.showHidden = !s.showHidden
	s.entries = listDirEntries(s.currentDir, s.showHidden)
	s.topIdx = 0

	found := false
	if oldName != "" {
		for i, e := range s.entries {
			if e.name == oldName {
				s.cursor = i + 1
				found = true
				break
			}
		}
	}
	if !found {
		idx := oldIdx
		if idx >= len(s.entries) {
			idx = len(s.entries) - 1
		}
		if idx < 0 {
			idx = 0
		}
		if len(s.entries) == 0 {
			s.cursor = 0
		} else {
			s.cursor = idx + 1
		}
	}
	fs.ensureVisible()
	screen.Redraw()
}

// cursorRowKind 返回光标所在行的种类。
func (fs *FileSelector) cursorRowKind() rowKind {
	s := fs.state
	if s.cursor == 0 {
		return rowBreadcrumb
	}
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.entries) {
		return rowFile // 防御
	}
	if s.entries[idx].isDir {
		return rowDir
	}
	return rowFile
}

// cursorIsDir 光标是否在目录条目上。
func (fs *FileSelector) cursorIsDir() bool {
	s := fs.state
	if s.cursor < 1 || s.cursor-1 >= len(s.entries) {
		return false
	}
	return s.entries[s.cursor-1].isDir
}

// ensureVisible 把视口拉到包含当前选中条目的最小位置（参照 selectpane.go:193）。
func (fs *FileSelector) ensureVisible() {
	s := fs.state
	if s.cursor == 0 {
		return // 面包屑行恒在内容区行 0，不受 topIdx 影响
	}
	idx := s.cursor - 1
	listH := s.listH
	if listH <= 0 {
		return
	}
	if idx < s.topIdx {
		s.topIdx = idx
	}
	if idx >= s.topIdx+listH {
		s.topIdx = idx - listH + 1
	}
}

// ---- 显示宽度工具（CJK/全角字符占 2 列，F0 §4.2 rune-safe）----
//
// 终端写屏必须按显示列宽而非 rune 数推进列坐标：
// 一个中文 rune 调 SetContent 后会占 2 列，若 col 只 +1，
// 下一个字符的 SetContent 会覆盖中文的第 2 列，表现为"隔一个少一个"。

// runeWidth 返回单个 rune 的显示列宽（CJK/全角=2，ASCII=1）。
// 宽度 0 的组合字符兑底为 1，避免原地覆盖。
func runeWidth(r rune) int {
	w := runewidth.RuneWidth(r)
	if w < 1 {
		return 1
	}
	return w
}

// stringWidth 返回字符串的显示列宽总和。
func stringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// tailByWidth 返回 s 的尾部子串，其显示宽度 ≤ maxW（按列宽从末尾向前取整字符）。
func tailByWidth(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	runes := []rune(s)
	w := 0
	start := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		rw := runeWidth(runes[i])
		if w+rw > maxW {
			break
		}
		w += rw
		start = i
	}
	return string(runes[start:])
}

// headByWidth 返回 s 的头部子串，其显示宽度 ≤ maxW（按列宽从头部向后取整字符）。
func headByWidth(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	runes := []rune(s)
	w := 0
	end := 0
	for i := 0; i < len(runes); i++ {
		rw := runeWidth(runes[i])
		if w+rw > maxW {
			break
		}
		w += rw
		end = i + 1
	}
	return string(runes[:end])
}

// ---- 截断工具 ----

// truncateLeftPath 左截断路径到 maxW runes，恒保留尾段"当前目录/"（F0 §6.2）。
//
// 在路径段（/）边界处截断，截断处用 "…/" 标记（对齐 F0 §6.2 示例 …/components/md/）。
// path 须以路径分隔符结尾。
// maxW 是显示列宽预算（非 rune 数）：一个 CJK 字符占 2 列。
func truncateLeftPath(path string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if stringWidth(path) <= maxW {
		return path
	}
	sep := string(filepath.Separator)
	trimmed := strings.TrimSuffix(path, sep)
	parts := strings.Split(trimmed, sep) // 绝对路径 parts[0]==""（根）；相对路径无空首段
	if len(parts) == 0 {
		return path
	}
	ellW := runeWidth('…')
	if maxW <= ellW {
		return "…"
	}

	// "…/" 标记宽度 = … + 分隔符
	markerW := ellW + runeWidth(filepath.Separator)
	curSeg := parts[len(parts)-1] + sep
	curW := stringWidth(curSeg)

	// 连 "…/" 都放不下：只显示 … + 当前段尾
	if maxW < markerW {
		avail := maxW - ellW
		if avail <= 0 {
			return "…"
		}
		return "…" + tailByWidth(curSeg, avail)
	}

	budget := maxW - markerW // 给 kept 的总列宽预算

	// 当前段放不下 budget：… + 段尾
	if curW > budget {
		avail := maxW - ellW
		if avail <= 0 {
			return "…"
		}
		return "…" + tailByWidth(curSeg, avail)
	}

	// 从末尾向前累加完整段，直到下一段放不下
	kept := curSeg
	used := curW
	for i := len(parts) - 2; i >= 0; i-- {
		seg := parts[i] + sep
		segW := stringWidth(seg)
		if used+segW > budget {
			break
		}
		kept = seg + kept
		used += segW
	}
	return "…" + sep + kept
}

// truncateNameKeepExt 把 name 截断到 maxCols 显示列宽，保留文件扩展名（F0 §4.2 / R3）。
//   超长 → 右侧加 …，保留 basename 头部 + "扩展名"（最后一个 . 之后）可见（对齐 yazi）。
//   无扩展名/目录 → 保留头部 + …。
//   isDir=true 时不按扩展名处理（目录名里的 . 不是扩展名），直接保留头部。
//   按 CJK 显示列宽计算（中文占 2 列），rune-safe。
func truncateNameKeepExt(name string, maxCols int, isDir bool) string {
	if maxCols <= 0 {
		return ""
	}
	if stringWidth(name) <= maxCols {
		return name
	}
	ellW := runeWidth('…')
	if maxCols <= ellW {
		return "…"
	}
	r := []rune(name)
	headBudget := maxCols - ellW
	if headBudget <= 0 {
		return "…"
	}

	keepHead := func() string {
		return headByWidth(name, headBudget) + "…"
	}

	if isDir {
		return keepHead()
	}
	// 找扩展名（basename 内最后一个 .）
	dotIdx := -1
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] == '.' {
			dotIdx = i
			break
		}
	}
	if dotIdx <= 0 {
		return keepHead() // 无扩展名
	}
	ext := string(r[dotIdx:]) // 含 .
	extW := stringWidth(ext)
	basenameBudget := headBudget - extW
	if basenameBudget <= 0 {
		return keepHead() // 扩展名放不下，退化保头
	}
	head := string(r[:dotIdx])
	return headByWidth(head, basenameBudget) + "…" + ext
}

// ---- git 状态渲染（F0 §7.5 颜色建议）----

func gitStatusChar(st statusKind) rune {
	switch st {
	case stModified:
		return 'M'
	case stUntracked:
		return 'U'
	case stAdded:
		return 'A'
	case stDeleted:
		return 'D'
	case stRenamed:
		return 'R'
	case stIgnored: // F7
		return 'I'
	}
	return ' '
}

func gitStatusStyle(st statusKind) tcell.Style {
	switch st {
	case stModified:
		return config.DefStyle.Foreground(tcell.ColorYellow)
	case stUntracked:
		return config.DefStyle.Foreground(tcell.ColorBlue)
	case stAdded:
		return config.DefStyle.Foreground(tcell.ColorGreen)
	case stDeleted:
		return config.DefStyle.Foreground(tcell.ColorRed)
	case stRenamed:
		return config.DefStyle.Foreground(tcell.ColorPurple)
	case stIgnored: // F7: 用 default 颜色（普通文本，不带 color/dim）
		return config.DefStyle
	default:
		return config.DefStyle
	}
}
