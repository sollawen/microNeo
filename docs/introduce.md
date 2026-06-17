# microNeo 项目概览

> 给 AI 阅读的项目说明。了解全貌后，细节再去读对应文件。

---

## 是什么

对 Micro 编辑器的 Markdown 渲染增强，只服务 `.md` / `.markdown` 文件。

**核心目标**：打开 MD 文件看到完整渲染（标题、表格、代码块、列表等），不是原始 `# ** |`。

**设计原则**：对 Micro 原生代码侵入越小越好。所有 MD 逻辑隔离到 `*_md.go` 文件。

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

**Segment**：每行 buffer 属于一个 Segment。表格/代码块的 `Segment.Rows` 行数 **>** buffer 行数（frame 装饰行），是 v1.0.5 光标跨段滚动的核心难点。

**检测/渲染分离**：`buffer 变化 → DetectSegments() → SharedBuffer.MDSegments → displayBufferMD() 读 → renderer 算 → 写屏`。detect 不依赖屏宽；render 按屏宽布局。触发点 `buffer.go:211-212`（NewBuffer）+ `1031-1032`（编辑增量）。

**阅读/编辑模式**：`BufWindow.editMode`（`bufwindow.go:42`）。`observeEditModeToggle` 观察 ESC/click 切换；主循环里 `editMode && 光标在 seg 内` → 回退原生渲染。

---

## 关键设计决策

- `IsMD` / `MDSegments` 放 `SharedBuffer`（buffer 固有属性）
- `editMode` 放 `BufWindow`（渲染动态状态）
- MD 逻辑全在 `*_md.go`（不改 Micro 原版，零侵入）
- renderer 是普通函数，`Segment.Render` 字段挂引用
- `viewportRowmap` 用 2D `[]SLoc`（v1.0.5 精确光标定位）

---

## 开发状态

✅ Step 0 架构骨架 / Step 1 全部 renderer + inline + wrap / Step 2 模式切换 / Step 3 光标跨段滚动（v1.0.4 viewportRowmap → v1.0.5 2D SLoc）。当前 **v1.0.5**（见 `CHANGELOG.md`）。

---

## 快速上手

```bash
make build          # 完整构建（带 generate）
make build-quick    # 跳过 generate
./microneo yourfile.md
go test ./internal/md/... -v
```

> ⚠️ AGENTS.md 规定必须用 `make build`，不能 `go build`。二进制名 `microneo`，输出在项目根。

---

## 关键文件

| 文件 | 作用 |
|------|------|
| `bufwindow.go:932 / 418` | `Display()` 入口 / 原版 `displayBuffer()`（零改动） |
| `bufwindow_md.go:700` | `displayBufferMD()` MD 主循环 |
| `bufpane_md.go:15, 42` | `initMDConfig` + `observeEditModeToggle` |
| `buffer.go:86, 93` | `IsMD` / `MDSegments` |
| `md/render_table.go` | 表格（849 行，最复杂） |
| `md/render_codeblock.go` | 代码块（边框映射真实行） |

---

## 注意事项

- `bufwindow.go` 改动只 3 行 if/else；`bufpane.go` 改动只 1 行调用
- 装饰行（表格 frame / 代码块边框 / HR）`BufLine=-1`，点击要忽略
- 光标跨段切换需 dry-run 预渲染（见 `光标滚动-方案A-架构设计.md`）
