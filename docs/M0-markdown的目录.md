我想在microNeo里面加个功能：列出pane里面正在编辑的文件的目录
- 在pane里面编辑的时候，按某一个快捷键，就弹出一个select dialog。里面是这个文档的各级标题的目录。
	- markdown 文件，就是所有的各级的标题 #/##/###/......
	- 其它文件忽略此热键
- 然后用户在select dialog里面，如果enter了一个条目，就关掉select，并跳到这个条目的所在行

初步设想的UI
- 这个selectDialog，宽度固定是pane的宽度的40%（窄 pane 有 20 列下限，见实施计划），靠在pane的右边。高度和 pane 的文本区相同（不含 pane 自己的 statusline 行）。
- 里面的每行，是一个md的标题+行号
	- 标题居左对齐，包括完成的#号，也就是说各级可以直观看#号就知道了
	- 行号居右对齐
- 这样，selectDialog返回的enter选中的那行string，就完整的包括了标题和行号了

显示效果示例：

```
# 1. 总纲･･･････････････････1
## 1.1 需求定义････････････10
## 1.2 名词定义････････････25
# 2. 模块划分･･････････････37
## 2.1 base层模块･･････････59
## 2.2 engine层模块･･･････120
```

估计Select dialog功能不太够用，那就新写一个dialog吧
- 可以指定位置和大小：锚点+宽高，这两个参数由调用者计算并传给dialog,
- 还可选传入 边框颜色，正文颜色。不传就是默认
- 传入的数据是 string[]，每条都是排好版的整行文本（标题+引导线+行号），每条string的长度都是相同的
- enter后，反回的是选中的那一条 string，行号就在这条串的最右边

命令与键位
- 这个新 dialog 命名为 `OutlineDialog`
- 增加 `:outline` 命令，效果是打开当前 pane 的 `OutlineDialog`
- 默认键位 `Alt-o` 绑在 `:outline` 命令上
- 用户可以在自己的 `bindings.json` 里改键位
- 用户也可以把 `Alt-o` 改绑到别的 action

---

## 实施计划

### 目标

MD 文件内按 Alt-o（或 `:outline` 命令）弹出 `OutlineDialog`，Enter 跳到标题所在行，非 MD 文件忽略该热键。不动 FloatFrame、不动渲染层。
行数估计：
- md 层 ~80 行（含单测 ~120 行）
- dialog 层 ~130 行
- action 层 ~60 行
- 改现有文件 ~5 行

键位与命令解耦：默认绑定写 `"Alt-o": "command:outline"`，用户 `bindings.json` 里的绑定在默认键位之后加载，可直接覆盖。

### 几何量来源

`BufWindow` 里嵌的是 `*View` 指针（bufwindow.go:18），所以 `w.Width` / `w.Height` / `w.X` / `w.Y` 就是 View 的字段，`Resize` 时同步更新。两套取法意思不同，别混：

| 量 | 来源 | 含义 |
|---|---|---|
| pane 总宽 | `v.Width`，`v := h.GetView()` | pane 全部宽度，含 gutter 与滚动条 |
| pane 总高 | `v.Height` | pane 全部高度，含 pane 自己的 statusline 行 |
| pane 左缘 / 顶 | `v.X` / `v.Y` | 弹窗右缘 = `v.X + v.Width` |
| 编辑区高 | `h.BufView().Height` | **已扣掉 pane 自己的 statusline 行** |
| 文本区宽 | `h.BufView().Width` | 扣掉 gutter 与滚动条 |

`GetView()` 与 `BufView()` 的差别：
- `GetView()` 走的是 View 本身的字段，Width/Height = pane 总尺寸
- `BufView()` 另造一个 View 字面量（bufwindow.go:140-150），Width = `bufWidth`（扣 gutter 与滚动条），Height = `bufHeight`（扣 statusline），X 还要加上 `gutterOffset`

先例：
- finder 就是用 `h.BWindow.GetView()` 拿 pane 矩形（fileops.go:30-31），然后自己 `rect.H - 1` 留 statusline（finder/session.go:98）
- action 层拿编辑区高度用 `h.BufView().Height`（bufpane.go:420 有先例）

### 宽度口径

本功能用到三种宽度，名字全程统一，别混：

