# F4d — 消除存盘双重提示（换入复用原生 :open 检查）

## 0. 一句话

modified 的 noName pane 按 `Ctrl-q` 时会被问两次「是否保存」——开 selector 前一次、selector 里再 `Ctrl-q` 时 `h.Quit()` 内部又一次。去掉开 selector 前的预检，把换入路径改成调原生 `:open`（OpenCmd，自带 modified 检查）。结果：三个出口各走一次原生检查、零自写检查代码。

---

## 1. 背景 / 动机

- F4b 落地后，noName pane（编辑了文件、modified）按 `Ctrl-q` 会触发**两次**「Save changes to X before closing? (y,n,esc)」提示。
- 用户反馈：第二次检查（`h.Quit` 内部）既然还在，第一次（开 selector 前）就是冗余，想去掉。
- 但直接删第一次检查会引入数据丢失——见 §2 根因。

前置依赖：**F4b**（`filemanager.go` 的 `QuitNeo` / `OpenBirthSelector` 必须已存在）。与 **F4c**（删 Fn 键）正交，先后任意。

---

## 2. 改前：双重检查的根因

### 2.1 两次 closePrompt 的位置

QuitNeo（`filemanager.go:76`）对 noName pane 的流程：

```
Ctrl-q → QuitNeo
  ├─ 第一次检查：h.Buf.Modified() → closePrompt("Close", proceed)   ← filemanager.go:107
  │      用户选 n → proceed()
  └─ proceed() → 开 quit selector
         ├─ Enter 选文件 → 手写 NewBufferFromFile + OpenBuffer       ← filemanager.go:90（无检查！）
         ├─ Esc          → return（取消）
         └─ Ctrl-q       → 回调 h.Quit()                              ← 第二次检查在这里
                              └─ h.Buf.Modified() → closePrompt("Quit", ForceQuit)  ← actions.go:1935
```

`closePrompt`（`actions.go:1916`）= `InfoBar.YNPrompt("Save changes to ... before closing? (y,n,esc)")`，action 参数只影响「选 y 时」的 SaveCB 行为。

### 2.2 三出口的检查分布（改前）

| 出口 | 第一次（QuitNeo 开 selector 前） | 第二次（h.Quit 内部） | 会丢数据？ |
|---|---|---|---|
| Enter 换入 | ✅ 拦 | ❌ 无 | **是** |
| Esc 取消 | ✅ 拦（多余） | ❌ | 否 |
| Ctrl-q 退出 | ✅ 拦 | ✅ 拦 | 是 |

**关键**：Enter 换入路径**只有第一次检查保护**——当前是手写 `NewBufferFromFile + OpenBuffer`（`filemanager.go:62` 和 `:90`），`OpenBuffer` 直接替换 buffer，modified 内容静默丢弃。直接删第一次检查 → Enter 裸丢数据。

---

## 3. 方案：复用原生 OpenCmd + 去预检

### 3.1 关键发现：micro 原生 `:open` 自带检查

`command.go:304` 的 `OpenCmd`（即 `:open` 命令）：

```go
func (h *BufPane) OpenCmd(args []string) {
	if len(args) > 0 {
		open := func() {
			b, err := buffer.NewBufferFromFile(args[0], buffer.BTDefault)
			if err != nil { InfoBar.Error(err); return }
			h.OpenBuffer(b)
		}
		if h.Buf.Modified() && !h.Buf.Shared() {
			h.closePrompt("Save", open)   // ← 原生 modified 检查
		} else { open() }
	} else { InfoBar.Error("No filename") }
}
```

它的 `open` 闭包和当前 Enter 回调手写的换入逻辑**一模一样**（`NewBufferFromFile + OpenBuffer`），且自带 `closePrompt("Save", open)`。所以 Enter 回调直接调 `h.OpenCmd([]string{r.Path})` 即可**免费获得检查**，行为与 `:open` 完全一致，零行为分歧。

### 3.2 两条改动

1. **两个 selector 回调的 Enter 换入**：手写 `NewBufferFromFile + OpenBuffer` → 改调 `h.OpenCmd([]string{r.Path})`
   - `OpenBirthSelector` 回调（birth selector，`isQuit=false`）
   - `QuitNeo` 的 proceed 回调（quit selector，`isQuit=true`）
2. **去掉 QuitNeo 开 selector 前的第一次检查**（`closePrompt("Close", proceed)` 整段删除），直接 `proceed()`

---

## 4. 改后：检查分布

| 出口 | 检查机制 | 来源 | 自写检查？ |
|---|---|---|---|
| Esc 取消 | 无（不丢数据） | — | 否 |
| Ctrl-q 退出 | `h.Quit()` 内部 closePrompt | 原生（`actions.go:1935`） | 否 |
| Enter 换入 | `OpenCmd` 内部 closePrompt | 原生（`command.go:314`） | 否 |

**每个丢数据出口恰好一次检查，全部复用 micro 原生，零自写检查代码。**

附带收益：birth selector 的 Enter 换入（`OpenBirthSelector`）原本也是手写、无检查；改成 OpenCmd 后，若用户在 birth 空 buffer 里打了字再换入，也会得到 modified 提示——之前会静默丢失，现在更安全。

---

## 5. 改动清单（逐字，仅 `internal/action/filemanager.go`）

### 5.1 OpenBirthSelector 回调的 Picked 分支（约 59-67 行）

改前：
```go
		if pane.Buf == nil { // R7 防御：pane 在打开期间被关
			return
		}
		b, err := buffer.NewBufferFromFile(r.Path, buffer.BTDefault)
		if err != nil {
			InfoBar.Error(err)
			return
		}
		pane.OpenBuffer(b)
```

