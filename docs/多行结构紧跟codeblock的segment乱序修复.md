# List / Blockquote / Table 紧跟 Code Block 时的 Segment 乱序修复

> 状态：**计划中**（PLAN 模式产出，未实现）
> 关联文件：`internal/md/detect.go`
> 关联 issue：[#6](https://github.com/sollawen/microNeo/issues/6)
> 关联现象：list 的下一行紧跟 code block 时，渲染会判断错误行的顺序

---

## 1. 问题

### 现象

当 **list / blockquote / table** 的下一行**紧跟一个 fenced code block**（中间无空行，且 fence 带 highlighter 能识别的语言标签）时，`DetectSegments()` 产出的 segment 会同时出现两个错误：

1. **重叠**：多行结构段（list/blockquote/table）的 `BufEndLine` 被扩到 `visibleEnd`，把后面的 code block（甚至更多内容）整个吞掉。
2. **乱序**：该多行结构段本应按 `BufStartLine` 排在 code block 之前，却因为兜底分支在循环末尾执行，被 append 到 code block 段**之后**。

`displayBufferMD()` 按 slice 顺序消费 segment，于是渲染时行顺序被判断错——code block 被 list 覆盖、位置串行。

### 三个触发场景（已逐行 trace）

**Case A：list 紧跟 code block**

````markdown
- item one
- item two
```go
const x = 1
```
````

逐行 buffer（行号 0–4）：`- item one` / `- item two` / ` ```go ` / `const x = 1` / ` ``` `。
highlighter state：行 2–3 为非 nil（进入 fence region），行 4 退出为 nil。

| y | 行 | state 转折 | 行为 |
|---|----|-----------|------|
| 0 | `- item one` | — | stateNormal → isListItem → `state=stateList, startLine=0` |
| 1 | `- item two` | — | stateList → 继续 list |
| 2 | ` ```go ` | nil→非nil | `codeblockStart=2`；`curState!=nil` → **continue，跳过 switch** |
| 3 | `const x=1` | — | `curState!=nil` → continue |
| 4 | ` ``` ` | 非nil→nil | append **CodeBlock[2,4]**，`codeblockStart=-1` |
| — | (循环末尾) | — | `state==stateList` → 兜底 append **List[0, visibleEnd]** |

结果：`CodeBlock[2,4]` 在前，`List[0,≥4]` 在后 → **重叠（都覆盖 2–4）+ 顺序倒挂**。

**Case B：blockquote 紧跟 code block**

````markdown
> quote line 1
> quote line 2
```go
fmt.Println("x")
```
````

结构完全对称 → `CodeBlock[2,4]` 在前，`Blockquote[0,visibleEnd]` 在后，同样的重叠 + 乱序。

**Case C：table 紧跟 code block**

````markdown
| col A | col B |
|-------|-------|
| data1 | data2 |
```go
code
```
````

逐行（行号 0–5）：前 3 行是表格行，行 3 进 fence，行 5 出 fence。
结果：`CodeBlock[3,5]` 在前，`Table[0,5]` 在后，同样错。

### 根因（已定位）

`internal/md/detect.go` 主循环中，**code block 边界处理位于 `switch state` 之前**，且遇到 code block 内部行直接 `continue`：

```go
// detect.go:54-56
if lastState == nil && curState != nil {
    codeblockStart = y // 进入 codeblock
}
...
// detect.go:69-72
// codeblock 内部行：不产生独立 segment，跳过
if curState != nil {
    lastState = curState
    continue   // ← 抢在 switch 之前，list/blockquote/table 的关闭分支永远到不了
}
```

当处于 `stateList`（同理 `stateBlockquote` / `stateTable`）时，遇到 code block 起始 fence：

1. `lastState==nil && curState!=nil` 命中（detect.go:54）→ 记下 `codeblockStart`。
2. 紧接着 `if curState != nil { continue }`（detect.go:69）→ **跳过了 `switch state`**，于是多行结构的「遇到非匹配行即关闭」分支（detect.go:115-121 / 128-134 / 141-147）**永远不被执行**。
3. 多行结构段只能落到循环末尾的「未闭合」兜底分支（detect.go:167-184），`BufEndLine = visibleEnd`，把 code block 整个吞掉。
4. 而 code block 段是在循环过程中（遇闭合 fence 时，detect.go:57-65）append 的，**时间上早于末尾兜底的多行段** → slice 里 CodeBlock 在前、List 在后，顺序倒挂。

一句话：**多行结构在遇到 code block fence 时没有被正确关闭，code block 的早退 `continue` 抢在了关闭逻辑前面。**

---

## 2. 影响面

| 维度 | 说明 |
|---|---|
| 受影响结构 | list / blockquote / table（共用同一段状态机 + 同一处兜底分支） |
| 触发条件 | 上述任一结构的**下一行紧跟** fenced code block，且 fence 能被 highlighter 识别为 region（带语言标签如 ` ```go ` 必然触发；纯 ` ``` ` 视 markdown.yaml 而定） |
| 不触发 | 中间有**空行**分隔（空行使多行结构提前在 switch 里正常关闭）；code block 在前、多行结构在后（无嵌套） |
| 可见后果 | segment 重叠 + 乱序 → `displayBufferMD()` 渲染行顺序错乱，code block 被 list 覆盖或位置串行 |
| 已有 issue | [#6](https://github.com/sollawen/microNeo/issues/6) |

---

## 3. 方案

### 核心决策：进入 code block 时，先关闭未闭合的多行结构

候选方案对比：

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| **A. 进入 codeblock 分支内补关闭逻辑** ✅ | 在 detect.go:54-56 分支里加一个 `switch state`，把未闭合的 list/blockquote/table 在 `y-1` 处关闭 | 改动最小、最局部；闭合 fence 那条路径完全不用动 | — |
| B. 把 codeblock 检测挪到 switch 之后 | 调整代码顺序 | 语义上"统一" | 牵动 `continue`/`lastState`/`reprocess` 的耦合，回归面大，得不偿失 |
| C. 末尾兜底分支做去重/截断 | 检测重叠并裁剪 BufEndLine | 不动主循环 | 治标：slice 顺序仍乱（多行段在 codeblock 段之后），下游依旧可能出错 |

**选 A**：根因就在「进入 codeblock 时没关闭多行结构」，在原地补上关闭逻辑是最直接、最低风险的修法。

### 为什么一个修复点能同时治愈三种结构（render 层无跨段依赖）

三种 renderer（`RenderBlockquote` / `RenderTable` / `RenderList`）**各自独立**，不存在跨 segment 依赖：

- 签名统一只收单个 segment：`func RenderXxx(seg Segment, width int, cfg MDConfig) *RenderedSegment`，不读前后 segment。
- `displayBufferMD()` 主循环（`bufwindow_md.go:862` 的 `for _, seg := range segments`）逐个独立消费，唯一跨段传递的是物理 `vY`（屏幕行位置累加），**不是语义状态**。
- row 绝对化自包含（`bufwindow_md.go:107`：`rendered.Rows[ri].BufLine += seg.BufStartLine`），不依赖邻居 segment。

因此三种结构观察到的渲染错误（code block 被多行结构覆盖、位置串行）**根因 100% 在 detect 层**：只要 detect 产出的 segment 行范围正确、不重叠、有序，三种 renderer 各自就能正确渲染。方案 A 在 `detect.go` 一个点修复，三种结构的渲染错误同时消除，**无需改动任何 `render_*.go`**。

### 为什么关闭点是 `y - 1`

行 `y` 是第一个 `curState != nil` 的行（fence 起始），它属于 code block（`codeblockStart = y`）。所以多行结构应在 `y - 1` 收尾。

边界安全：能进入 `stateList`/`stateBlockquote`/`stateTable` 说明 `state` 是在**之前某次迭代**置上的（`startLine = 之前的某个 y`），而当前 `y` 是 fence 起始行，因此 `startLine <= y-1` 恒成立，不会出现负区间或空区间。

关闭后该多行段在循环中段就被 append，**自然排在后到的 code block 段之前**，slice 顺序回归正确。

---

## 4. 详细改动

### 改动 1：`detect.go:54-56` 进入 codeblock 前关闭多行结构

**位置**：`if lastState == nil && curState != nil` 分支内，`codeblockStart = y` 之后。

**现状**：

```go
// detect.go:54-56
if lastState == nil && curState != nil {
    codeblockStart = y // 进入 codeblock
}
```

**改为**：

```go
if lastState == nil && curState != nil {
    codeblockStart = y // 进入 codeblock

    // ★ 新增：进入 codeblock 前，先关闭未闭合的多行结构（list/blockquote/table）。
    // 否则它们的兜底分支（detect.go:167-184）会把 BufEndLine 扩到 visibleEnd，
    // 吞掉 codeblock 并导致 segment 顺序倒挂（issue #6）。
    switch state {
    case stateBlockquote:
        segments = append(segments, Segment{
            BufStartLine: startLine,
            BufEndLine:   y - 1,
            Render:       RenderBlockquote,
        })
        state = stateNormal
    case stateTable:
        segments = append(segments, Segment{
            BufStartLine: startLine,
            BufEndLine:   y - 1,
            Render:       RenderTable,
        })
        state = stateNormal
    case stateList:
        segments = append(segments, Segment{
            BufStartLine: startLine,
            BufEndLine:   y - 1,
            Render:       RenderList,
        })
        state = stateNormal
    }
}
```

闭合 fence 那条路径（detect.go:57-65）**不动**：退出 code block 时 `state` 已是 `stateNormal`，无需处理；后续行正常走 switch。

> 三段 `case` 重复度较高，但**不抽 helper**：和 detect.go 现有「未闭合」兜底分支（detect.go:168-183）的写法保持一致，统一内联，避免引入签名笨重的辅助函数（`state`、`startLine`、`y`、`segments` 四个上下文变量都要传）。

---

## 5. 验证方式

### 端到端验证（sample.md）

在 `docs/sample.md` 末尾补一段「List/Blockquote/Table + code block 紧邻」样例（三种各一组），`make build-quick` 后打开 sample.md 肉眼对照：

| 场景 | 修复前（bug） | 修复后 |
|---|---|---|
| list 下一行紧跟 ` ```go ` | list 覆盖 codeblock，codeblock 错位/丢失 | list 正常收尾，codeblock 独立成块、顺序正确 |
| blockquote 紧跟 codeblock | 同上 | blockquote 收尾，codeblock 独立 |
| table 紧跟 codeblock | 同上 | table 收尾，codeblock 独立 |
| 中间有空行分隔 | 本就正常 | 仍正常（回归） |

回归：独立 codeblock / 独立 list / 独立 table / 未闭合 codeblock（detect.go:158-164）等既有路径渲染不变。

---

## 6. 风险

### R1：闭合点 `y - 1` 的边界（低风险）

`startLine <= y-1` 恒成立：能进入 `stateList/Blockquote/Table` 说明 `state` 是在**之前某次迭代**置上的（`startLine = 之前的某个 y`），而 `y` 是 fence 起始行，故 `startLine <= y-1`，不会出现负/空区间。空行不参与本次关闭点——多行结构遇空行会**提前关闭**（`stateList/Blockquote/Table` 遇非匹配行即收尾并 `reprocess`），这是终端渲染下有意为之的行为（空行还原作者的分隔意图）；本次修复的关闭点 `y-1` 始终是多行结构的最后一个内容行。

**缓解**：sample.md 验收表「中间有空行分隔」一行覆盖「空行紧贴 fence」情况。

### R2：slice 顺序假设（低风险）

修复后多行段在循环中段 append，codeblock 段在后到，自然有序。`displayBufferMD()` 假设 segment 按 `BufStartLine` 升序、不重叠——本修复正是让产物满足该假设。

**缓解**：sample.md 三种紧邻场景肉眼验证多行段排在 codeblock 段之前、位置正确。

### R3：纯 ` ``` ` fence 是否触发（不在本次范围）

不带语言标签的纯 fence 是否被 highlighter 分配 state，取决于 `markdown.yaml` 是否有匹配的 region。本修复只处理「已被 highlighter 认成 codeblock」的情况，不改变 fence 识别本身。

---

## 7. 验收标准

- [ ] list 紧跟 code block：产出 2 段且 `List[0, fenceStart-1]` + `CodeBlock[fenceStart, ...]`，不重叠、有序。
- [ ] blockquote 紧跟 code block：同上结构。
- [ ] table 紧跟 code block：同上结构。
- [ ] 中间有空行分隔时行为不变（多行结构在空行处提前关闭，code block 独立）。
- [ ] `make build-quick` 通过；sample.md 三种紧邻场景肉眼正确。
- [ ] 原生 micro 代码零改动（所有改动在 `internal/md/` 内）。

---

## 8. 不做的事

- ❌ 不调整 codeblock 检测与 `switch state` 的代码先后顺序（方案 B，回归面过大）。
- ❌ 不在末尾兜底分支做重叠裁剪（方案 C，治标不治本，slice 顺序仍乱）。
- ❌ 不改闭合 fence 路径（detect.go:57-65，退出 code block 时 `state` 已是 normal，无需处理）。
- ❌ 不改 `markdown.yaml` / fence 识别逻辑（纯 ` ``` ` 是否触发 state 由语法文件决定，属另一议题）。
- ❌ 不抽多行结构关闭的公共 helper（与既有兜底分支写法保持一致，内联更易读）。
- ❌ 不写单元测试、不升级 `mockBuffer`、不修 `render_table_test.go:381` 的预先存在编译错误（`makeTableSeparator` 缺参）——本次以 `make build-quick` + sample.md 肉眼验证为准；`make build` 不编译 test 文件，上述 test 问题不阻塞构建。
