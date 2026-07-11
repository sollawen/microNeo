# F5 — Size 过小场景的处理

本文档解决 FileSelector 在「窗口 size 过小」时的行为设计。同时顺带完成一项相关改动：三个 selector 入口的 `isQuit` 参数统一为 `true`。

---

## 0. 一句话

把 size 过小拆成两种语义不同的 Reason：`ReasonSize`（打开时就放不下，selector 从未显示）和 `ReasonResize`（已显示、运行中被 resize 打断）。调用方据此分流——`isNoName` 终身不变，死锁靠「打开时过小 → 退出」自然化解。

---

## 1. 要解决的问题

旧实现（F4 §7 第 6 条）只有一种 `ReasonResize`，三种来源混用：

| 来源 | 时机 | 旧 Reason | quit 回调旧处理 |
|---|---|---|---|
| `computeLayout` 预检失败 | 打开时，未显示 | ReasonResize | h.Quit()（破死锁） |
| `TheFloatFrame.Open` 失败 | 打开时，未显示 | ReasonResize | h.Quit()（破死锁） |
| `OnCancel`（FloatFrame 强制关） | 运行中，已显示 | ReasonResize | h.Quit()（破死锁） |

问题：quit 回调对所有 `ReasonResize` 一律 `h.Quit()`，是为了破「窄窗口下 noName pane 退不出去」的死锁。但这导致**运行中 resize 也强制退出**——用户只是不小心缩了窗口，整个 pane/程序就没了，像崩溃（「假崩溃」问题）。

如果简单改成「resize 不退出只关 selector」，死锁又回来：noName pane 按 Ctrl-q → 开不出 selector → 回编辑 → 再按 → 又开不出……无限循环。

---

## 2. 两个核心决策

### 2.1 isNoName 始终不变（恢复 sticky 出身语义）

`isNoName` 不再因 resize 被动态降级。它回到 F4 §2.1 的原始定义——**sticky 出身不变量，赋值一次终身不变**。

更精确的一句话定义：

**isNoName = pane 是否用 quit selector 替代原生 Quit。**
- `true` = noName 出身的 pane：按 Ctrl-q 先弹 quit selector，而非直接原生 Quit
- `false` = file-born 的 pane：按 Ctrl-q 直接走原生 Quit

这个标志**只管 Ctrl-q**，不掺杂 Ctrl-o 的逻辑（Ctrl-o 对所有 pane 一视同仁，见 §5 场景 3/4）。

### 2.2 拆分 ReasonSize / ReasonResize

把旧的 `ReasonResize` 拆成两个，区分「没机会显示」和「显示过但被打断」：

| Reason | 含义 | 触发时机 | 用户感知 |
|---|---|---|---|
| **ReasonSize** | 打开时窗口已过小，selector 从未显示 | `computeLayout` 预检失败 / `TheFloatFrame.Open` 失败 | 按 Ctrl-q/Ctrl-o 只看到 InfoBar 提示，selector 没弹出 |
| **ReasonResize** | selector 已正常显示，运行中被窗口缩小打断 | `OnCancel`（FloatFrame 强制关） | selector 在浏览中消失，回编辑界面 |

---

## 3. 六个场景

三个入口（Ctrl-q / Ctrl-o / birth）× 两种 size 情况（打开时过小 / 运行中 resize）= 六个场景：

| 场景 | 入口 | 触发前提 | size 情况 | Reason | 调用者行为 |
|---|---|---|---|---|---|
| 1 | Ctrl-q | isNoName=true | 打开时过小 | ReasonSize | **h.Quit()（退出）** |
| 2 | Ctrl-q | isNoName=true | 运行中 resize | ReasonResize | no-op（回编辑，取消退出） |
| 3 | Ctrl-o | 任意 pane | 打开时过小 | ReasonSize | no-op |
| 4 | Ctrl-o | 任意 pane | 运行中 resize | ReasonResize | no-op |
| 5 | birth | spawn 新 noName pane | 打开时过小 | ReasonSize | no-op（新 pane 留空 buffer） |
| 6 | birth | spawn 新 noName pane | 运行中 resize | ReasonResize | no-op（回空 buffer） |

注:上表只覆盖 size 过小触发的 `ReasonSize` / `ReasonResize`。用户主动按 Esc / Ctrl-q（`ReasonEsc` / `ReasonQuit`）与 size 无关，行为见 §4 回调分流矩阵。特别澄清 **birth selector 的完整行为**：只有按 Ctrl-q 才退出该空 pane（`ReasonQuit → pane.Quit()`）；按 Esc（`ReasonEsc`）、打开时过小（`ReasonSize`）、运行中 resize（`ReasonResize`）都只关 selector、留在 pane 编辑空 buffer（即 birth 里 `ReasonResize` 等同 `ReasonEsc`）。birth 在 `ReasonQuit` 上与 browse 一致（都退出 pane），其余 Reason 都 no-op，勿把场景 5/6 的 no-op 误读为 birth 全 no-op。

