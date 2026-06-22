# D17 — 重命名 AIBP 为 AIBP

> **状态**：待实施
>
> **性质**：纯重命名（mechanical rename），不改任何协议语义、报文结构、运行时行为
>
> **依据**：用户决策（见 §〇）
>
> **目标**：执行完毕后，全仓库（文档 + Go 代码 + TS 代码 + 目录 + 配置路径）统一使用 **AIBP**，无 `eabp`/`EABP` 残留，编译通过、E2E 链路（microNeo → pi）行为不变。
>
> **本文档回答**：工程师拿到能照着干——文件/行号精确、阶段可独立验证、风险可回滚。

---

## 〇、决策摘要

| # | 决策点 | 选择 |
|---|--------|------|
| 1 | AIBP 全称 | **AI Bridge Protocol**（A+I = AI，B = Bridge，P = Protocol） |
| 2 | 协议版本字符串 `eabp-1` → `aibp-1` | ✅ 两端常量一起改 |
| 3 | 旧用户配置 `~/.config/aibp/` | **直接删除，不做迁移** |

**全称订正**（顺手修历史不一致）：
- `说明-AIBP.md` 原写 "Editor-Agent Bridge Protocol"
- `aibp-pi/README.md` 原写 "Embedded Agent Bridge Protocol"（与上者矛盾）
- → 统一为 **AI Bridge Protocol**

---

## 一、命名映射总表

| 类别 | OLD | NEW | 影响范围 |
|------|-----|-----|---------|
| 协议名（prose） | `AIBP` | `AIBP` | 全部 .md + 代码注释 |
| 协议全称 | `Editor-Agent Bridge Protocol` / `Embedded Agent Bridge Protocol` | `AI Bridge Protocol` | 2 处权威定义 + 引用 |
| 协议版本字符串 | `"aibp-1"` | `"aibp-1"` | Go 常量 + TS 常量 + 注释 |
| Go 包目录 | `internal/aibp/` | `internal/aibp/` | `git mv` |
| Go 包名 | `package eabp` | `package aibp` | 4 个 .go 文件 |
| Go import 路径 | `internal/eabp` | `internal/aibp` | 3 个引用方 |
| Go 标识符 | `aibp.Discover` / `aibp.RegFile` / `aibp.Envelope` / `aibp.Sender` / `aibp.Position` / `aibp.ContextPayload` / `aibp.Selection` | `aibp.*` | notepane.go + 2 个 cmd |
| 接收端根目录 | `aibp-receivers/` | `aibp-receivers/` | `git mv` |
| 接收端子目录 | `aibp-receivers/aibp-pi/` | `aibp-receivers/aibp-pi/` | `git mv` |
| `package.json` name | `"aibp-pi"` | `"aibp-pi"` | 1 处 |
| TS 日志前缀 | `[aibp-pi]` | `[aibp-pi]` | index.ts 3 处 |
| TS footer key | `setStatus("aibp", ...)` | `setStatus("aibp", ...)` | index.ts 2 处 |
| TS 配置目录 | `~/.config/aibp/` | `~/.config/aibp/` | index.ts 2 处 + D11 文档多处 |
| TS 配置文件 | `aibp-names.json` | `aibp-names.json` | index.ts 2 处 + D11 文档多处 |
| 文档文件名 | `说明-AIBP.md` | `说明-AIBP.md` | `git mv` + 全仓库链接 |

---

## 二、不改清单（避免误伤）⚠️

以下名字**与 AIBP 无关**或**跨端契约**，**一律不动**：

| 名字 | 含义 | 为什么不改 |
|------|------|-----------|
| `MNAB_REG_DIR` | **M**icro**N**eo **A**gent **B**ridge 的缩写（注册表目录调试覆盖） | 与 AIBP 是两个并存的缩写；改了要两端同步，无收益 |
| `microneo-agent-bridge-$UID` | 注册表目录名（MNAB 的展开） | 跨端 socket 发现契约；改了旧 socket 残留全废、两端必须同时升级 |
| `ai-*.json` / `ai-*.sock` | 注册文件/socket 前缀（"AI agent 命名空间"，见 `说明-AIBP §3.2`） | `ai-` 是命名空间标记，不是协议缩写 |
| 名字池内容（`Alpha`/`Bravo`/...） | 用户可配的 receiver 名字 | 与协议名无关 |
| `microNeo` / `microNeo` 字符串 | sender.name | 与协议名无关 |

