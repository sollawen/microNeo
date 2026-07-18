package finder

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

// ---- 公共契约 ----

// Rect 是屏坐标的一个矩形区域。finder 通过 Open 接收 owner pane 的绝对屏幕矩形。
type Rect struct {
	X, Y, W, H int
}

// CloseReason 描述 finder 会话以何种原因关闭。
type CloseReason uint8

const (
	// Picked：用户 Enter 选中了一个文件（Result.File 有效）。
	Picked CloseReason = iota
	// Esc：用户按 Esc 取消。
	Esc
	// Quit：用户按 Ctrl-q（或 q）请求退出。
	Quit
	// Resize：终端运行中 resize，已显示后被打断。
	Resize
)

// Result 承载一次 finder 会话的关闭结果。
//
//	Cwd     关闭时所在目录（始终填，纯目录、不带文件名）
//	File    仅 Picked 时：选中的文件名（不含路径）；其余为空
//	IsQuit  原样回显 Open 入参 isQuit；finder 全程不读它
type Result struct {
	Reason CloseReason
	Cwd    string
	File   string
	IsQuit bool
}

// ---- Session 类型 ----

// pane-local 布局常量。极窄 / 极矮 pane 在 Open 入口预检拒开（不开会话、不触发 onClose）。
const (
	fsMinWidth  = 20 // 极窄 pane 拒开阈值（列）
	fsMinHeight = 10 // 极矮 pane 拒开阈值（行，保证内容区≥8、文件列表≥6 可滚动）
)

// rowKind 分类光标所在行（Enter 上下文语义）。
type rowKind uint8

const (
	rowFile rowKind = iota
	rowBreadcrumb
	rowDir
)

// rightMode 决定每行右侧固定列的内容。
type rightMode uint8

const (
	rightNone rightMode = iota // 不显示右侧列（name 延伸到 scroll 前）
	rightSize                  // 5 列 humanSize
	rightTime                  // 11 列 formatMtime
)

// finderState 是 Session 的可变状态（Model 层）。
//
// 并发模型：allEntries（含各 gitChar）、showEntries（指针）、isRepo/gitBranch 都是
// fetchGit（后台写）与 display（UI 读）的共享结构，mu（RWMutex）全保护。
// 写（持 Lock）：chdir 重赋 allEntries + rebuildShow；toggleHidden/sort 切换调 rebuildShow
// （重排 showEntries 指针）；fetchGit 写 allEntries[i].gitChar + isRepo/gitBranch。
// 读（持 RLock）：display 读 isRepo + 拷出可见 entry 值副本（含 gitChar），RUnlock 后才画。
type finderState struct {
	// —— 目录与条目（读一次，过滤靠视图）——
	currentDir  string   // 当前目录绝对路径
	allEntries  []entry  // 当前目录全部条目（含 hidden），chdir 时读一次；排序（目录优先）；fetchGit 在此填 gitChar
	showEntries []*entry // 显示视图 = allEntries 按 showHidden 过滤后的指针子集；cursor/topIdx 索引它

	// —— git（后台 goroutine 写、UI 读，mu 保护）——
	isRepo    bool   // 当前目录是否在 git 仓库内。独立于 gitBranch：detached/unborn 时 true 但 gitBranch=""
	gitBranch string // 分支名；"" = detached / unborn / 非仓库。记录用，显示待后续（状态栏/标题）
	// 每文件的 git 状态字符在 entry.gitChar 上，不另存 map。

	// —— 显示模式（字段留好；切换功能以后做）——
	rightMode  rightMode // 每行右侧列：size / time / none。本期恒 rightSize
	sortMode   sortMode  // 排序键：name / size / time。本期恒 sortName
	sortDesc   bool      // false=升序 / true=降序。本期恒 false
	showHidden bool

	// —— 视图 ——
	cursor  int // 0=面包屑行, 1..len(showEntries)=条目
	topIdx  int // 文件列表视口顶（指向 showEntries 索引）
	pickerW int // 内容区宽（截断用）
	listH   int // 文件列表可见行数（内容区高 - 面包屑 - hint）

	mu sync.RWMutex
}

