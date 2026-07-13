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

// rightMode 决定每行右侧固定列的内容。
type rightMode uint8

const (
	rightNone rightMode = iota // 不显示右侧列（name 延伸到 scroll 前）
	rightSize                  // 5 列 humanSize
	rightTime                  // 11 列 formatMtime
)

// sortMode 决定文件组内的排序键（目录组恒先、恒按名排）。
type sortMode uint8

const (
	sortName sortMode = iota // 字母（大小写不敏感）
	sortSize                 // size
	sortTime                 // mtime
)

// entry 是一个目录条目。gitChar 在 readDir 时为 0（干净/非仓库），fetchGit 回来后填充。
// info 存全量 FileInfo（lstat，不跟随 symlink），恒非 nil（d.Info() 失败的条目直接跳过）。
type entry struct {
	name    string
	isDir   bool
	info    os.FileInfo
	gitChar rune // git 状态字符 'M'/'U'/'A'/'R'/'I'；0=干净/非仓库
}

// fileSelectorState 是 FileSelector 的可变状态（Model 层，F1 §5 / D3）。
//
// 并发模型：allEntries（含各 gitChar）、showEntries（指针）、isRepo/gitBranch 都是
// fetchGit（后台写）与 display（UI 读）的共享结构，mu（RWMutex）全保护。
// 写（持 Lock）：chdir 重赋 allEntries + rebuildShow；toggleHidden/sort 切换调 rebuildShow
// （重排 showEntries 指针）；fetchGit 写 allEntries[i].gitChar + isRepo/gitBranch。
// 读（持 RLock）：display 读 isRepo + 拷出可见 entry 值副本（含 gitChar），RUnlock 后才画。
type fileSelectorState struct {
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
func (s *fileSelectorState) rebuildShow() {
	s.showEntries = s.showEntries[:0]
	for i := range s.allEntries {
		if !s.showHidden && isHiddenName(s.allEntries[i].name) {
			continue
		}
		s.showEntries = append(s.showEntries, &s.allEntries[i])
	}
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
	ReasonQuit                      // 用户按 Ctrl-q（或 welcome 态的 Quit）
	ReasonSize                      // 打开时窗口过小，selector 从未显示（F5）
	ReasonResize                    // 运行中窗口 resize，已显示后被打断（F5）
)

// SelectResult 承载 FileSelector 的关闭结果（F3 §3）。
// 原来 onSelect 签名是 func(*string)（nil=取消），现在换成带原因的 SelectResult，
// 让调用方能区分「选中文件」和「各种关掉」，welcome 调用方据此决定行为（退出/重开/no-op）。
type SelectResult struct {
	Kind   ResultKind  // Picked | Closed
	Path   string      // Kind==Picked 时：选中的文件绝对路径
	Reason CloseReason // Kind==Closed 时：关闭原因
}

// FileSelector 是文件选择器浮窗本体。
type FileSelector struct {
	state    *fileSelectorState
	pane     *BufPane
	onSelect func(SelectResult)
}

// NewFileSelector 返回一个未打开的 FileSelector。
func NewFileSelector() *FileSelector {
	return &FileSelector{}
}

// Open 打开文件选择器（F1 §3.2 / §6.2 / §10.7 异步时序）。
//
//	pane      发起 :file 的 pane（pane-local 布局 + 选中后开进此 pane）
//	startDir  起始目录（F1 §8.1 / R6）
//	onSelect  回调（SelectResult）；browse/birth 调用方只关心 Picked，quit 调用方按 Reason 分流
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

// fetchGit 后台查询某目录的 git 状态并合并进 allEntries 后触发重绘。
// 把 getGitStatus 返回的 chars（name→rune）按文件名合并进各 entry.gitChar；
// 仅当仍停留在该目录时才应用结果，避免快速导航下的竞态污染。
func (fs *FileSelector) fetchGit(dir string) {
	isRepo, branch, chars, allIgnored := getGitStatus(dir)
	if fs.state == nil {
		return
	}
	s := fs.state
	s.mu.Lock()
	if s.currentDir != dir { // 检查放锁内，防 TOCTOU：chdirTo 可能刚换 dir + allEntries
		s.mu.Unlock()
		return // 已导航离开，丢弃（旧 chars 不污染新 entries）
	}
	s.isRepo = isRepo
	s.gitBranch = branch
	for i := range s.allEntries {
		if allIgnored {
			// 当前目录本身被 ignore：git 只报了折叠目录 "!! dir/"，
			// 不报里面每个文件的 ignored 记录——所以 chars 必为空，
			// 这里给所有条目打 I 是唯一正确的做法。
			s.allEntries[i].gitChar = 'I'
			continue
		}
		if ch, ok := chars[s.allEntries[i].name]; ok {
			s.allEntries[i].gitChar = ch
		}
	}
	s.mu.Unlock()
	screen.Redraw()
}

// newState 构造初始 State（F0 §5.3 光标起始 / §5.4 dotfile 自动显隐）。
func newState(dir, currentFile string) *fileSelectorState {
	dir = filepath.Clean(dir)
	s := &fileSelectorState{
		currentDir: dir,
		rightMode:  rightSize, // 本期恒 size
		sortMode:   sortName,  // 本期恒字母序
	}
	// 当前文件是 dotfile 且默认隐藏 → 自动置 showHidden=true（F0 §5.4）
	if isHiddenName(currentFile) {
		s.showHidden = true
	}
	s.allEntries = readDirEntries(dir, s.sortMode, s.sortDesc)
	s.rebuildShow()
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
	for i, e := range s.showEntries {
		if e.name == currentFile {
			s.cursor = i + 1
			return
		}
	}
}

// ---- readDirEntries / sortEntries / rebuildShow（F0 §5.4 / §5.5）----

// readDirEntries 读目录【全部】条目（含 hidden，不过滤）、建 entry（gitChar=0）、
// d.Info() 失败则跳过、排序返回。只在 chdir 时调一次（不再每次 toggle 重读）。
func readDirEntries(dir string, sm sortMode, desc bool) []entry {
	dirEntries, _ := os.ReadDir(dir)
	all := make([]entry, 0, len(dirEntries))
	for _, d := range dirEntries {
		info, err := d.Info() // lstat，不跟随 symlink，对齐 ls -l；失败跳过
		if err != nil {
			continue
		}
		all = append(all, entry{name: d.Name(), isDir: d.IsDir(), info: info /* gitChar=0 */})
	}
	sortEntries(all, sm, desc)
	return all
}

// sortEntries 原地排序：目录恒在文件前；目录组恒按名升序；文件组主键随 sm + desc，
// 并列恒回退字母升序（不受 desc 影响，保稳定）。readDirEntries 与 sort 切换都调它。
func sortEntries(all []entry, sm sortMode, desc bool) {
	lessName := func(a, b entry) bool { return strings.ToLower(a.name) < strings.ToLower(b.name) }
	sort.SliceStable(all, func(i, j int) bool {
		ai, aj := all[i], all[j]
		if ai.isDir != aj.isDir {
			return ai.isDir // 目录恒在文件前
		}
		if ai.isDir {
			return lessName(ai, aj) // 目录组恒按名升序
		}
		switch sm { // 文件组
		case sortSize:
			si, sj := ai.info.Size(), aj.info.Size()
			if si == sj {
				return lessName(ai, aj)
			}
			if desc {
				return si > sj
			}
			return si < sj
		case sortTime:
			ti, tj := ai.info.ModTime(), aj.info.ModTime()
			if ti.Equal(tj) {
				return lessName(ai, aj)
			}
			if desc {
				return ti.After(tj)
			}
			return ti.Before(tj)
		default: // sortName
			if strings.EqualFold(ai.name, aj.name) {
				return ai.name < aj.name // 大小写并列：原样升序
			}
			if desc {
				return lessName(aj, ai)
			}
			return lessName(ai, aj)
		}
	})
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
//
// 锁边界：fetchGit（后台写 gitChar/isRepo）的唯一并发读端，一把 RLock 内读 isRepo +
// cursor/topIdx + 拷出可见 entry 值（含 gitChar），释放锁后才画（RWMutex 不可重入）。

func (fs *FileSelector) display(area Rect) {
	s := fs.state
	if s == nil {
		return
	}
	revStyle := config.DefStyle.Reverse(true)

	listTop := area.Y + 1
	listH := area.H - 2 // 减面包屑 + hint
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

	// 行 0：面包屑（目录路径，用 type 色，对齐子目录行，F0 §7.5）
	bcStyle := config.GetColor("type")
	if cursorOnBc {
		bcStyle = revStyle
	}
	fs.drawBreadcrumb(area.X, area.Y, area.W, bcStyle)

	// —— 锁外：逐行画（drawEntry 拿 gitOn 入参，自己不拿锁）——
	for vi := 0; vi < visibleH; vi++ {
		fs.drawEntry(area.X, listTop+vi, area.W, vis[vi], gitOn, topIdx+vi+1 == cursor, revStyle)
	}

	// 滚动指示符（用锁内拷出的 gitOn/cursor/topIdx/total，不碰锁）；画在 drawEntry 写好空格的 scroll 列上。
	scrollCol := area.X + area.W - 1
	if gitOn {
		scrollCol = area.X + area.W - 2
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
	hintRow := area.Y + area.H - 1
	text := fs.buildMetaLine(area.W) // 内部判 cursor==0/越界 → 返回 ""
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

// drawEntry 画一个文件/目录条目行。gitOn 决定是否预留 git 列（绑全局 isRepo，不由逐行状态决定）。
//
// 列结构（从左到右）：[marker] [name…] [sep?] [right?] [scroll(1)] [git(1)?]
//   - marker：1 列（目录 ▸ / 文件空格）。
//   - name：超长中间截断（head…ext，保留头部 + 扩展名，对齐 yazi）。
//   - sep：name 与 right 之间恒留 1 空格（rightNone 时无 sep）。
//   - right：rightW 列（0/5/11），区内右对齐；目录留空。
//   - scroll：1 列恒空格，溢出时由 display 覆盖 ▲/▼。
//   - git：1 列仅 gitOn 时预留；脏文件画状态字符，干净文件画空格。
//
// e 是 display 在 RLock 内拷出的值副本（含 gitChar），本函数不拿锁（RWMutex 不可重入）。
func (fs *FileSelector) drawEntry(x, y, w int, e entry, gitOn bool, selected bool, revStyle tcell.Style) {
	s := fs.state

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

	// scroll 列：恒空格（display 在溢出时覆盖首/末行）
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

// buildMetaLine 组装光标条目的元数据行（perms + size + mtime）。
// 所有数据从 entry.info 读，零 stat。w 参数控制窄屏字段优先级裁剪。
func (fs *FileSelector) buildMetaLine(w int) string {
	s := fs.state
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

// fitMeta 按 w 宽挑能放下的组合；砍字段优先级 权限→size，mtime 保底。
func fitMeta(w int, perms, size, mtime string) string {
	type f struct{ k, v string }
	order := []f{{"p", perms}, {"s", size}, {"m", mtime}}
	keep := map[string]bool{"p": true, "s": true, "m": true}
	drop := []string{"p", "s"} // m 永不砍
	for {
		var parts []string
		for _, x := range order {
			if keep[x.k] {
				parts = append(parts, x.v)
			}
		}
		line := strings.Join(parts, "  ")
		if stringWidth(line) <= w {
			return line
		}
		killed := false
		for _, d := range drop {
			if keep[d] {
				keep[d] = false
				killed = true
				break
			}
		}
		if !killed {
			return tailByWidth(line, w) // 只剩 mtime 仍超宽（极窄 pane），左截断保底
		}
	}
}

// ---- Controller：handleEvent（F1 §8.2 键位表）----
//
// resize 不会到达这里（FloatFrame 已拦截 → Close + OnCancel）。
// 显示模式切换键（s/S/c）本期不接，留待以后。

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
			fs.chdir(s.showEntries[s.cursor-1].name)
		}
	case tcell.KeyEscape:
		// 两种态都收 Esc → 取消：关 selector，回到调起前的编辑状态（F4 §4）
		fs.finish(SelectResult{Kind: Closed, Reason: ReasonEsc})
	case tcell.KeyCtrlQ:
		// 三入口统一收 Ctrl-q → ReasonQuit，回调里统一 h.Quit()/pane.Quit()（F5 §5.2）
		fs.finish(SelectResult{Kind: Closed, Reason: ReasonQuit})
	default:
		// 可打印字符走 KeyRune（tcell 无 KeyQ 之类常量，字母/符号只能按 rune 匹配）。
		// 浮窗模态拦截所有事件，绕过 micro 键位绑定（F1b §3.5）。
		if ev.Key() == tcell.KeyRune {
			switch ev.Rune() {
			case '.':
				fs.toggleHidden()
			case 'q':
				// 等价 Ctrl-q：q 退出（三入口统一 ReasonQuit，回调里 h.Quit()/pane.Quit()）
				fs.finish(SelectResult{Kind: Closed, Reason: ReasonQuit})
			}
		}
		// 其它键吞掉（modal）
	}
}

// moveCursor 上下移动光标（不循环，clamp [0, len]，F0 §5.1）。
func (fs *FileSelector) moveCursor(delta int) {
	s := fs.state
	max := len(s.showEntries) // cursor ∈ [0, len]
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
	switch fs.cursorRowKind() {
	case rowBreadcrumb:
		fs.chdirParent()
	case rowDir:
		s := fs.state
		if s.cursor >= 1 && s.cursor-1 < len(s.showEntries) {
			fs.chdir(s.showEntries[s.cursor-1].name)
		}
	case rowFile:
		fs.pick()
	}
}

// pick 选中当前文件：上报 Picked + 路径（F3 §4.1d，替换原来的 onSelect(&p)）。
func (fs *FileSelector) pick() {
	s := fs.state
	idx := s.cursor - 1
	if idx < 0 || idx >= len(s.showEntries) {
		return
	}
	e := s.showEntries[idx]
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
//
//	target     目标目录绝对路径
//	focusName  非空 → 落到该名称的条目上（回上级用）；空 → 首条目
//
// 编排：readDirEntries（同步读盘，μs）+ rebuildShow + cursor → Redraw（首帧无 git）
// → go fetchGit（异步填 allEntries[*].gitChar）。
func (fs *FileSelector) chdirTo(target, focusName string) {
	s := fs.state
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
	fs.ensureVisible()
	s.mu.Unlock()

	screen.Redraw()        // 首帧：git 列空
	go fs.fetchGit(target) // 异步填 allEntries[*].gitChar → 内部 Redraw
}

// toggleHidden 切换 dotfile 显隐（F0 §5.4）。
// 只 rebuildShow（重过滤）；allEntries + 各 gitChar 原封不动 → 不重读目录、不重取 git
// （露出的 hidden 条目早在 chdir 的 fetchGit 里填过 gitChar）。
// 显隐翻转后尽量保持光标位置：同名优先，否则按索引 clamp 到最近可见条目。
func (fs *FileSelector) toggleHidden() {
	s := fs.state
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
	fs.ensureVisible()
	s.mu.Unlock()

	screen.Redraw()
}

// cursorRowKind 返回光标所在行的种类。
func (fs *FileSelector) cursorRowKind() rowKind {
	s := fs.state
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
func (fs *FileSelector) cursorIsDir() bool {
	s := fs.state
	if s.cursor < 1 || s.cursor-1 >= len(s.showEntries) {
		return false
	}
	return s.showEntries[s.cursor-1].isDir
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
//
//	超长 → 右侧加 …，保留 basename 头部 + "扩展名"（最后一个 . 之后）可见（对齐 yazi）。
//	无扩展名/目录 → 保留头部 + …。
//	isDir=true 时不按扩展名处理（目录名里的 . 不是扩展名），直接保留头部。
//	按 CJK 显示列宽计算（中文占 2 列），rune-safe。
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
