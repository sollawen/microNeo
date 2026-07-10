# F4c — 删除 Fn 功能键默认绑定

## 0. 一句话

从 microNeo 默认键绑定中移除所有 Fn 功能键绑定（实际只有 F2/F3/F4/F7/F10 被默认绑过）。这些键「全世界没几个人用」，且常与终端 / 文件管理器的 Fn 冲突。从今天起 microNeo 不再提供 Fn 默认绑定，统一改用 Ctrl 组合键。

---

## 1. 背景 / 动机

- F4/F10 在旧 welcome 路径下被重绑到 QuitNeo（`welcome_md.go:46-47`），F4b 整删 welcome_md.go 后这层重绑已消失。但 defaults 里的**默认**绑定 F2/F3/F4/F7/F10 还在。
- 这些绑定来自 micro 原生的 `// Integration with file managers` 历史遗留（让外部文件管理器调 micro 时用 Fn 触发 Save/Find/Quit）。
- 全部有等价的 Ctrl 替代键（见 §5），删了不丢任何功能。
- **用户决策**：microNeo 全面清退 Fn 键，直接从原生 defaults 删。

---

## 2. 执行时序

**F4b 完成 + 测试通过之后**再执行 F4c。F4c 是独立的收尾清理，与 F4b 的 neo 代码正交（F4b 删 welcome_md.go 已带走 F4/F10 的 welcome 重绑；F4c 只动 defaults + help 文档）。

---

## 3. 删除清单（逐文件逐行）

### 3.1 `internal/action/defaults_darwin.go`

**buffer pane 绑定表**（约 89-94 行）：
```
	// Integration with file managers
	"F2":  "Save",
	"F3":  "Find",
	"F4":  "Quit",
	"F7":  "Find",
	"F10": "Quit",
```
→ 整块删除（含 `// Integration with file managers` 注释，因删完该注释下方已无 Fn 内容）。

**prompt pane 绑定表**（约 178-179 行）：
```
	// Integration with file managers
	"F10": "AbortCommand",
```
→ 删 `"F10": "AbortCommand",` 一行。上方 `// Integration with file managers` 注释**保留**（紧邻的 `"Esc": "AbortCommand"` 仍在，注释对它仍成立）。

### 3.2 `internal/action/defaults_other.go`

与 3.1 **完全对称**，行号约 +3（buffer 92-97、prompt 181-182）。删除内容、注释处理规则同 3.1。

> 两文件已 diff 确认 Fn 绑定完全一致，无差异。

### 3.3 `runtime/help/defaultkeys.md`

默认键表约 136-141 行，删除 6 行：
```
| F1    | Open help                 |
| F2    | Save                      |
| F3    | Find                      |
| F4    | Quit                      |
| F7    | Find                      |
| F10   | Quit                      |
```
> 注：`F1 → Open help` 是 **stale 文档**——defaults 里实际从未绑 F1（已核实）。一并删除以保持文档与默认绑定一致。

### 3.4 `runtime/help/keybindings.md` —— 不删（键名参考保留）

391-449 行是 F1-F59 的「可绑定键名参考」列表——纯键名清单（无功能说明），陈述的是底层可绑定能力，不是默认绑定。按本任务原则「删了哪些键的默认绑定，就删哪些键的说明文字」：只有 defaultkeys.md（§3.3，含功能说明的默认键位表）需要删行；keybindings.md 是键名索引、不在删除范围。且它与 §4 保留 bindings.go 同理——microNeo 是「不预置 Fn 默认绑定」，不是「禁止绑 F 键」，用户仍可在 settings.json 手动绑，该参考列表保留。

---

## 4. 不碰的东西

- **`internal/action/bindings.go` 的 `"F1".."F65" → tcell.KeyFn` 映射表**（约 416-480 行）：这是**底层按键识别层**——终端发来 F 键事件时，靠它把按键名翻译成 `tcell.Key`。删了会让 microNeo 连 F 键都识别不了，易出 bug、且无收益。**保留**。
  - 含义：用户仍可在自己的 `settings.json` 里手动绑 `"F5": "Save"` 之类（底层能识别），microNeo 只是**不预置 Fn 默认绑定**（defaultkeys.md 同步删除）。这是合理的工程边界。
- **`welcome_md.go`**：F4b 已整删（含 F4/F10 的 welcome 重绑），F4c 不涉及。
- **F5/F6/F8/F9/F11/F12**：defaults 里**本来就没有**任何默认绑定（已核实），无需处理。

---

## 5. 替代键对照（删 Fn 后用户用什么）

| 原 Fn 绑定 | 功能 | 替代键（已存在）|
|---|---|---|
| F2 | Save | `Ctrl-s` |
| F3 / F7 | Find | `Ctrl-f`（`Ctrl-n` / `Ctrl-p` 翻下一个）|
| F4 / F10（buffer）| Quit | `Ctrl-q`（microNeo 的 QuitNeo，F4b 绑定）|
| F10（prompt）| AbortCommand | `Ctrl-q` / `Esc`（prompt 早有）|

所有替代键在 `defaults_darwin.go` 已确认存在（`Ctrl-s` :40 / `Ctrl-f` :41 / `Ctrl-q` :74），删除 Fn 后**零功能损失**。

---

## 6. 原生侵入说明

本任务是**纯原生改动**（`defaults_darwin.go` / `defaults_other.go` / help 文档），无 neo 文件参与。与 microNeo「原生侵入最小」原则的关系：

- 这是用户主动的**产品决策**（清退 Fn），不是 neo 逻辑需要。
- 改动是**删除**而非注入 neo 代码，性质上是给 microNeo 做减法，不增加维护负担、不改变渲染/交互逻辑。
- 与 F4b 的 neo 改动（`bufpane.go` / `micro.go`）完全正交，互不影响，可独立回滚。

---

## 7. 验证清单

- [ ] 启动 microNeo，按 F2 / F3 / F4 / F7 / F10 → 均无响应（不触发 Save / Find / Quit）。
- [ ] `Ctrl-s` 仍能保存、`Ctrl-f` 仍能查找、`Ctrl-q` 仍走 QuitNeo。
- [ ] prompt 状态下按 F10 → 无响应（不触发 AbortCommand）；`Ctrl-q` / `Esc` 仍能取消命令。
- [ ] `settings.json` 里手动绑 `"F5": "Save"` → 仍生效（底层识别能力保留）。
- [ ] `:help` 打开帮助，键位表不再出现 F1-F10。
- [ ] 回归：确认无其它依赖 Fn 的代码路径报错（全库已搜，除 defaults/help/welcome 外无 Fn 引用）。

---

## 8. CHANGELOG

- 移除所有 Fn 功能键默认绑定（F2/F3/F4/F7/F10）；保存/查找/退出改用 Ctrl-s / Ctrl-f / Ctrl-q。底层 F 键识别能力保留，用户仍可自行绑定。
