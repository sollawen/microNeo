# D12 — notePane 多 receiver 处理

> **状态**：方案设计（已拍板，待实施）
>
> **范围**：仅 microNeo 发送端（`internal/action/notepane.go` + `internal/action/selectpane.go` + `internal/action/bufpane.go` forwarding）。EABP 协议不改、接收端不改、micro 原生代码不动。
>
> **依赖**：**D13**（SelectPane）——**已实施完成**。本 D12 调用 `TheSelectPane.Open()` 实现多 receiver 选择 UI。SelectPane 自身设计见 [`D13-SelectPane设计.md`](./D13-SelectPane设计.md)。
>
> **前身**：D11 解决"多接收端如何命名"（让"多接收端并存"变正常、变常见）；D12 解决"发送端拿到多个名字后如何让用户挑"。

---

## 一、问题

当前实现（`notepane.go` `NotePaneSend` 内）：

```go
receivers, err := eabp.Discover()
...
if len(receivers) != 1 {
    if len(receivers) == 0 {
        InfoBar.Message("✗ no receiver found")
    } else {
        InfoBar.Message("✗ multiple receivers found, need exactly one")
    }
    return false
}
```

两个问题：

1. **Discover 时机错位**：Discover 在 `NotePaneSend`（用户写完草稿按 Alt-Enter 发送时），不在 `notePaneOpen`（打开 notePane 时）。导致 notePane 边框无法显示"将发到哪"。
2. **多 receiver 整批拒收**：D11 落地后"多接收端并存"**变正常、变常见**，但当前遇到 ≥2 个 receiver 就拒收，与 D11 收益矛盾——用户看到的永远是 `✗ multiple receivers found, need exactly one`。

**需要**：
- Discover **前移到 `notePaneOpen`**（打开时就发现）
- Discover 到 2+ receiver 时，弹 SelectPane 让用户**选一个**，选完再开 notePane
- notePane 边框显示将发到的 receiver 名字（affordance + 防御）

---

## 二、目标（用户视角）

### 2.1 四种场景

| # | 场景 | 期望行为 |
|---|------|----------|
| 1 | Discover 出错 | **不打开 notePane**，InfoBar 提示 `✗ discover error: ...` |
| 2 | 0 receiver | **不打开 notePane**，InfoBar 提示 `✗ no receiver found` |
| 3 | 1 receiver | 打开 notePane，**边框上显示该 receiver 的名字**（一眼看出"我将要发到 Alpha"），Alt-Enter 直接发 |
| 4 | 2+ receiver | **不打开 notePane**，打开 SelectPane 让用户选择一个；关闭 SelectPane，再打开 notePane（边框带选中的名字） |

### 2.2 关键设计点

- **Discover err / 0 / 2+ → 不打开 notePane / 先选**：分流发生在 `notePaneOpen` 入口，而非 `NotePaneSend`。
- **1 receiver → 打开 notePane + 边框带名字**：affordance（让用户安心知道发到哪）+ 防御（万一起错 pi 也能及时 Esc 取消）。
- **2+ receiver → 先选再开**：选完一个再开 notePane，两浮层不共存；用户视角是"先选人 → 再写消息"，状态机清晰。
- **选择 UI 用通用 SelectPane**（D13 已实现）：本 D12 不感知 SelectPane 内部细节，只调用 `TheSelectPane.Open(receiverNames, "Receiver", onSelect)`。
- **SelectPane 收事件靠 BufPane forwarding**（D13 已写、D12 转正）：弹 SelectPane 时 notePane 还没开，焦点在主编辑器 BufPane，事件经 `BufPane.HandleEvent` 顶部 forwarding 进入 SelectPane。

---

## 三、流程图（Alt-Enter 全流程）

