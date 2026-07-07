# F1b · FileSelector 实施方案

**状态**：草案（待评审）
**依据**：F1（架构，v1 定稿）+ F0（产品）+ 源码现状（`command_neo.go` / `selectpane.go` / `buffer`）
**前置**：**F1a 必须先合入**（`FloatOpenSpec` / `AutoExpand=false` / `onCancel` 就位，F1 D5）。
**交付**：`:file` 命令完整闭环——打开 → 列目录 → 导航 → 选中开文件，含 git 状态渐进显示与多重降级。
**原生侵入**：`settings.go` + `settings.json` 各一行（沿用 `status-separator` 先例）；其余全在 microNeo 自有/新建文件。

---

## 1. 目标与范围

实现 F1 架构落地的 FileSelector 全部 v1 能力（F1 §14.1）：
- pane-local layout（全高 + 40% 宽）+ min 拒开
- 浏览 / 导航（↑↓←→/Enter 上下文）/ 选中打开闭环
- 面包屑、上下文 hint、当前文件定位、dotfile 自动显隐
- git 状态（M/U/A/D/R）行内显示 + 异步渐进 + 多重降级
- modified 保护；resize 即关（继承 F1a）

本方案只讲 **怎么干、按什么顺序、怎么验**——架构论证见 F1，产品行为见 F0，不重复。

---

## 2. 前置与交付物

| 项 | 说明 |
|----|------|
| 代码前置 | **F1a 已合入**（`Open(FloatOpenSpec)`、`AutoExpand`、`onCancel`/resize 即关） |
| 新建文件 | `internal/action/fileselector.go`（主体）、`internal/action/fileselector_git.go`（git 子模块） |
| 改动文件 | `internal/action/command_neo.go`（注册 `:file` + `FileCmd`）、`internal/config/settings.go` + `runtime/settings.json`（`fileselectwidth`） |
| 零改动 | `cmd/micro/micro.go`（事件路由/绘制顺序复用 FloatFrame）、`floatframe.go`（F1a 后不再动）、原生 `command.go`/`actions.go`/`bufpane.go` |
| 不在本方案 | fuzzy 过滤（v1.1）、I(Ignored) 状态（R4）、`:file <path>` 接参数 |

---

## 3. 任务分解

四个任务。**T1、T2 相互独立可并行**；T3 是主体（最重）；T4 串联（依赖 T3）。推荐顺序：T1 → T2（可独立单测）→ T3 → T4 → 验证。

---

### T1 · 配置注入（`settings.go` + `settings.json`）

**目标**：注册 `fileselectwidth`（默认 0.4）。最小、独立，先落地解阻塞。

**改动**（各一行，沿用 `status-separator` 先例，F1 D6 / §11）：

1. `internal/config/settings.go` `defaultCommonSettings` map 加：
```go
    "fileselectwidth": 0.4,
```
2. `runtime/settings.json` 加：
```json
    "fileselectwidth": 0.4,
```

**验收**：`config.GetGlobalOption("fileselectwidth").(float64)` 返回 `0.4`；用户在 settings.json 改值后能读到新值。

**注意**：`diffgutter` 是复用原生设置（默认 false），**不新增**——它同时管编辑器 diff indicators 与 FileSelector git 开关。

---

### T2 · git 子模块（`fileselector_git.go`）— 独立、可单测

**目标**：实现 F1 §6.4 的 `gitStatusCache`，封装 `git status --porcelain` 调用、解析、缓存、超时降级。**与主体经接口解耦**，可独立单测、可 mock（F1 §10.6）。

**改动**：

1. **状态枚举 + 接口**（F1 §6.4 / §10.2 状态码表）：
```go
type statusKind uint8
const (
    stNone statusKind = iota
    stModified  // M
    stUntracked // U (??)
    stAdded     // A
    stDeleted   // D
    stRenamed   // R
    // I(Ignored) 延后(F1 §10.5 / R4)
)

type gitStatusCache interface {
    statusFor(dir string) (map[string]statusKind, bool)
    // false = 不可用(非仓库/超时/diffgutter关); 调用方降级为不显示
}
```

