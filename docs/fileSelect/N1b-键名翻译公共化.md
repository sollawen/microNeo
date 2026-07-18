# N1b — 键名翻译公共化（KeyName 下沉 config）

把 `tcell.EventKey → 标准键名` 的翻译能力从 `internal/action` 的私有 `keyEvent().Name()` 下沉到 `internal/config`，成为公共函数 `config.KeyName`。让所有包（尤其叶子包 `dialog` / `finder`）能自己闭环处理键盘并跟随用户自定义键位，彻底消除 `keyResolver` 注入机制。

本方案是对 **N1a §7.2 / §7.3** 决策的修正：N1a 当时为「不碰现有代码、不新建包」选择了 ownerPane 注入 `keyResolver`，并否决了共享包。N1b 在新的认知下推翻这个选择——不新建包（放 `config`）、单一逻辑源、注入机制本质上是症状而非设计。详见 §3。

---

## 1. 目标与动机

### 目标

- 在 `config` 包新增公共函数 `KeyName(e *tcell.EventKey) string`，把按键事件翻译成与 `bindings.json` 的 key 一致的标准字符串。
- `action` 包内部的 `keyEvent().Name()` 改为委托 `config`，键名拼接逻辑**只保留一份**。
- `dialog` 包摘掉 `KeyResolver` 类型与注入参数；`finder` 不再需要注入。
- 全程不改任何运行时行为（纯结构重构，键名产出与现状逐字一致）。

### 动机：键位系统被不对称地劈成了两半

micro 的「用户自定义键位」功能由**两个共生部分**组成，缺一不可：

| 部分 | 职责 | 现状位置 | 可见性 |
|---|---|---|---|
| 热键表 `config.Bindings` | 键名 → action 的映射 | `config` 包 | **公共**（任何包可读） |
| 键名翻译 | `tcell.EventKey` → 键名字符串 | `action` 包 `events.go` | **私有**（`keyEvent`/`KeyEvent` 未导出） |

这两部分**谁离了谁都没用**：翻译出的键名不去查表就是废字符串；表没有键名作 key 来查就是废数据。它们必须配齐，才能让「用户按某个键 → 执行某个 action」落地。

问题在于它们被劈在了两个包，而且一个公共一个私有。这导致 `action` 包之外的所有叶子组件，处理键盘时只有两条窘路：

- **硬编码固定键位**（不跟随用户键位）：`finder/session.go`、`dialog/select.go` 直接 `switch ev.Key()`。
- **靠 owner 注入闭包**（跟随用户键位，但代价大）：`dialog/input.go` 的 `InputDialog` 靠 `action` 在调用点现造一个闭包喂进来。

N1b 把翻译这一半也挪到 `config`，让两半团聚在同一个公共包，叶子组件就能自己闭环。

---

## 2. 现状盘点（已搜已查，列出依据）

### 2.1 翻译能力本体

| 符号 | 位置 | 可见性 | 被谁用 |
|---|---|---|---|
| `keyEvent(e)` 构造函数 | `action/events.go:47` | 私有 | `bufpane:520` / `infopane:89` / `termpane:128` / `events:169` / `command_neo:128` |
| `KeyEvent` 类型 | `action/events.go:32` | 导出但字段私有（`code/mod/r/any`） | `keytree.go`、`bindings.go`、各 pane 的 `DoKeyEvent` |
| `KeyEvent.Name()` | `action/events.go:58` | 方法 | 见 §2.3 |
| `metaToAlt(mod)` | `action/events.go:39` | 私有 | `events:50` / `events:177` / `bufpane:530` |

### 2.2 注入机制（要消除的对象）

| 符号 | 位置 | 说明 |
|---|---|---|
| `KeyResolver` 类型 | `dialog/input.go:35` | 仅为弥补叶子包够不着翻译能力而存在 |
| `keyResolver` 字段 | `dialog/input.go` InputDialog | 持有注入的闭包 |
| `Open()` 的 `keyResolver` 参数 | `dialog/input.go` | 每个调用方都要传 |
| 注入闭包构造点 | `action/command_neo.go:126` | `InputTest` 里现造；F4-rename 的 `bufpane.go:282` 计划再造一份**逐字相同**的闭包 |

### 2.3 `action` 内部对 `Name()` 的调用点（委托后自动受益，无需逐个改）

