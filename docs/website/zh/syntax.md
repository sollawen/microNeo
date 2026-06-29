
## 语法高亮

microNeo 内置 **158 个**语法文件，覆盖 100+ 种语言（Go、Rust、Python、C/C++、JS/TS、HTML、CSS、Shell ……都有）。打开文件时会自动识别语言并应用高亮。

### 识别方式

语法识别基于两点：

- **文件扩展名** — 最常见。例如 `main.go` 识别为 Go，`app.py` 识别为 Python。
- **首行 header** — 无扩展名或扩展名不明确的脚本。例如首行是 `#!/bin/bash` 的文件会识别为 shell。

### 自定义语法

如果内置的不够用（比如新语言、私有格式、或想微调现有规则），把自定义的 `.yaml` 语法文件放在 `~/.config/microNeo/syntax/` 下，会**覆盖同名的内置语法**。

一个语法文件的结构（以简化版为例）：

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

说明：

- `filetype` — 语法名称（唯一标识）。
- `detect` — 识别规则。`filename` 是匹配扩展名的正则，`header` 是匹配首行的正则，两者至少填一个。
- `rules` — 高亮规则。每条 `- <highlight-group>: "<正则>"` 把匹配到的文本归到某个高亮组。
- `highlight-group` 就是 theme 文件里 `color-link` 后面的 token 名（如 `keyword`、`type`、`constant.string`、`comment`）。所以**语法的颜色最终由当前 theme 决定**，换 theme 即换色。

更详细的语法规则（region、嵌套、子组等）可参考 micro 原生的 `help/colors` 文档。