2. **实现 `*gitStatus`**（阻塞查询原语，F1 §6.4 契约；异步包裹由调用方 T3 负责）：
```go
type gitStatus struct {
    mu      sync.Mutex
    cache   map[string]map[string]statusKind  // key=dir
}

func (g *gitStatus) statusFor(dir string) (map[string]statusKind, bool) {
    // 1. diffgutter 总开关(F1 §10.4 降级链)
    if !config.GetGlobalOption("diffgutter").(bool) {
        return nil, false
    }
    // 2. 缓存命中(F1 §10.6)
    g.mu.Lock(); m, ok := g.cache[dir]; g.mu.Unlock()
    if ok { return m, true }   // 空map也是 valid 结果(目录里无变更)

    // 3. fork git(F1 §10.2): pathspec 钉死当前目录, 2s ctx(F1 §10.4)
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    cmd := exec.CommandContext(ctx, "git", "-C", dir,
        "status", "--porcelain=v1", "-z", "--", ".")
    out, err := cmd.Output()
    if err != nil {
        return nil, false   // 非仓库 / 超时 / git 不存在 → 一律静默降级
    }
    m = parsePorcelain(out)
    g.mu.Lock(); g.cache[dir] = m; g.mu.Unlock()
    return m, true
}
```
   （上面 cache 命中分支写法仅为示意，实现时理顺：命中即返回 `(m, true)`。）

3. **`parsePorcelain(out []byte) map[string]statusKind`**：按 `-z`（NUL）分隔每条记录，取前两字符 `XY`，按 F1 §10.2 表映射（`??`→U、含 `M`→M、`A `→A、`D `/` D`→D、`R `→R、全空格→跳过）；路径取第三个 NUL 之前、做 `filepath.Base()`（FileSelector 只显示当前目录条目名）。

**单测**（无需真 git）：构造几段典型 porcelain 输出字节串（含空格/中文文件名/NUL 分隔），断言 `parsePorcelain` 输出正确 map。`statusFor` 的 git 调用部分靠集成验证（T5）。

**验收**：`go test` 通过；真实仓库目录跑 `statusFor` 返回正确状态；非 git 目录返回 `(nil, false)` 不报错。

**职责边界**（F1 §10.2）：本文件**不读文件内容、不算哈希、不碰 `.git/index`**——只 `os/exec` 起 git、读 stdout、解析两字符码。git 是黑盒。

---

### T3 · FileSelector 主体（`fileselector.go`）— 最重

**目标**：State/View/Controller 三层（F1 D3）+ Open + layout + 光标定位。参照 `selectpane.go` 的同构范式。

分小步，每步可独立 review：

#### T3.1 · State 结构（F1 §5 / D3）

```go
type entry struct {
    name  string
    isDir bool
}

type fileSelectorState struct {
    currentDir string
    entries    []entry   // 已排序(目录优先+字母序)、已按 showHidden 过滤
    cursor     int       // 选中条目(0=面包屑行, 1..=条目)
    topIdx     int       // 滚动视口顶
    showHidden bool
    pickerW    int       // 截断用(内容区宽)

    // git(异步, 需并发保护, F1 §10.7)
    gitStatus map[string]statusKind
    gitOK     bool       // false=无git列(降级)
    mu        sync.RWMutex
}

func (s *fileSelectorState) setGitStatus(m map[string]statusKind, ok bool) {
    s.mu.Lock(); s.gitStatus, s.gitOK = m, ok; s.mu.Unlock()
}
func (s *fileSelectorState) gitOf(name string) (statusKind, bool) {
    s.mu.RLock(); defer s.mu.RUnlock()
    if !s.gitOK { return stNone, false }
    st, has := s.gitStatus[name]
    return st, has
}
```