```
bufpane.go:94    config.Bindings["buffer"][k.Name()]   k 是 KeyEvent
termpane.go:32   config.Bindings["terminal"][k.Name()]
infopane.go:25   config.Bindings["command"][k.Name()]
keytree.go:236   buf.WriteString(e.Name())             e 是 Event 接口
events.go:108    buf.WriteString(e.Name())             KeySequenceEvent 序列化
rawpane.go:44    e.Name()                              RawEvent
command_neo.go:128  return keyEvent(k).Name()          ← 注入闭包内，随闭包一起删
```

前三处是 `KeyEvent.Name()` 调用；只要 `Name()` 内部委托 `config`，这三处自动走新逻辑。`keytree`/`events`/`rawpane` 走 `Event` 接口，`KeyEvent.Name()` 改实现后同样自动生效。**这些调用点本身都不用改。**

### 2.4 不在本次范围

- **`KeyEvent` 类型不搬**。它被 `keytree.go`（按键树节点、`PaneKeyAnyAction`、wildcards）、`bindings.go`（从 `bindings.json` 反向构造 `KeyEvent`，用 `keyEvents` map）、各 pane 的 `DoKeyEvent(KeyEvent)` 深度结构化使用。搬它要连 `Event`/`KeySequenceEvent`/`MouseEvent`/`ConstructEvent`/`Handler` 整套事件模型一起搬（重切，代价大）。N1b 只搬「键名翻译」这个纯函数，不碰类型。
- **`bindings.go` 的 `keyEvents` map（string→tcell.Key 反向表）不搬**。它是「键名 → tcell.Key」的反向解析，服务 `bindings.json` 加载，和 `KeyEvent` 构造耦合，留 `action`。
- **SelectDialog / finder 现有的硬编码键位不在本次改**。它们能否改用 `config.KeyName` 是各自的后续工作；N1b 只负责把能力公共化，不强求所有消费方立即切换。

---

## 3. 核心论证：为什么是下沉而不是注入

N1a §7.2 选了注入，§7.3 否决了共享包。N1b 推翻这两点，依据是下面四条在 N1a 之后才逐步看清的事实。

### 3.1 注入是「症状」不是「设计」

`action` 包**根本不消费** `keyResolver`。证据：

- `action` 自己处理键盘（`bufpane`/`infopane`/`termpane`）用的是原生 `keyEvent(e).Name()`，走自家私有函数，**一个 `keyResolver` 都不碰**。
- `action` 里和 `keyResolver` 沾边的只有 `command_neo.go:126` 一个**临时闭包变量**——那是 `InputTest` 创建 `InputDialog` 时，把私有 `keyEvent` 包成 `func(tcell.Event) string` 的壳子喂出去。它不是类型、不是字段、不是 `action` 的固有组成。

所以「给 InputDialog 注入 keyResolver」的真相是：`action` 在调用点**临时把自家私有能力重新包装成一个一次性闭包**喂给叶子包。这个闭包用完即弃，下一个调用点（F4-rename）要再包一遍**逐字相同**的闭包。

`KeyResolver` 这套机制（类型 + 字段 + 参数 + 闭包构造）的存在，纯粹是为了绕过「能力被私藏在 action」这道本不该存在的墙。墙是越位囤积砌出来的，马甲是为了翻墙现折的——两者都是症状。

### 3.2 不对称的撕裂是根因

`config.Bindings` 早就公共化了（`globals.go:7`，`dialog`/`display` 现在就在读它）。但翻译能力还锁在 `action` 私有。这种「一半公共一半私有」导致：叶子包看得见表、够不着查表用的 key 字符串，于是只能硬编码或靠注入。

把翻译也挪到 `config`，两半团聚，叶子包就能 `config.KeyName(e) → config.Bindings[ctx]` 自己闭环，全程不碰 `action`。

### 3.3 回应 N1a §7.3 否决共享包的理由

N1a §7.3 否决「新建 `internal/keyevent` 包」的两条理由：

- 「只有 InputDialog 需要键名解析，新建一个包只为一个组件服务」——**已过时**。F4-rename 让 `finder` 成为第二个消费方；`SelectDialog` 将来想跟用户键位也是第三个。消费方不止一个。
- 「需要改 `action/events.go`，碰核心文件」——**放 `config` 而非新包**就不存在「新建包」的成本；而 `events.go` 的改动是「委托」，不是「搬家」，见 §5.2，风险可控。

N1b 选择放 `config` 而非新建包：`config` 已 import `tcell`（`colorscheme.go:9`），已存 `Bindings`，键名与表天生一家，就近原则，零新包。