// rebuildShow 从排好序的 allEntries 按 showHidden 过滤，重建 showEntries（指针指向
// allEntries 元素）。chdir / toggleHidden / sort 切换都调它。allEntries 不被重赋期间
// 指针稳定（chdir 整体换新切片后会重调）。
func (s *finderState) rebuildShow() {
	s.showEntries = s.showEntries[:0]
	for i := range s.allEntries {
		if !s.showHidden && isHiddenName(s.allEntries[i].name) {
			continue
		}
		s.showEntries = append(s.showEntries, &s.allEntries[i])
	}
}

// Session 是文件选择器的一次会话实例。owner 持有一个 Session，两步式调用：
//
//	fm := finder.NewSession()
//	fm.Open(rect, cwd, file, isQuit, onClose)
//
// 关闭由内部事件驱动；外部通过 onClose 回调拿到 Result。Session 是单次使用的，
// 关闭后需重新 NewSession 才能再次 Open（与原始 FloatFrame 的"用完即弃"语义一致）。
type Session struct {
	state   *finderState
	rect    Rect // 外矩形（含边框），由 Open 入参
	isOpen  bool
	onClose func(Result)
	isQuit  bool // 全程不读；仅在 close 时原样回填 Result.IsQuit
}

// NewSession 返回一个未打开的 Session。
func NewSession() *Session {
	return &Session{}
}

// IsOpen 返回 Session 是否处于打开状态。
func (fm *Session) IsOpen() bool {
	return fm.isOpen
}

// Open 打开 finder 会话。
//
//	rect    owner pane 的绝对屏幕矩形（含 statusLine，finder 内部减 1 行）
//	cwd     起始目录（绝对路径）
//	file    光标起始定位用的文件名（basename；空=首条目）
//	isQuit  仅用于 Result.IsQuit 回显；finder 全程不读
//	onClose 关闭回调（始终非 nil 时收到一次回调）
//
// 返回 false：预检放不下（rect 太小），不开会话、不触发 onClose。owner 拿到 false 后
// 自己决定如何处理（quit 入口 → Quit；browse/birth 入口 → no-op + 提示）。
func (fm *Session) Open(rect Rect, cwd, file string, isQuit bool, onClose func(Result)) bool {
	fm.onClose = onClose
	fm.isQuit = isQuit

	if rect.W < fsMinWidth || rect.H < fsMinHeight {
		return false // 预检放不下：不开会话、不触发 onClose
	}
	fm.state = newFinderState(cwd, file)

	// 算内容尺寸：外矩形减边框 2，再减 statusLine 1
	widthFrac := config.GetGlobalOption("fileselectwidth").(float64)
	pickerW := int(widthFrac * float64(rect.W))
	if pickerW < fsMinWidth {
		pickerW = fsMinWidth
	}
	if pickerW > rect.W {
		pickerW = rect.W
	}
	contentW := pickerW - 2
	contentH := rect.H - 1 - 2 // -1 statusLine, -2 边框
	if contentW < 1 {
		contentW = 1
	}
	if contentH < 1 {
		contentH = 1
	}
	fm.state.pickerW = contentW
	fm.state.listH = contentH - 2 // 减面包屑 + hint
	if fm.state.listH < 0 {
		fm.state.listH = 0
	}

	// 外框尺寸由 fileselectwidth 比例 + statusLine 预留共同决定：宽取 pickerW
	//（按比例缩放，不溢出 owner pane），高取 rect.H - 1（末行留给 statusLine）。
	// 与 fm.state 的 contentW/contentH 保持相差 2（边框）。
	fm.rect = Rect{X: rect.X, Y: rect.Y, W: pickerW, H: rect.H - 1}
	fm.isOpen = true
	go fm.fetchGit(fm.state.currentDir)
	return true
}