#### T3.2 · layout 计算 + 预检（F1 §7）

```go
func (fs *FileSelector) computeLayout(pane *BufPane) (anchor Pos, contentSize Size, ok bool) {
    view := pane.BWindow.GetView()          // *View{X,Y,Width,Height}; 范式参照 notepane.go:238(BufPane 嵌入 BWindow, bufpane.go:207)
    avail := view.Height - 1                // 全高模型(F0 §4.1): 留 statusLine

    widthFrac := config.GetGlobalOption("fileselectwidth").(float64)
    pickerW := int(widthFrac * float64(view.Width))
    if pickerW < minWidth { pickerW = minWidth }

    // 预检(F1 §7.3 / D2): 拒开条件
    if view.Width < minWidth {
        InfoBar.Message("pane too narrow for file selector (need ", minWidth, " cols)")
        return Pos{}, Size{}, false
    }
    if avail < minHeight {
        InfoBar.Message("pane too short for file selector (need ", minHeight, " rows)")
        return Pos{}, Size{}, false
    }
    return Pos{X: view.X, Y: view.Y}, Size{W: pickerW - 2, H: avail - 2}, true
}
```
   常量：`minWidth = 20`、`minHeight = 10`（F0 §4.3，minHeight=10 保证内容区≥8 行、文件列表≥6 行可滚动）。

#### T3.3 · 换算 `FloatOpenSpec`（F1 §6.1 / §7.4）

```go
spec := FloatOpenSpec{
    Anchor:      anchor,                    // AutoExpand=false 时 = 外矩形左上角(含边框)
    ContentSize: contentSize,
    Title:       "Open File",
    FrameColor:  tcell.Style{},             // 零值 → FloatFrame 用 config.DefStyle
    Display:     fs.display,
    HandleEvent: fs.handleEvent,
    AutoExpand:  false,                     // F1 D2: 钉死 pane 左上角, 跳过 expandAnchor
    OnCancel:    func() { if fs.onSelect != nil { fs.onSelect(nil) } },  // resize 即关
}
```

#### T3.4 · View `display(area)`（F1 §9 / §7.5）

三段（内容区高 = `area.H`，由 minHeight=10 保证恒 ≥8）：

1. **行 0 面包屑**：左截断全路径、恒保留 `当前目录/`；根目录显 `/`；光标在此行时 Reverse。
2. **行 1..H-2 文件列表**：视口 `[topIdx, topIdx+visibleH)`；每行 `[标记]名[/] [git]`：
   - 标记：目录 `▸`、文件空格（预留 1 字符）。
   - 名：目录带 `/` 后缀；超长**左截断保留扩展名**（F0 §4.2，R3：v1 用"最后一个 `.` 之后"启发式，rune-safe）。
   - git：靠右单字符（`fs.state.gitOf(name)`）；`gitOK=false` 时整列隐藏。
   - 选中行 Reverse；溢出右侧画 `▲`/`▼`（对齐 SelectPane 滚动指示范式，`selectpane.go:119-134`）。
3. **末行 hint**：上下文文案（F0 §7.3，随光标位置 + showHidden 变）。

   title `"Open File"` 由 FloatFrame 嵌上边框，与面包屑（内容区行 0）相邻两行、各管各的。

#### T3.5 · Controller `handleEvent(ev)`（F1 §8.2 键位表）

