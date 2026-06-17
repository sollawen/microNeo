# D10 - notePane 发送优化(空内容拦截 + selection 智能内联)

> 配套关系:
> - **发送端**:`internal/action/notepane.go`(microNeo 侧,两处改动)
> - **接收端**:`eabp-receivers/eabp-pi/index.ts`(pi 扩展侧,一处改动)
>
> 两端可独立实施,但组合起来才完整。

## 目标

三个相关的小优化,合在一起提升 notePane 发送链路的体验与 token 经济性:

1. **空内容拦截**:用户在 notePane 里啥都没写就按 Alt-Enter 发送 → 直接关闭,不发送。用代码强制执行语义。
2. **selection 行数阈值(发送端)**:把 microNeo 原有的 2KB 字节上限改成行数上限,决策更直观。超阈值的 selection 不发 `Text`,只发 `Selection.Start/End`(位置信息)。
3. **selection 智能内联(接收端,方案 B)**:pi 端**只看 `Selection.Text` 是否为空这一个信号**--有 Text 就内联文字 + 自然语言标位置(省一次 LLM 读文件);没 Text 就走 `@path :lineA-lineB` 让 LLM 自己读。**接收端不做行数判断**,行数是发送端决策的结果,已经体现在 Text 是否为空上。

## 背景:为什么需要 D10

### 当前发送链路的问题

**问题 1:空 message 是无意义操作**

用户行为链分析:
- 打开 notePane → 一定有目的(想说点什么)
- 没话可说 → Esc 关闭,不发(D9 已实施)
- 有话 → 写 + Alt-Enter 发
- **打开 pane + 不写 + Alt-Enter 发** → 这个组合没有合理用户意图

现在 `NotePaneSend` 会让空 message 一路走到 eabp.Discover → 序列化 → 发包 → pi 接收 → `pi.sendUserMessage(text)` → 触发一次 LLM 回合。整条链路为空 message 付出全部成本,纯浪费。

**问题 2:2KB 字节上限不直观**

`notepane.go:515-517` 现有保护:

```go
selText := string(bw.Buf.Substr(start, end))
if len(selText) > 2048 {
    selText = ""
}
```

用户在编辑器里看到的是**行数**不是**字节数**。30 行的代码大约 1-3KB,50 行可能 2-5KB--用户无法预判"我选的这段会不会超过 2KB 被清空"。字节阈值对用户是不可见的。

**问题 3:pi 端无视 Selection.Text,永远让 LLM 读文件**

`eabp-pi/index.ts:54-59` 的 `formatText`:

```typescript
function formatText(p: any): string {
  const focus = p.selection
    ? `line${p.selection.start.line}-line${p.selection.end.line}`
    : `${p.cursor.line}`;
  const base = `@${p.path} :line${focus}`;   // ← 永远走 @ 引用格式
  return p.message ? `${base}\n\n${p.message}` : base;
}
```

无论有没有 selection、selection 内容是否已随包发过来,都拼成 `@path :lineX` 让 LLM 读文件。`Selection.Text` 字段(D2 协议里早就定义的)从未被 pi 端使用。

**问题 4:LLM 看到文件引用就一定会去读**

关键认知:`@path :lineN` 是 pi 的标准"文件引用语法"。**只要这个格式出现在消息里,LLM 就会被系统提示触发去 `read` 文件**--哪怕文件内容已经以 `Selection.Text` 形式内联在消息里。

所以"既放文件引用又内联文字"的方案不省 token,反而双倍消耗(read 文件 + 内联文字都在)。**省 token 的唯一办法是:内联时彻底不放 `@` 引用格式。**

## 已敲定的设计

### 决策 1:空内容拦截在发送端做

在 `NotePaneSend` 开头加守卫:
- 用 `strings.TrimSpace(string(n.BufPane.Buf.Bytes())) == ""` 判断(纯空白也算空)
- 空 → `InfoBar.Message("✗ 内容为空,未发送")` + `n.close()` + return
- 非空 → 走原流程

