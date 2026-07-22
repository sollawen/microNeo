package action

import (
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/finder"
)

// ---- finder 接线层（owner 与 internal/finder.Session 之间的胶水）----

// OpenFinder 是三个入口（Ctrl-o / noName 的 Ctrl-q / birth hook）唯一统一的对外接口。
// 它算出起始目录和文件名，取当前 BWindow 的屏坐标矩形，调 finder.Session.Open。
// finder 自带预检：矩形过窄/过矮会拒绝开会话并返回 false，此时代码不触发 onClose，
// 而是按入口语义回退——quit 入口窄窗直接退程序（无 Cwd，不写 LastFinderCwd），
// browse 入口静默返回（finder 不再依赖 InfoBar，无法给用户提示）。
//
// 调用契约：h.Buf 已就位、h.BWindow 已拿到真尺寸（由 caller 时序保证：
//   - Ctrl-o / Ctrl-q：用户在已可见的 pane 上按键，尺寸天然就绪；
//   - birth hook：finishInitialize 在首次 Resize 末尾触发，此时 BWindow 已填真值）。
func (h *BufPane) OpenFinder(isQuit bool) {
	var dir, file string
	dir = h.Buf.Dir // 已维护：命名 = filepath.Dir(AbsPath)，noName = 继承/cwd
	if h.Buf.HasFilename() {
		file = filepath.Base(h.Buf.AbsPath) // 仅命名 buffer 预选当前文件
	}
	v := h.BWindow.GetView()
	ok := h.finder.Open(finder.Rect{X: v.X, Y: v.Y, W: v.Width, H: v.Height}, dir, file, isQuit, h.onFinderClose)
	if !ok {
		// 预检放不下：finder 没开会话、没触发 onClose。quit 入口窄窗直接退。
		if isQuit {
			h.Quit()
		}
		return
	}
}

// onFinderClose 是 finder 会话关闭时的统一回调，按 Result.Reason 一维分发。
//   - Picked：选中的文件若与当前 buffer 同路径则 no-op（避免重入），否则 OpenCmd 换入；
//   - Quit：先暂存导航目录（供退出时上报 shell），再调原生 Quit（自带存盘提示）；
//   - Esc / Resize：no-op（取消，回编辑态）。
//
// 注意它是 *BufPane 的方法而非包级函数：回调体要调 h.OpenCmd / h.Quit，
// 三个 caller 的 h 是不同的 *BufPane 实例；OpenFinder 传 h.onFinderClose（方法值）
// 会把当次的 h 绑定带走。
func (h *BufPane) onFinderClose(r finder.Result) {
	// noName buffer 跟着 finder 走：关闭时把最后停留目录同步到 buffer.Dir，
	// 下次再开 finder 就停在这里。命名 buffer 不动（Dir 在创建/保存时
	// 维护成 filepath.Dir(AbsPath)，手动改会偏离真相）。
	if !h.Buf.HasFilename() && r.Cwd != "" {
		h.Buf.Dir = r.Cwd
	}

	switch r.Reason {
	case finder.Picked:
		if filepath.Join(r.Cwd, r.File) == h.Buf.AbsPath {
			return // 同文件 no-op
		}
		h.OpenCmd([]string{filepath.Join(r.Cwd, r.File)})
	case finder.Quit:
		h.doQuit(r)
	case finder.Resize, finder.Esc:
		// no-op
	}
}

// doQuit 先暂存 finder 导航到的目录，再调原生 Quit。
// 顺序不能反：Quit 之后 pane 状态已被销毁，r.Cwd 虽是值拷贝不受影响，
// 但语义上「记录最后导航成果」必须在 pane 仍存活时落定。
func (h *BufPane) doQuit(r finder.Result) {
	LastFinderCwd = r.Cwd
	h.Quit()
}

// LastFinderCwd 是包级暂存变量：finder 会话因 Quit 关闭时由 doQuit 写入，
// 退出程序时由 cmd/micro 的 lastWorkingDir 读取上报给 shell 的 cwd-file。
// 空串表示本次会话没用 finder 做过导航（用户在 file-born pane 里直接退出）。
var LastFinderCwd string

// onOwnerBlur 在 owner（BufPane）失焦时调用：若 finder 会话开着，通知它取消。
// 触发点是 BufPane.SetActive(false)——Tab 的鼠标路由（切焦点/拖分割条）在 pane 的
// HandleEvent 转发之上执行，owner 的 modal return 拦不住点别的 pane 这条路径，
// 故用失焦通道兜底：点框外 → owner 失焦 → finder 按 Esc 自关。语义等价「点框外取消」。
func (h *BufPane) onOwnerBlur() {
	if h.finder != nil && h.finder.IsOpen() {
		h.finder.NotifyBlur()
	}
}
