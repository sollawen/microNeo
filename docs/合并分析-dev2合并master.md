# 合并分析：dev2 ← master（方案B 版，v2）

> 本文档取代旧文 `docs/合并分析-dev2合并master.md`。旧文基于 **v1.0.5（viewportRowmap）**
> 阶段写就，预言了 2 处代码冲突 + 4 处编译 breakage；但此后 master 演进到 **方案B（screenBuffer
> 单一数据源）**，合并基也随之前移，那些冲突已自然消解。本文是 **2026-06-17 基于当前两分支
> HEAD 的实测结果**，不是推演。
>
> 关联：master 的方案详见 `docs/方案B-实施总结.md`（master 分支）。

---

## 0. TL;DR

| 维度 | 结论 |
|------|------|
| **代码（.go）冲突** | **0 处** ✅ |
| **代码编译** | `go build ./...` **通过**（exit 0）✅ |
| **方案B 测试** | `go test ./internal/display/...` **ok**（master 的新测试全过）✅ |
| **文档冲突（git 标记）** | **2 处**：`docs/MKT-plan.md`（内容）、`docs/plan-viewport-row-map.md`（rename/delete）|
| **API 兼容性** | dev2 的 notePane 调 `LineToScreenRow(line,row)` ↔ master 导出同名同签名函数，**完全对齐** ✅ |
| **合并整体风险** | **低**。解完 2 个文档冲突即可合并，代码无需手工干预 |

**一句话**：代码层零冲突、能编译、测试过；只剩两个文档要手动合一下。

---

## 1. 分支拓扑与合并基

```
   884108c7  refactor(display): export LineToScreenRow and ScreenRowToLine   ← 合并基
       │
       ├──► master：在 exported API 之上重写为方案B（screenBuffer 单源）
       │    （9c2c8203 … 6b3306f5，共 27 个提交）
       │
       └──► dev2：消费 exported API 做 notePane / EABP / hideStatusLine
            （cf61d416 … aabbc43d，共 35 个提交）
```

**关键洞察**：合并基 `884108c7` 恰恰就是"导出 `LineToScreenRow` / `ScreenRowToLine`"
那个提交。也就是说，**两条分支从同一个 API 契约出发各自演进**：

- master 把这俩函数的**内部实现**从 viewportRowmap 升级到 screenBuffer，但**对外签名保持不变**；
- dev2 的 notePane 只消费这俩函数的**对外签名**，不关心内部实现。

→ 这是合并能零代码冲突的根本原因。契约稳定，实现替换不破坏调用方。

---

## 2. 自合并基以来，两边各改了什么

### 2.1 master 改动的 .go 文件（仅 3 个，全在 `internal/display/`）

| 文件 | 性质 |
|------|------|
| `internal/display/bufwindow.go` | 方案B：`viewportRowmap` 字段 → `sb *screenBuffer` + `sink cellSink` + `prevCursorY`；displayBuffer 内多处 `screen.SetContent` → `w.setCell` |
| `internal/display/bufwindow_md.go` | 方案B 全部新逻辑（screenBuffer / coversExtent / relocateVerticalMD / renderSegmentMD …）。**dev2 自合并基起未触碰此文件** → master 版本整体套用 |
| `internal/display/bufwindow_md_test.go` | 方案B 测试覆盖。dev2 同样未触碰 → 整体套用 |

### 2.2 dev2 改动的 .go 文件

| 文件 | 性质 | 是否与 master 冲突 |
|------|------|----|
| `internal/display/bufwindow.go` | 加 `hideStatusLine` 字段 + `SetHideStatusLine()` + statusLine 跳过 | **同文件但不同区域**，3-way 自动合并成功 |
| `internal/action/notepane.go`（新增 554 行） | notePane 浮窗，调 `bw.LineToScreenRow(sloc.Line, sloc.Row)` | 无（master 没动）|
| `internal/action/bufpane.go`、`globals.go` | 各 +1 行（notePane 接入）| 无 |
| `internal/config/settings_md.go` | ±2 行 | 无（master 没动）|
| `cmd/micro/micro.go` | +20 行（notePane/EABP 接入）| 无（master 没动）|
| `internal/eabp/*`、`eabp-receivers/*`、`proto/eabp-sender/*` | 全新文件 | 无（纯增量）|

### 2.3 唯一同改的代码文件：`bufwindow.go`

`git merge-file`（即标准 3-way 合并）结果：**exit 0，无冲突标记**。

- dev2 的改动集中在：struct 字段（line 34）、`SetHideStatusLine` 方法、statusLine 绘制条件、statusLine 早退；
- master 的改动集中在：struct 字段（line 36–48）、`NewBufWindow` 初始化、`LocFromVisual`、displayBuffer 内 `SetContent→setCell`。

两边**改的是同一文件的不同区域**，git 自动拼合。合并后 struct 同时含 `hideStatusLine`（dev2）与
`sb`/`sink`/`prevCursorY`（master），逻辑互不干扰。

---

## 3. 代码层验证（实测）

在 `/tmp` 重建合并树（dev2 全树 + master 的 `bufwindow_md.go` / `bufwindow_md_test.go` + 3-way 合并后的 `bufwindow.go`），不动用户仓库：

```
$ go build ./...            → exit 0   ✅
$ go test ./internal/display/...  → ok 0.535s   ✅（master 方案B 测试全过）
```

### 3.1 API 对齐点（旧文担心的"雷1/雷2/冲突①②"已全部不存在）

| 调用方（dev2 `notepane.go:434`） | 被调方（master `bufwindow_md.go:1122`） |
|------|------|
| `bw.LineToScreenRow(sloc.Line, sloc.Row)` | `func (w *BufWindow) LineToScreenRow(line, row int) (int, bool)` ✅ 同名同签名 |

