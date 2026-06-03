# Step 0 详细设计：架构骨架 + 背景色验证

> 阶段：Step 0（架构验证期）
> 依赖：架构设计V1.md 全部决策、PRD V2 第六章
> 目标：渲染管线全跑通，每个渲染模块用**专属背景色**可视化"我接管了哪些 buffer 行"
> 范围：**不**做实际内容渲染，只做架构验证
> 状态：待用户审阅

---

## 〇、Step 0 的本质

架构设计V1 §6.5 已经定义了 Step 0 的目标，本详细设计把每一行落到代码上。

**Step 0 = 渲染片机制**端到端跑通 + **每个渲染模块声明领地**（用背景色），交付物是一张能看出"谁负责渲染哪几行"的可视化地图。

如果这张地图对，Step 1~3 就是按地图逐格填色；如果不对，架构层面的问题在这个阶段就能发现，避免到 Step 2/3 才返工。

---

## 一、目录结构与文件骨架

### 1.1 新建文件清单

```
internal/
├── display/
│   ├── bufwindow.go          (改造: + IsMD 字段、+ displayBufferMD、+ Detect 缓存、Display() 加 if/else)
│   ├── window.go             (不变)
│   ├── softwrap.go           (不变，Step 0 不动 Scroll/Diff)
│   ├── statusline.go         (不变)
│   ├── tabwindow.go          (不变)
│   ├── termwindow.go         (不变)
│   ├── infowindow.go         (不变)
│   └── uiwindow.go           (不变)
│
└── md/                       ★ 新建包,与 display 平级,所有 MD 渲染相关代码
    ├── md.go                 (公共类型:Segment 结构(含 Render 函数字段)、Cell、RenderedSegment、MDConfig)
    ├── detect.go             (集中式扫描器:DetectSegments, 读行判断类型,给每个 Segment 挂渲染函数)
    ├── inline.go             (行内渲染器:renderInline() 粗体/斜体骨架,小写开头,内部工具)
    ├── config.go             (MDConfig 结构、NewMDConfig、isMarkdownFile、defer recover 降级)
    ├── render_table.go       (RenderTable(lines, width, cfg) → *RenderedSegment, Step 0 只输出背景色)
    ├── render_codeblock.go   (RenderCodeBlock(lines, width, cfg) → *RenderedSegment)
    ├── render_heading.go     (RenderHeading(lines, width, cfg) → *RenderedSegment)
    ├── render_list.go        (RenderList(lines, width, cfg) → *RenderedSegment)
    ├── render_blockquote.go  (RenderBlockquote(lines, width, cfg) → *RenderedSegment)
    ├── render_hr.go          (RenderHR(lines, width, cfg) → *RenderedSegment)
    ├── render_paragraph.go   (RenderParagraph(lines, width, cfg) → *RenderedSegment)
    ├── detect_test.go        (检测步骤的单元测试)
    └── render_table_test.go  (RenderTable 的单元测试,docs/markdown_table_test.go 不迁入)
```

### 1.2 文件依赖关系

```
依赖方向：action → display → md → buffer
                                    ↘ util

bufwindow.go (display 包)
   └── internal/md/config.go    (mdConfig 字段)
   └── internal/md/md.go        (Segment 结构、Cell、RenderedSegment)
   └── internal/md/detect.go    (DetectSegments, 返回 []Segment)

internal/md/detect.go           (集中扫描器, 不调任何 renderer 的函数, 直接引用函数名挂到 Segment 上)
   └── internal/md/md.go
   └── internal/md/render_table.go     (挂 RenderTable)
   └── internal/md/render_codeblock.go (挂 RenderCodeBlock)
   └── internal/md/render_heading.go   (挂 RenderHeading)
   └── internal/md/render_list.go      (挂 RenderList)
   └── internal/md/render_hr.go        (挂 RenderHR)
   └── internal/md/render_blockquote.go(挂 RenderBlockquote)
   └── internal/md/render_paragraph.go (挂 RenderParagraph)

internal/md/render_table.go          (纯函数, 入参 lines, 返回 *RenderedSegment)
   └── internal/md/md.go
   └── internal/md/config.go
   └── internal/md/inline.go    (调 renderInline 内部工具)

internal/md/render_paragraph.go      (纯函数)
   └── internal/md/md.go
   └── internal/md/config.go
   └── internal/md/inline.go
```