```
主编辑器 Alt-Enter  →  notePaneOpen(h)
    │
    ▼
Discover()  (复用现有 eabp.Discover)
    │
    ├── err ───────→ InfoBar "✗ discover error: ..." ─→ 结束（不开 notePane / SelectPane）
    │
    ├── 0 个 ──────→ InfoBar "✗ no receiver found" ──→ 结束（不开）
    │
    ├── 1 个 ──────→ selectedReceiver = receivers[0]
    │                 ─→ open notePane
    │
    └── 2+ 个 ─────→ selectedReceiver.Socket 在 receivers 列表里？
                         │
                         ├── ✅ 是 → 直接 open notePane（复用缓存，跳过选择器）
                         │
                         └── ❌ 否 → TheSelectPane.Open(names, "Receiver", onSelect)
                                       │   （焦点在 BufPane，事件经 forwarding 进 SelectPane）
                                       │
                                       ├── Esc ──→ onSelect(nil) → selectedReceiver 清零
                                       │           → InfoBar 提示 → 结束
                                       │           （走到此分支时缓存已失效，清掉避免脏值）
                                       │
                                       └── Enter → onSelect(&name) → 找到对应 RegFile
                                                    → selectedReceiver = 该 RegFile
                                                    → open notePane
```

**notePane 永远使用 `selectedReceiver` 作为接收者**（`open()` 无参，notePane 内部直接读 `n.selectedReceiver`）。

**关键设计点**：
- **缓存检查只在 `case 2+` 才有意义**：只有 2+ receiver 才需要决定"要不要弹选择器"。`case 1` 没有歧义（只有一个），直接赋值即可，与缓存命中结果一致。
- **日常快路径**（缓存命中）：用户一直跟 Alpha 聊，Alpha 仍在 → 每次 Alt-Enter 跳过选择器，直接进 notePane。
- **选择行为发生在 notePane 打开之前**：选完才开 notePane，notePane 边框上就有名字（统一表现）。

**六个出口**：
1. `Discover err` → InfoBar 提示，结束
2. `0 个` → InfoBar 提示，结束
3. `1 个` → 赋值 + open notePane
4. `2+ 缓存命中` → 直接 open notePane（不弹选择器）
5. `2+ 缓存未命中 + Esc` → selectedReceiver 清零 + InfoBar 提示，结束（决策 14）
6. `2+ 缓存未命中 + Enter` → 赋值 + open notePane

---

## 四、决策项（已拍板）

| # | 决策 | 拍板结果 | 理由 |
|---|------|----------|------|
| 1 | Discover 触发点 | **`notePaneOpen`**（从 `NotePaneSend` 前移） | 打开时就发现，notePane 边框能显示将发到哪；多 receiver 在打开前就弹选择 |
| 2 | Discover err / 0 receiver 是否开 notePane | **不开**，InfoBar 提示后直接结束 | 避免开了又发不出去，浪费用户操作 |
| 3 | 多 receiver 时选择完是否立即开 notePane | **是** | 统一路径：选完即开 notePane，边框带名字 |
| 4 | notePane 边框显示名字的位置 | **上边框左上**（`┌` 之后紧贴），格式 `→ pi-Alpha` | 上边框已有完整 `─` 字符可用；左上更易扫读；`→` 前缀表达"即将发到"的方向感 |
| 5 | 名字样式 | **不反色**，与边框 `─` 同样式（`config.DefStyle`） | 与 SelectPane title 样式统一（D13 有意决策）；视觉更干净 |
| 6 | 名字旁是否附加 cwd / label | **不带**（v1 简化） | cwd 可能很长（项目路径），会顶掉 name 主信息；label 是开放字段，接收端不一定填 |
| 7 | Esc 是否可中途取消选择 | **是** | onSelect(nil) 表达取消，调用方据此不开 notePane + InfoBar 提示 |
| 8 | selectedReceiver 缓存策略 | **进程内缓存**（NotePane 实例字段，microNeo 退出即丢） | 下次 Alt-Enter 时 Discover 命中复用、不命中重选；Discover 自然兜底（Alpha 关了就不会命中），无需磁盘持久化 |
| 9 | SelectPane 与 notePane 的打开顺序 | **SelectPane 先开 → 选完关 → 再开 notePane** | 两浮层不同时存在（按 §三 流程图），避免焦点/渲染冲突；状态机更简单（用户视角：先选人 → 再写消息） |
| 10 | `selectedReceiver` 字段类型 | **值类型 `eabp.RegFile`** | 与 `eabp.Discover()` 返回的 `[]RegFile` 一致，写起来更顺；同时承担"本次使用"和"下次缓存"双重职责（替代原 targetReceiver） |
| 11 | SelectPane 事件接入 | **保留 BufPane forwarding 并转正**（注释从「D13 scaffolding」改为「D12 集成」） | 弹 SelectPane 时 notePane 未开，焦点在 BufPane；forwarding 是 SelectPane 收键的依赖；侵入仅 2 个 if |
| 12 | Alt-I 测试键 + `selectTestOpen` | **删除** | D13 测试入口，生产代码不夹测试入口；D12 后生产入口是 `notePaneOpen` 的多 receiver 分流 |
| 13 | 缓存命中判据 | **Socket 路径**（`RegFile.Socket`） | Socket 唯一；Name 在 D11 下不保证绝对不重名；PID 可能被新进程复用 |
| 14 | 2+ Esc 时 selectedReceiver 处理 | **清零**（`n.selectedReceiver = eabp.RegFile{}`） | 走到 Esc 分支的前提是 selectedReceiver.Socket 已不在 receivers（缓存失效），保留脏值无意义；清零更安全，避免万一漏判命中时用失效值发送 |