### 3.4 下沉后的收益清单

| 消失的东西 | 现在在哪 |
|---|---|
| `KeyResolver` 类型 | `dialog/input.go:35` |
| InputDialog 的 `keyResolver` 字段 + `Open()` 参数 | `dialog/input.go` |
| `InputTest` 的注入闭包 | `action/command_neo.go:126` |
| F4-rename 计划里的 `Session.keyResolver` 字段 + `NewSession` 参数 + owner 闭包 | F4-rename 方案 §5.1/§5.2（尚未实现，直接不做了） |
| `action` 与叶子包之间「翻译能力」的依赖墙 | 结构上消失 |

`action` 包从「能力的房东 + 快递员」退化为「普通消费者」——它 `import config` 调 `config.KeyNameOf`，和叶子包地位平等。

---

## 4. 设计决策

### 4.1 放 `config` 包

`config` 已 import `tcell`，已持有 `Bindings`，是被 `action`/`dialog`/`finder`/`display`/`buffer` 共同依赖的叶子包。放这里零新依赖边、零循环风险（`tcell` 是第三方底层库，无人反向依赖 `config`）。

### 4.2 单一逻辑源

键名拼接逻辑（modifier 折叠、`Ctrl-` 前缀缩写、字符映射）**只在 `config` 保留一份**。`action` 的 `KeyEvent.Name()` 委托 `config`，不复制。

为支持委托，`config` 导出两个层次的函数：

- `KeyName(e *tcell.EventKey) string`——叶子包入口，从原始事件翻译。
- `KeyNameOf(code tcell.Key, mod tcell.ModMask, r rune) string`——底层拼接，供 `action` 的 `KeyEvent.Name()` 用 `k.code/k.mod/k.r` 调用。

`metaToAlt` 一并搬到 `config`（导出为 `MetaToAlt`），消除 `action` 内 3 处调用的重复。

### 4.3 不搬 `KeyEvent` 类型（见 §2.4）

只搬「翻译」纯函数，不碰类型。`action` 的 `keyEvent()` 构造、`KeyEvent` 类型、`keytree`/`bindings` 全部原样保留。

### 4.4 `action` 内部委托，保留 `any` 特殊态

`KeyEvent` 有个 `any` 字段（`keytree` 的 wildcard 用），`Name()` 在 `any==true` 时返回 `"<any>"`。这个特殊态是 `action` 事件模型独有的，`config.KeyNameOf` 不需要知道。所以 `Name()` 保留 `any` 判断，只在非 `any` 时委托：

```go
func (k KeyEvent) Name() string {
	if k.any {
		return "<any>"
	}
	return config.KeyNameOf(k.code, k.mod, k.r)
}
```

---

## 5. 实现步骤

前置：`git status` 干净，便于事后 diff 核对。

### 5.1 `config` 新增 `internal/config/keyname.go`

```go
package config

import (
	"fmt"
	"strings"

	"github.com/micro-editor/tcell/v2"
)

// MetaToAlt 把 Meta 修饰键折叠成 Alt。终端层 Meta 与 Alt 常等价，
// 统一成 Alt 才能和 tcell.KeyNames / bindings.json 的键名约定对齐。
func MetaToAlt(mod tcell.ModMask) tcell.ModMask {
	if mod&tcell.ModMeta != 0 {
		mod &= ^tcell.ModMeta
		mod |= tcell.ModAlt
	}
	return mod
}

// KeyNameOf 把按键三要素翻译成标准键名（与 bindings.json 的 key 一致）。
// mod 会先经 MetaToAlt 规范化，因此传原始或已规范化的 mod 都正确（幂等）。
func KeyNameOf(code tcell.Key, mod tcell.ModMask, r rune) string {
	mod = MetaToAlt(mod)
	m := []string{}
	if mod&tcell.ModShift != 0 {
		m = append(m, "Shift")
	}
	if mod&tcell.ModAlt != 0 {
		m = append(m, "Alt")
	}
	if mod&tcell.ModMeta != 0 {
		m = append(m, "Meta")
	}
	if mod&tcell.ModCtrl != 0 {
		m = append(m, "Ctrl")
	}

	s, ok := tcell.KeyNames[code]
	if !ok {
		if code == tcell.KeyRune {
			s = string(r)
		} else {
			s = fmt.Sprintf("Key[%d]", code)
		}
	}
	if len(m) != 0 {
		if mod&tcell.ModCtrl != 0 && strings.HasPrefix(s, "Ctrl-") {
			s = s[5:]
			if len(s) == 1 {
				s = strings.ToLower(s)
			}
		}
		return fmt.Sprintf("%s-%s", strings.Join(m, "-"), s)
	}
	return s
}

// KeyName 从 tcell 按键事件翻译出标准键名。叶子包处理键盘的统一入口。
func KeyName(e *tcell.EventKey) string {
	code := e.Key()
	r := rune(0)
	if code == tcell.KeyRune {
		r = e.Rune()
	}
	return KeyNameOf(code, e.Modifiers(), r)
}
```