> **判断法**：替换前自问「这个 `eabp` 是指**协议本身**，还是恰好含这四个字母的别的概念？」只有前者改。

---

## 三、执行阶段

### Phase 1 — 文档（12 个 .md）

> PLAN 模式下可直接执行，无需代码许可。

#### 1.1 重命名文件

```bash
git mv "docs/agent-comm/说明-AIBP.md" "docs/agent-comm/说明-AIBP.md"
```

#### 1.2 文档内容替换

对下列 12 个文件，做**大小写敏感**的三组替换：

| 替换 | 说明 |
|------|------|
| `EABP` → `AIBP` | 协议名（prose、标题、表格） |
| `eabp-1` → `aibp-1` | 协议版本字符串 |
| `eabp-` → `aibp-` | 目录/文件/标识符前缀（`eabp-pi`→`aibp-pi`、`eabp-receivers`→`aibp-receivers`、`eabp-names.json`→`aibp-names.json`、`internal/eabp`→`internal/aibp`） |

文件清单（按出现密度降序，便于估时）：

| 文件 | 出现数 |
|------|--------|
| `docs/agent-comm/说明-发送端.md` | 40 |
| `docs/agent-comm/说明-接收端.md` | 40 |
| `docs/agent-comm/说明-架构设计.md` | 34 |
| `docs/agent-comm/说明-AIBP.md`（原 说明-AIBP） | 23 |
| `docs/agent-comm/D11-名字分配方案.md` | 47 |
| `docs/agent-comm/README.md` | 14 |
| `docs/agent-comm/D12-多receiver选择.md` | 12 |
| `docs/agent-comm/说明-notepane.md` | 8 |
| `docs/agent-comm/D16-notePane-open参数化.md` | 8 |
| `docs/agent-comm/D12-实施计划.md` | 9 |
| `docs/agent-comm/D13-SelectPane设计.md` | 1 |
| `docs/agent-comm/D15-notePane切换receiver.md` | 1 |

#### 1.3 链接订正

所有 `(./说明-EABP.md)` / `(./说明-EABP)` 锚点链接 → `(./说明-AIBP.md)` / `(./说明-AIBP)`。包含上述三组替换会覆盖绝大多数；需额外检查 README 表格里的 `[说明-EABP](./说明-EABP.md)` 显示文本。

#### 1.4 全称订正（2 处权威定义）

| 文件 | OLD | NEW |
|------|-----|-----|
| `说明-AIBP.md` 第 3 行 | `AIBP（**E**ditor-**A**gent **B**ridge **P**rotocol）` | `AIBP（**A**rtificial **I**ntelligence **B**ridge **P**rotocol）` |
| `aibp-receivers/aibp-pi/README.md`（Phase 3 重命名后） | `AIBP (Embedded Agent Bridge Protocol)` | `AIBP (AI Bridge Protocol)` |

> 全称拼写：决策 1 选 "AI Bridge Protocol"。首字母展开写法用 **A**rtificial **I**ntelligence（与 AI 缩写对齐）。

#### 1.5 Phase 1 验证（build gate）

```bash
# 无残留（排除 MNAB_REG_DIR、microneo-agent-bridge、ai- 前缀这些合法项）
grep -rni "eabp" docs/agent-comm/ | grep -v "MNAB_REG_DIR" | grep -v "microneo-agent-bridge"
# 期望：空输出
```

---

### Phase 2 — Go 代码（目录 + 包 + import + 标识符）

> ⚠️ 需要用户许可后执行（PLAN 模式禁止改 .go）。

#### 2.1 重命名目录

```bash
git mv internal/eabp internal/aibp
```

#### 2.2 包名（4 个 .go 文件）

| 文件 | OLD | NEW |
|------|-----|-----|
| `internal/aibp/registry.go:1` | `package eabp` | `package aibp` |
| `internal/aibp/message.go:1` | `package eabp` | `package aibp` |
| `internal/aibp/cmd/discover/main.go` | 无 package 声明变化（`package main`） | — |
| `internal/aibp/cmd/send/main.go` | 同上 | — |

#### 2.3 import 路径（3 个引用方）

| 文件:行 | OLD | NEW |
|---------|-----|-----|
| `internal/action/notepane.go:13` | `"github.com/micro-editor/micro/v2/internal/eabp"` | `.../internal/aibp` |
| `internal/aibp/cmd/discover/main.go:8` | 同上 | 同上 |
| `internal/aibp/cmd/send/main.go:11` | 同上 | 同上 |

