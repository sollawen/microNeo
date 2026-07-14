# 键位可配置（resize / split 键）—— 计划

需求来源：`Z0-切换布局-设计方案.md` 落地后，发现 neo 的功能键（`Alt-=`/`Alt--` 放大缩小、`Ctrl-t` 分屏）都写死在代码里，用户无法用 `bindings.json` 改。本文是修复计划。

---

## 1. 目标

让以下键都能被用户的 `bindings.json` 覆盖，对齐 `Ctrl-o`、`Alt-Enter` 等已有功能键的待遇：

- `Alt-=` / `Alt--` —— GrowPane / ShrinkPane（放大/缩小）
- `Ctrl-t` —— 分屏（neo 的 HSplit wrapper）

不在本计划范围：`Ctrl-q`（核心退出键，写死是有意保护，见 §4.4）。

---

## 2. 现状与根因

**问题**：用户在 `~/.config/microNeo/bindings.json` 改这些键后重启，设置被无视。

**根因在加载顺序**（`cmd/micro/micro.go:434-437`）：

```
434  InitBindings()       // 读 defaults + 读 bindings.json
435  InitNeoBindings()    // 代码 BindKey：Ctrl-q、Alt-=、Alt--   ← 晚于 json，覆盖一切
437  InitNeoCommands()    // HSplit/VSplit/AddTab wrapper 覆盖 + Ctrl-t BindKey   ← 晚于 json
```

`InitNeoBindings` / `InitNeoCommands` 都排在读 json **之后**，每次启动都把这些键钉回 neo 默认，json 的修改被覆盖。

**为什么会这样**：`Z0-切换布局-设计方案.md` 原文是「仿照 `neoHSplitAction` 的写法……绑定放 `InitNeoBindings`，紧跟现有 Ctrl-q 绑定」。实现时照抄了 `Ctrl-q` 的模板，没区分两种键的性质：

- `Ctrl-q` 是**核心退出键**，写死在最后执行是刻意的——防止用户在 json 里误改成别的导致「退不出去」。核心键该钉死。
- `Alt-=` / `Alt--` / `Ctrl-t` 是**功能键**，用户改错也没损失，照搬「钉死」是过度。

**而且这违背了 neo 自己的惯例**：neo 已经把功能键放进 defaults 的先例——

```
defaults_darwin.go / defaults_other.go:
  "Ctrl-o":     "command:file"     // neo 加的，json 可覆盖
  "Alt-Enter":  "NotePaneOpen"     // neo 加的，json 可覆盖
```

---

## 3. 关键约束（决定方案形态）

`command_neo.go:27` 的注释点明了一个坑：**`BindKey` 解析时会当场查一次 `BufKeyActions` 并缓存函数指针，运行时按键用缓存、不再查 map。**

推论：要绑的 action 必须在 `BindKey` 之前就已注册进 `BufKeyActions`，否则缓存到空指针、按键失效。

`BufKeyActions` 是包级 map（`bufpane.go:747`），包初始化时就填好，早于一切 `Init*`。`NotePaneOpen` 就注册在里面（`bufpane.go:877`，值是包内函数 `notePaneOpen`），所以 defaults 能绑它、且 json 能覆盖——这是现成的、可照搬的同构先例。

---

## 4. 设计

三类键的处理难度不同，分开讲。

### 4.1 `Alt-=` / `Alt--`（简单：固定方法，直接前移）

`GrowPane`/`ShrinkPane` 是固定方法（无 wrapper 覆盖），直接照搬 `NotePaneOpen` 模式。

**第一步：action 注册前移到包级 map。** `bufpane.go:877`（`NotePaneOpen` 旁）加：

```go
"NotePaneOpen":              notePaneOpen,
"GrowPane":                  (*BufPane).GrowPane,
"ShrinkPane":                (*BufPane).ShrinkPane,
```

方法本体在 `command_neo.go:242 / 252`，同属 `action` 包，包级 map 字面量跨文件引用同包方法无问题（`NotePaneOpen`→`notePaneOpen` 即跨文件先例）。**方法实现不动，只改注册位置。**