| 名字 | 算式 | 含义 |
|---|---|---|
| `w` | 调用方算 | 弹窗外框总宽，含左右边框各 1 列 |
| `C` | `w - 2` | 内容区宽，也就是边框里面的宽度，对应 FloatFrame 的 `ContentSize.W` |
| `textW` | `C - 1 = w - 3` | 条目文本可用宽，md 层拼串就按这个宽 |

为什么文本宽要比内容区宽再减 1：
- ▲▼ 指示器落在内容区最后一列（SelectDialog 就是这么画的）
- 条目文本只能画到内容区倒数第二列，能用的列数 = `C - 1`
- ▲▼ 与行号之间不留空列，指示器紧贴着行号的最后一位（已定）

所以：
- action 层把 `textW` 传给 md 层
- md 层拼出的每条串，显示宽度都等于 `textW`
- 如果拿 `C` 去拼，串会比可用列数多 1，指示器会盖掉行号的最后一位

最小宽度：
- `textW` 小于 12 列就不执行 `:outline`，InfoBar 提示一句
- 12 列的依据：标题部分至少留 10 列给内容看，中间 1 列分隔符，剩下的给行号
- 标题实际可用列数 = `textW - 该行行号位数 - 1`，每行按自己的行号位数算
  - textW = 12、行号 1 位数时，标题有 10 列
  - textW = 12、行号 3 位数时，标题只剩 8 列
- 换算到 pane 宽度：textW = w - 3，w 又不超过 pane 宽，所以 pane 窄到 14 列以内就不弹目录

宽度策略与「40%」的偏离：
- 弹窗宽 `w = min(max(pane宽*4/10, 20), pane宽)`
- 20 列是下限，pane 宽不足 50 列时 40% 算出来不到 20，弹窗就取 20 列
  - pane 宽 40 → 40% 是 16 → 取下限 20 → 实际占 pane 的 50%
  - pane 宽 15~19 → w = pane 宽 → 弹窗占满整个 pane 宽度
- 这是有意为之：标题至少要 10 列才看得出内容，宁可宽一点

### 复用依据

计划全部建立在代码库已有机制上，不新写同类逻辑：
- 标题检测：`md.DetectSegments` 已识别 `#` 标题（detect.go:83），fence 跟踪保证 codeblock 里的假标题不会混入
- 浮窗容器：`FloatFrame`（dialog/frame.go）已做边框/标题/清除/生命周期/事件路由，`AutoExpand=false` 时 Anchor 直接作为外框左上角，正好满足「调用者传锚点+尺寸」
- 行号跳转：`GotoCmd`（action/command.go:908）已有 clamp + 去多光标 + `GotoLoc` 的完整写法，照搬
- 键盘与滚动：SelectDialog 的 Up/Down + topIdx 视口逻辑（dialog/select.go）复制改造
- MD 判断：`h.Buf.IsMD`（buffer.go:92）现成
- pane 矩形：`h.GetView()` 拿 pane 总宽高（finder 同款，fileops.go:30），`h.BufView()` 拿编辑区高度
- 显示宽度：用 `github.com/mattn/go-runewidth` 的 `RuneWidth` / `StringWidth`，md 层与 dialog 层必须同源
    - md 包内已有先例（internal/md/inline.go:81）
    - 不要用 `util.StringWidth`，它的签名是 `StringWidth(b []byte, n, tabsize int) int`（internal/util/util.go:208），要传字节切片 + rune 个数 + tabsize，而 md 层拿不到 tabsize
    - 标题里的 `\t` 一律按 1 列算：runewidth 对 `\t` 记 0 宽，tcell 实际画 1 格，两边口径不同，md 层先把 tab 换成空格最省事

### 分层

职责分三层，边界清楚：
- md 层：
    - 找出哪些 buffer 行是标题
    - 按调用方给的宽度拼成等宽条目串
    - 行号放在串的最右端
- action 层：
    - 算锚点与尺寸
    - 把宽度传给 md 层
    - 开弹窗
    - 解析行号并跳转
- dialog 层：
    - 只渲染等宽的条目串
    - 处理上下选择与 ▲▼
    - 不排版

扩展口：将来要支持代码文件（函数索引之类），只换 md 层那个函数，约定不变——返回等宽串、行号在串尾。action 层与 dialog 层一行都不用改。

### 改动文件总览

