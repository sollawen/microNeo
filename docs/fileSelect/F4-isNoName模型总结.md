# F4 — Per-pane 文件导航（isNoName 模型）

> 本文是 F4a/F4b/F4c/F4d 的总结归纳。F4a-d 实施文档已归档删除，F4 是 F4 系列的唯一权威索引——读完即可建立 F4 全貌心智模型。

---

## 0. 一句话

microNeo 给每个 pane 引入「出身」概念（isNoName）：以无名空 buffer 诞生的 pane，会在诞生时弹文件选择器让你挑文件（birth selector），按 `Ctrl-q` 时弹选择器让你挑「换入文件 or 退出」（quit selector）；`Ctrl-o` 同样用选择器替代原生 `:open` 文本输入。三个入口的选择器换入全部复用 micro 原生 `OpenCmd`，modified 检查零自写。

---

## 1. 解决什么问题

micro 原生新建 pane（`Ctrl-t`/`Ctrl-e`/`:vsplit`/`:hsplit`）永远开一个空白 buffer，用户得手动 `:open <path>`。microNeo 把「新 pane → 挑文件」自动化：新 pane 一诞生就让你选文件，省去敲路径。附带清退了所有 Fn 默认键绑定（F4c），退出/保存/查找统一走 Ctrl 组合键。

---

## 2. 核心数据结构：isNoName 出身模型

### 2.1 isNoName 是「sticky 出身不变量」，不是缓存

`BufPane.isNoName`（`bufpane.go:260`，bool）记录的是 **pane 怎么诞生的**，赋值一次终身不变：

- `true` = 以无名空 buffer 诞生（如 `Ctrl-t` 开新 tab）
- `false` = 以文件诞生（如 `:vsplit foo.md`）

**关键**：用户在 birth selector 选了文件换入后，`isNoName` **保持 true 不清**。它是「出身」语义，不是「当前是否无名」。QuitNeo 用 `isNoName`（出身）而非 `isNoNameBuf()`（当前状态）分流。

### 2.2 isNoNameBuf 三条件 AND（门控函数）

`isNoNameBuf`（`filemanager.go:19`）判定 buffer 是否「noName」，作 OpenBirthSelector 的门控：

```go
buf.AbsPath == "" && buf.Type == buffer.BTDefault && buf.Size() == 0
```

- `AbsPath==""`：无路径（非文件 buffer）
- `Type==BTDefault`：排除 Help/Log/Raw/Info/Scratch
- `Size()==0`：排除管道内容（管道有内容 → Size>0 → 非 noName，等价旧 isatty 效果）

**Size()==0 的精确语义**：`buffer.Size()`（buffer.go:726）逐行字节求和、末行不加分隔符。`NewBufferFromString("","")` 单行空 buffer Size==0（满足）；用户敲过字（含回车产生第二行）的 buffer Size≥1（不满足）。**勿改成 `len` 判空**。

---

## 3. 架构：spawn 包装 + birth selector（v2）

### 3.1 核心决策：不碰 Resize / finishInitialize

v1 曾想把 birth selector 塞进 `finishInitialize`/`Resize`，造成 neo 逻辑污染纯几何职责。**v2 彻底重设计**：birth selector 开在「spawn 包装」里——调完原生 spawn 之后、返回前。

```
spawn 包装 = birthDir(捕获父目录) → 原生 spawn() → MainTab().Resize()(必要时) → OpenBirthSelectors(新pane)
```

Resize 保持**纯几何职责**，零 neo 耦合。

### 3.2 为什么能在 spawn 之后立刻开

三种 spawn（`VSplitIndex`/`HSplitIndex`/`AddTab` 及对应 `*Cmd`）末尾都同步调 `SetActive`+`Resize`。spawn 包装返回时：
- 新 pane = `MainTab().CurPane()`（已是 active）
- BWindow 已有真实几何（`computeLayout` 预检不会 0×0 误判）

故 `OpenBirthSelector` 可立即开。

### 3.3 六个 spawn 包装（filemanager.go:113+）

3 个 key action + 3 个 command，覆盖所有「新建 pane」路径：

| 包装 | 替代的原生 | 快捷键 | 备注 |
|---|---|---|---|
| `neoAddTabAction` | AddTab | `Ctrl-t` | AddTab 不内部 Resize，需补 `MainTab().Resize()` |
| `neoVSplitAction` | VSplit | （无默认键，备用） | VSplitAction 内部已 Resize |
| `neoHSplitAction` | HSplit | （无默认键，备用） | 同上 |
| `neoNewTabCmd` | NewTabCmd | `:tab` 命令 | 补 Resize |
| `neoVSplitCmd` | VSplitCmd | `:vsplit` 命令 | |
| `neoHSplitCmd` | HSplitCmd | `:hsplit` 命令 | |

