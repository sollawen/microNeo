# 键位可配置（放大/缩小）—— 计划

需求来源：`Z0-切换布局-设计方案.md` 落地后，发现 `Alt-=`/`Alt--`（放大/缩小）写死在代码里，用户无法用 `bindings.json` 改这两个键。本文是修复计划。

---

## 1. 目标

让 `GrowPane`（放大）/ `ShrinkPane`（缩小）这两个 action 的默认键（`Alt-=` / `Alt--`）能被用户的 `bindings.json` 覆盖——和 `Ctrl-o`、`Alt-Enter` 等已有功能键的待遇一致。

不在本计划范围：`Ctrl-q`（核心退出键，写死是有意保护，见 §4）、`Ctrl-t`（依赖 wrapper 覆盖顺序，见 §4）。

---

## 2. 现状与根因

**问题**：用户在 `~/.config/microNeo/bindings.json` 写 `{"Alt-=": "CursorUp"}` 后重启，`Alt-=` 仍是放大，用户设置被无视。

**根因在加载顺序**（`cmd/micro/micro.go:434-437`）：

```
434  InitBindings()       // 读 defaults + 读 bindings.json
435  InitNeoBindings()    // 代码 BindKey：Ctrl-q、Alt-=、Alt--   ← 最后执行，覆盖一切
437  InitNeoCommands()    // HSplit/VSplit/AddTab 的 wrapper 覆盖 + Ctrl-t BindKey
```

`InitNeoBindings` 排在读 json **之后**，每次启动都把 `Alt-=`/`Alt--` 钉回 `GrowPane`/`ShrinkPane`，所以 json 对这两个键的修改被覆盖。

**为什么会这样**：`Z0-切换布局-设计方案.md` 原文是「仿照 `neoHSplitAction` 的写法……绑定放 `InitNeoBindings`，紧跟现有 Ctrl-q 绑定」。实现时照抄了 `Ctrl-q` 的模板，没区分两种键的性质：

- `Ctrl-q` 是**核心退出键**，写死在最后执行是刻意的——防止用户在 json 里误改成别的导致「退不出去」。核心键该钉死。
- `Alt-=` / `Alt--` 是**纯功能键**，用户改错也没损失，照搬「钉死」是过度。

**而且这违背了 neo 自己的惯例**：neo 已经把功能键放进 defaults 的先例——

```
defaults_darwin.go / defaults_other.go:
  "Ctrl-o":     "command:file"     // neo 加的，json 可覆盖
  "Alt-Enter":  "NotePaneOpen"     // neo 加的，json 可覆盖
```

`GrowPane`/`ShrinkPane` 当初没跟着走，是不一致。

---

## 3. 关键约束（决定方案形态）

`command_neo.go:27` 的注释点明了一个坑：**`BindKey` 解析时会当场查一次 `BufKeyActions` 并缓存函数指针，运行时按键用缓存、不再查 map。**

推论：要绑的 action 必须在 `BindKey` 之前就已注册进 `BufKeyActions`，否则缓存到空指针、按键失效。

`BufKeyActions` 是包级 map（`bufpane.go:747`），包初始化时就填好，早于一切 `Init*`。`NotePaneOpen` 就注册在里面（`bufpane.go:877`，值是包内函数 `notePaneOpen`），所以 defaults 能绑它、且 json 能覆盖——这是现成的、可照搬的同构先例。

---

## 4. 设计

### 4.1 两步改动（都有 NotePaneOpen 先例）

**第一步：action 注册前移到包级 map。**

`bufpane.go:877`（`NotePaneOpen` 旁）加两行：

```go
"NotePaneOpen":              notePaneOpen,
"GrowPane":                  (*BufPane).GrowPane,
"ShrinkPane":                (*BufPane).ShrinkPane,
```

`GrowPane`/`ShrinkPane` 方法本体定义在 `command_neo.go:242 / 252`，和 `bufpane.go` 同属 `action` 包，包级 map 字面量跨文件引用同包方法无任何问题（`NotePaneOpen`→`notePaneOpen` 也是跨文件先例）。**方法实现不动，只改注册位置。**

**第二步：键位进 bufdefaults，删 InitNeoBindings 的硬绑。**

`defaults_darwin.go` + `defaults_other.go` 的 `bufdefaults` 各加两行（放 `Alt-,`/`Alt-.` 那一组附近即可）：

```go
"Alt-=": "GrowPane",
"Alt--": "ShrinkPane",
```

`command_neo.go:40-43` 这 4 行全删（2 行注册 + 2 行 BindKey）：

```go
BufKeyActions["GrowPane"]   = (*BufPane).GrowPane
BufKeyActions["ShrinkPane"] = (*BufPane).ShrinkPane
BindKey("Alt-=", "GrowPane",   Binder["buffer"])
BindKey("Alt--", "ShrinkPane", Binder["buffer"])
```