| 文件 | 动作 | 内容 |
|---|---|---|
| internal/md/collect_headings.go | 新增 | `CollectHeadings`（返回等宽条目串） |
| internal/md/collect_headings_test.go | 新增 | CollectHeadings 单测 |
| internal/dialog/outline.go | 新增 | `OutlineDialog`（锚点+尺寸列表弹窗） |
| internal/action/outline_neo.go | 新增 | `OutlineCmd`（算锚点尺寸、开弹窗、解析行号跳转） |
| internal/action/command_neo.go | 加一行 | 注册 `:outline` 命令 |
| internal/action/defaults_darwin.go | 加一行 | 键位 `Alt-o` → `command:outline` |
| internal/action/defaults_other.go | 加一行 | 键位 `Alt-o` → `command:outline` |

### 任务 1：md 包新增 `CollectHeadings`

md 层负责把标题行变成可直接显示的条目串：标题原文 + 引导线填充 + 右对齐行号，行号在串的最右端。

行号基准跟 pane 的 gutter 一致，用 1 基（人看的行号，第一行是 1）：
- gutter 那边是 `bloc.Y + 1`（internal/display/bufwindow.go:386），目录里也用 1 基，两边对得上
- buffer 内部行号是 0 基，md 层输出时 `+1`，action 层解析回来再 `-1`
- `relativeruler` 开启时 gutter 会显示相对行号，目录里一律显示绝对行号

不导出任何结构体：层级靠标题文本开头的 `#` 号体现，行号靠串尾的数字体现，中间过程用私有 struct 承载即可。

对外只有一个函数：

```
// CollectHeadings 扫描 buffer，返回可直接显示的 Outline 条目串。
// 每条 = 标题原文 + 引导线填充 + 右对齐行号，行号在串的最最右端。
// width = 调用方算好的条目文本可用宽（= 内容区宽 - 1，指示器那列已扣掉）。
// CollectHeadings 不再自己扣指示器列，直接按 width 拼装。
// 所有条目串的显示宽度都等于 width。
func CollectHeadings(buf BufferReader, width int) []string
```

伪代码（对应任务 1）：

```
func CollectHeadings(buf BufferReader, width int) []string:
    hits := []heading{}                 // 私有 struct: line int, text string
    for seg in DetectSegments(buf, 0, buf.LinesNum()-1):
        if seg.BufStartLine != seg.BufEndLine: continue
        line := trimSpace(buf.LineBytes(seg.BufStartLine))
        if !hasPrefix(line, "#"): continue      // 非 heading segment
        hits = append(hits, heading{seg.BufStartLine, line})
    if len(hits) == 0: return nil
    maxLine := hits 里最大的行号
    numW := digits(maxLine)                     // 最大行号的位数
    if width-numW-1 < 1: return nil             // 连「标题1字+分隔符+行号」都放不下
    out := make([]string, 0, len(hits))
    for h in hits:
        out = append(out, buildEntry(h.text, h.line+1, width))   // 行号用 1 基，跟 gutter 一致
    return out

// buildEntry 拼一条条目串
// 标题从右边截断，至少留 1 个字符
// 结果的显示宽度恒等于 width
func buildEntry(text string, line, width int) string:
    num := itoa(line)                    // 行号不带前导空格
    avail := width - 显示宽度(num) - 1    // 至少 1 个分隔符
    t := 按显示宽度截断(text, avail)
    fill := width - 显示宽度(num) - 显示宽度(t)
    return t + repeat("･", fill) + num
```

其中：
- `显示宽度(s)` = `runewidth.StringWidth(s)`
- `按显示宽度截断(s, avail)` = 逐 rune 累加 `runewidth.RuneWidth`，累加超过 avail 就停，不能把宽字符切成两半

要点：
- 每条串至少装得下「10 个标题字符 + 1 个分隔符 + 行号」，这是下限
- 标题太长就从右边截断（保留左边的 `#` 号与开头文字）
- 所有条目串的显示宽度相同，都等于 width
- 字节长度可能不同（CJK 占 2 列），对齐一律按显示宽度算，用 `runewidth.StringWidth`
- 标题里的 `\t` 先换成空格再算宽度，避免 runewidth（0 宽）与 tcell（1 格）两边口径不一致
- 分隔符用 `･`（U+FF65 半角片假名中点）：
  - 恒定占 1 列，跟英文字母同宽，不会变 2 列
  - 字形在基线偏上，是「上下居中的小点」，跟标题里贴基线的小数点一眼能分开
  - 标题里不会出现，不会和小数点、减号混淆
  - 宽度固定 1 列，每行直接按列数对齐，不用算余数