**第二步：键位进 bufdefaults。** `defaults_darwin.go` + `defaults_other.go` 各加：

```go
"Alt-=": "GrowPane",
"Alt--": "ShrinkPane",
```

**第三步：删 `InitNeoBindings` 的硬绑。** `command_neo.go:40-43` 这 4 行全删（2 行注册 + 2 行 BindKey）——注册前移到包级 map 后这里多余。

### 4.2 `Ctrl-t`（复杂：HSplit 是被 wrapper 覆盖的 action 名）

难点：`Ctrl-t` 绑的 `"HSplit"` 这个 action 名，被 neo 运行时覆盖成了 wrapper（`BufKeyActions["HSplit"]=neoHSplitAction`，`command_neo.go:26`）。要让 defaults 绑 `Ctrl-t` 时缓存到 wrapper，必须让 `"HSplit"` 在**包初始化时**就指向 wrapper，而不是等到 `InitNeoCommands`。

**关键事实（已核实，决定方案可行）**：

- `neoHSplitAction`（`command_neo.go:130`）内部直接调 `h.HSplitAction()`（**方法调用，不查 map**），所以把 `"HSplit"` 指向 wrapper 不会递归。
- 全代码库对 `HSplitAction`/`VSplitAction`/`AddTab` 的引用都是**直接方法调用**（`command.go:531 h.HSplitAction()` 等），不经过 `BufKeyActions` map。所以 `"HSplit"` 这个 action 名的 map 值改成 wrapper，**没有别的运行时依赖**会受影响。

**方案**：

1. **包级 map 直接注册 wrapper**：`bufpane.go:859` 把原生 `"HSplit": (*BufPane).HSplitAction` 改成 `"HSplit": (*BufPane).neoHSplitAction`。
2. **defaults 改 Ctrl-t 的值**：`defaults_darwin.go:55` + `defaults_other.go:58` 把 `"Ctrl-t": "AddTab"` 改成 `"Ctrl-t": "HSplit"`（这正是 neo 想要的语义，原本靠 `InitNeoCommands` 的 BindKey 实现，现在前移到 defaults）。
3. **删 `InitNeoCommands` 的两行**（`command_neo.go:26-27`）：`BufKeyActions["HSplit"]=neoHSplitAction`（包级 map 已注册）+ `BindKey("Ctrl-t","HSplit")`（defaults 已绑）。

改完后包初始化时 `BufKeyActions["HSplit"]` 就是 wrapper，defaults 绑 `Ctrl-t:"HSplit"` 缓存到 wrapper ✓，json 可覆盖 ✓。

**为什么不顺手把 `VSplit`/`AddTab` 也前移**：它们在 defaults 里**没有键绑定**（`VSplit`/`HSplit` 走命令，`AddTab` 原绑的 `Ctrl-t` 已被 neo 改成 `HSplit`），不涉及用户键位配置。保留它们在 `InitNeoCommands` 的运行时覆盖即可（注释说「VSplit 的 BufKeyActions 覆盖仅备用」），如非必要勿增实体。若日后给 `VSplit`/`AddTab` 绑默认键，再用同样的前移手法。

### 4.3 改完后的加载顺序

```
包初始化     BufKeyActions: HSplit=neoHSplitAction、GrowPane/ShrinkPane 注册（像 NotePaneOpen）
InitBindings  defaults 绑 Ctrl-t/Alt-=/Alt--  →  json 覆盖（这三个键从此可被用户改）
InitNeoBindings  只剩 Ctrl-q 写死
InitNeoCommands  只剩 commands 覆盖（neoHSplitCmd 等）+ VSplit/AddTab 的备用 wrapper 覆盖
```

`InitBindings` 内部先绑 defaults、后绑 json（`bindings.go:54-86`，后者覆盖前者），所以用户 json 对这三个键的修改从此生效。

### 4.4 为什么 `Ctrl-q` 仍写死

核心退出键。写死在最后执行是刻意的防误改保护——若用户在 json 把 `Ctrl-q` 改成别的，会「退不出去」。保留在 `InitNeoBindings` 钉死。