逻辑逐字取自原 `action/events.go` 的 `metaToAlt` + `Name()`（去掉 `any` 分支），保证产出与现状一致。

### 5.2 `action/events.go` 委托 `config`

改动点（4 处）：

1. import 块加 `"github.com/micro-editor/micro/v2/internal/config"`。
2. 删除 `metaToAlt` 函数定义（迁 `config.MetaToAlt`）。
3. `keyEvent(e)` 内 `metaToAlt(...)` → `config.MetaToAlt(...)`。
4. `KeyEvent.Name()` 改为 §4.4 的委托实现（删掉原拼接逻辑）。
5. `ConstructEvent` 里 MouseEvent 的 `metaToAlt(...)` → `config.MetaToAlt(...)`（`events.go:177`）。

改后 `events.go` 的 `Name()` 只剩 `any` 判断 + 一行委托，拼接逻辑全部移交 `config`。

### 5.3 `action/bufpane.go` 的 MouseEvent 构造

`bufpane.go:530` 的 `metaToAlt(e.Modifiers())` → `config.MetaToAlt(e.Modifiers())`。

### 5.4 `dialog/input.go` 摘掉注入

改动点：

1. import 加 `"github.com/micro-editor/micro/v2/internal/config"`。
2. 删除 `type KeyResolver func(event tcell.Event) string`。
3. InputDialog 结构体删 `keyResolver` 字段。
4. `Open()` 签名删 `keyResolver KeyResolver` 参数（及其文档行）。
5. `Open()` 内删 `d.keyResolver = keyResolver`。
6. `handleEvent` 内 `keyName := d.keyResolver(event)` → `keyName := config.KeyName(ev)`（`ev` 是已断言的 `*tcell.EventKey`；类型断言发生在上面的 `if !ok { return }` 分支）。

### 5.5 `action/command_neo.go` 的 InputTest 简化

删除 `keyResolver` 闭包定义（`command_neo.go:125-130`），`dlg.Open(...)` 调用去掉末尾的 `keyResolver,` 实参。

改后 `InputTest` 不再需要 `keyEvent`（那行 `return keyEvent(k).Name()` 随闭包一起消失）。

**效果**：`action` 包内对私有 `keyEvent` 的引用从 5 处降到 4 处（`termpane:128` / `bufpane:520` / `infopane:89` / `events:169` 保留），减少了「私有能力越位囤积」的证据——唯一向外输送的那一处（`command_neo.go` 的注入闭包）已被消除。

### 5.6 编译验证

```bash
make build
```

- 若报循环 import：检查 `config` 是否误引了 `action`（`rg '"internal/action"' internal/config/`）。预期不会，`config` 本就是被 `action` 依赖的叶子包。
- 若报 `metaToAlt` undefined：漏改了某处调用点，`rg 'metaToAlt\b' internal/action/` 找出补成 `config.MetaToAlt`。

---

## 6. 对 F4-rename 的影响（直接受益）

F4-rename 实现方案原本规划了 `finder` 注入 `keyResolver`（§5.1 加字段、§5.2 owner 注入闭包）。N1b 落地后，这部分**全部取消**：

- `finder.Session` 不加 `keyResolver` 字段。
- `finder.NewSession()` 不加参数，保持无参签名。
- `bufpane.go:282` 的 `finder.NewSession()` 调用点不改。
- `startRename()` 里 `dlg.Open(...)` 不传 `keyResolver`。

**建议执行顺序：先 N1b，后 F4-rename（简化版）**。这样 F4-rename 方案可以删掉 §4.1（keyResolver 注入决策）、§5.1（Session 加字段）、§5.2（owner 注入）三节，只保留 rename 业务逻辑本身。

---

## 7. Files to Modify / New Files 速查

**New Files**

- `internal/config/keyname.go` — `MetaToAlt` / `KeyNameOf` / `KeyName`。

