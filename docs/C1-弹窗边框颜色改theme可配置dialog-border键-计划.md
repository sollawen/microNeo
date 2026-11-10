# C1：弹窗边框颜色改为 theme 可配置的 dialog-border 键 — 实施计划

## 1. 背景与目标

现状：

- 所有弹窗（ConfirmDialog / InputDialog / MsgDialog / SelectDialog）的边框配色（含 title 与背景）都来自 FloatFrame 的 frameColor
- 调用方传入两种色值：
    - config.DefStyle：主题加载后被 default 键改写（s-light 为 #222222,#fffbee），非零值
    - tcell.Style{} / tcell.StyleDefault：零值，命中 FloatFrame 的零值回落分支，实际渲染为 config.DefStyle
- 传 config.DefStyle 的调用方直接渲染 DefStyle，不走回落分支，边框颜色无法通过 theme 定制

目标：

- theme 里新增 dialog-border 键，统一控制所有弹窗三处配色：
    - 边框
    - title
    - 背景
- dialog-border 键缺失时兜底 DefStyle
- 弹窗文本颜色保持现状

## 2. 可行性结论

可行，改动小。要点：

- FloatFrame.Open 对零值有统一回落分支（frame.go 第 117 行），改回落为 config.DialogBorderColor() 后，零值调用方自动跟随主题
- 传 config.DefStyle 的调用方在主题加载后拿到的是非零值（被 default 键改写），不走回落，必须由调用方改为传零值，统一走回落
- 弹窗框架其余部分零改动

## 3. 现状梳理

### 3.1 边框色的取色链路

- ConfirmDialog / InputDialog / MsgDialog / SelectDialog 的 Open 都接收 frameColor 参数，透传给 FloatOpenSpec.FrameColor
- FloatFrame.Display 用 f.frameColor 绘制三处：
    - 边框
    - title
    - 整块背景
- 调用方传色分两类：
    - 传 config.DefStyle 的 4 处，本次改为传零值：
        - actions.go 的 closePrompt
        - finder 的 delete / rename / add 弹窗
    - 本就传零值的调用方，本次不动：
        - :theme 选择器（command_neo.go）
        - notepane 的 receiver 选择器（两处）
        - finder 的 showError 弹窗
        - command_neo.go 的测试弹窗（MsgTest / InputTest）

### 3.2 DefStyle 非零值的陷阱

config.DefStyle 在主题加载后被 default 键改写（ParseColorscheme 解析到 color-link default 时同步赋值），加载后不是零值。FloatFrame 的零值判定只命中 tcell.Style{} / tcell.StyleDefault，传 config.DefStyle 的调用方不命中回落，直接渲染 DefStyle。所以「只改 FloatFrame 一处」覆盖不到这类调用方。

### 3.3 GetColor 的坑

config.GetColor 遇到未定义键时会把键名当颜色字符串解析（StringToStyle("dialog-border")），结果不可控。取色必须先用 Colorscheme map 查存在性，不能裸调 GetColor。

### 3.4 default 键的作用

- default 键控制正文默认色，ParseColorscheme 解析到它时同步改写包级 DefStyle
- 8 个主题定义 default 是显式指定正文色，保证跨终端一致；solarized 不定义，正文跟随终端默认色，两者都是合法设计，本次都不动
- 本方案兜底链不依赖 default 键，solarized 缺不缺它都不影响

## 4. 方案设计

### 4.1 config 新增 DialogBorderColor helper

两阶兜底，放 colorscheme.go 的 GetColor 附近：

```go
// DialogBorderColor returns the dialog frame color: prefers the dialog-border
// key, falls back to DefStyle when the key is missing.
func DialogBorderColor() tcell.Style {
	if s, ok := Colorscheme["dialog-border"]; ok {
		return s
	}
	return DefStyle
}
```

说明：

- 不裸调 GetColor，理由见 3.3
- Colorscheme 读 nil map 安全（Go 返回零值 + ok=false），启动早期调用也只是拿到 DefStyle

### 4.2 取色统一（FloatFrame 回落 + 调用方改传零值）

两处改动配合，缺一处都覆盖不全：

- frame.go 第 117 行附近，零值回落改为 DialogBorderColor
- 传 config.DefStyle 的 4 处调用方改为传 tcell.Style{}，统一走回落

frame.go：

```go
	fc := spec.FrameColor
	if fc == (tcell.Style{}) {
		fc = config.DialogBorderColor() // 改前 config.DefStyle
	}
```

调用方（以 finder/delete.go 为例，其余 3 处相同）：

```go
		config.DefStyle, // 改前
		→
		tcell.Style{}, // 改后
```

说明：