#### 2.4 标识符（`eabp.X` → `aibp.X`）

**`internal/action/notepane.go`**（17 处）：

| 行 | OLD | NEW |
|----|-----|-----|
| 32 | `selectedReceiver aibp.RegFile` | `aibp.RegFile` |
| 101 | `// AIBP send` | `// AIBP send` |
| 134 | `receivers, err := aibp.Discover()` | `aibp.Discover()` |
| 204 | `receivers, err := aibp.Discover()` | `aibp.Discover()` |
| 244 | `n.selectedReceiver = aibp.RegFile{}` | `aibp.RegFile{}` |
| 428 | `func (n *NotePane) open(receiver aibp.RegFile)` | `aibp.RegFile` |
| 511 | `// NotePaneSend sends the note content via AIBP to a receiver.` | `via AIBP` |
| 521 | `// 链路，避免空 message 走完整 eabp 链路。` | `aibp 链路` |
| 533 | `// Convert buffer.Loc ... to AIBP Position ...` | `AIBP Position` |
| 534 | `cursorPos := aibp.Position{...}` | `aibp.Position{...}` |
| 537 | `payload := aibp.ContextPayload{...}` | `aibp.ContextPayload{...}` |
| 545 | `payload.Selection = &aibp.Selection{` | `&aibp.Selection{` |
| 546 | `Start: aibp.Position{...}` | `aibp.Position{...}` |
| 547 | `End:   aibp.Position{...}` | `aibp.Position{...}` |
| 560 | `env := aibp.Envelope{` | `aibp.Envelope{` |
| 563 | `Sender:  aibp.Sender{...}` | `aibp.Sender{...}` |

**`internal/aibp/cmd/send/main.go`**（10 处）：`aibp.ContextPayload` / `aibp.Position` / `aibp.Selection` / `aibp.Envelope` / `aibp.Sender` / `aibp.Discover` 全部 `eabp.` → `aibp.`。

**`internal/aibp/cmd/discover/main.go`**（3 处）：`eabp.Discover` → `aibp.Discover`。

#### 2.5 注释里的 `说明-EABP` → `说明-AIBP`

`registry.go` / `message.go` / `cmd/*` 里所有 `说明-EABP §x.x` 注释引用 → `说明-AIBP §x.x`。

#### 2.6 Phase 2 验证（build gate）

```bash
make build-quick          # 必须编译通过
# 调试工具单独编译（make build 不含它们）
go build ./internal/aibp/cmd/discover && go build ./internal/aibp/cmd/send
# 无残留
grep -rni "eabp" internal/ | grep -v "MNAB_REG_DIR" | grep -v "microneo-agent-bridge"
# 期望：空输出
```

---

### Phase 3 — TS 接收端 + 协议字符串 + 配置路径

> ⚠️ 需要用户许可后执行。

#### 3.1 重命名目录

```bash
git mv aibp-receivers aibp-receivers
git mv aibp-receivers/aibp-pi aibp-receivers/aibp-pi
```

#### 3.2 `aibp-receivers/aibp-pi/package.json`

```diff
- "name": "aibp-pi",
+ "name": "aibp-pi",
```

#### 3.3 `aibp-receivers/aibp-pi/index.ts`

| 行 | OLD | NEW | 类别 |
|----|-----|-----|------|
| 7 | `const PROTOCOL = "aibp-1";` | `"aibp-1"` | 协议字符串（决策 2） |
| 35 | `` `[aibp-pi] skip illegal name: ...` `` | `[aibp-pi]` | 日志前缀 |
| 48 | `path.join(configBase, "eabp", "aibp-names.json")` | `"aibp", "aibp-names.json"` | 配置路径（决策 3） |
| 61 | `"⚠ eabp/aibp-names.json 格式错误..."` | `aibp/aibp-names.json` | notify 文案 |
| 137 | `` `[aibp-pi] server runtime error: ...` `` | `[aibp-pi]` | 日志前缀 |
| 148 | `` `[aibp-pi] listen error on ...` `` | `[aibp-pi]` | 日志前缀 |
| 241 | `ctx.ui.setStatus("aibp", ...)` | `"aibp"` | footer key |
| 246 | `ctx.ui.setStatus("aibp", undefined)` | `"aibp"` | footer key |

#### 3.4 `aibp-receivers/aibp-pi/README.md`