### 3.1 场景 1 的死锁化解

noName pane（isNoName=true）在窄窗口下按 Ctrl-q：

```
Ctrl-q → QuitNeo → isNoName=true → 开 quit selector
      → computeLayout 预检失败 → InfoBar 提示 "pane too narrow"
      → 回调收到 ReasonSize → h.Quit() → 退出（空 buffer 无修改，直接退）
```

只按一次就退出，不循环。语义自洽：Ctrl-q 本就是退出键，selector 只是可选中间层，开不出就执行底层退出意图。若 pane 有未保存内容，原生 `h.Quit()` 会弹存盘提示（actions.go:1927），数据不丢。

### 3.2 场景 2 为什么不退出

用户已经在 selector 里浏览了，窗口缩小是意外。此时强制退出很突兀（假崩溃）。关掉 selector 回编辑，让用户放大窗口后重试。`isNoName` 保持 true，再按 Ctrl-q 还能重新开 selector。

---

## 4. 回调分流矩阵

三个入口的回调，按 `SelectResult` 分流：

| 入口 | Picked | ReasonEsc | ReasonQuit | ReasonSize | ReasonResize |
|---|---|---|---|---|---|
| **quit (Ctrl-q)** | OpenCmd | return（取消退出） | h.Quit() | **h.Quit()** | return（取消退出） |
| **browse (Ctrl-o)** | OpenCmd | return | h.Quit() | return | return |
| **birth (spawn)** | OpenCmd | return | h.Quit() | return | return |

观察：
- **browse 和 birth 的回调完全一致**（可抽公共函数）。
- quit 与两者的唯一差异在 `ReasonSize`：quit 退出（破死锁），其它 no-op。
- `ReasonQuit`（selector 内按 Ctrl-q）三个入口统一 `h.Quit()`——这就是「isQuit 三入口统一 true」的体现。

---

## 5. 代码改动清单

### 5.1 fileselector.go — 新增 ReasonSize + 拆分上报点

**(a) 新增 ReasonSize 枚举**（约 114-116 行）：

```go
const (
    ReasonEsc    CloseReason = iota // 用户按 Esc
    ReasonQuit                       // 用户按 Ctrl-q
    ReasonSize                       // 打开时窗口过小，selector 从未显示（F5）
    ReasonResize                     // 运行中窗口 resize，已显示后被打断（F5）
)
```

**(b) 三个上报点拆分**：

| 位置 | 旧 | 新 | 说明 |
|---|---|---|---|
| `Open` 内 computeLayout 预检失败（约 172 行） | ReasonResize | **ReasonSize** | 打开时放不下 |
| `Open` 内 TheFloatFrame.Open 失败（约 183 行） | ReasonResize | **ReasonSize** | 打开时放不下 |
| `buildSpec` 的 OnCancel（约 310 行） | ReasonResize | **ReasonResize（不变）** | 运行中被打断 |

### 5.2 fileselector.go — isQuit 参数简化

三个入口统一传 `true` 后，`isQuit` 参数变为冗余。建议从 `Open` 签名删除、字段删除、`handleEvent` 里 Ctrl-q 永远触发 `ReasonQuit`：

```go
// 旧：func (fs *FileSelector) Open(pane, startDir, onSelect, isQuit bool)
// 新：func (fs *FileSelector) Open(pane *BufPane, startDir string, onSelect func(SelectResult))
```

`handleEvent`（约 653-655 行）简化为无条件上报：

```go
case tcell.KeyCtrlQ:
    fs.finish(SelectResult{Kind: Closed, Reason: ReasonQuit})
```

**行为变化（birth/browse）**：旧实现 `isQuit=false` 会吞掉 Ctrl-q；删 `isQuit` 后 Ctrl-q 在所有入口都上报 `ReasonQuit`，回调里 `h.Quit()`/`pane.Quit()`。即 **birth 与 browse selector 里按 Ctrl-q 现在会退出 pane**（不再被吞）；Esc 行为不变（三入口都关 selector 留 pane）。

### 5.3 filemanager.go — 两个回调改分流

**OpenBirthSelector 回调**（约 55-63 行）：