注册前移到包级 map（第一步）后，这里再赋值就是多余，必须一起删。

### 4.2 改完后的加载顺序

```
包初始化     BufKeyActions 注册 GrowPane/ShrinkPane（像 NotePaneOpen）
InitBindings  defaults 绑 Alt-=/Alt--  →  json 覆盖（这两个键从此可被用户改）
InitNeoBindings  只剩 Ctrl-q 写死
```

`InitBindings` 内部先绑 defaults、后绑 json（`bindings.go:54-86`，后者覆盖前者），所以用户 json 对 `Alt-=`/`Alt--` 的修改从此生效。defaults 绑定时 `BufKeyActions["GrowPane"]` 已在包级 map 就绪，`BindKey` 缓存到正确函数指针。

### 4.3 为什么只改这两个，不动 Ctrl-q / Ctrl-t

| 键 | 是否下沉 defaults | 原因 |
|---|---|---|
| `Alt-=` / `Alt--` | **是** | 功能键；action 实现（`GrowPane`/`ShrinkPane`）是固定方法，无 wrapper 覆盖，可安全前移到包级 map。 |
| `Ctrl-q` | 否 | 核心退出键。写死在最后执行是刻意的防误改保护，保留。 |
| `Ctrl-t` | 否 | 它绑的 `HSplit` action 被 neo wrapper 覆盖成 `neoHSplitAction`（`command_neo.go:26`），且 `BindKey` 必须在覆盖之后才能缓存到 wrapper（见 `command_neo.go:27` 注释）。若前移到 defaults，绑定时 `BufKeyActions["HSplit"]` 还是原生 `HSplitAction`，会缓存错、birth selector 丢失。所以 Ctrl-t 必须留在「先覆盖 action、再 BindKey」的 `InitNeoCommands` 里。 |

---

## 5. 涉及文件

| 文件 | 改动 |
|---|---|
| `internal/action/bufpane.go` | `BufKeyActions` 包级 map（747 起）加 `"GrowPane"` / `"ShrinkPane"` 两行注册，挨着 `NotePaneOpen`（877）。 |
| `internal/action/defaults_darwin.go` | `bufdefaults` 加 `"Alt-=": "GrowPane"`、`"Alt--": "ShrinkPane"`。 |
| `internal/action/defaults_other.go` | 同上（两份 defaults 必须同步）。 |
| `internal/action/command_neo.go` | `InitNeoBindings` 删 `40-43` 共 4 行（GrowPane/ShrinkPane 的注册 + BindKey）。`InitNeoBindings` 之后只剩 Ctrl-q。 |

无新文件、无新抽象。全部复用既有机制（defaults + 包级 map 注册），与 `NotePaneOpen`/`Ctrl-o` 完全同构。

---

## 6. 风险 / 边界

1. **两份 defaults 必须同步**：`defaults_darwin.go` 和 `defaults_other.go` 是两份独立 map，漏改一份会导致该平台下 `Alt-=`/`Alt--` 无默认绑定（按键无反应）。实施时两份一起改、一起验证。
2. **注册前移不影响 Ctrl-t 等其它键**：本次只动 GrowPane/ShrinkPane 的注册位置，不碰 `InitNeoCommands` 里 HSplit/VSplit/AddTab 的 wrapper 覆盖（它们必须留在原位，见 §4.3）。
3. **用户 json 写错 action 名**：如 `{"Alt-=": "GroPane"}`（拼错），`BindKey` 解析时 `BufKeyActions` 查不到、缓存空指针，按键静默失效——这是 micro 原生行为（defaults 里写错也这样），非本计划引入，不额外处理。
4. **`GrowPane`/`ShrinkPane` 方法本身不动**：方法实现（`command_neo.go:242 / 252`）和 2-pane 卡口、三档比例逻辑全不受影响，本次只搬注册位置。

---

## 7. 实施顺序（出 PLAN 模式后）

1. `bufpane.go`：包级 map 加 GrowPane/ShrinkPane 注册 → `make build-quick`。
2. `defaults_darwin.go` + `defaults_other.go`：bufdefaults 加两行。
3. `command_neo.go`：删 `InitNeoBindings` 的 4 行 → `make build-quick`。
4. 验证默认行为：2 pane 时 `Alt-=`/`Alt--` 三档切换正常（与改动前一致）。
5. 验证 json 可覆盖（核心验收）：在 `~/.config/microNeo/bindings.json` 写 `{"Alt-=": "CursorUp"}`，重启 microNeo，确认 `Alt-=` 变成上移光标、不再是放大；删掉该行重启，确认恢复放大。
6. 验证换键：在 json 写 `{"Alt-+": "GrowPane"}`，确认 `Alt-+` 也能放大（证明 action 名可被任意键引用）。