**为什么给 InfoBar 提示而不是静默**:和 Esc 行为区分开--用户按 Alt-Enter 得到"未发送"反馈,知道是"内容为空被拦截",不会误以为系统瞎搞。

**顺带效果**:这个守卫让"无 message 路径"在 notePane 这条链路上基本死掉。pi 端的"无 message 路径"(`setEditorText` 填输入框)不用投入设计成本,保持现状即可。

### 决策 2:行数阈值替代字节阈值

`lowestCursorScreenRow` 里捕获 selText 时(`notepane.go:515-517`),把:

```go
if len(selText) > 2048 {
    selText = ""
}
```

改成(行数判断):

```go
lineSpan := end.Y - start.Y + 1
if lineSpan > MaxSelectionLines {     // 常量,见决策 3
    selText = ""
}
```

**行数怎么算**:直接用已有的 `start` / `end`(buffer.Loc,Y 是行号),`end.Y - start.Y + 1` 即行数(含首尾)。不需要数 `\n`。

**不会出现负数**:`start`/`end` 是 `lowestCursorScreenRow` 里经过 `if start.GreaterThan(end) { start, end = end, start }` 规范化后的结果，`end.Y >= start.Y` 恒成立，`lineSpan >= 1`。

**注意**:即便 selText 被清空,`n.fileSelection`(位置信息)和 `payload.Selection.Start/End` 仍然发--pi 端 fallback 到 `@path :lineA-lineB` 时需要这个。

### 决策 3:行数阈值 = 30(已敲定)

实施时用常量表达,方便日后调整:

```go
// MaxSelectionLines 是 selection 文字内联发送的行数上限。
// 超过此行数,selection.Text 不发送(仅发 Start/End 位置),
// 接收端 fallback 到 @path :lineA-lineB 让 LLM 自己读文件。
const MaxSelectionLines = 30
```

### 决策 4:pi 端方案 B(内联 + 自然语言标位置,不用 @ 语法)

`formatText` 重写:

```typescript
function formatText(p: any): string {
  const sel = p.selection;
  const selText = sel?.text && sel.text.length > 0 ? sel.text : "";

  if (sel && selText) {
    // 有选区且文字未截断:内联文字(用自然语言标位置,不用 @ 语法 → 不触发 LLM 读文件)
    const header = `来自 ${p.path} 第 ${sel.start.line}-${sel.end.line} 行的选中内容:`;
    return p.message ? `${header}\n\n${selText}\n\n${p.message}` : `${header}\n\n${selText}`;
  }

  // 无选区 / 选区文字被截断(超过 MaxSelectionLines):走 @ 引用,让 LLM 自己读
  const focus = sel
    ? `line${sel.start.line}-${sel.end.line}`
    : `${p.cursor.line}`;
  const base = `@${p.path} :line${focus}`;
  return p.message ? `${base}\n\n${p.message}` : base;
}
```

**关键点**:
- 有 `Selection.Text` → 用自然语言标位置("来自 X 第 N-M 行"),**不放 `@path :lineX`** → LLM 不触发文件 read,直接用内联文字
- 无 `Selection.Text`(被截断或无选区)→ fallback 到 `@path :lineX` → LLM 自己读
- 两端决策标准统一是"`Selection.Text` 是否为空":microNeo 决定发不发 Text,pi 决定怎么用 Text

**为什么不用 `@path` + 内联文字组合**:见上面"问题 4"--`@` 语法会强制触发文件 read,内联文字就被无视,反而双倍消耗。

### 决策 5:无 message 路径保持现状

`onMessage` 里"无 message → `setEditorText` 填输入框"那条路径**不动**。

理由:
- 加了决策 1(空内容拦截)后,notePane 这条链路保证 `Message` 非空
- "无 message"路径在 notePane 链路上基本死路
- 不投入设计成本,避免方案蔓延

## 行为变化矩阵

### microNeo 端(NotePaneSend 守卫)