```go
switch ev := event.(type) {
case *tcell.EventKey:
    switch ev.Key() {
    case tcell.KeyDown:  fs.moveCursor(+1)             // clamp [0,末], 不循环
    case tcell.KeyUp:    fs.moveCursor(-1)
    case tcell.KeyEnter:
        switch fs.cursorRowKind() {
        case rowBreadcrumb: fs.chdir(filepath.Dir(state.currentDir))  // 回上级
        case rowDir:         fs.chdir(state.entries[idx].name)         // 进目录
        case rowFile:        fs.pick(state.entries[idx].name)          // Close+onSelect(&path)
        }
    case tcell.KeyLeft:  fs.chdir(filepath.Dir(state.currentDir))      // 回上级, 不移光标
    case tcell.KeyRight: if fs.cursorIsDir() { fs.chdir(...) }         // 进目录
    case tcell.KeyTab, tcell.KeyCtrlR, ...: // '.' 切 dotfile(选可用键位)
                            state.showHidden = !state.showHidden; fs.relist()
    case tcell.KeyEscape: fs.cancel()                                   // Close+onSelect(nil)
    }
    // 其它键吞掉(modal)
}
// 注意: EventResize 不会到达这里(F1a 已在 FloatFrame 拦截)
```
   每个改 State 的分支末尾 `screen.Redraw()`；导航/重列后 `ensureVisible()`（参照 `selectpane.go:193`）+ 光标 clamp。

   **`.` 键位**：micro 里 `.` 可能已被绑定，实现时确认可用键位（备选 `Ctrl-H` 或 hint 提示的键）——这是实现细节，不阻塞架构。

#### T3.6 · `Open`（F1 §3.2 / §6.2 / §10.7 异步时序）

```go
func (fs *FileSelector) Open(pane *BufPane, startDir string, onSelect func(*string)) {
    fs.onSelect = onSelect
    fs.pane = pane

    // 1. State init(os.ReadDir, μs级, 同步) → gitStatus 空(F1 §10.7 第1步)
    fs.state = newState(startDir)         // 列目录、排序、定位当前文件(F0 §5.3)、dotfile自动显
    fs.state.gitStatus, fs.state.gitOK = nil, false

    // 2. layout 预检 + 算 spec
    anchor, contentSize, ok := fs.computeLayout(pane)
    if !ok { onSelect(nil); return }       // 预检拒开 → 透明返回(F1 §7.3)
    fs.state.pickerW = contentSize.W

    // 3. 打开(首次 Display: 列表已可见、无 git 标志, F1 §10.7 第2步)
    spec := fs.buildSpec(anchor, contentSize)
    if !TheFloatFrame.Open(spec) { onSelect(nil); return }

    // 4. 后台启 git(渐进显示, F1 §10.7 第3-5步) —— 绝不阻塞首次渲染
    go func() {
        m, ok := fs.gitCache.statusFor(fs.state.currentDir)   // 阻塞查询, 带 2s ctx
        fs.state.setGitStatus(m, ok)                          // RWMutex 保护(F1 §10.7 留待项)
        screen.Redraw()                                       // 通知主循环重绘 → 列表补 git 标志
    }()
}
```

   **gitCache 注入**：FileSelector 持 `gitCache gitStatusCache` 字段，默认 `&gitStatus{cache: map[string]...}`；测试/mock 时替换。这兑现 F1 §10.6"接口注入便于 mock"。

#### T3.7 · 光标起始定位（F1 §8.4 / F0 §5.3）

`newState` 里：在条目中找当前 buffer 文件名（`filepath.Base(pane.Buf.AbsPath)`）；找到→停其上；当前文件是 dotfile 且默认隐藏→`showHidden=true` 后停其上；找不到/无路径→首条目。

#### T3.8 · 并发安全（F1 §10.7 留待项）

后台 goroutine 写 `gitStatus`、主循环 Display 读——靠 `fileSelectorState.mu sync.RWMutex`（T3.1 已含 `setGitStatus`/`gitOf`）。Display 读 git 时一律走 `gitOf()`（RLock），绝不裸读 map。

**验收（T3 整体）**：能 `Open`、列目录、↑↓ 浏览滚动、Enter 进目录/选中、←/→ 导航、`.` 切 dotfile、Esc 关闭、当前文件定位。

---

### T4 · 命令注册（`command_neo.go`）— 串联

**目标**：注册 `:file`，实现 `FileCmd`（范式照搬 `ThemeCmd`，`command_neo.go:19`）。依赖 T3 的 `FileSelector.Open`。