- 标题与行号之间至少留 1 个分隔符，尾部连续数字才唯一等于行号，不会和标题里的数字粘连
- 行号不带前导空格，行号左边紧挨着分隔符
- 行号右对齐靠「所有条目串同宽」自然实现，每条串都以行号结尾，最右一列对齐
- codeblock 免疫靠 DetectSegments 的 fence 跟踪，md 内部解决，调用方不用操心

单测放在 collect_headings_test.go（同包引用 detect_test.go 里的 mockBuffer 即可），覆盖这些情况：
- codeblock 内的 `#` 行不计入
- 多级标题的条目串正确（`#` 号原样带出）
- 条目串尾部数字就是行号，可直接解析出来
- 无标题文件返回空列表
- 标题超长时从右边截断，仍保留分隔线与行号
- 宽度不够放下「1 字标题 + 分隔符 + 行号」时返回空
- 每条串的显示宽度都等于 width（核心不变式，用 runewidth.StringWidth 断言）
- CJK 标题按 2 列算，截断不把宽字符切成两半
- 行号位数变化：同一文件第 9 行与第 10 行的标题，两条串宽度仍相等、引导线长度不同
- 标题在 buffer 中部时行号正确（第 3 行标题 → 串尾是 3）
- 标题以数字结尾（`# 第2章`）时，尾部数字仍然是行号


### 任务 2：dialog 包新增 `OutlineDialog`

「锚点+尺寸」列表弹窗，能力上做成通用件，但按需求命名 `OutlineDialog`，文件放 internal/dialog/outline.go。

整体形态跟 `SelectDialog` 差不多，同一个 `FloatFrame` 容器、同一套键盘与滚动逻辑，代码基本可以直接抄 `internal/dialog/select.go` 再改：
- 抄的部分：
  - `Open` 的参数布局与顺序（items 在前、title 第二，跟 select.go:52 一致）
  - `FloatOpenSpec` 的填法
  - `handleEvent` 的 Up/Down/Enter/Esc 分支
  - `ensureVisible` 视口滚动
  - `display` 里画选中行 reverse 高亮与 ▲▼ 指示器
- 改的部分：
  - `AutoExpand=false`，锚点直接当外框左上角（SelectDialog 是 true，会自动贴着光标展开）
  - 内容区宽由调用方给定，不自己按最长条目算
  - 文本从 `area.X` 起画，没有左侧 padding（SelectDialog 左边空 1 列）
  - 逐 rune 画，推进列数用 `runewidth.RuneWidth(r)`，不能每个 rune 只推进 1 列
  - 视口高度由 `size.H` 决定，不单独传 `maxVisible`
  - Up/Down 固定回绕，不对外暴露 wrap 参数（SelectDialog 把它当参数传，这里固定 true）
  - 回传整条串而不是下标
  - 多一个 `itemStyle` 参数

`Open` 参数（顺序跟 select.go:52 保持一致）：

| 参数 | 类型 | 说明 |
|---|---|---|
| items | []string | 整行显示文本，调用方已排好版（含引导线与行号） |
| title | string | 顶边标题 |
| anchor | Pos | 外框左上角（AutoExpand=false 语义） |
| size | Size | 纯内容区宽高，不含边框（内容区宽 = w - 2） |
| frameColor | tcell.Style | 零值 = 默认边框色 |
| itemStyle | tcell.Style | 零值 = config.DefStyle |
| onSelect | func(*string) | Enter 传选中条目的串，Esc/resize 传 nil |

标题不能把外框撑宽：`FloatFrame.Open` 里 `outerW = max(C+2, len(title)+6)`（frame.go:96-99），标题太长外框会变宽而内容区宽度不变，右边多出一段空白。
- 标题 "Outline" → len+6 = 13
- `textW ≥ 12` 保证 `w ≥ 15` → 13 ≤ w，撑不到
- 以后换长标题时要重新算这个不等式

dialog 只干三件事：
- 逐行渲染 items（调用方保证等宽，渲染时不再逐条截断）
- 内容区最右一列留给 ▲▼ 指示器
- 选中行 reverse 高亮

视口高度：`visibleH = min(len(items), size.H)`，`ensureVisible` 与 ▲▼ 的显示判断都用它。