// fetchGit 后台查询某目录的 git 状态并合并进 allEntries 后触发重绘。
// 把 getGitStatus 返回的 chars（name→rune）按文件名合并进各 entry.gitChar；
// 仅当仍停留在该目录时才应用结果，避免快速导航下的竞态污染。
func (fm *Session) fetchGit(dir string) {
	isRepo, branch, chars, state := getGitStatus(dir)
	if fm.state == nil {
		return
	}
	s := fm.state
	s.mu.Lock()
	if s.currentDir != dir { // 检查放锁内，防 TOCTOU：chdirTo 可能刚换 dir + allEntries
		s.mu.Unlock()
		return // 已导航离开，丢弃（旧 chars 不污染新 entries）
	}
	s.isRepo = isRepo
	s.gitBranch = branch
	for i := range s.allEntries {
		switch state {
		case dirAllIgnored:
			// 当前目录本身被 ignore：git 只报了折叠目录 "!! dir/"，
			// 不报里面每个文件的 ignored 记录——所以 chars 必为空，
			// 这里给所有条目打 I 是唯一正确的做法。
			s.allEntries[i].gitChar = 'I'
			continue
		case dirAllUntracked:
			// 当前目录本身整个 untracked：git 只报了折叠目录 "?? dir/"，
			// 不报里面每个文件的 untracked 记录——chars 必为空，
			// 这里给所有条目打 U 是唯一正确的做法（对称于 dirAllIgnored）。
			s.allEntries[i].gitChar = 'U'
			continue
		}
		if ch, ok := chars[s.allEntries[i].name]; ok {
			s.allEntries[i].gitChar = ch
		}
	}
	s.mu.Unlock()
	screen.Redraw()
}

// newFinderState 构造初始 State（光标起始 + dotfile 自动显隐）。
func newFinderState(dir, currentFile string) *finderState {
	dir = filepath.Clean(dir)
	s := &finderState{
		currentDir: dir,
		rightMode:  rightSize, // 本期恒 size
		sortMode:   sortName,  // 本期恒字母序
	}
	// 当前文件是 dotfile 且默认隐藏 → 自动置 showHidden=true
	if isHiddenName(currentFile) {
		s.showHidden = true
	}
	s.allEntries = readDirEntries(dir, s.sortMode, s.sortDesc)
	s.rebuildShow()
	s.locate(currentFile)
	return s
}

// locate 把光标停到当前文件上；找不到 / 无路径 → 首条目。
func (s *finderState) locate(currentFile string) {
	s.cursor = 1 // 默认首条目（cursor 1..len）
	s.topIdx = 0
	if currentFile == "" {
		return
	}
	for i, e := range s.showEntries {
		if e.name == currentFile {
			s.cursor = i + 1
			return
		}
	}
}

// ---- 关闭路径（统一 close）----

// pickedName 返回光标条目的文件名（仅文件有效；目录 / 越界返回空）。
func (fm *Session) pickedName() string {
	s := fm.state
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.showEntries) {
		return ""
	}
	e := s.showEntries[idx]
	if e.isDir {
		return ""
	}
	return e.name
}

// close 统一关闭路径：集中填 Cwd / IsQuit，5 条路径只换 Reason。
// Picked 时附 File（仅文件名，不含路径）；其余为空。
func (fm *Session) close(reason CloseReason) {
	if !fm.isOpen {
		return
	}
	r := Result{Reason: reason, Cwd: fm.state.currentDir, IsQuit: fm.isQuit}
	if reason == Picked {
		r.File = fm.pickedName()
	}
	cb := fm.onClose
	fm.reset()
	if cb != nil {
		cb(r)
	}
}

// reset 清空 Session 的可变状态（保 state 留作 GC 之前的过渡持有，避免 fetchGit goroutine
// 在 close 后还在写时崩溃）。
func (fm *Session) reset() {
	fm.isOpen = false
	fm.onClose = nil
}

// ---- HandleEvent / NotifyBlur / Display ----

// HandleEvent 转发键盘事件给 Session。EventResize 在头部拦截（运行中 resize → 取消）；
// 其余转 handleKey。
func (fm *Session) HandleEvent(event tcell.Event) {
	if !fm.isOpen {
		return
	}
	if _, ok := event.(*tcell.EventResize); ok {
		fm.close(Resize) // 运行中终端 resize → 取消（不转发）
		return
	}
	fm.handleKey(event)
}

// NotifyBlur 由 owner 在失焦时调用（不是 tcell 事件，走方法不走 HandleEvent 管道）。
// 等价 Esc：关会话、触发 onClose(Esc)。
func (fm *Session) NotifyBlur() {
	if fm.isOpen {
		fm.close(Esc)
	}
}

// Display 画 Session 全部内容：边框 + 内容区。自洽画框，不依赖外部容器。
func (fm *Session) Display() {
	if !fm.isOpen {
		return
	}
	screen.Screen.HideCursor()
	fm.drawBorder()
	fm.drawContent()
}

// ---- View：内容区三段布局（面包屑 / 文件列表 / hint） ----
//
// 内容区高恒 ≥1（Open 时保底）。listH = 内容区高 - 2。
// 锁边界：fetchGit（后台写 gitChar/isRepo）的唯一并发读端，一把 RLock 内读 isRepo +
// cursor/topIdx + 拷出可见 entry 值（含 gitChar），释放锁后才画（RWMutex 不可重入）。