**改动**：

1. `InitNeoCommands()` 加一行（与 `:theme` 同通道，零侵入）：
```go
    MakeCommand("file", (*BufPane).FileCmd, nil)
```

2. `FileCmd`（handler 签名 `func(h *BufPane) FileCmd(args []string)`，对齐 ThemeCmd）：
```go
func (h *BufPane) FileCmd(args []string) {
    // 1. modified 保护(F0 §8 / F1 §8.1 / R5): n=取消, 不同于 :open 的 n=不保存继续
    if h.Buf.Modified() && !h.Buf.Shared() {
        // YNPrompt 回调签名是 func(yes, canceled bool)(infobuffer.go:123), 两参数
        InfoBar.YNPrompt("Buffer modified. Save? (y,n,esc)", func(yes, canceled bool) {
            if !yes { return }                      // n / Esc → 取消(不继续), 这正是 R5 与 :open 的区别
            if h.SaveAll() { openSelector() }       // y → 先保存(h.SaveAll 存在, actions.go:957) 成功才继续
        })
        return                                      // YNPrompt 异步: 立即返回, 等回调
    }
    openSelector()
}

func (h *BufPane) openSelector() {
    // 2. 起始目录(F1 §8.1 / R6)
    startDir := filepath.Dir(h.Buf.AbsPath)
    if startDir == "." || h.Buf.AbsPath == "" {
        startDir, _ = os.Getwd()
    }

    // 3. 打开(onSelect 闭包: 选中→开进发起 pane, 与 :open 同路径 command.go:304)
    NewFileSelector().Open(h, startDir, func(picked *string) {
        if picked == nil { return }                 // Esc / resize / 拒开
        // pane 在打开期间被关的防御(R7):
        if h.Buf == nil { return }
        b, err := buffer.NewBufferFromFile(*picked, buffer.BTDefault)  // 签名参照 OpenCmd
        if err != nil { InfoBar.Error(err); return }
        h.OpenBuffer(b)                             // 开进发起 :file 的那个 pane
    })
}
```

   **关键差异（R5）**：`:open` 的 modified 保护用原生 `closePrompt`（`actions.go:1916`），其 n=不保存也继续；`:file` 的 n=**取消**。**勿直接套 `closePrompt`**，自己写 YNPrompt 回调、文案明确（F0 §8）。

**验收**：`:file` 触发 FileSelector；选中文件开进当前 pane；modified 时弹 y/n、n 取消。

---

## 4. 提交策略与并行

- **T1 独立提交**（配置，最先、解阻塞）。
- **T2 独立提交**（git 子模块 + 单测，可与 T3 并行开发）。
- **T3 拆 2–3 提交**（建议：State+layout+Open / View display / Controller handleEvent），或一个主体提交。
- **T4 提交**（串联，最后）。
- **并行**：T2（git 子模块）与 T3（主体）经 `gitStatusCache` 接口解耦——可两人并行，主体先用 mock cache 跑通，git 子模块就位后换真实实现。

---

## 5. 验证与集成回归清单

`make build`（完整）后逐项手测：

**基础闭环**
- [ ] `:file` 打开，列表显示当前目录（目录优先 + 字母序）。
- [ ] **pane-local**：picker 左上角对齐 pane、不溢出到相邻 pane（hsplit/vsplit/tab 各试）。
- [ ] ↑↓ 浏览、滚动指示 `▲`/`▼` 出现。
- [ ] Enter（文件行）→ 选中文件开进发起 pane（与 `:open` 同结果）。
- [ ] Enter（目录行）/ → → 进子目录；Enter（面包屑行）/ ← → 回上级。
- [ ] `.` 切 dotfile 显隐，光标 clamp。
- [ ] Esc 关闭、回到编辑器。
- [ ] 当前 buffer 文件在打开时光标停其上（dotfile 文件自动显隐后停其上）。