| 调用方（合并后 `bufwindow.go` LocFromVisual） | 被调方 |
|------|------|
| `w.ScreenRowToLine(svloc.Y - w.Y)` | `func (w *BufWindow) ScreenRowToLine(screenOffset int) (int, bool)` ✅ |

并且 **`MDRender` 在两边的 `internal/`、`cmd/` 里都无任何引用**（master 也已演进过这关），
旧文担心的"`&& w.mdConfig.MDRender` 引用已删字段"已不成立。

### 3.2 `go vet` 的警告与本次合并**无关**

vet 退出码非 0，但逐条核对：

| 警告 | 来源 | 是否合并引入 |
|------|------|----|
| `internal/md/render_table_test.go:381` `makeTableSeparator` 参数数不符 | **dev2 自带**（dev2 的测试文件调 4 参，dev2 自己的函数要 5 参）；master 未改 `internal/md` | ❌ 否，合并前就存在 |
| 大量 `unkeyed fields` / `copies lock` | micro 上游既有风格警告 | ❌ 否，两分支独立存在 |

→ 无一条 vet 警告由合并产生。`internal/md` 的测试签名问题是 dev2 历史遗留，建议另开 issue 修，
但**不阻塞本次合并**（`go build` 与 display 测试都不涉及它）。

---

## 4. 两个文档冲突（git 会标记，需手工解）

### 4.1 `docs/MKT-plan.md` —— 内容冲突

- base 196 行 → dev2 284 行 / master 205 行，**两边都实质性扩写**，3-way 合并产生 **7 处冲突块**。
- 性质：营销计划文档，无技术风险，**手工合议内容**即可（建议以 dev2 版为底，并入 master 的徽章/品牌更新）。
- 解法：`git merge` 后编辑器里逐块取舍。

### 4.2 `docs/plan-viewport-row-map.md` —— rename/delete 冲突

- master **删除**了它（commit `3170a288` 清理过时滚动方案文档）；
- dev2 把它**原样移动**到 `docs/agent-comm/plan-viewport-row-map.md`（内容与 base 逐字节一致，纯 rename）。
- 两种合理解法（择一）：
  - **跟 master 删**：`git rm docs/plan-viewport-row-map.md`（文档已过时，方案B 已取代 viewportRowmap）；
  - **跟 dev2 留**：`git add docs/agent-comm/plan-viewport-row-map.md` 并 `git rm` 旧路径（作为历史参考归档到 agent-comm）。
- 推荐：**跟 dev2 留**（成本为零，agent-comm 本就是归档目录，保留 viewportRowmap 方案的历史脉络对理解方案B 演进有帮助）。

---

## 5. 为什么这次比旧分析干净这么多

旧文（commit `f0f533cd`）预言的 4 处 breakage 全部消失，原因是**两分支自然演进消解了 API 错配**：

1. 旧文写于 master = **v1.0.5 viewportRowmap**、dev2 刚删 MDRender 的时点，那时 `LocFromVisual`/`Relocate` 的条件与 API 确实纠缠；
2. 此后 master 升级到 **方案B**，但**保留并稳定了 `LineToScreenRow` / `ScreenRowToLine` 的对外签名**；
3. 合并基前移到 `884108c7`（正是"导出这俩函数"的提交）——这一步**对齐了契约**；
4. dev2 的 notePane 又恰好只用这俩函数的签名层。

→ 旧文已**整体过时**，可保留作历史记录，但**不要再据它执行合并**。

---

## 6. 执行清单（按顺序）

1. `git checkout dev2 && git merge master`
2. 解 `docs/MKT-plan.md`：逐块取舍内容（以 dev2 版为底，并入 master 的品牌/徽章段落）
3. 解 `docs/plan-viewport-row-map.md`：`git rm docs/plan-viewport-row-map.md` 并 `git add docs/agent-comm/plan-viewport-row-map.md`（推荐留 dev2 的归档版）
4. `git commit` 完成合并
5. `make build-quick` 验证（预期通过）
6. `make build`（走完整 generate 流程，作为最终验证）
7. 手测：MD 文件光标滚动（方案B 8 个 bug 的回归场景）+ notePane 浮窗定位 + 三分屏/EABP

> 步骤 5–7 预计全部通过：步骤 5 已在 /tmp 等价验证过（`go build ./...` + display 测试 ok）。
> 步骤 7 是运行期语义验证——合并本身不会破坏，但要确认 dev2 的 notePane 在方案B 的 sb 语义下
> 浮窗定位仍精确（旧文 §5 论证过：2D 比 1D 更精确，预期更好）。

---

## 附录 A：本次分析的方法（可复现）

```bash
# 合并基
MB=$(git merge-base master dev2)        # = 884108c7

# git 层冲突清单（不动工作区）
git merge-tree --write-tree --name-only master dev2

# 单文件 3-way 合并预演（bufwindow.go）
git show $MB:internal/display/bufwindow.go > /tmp/base.go
git show dev2:internal/display/bufwindow.go > /tmp/dev2.go
git show master:internal/display/bufwindow.go > /tmp/master.go
cp /tmp/dev2.go /tmp/ours.go
git merge-file -p /tmp/ours.go /tmp/base.go /tmp/master.go   # exit 0 = 无冲突

# 编译验证（/tmp 重建合并树）
git archive dev2 | tar -x
git show master:internal/display/bufwindow_md.go       > internal/display/bufwindow_md.go
git show master:internal/display/bufwindow_md_test.go  > internal/display/bufwindow_md_test.go
cp /tmp/ours.go internal/display/bufwindow.go          # 用 3-way 合并结果
go build ./...                                          # exit 0
go test ./internal/display/...                          # ok
```