func (fm *Session) drawContent() {
	s := fm.state
	if s == nil {
		return
	}
	revStyle := config.DefStyle.Reverse(true)

	// 内容区 = 外矩形内缩 1
	x := fm.rect.X + 1
	y := fm.rect.Y + 1
	w := fm.state.pickerW
	h := fm.state.listH + 2 // 列表 + 面包屑 + hint

	listTop := y + 1
	listH := h - 2
	if listH < 0 {
		listH = 0
	}

	// —— 锁内：读 isRepo + cursor/topIdx + 拷出可见 entry 值（gitChar 含在内）——
	s.mu.RLock()
	gitOn := s.isRepo
	cursorOnBc := s.cursor == 0
	total := len(s.showEntries)
	visibleH := min(total, listH)
	vis := make([]entry, visibleH)
	for vi := 0; vi < visibleH; vi++ {
		i := s.topIdx + vi
		if i >= total {
			break
		}
		vis[vi] = *s.showEntries[i] // 解引用 → 值副本，gitChar 被原子捕获
	}
	cursor, topIdx := s.cursor, s.topIdx
	s.mu.RUnlock()

	// 行 0：面包屑（目录路径，用 type 色，对齐子目录行）
	bcStyle := config.GetColor("type")
	if cursorOnBc {
		bcStyle = revStyle
	}
	fm.drawBreadcrumb(x, y, w, bcStyle)

	// —— 锁外：逐行画（drawEntry 拿 gitOn 入参，自己不拿锁）——
	for vi := 0; vi < visibleH; vi++ {
		fm.drawEntry(x, listTop+vi, w, vis[vi], gitOn, topIdx+vi+1 == cursor, revStyle)
	}

	// 滚动指示符（用锁内拷出的 gitOn/cursor/topIdx/total，不碰锁）；画在 drawEntry 写好空格的 scroll 列上。
	scrollCol := x + w - 1
	if gitOn {
		scrollCol = x + w - 2
	}
	if total > visibleH && visibleH > 0 {
		topStyle := config.DefStyle
		if topIdx+1 == cursor {
			topStyle = revStyle
		}
		botStyle := config.DefStyle
		if topIdx+visibleH == cursor {
			botStyle = revStyle
		}
		if topIdx > 0 {
			screen.Screen.SetContent(scrollCol, listTop, '▲', nil, topStyle)
		}
		if topIdx+visibleH < total {
			screen.Screen.SetContent(scrollCol, listTop+visibleH-1, '▼', nil, botStyle)
		}
	}

	// 末行：perms + size + mtime（全从 e.info 读，零 stat）
	hintRow := y + h - 1
	text := fm.buildMetaLine(w) // 内部判 cursor==0/越界 → 返回 ""
	fm.drawString(x, hintRow, w, text, config.DefStyle)
}

// drawBorder 画方框边框（自洽，不依赖外部容器）。照搬浮窗框架的清屏 + 4 角 + 上下 ─
// + 左右 │ + 上边框嵌 title 三块机械代码。
func (fm *Session) drawBorder() {
	x, y, w, h := fm.rect.X, fm.rect.Y, fm.rect.W, fm.rect.H
	color := config.DefStyle

	// 1. clear 整个矩形
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			screen.Screen.SetContent(x+col, y+row, ' ', nil, color)
		}
	}
	// 2. 4 角 + 上下 ─
	screen.Screen.SetContent(x, y, '┌', nil, color)
	screen.Screen.SetContent(x+w-1, y, '┐', nil, color)
	screen.Screen.SetContent(x, y+h-1, '└', nil, color)
	screen.Screen.SetContent(x+w-1, y+h-1, '┘', nil, color)
	for i := 1; i < w-1; i++ {
		screen.Screen.SetContent(x+i, y, '─', nil, color)
		screen.Screen.SetContent(x+i, y+h-1, '─', nil, color)
	}
	// 3. 左右 │
	for row := 1; row < h-1; row++ {
		screen.Screen.SetContent(x, y+row, '│', nil, color)
		screen.Screen.SetContent(x+w-1, y+row, '│', nil, color)
	}
	// 4. 上边框嵌 ──<title>──...─
	title := "Open File"
	col := x + 1
	write := func(r rune) {
		if col < x+w-1 {
			screen.Screen.SetContent(col, y, r, nil, color)
			col++
		}
	}
	write('─')
	write('─')
	for _, r := range title {
		write(r)
	}
	write('─')
	write('─')
	for col < x+w-1 {
		write('─')
	}
}

