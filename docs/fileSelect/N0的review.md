已读完方案 + 核对相关代码（`fileselector.go` / `filemanager.go` / `floatframe.go` / `command_neo.go` / `bufpane.go` / `tab.go` / `micro.go`）。下面是 CTO 视角的技术评估。

---

## 一、总体判断

**方向正确，核心架构判断站得住，但存在 1 个会导致代码编不过/语义矛盾的设计硬伤，以及 1 个落地阻塞项。** 修掉硬伤、拍板开放项后可执行。落地顺序（E 节）合理，低风险先行。

方案的"数据进数据出 + intent-agnostic + 断 floatFrame"三原则是干净的，对现有代码的依赖分析（import 环、R7 nil 防御、birth 收敛）我逐一核过代码，结论成立。

---

## 二、已核验成立的判断（这些是方案的底气）

1. **断 floatFrame 的 import 环是真约束**。`finder → action（用 TheFloatFrame）` + `action/fileops → finder（调 Open）` 确实成环。同时 `screen`/`config` 都不反向 import `action`（已 grep 确认），所以 `finder → screen` / `finder → config` 合法。A.5 的双重论证成立。

2. **R7 nil 防御触发条件为空集**。已确认：`BufPane.Close()`（bufpane.go:713）只调 `Buf.Close()` 不置 nil；`Tab.RemovePane`（tab.go:373）nil 的是 slice 元素不是 `pane.Buf`；全库无 `.Buf = nil` 赋值点。B.3 删 nil 检查安全。

3. **birth trigger 收敛（7→1）是真简化**。6 个 spawn wrapper（command_neo.go:253-303）确实 100% 是"原生之后追加 OpenBirthSelector"，hook 接管后它们退化成原生等价物，可整段删。`isNoNameBuf` 门控塌进 `if h.Buf.AbsPath == ""` 也成立。

4. **Cwd 缺口（B.8）是真 bug**。`lastWorkingDir()`（micro.go:285）只读 `pane.Buf.AbsPath` 的父目录，对 finder 内导航完全无感。noName pane + quit finder 导航到 B 目录的场景确实会丢同步。加 `Result.Cwd` 是对的。

5. **Result.Kind 坍缩**（Picked 与 Esc/Quit/Resize/TooSmall 同级）正确，原两级分类是过度结构。

6. **isNoName sticky 语义保留**的论证严谨（B.4）：出生 Enter 选文件后 AbsPath 非空但 isNoName 仍 true，Ctrl-q 仍开 quit finder——这是必须保住的行为，不能用 `AbsPath==""` 替代。

---

## 三、必须修正的硬伤（开干前改）

### 硬伤 1：`finderOnClose` 的 `h` 作用域自相矛盾（B.3 / B.6）

方案一边声明"无状态包级 var"，一边让回调体调用实例方法：

```go
var finderOnClose func(finder.Result)          // B.3：包级 var，"无状态"
func onFinderClose(r finder.Result) {
    switch r.Reason {
    case Picked: h.OpenCmd(...)                 // ← h 从哪来？
    case Quit:   doQuit(r)                      // doQuit 内部也要 h.Quit()
    }
}
```

包级 `onFinderClose` 里 **`h` 不在作用域**。三个 caller（FileCmd / QuitNeo / finishInitialize hook）的 `h` 是不同的 pane 实例，一个无状态包级函数拿不到。

**这是代码级矛盾，照写编不过/行为错。** 修法二选一，方案需明确：

- **(a) 闭包绑定（推荐）**：`finderOnClose` 不是顶层 `var` + 顶层 `func`，而是 `OpenFinder` 内每次赋值的闭包，捕获 `h`：
  ```go
  func (h *BufPane) OpenFinder(isQuit bool) {
      finder.OnClose(func(r finder.Result) { h.onFinderResult(r, isQuit) })
      finder.Open(...)
  }
  ```
  "共享同一份逻辑"通过 `onFinderResult` method 复用实现，**但回调本身是 per-call 的、有状态的（捕获 h）**。方案"无状态"的措辞要改。

- **(b) 全局 current-owner**：finder 包或 action 包持一个 `var currentOwner *BufPane`，OpenFinder 时赋值，onClose 读。代价是引入进程级可变状态，违反方案自己的 §2.4"零状态"。

**建议选 (a)**，并把 B.6 的决策表挂在 `(*BufPane).onFinderResult(r, isQuit)` 上。这样 isQuit 也从 Result 字段退化成闭包捕获的入参——`Result.IsQuit` 这个字段可以**整段删掉**（见硬伤 2 衍生）。