- 标题 `# EABP Receiver for pi` → `# AIBP Receiver for pi`
- 副标题 `EABP (Embedded Agent Bridge Protocol)` → `AIBP (AI Bridge Protocol)`（全称订正）
- 协议版本 `` `eabp-1` `` → `` `aibp-1` ``
- 正文中所有 `EABP` → `AIBP`

#### 3.5 删除旧用户配置（决策 3）

**本机（开发者）清理**：

```bash
rm -rf ~/.config/eabp
```

**面向最终用户的文档说明**（写进 `说明-AIBP.md` §3.x 或 `说明-接收端.md` 合适位置）：

> **升级提示**：若曾自定义过名字池，旧路径 `~/.config/aibp/aibp-names.json` 已废弃。新路径为 `~/.config/aibp/aibp-names.json`。旧文件可手动删除；自定义内容需重新填到新路径。

#### 3.6 Phase 3 验证（build gate）

```bash
# TS 类型/语法检查（若有 tsc）
cd aibp-receivers/aibp-pi && npx tsc --noEmit index.ts 2>/dev/null || echo "无 tsc，跳过"
# 无残留
grep -rni "eabp" aibp-receivers/
# 期望：空输出
```

---

### Phase 4 — 端到端验证

#### 4.1 全仓库无残留

```bash
grep -rni "eabp" . --include="*.go" --include="*.ts" --include="*.js" \
  --include="*.json" --include="*.md" --include="*.yaml" --include="*.yml" \
  | grep -v "\.git/" \
  | grep -v "MNAB_REG_DIR" \
  | grep -v "microneo-agent-bridge"
# 期望：空输出
```

> `ai-*.json` / `ai-*.sock` 的 `ai-` 前缀不含 `eabp`，不会误匹配，无需排除。

#### 4.2 编译

```bash
make build-quick                                    # microNeo 主程序
go build ./internal/aibp/cmd/discover               # 调试工具
go build ./internal/aibp/cmd/send                   # 调试工具
```

#### 4.3 运行时 E2E（行为不变性）

```bash
# 1. 启动一个 pi（装载 aibp-pi 扩展）
# 2. 确认 pi footer 显示 "● <name>"（如 ● Alpha）
# 3. 确认 ~/.config/aibp/aibp-names.json 被种子（首次）
# 4. 确认注册表 /run/user/$UID/microneo-agent-bridge-$UID/ai-<name>.json
#    里 "protocol": "aibp-1"
# 5. microNeo 打开任意文件 → Alt-Enter → notePane → 写消息 → Alt-Enter 发送
# 6. pi 收到消息并触发 LLM 对话
```

**验收点**：注册表目录名仍为 `microneo-agent-bridge-$UID`（不改），文件名仍为 `ai-<name>.json`（不改），只有 `protocol` 字段值变成 `aibp-1`。

---

## 四、Commit 策略

### 推荐：两次提交（贴合 PLAN 模式现实约束）

| Commit | 内容 | 时机 |
|--------|------|------|
| `refactor(aibp): rename EABP→AIBP in docs` | Phase 1 全部（12 个 .md） | 立即（PLAN 模式允许） |
| `refactor(aibp): rename EABP→AIBP in code` | Phase 2 + 3 全部（Go + TS + 目录） | 用户许可后 |

> **中间状态权衡**：Commit 1 之后、Commit 2 之前，文档说 `internal/aibp/` 但代码还是 `internal/aibp/`，存在短暂不一致。**约束**：两次提交紧挨着做，中间不插入其他工作。

### 备选：单次提交

若用户先许可代码改动、再一次性执行全部 Phase，可合并为一个 commit：`refactor(aibp): rename EABP→AIBP across docs and code`。一致性更好，但 review 体量大。

---

## 五、风险与回滚

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| Go import 路径漏改 → 编译失败 | 中 | 编译过不去 | Phase 2.6 `make build-quick` 即刻暴露 |
| 标识符 `eabp.X` 漏改 → 编译失败 | 中 | 同上 | 同上 |
| TS 配置路径改了但旧文件残留 → pi 用旧池子 | 低 | 名字池"看起来没变" | 决策 3 明确 `rm -rf ~/.config/eabp` |
| 文档链接 `说明-AIBP` 漏改 → 死链 | 低 | 文档跳转 404 | Phase 1.5 grep 兜底 |
| 误改 `MNAB_REG_DIR` / `microneo-agent-bridge` → 两端对不上 | 低 | socket 发现失败、僵尸注册堆积 | §二 不改清单 + 各 Phase grep 排除项 |
| 协议字符串改了但某一端漏改 → 版本不匹配跳过 | 中 | microNeo 发现不了 pi | Phase 4.3 step 4 验收 `protocol: aibp-1` |
| **用户机器 pi 全局配置 `packages` 引用旧扩展路径**（**已发生**） | 中 | pi reload 后注册失败、footer 无名字、注册表空 | §八 迁移注意事项：手动改 `~/.pi/agent/settings.json` |