// drawBreadcrumb 画面包屑行（左截断全路径，恒保留"当前目录/"；根目录显 /）。
func (fm *Session) drawBreadcrumb(x, y, w int, style tcell.Style) {
	s := fm.state
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

// drawEntry 画一个文件/目录条目行。gitOn 决定是否预留 git 列（绑全局 isRepo，不由逐行状态决定）。
//
// 列结构（从左到右）：[marker] [name…] [sep?] [right?] [scroll(1)] [git(1)?]
//   - marker：1 列（目录 ▸ / 文件空格）。
//   - name：超长中间截断（head…ext，保留头部 + 扩展名，对齐 yazi）。
//   - sep：name 与 right 之间恒留 1 空格（rightNone 时无 sep）。
//   - right：rightW 列（0/5/11），区内右对齐；目录留空。
//   - scroll：1 列恒空格，溢出时由 displayContent 覆盖 ▲/▼。
//   - git：1 列仅 gitOn 时预留；脏文件画状态字符，干净文件画空格。
//
// e 是 displayContent 在 RLock 内拷出的值副本（含 gitChar），本函数不拿锁（RWMutex 不可重入）。
func (fm *Session) drawEntry(x, y, w int, e entry, gitOn bool, selected bool, revStyle tcell.Style) {
	s := fm.state

	const sizeW, timeW = 5, 11
	R := x + w

	var gitCol, scrollCol int
	if gitOn {
		gitCol, scrollCol = R-1, R-2
	} else {
		scrollCol = R - 1
	}
	rightW := 0
	switch s.rightMode {
	case rightSize:
		rightW = sizeW
	case rightTime:
		rightW = timeW
	}
	rightStart := scrollCol
	nameLimit := scrollCol - (x + 1)
	if rightW > 0 {
		rightStart = scrollCol - rightW
		nameLimit = rightStart - (x + 1) - 1 // -1 = sep
	}
	if nameLimit < 0 {
		nameLimit = 0
	}
	nameCap := rightStart
	if rightW == 0 {
		nameCap = scrollCol
	}

	// 颜色：目录 type 色，文件 default，git 状态色；选中行统一 Reverse
	nameStyle := config.DefStyle
	if e.isDir {
		nameStyle = config.GetColor("type")
	}
	fillStyle := config.DefStyle
	gitStyle := gitCharStyle(e.gitChar)
	if selected {
		nameStyle, fillStyle, gitStyle = revStyle, revStyle, revStyle
	}

	// marker
	col := x
	marker := ' '
	if e.isDir {
		marker = '▸'
	}
	screen.Screen.SetContent(col, y, marker, nil, nameStyle)
	col += runeWidth(marker)

	// name（截断）+ 填充到 nameCap（含 sep 列空格）
	dispName := e.name
	if e.isDir {
		dispName += string(filepath.Separator)
	}
	dispName = truncateNameKeepExt(dispName, nameLimit, e.isDir)
	for _, r := range dispName {
		rw := runeWidth(r)
		if col+rw > nameCap {
			break
		}
		screen.Screen.SetContent(col, y, r, nil, nameStyle)
		col += rw
	}
	for col < nameCap {
		screen.Screen.SetContent(col, y, ' ', nil, fillStyle)
		col++
	}

	// right 区 [rightStart, scrollCol)：目录留空；文件右对齐 size/time
	if rightW > 0 {
		for c := rightStart; c < scrollCol; c++ {
			screen.Screen.SetContent(c, y, ' ', nil, fillStyle)
		}
		if !e.isDir {
			var rs string
			if s.rightMode == rightTime {
				rs = formatMtime(e.info.ModTime())
			} else {
				rs = humanSize(e.info.Size())
			}
			sw := stringWidth(rs)
			c := scrollCol - sw
			if c < rightStart {
				c = rightStart
			}
			for _, r := range rs {
				screen.Screen.SetContent(c, y, r, nil, fillStyle)
				c += runeWidth(r)
			}
		}
	}

	// scroll 列：恒空格（displayContent 在溢出时覆盖首/末行）
	screen.Screen.SetContent(scrollCol, y, ' ', nil, fillStyle)

	// git 列：有标志画标志，否则空格（e.gitChar=0=干净/非仓库）
	if gitOn {
		if e.gitChar != 0 {
			screen.Screen.SetContent(gitCol, y, e.gitChar, nil, gitStyle)
		} else {
			screen.Screen.SetContent(gitCol, y, ' ', nil, fillStyle)
		}
	}
}

// drawString 在 (x,y) 起、限宽 w 列内写文本，尾部填 style（hint 行用）。
// 按 CJK 显示列宽推进（中文占 2 列），放不下完整双宽字符则停。
func (fm *Session) drawString(x, y, w int, text string, style tcell.Style) {
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

// buildMetaLine 组装光标条目的元数据行（perms + size + mtime）。
// 所有数据从 entry.info 读，零 stat。w 参数控制窄屏字段优先级裁剪。
func (fm *Session) buildMetaLine(w int) string {
	s := fm.state
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.showEntries) {
		return ""
	}
	fi := s.showEntries[idx].info
	perms := fi.Mode().String()
	size := humanSize(fi.Size())
	mtime := formatMtime(fi.ModTime())
	return fitMeta(w, perms, size, mtime)
}

// ---- Controller：handleKey 键位映射 ----
//
// resize 不会到达这里（HandleEvent 已拦截 → close(Resize)）。
// 显示模式切换键（s/S/c）本期不接，留待以后。

func (fm *Session) handleKey(event tcell.Event) {
	s := fm.state
	if s == nil {
		return
	}
	ev, ok := event.(*tcell.EventKey)
	if !ok {
		return
	}
	switch ev.Key() {
	case tcell.KeyDown:
		fm.moveCursor(+1)
	case tcell.KeyUp:
		fm.moveCursor(-1)
	case tcell.KeyEnter:
		fm.activate()
	case tcell.KeyLeft:
		fm.chdirParent()
	case tcell.KeyRight:
		if fm.cursorIsDir() {
			fm.chdir(s.showEntries[s.cursor-1].name)
		}
	case tcell.KeyEscape:
		fm.close(Esc)
	case tcell.KeyCtrlQ:
		fm.close(Quit)
	default:
		// 可打印字符走 KeyRune（tcell 无 KeyQ 之类常量，字母/符号只能按 rune 匹配）。
		if ev.Key() == tcell.KeyRune {
			switch ev.Rune() {
			case '.':
				fm.toggleHidden()
			case 'q':
				// 等价 Ctrl-q：q 退出
				fm.close(Quit)
			case 'd':
				fm.startDelete()
			case 'r':
				fm.startRename()
			}
		}
		// 其它键吞掉（modal）
	}
}

// moveCursor 上下移动光标（不循环，clamp [0, len]）。
func (fm *Session) moveCursor(delta int) {
	s := fm.state
	max := len(s.showEntries) // cursor ∈ [0, len]
	s.cursor += delta
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor > max {
		s.cursor = max
	}
	fm.ensureVisible()
	screen.Redraw()
}

// activate 处理 Enter（上下文敏感）。
func (fm *Session) activate() {
	switch fm.cursorRowKind() {
	case rowBreadcrumb:
		fm.chdirParent()
	case rowDir:
		s := fm.state
		if s.cursor >= 1 && s.cursor-1 < len(s.showEntries) {
			fm.chdir(s.showEntries[s.cursor-1].name)
		}
	case rowFile:
		fm.pick()
	}
}

// pick 选中当前文件：close(Picked)。
func (fm *Session) pick() {
	if fm.cursorIsDir() {
		return // 防御：仅文件可打开
	}
	fm.close(Picked)
}

// chdirParent 回上级目录（Enter on 面包屑 / ← 快捷键）。
// 回上级时光标落在"刚离开的目录"上（ranger/lf 约定）。
func (fm *Session) chdirParent() {
	s := fm.state
	parent := filepath.Dir(s.currentDir)
	if parent == s.currentDir {
		return // 已在根，no-op
	}
	fm.chdirTo(parent, filepath.Base(s.currentDir))
}

// chdir 进入光标所在子目录（→ 快捷键 / Enter on 目录条目）。
func (fm *Session) chdir(sub string) {
	fm.chdirTo(filepath.Join(fm.state.currentDir, sub), "")
}

// chdirTo 切到目标目录并重定位光标 + 后台重查 git。
//
//	target     目标目录绝对路径
//	focusName  非空 → 落到该名称的条目上（回上级用）；空 → 首条目
//
// 编排：readDirEntries（同步读盘，μs）+ rebuildShow + cursor → Redraw（首帧无 git）
// → go fetchGit（异步填 allEntries[*].gitChar）。
func (fm *Session) chdirTo(target, focusName string) {
	s := fm.state
	target = filepath.Clean(target)
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		return // 防御：非目录不进
	}
	s.mu.Lock()
	s.currentDir = target
	s.allEntries = readDirEntries(target, s.sortMode, s.sortDesc) // 读全部（含 hidden）+ 排序；gitChar 全 0
	s.rebuildShow()                                               // 过滤 → showEntries
	s.topIdx = 0
	s.cursor = 1
	if focusName != "" { // 回上级：落到原目录名上
		for i, e := range s.showEntries {
			if e.name == focusName {
				s.cursor = i + 1
				break
			}
		}
	}
	if len(s.showEntries) == 0 {
		s.cursor = 0 // 仅面包屑
	}
	fm.ensureVisible()
	s.mu.Unlock()

	screen.Redraw()        // 首帧：git 列空
	go fm.fetchGit(target) // 异步填 allEntries[*].gitChar → 内部 Redraw
}