回传整条串而不是下标：调用方要的行号就在串尾，自己解析即可。

Open 失败的处理：`FloatFrame.Open` 返回 false（已有浮窗、屏幕放不下）时，直接 `onSelect(nil)`，不设任何状态，与 SelectDialog 的做法一致。

显示规则：
- 文本左对齐，从内容区第一列（`area.X`）开始逐 rune 画，画到 `area.X+area.W-2` 止（最后一列留给指示器）
- 每画一个 rune，列数推进 `runewidth.RuneWidth(r)`，不能只推进 1 列：tcell 画宽字符占 2 格，只推进 1 列会让下一个字符写进宽字符的第二格，整行错位
- items 等宽，行号自然右对齐成一列，dialog 不二次排版
- ▲▼ 落在内容区最后一列，出现时机与着色逻辑照抄 SelectDialog
- 选中行 reverse 高亮

显示效果示例：

```
# 1. 总纲･･･････････････････1
## 1.1 需求定义････････････10
## 1.2 名词定义････････････25
# 2. 模块划分･･････････････37
## 2.1 base层模块･･････････59
## 2.2 engine层模块･･･････120
```

键盘处理照抄 SelectDialog.handleEvent：Up/Down 移动并 ensureVisible，Enter/Esc 先 `TheFloatFrame.Close()` 再回调，其余按键吞掉。
- Up/Down 回绕：到顶再按 Up 跳到最后一条，到底再按 Down 跳回第一条
- 跳完照样调 ensureVisible，视图跟着滚到新位置（从顶跳到底时直接落到最后一屏）
- ensureVisible 用 `visibleH = min(len(items), size.H)`

resize 不用自己处理，FloatFrame 全包了（frame.go:223）：
- `FloatFrame` 拦下 `*tcell.EventResize`，先 `Close()` 再调 `OnResize` 回调
- `OutlineDialog` 把 `OnResize` 接到 `onSelect(nil)`，跟 SelectDialog 同一个接法
- 弹窗在 resize 的瞬间就没了，不存在「旧宽度排的版被拉到新宽度上」的错位，条目不用重排
- 用户 resize 完再按 `Alt-o`，action 层拿新的 `v.Width` 重算宽高，重排一遍

### 任务 3：新增 `:outline` 命令并把 `Alt-o` 绑上去

新文件 internal/action/outline_neo.go，签名 `func (h *BufPane) OutlineCmd(args []string)`。

命令注册放在 internal/action/command_neo.go 的 `InitNeoCommands` 里（那里已有 theme/file/quit 的注册先例）：

```
MakeCommand("outline", (*BufPane).OutlineCmd, nil)
```

键位默认绑定加在 defaults_darwin.go 与 defaults_other.go 的 bufdefaults：

```
"Alt-o": "command:outline",
```

伪代码（对应任务 3）：

```
func (h *BufPane) OutlineCmd(args []string):
    if !h.Buf.IsMD: return                // 非 MD 静默忽略
    v := h.GetView()                     // pane 矩形：总宽高 + 左缘
    w := min(max(v.Width*4/10, 20), v.Width)   // 40% 宽，20 列下限，且不超 pane 宽
    textW := w - 3                        // 去掉左右边框各 1 列 + 指示器 1 列
    if textW < 12:
        InfoBar.Message("pane too narrow for outline")
        return                           // 标题看不下，直接不执行
    outerH := h.BufView().Height          // 弹窗外框高 = pane 编辑区高（已扣 pane 自己的 statusline）
    scrH, _ := screen.Screen.Size()
    limit := scrH - config.GetInfoBarOffset() - 1 - v.Y   // 从 pane 顶到屏幕底允许的外框高
    outerH = min(outerH, limit)          // 屏幕放不下就矮一点
    items := md.CollectHeadings(h.Buf, textW)   // 每条显示宽度都等于 textW
    if len(items) == 0:
        InfoBar.Message("No headings found")
        return
    dialog.NewOutlineDialog().Open(items,
        "Outline",
        dialog.Pos{v.X + v.Width - w, v.Y},   // 右缘贴 pane 右边，顶边对齐 pane 顶
        dialog.Size{w - 2, outerH - 2},        // 纯内容区尺寸，不含边框
        tcell.Style{}, tcell.Style{},
        func(picked *string):
            if picked == nil: return      // Esc/resize
            line := parseTrailingInt(*picked)   // 取尾部连续数字
            if line < 1: return                 // 解析失败就不跳
            line = util.Clamp(line-1, 0, h.Buf.LinesNum()-1)   // 转回 buffer 的 0 基并夹住
            h.RemoveAllMultiCursors()
            h.Cursor.Deselect(true)
            h.GotoLoc(buffer.Loc{0, line}))
```