| 场景 | 旧行为 | D10 后 |
|---|---|---|
| notePane 空内容 + Alt-Enter | 发空 message,走完整 eabp 链路,pi 触发 LLM 空回合 | **InfoBar 提示 + 直接关闭,不发送** ✅ |
| notePane 纯空白 + Alt-Enter | 同上(TrimSpace 前非空) | **同上,被 TrimSpace 判空拦截** ✅ |
| notePane 有内容 + Alt-Enter | 正常发送 | 正常发送(不变) |

### microNeo 端(selection 行数阈值)

| selection 行数 | 旧行为(2KB 字节) | D10 后(30 行) |
|---|---|---|
| 30 行约 1KB 代码 | 发 Text | 发 Text ✅ |
| 30 行约 3KB 代码(长行) | **不发** Text(超 2KB) | 发 Text ✅(行数优先)|
| 50 行约 2KB 代码 | 发 Text | **不发** Text(超 30 行) |
| 50 行约 5KB 代码 | 不发 Text | 不发 Text |

**关键差异**:行数阈值对用户可见(编辑器里能数行),字节阈值不可见。

### pi 端(方案 B 内联)

| payload 情况 | 旧 formatText | D10 formatText | LLM 行为 |
|---|---|---|---|
| 无 selection | `@path :lineN\n\nmsg` | `@path :lineN\n\nmsg` | 读文件(不变) |
| selection 短(Text 非空)| `@path :lineA-lineB\n\nmsg` | `来自 path 第 A-B 行...\n\n<text>\n\nmsg` | **直接用内联,不读文件** ✅ |
| selection 长(Text 为空)| `@path :lineA-lineB\n\nmsg` | `@path :lineA-lineB\n\nmsg` | 读文件(不变) |

## 不动的东西

- `NotePaneSend` 函数的发送/eabp 部分(discover → marshal → dial → write)-- 只在开头加守卫
- `Selection` 结构体定义(D2 协议,`internal/eabp/message.go`)-- `Text` 字段早就在
- notePane 的 `Alt-Enter → NotePaneSend` 绑定(D8 / 既有)
- notePane 的 `Esc → NotePaneClose` 绑定(D9)
- pi 端 `onMessage` 的"无 message → setEditorText"路径
- pi 端 socket 监听、协议校验、registry 文件管理

## 改动清单

| 文件 | 改动 | 决策 |
|---|---|---|
| `internal/action/notepane.go` | (1) `NotePaneSend` 开头（Discover 之前）加空内容守卫；(2) `lowestCursorScreenRow`（约 515-517 行）把字节阈值改行数阈值；(3) 加 `const MaxSelectionLines = 30`（**必须 package level**，约 15-25 行附近，Go 不允许函数内声明 const） | 决策 1, 2, 3 |
| `eabp-receivers/eabp-pi/index.ts` | 重写 `formatText`（约 54-60 行）为方案 B（内联 + 自然语言标位置） | 决策 4 |

> **关于 `lowestCursorScreenRow` 的调用时机**：该函数在 `open()` 和 `reposition()`（resize 时）都会被调。阈值判断 (`lineSpan > MaxSelectionLines`) 因此会重复执行。但因为 start/end 是 open() 时捕获的原始位置，不随 resize 变化，重复判断结果一致——**不影响正确性**。列出这个调用上下文只是为了避免后续维护者困惑。

## 验证步骤

1. `make build`(microNeo 侧)
2. `go vet ./...` 无新增 warning
3. pi 扩展重新加载(`pi install` / 重启 pi)
4. 空内容拦截:
   - 打开任意文件 → Alt-Enter 开 notePane → **不输入任何内容** → Alt-Enter → InfoBar 提示 "✗ 内容为空,未发送" + notePane 关闭 ✅
   - 同上但输入几个空格 → 同样被拦截 ✅
   - 输入有内容 → 正常发送 ✅