// toggleHidden 切换 dotfile 显隐。
// 只 rebuildShow（重过滤）；allEntries + 各 gitChar 原封不动 → 不重读目录、不重取 git
// （露出的 hidden 条目早在 chdir 的 fetchGit 里填过 gitChar）。
// 显隐翻转后尽量保持光标位置：同名优先，否则按索引 clamp 到最近可见条目。
func (fm *Session) toggleHidden() {
	s := fm.state
	s.mu.Lock()
	oldName := ""
	oldIdx := s.cursor - 1
	if s.cursor >= 1 && s.cursor-1 < len(s.showEntries) {
		oldName = s.showEntries[s.cursor-1].name
	}
	s.showHidden = !s.showHidden
	s.rebuildShow()
	found := false
	if oldName != "" {
		for i, e := range s.showEntries {
			if e.name == oldName {
				s.cursor = i + 1
				found = true
				break
			}
		}
	}
	if !found {
		idx := oldIdx
		if idx >= len(s.showEntries) {
			idx = len(s.showEntries) - 1
		}
		if idx < 0 {
			idx = 0
		}
		if len(s.showEntries) == 0 {
			s.cursor = 0
		} else {
			s.cursor = idx + 1
		}
	}
	fm.ensureVisible()
	s.mu.Unlock()

	screen.Redraw()
}

