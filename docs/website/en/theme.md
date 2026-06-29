
## Config Directory

microNeo uses `$XDG_CONFIG_HOME/microNeo/` (default `~/.config/microNeo/`) as its configuration directory.

## Color Schemes (Themes)

microNeo comes with a variety of built-in color schemes, and you can also fully customize your own.

### How to switch color schemes

1. In the editor, press `Ctrl+E` to enter command mode
1. Type `theme` and press `Enter`
1. Select with `Up/Down` keys, then press `Enter`
1. The new color scheme takes effect

### Built-in themes

microNeo currently ships 9 color schemes:

| Theme | Description |
| --- | --- |
| default | Plain 16-color scheme, compatible with any legacy 8/16-color terminal. Quite ugly. |
| dracula-tc | Dracula theme (true color). Deep purple-blue background, vivid colors. |
| gruvbox | Gruvbox retro warm palette (256 colors). |
| monokai | Classic Monokai palette (true color). |
| one-dark | Atom One Dark style (true color). |
| s-dark | microNeo custom dark theme, deep black background. |
| s-light | microNeo custom light theme, warm white background. |
| solarized | Solarized palette (ANSI colors; depends on the terminal's solarized palette). |
| zenburn | Zenburn low-contrast soft palette (256 colors), easier on the eyes for long reading. |

### Custom themes

Your custom theme files go in `~/.config/microNeo/colorschemes/`.

Using the built-in `s-dark` as an example, here is what a theme file looks like:

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

# Markdown-specific
color-link md-header "bold #db7c41,#100F0F"         # ← was special
color-link md-hr "bold #db7c41,#100F0F"             # ← was special
color-link md-blockquote "#62B1FE,#100F0F"          # ← was preproc
color-link md-bold "#d4d4d4,#100F0F"                # ← was constant.string
color-link md-italic "#b5b8b6,#100F0F"              # ← was default
color-link md-strikethrough "#b5b8b6,#100F0F"       # ← was default
color-link md-inline-code "#85b654,#1f1a10"         # ← was keyword
color-link md-list "#96CBFE,#100F0F"                # ← was statement
color-link md-checkbox "#96CBFE,#100F0F"            # ← was statement
color-link md-link "#FF73FD,#100F0F"                # ← was constant
color-link md-image "#D33682,#100F0F"
color-link md-url "#FF73FD,#100F0F"                 # ← was constant
color-link md-codeblock "#b5b8b6,#201F1F"           # ← plain code block text (no language)
color-link md-frame "#505050,#100F0F"               # ← new (decorative border)
color-link md-frame-label "#7090b0,#100F0F"         # ← new (code block language label)
color-link md-misc "#62B1FE,#100F0F"                # ← was preproc (special symbols)
```

Notes:

- Each line `color-link <token> "<foreground>,<background>"` defines the color of a syntax element.
- You can specify only the foreground (e.g. `color-link diff-added "#00AF00"`), and you can add modifiers (e.g. `bold`).
- Save the file, then use the `:theme` command to pick your theme and it takes effect.