### 硬伤 2：`Open` 是 method 还是包级函数，前后不一致（A.2 vs B.3）

- A.2 签名：`func (fm *FileManager) Open(...)` —— method，意味着 finder 是**实例**
- B.3 调用：`finder.Open(...)` —— 包级函数，意味着 finder 是**包级单例**（像 `TheFloatFrame`）
- A.4 又列 `IsOpen()`/`HandleEvent()`/`Display()` 为方法 —— 方法要有 receiver

**finder 到底是"owner 持有的实例"还是"包级单例"？** 这决定了：
- 多 pane 能否各持一个 finder（现状 FloatFrame 是全局单例，C1 约束"同时最多一个"）
- 主循环怎么找到"当前要画的 finder"（D.1 渲染归属直接依赖于此）

现状 FloatFrame 是全局单例（`TheFloatFrame`，globals.go InitGlobals），主循环 `if TheFloatFrame.IsOpen() { TheFloatFrame.Display() }`（micro.go:536-537）。**finder 要顶替它的位置，最省事的就是同样做包级单例**（包内 `var theFinder *FileManager`，导出 `Open/IsOpen/HandleEvent/Display` 包级函数转发）。那 A.2 的 method 签名是笔误，应统一成包级函数。**需在方案里拍板并统一所有签名。**

---

## 四、落地阻塞项（D 节，方案自己标了"必须拍板"）

### 阻塞 1：D.1 overlay 渲染归属 —— 这是最大的未决项

方案正确指出两难：
- `BufPane.Display()` 末尾画 → 会被后画的兄弟 pane / NotePane 盖掉（渲染顺序 bug）
- 主循环末尾画 → 和现状 FloatFrame 一致，但"调用者"就不再是 BufPane 了

我补充一个**方案没点破的关键点**：D.1 的答案几乎被现状代码锁定为主循环。理由：
- 现状 FloatFrame 就是在主循环画（micro.go:536），finder 断 FloatFrame 后要无缝顶替，最自然就是接同一位置。
- N1a-floatFrame技术架构方案.md 里已经为同一问题纠结过，结论也是"渲染回到主循环"（owner-local 只管 lifecycle + 事件路由，渲染是全局视野问题）。
- finder 是 pane-local 锚点（钉在 owner pane 左上角），但**画的动作**仍然要晚于所有 pane。这两个不矛盾：lifecycle owner = BufPane，per-frame renderer = 主循环。方案 D.1 末尾已经说到这层区分，但没敢落定。

**CTO 建议：直接定为主循环渲染**，D.1 关闭。代价：主循环多一处 `if finder.IsOpen() { finder.Display() }`，且要加进 resize 广播列表（micro.go:584 附近）。这是机械改动，不是新设计。

### 阻塞 2：D.3 与 N0b 的执行顺序 —— 方案没给推荐

N0 步 3（升格 overlay）和 N0b（per-owner FloatFrame）都动 overlay 路由，必须错开。方案列了约束但没说谁先。

**CTO 建议**：
- N0 步 1（平移 git/字符串工具进 finder 包）+ 步 2（Open 改纯值、fileops 改名）**与 N0b 完全正交**，先合，立刻减重。
- 步 3（升格 overlay）+ 步 4（接 cwd）**等 N0b 定向后再定**。若 N0b 短期不动，N0 步 3 可独立做（finder 接管主循环渲染位，FloatFrame 暂留服务 SelectPane）；若 N0b 要先做，N0 步 3 顺势收尾。**不要两个 PR 并行开。**

---

## 五、可接受但有代价的取舍（方案诚实承认了，我确认代价）

1. **`birthDir` 回归（B.3）**：`:cd ~/other` 后 HSplit，新 pane finder 起始目录从 `dir(当前文件)` 退到 cwd。方案说"绝大多数 cwd==dir(当前文件)"——对，但 **`:cd` 用户必中**。代价微小，可接受。若想零回归，hook 处拿不到父 pane 目录（新 pane 构造时没传），要保就得在 `newBufPane`/`NewBufPaneFromBuf` 加一个"建议起始目录"参数——侵入面比删 wrapper 大，不值。**维持方案取舍。**

