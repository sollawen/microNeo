
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