**关键约束**:
- `md` 包**不** import `internal/display`、`internal/config`、`internal/action`，只 import 标准库 + tcell + runewidth + `internal/buffer` + `internal/util`
- `md` 包通过纯计算输入输出与 `display` 解耦：接收 buffer 引用 + 参数，返回渲染结果，不碰 tcell screen
- 配置通过 `MDConfig` 结构（值类型）传入，而不是直接读 `config.GetGlobalOption` / `Buf.Settings`
- 依赖方向严格单向：`action → display → md → buffer`，不存在反向依赖
- `internal/md/` 作为独立包与 `internal/display/` 平级，与上游 Micro 的 `display` 目录改动隔离，合并冲突风险最低
- 所有 renderer 都是**普通函数**（不是接口方法），入参 `lines []string, width int, cfg MDConfig`，返回 `*RenderedSegment`
- `inline.go` 里的 `renderInline()` 小写开头，是内部工具函数，只被 block/line render 函数调用，不对外暴露

### 1.3 docs/markdown_table.go 暂不迁入

`docs/markdown_table.go` / `docs/markdown_table_test.go` **Step 0 保持原状**,不迁入 `internal/md/`。

**理由**:
- Step 0 的表格 renderer 只做"识别表格起点 + 填背景色",**不需要** `DetectTables` 里的完整列宽计算、跨表格追踪、代码块避让等复杂逻辑
- `docs/markdown_table.go` 迁入是与 `internal/md/` 产生代码集成的重动作,Step 0 阶段避免重动作
- 迁入的合适时机是 **Step 2**(表格实际渲染时)。Step 2 需要列宽计算时,一次性迁入 + 改造为 `RenderTable()` 纯函数
- Step 0 期间该文件仍是独立的 `display` 测试包,不链接进主二进制(原状)

**Step 0 的表格检测仅用 ~30 行简单逻辑**:
- 扫描可见区域,识别以 `|` 开头或包含 `|` 的行
- 从第一行 `|` 开始,往下找到连续多行的表块(中间不能有连续空行)
- 实际不区分表头/分隔/数据行,只粗略返回 [BufStart, BufEnd]

完整迁入留 Step 2,详见 §十二.3。

### 1.4 与 SoftWrap 接口的关系

Step 0 **不**动 `softwrap.go`。`displayBufferMD()` 在检测阶段输出"行高",但 Scroll/Diff 仍按 `softwrap` 行为计算。**首帧缓存未命中**(Step 0 的 BufPane 刚创建)按 buffer 行 == screen 行退化为原生行为,第二帧起使用真实检测结果——这个退化方案在 Step 0 早期不存在问题(Scroll 几乎不会被调用),Step 3 滚动优化再做。

详细策略:在 §6.1 详述。

---

预估行数见下文「Step 0 全部新增/改动文件行数预估」表格。

**Step 0 全部新增/改动文件行数预估**(全量):

| 文件 | 类型 | 预估行数 |
|------|------|----------|
| `internal/display/bufwindow.go` | 改 | +250 |
| `internal/action/bufpane.go` | 改 | +30 |
| `internal/config/settings.go` | 改 | +15 |
| `internal/md/md.go` | 新 | +100 |
| `internal/md/detect.go` | 新 | +200 |
| `internal/md/inline.go` | 新 | +50 |
| `internal/md/config.go` | 新 | +100 |
| `internal/md/render_table.go` | 新 | +100 |
| `internal/md/render_codeblock.go` | 新 | +80 |
| `internal/md/render_heading.go` | 新 | +50 |
| `internal/md/render_list.go` | 新 | +80 |
| `internal/md/render_blockquote.go` | 新 | +60 |
| `internal/md/render_hr.go` | 新 | +40 |
| `internal/md/render_paragraph.go` | 新 | +40 |
| `internal/md/detect_test.go` | 新 | +400 |
| `internal/md/render_table_test.go` | 新 | +80 |
| `docs/Step0-测试样本.md` | 新 | +40 |
| `docs/Step0-完成报告.md` | 新 | +50 |
| **新增合计** | | **+1900** |
| **删除合计** | | **−0** |
| **净变化** | | **+1900** |

> **行数估算依据**:
> - 每个 renderer 就是导出一个纯函数(如 `RenderTable`)，平均 40-100 行，比接口+struct 方案更轻量
> - 没有接口定义、没有 SegmentType 枚举、没有 Detect 方法，减少了约 200 行模板代码
> - `detect.go` 集中处理所有类型的识别，约 200 行(含状态机逻辑)
> - 测试代码量与生产代码量比约 1:1(测试覆盖率目标 ≥80%)
> - **docs/markdown_table.go 迁入推迟到 Step 2**(Step 0 表格 renderer 只输出背景色)
> - 删除了 `link.go`，因为链接是 inline render 范畴，不是独立的 block/line render
