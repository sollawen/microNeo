
## Theme文件的目录

microNeo 使用 `$XDG_CONFIG_HOME/microNeo/`（默认 `~/.config/microNeo/`）作为配置目录。

## 配色方案 Theme

microNeo内置多种不同的配色方案，也可完全定制自己喜欢的配色方案。

### 切换 Theme 的方法：

1. 在编辑界面里，按 `Ctrl+E` 进入命令行
1. 输入 `theme 回车`
1. `上/下键`选择，然后`回车`
1. 新的配色方案生效


### 内置的 Themes


microNeo 目前内置 9 种配色方案：

| Theme | 说明 |
| --- | --- |
| default | 纯16色，兼容任何老式 8/16 色终端。特别难看 |
| dracula-tc | Dracula 主题（true color）。深紫蓝底，色彩鲜明。 |
| gruvbox | Gruvbox 复古暖色（256 色）。 |
| monokai | 经典 Monokai 配色（true color）。 |
| one-dark | Atom One Dark 风格（true color）。 |
| s-dark | microNeo 自定义暗色主题，深黑底。 |
| s-light | microNeo 自定义亮色主题，暖白底。 |
| solarized | Solarized 配色（ANSI 色，依赖终端的 solarized palette）。 |
| zenburn | Zenburn 低对比度柔色（256 色），长时间阅读更护眼。 |

### 自定义theme

用户自定义的theme文件放在 `~/.config/microNeo/colorschemes/` 下。

以内置的 `s-dark` 为例，一个 theme 文件的内容如下：

```micro
color-link default "#b5b8b6,#100F0F"
color-link comment "#7C7C7C,#100F0F"
color-link identifier "#F9EE98,#100F0F"
color-link keyword "#b58654,#1f1a10"
color-link constant "#FF73FD,#100F0F"
color-link constant.string "#d4d4d4,#100F0F"
color-link statement "#96CBFE,#100F0F"
color-link symbol "#96CBFE,#100F0F"
color-link preproc "#62B1FE,#100F0F"
color-link type "bold #4290c9,#100F0F"
color-link special "bold #db7c41,#100F0F"
color-link underlined "#D33682,#100F0F"
color-link error "bold #FF4444,#100F0F"
color-link todo "bold #FF8844,#100F0F"
color-link hlsearch "#000000,#B4EC85"
color-link statusline "#c0c0c0,#303030"
color-link statusline.special "#101010,#db7c41"
color-link statusline.dim "#f0f0f0,#4070a0"
color-link statusline.normal "#a0a0a0,#404040"
color-link tabbar "#100F0F,#C5C8C6"
color-link indent-char "#505050,#100F0F"
color-link line-number "#656866,#100F0F"
color-link current-line-number "#a5a8a6,#100F0F"
color-link diff-added "#00AF00"
color-link diff-modified "#FFAF00"
color-link diff-deleted "#D70000"
color-link gutter-error "#FF4444,#100F0F"
color-link gutter-info "#666666,#100F0F"
color-link gutter-warning "#EEEE77,#100F0F"
color-link cursor-line "#202020"
color-link color-column "#2D2F31"
#color-link symbol.brackets "#96CBFE,#100F0F"
#No extended types (bool in C, etc.)
#color-link type.extended "default"
#Plain brackets
color-link match-brace "#100F0F,#62B1FE"
color-link tab-error "#D75F5F"
color-link trailingws "#D75F5F"
color-link scrollbar "#707070,#100F0F"
color-link message "#6090b0,#100F0F"
color-link selection "#d5d1c0,#6a6049"

# Markdown 专用
color-link md-header "bold #db7c41,#100F0F"         # ← 原 special
color-link md-hr "bold #db7c41,#100F0F"             # ← 原 special
color-link md-blockquote "#62B1FE,#100F0F"          # ← 原 preproc
color-link md-bold "#d4d4d4,#100F0F"                # ← 原 constant.string
color-link md-italic "#b5b8b6,#100F0F"              # ← 原 default
color-link md-strikethrough "#b5b8b6,#100F0F"       # ← 原 default
color-link md-inline-code "#85b654,#1f1a10"         # ← 原 keyword
color-link md-list "#96CBFE,#100F0F"                # ← 原 statement
color-link md-checkbox "#96CBFE,#100F0F"            # ← 原 statement
color-link md-link "#FF73FD,#100F0F"                # ← 原 constant
color-link md-image "#D33682,#100F0F"
color-link md-url "#FF73FD,#100F0F"                 # ← 原 constant
color-link md-codeblock "#b5b8b6,#201F1F"           # ← 无语言代码块文字
color-link md-frame "#505050,#100F0F"               # ← 新增（装饰边框）
color-link md-frame-label "#7090b0,#100F0F"         # ← 新增（代码块语言名）
color-link md-misc "#62B1FE,#100F0F"                # ← 原 preproc（特殊符号）
```

说明：

- 每行的 `color-link <token> "<前景色>,<背景色>"` 定义了一个语法元素的颜色。
- 可以只写前景色（如 `color-link diff-added "#00AF00"`），也可以加修饰符（如 `bold`）。
- 改好后保存，用 `:theme` 命令选择你的主题即可生效。
