## 基本规则

- **如非必要，勿增实体**。能复用已有代码和方法的，就不要新写代码的方法
- 编译本项目时，必须使用 `make build`，不要直接使用 `go build`。如果想快速编译跳过 generate 步骤，可以使用 `make build-quick`
- 用户配置目录统一在 `$XDG_CONFIG_HOME/microNeo`（未设置时 fallback 到 `~/.config/microNeo`），启动时可用 `--config <dir>` 临时覆盖
- `docs/`目录是本项目所有设计方案和计划
- **代码注释不引用文档**：注释必须自包含，用散文讲清「为什么」，不依赖任何外部文档即可读懂。**禁止在代码注释里写设计文档的文件名或章节号**
- MD 诊断日志：`make build-dbg` 构建时写到 `/tmp/microNeo_debug.log`（对齐 micro 原生 `util.Debug` 开关）；`make build` / `make build-quick` 默认 OFF，不写日志。日志开关在 `internal/display/bufwindow_md.go` 的 `dbgLog`

---

## 项目结构

```
internal/
├── action/bufpane.go + bufpane_md.go   # 主控 + ⭐ MD 配置/editMode
├── buffer/buffer.go                    # IsMD / MDSegments（86, 93 行）
├── config/settings.go + settings_md.go # 全局 + ⭐ MD 专用设置
├── display/
│   ├── bufwindow.go                    # Display() 入口（932 行），分流 938-940
│   ├── bufwindow_md.go                 # ⭐ displayBufferMD()（700 行起，926 行）
│   └── softwrap.go
└── md/                                 # ⭐ 渲染管线
    ├── md.go / detect.go / config.go   # 数据结构 / 分类器 / 开关
    ├── inline.go / wrap.go             # 行内元素 / 软换行
    └── render_*.go                     # heading/blockquote/list/codeblock/table/hr/normal

runtime/syntax/markdown.yaml  # ⭐ 需完整版（含 codeblock region）
Makefile                      # ⭐ 编译入口
```


---

## 核心概念

- **Segment**：每行 buffer 属于一个 Segment。表格/代码块的 `Segment.Rows` 行数 **>** buffer 行数（frame 装饰行），是 v1.0.5 光标跨段滚动的核心难点。
- **检测/渲染分离**：`buffer 变化 → DetectSegments() → SharedBuffer.MDSegments → displayBufferMD() 读 → renderer 算 → 写屏`。detect 不依赖屏宽；render 按屏宽布局。触发点 `buffer.go:211-212`（NewBuffer）+ `1031-1032`（编辑增量）。
- **阅读/编辑模式**：`BufWindow.editMode`（`bufwindow.go:42`）。`observeEditModeToggle` 观察 ESC/click 切换；主循环里 `editMode && 光标在 seg 内` → 回退原生渲染。