**git 状态（渐进 + 降级）**
- [ ] **渐进显示**：打开瞬间列表无 git 标志、可立即浏览；git 回来后列表补上 M/U 等标志（光标/滚动不受影响）。
- [ ] **diffgutter=true** 才显示 git 列；`diffgutter=false` 整列隐藏。
- [ ] **非 git 目录**打开 → 无 git 列、不报错。
- [ ] **2s 超时**（构造慢场景 / 巨型目录）→ 列表早已显示、git 列始终不出现（静默降级，不干等）。

**边界与保护**
- [ ] **极窄 pane**（W<20）/ **极矮 pane**（avail<10）→ 拒开 + InfoBar 提示。
- [ ] **modified buffer** → y/n 提示；y=保存后继续、n=取消（≠ `:open` 的 n）。
- [ ] **resize** → picker 关闭（继承 F1a），无残留。
- [ ] **picker 开着时关 tab**（R7）→ 不崩溃（`h.Buf==nil` 守卫）。
- [ ] `:file` 关闭后能再次正常打开（无状态泄漏）。

---

## 6. 风险与注意

| # | 项 | 应对 |
|---|----|------|
| 1 | **并发：git goroutine vs Display 读** | `fileSelectorState.mu sync.RWMutex`；Display 一律走 `gitOf()`（RLock），绝不裸读 map（T3.8） |
| 2 | **modified 保护语义差异（R5）** | `:file` 的 n=取消，**勿套 `closePrompt`**；自写 YNPrompt 回调 |
| 3 | **`NewBufferFromFile` / `OpenBuffer` 签名** | 实现时核对 `OpenCmd`（`command.go:304`）用法，照搬 |
| 4 | **pane 打开期间被关（R7）** | onSelect 闭包判 `h.Buf != nil`（参照 `notepane.go` reposition 守卫） |
| 5 | **左截断保留扩展名（R3）** | v1 用"最后一个 `.` 之后"启发式 + rune-safe；双宽/组合字符边界留详细设计 |
| 6 | **`.` 键位冲突** | micro 可能已绑 `.`；实现时确认可用键，备选 `Ctrl-H` |
| 7 | **起始目录无路径（R6）** | `AbsPath==""` / `Dir=="."` → fallback `os.Getwd()` |
| 8 | **borderless vs bordered（R1）** | 架构按"复用 FloatFrame 标准边框"；视觉细节与产品确认，可配置收敛 |
| 9 | **git 慢拖体验** | 已由异步渐进 + 2s 超时 + 会话内不重试（F1 §10.4）兜底 |

---

## 7. 工时估算

**3–5 天**：

| 任务 | 估算 |
|------|------|
| T1 配置注入 | 0.5 h |
| T2 git 子模块 + 单测 | 1 天 |
| T3 主体（State/layout/View/Controller/Open/光标/并发） | 2–2.5 天 |
| T4 命令注册 + modified 保护 | 0.5 天 |
| T5 集成验证（§5 清单） | 0.5–1 天 |

T3 是关键路径；T2 可并行前置。

---

## 附：与 F1 的对应

| 本方案任务 | F1 章节 |
|-----------|--------|
| T1 配置 | F1 §11 / D6 |
| T2 git 子模块 | F1 §6.4 / §10（10.2 机制 / 10.4 降级 / 10.6 缓存 / 10.7 时序） |
| T3.1 State | F1 §5 / D3 |
| T3.2 layout | F1 §7（7.1–7.4） |
| T3.3 FloatOpenSpec | F1 §6.1 / §7.4 |
| T3.4 View | F1 §9 / §7.5 |
| T3.5 Controller | F1 §8.2 |
| T3.6 Open（异步 git） | F1 §3.2 / §10.7 |
| T3.7 光标起始 | F1 §8.4 |
| T4 命令注册 | F1 §8.1 / D6 |
| §5 回归 / §6 风险 | F1 §13 / §14.1 |