2. **启动段时序（micro.go:489-513）**：现状启动 pane 走 `NewBufPaneFromBuf`（ postpone finishInitialize），靠主循环开跑前那个 resize 事件（micro.go:489 的 `select`）触发首次 Resize → finishInitialize。方案删 micro.go:513、改由 finishInitialize hook 自动开 birth finder。**风险**：若那个 resize 事件走 `<-time.After(10ms)` 超时分支（没收到 resize），finishInitialize 推迟到主循环第一个 DoEvent 里的 resize——birth finder 会晚一帧才开，用户可能看到一闪空 pane。现状的 513 显式调用是兜底。**建议保留启动段的显式触发作为兜底，或确认 10ms 超时分支在所有终端下不会漏掉 birth。** 至少要在落地时手测 ssh/慢终端。

3. **display/handleEvent 不做 stub 单测（A.7）**：诚实的取舍，但和 §2.4"零状态、纯值+stub Host 即可单测"措辞冲突——**真正能单测的只有 git 解析 + 字符串工具**，交互层仍依赖 screen/config 单例。建议把 §2.4 第 4 条改成"纯函数部分零状态可单测"，别让后人误以为整个 finder 能脱离 screen 测。

---

## 六、几处小缝（落地时顺手收）

1. **`Rect` 类型归属**：现 `Rect`/`Pos`/`Size` 定义在 `floatframe.go`（action 包）。finder 断 FloatFrame 后要**自定义 `Rect`**（A.2 的 rect 入参类型），别 import action 的、也别 import `display.View`（会抬耦合层）。方案没明说，补一句。

2. **fsMinWidth/fsMinHeight + InfoBar 提示**：现 `computeLayout`（fileselector.go:365/369）用 action 的 `InfoBar.Message` 报阈值。搬包后 finder 不能调 action.InfoBar，A.2 说挪到 fileops 的 TooSmall 分支。那 finder 要**导出 `MinWidth`/`MinHeight` 常量**供 fileops 拼 "pane too narrow (need N cols)"，否则阈值数字两边各写一份会漂。

3. **`lastFinderCwd` 跨包**：`lastWorkingDir()` 在 package `main`（micro.go:285），`lastFinderCwd` 在 package `action`。main 读 action 的变量要**导出**（`action.LastFinderCwd` 或 getter）。B.6 符号清单里没标导出，补。

4. **finder 关闭后的 fetchGit 竞态**：`go fs.fetchGit` 可能在 finder close 后才回来写 `state.allEntries` 并 `screen.Redraw()`。现状靠 FloatFrame.IsOpen 守 Display；finder 升格后自己的 `Display()` 也要 `if !isOpen { return }` 守住（A.4 的 IsOpen 正好用上）。方案没显式提，落地时确认 close 置 `isOpen=false` 在前、Display 守门。

5. **同步 TooSmall 回调的重入**：若 `Open` 内同步触发 `onClose(TooSmall)`（预检拒开），回调在 `OpenFinder` 调用栈内执行。birth+isQuit=false 时 TooSmall→no-op，安全；但要确认 onClose 里不出现"在 finishInitialize 栈里 Quit 自己"的路径。birth 走 false 不会，OK。

---

## 七、落地建议（在方案 E 节基础上的收紧）

| 步 | 内容 | 阻塞？ | 建议 |
|---|---|---|---|
| 1 | 平移 `fileselector_git.go` + 字符串工具 + 测试进 `internal/finder/` | 无 | **立即合**，零行为变化，立刻减重、立刻可跑 |
| 2 | `Open` 改纯值入参；`filemanager.go` → `fileops.go`；**修硬伤 1/2**（finder 定为包级单例，onClose 改闭包绑定 h）；删 R7 nil 检查 | 无 | 合。**先把硬伤 1/2 在这步修掉**，否则步 3 没法接 |
| 3 | 升格 pane 级 overlay（搬边框/resize/预检）；统一 onClose 决策表；**D.1 定为主循环渲染** | 与 N0b 互斥 | 等 N0b 定向。若 N0b 暂缓，本步可独立做 |
| 4 | `Result.Cwd` + `doQuit` 暂存 + `lastWorkingDir` 优先读 | 无 | 可与步 3 合并或紧跟 |

**步 1-2 现在就能开干**，且把硬伤 1/2 在步 2 一并修掉。步 3-4 等 N0b 拍顺序。

---

## 八、一句话结论

**架构方向对、依赖分析扎实、简化点都是真的；但 B.3/B.6 的 `finderOnClose` 作用域是必须修的代码级硬伤，A.2/B.3 的 method-vs-包函数不一致要拍板，D.1 渲染归属建议直接定主循环以解阻塞。** 修完这三处，按 E 节顺序、步 1-2 先行，就可以执行了。

需要我把这份评估整理成 `docs/fileSelect/N0-技术评估.md` 存档吗？