### 决策点补充说明

**决策 4-5 渲染细节**（notePane 上边框嵌入 receiver 名字）：

```
┌─→ pi-Alpha────────────────────────────┐
│  (notePane 内容区)                     │
└────────────────────────────────────────┘
```

- `┌` 紧跟 `─` 再接 `→ pi-Alpha` 再续 `─`
- name 部分用 `config.DefStyle`（**不反色**，与边框 `─` 同样式）—— 跟随 D13 SelectPane title 的有意决策
- 名字截断到上边框可用宽度的 1/3（避免"Alpha" 撑满全行，也避免 "pi-WashingtonMonument" 顶出去）
- 截断上限参考 D11 §4.3 的 10 字符规则（name 本身已被接收端截断过，这里二次防御）

**决策 9 状态机**：见 §三 流程图四个出口。

---

## 五、范围之外（明确不做）

### 5.1 协议 / 接收端
- **EABP 协议不改**：选择是发送端 UI 决策，协议不关心
- **接收端不改**：pi / opencode 接收端不需要任何改动

### 5.2 选择 UI（v1 不做）
- **不做跨重启持久化**（写配置文件那种）：进程内缓存（§四 决策 8）已够用；跨重启场景少且增加磁盘 IO
- **不做 fuzzy search**：receiver 数量通常 < 10，名字又是 NATO 词表，不需要模糊匹配
- **不做"默认 receiver"配置文件**：D11 已经让名字够短够清晰，配默认值反而引入"用户忘了改默认"的坑
- **不向接收端推送"我选了谁"**：协议层不关心选择，只关心最终 envelope
- **不做反向"接收端主动询问用户"**：v1 单向

> SelectPane 自身的范围之外见 D13 §六。

### 5.3 未来扩展（v2 候选，本次 D12 不做）
- **notePane 内热键主动打开 SelectPane**：用户已进入 notePane 后，按某热键（待定，候选 Alt-S / Tab / Ctrl-R）主动重新打开 SelectPane 切换 receiver。**本次 D12 不做，仅占位**。用户场景：开了 notePane 后临时想换个 pi 发，不想 Esc 退出重来。

---

## 六、待办（实施 checklist）