```go
NewFileSelector().Open(pane, dir, func(r SelectResult) {
    if r.Kind == Picked {
        if pane.Buf == nil { return }
        pane.OpenCmd([]string{r.Path})
        return
    }
    if r.Reason == ReasonQuit {
        pane.Quit()  // selector 内 Ctrl-q → 退出新 pane
        return
    }
    // ReasonEsc / ReasonSize / ReasonResize → no-op，继续编辑空 buffer
})  // isQuit 参数已删，恒 true
```

**QuitNeo 回调**（约 82-95 行）：

```go
NewFileSelector().Open(h, d, func(r SelectResult) {
    if r.Kind == Picked {
        if h.Buf == nil { return }
        h.OpenCmd([]string{r.Path})
        return
    }
    if r.Reason == ReasonQuit || r.Reason == ReasonSize {
        h.Quit()  // Ctrl-q 或打开时过小 → 退出（后者破死锁）
        return
    }
    // ReasonEsc / ReasonResize → no-op，回编辑（取消退出）
})
```

### 5.4 command_neo.go — openSelector 回调改分流

**openSelector 回调**（约 93-105 行）：

```go
NewFileSelector().Open(h, startDir, func(r SelectResult) {
    if r.Kind == Picked {
        if h.Buf == nil { return }
        h.OpenCmd([]string{r.Path})
        return
    }
    if r.Reason == ReasonQuit {
        h.Quit()  // selector 内 Ctrl-q → 退出 pane
        return
    }
    // ReasonEsc / ReasonSize / ReasonResize → no-op，回编辑
})
```

---

## 6. 关于 Ctrl-o 不看 isNoName

讨论中曾考虑「noName pane 禁用 Ctrl-o」（因 Ctrl-q 已能开 selector，Ctrl-o 功能重复）。结论：**不禁用，保留**。理由：

1. Ctrl-o 是「开文件」的通用心智，禁用反令用户困惑。
2. 保留则 `openSelector` 零判断，代码更简。
3. 两入口语境不同（quit 带「退出」语义，browse 纯开文件），给用户选择。

故 `isNoName` 的定义保持纯粹：**只管 Ctrl-q 是否被 selector 接管**。

---

## 7. 对 F4 的影响（文档同步）

### 7.1 废止 F4 §7 第 6 条

F4 §7 第 6 条旧文：

> **ReasonResize 必须走 h.Quit()**（不能只关 selector）。否则窄窗口下 noName pane 退不出去（死锁）。

本方案后废止。死锁改由「ReasonSize → h.Quit()」化解（场景 1），运行中 resize（ReasonResize）不再强制退出。

### 7.2 F4 §4 表格更新

F4 §4 的三入口 isQuit 表（`false/false/true`）更新为「统一 true」：

| 入口 | isQuit | Ctrl-q 行为 | Esc 行为 | Size 过小 | 运行中 Resize |
|---|---|---|---|---|---|
| birth | true | 关 selector → h.Quit() | 关 selector，继续编辑空 buffer | no-op | no-op |
| browse | true | 关 selector → h.Quit() | 关 selector，回编辑 | no-op | no-op |
| quit | true | 关 selector → h.Quit() | 关 selector，回编辑（取消退出） | **h.Quit()** | no-op |

### 7.3 isNoName 语义不变

F4 §2.1「sticky 出身不变量」**重新生效**（中间曾考虑降级方案，已否决）。isNoName 赋值一次终身不变，不因 resize 改动。

---

## 8. 体验细节（可选优化）

场景 1（Ctrl-q 打开时过小→直接退出）当前依赖 `computeLayout` 的 InfoBar 提示 `pane too narrow for file selector (need N cols)`。用户看到提示后 pane 随即退出。可选增强：把提示改为说明后果，例如：

```
pane too narrow, Ctrl-q fell back to native quit
```

让用户明确「已退出」而非「selector 没开出来」。非阻塞，可后续迭代。

---

## 9. 实施顺序建议

1. fileselector.go：新增 `ReasonSize` 枚举 + 拆分三个上报点（§5.1）。
2. fileselector.go：删 `isQuit` 参数/字段，简化 `handleEvent`（§5.2）。
3. filemanager.go：改 `OpenBirthSelector` / `QuitNeo` 回调（§5.3）。
4. command_neo.go：改 `openSelector` 回调（§5.4）。
5. `make build` 验证编译；手动测试六个场景。**重点验证场景 1**：窄窗口下 noName pane 按 Ctrl-q，确认一次退出、不死锁（F5 的核心目标）。
6. 更新 F4 文档（§7.1 废止第 6 条、§7.2 表格）。