5. selection 智能内联(microNeo + pi 协同):
   - 选 10 行代码 → Alt-Enter → 输入"讲解一下" → Alt-Enter 发送
   - pi 端收到的消息应为:`来自 .../xxx 第 A-B 行的选中内容:\n\n<10 行代码>\n\n讲解一下`(**无 `@path :lineX`**)→ LLM 不读文件,直接讲解 ✅
   - **关键验证**：选**正好 30 行**代码 → 同样操作 → 确认 pi 收到的消息**仍然没有** `@path :lineX`（30 行未超阈值，Text 仍发）✅
   - 选 50 行代码 → 同样操作
   - pi 端收到的消息应为:`@path :lineA-lineB\n\n讲解一下`(fallback,Text 被截断)→ LLM 读文件 ✅
6. 无 selection(只光标):
   - 光标停在某行 → Alt-Enter → 输入消息 → 发送
   - pi 端:`@path :lineN\n\n<消息>`(不变)→ LLM 读文件 ✅
7. `git diff --stat` 确认只动两个文件

## 风险评估

- **空内容拦截**:可能拦截到"用户故意发空消息"的场景--但如决策 1 分析,此场景无合理用户意图。风险等级:**极低**
- **行数阈值替代字节阈值**:长行场景(单行很长)下,30 行可能远超原 2KB;短行场景下,30 行可能远低于 2KB。但行数对用户可见、可预测,是更好的 UX 权衡。风险等级:**低**
- **方案 B 内联失去全文上下文**:LLM 看不到 selection 周边的代码,回答可能不如"读全文"全面。但这是用户主动选择(选了短片段问问题)的合理结果,且用户可通过"只放光标、不选"走老路径保留读全文能力。**两条路径并存,用户自己选**。风险等级:**低**
- **方案 B 的自然语言标位置被 LLM 误判为文件引用**：`@path :lineX` 是 pi 的特有语法，缺 `@` 前缀的字符串（如“来自 X 第 N-M 行”）不会触发文件 read。风险等级：**低**（需实测验证）
- **风险总评**:**低**--三处改动都是局部、可回退,无既有路径的语义改动

## 实施建议

可拆成两个独立 commit(发送端 + 接收端),也可合并为一个。建议合并:

```
feat(notepane): optimize send path - block empty, inline short selection

三处协同优化 notePane → pi 的发送链路:

1. 空内容拦截(microNeo):NotePaneSend 开头加守卫,
   TrimSpace 后为空则 InfoBar 提示 + 直接关闭,不发送。
   "打开 pane + 不写 + 发送" 无合理用户意图,用代码强制
   执行语义,避免空 message 走完整 eabp 链路触发 LLM 空回合。

2. selection 行数阈值(microNeo):把原 2KB 字节上限改成
   30 行上限。行数对用户可见可预测,字节不可见。常量化
   (MaxSelectionLines=30) 便于日后调整。超阈值 Text 不发,
   但 Selection.Start/End 仍发(供 pi fallback)。

3. selection 智能内联(pi, 方案 B):formatText 重写,
   有非空 Selection.Text 时用自然语言标位置("来自 X 第 N-M
   行的选中内容")+ 内联文字,不用 @ 语法 → 不触发 LLM 读
   文件,省 token。无 Text 时 fallback 到 @path :lineA-lineB。

两端决策标准统一:Selection.Text 是否为空。microNeo 决定
发不发 Text,pi 决定怎么用 Text。

Scope:纯增量优化,不动 D8/D9 已落地的键位绑定。
```

---

## 与其他 D 计划的关系

| 计划 | 关系 |
|---|---|
| D2(通信协议) | `Selection.Text` 字段在 D2 就定义了,D10 是首次让 pi 端真正使用它 |
| D8(Alt-Enter 打开) | 独立,D10 不动打开键 |
| D9(Esc 关闭) | 独立,D10 不动关闭键;D9 的 `notePaneClose` 复用,D10 的空内容守卫调 `n.close()`(同一函数) |