**回滚**：

```bash
# 整体回滚到基线
git revert <commit-hash>
# 或
git reset --hard <baseline-tag>
```

建议执行前 `git tag d17-baseline`。

---

## 六、验收清单

执行完毕，逐项确认：

- [ ] `grep -rni eabp` 全仓库（排除 MNAB_REG_DIR / microneo-agent-bridge）无输出
- [ ] `make build-quick` 通过
- [ ] `go build ./internal/aibp/cmd/{discover,send}` 通过
- [ ] pi 启动后 footer 显示 receiver 名字
- [ ] 注册文件 `protocol` 字段 = `"aibp-1"`
- [ ] microNeo Alt-Enter → notePane → 发送 → pi 收到（E2E 通）
- [ ] pi 全局配置 `~/.pi/agent/settings.json` 的 `packages` 路径已更新（见 §八）
- [ ] `~/.config/aibp/` 已删除；`~/.config/aibp/aibp-names.json` 存在
- [ ] `说明-AIBP.md` / `aibp-receivers/aibp-pi/README.md` 全称统一为 "AI Bridge Protocol"
- [ ] 所有文档内部链接到 `说明-AIBP.md` 可跳转

---

## 七、估时

| Phase | 工作量 |
|-------|--------|
| Phase 1（文档） | ~15 分钟（脚本化 sed/edit） |
| Phase 2（Go） | ~10 分钟 |
| Phase 3（TS） | ~5 分钟 |
| Phase 4（验证） | ~10 分钟 |
| **合计** | **~40 分钟** |

---

## 八、迁移注意事项：用户机器配置（**执行时踩坑实录**）

> 本节是 D17 原始规划的盲区——执行时真实踩到，事后补记。未来类似"被外部引用的路径"重命名时必读。

### 8.1 现象

仓库内改动全部完成、编译通过后，**pi reload 后注册失败**：footer 不显示 receiver 名字（如 `● Alpha`），注册表目录为空，`~/.config/aibp/` 未种子。

### 8.2 根因

pi 通过 **`~/.pi/agent/settings.json` 的 `packages` 数组**用相对路径引用扩展源码目录。本次改名后该数组里的路径失效：

```json
"packages": [
  "../../pi-dev/pi-in-zellij",
  "../../pi-dev/pi-to-chrome",
  "../../pi-dev/microNeo/eabp-receivers/pi",      ← 目录已改名（此条更早就失效，见 8.4）
  "../../pi-dev/microNeo/eabp-receivers/eabp-pi"  ← 改名为 aibp-receivers/aibp-pi 后失效
]
```

### 8.3 隐蔽性（**最危险**）

pi 加载扩展失败时**静默跳过，不报错**。死路径可长期潜伏不被发现——本次还顺带揪出一条 commit `4d2fb791`（`eabp-receivers/pi` → `eabp-receivers/eabp-pi`）之后就已失效的旧路径，潜伏至今。

### 8.4 处理

手动编辑 `~/.pi/agent/settings.json`，把 `packages` 里指向旧扩展目录的路径改成新目录名（`aibp-receivers/aibp-pi`），顺手删除早已失效的死路径。改完**完全重启 pi**（reload 不一定重读 settings）。

### 8.5 通用教训

仓库内重命名若涉及"被外部配置引用的路径"，**仅靠仓库内 grep 看不到这些外部引用**，必须单独排查：

- `~/.pi/agent/settings.json` 的 `packages`（本机 pi 通过相对路径引用扩展源码）
- `pi install` 复制的扩展副本（若有）
- 安装文档 / README 里的路径示例与安装命令
- shell 别名 / 脚本 / CI 里的硬编码路径

**判断法**：重命名后若某端"编译通过但运行时行为异常"，第一时间怀疑被外部配置引用的旧路径。