- config.DefStyle 加载后非零值，必须由调用方改传零值（见 3.2）
- 零值调用方本就传零值，不动：
    - :theme 选择器
    - notepane 选择器
    - showError 弹窗
    - 测试弹窗
- 显式指定其它边框色的能力保留（传非零值不回落）
- 弹窗框架其余部分零改动：confirm / input / msg / select 的函数体不动，仅 4.3 的注释同步

### 4.3 注释同步（不改逻辑）

「零值 = config.DefStyle」的 doc 注释描述的是回落行为，现回落已改为 DialogBorderColor()，注释需同步。仅注释，函数体零改动：

- internal/dialog/frame.go 的 FloatOpenSpec.FrameColor 与 FloatFrame.frameColor 注释
- internal/dialog/confirm.go / input.go / msg.go / select.go 的 frameColor 参数注释
- docs/弹窗机制/弹窗框架设计.md 的「零值 = default」约定

### 4.4 theme 文件改动

9 个 theme 各加一行 color-link dialog-border，具体取值见第 5 节：

```text
color-link dialog-border "<第 5 节取值>"
```

颜色值必须带双引号；漏引号会让整份主题解析失败并静默回落到 default 主题（ParseColorscheme 的 colorParser 正则要求值带引号，未命中即整份主题报错回落）。

### 4.5 不动的部分

- 弹窗文本与按钮颜色（config.DefStyle）
- 零值调用方，本次不动：
    - :theme 选择器
    - notepane 选择器
    - showError 弹窗
    - 测试弹窗
- InfoBar 兜底分支
- 8 个主题的 default 键
- solarized 保持无 default 键

## 5. 配色建议（待实测微调）

取值原则：背景与正文 default 键一致，前景取主题内强调色，与正文有明显差别。背景必须与 default 键一致，否则弹窗内文本行底色（DefStyle 重刷）与边框底色（dialog-border）会出现双色拼贴。

- default.micro → white,black（16 色主题，与正文同色）
- gruvbox.micro → 214,235
- one-dark.micro → #E5C07B,#21252C
- dracula-tc.micro → #BD93F9,#282A36
- monokai.micro → #F92672,#282828
- s-dark.micro → #db7c41,#100F0F
- s-light.micro → #6BA1D7,#fffbee（实测后定稿：浅蓝边框线，背景与正文一致）
- zenburn.micro → 214,237
- solarized.micro → brightyellow,default

标注：s-light 已实测定稿；其余 8 个主题取值是初稿，待实测观感后再微调。

## 6. 边界情况

- 用户自写 theme 缺 dialog-border 键 → 回落 DefStyle
- dialog-border 解析成零值（如 "default,default"）→ 被 FloatFrame 当未指定，回落 DefStyle
- dialog-border 背景必须与 default 键背景一致，否则双色拼贴
- theme 热切换：已打开的弹窗保持旧配色，关掉重开才用新色，属可接受行为
- 弹窗文本颜色与改动前一致

## 7. 验收

- make build 通过
- 各主题下弹窗三处配色跟随 dialog-border 键：
    - 边框
    - title
    - 背景
- 覆盖两类调用方，边框颜色一致：
    - 传零值的弹窗（:theme 选择器 / notepane 选择器 / showError）
    - 改传零值的弹窗（closePrompt、finder delete / rename / add）
- 临时删掉某主题的 dialog-border 键 → 边框回落到 DefStyle
- 反向用例：写一行不带引号的 color-link，确认整份主题回落到 default
- 切换主题后新开弹窗用新配色，已打开的保持旧配色
- 弹窗文本颜色与改动前一致

## 8. 文件清单

internal/config/colorscheme.go：

- 新增 DialogBorderColor helper

internal/dialog/frame.go：

- 零值回落改 config.DialogBorderColor()
- FloatOpenSpec.FrameColor 与 frameColor 注释同步

internal/action/actions.go：

- closePrompt 的 config.DefStyle 改 tcell.Style{}，tcell import 已有

internal/finder/delete.go / add.go：

- config.DefStyle 改 tcell.Style{}
- import 删 config、加 tcell

internal/finder/rename.go：

- InputDialog 的 config.DefStyle 改 tcell.Style{}
- import 删 config，tcell 已有（showError 仍用）

internal/dialog/confirm.go / input.go / msg.go / select.go：

- frameColor 参数 doc 注释同步

runtime/colorschemes/ 下 9 个 .micro：

- 各加一行 color-link dialog-border "<第 5 节取值>"

runtime/help/colors.md：

- 颜色键清单补 dialog-border 一行（说明：弹窗边框 / title / 背景色）

docs/弹窗机制/弹窗框架设计.md：

- 「零值 = default」约定同步