高度口径：
- 弹窗外框高 = `h.BufView().Height` = pane 的编辑区高度，`BufView()` 已经把 pane 自己的 statusline 行扣掉了（bufwindow.go:168-171）
- 外框含上下边框 2 行，所以内容区高 = `outerH - 2`
- `FloatFrame.Open` 的前置检查是 `outerH > 屏高 - infobar偏移 - 1 → 拒绝`（frame.go:104-107）
  - 默认配置（pane 自己的 statusline 开）：编辑区高恰好等于这个上限，clamp 不生效
  - pane 自己的 statusline 关掉时：编辑区高多 1 行（那一行让给了 statusline），会超出上限，clamp 掉 1 行
  - 这 1 行是屏幕物理限制：浮窗外框不能压到 infobar
- 所以「弹窗高度 = 编辑区高度」在默认配置下完全成立

宽度口径见上文「宽度口径」小节，伪代码里再强调一遍：
- textW = w - 3 = 内容区宽 C 再减去指示器 1 列
- 传给 md 层的必须是 textW，不是 C

窄宽度的处理：
- 弹窗宽度取 40%，但不能超过 pane 宽度，否则弹窗比 pane 还宽
- textW = 弹窗宽 - 边框 2 列 - 指示器 1 列
- textW 小于 12 列就直接不执行 `:outline`，InfoBar 提示一句，不弹窗
- textW 够用但标题太长时，由 md 层把标题从右边截断，保留引导线与行号

行号解析规则：
- 取条目串尾部连续的十进制数字，那就是行号
- 尾部数字的前一个字符必须是分隔符 `･`，不满足就当作取消处理
    - 这条约束靠 md 层「至少补 1 个分隔符」兑现，两边都不能破
    - 破了也不会跳错行（解析失败就不跳），但会变成“按了没反应”
- 标题与行号之间至少 1 个分隔符（`･`），保证尾部数字不会和标题里的数字连成一片
- 行号是 1 基，跟 pane 的 gutter 对得上；跳转前 `-1` 转回 buffer 的 0 基
- 解析不到数字就当作取消处理，不跳转

拼接规则（在 md 层）：
- 所有条目串的显示宽度相同，都等于 width
- 标题按显示宽度截断后用引导线补齐到同一宽度，引导线一直补到行号前一列
- 行号不带前导空格，左边紧挨分隔符；行号右对齐靠所有条目串同宽自然实现
- 字节长度可能不同（CJK 占 2 列），对齐一律按显示宽度算，用 `runewidth.StringWidth`
- 引导线用 `･`（U+FF65），恒 1 列的居中小点，不会和标题里的「1.1」「kebab-case」混淆
- 标题与行号之间至少留 1 个分隔符，尾部连续数字才唯一等于行号，不会和标题里的数字粘连
- 行号不带前导空格，行号左边紧挨着分隔符
- 行号右对齐靠「所有条目串同宽」自然实现，每条串都以行号结尾，最右一列对齐

为什么走 `command:outline` 而不是自定义 BufKeyAction：
- 命令与键位解耦，改键位不用碰代码
- `command:` 前缀由 BufMapEvent 直接识别（bufpane.go:116），命令走 `MainTab().CurPane().HandleCommand("outline")`，与手打 `:outline` 完全同一条路径
- 不需要往 `BufKeyActions` 里注册，少一处注册点

用户改键位的路径：
- `bindings.json` 写 `{"Alt-o": "command:outline"}` 保持默认，或把命令挪到别的键（如 `{"Alt-i": "command:outline"}`）
- `InitBindings` 先灌默认键位再灌用户 bindings.json，用户配置后加载，天然覆盖默认
- 原生 `:unbind` 会把默认绑定恢复回去，删不掉默认键位；要彻底停用 Alt-o 得把它改绑到一个无害 action

### 任务 4：编译与手测

