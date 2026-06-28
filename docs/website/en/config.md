## Config Directory

microNeo uses `$XDG_CONFIG_HOME/microNeo/` (default `~/.config/microNeo/`) as its configuration directory.

## Color Schemes (Themes)

microNeo comes with a variety of built-in color schemes, and you can also fully customize your own.

### How to switch color schemes
```
1. Press Ctrl+E to enter command mode
2. Type theme and press Enter
3. Use Up/Down to select, then press Enter
4. The new color scheme takes effect
```

### Built-in themes

Built-in color schemes can be customized via files (see "Custom themes" below). microNeo currently ships 9 color schemes:

- `default` — Plain 16-color scheme, compatible with any legacy 8/16-color terminal. Quite ugly.
- `dracula-tc` — Dracula theme (true color). Deep purple-blue background, vivid colors.
- `gruvbox` — Gruvbox retro warm palette (256 colors).
- `monokai` — Classic Monokai palette (true color).
- `one-dark` — Atom One Dark style (true color).
- `s-dark` — microNeo custom dark theme, deep black background.
- `s-light` — microNeo custom light theme, warm white background.
- `solarized` — Solarized palette (ANSI colors; depends on the terminal's solarized palette).
- `zenburn` — Zenburn low-contrast soft palette (256 colors), easier on the eyes for long reading.

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

Note: each line `color-link <token> "<foreground>,<background>"` defines the color of a syntax element. You can specify only the foreground (e.g. `color-link diff-added "#00AF00"`), and you can add modifiers (e.g. `bold`). Save the file, then use the `:theme` command to pick your theme and it takes effect.

## Syntax Highlighting

microNeo ships **158** syntax files, covering 100+ languages (Go, Rust, Python, C/C++, JS/TS, HTML, CSS, Shell, …). When you open a file, the language is detected automatically and highlighting is applied.

### Detection

Syntax detection is based on two things:

- **File extension** — the most common case. For example, `main.go` is detected as Go, `app.py` as Python.
- **First-line header** — for scripts without an extension or with an ambiguous extension. For example, a file whose first line is `#!/bin/bash` is detected as shell.

### Custom syntax

If the built-in ones aren't enough (e.g. a new language, a private format, or you want to tweak existing rules), place your custom `.yaml` syntax file in `~/.config/microNeo/syntax/`. It will **override the built-in syntax with the same name**.

Here is the structure of a syntax file (simplified example):

```yaml
filetype: mylang

detect:
    filename: "\\.(mylang|my)$"
    header: "^#!.*mylang"

rules:
    - keyword: "\\b(if|else|while|func)\\b"
    - type: "\\b(int|string|bool)\\b"
    - constant.string: '"[^"]*"'
    - comment: "#.*$"
```

Notes:

- `filetype` — the syntax name (unique identifier).
- `detect` — detection rules. `filename` is a regex matching the extension, `header` is a regex matching the first line; at least one is required.
- `rules` — highlighting rules. Each `- <highlight-group>: "<regex>"` assigns the matched text to a highlight group.
- `highlight-group` is the token name after `color-link` in a theme file (e.g. `keyword`, `type`, `constant.string`, `comment`). So **the color of syntax is ultimately decided by the current theme** — switch the theme to switch the colors.

For more detailed syntax rules (regions, nesting, subgroups, etc.), refer to micro's native `help/colors` documentation.


## settings.json

- **`settings.json`** — If you see garbled text on mouse click (common on Linux), set clipboard mode to `terminal`:

    ```json
    {
      "clipboard": "terminal"
    }
    ```

- **Font** — A Nerd Font or similar powerline-compatible font is recommended. If the status separator `` looks broken, change `status-separator` in `~/.config/microNeo/settings.json` (e.g., to `│`).