改后：
```go
		if pane.Buf == nil { // R7 防御：OpenCmd 访问 h.Buf，nil 会 panic
			return
		}
		pane.OpenCmd([]string{r.Path}) // 复用原生 :open，自带 modified 检查（与 :open 行为一致）
```

### 5.2 QuitNeo 文档注释（约 71-76 行）

改前：
```go
// QuitNeo 是 microNeo 的 Ctrl-q / :quit 路由（重写，替换 welcome_md.go 旧版）。
//   - file-born pane（isNoName=false）→ 直接 h.Quit()（原生自带存盘提示；最后→退程序）。
//   - noName-born pane → 开 quit selector（isQuit=true）：
//       Enter 选文件 → 原地换入；Esc → 取消（回编辑，不关 pane）；
//       Ctrl-q（ReasonQuit）或窗口过窄（ReasonResize）→ h.Quit()。
```

改后：
```go
// QuitNeo 是 microNeo 的 Ctrl-q / :quit 路由（重写，替换 welcome_md.go 旧版）。
//   - file-born pane（isNoName=false）→ 直接 h.Quit()（原生自带存盘提示；最后→退程序）。
//   - noName-born pane → 开 quit selector（isQuit=true），三出口各走原生检查、不在开 selector 前预检：
//       Enter 选文件 → OpenCmd（原生 :open，自带 modified 检查）；
//       Esc → 取消（回编辑，不丢数据、不检查）；
//       Ctrl-q（ReasonQuit）/ 窗口过窄（ReasonResize）→ h.Quit()（原生自带 modified 检查）。
```

### 5.3 QuitNeo proceed 回调的 Picked 分支（约 86-95 行）

改前：
```go
			if r.Kind == Picked {
				if h.Buf == nil { // R7 防御
					return
				}
				b, err := buffer.NewBufferFromFile(r.Path, buffer.BTDefault)
				if err != nil {
					InfoBar.Error(err)
					return
				}
				h.OpenBuffer(b)
				return
			}
```

改后：
```go
			if r.Kind == Picked {
				if h.Buf == nil { // R7 防御：OpenCmd 访问 h.Buf，nil 会 panic
					return
				}
				h.OpenCmd([]string{r.Path}) // 复用原生 :open，自带 modified 检查
				return
			}
```

### 5.4 去掉 QuitNeo 开 selector 前的第一次检查（约 107-111 行）

改前：
```go
	if h.Buf.Modified() && !h.Buf.Shared() {
		h.closePrompt("Close", proceed) // y/n → proceed（开 selector）；esc → 取消留编辑
		return true
	}
	proceed()
	return true
```

改后：
```go
	proceed() // 直接开 selector：modified 检查推迟到具体出口（Enter→OpenCmd / Ctrl-q→Quit）
	return true
```

> 注：`proceed` 闭包变量保留（直接 `proceed()` 调用），不做内联——改动最小、可读性好。

---

## 6. 不碰的东西

- **`h.Quit()`（`actions.go:1927`）/ `closePrompt`（`actions.go:1916`）/ `OpenCmd`（`command.go:304`）**：全部是 micro 原生，本任务只**调用**不修改。原生 modified 检查逻辑零触碰。
- **`isNoName` 分流、Reason 分流（Esc/Quit/Resize）**：F4b 既有的 selector 出口逻辑不变，本任务只动 Enter 换入的实现方式 + 去掉前置预检。
- **R7 防御（`if h.Buf == nil`）**：保留——`OpenCmd` 首行就访问 `h.Buf.Modified()`，nil 会 panic。
- **`buffer` import**：仍被 `isNoNameBuf`（`buffer.Buffer` 类型 + `buffer.BTDefault`）使用，保留。

---

## 7. 原生侵入说明

本任务是**纯 neo 改动**（仅 `filemanager.go` 一个文件），零原生侵入。方向是「删除自写检查 + 复用原生」——既减少 neo 代码量，又让换入行为与 `:open` 严格一致，降低维护负担。

---

## 8. 验证清单

- [ ] `make build` 通过。
- [ ] **场景 A（Ctrl-q 双重提示消除）**：`microneo` → birth selector 选文件（noName pane 有了内容）→ 改几行（modified）→ `Ctrl-q` → quit selector 开（**无第一次提示**）→ selector 里 `Ctrl-q` → 恰好一次「Save changes?」提示。
- [ ] **场景 B（Enter 换入提示）**：同上进入 quit selector → Enter 选另一个文件 → 恰好一次「Save changes?」提示（与 `:open <file>` 行为一致）→ 选 n 后换入成功、原修改丢弃；选 esc 取消、留在原文件。
- [ ] **场景 C（Esc 不提示）**：modified → `Ctrl-q` → quit selector → `Esc` → 无提示，回到编辑界面，修改保留。
- [ ] **场景 D（未 modified 零提示）**：未修改的 noName pane → `Ctrl-q` → quit selector → `Ctrl-q` / Enter / Esc 三出口均无提示。
- [ ] **场景 E（birth selector 换入）**：空 birth buffer 里打几个字 → birth selector → Enter 选文件 → 得到「Save changes?」提示（F4d 前 会静默丢失）。
- [ ] **回归**：file-born pane（`microneo foo.md`）`Ctrl-q` 仍是原生一次提示；`:open` 命令行为不变。

---

## 9. CHANGELOG

- 不再在 noName pane 开 quit selector 前预先提示保存（此前会与 `h.Quit` 内部的保存提示重复）。selector 里 Enter 换入现在复用原生 `:open` 的保存检查，行为与 `:open` 一致。三个出口（Esc/Ctrl-q/Enter）各走一次原生检查，消除双重提示。
