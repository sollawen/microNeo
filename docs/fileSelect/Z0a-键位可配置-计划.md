# `Ctrl-t` 可配置 —— 计划

需求来源：`Z0-切换布局-设计方案.md` 落地后，发现 neo 的 `Ctrl-t` 分屏键写死在代码里，用户无法用 `bindings.json` 改。本文是修复计划。

---

## 1. 目标

让 `Ctrl-t` 能被用户的 `bindings.json` 覆盖，对齐 `Ctrl-o`、`Alt-Enter` 等已有功能键的待遇。

---

## 2. 现状与根因

**问题**：用户在 `~/.config/microNeo/bindings.json` 改 `Ctrl-t` 后重启，设置被无视。

**根因在加载顺序**（`cmd/micro/micro.go:434-437`）：

```
434  InitBindings()       // 读 defaults + 读 bindings.json
435  InitNeoBindings()    // 代码 BindKey：Ctrl-q              ← 晚于 json，覆盖一切
437  InitNeoCommands()    // HSplit wrapper 覆盖 + Ctrl-t BindKey   ← 晚于 json
```

`InitNeoCommands` 排在读 json **之后**，每次启动都把 `Ctrl-t` 钉回 neo 默认，json 的修改被覆盖。

---

## 3. 关键约束

`command_neo.go:27` 的注释：**`BindKey` 解析时会当场查一次 `BufKeyActions` 并缓存函数指针，运行时按键用缓存、不再查 map。**

推论：要绑的 action 必须在 `BindKey` 之前就已注册进 `BufKeyActions`。

`BufKeyActions` 是包级 map（`bufpane.go:747`），包初始化时就填好，早于一切 `Init*`。

---

## 4. 设计

`Ctrl-t` 绑的 `"HSplit"` 这个 action 名，被 neo 运行时覆盖成了 wrapper（`BufKeyActions["HSplit"]=neoHSplitAction`，`command_neo.go:26`）。要让 defaults 绑 `Ctrl-t` 时缓存到 wrapper，必须让 `"HSplit"` 在**包初始化时**就指向 wrapper。

**关键事实（已核实）**：

- `neoHSplitAction`（`command_neo.go:130`）内部直接调 `h.HSplitAction()`（方法调用，不查 map），不会递归。
- 全代码库对 `HSplitAction` 的引用都是**直接方法调用**（`command.go:531 h.HSplitAction()`），不经过 `BufKeyActions` map。

**方案**：

1. **包级 map 直接注册 wrapper**：`bufpane.go` 把 `"HSplit"` 的 value 从 `(*BufPane).HSplitAction` 改成 `(*BufPane).neoHSplitAction`。
2. **defaults 改 Ctrl-t 的值**：`defaults_darwin.go` + `defaults_other.go` 把 `"Ctrl-t": "AddTab"` 改成 `"Ctrl-t": "HSplit"`（这正是 neo 想要的语义，原本靠 `InitNeoCommands` 的 BindKey 实现，现在前移到 defaults）。
3. **删 `InitNeoCommands` 的两行**（`command_neo.go:26-27`）：`BufKeyActions["HSplit"]=neoHSplitAction`（包级 map 已注册）+ `BindKey("Ctrl-t","HSplit")`（defaults 已绑）。

**注**：`VSplit` wrapper 保留不动——它不在任何默认键位上，无需前移；保留在 `InitNeoCommands` 中仍作为备用，保证用户若自行写 `{"Ctrl-t": "VSplit"}` 时能拿到带 birth selector 的版本。

改完后包初始化时 `BufKeyActions["HSplit"]` 就是 wrapper，defaults 绑 `Ctrl-t:"HSplit"` 缓存到 wrapper ✓，json 可覆盖 ✓。

---

## 5. 改完后的加载顺序

```
包初始化      BufKeyActions: HSplit=neoHSplitAction
InitBindings   defaults 绑 Ctrl-t → json 覆盖（从此可被用户改）
InitNeoBindings   Ctrl-q 写死
InitNeoCommands   commands 覆盖（neoHSplitCmd 等）+ VSplit/AddTab 的备用 wrapper
```

`InitBindings` 内部先绑 defaults、后绑 json（`bindings.go:54-86`），后者覆盖前者。

---

## 6. 涉及文件

| 文件 | 改动 |
|---|---|
| `internal/action/bufpane.go` | 包级 map（:859）：把 `"HSplit"` 从 `HSplitAction` 改成 `neoHSplitAction`。 |
| `internal/action/defaults_darwin.go` | `bufdefaults`：`Ctrl-t` 从 `"AddTab"` 改 `"HSplit"`。 |
| `internal/action/defaults_other.go` | 同上（两份 defaults 必须同步）。 |
| `internal/action/command_neo.go` | `InitNeoCommands` 删 `26-27`（HSplit wrapper 覆盖 + Ctrl-t BindKey）。 |

---

## 7. 风险 / 边界

1. **两份 defaults 必须同步**：`defaults_darwin.go` 和 `defaults_other.go` 是两份独立 map，漏改一份会导致该平台下 `Ctrl-t` 无默认绑定。
2. **`"HSplit"` 全局语义变化（时机前移，运行时一致）**：包级 map 把 `"HSplit"` 指向 wrapper 后，从包初始化起就是 `neoHSplitAction`（原本是 `InitNeoCommands` 运行时才覆盖）。两者都在首次按键前完成，**运行时行为无差异**；且全代码库无人通过 map 查 `"HSplit"`，故无副作用。
3. **`neoHSplitAction` 不递归**：它调 `h.HSplitAction()`（原生方法，`actions.go:2031`），不查 map，不会循环。
4. **用户 json 写错 action 名**：如 `{"Ctrl-t": "HSplitt"}`，`BindKey` 解析时查不到、缓存空指针，按键静默失效——micro 原生行为。

---

## 8. 实施顺序（出 PLAN 模式后）

1. `bufpane.go`：包级 map `"HSplit"` 改 `neoHSplitAction` → `make build-quick`。此步单独完成时 `Ctrl-t` 默认行为不变（仍开 tab），仅 `BufKeyActions["HSplit"]` 的值变成 wrapper。
2. `defaults_darwin.go` + `defaults_other.go`：`Ctrl-t` 改 `"HSplit"`。
3. `command_neo.go`：删 `InitNeoCommands` 的 2 行 → `make build-quick`。
4. 验证默认行为（与改动前一致）：
   - `Ctrl-t` 仍是上下分屏 + birth selector（验证 selector 是否弹出）。
   - 2 pane 时再 `Ctrl-t` 撞 2-pane 卡口 + 提示。
5. 验证 json 可覆盖（核心验收）：
   - `{"Ctrl-t": "AddTab"}` 重启 → `Ctrl-t` 变回开新 tab（证明能改回原生）。
   - 删掉这行重启 → 恢复 neo 默认。
6. 验证换键：`{"Alt-t": "HSplit"}` → `Alt-t` 也能分屏（证明 action 名可被任意键引用）。