**Files to Modify**

| 文件 | 改动 |
|---|---|
| `internal/action/events.go` | 加 config import；删 `metaToAlt`；`keyEvent`/`ConstructEvent` 的 `metaToAlt` → `config.MetaToAlt`；`Name()` 委托 `config.KeyNameOf` |
| `internal/action/bufpane.go` | `:530` MouseEvent 的 `metaToAlt` → `config.MetaToAlt` |
| `internal/action/command_neo.go` | `InputTest` 删 keyResolver 闭包 + `Open()` 去掉末尾实参 |
| `internal/dialog/input.go` | 加 config import；删 `KeyResolver` 类型 + `keyResolver` 字段 + `Open()` 参数；`handleEvent` 改调 `config.KeyName(ev)` |

**不改的文件**

- `internal/action/keytree.go`、`bindings.go`、`infopane.go`、`termpane.go`、`notepane.go`、`rawpane.go`——它们调 `Name()` 走接口/方法，委托后自动受益。
- `internal/finder/session.go`、`internal/dialog/select.go`——现有硬编码键位不在本次改。
- `cmd/micro/micro.go`——主循环不涉及。

---

## 8. 风险与边界

| 风险 | 说明 | 应对 |
|---|---|---|
| 行为不一致 | 委托后键名产出与现状不同 | `KeyNameOf` 逻辑逐字取自原 `Name()`，仅去 `any` 分支；`metaToAlt` 幂等，多次调用无害。手测 §9 逐项验证组合键 |
| `config` 包职责扩张 | 从「纯配置数据」沾上「终端键名翻译」 | `config` 早非纯配置包（`colorscheme.go` 已 import tcell、用 `tcell.Style`）；键名与 `Bindings` 同源，就近合理 |
| 循环依赖 | 理论不会（config 是被 action 依赖的叶子） | `make build` 后 `rg '"internal/action"' internal/config/` 确认 |
| `events.go` 改核心文件 | 改动是「委托」不是「搬家」，`KeyEvent` 类型与 `keyEvent()` 构造原样保留 | diff 核对：`Name()` 只剩 `any` 判断 + 一行 `config.KeyNameOf` 调用 |
| 漏改 `metaToAlt` 调用点 | `action` 内 3 处 | `rg 'metaToAlt\b' internal/action/` 编译期必报 undefined，不会漏过 |

---

## 9. 手测清单

聚焦「键名产出与现状逐字一致」+「注入链路摘除后功能不回归」：

1. **主编辑器键位**：打开文件，`h/j/k/l` 移动、`Ctrl-s` 保存、`Ctrl-q` 退出、`Alt-方向键`（若已绑）等全部正常——验证 `bufpane`/`infopane` 经委托后的键名正确。
2. **Terminal pane**：`:term` 开终端，`Ctrl-q` 退出正常——验证 `termpane`。
3. **用户自定义键位**：在 `bindings.json` 把某动作（如 `CursorLeft`）绑到非常规键，确认主编辑器跟随——验证 `config.Bindings` 查询链路。
4. **组合键名正确**：`Ctrl-X`、`Alt-a`、`Shift-Tab`、`Meta-x`（终端支持时）均被正确识别——验证 modifier 折叠与 `Ctrl-` 前缀缩写。
5. **InputDialog 全功能**（`:inputtest`）：光标移动（`Left/Right/Home/End`、`Ctrl-左右` 跳词）、`Backspace`/`Delete`、词删除（`Ctrl-Backspace`/`Ctrl-Delete` 若绑）、`Tab` 插入、中文/双宽字符、水平滚动、`Enter` 返回、`ESC` 取消——验证摘掉 `keyResolver` 注入后 InputDialog 自读 `config` 闭环正常。
6. **用户键位在 InputDialog 生效**：改 `bindings.json` 把 `CursorLeft` 绑到别的键，InputDialog 内跟随——验证注入摘除未丢失「跟随用户键位」能力（这是当初引入 `keyResolver` 的全部理由，必须确认依然成立）。
7. **ESC 退出验证 canceled 回调**（`:inputtest`）：在 InputDialog 打开时按 `ESC`，浮窗关闭且 `InfoBar` 显示「InputDialog: canceled」，验证 `canceled=true` 正确传递——确认摘掉 `keyResolver` 注入后浮窗关闭链路不受影响。

8. **resize**：InputDialog 打开时缩放终端，浮窗关闭且回调收到 `canceled`。
