# NotePane 快速原型计划

## 目标

用最小改动，快速跑起来一个可用的 NotePane 浮窗，验证以下核心概念：
1. 能在编辑器上叠加一个浮窗
2. 浮窗内有完整的编辑能力
3. 快捷键切换开关，模态拦截所有事件
4. 内容持久化到 notes.md

## 策略：不嵌入 BufPane，而是手写极简编辑器

原计划嵌入 `*BufPane` 来复用全部编辑能力，但这引入了 BufWindow 定位、statusLine 隐藏、
action 冲突等一系列问题。

**原型策略改为**：NotePane 自己实现一个最简文本编辑逻辑（单行编辑 → 后续可扩展多行），
不依赖 BufPane/BufWindow。这大大降低复杂度，更快出原型。

> 等原型验证了"overlay pane"这个 UI 概念可行后，Phase 2 再替换为嵌入 BufPane 的完整方案。

## 原型范围

### ✅ 做什么
- `Alt-i` 打开/关闭浮窗
- 浮窗出现在当前光标下方，左右充满编辑窗口
- ASCII box-drawing 边框（`┌─┐│└─┘`）
- 内部可以输入文字、退格删除、左右移动光标
- 内容自动保存到 `~/.config/microNeo/notes.md`
- 完全模态：打开时所有事件被拦截

### ❌ 不做什么（留给完整版）
- 多行编辑（原型只做单行输入）
- 复杂编辑操作（复制粘贴、搜索替换等）
- 颜色/语法高亮
- resize 自适应

## 需要的改动

### 1. 新建 `internal/action/notepane.go`（~150行）

```go
package action

type NotePane struct {
    isOpen    bool
    lines     []string       // notes.md 的内容
    curLine   int            // 当前行
    curCol    int            // 当前列
    x, y      int            // 浮窗屏幕位置
    width     int            // 内容宽度
    height    int            // 内容高度（固定5行）
    noteFile  string         // notes.md 路径
    modified  bool
}

var TheNotePane *NotePane

func NewNotePane() *NotePane       // 初始化，加载 notes.md
func (n *NotePane) Toggle()         // 开关切换
func (n *NotePane) IsOpen() bool
func (n *NotePane) Display()        // 画边框 + 内容
func (n *NotePane) HandleEvent(event tcell.Event)  // 处理键盘输入
func (n *NotePane) open()           // 计算位置，打开
func (n *NotePane) close()          // 保存，关闭
func (n *NotePane) loadFile()       // 从 notes.md 加载
func (n *NotePane) saveFile()       // 保存到 notes.md
```

HandleEvent 逻辑：
- `Alt-i` → 关闭（Toggle）
- 普通字符 → 插入到当前行当前列
- `Backspace` → 删除
- `Enter` → 换行（新增一行）
- `Up/Down` → 上下移动行
- `Left/Right` → 左右移动列
- 其他所有事件 → 吃掉，不穿透

Display 逻辑：
1. 获取当前活跃 BufPane 的屏幕位置和光标位置
2. 计算浮窗位置（光标下方）
3. 用 `screen.Screen.SetContent()` 画 box-drawing 边框
4. 用 `screen.Screen.SetContent()` 画文本内容
5. 用 `screen.Screen.ShowCursor()` 显示光标

### 2. 修改 `cmd/micro/micro.go`（2处，~8行）

**事件分发**（在 resize 判断之后、`InfoBar.HasPrompt` 之前）：
```go
if TheNotePane != nil && TheNotePane.IsOpen() {
    TheNotePane.HandleEvent(event)
} else if action.InfoBar.HasPrompt {
```

**渲染**（在 `InfoBar.Display()` 之后）：
```go
if action.TheNotePane != nil && action.TheNotePane.IsOpen() {
    action.TheNotePane.Display()
}
```

### 3. 修改 `internal/action/globals.go`（~2行）

在 `InitGlobals()` 中添加：
```go
TheNotePane = NewNotePane()
```

### 4. 添加快捷键绑定

在 `defaults_darwin.go` 的 `bufdefaults` 中添加：
```go
"Alt-i": "ToggleNotePane",
```

在 `actions.go` 或 `notepane.go` 中添加 action 注册。

但实际上原型阶段可以更简单：**不在 action 系统中注册**，
直接在 `DoEvent()` 的事件分发中拦截 `Alt-i`：

```go
// 在 NotePane 事件处理中直接检查 Alt-i
if e.Key() == tcell.KeyRune && e.Modifiers() == tcell.ModAlt && e.Rune() == 'i' {
    n.Toggle()
    return
}
```

这样就完全不需要改动 action/defaults 系统！快捷键硬编码在 NotePane 内部。

## 侵入性汇总

| 文件 | 改动量 | 内容 |
|------|--------|------|
| `internal/action/notepane.go` | **新建** ~150行 | 全部 NotePane 逻辑 |
| `cmd/micro/micro.go` | ~8行 | 事件拦截 + 渲染调用 |
| `internal/action/globals.go` | ~2行 | 初始化 |

**对 micro 原生代码零侵入**（只添加，不修改现有逻辑）。

## 实施步骤

1. 创建 `notepane.go`：结构体 + loadFile/saveFile + Toggle + Display + HandleEvent
2. 修改 `globals.go`：InitGlobals 中初始化
3. 修改 `micro.go`：DoEvent 中加事件拦截和渲染
4. `make build-quick` 编译测试
5. 验证：打开 microNeo → 编辑文件 → Alt-i → 看到浮窗 → 输入文字 → Alt-i → 浮窗关闭 → 确认 notes.md 已保存

## 预计时间

30分钟内可以跑通。