`make build` 编译通过后逐项手测：
- 弹窗几何：MD 文件按 Alt-o 弹出且符合设计
    - 右缘贴 pane 右边
    - 宽度 40%（pane 宽 40 时是 20 列下限，占 50%）
    - 高度等于 pane 编辑区高度（pane 自己的 statusline 不被盖）
    - pane 自己的 statusline 关掉时也能弹出来（高度 clamp 生效，矮 1 行）
    - 各级标题与行号显示正确
- 跳转：Enter 选中的条目跳到对应行
- 排版：引导线填充到行号前，行号右对齐成一列，每行显示宽度相同
- 行号解析：标题里带数字的条目（如 `# 1. 总纲`）选中后仍跳对行，不被标题里的数字干扰
- 中日韩标题：中文标题按 2 列算，截断不切坏字符，整列对齐
- 指示器：条目超过高度时 ▲▼ 出现且可滚动
    - 能滚到顶、滚到底
    - Up/Down 回绕：顶再按 Up 跳到最后一条，底再按 Down 跳回第一条，视图跟着滚
- 竖向分屏：弹窗贴当前 pane 的右边，不越界
- 已有别的浮窗打开时按 Alt-o：什么都不发生（事件全被 FloatFrame 吃掉，action 层不会被调用）
- codeblock 免疫：`# 注释` 不出现在目录
- 非 MD 文件：按 Alt-o 无反应
- 关闭：Esc 与 resize 都能关掉弹窗
- 空目录：无标题 MD 文件只出 InfoBar 提示，不弹窗
- 命令入口：在命令栏敲 `:outline` 能打开弹窗，效果与按 Alt-o 一致
- 可配置：在 `bindings.json` 写 `{"Alt-o": "command:outline"}` 或改成别的键，重启后生效
- 窄 pane：pane 宽 14 列以内时按 Alt-o 不弹窗，InfoBar 提示一句
- 长标题：标题长到放不下时从右边截断，条目仍保留引导线与行号

### 边界行为

这些跟 md 渲染层保持一致，不特殊处理，但写明免得被当成漏做：
- setext 标题（标题下面跟一行 `===`）不算标题，micro 的渲染层也只认 `#` 开头（detect.go:83）
- front matter（`---` 包裹）里的 `#` 注释会被当标题收进来，跟渲染层一致
- `#######`（7 个以上 `#`）也算标题，渲染层只判 `#` 前缀（detect.go:83），不额外限制层级
- MD 判定按扩展名：只有 `.md` / `.markdown` 算 MD（internal/md/config.go:70），无扩展名的 `README` 不触发
- 行号一律是绝对行号，`relativeruler` 开启时也不显示相对行号

### 风险与验证点

- ESC 联动 editMode：需验证一次
    - 弹窗打开时事件由 FloatFrame 接管，理论上到不了 observeEditModeToggle
    - 即使误切到阅读模式也无功能损害，任务 4 顺手确认
- 弹窗高度：外框高 = `h.BufView().Height`（pane 编辑区高，已扣掉 pane 自己的 statusline 行），再对屏幕底线 clamp
    - 默认配置下 clamp 不生效，弹窗高度就是编辑区高度
    - pane 自己的 statusline 关掉时编辑区高多 1 行，clamp 掉（屏幕物理限制，浮窗不能压 infobar）
    - finder 的做法一致：拿 pane 总高再自己减 1 留 statusline（finder/session.go:98）
- 全量 O(n) 扫描：弹窗打开的一次性开销，先不做缓存，实测慢再优化
- 行号靠串尾数字反解：靠「标题与行号之间至少 1 个分隔符」保证不粘连，解析时还要校验数字前一个字符是分隔符
- 行号位数变宽的边角：行号从 2 位变 3 位时，引导线长度与标题可用宽度按每行自己的位数变化（宽度口径一节已有规则）
    - textW ≥ 12 时，3 位行号的标题还有 8 列，放得下
- md 层返回字符串而不是结构体：扩展口要求「只换 md 层实现，action/dialog 不变」，字符串契约保住了这一点，代价是 action 层要解析行号
    - 不返回结构体的原因：dialog 层只认字符串（需求定的是「enter 返回选中的那一条 string」），行号得随文本一起传
    - 如果 md 层返回 `{Text, Line}`，action 层只能拿选中的文本反查行号，而两个同名标题（`# 概述` 出现两次）就会反查错
    - 以后真要做代码文件函数索引，md 层换成按函数名分组的条目串，契约不变