// cursorRowKind 返回光标所在行的种类。
func (fm *Session) cursorRowKind() rowKind {
	s := fm.state
	if s.cursor == 0 {
		return rowBreadcrumb
	}
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.showEntries) {
		return rowFile // 防御
	}
	if s.showEntries[idx].isDir {
		return rowDir
	}
	return rowFile
}

// cursorIsDir 光标是否在目录条目上。
func (fm *Session) cursorIsDir() bool {
	s := fm.state
	if s.cursor < 1 || s.cursor-1 >= len(s.showEntries) {
		return false
	}
	return s.showEntries[s.cursor-1].isDir
}

// ensureVisible 把视口拉到包含当前选中条目的最小位置。
func (fm *Session) ensureVisible() {
	s := fm.state
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

// ---- git 状态渲染（显示层，按 colorscheme 名解色）----

// gitCharStyle 把 git 状态字符映射成 colorscheme 颜色。
// M/R→diff-modified；A/U→diff-added；D→diff-deleted；I 及干净→默认。
// M/R 合并、A/U 合并是因为颜色语义相同（"改了" / "新的"），9/9 colorscheme 都覆盖这三个名。
func gitCharStyle(c rune) tcell.Style {
	switch c {
	case 'M', 'R':
		return config.GetColor("diff-modified")
	case 'A', 'U':
		return config.GetColor("diff-added")
	case 'D':
		return config.GetColor("diff-deleted")
	}
	return config.DefStyle // 'I'（ignored）及干净/非仓库默认
}

// min 是内建的极简版本（仅 int 两个参数；Go 1.21+ 有内建 min，本项目用 Go 1.19 故自备）。
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