带文件参数的 `:vsplit foo` 开 file-born pane（`isNoNameBuf=false`），`OpenBirthSelector` 直接 bail。

### 3.4 启动段（micro.go:406-409）

```go
action.InitBindings()
action.InitNeoBindings()      // Ctrl-q→QuitNeo
action.InitCommands()
action.InitNeoCommands()      // :file/:quit 命令 + spawn 覆盖（最后调用，不被 clobber）
```

主窗口首个 pane 若是 noName，由 micro.go 启动段调 `OpenBirthSelectors(pane, "")`（跨包，故该函数导出大写）。

---

## 4. 运行时：三个 selector 入口

复用同一套 `FileSelector`（fileselector.go，F3 实现），通过 `isQuit` 参数区分模态：

| 入口 | 触发 | isQuit | Ctrl-q 行为 | Esc 行为 |
|---|---|---|---|---|
| **birth selector** | spawn 包装 / 启动段 | false | 吞掉（不关） | 关 selector，继续编辑空 buffer |
| **browse selector** | `Ctrl-o`（FileCmd） | false | 吞掉（不关） | 关 selector，回编辑 |
| **quit selector** | `Ctrl-q`（QuitNeo，noName pane） | true | 关 selector → h.Quit() | 关 selector，回编辑（取消退出） |

### 4.1 三个入口的回调（全部在 filemanager.go / command_neo.go）

- **birth**：`OpenBirthSelectors`（filemanager.go:48）
- **quit**：`QuitNeo`（filemanager.go:73）
- **browse**：`openSelector`（command_neo.go:80），由 `FileCmd`（:75）调用

三者 Enter 换入**完全一致**：`h.OpenCmd([]string{r.Path})`。

---

## 5. modified 检查：复用原生机制（零自写检查）

### 5.1 设计原则

三个 selector 入口的 modified 检查**全部复用 micro 原生**，不自写 `closePrompt` / `YNPrompt`：

| 出口 | 检查机制 | 原生位置 |
|---|---|---|
| Enter 换入 | `OpenCmd` 内部 `closePrompt("Save", open)` | command.go:304 |
| Ctrl-q 退出 | `h.Quit()` 内部 `closePrompt("Quit", ForceQuit)` | actions.go:1927 |
| Esc 取消 | 无（不丢数据） | — |

### 5.2 为什么不在「开 selector 前」预检

F4d 之前的 QuitNeo/F4d 之前的 FileCmd 在开 selector 前有 `closePrompt` 预检，与出口处的原生检查**串联成双重提示**。修复方案：去掉预检，让 modified 检查**推迟到具体出口**。每个丢数据出口恰好一次检查，全部原生。

### 5.3 SelectResult 分流（quit selector 回调）

```go
if r.Kind == Picked { h.OpenCmd([]string{r.Path}); return }
if r.Reason == ReasonEsc { return }              // 取消：回编辑，不关 pane
h.Quit()                                         // ReasonQuit(Ctrl-q) / ReasonResize(窄窗口)
```

---

## 6. 文件清单与原生侵入

### 6.1 neo 文件（新增/专属）

| 文件 | 职责 |
|---|---|
| `internal/action/filemanager.go` | **核心**：isNoName 模型、OpenBirthSelectors、QuitNeo、6 个 spawn 包装、InitNeoBindings |
| `internal/action/command_neo.go` | 命令注册（InitNeoCommands）、spawn key action 覆盖、FileCmd/openSelector、QuitNeoCmd |

### 6.2 原生侵入（极小）

| 文件 | 改动 | 行号 |
|---|---|---|
| `internal/action/bufpane.go` | +1 字段 `isNoName bool` | :260 |
| `cmd/micro/micro.go` | 启动段 4 行（InitNeoBindings/InitNeoCommands/OpenBirthSelectors） | :406-409 |
| `internal/action/defaults_darwin.go` | 删 Fn 默认绑定（F4c） | |
| `internal/action/defaults_other.go` | 同上（对称） | |
| `runtime/help/defaultkeys.md` | 删 Function keys 小节（F4c） | |

`finishInitialize` / `Resize` / `bufwindow` / 渲染管线——**零触碰**。

### 6.3 已删除

- `internal/display/welcome_md.go`：v1 的 welcome 模式（F4b 整删，被 isNoName 模型取代）

---

## 7. 关键不变量与边界（勿违反）