---

## 5. 涉及文件

| 文件 | 改动 |
|---|---|
| `internal/action/bufpane.go` | 包级 map（747 起）：① `:877` 旁加 `GrowPane`/`ShrinkPane` 注册；② `:859` 把 `"HSplit"` 从 `HSplitAction` 改成 `neoHSplitAction`。 |
| `internal/action/defaults_darwin.go` | `bufdefaults`：加 `"Alt-=": "GrowPane"`、`"Alt--": "ShrinkPane"`；`Ctrl-t` 从 `"AddTab"` 改 `"HSplit"`。 |
| `internal/action/defaults_other.go` | 同上（两份 defaults 必须同步）。 |
| `internal/action/command_neo.go` | `InitNeoBindings` 删 `40-43`（GrowPane/ShrinkPane 注册+BindKey）；`InitNeoCommands` 删 `26-27`（HSplit wrapper 覆盖 + Ctrl-t BindKey）。 |

无新文件、无新抽象。全部复用既有机制（defaults + 包级 map 注册），与 `NotePaneOpen`/`Ctrl-o` 完全同构。

---

## 6. 风险 / 边界

1. **两份 defaults 必须同步**：`defaults_darwin.go` 和 `defaults_other.go` 是两份独立 map，漏改一份会导致该平台下相应键无默认绑定。实施时两份一起改、一起验证。
2. **`"HSplit"` 全局语义变化（时机前移，运行时一致）**：包级 map 把 `"HSplit"` 指向 wrapper 后，`"HSplit"` 从包初始化起就是 `neoHSplitAction`（原本是 `InitNeoCommands` 运行时才覆盖）。两者都在首次按键前完成，**运行时行为无差异**；且全代码库无人通过 map 查 `"HSplit"`（都是直接方法调用），故无副作用。方向上也和 neo 设计一致（neo 本就想要 HSplit 带 birth selector + 2-pane 卡口）。
3. **`neoHSplitAction` 不递归**：它调 `h.HSplitAction()`（原生方法，`actions.go:2031`，未删），不查 map，不会循环。
4. **用户 json 写错 action 名**：如 `{"Ctrl-t": "HSplitt"}`，`BindKey` 解析时查不到、缓存空指针，按键静默失效——micro 原生行为，非本计划引入。
5. **`AddTab`/`VSplit` 不动**：保留在 `InitNeoCommands` 的运行时覆盖，功能不变；它们无默认键，不影响可配置性。
6. **方法实现全不动**：`GrowPane`/`ShrinkPane`（command_neo.go:242/252）、`neoHSplitAction`（:130）、`HSplitAction`（actions.go:2031）实现都不变，本次只搬注册位置 + 键位归属。

---

## 7. 实施顺序（出 PLAN 模式后）

1. `bufpane.go`：包级 map 加 `GrowPane`/`ShrinkPane` 注册；`"HSplit"` 改 `neoHSplitAction` → `make build-quick`。
2. `defaults_darwin.go` + `defaults_other.go`：加 `Alt-=`/`Alt--`；`Ctrl-t` 改 `"HSplit"`。
3. `command_neo.go`：删 `InitNeoBindings` 的 4 行 + `InitNeoCommands` 的 2 行 → `make build-quick`。
4. 验证默认行为（与改动前一致）：
   - `Ctrl-t` 仍是上下分屏 + birth selector。
   - 2 pane 时 `Alt-=`/`Alt--` 三档切换正常；再 `Ctrl-t` 撞 2-pane 卡口 + 提示。
5. 验证 json 可覆盖（核心验收）：
   - `{"Ctrl-t": "AddTab"}` 重启 → `Ctrl-t` 变回开新 tab（证明能改回原生）。
   - `{"Alt-=": "CursorUp"}` 重启 → `Alt-=` 变上移光标。
   - 删掉这些行重启 → 恢复 neo 默认。
6. 验证换键：`{"Alt-+": "GrowPane"}` → `Alt-+` 也能放大（证明 action 名可被任意键引用）。