```
☑ 决策项拍板（§四）
☑ 前置：D13 完成（SelectPane + forwarding scaffolding，见 D13 §六）

□ D12.1 NotePane 加 selectedReceiver 字段（值类型，决策 8/10）
   - struct 加字段：selectedReceiver eabp.RegFile
   - 「未缓存」判据：`n.selectedReceiver.Socket == ""`（Socket 空串即零值）
   - open() 签名改造：无参，直接读 n.selectedReceiver
   - 同时承担"本次使用"和"下次缓存"双重职责（替代原 targetReceiver）
   - 清零写法：`n.selectedReceiver = eabp.RegFile{}`（值类型不能赋 nil，用零值）

□ D12.2 notePaneOpen 改造（Discover 前移 + 分流，缓存命中检查只在 2+ 分支）
   - Discover 后按 err / 0 / 1 / 2+ 分流
   - err / 0 → InfoBar 提示，return（不开 notePane / SelectPane）
   - 1 个 → n.selectedReceiver = receivers[0] → TheNotePane.open()
     （无需查缓存——只有一个，直接赋值；与缓存命中结果一致）
   - 2+ 个 → 先查缓存命中（决策 13：selectedReceiver.Socket 在 receivers 列表里）
     - 命中 → TheNotePane.open()（复用缓存，跳过选择器）
     - 未命中 → 收集 receiver.Name 列表 → TheSelectPane.Open(names, "Receiver", onSelect)
       - onSelect(nil) → n.selectedReceiver = eabp.RegFile{}（清零，决策 14）→ InfoBar 提示，不开 notePane
       - onSelect(&name) → 从 receivers 找到 name 对应的 RegFile
         → n.selectedReceiver = 该 RegFile → TheNotePane.open()

□ D12.3 NotePaneSend 改造（不再自己 Discover）
   - 删除 Discover + 分流逻辑（已前移到 notePaneOpen）
   - 用 n.selectedReceiver.Socket / .Name 直接发送
   - 空内容拦截保留（在 Discover 之前——其实 Discover 已不在 Send 里，空拦截就是 Send 第一道关）

□ D12.4 notePane Display 边框显示名字（决策 4-5）
   - 上边框嵌入 "→ <name>"，紧贴 ┌
   - name 用 config.DefStyle（不反色）
   - name 截断（参考 D11 §4.3 的 10 字符规则）

□ D12.5 BufPane forwarding 转正（决策 11）
   - bufpane.go HandleEvent 顶部 + Display 末尾的 2 个 if
   - 注释从「D13 test scaffolding」改为「D12 集成：多 receiver 选择」

□ D12.6 删除 Alt-I 测试入口（决策 12）
   - defaults_other.go / defaults_darwin.go：删 "Alt-i": "selectTestOpen"
   - bufpane.go：删 BufKeyActions["selectTestOpen"] 注册
   - selectpane.go：删 selectTestOpen 函数

□ D12.7 测试：err / 0 / 1 / 2+ / 缓存命中 / 缓存失效 六种场景
   - err：临时把 MNAB_REG_DIR 指向不可读目录
   - 0：无 receiver 在跑
   - 1：开 1 个 pi
   - 2+：开 2 个 pi → 弹 SelectPane → Esc / Enter 两路径
   - 缓存命中：先 2+ 选了 Alpha，再 Alt-Enter（Alpha 仍在）→ 不弹 SelectPane 直接开 notePane
   - 缓存失效：先选 Alpha，关掉 Alpha 再 Alt-Enter（剩 Bravo）→ 缓存未命中，走重新分流

□ D12.8 与 D11 名字池端到端联调

□ D12.9（顺手，D13 文档对齐）回填 D13-SelectPane设计.md §3.4 / §3.5
   - title 样式：Reverse → 不反色（对齐代码）
   - 上下键：到边停 → wrap-around（对齐代码）
```

---

## 七、参考

- **D11** — `D11-名字分配方案.md`（名字分配机制，让多接收端"能有名字"成为常态）
- **D13** — `D13-SelectPane设计.md`（SelectPane 选择浮窗设计，本 D12 依赖，**已实施完成**）
- **`说明-EABP.md` §3** — `eabp.Discover()` 行为契约（Discover 是 SelectPane 的数据源）
- **`说明-发送端.md` §3** — 当前发送流程（Discover 要从 `NotePaneSend` 前移到 `notePaneOpen`）
- **`说明-notepane.md`** — notePane 当前实现（边框绘制、buffer 生命周期）
- **`说明-架构设计.md` §九 开放问题 #4** — "多 receiver 选择 UI" 在架构层已挂账
- **`notepane.go` `NotePaneSend`** — 当前 Discover + "必须恰好 1 个"拒绝逻辑（待改造：Discover 前移、分流删除）
- **`notepane.go` `notePaneOpen`** — Discover 前移的目标位置（D12.2 改造起点）
- **`notepane.go` `Display`** — notePane 边框绘制流程（D12.4 改造起点，嵌入 receiver 名字）
- **`bufpane.go` HandleEvent 顶部 / Display 末尾** — BufPane forwarding（D12.5 转正）
- **`selectpane.go`** — SelectPane 实现（D12 调用方，D12.6 删除 `selectTestOpen`）