1. **isNoName 是 sticky 出身，不是缓存**。Picked 换入后不清。QuitNeo 用 isNoName（出身）判分流。
2. **isNoNameBuf 三条件用 AND，含 Size()==0**。勿改 `len` 判空（语义不同，见 §2.2）。
3. **Resize 保持纯几何**。任何 neo 逻辑不得进 Resize/finishInitialize。birth selector 只在 spawn 包装里开。
4. **spawn 包装必须补 Resize（AddTab/NewTabCmd）**。VSplit/HSplit 内部已 Resize。补 Resize 是为了让新 pane BWindow 几何就绪。
5. **modified 检查走原生，不自写**。Enter→OpenCmd、Ctrl-q→h.Quit、Esc→不检查。
6. **ReasonResize 必须走 h.Quit()**（不能只关 selector）。否则窄窗口下 noName pane 退不出去（死锁）。
7. **R7 防御保留**：selector 回调里 `if h.Buf == nil { return }`——OpenCmd 首行访问 `h.Buf.Modified()`，nil 会 panic。

---

## 8. 已知坑（务必注意）

### 8.1 BindKey 缓存机制（Ctrl-t 曾因此失效）

`internal/action/bufpane.go` 的 `BindKey` 把 action 名通过 `BufKeyActions[a]` 解析成函数指针，**绑定时缓存进闭包**。运行时按键调缓存闭包、**不再查 map**。

**后果**：只改 `BufKeyActions["AddTab"]`（spawn 覆盖）而不重新 `BindKey`，按键仍走原版。

**修复**（command_neo.go:26）：改 map 后必须补 `BindKey("Ctrl-t", "AddTab", Binder["buffer"])`。VSplit/HSplit 无默认快捷键，覆盖仅备用。

### 8.2 InitNeoCommands 必须最后调用

注册时序（micro.go:406-409）：`InitBindings → InitNeoBindings → InitCommands → InitNeoCommands`。InitNeoCommands 最后调用，确保不被 InitCommands 的原生注册 clobber。

### 8.3 spawn 包装的 Resize 补丁

AddTab/NewTabCmd 不像 VSplitIndex 那样内部 Resize，spawn 包装里必须手动补 `MainTab().Resize()`，否则新 pane BWindow 几何未就绪，selector 预检会误判 0×0。

---

## 9. 配套改动：删除 Fn 默认绑定（F4c）

microNeo 产品决策：**不预置 Fn 默认绑定**，改用 Ctrl 组合键。不是「禁止绑 F 键」。

- **删**：`defaults_darwin.go` / `defaults_other.go` 的 F2/F3/F4/F7/F10（buffer）+ F10（prompt）；`defaultkeys.md` 整个 Function keys 小节。
- **保留**：`bindings.go` 的 F1-F65 按键识别表（底层识别层，删了 F 键无法识别）；`keybindings.md` 的 F1-F59 键名参考列表（陈述底层可绑定能力，与 bindings.go 同理）。
- **功能替代**：F2→`Ctrl-s`、F3/F7→`Ctrl-f`、F4/F10→`Ctrl-q`。零功能损失。

用户仍可在 `settings.json` 手动绑 Fn（底层识别能力保留）。

---

## 10. 与 micro 原生的关系

- **ExitPoint（文件打开）全走原生 OpenCmd**：换入 = `:open`，行为严格一致（modified 检查、错误处理）。
- **file-born pane 的 Ctrl-q = 原生 h.Quit()**：`QuitNeo` 开头 `if !h.isNoName { return h.Quit() }`，零行为变化。
- **FileSelector（fileselector.go）**：F3 实现的选择器 UI（floatframe 浮层），F4 复用它做三个入口。FileSelector 本身的设计在 F3 文档。

---

## 11. 后续可扩展点

- **更多 spawn 路径**：若 micro 新增 pane 创建方式，加对应的 neo 包装（capture dir → 原生 spawn → Resize → OpenBirthSelectors）。
- **isNoName 的持久化**：当前仅在 pane 生命周期内，不跨会话。
- **selector 起始目录策略**：现取父 pane 目录，可扩展为最近打开目录/书签。

---

## 12. 历史与演进

F4a（isNoName 模型设计）→ F4b（v2 实施方案，废弃 F4a 的 finishInitialize/Resize 耦合设计）→ F4c（删 Fn 键）→ F4d（消除存盘双重提示，换入复用 OpenCmd）。

F4a-d 实施文档已归档删除，本文为唯一总结。关键 commit：
- `6e56674c` F4b 主体（per-pane file navigation）
- `ad9cff44` Esc 取消 quit selector
- `cb3e8370` F4d（birth/quit 回调复用 OpenCmd + 去预检）
- `d6f3bf2e` F4d 对称应用到 Ctrl-o
- `970d1f33` F4c（删 Fn 键默认绑定）
