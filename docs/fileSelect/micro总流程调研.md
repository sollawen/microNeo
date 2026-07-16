# microNeo 事件分发

完整描述键盘/鼠标/粘贴/resize 等事件从终端到处理器的路径，以及各 modal 层如何从中插入自己的优先级。

本文档对应代码：事件经过 `cmd/micro/micro.go` 的 main loop → 各 `action.*HandleEvent`。所有行号基于当前 `main` 分支。

---

## 主流程伪代码

主循环里跑的事其实就这几步：

```text
# 主循环：每一帧
for {
    DoEvent()
}

# DoEvent 一帧四步
func DoEvent():
    Render()                # 1. 画一帧
    event = WaitEvent()     # 2. select 等事件
    if event is ErrorEvent: # 3. 错误处理
        handleError(event)
    Dispatch(event)         # 4. 分发
    RunPlugin("onAnyEvent", event)  # 4.5 插件（看到的是已路由过的事件）


# 渲染：从底到顶依次画
func Render():
    Fill(' ')                         # 用空格刷整屏 = 清屏（全量重绘，无脏区域）
    Tabs.Display()                    # 顶部 tab bar（多 tab 时才有）
    for pane in MainTab().Panes:      # 每个 pane 内部画 buffer + 自己的 statusLine
        pane.Display()
    MainTab().Display()               # split divider（只画 VSplit 的 |，HSplit 不需要）
    InfoBar.Display()                 # 底部 info bar
    if NotePane.IsOpen():
        NotePane.Display()            # 次浮
    if FloatFrame.IsOpen():
        FloatFrame.Display()          # 最浮
    Show()


# 分发：resize 例外，其他按优先级
func Dispatch(event):
    if event is ResizeEvent:
        # broadcast 四方，不走优先级
        InfoBar.HandleEvent(event)
        Tabs.HandleEvent(event)
        if NotePane.IsOpen():   NotePane.HandleEvent(event)
        if FloatFrame.IsOpen(): FloatFrame.HandleEvent(event)
        return

    # 非 resize：从上到下匹配，第一个赢
    if FloatFrame.IsOpen():
        FloatFrame.HandleEvent(event)
    else if NotePane.IsOpen():
        NotePane.HandleEvent(event)
    else if InfoBar.HasPrompt:
        InfoBar.HandleEvent(event)
    else:
        # 默认落到 Tabs，内部还会转发：
        #   TabList → 当前 active Tab → 当前 active BufPane
        Tabs.HandleEvent(event)
```

读这段时只要抓住三个点：

- **每帧**：渲染 → 等事件 → 处理 → 分发 → 插件
- **渲染顺序决定视觉层叠**：最后画的在最上面
- **分发优先级**：FloatFrame > NotePane > InfoBar prompt > Tabs

下面各节就是把这三个点拆开讲细节。

---

## 一、三层结构

事件从终端到达处理器，走过三层：

| 层 | 位置 | 角色 |
|---|---|---|
| **采集** | `micro.go:488-496`（独立 goroutine） | 从 tcell 拉事件，塞进 channel |
| **缓冲** | `micro.go:498-602`（`DoEvent` 的 `select`） | 等任意事件 / 信号 / 重绘 |
| **分发** | `micro.go:580-594`（priority chain） | 路由到一个 handler |

每层只关心自己的事，互不越界：

- 采集层不过滤事件类型——键盘、鼠标、粘贴、resize 都进同一个 channel
- 缓冲层不知道事件是什么——只是 `select` 等着，配上 jobs / autosave / close-terms / drawChan / timer / signals 几个通道
- 分发层按优先级匹配 open 的 modal，谁 open 谁先接手

---

## 二、数据层级：Tabs / Tab / Pane

事件在三层结构（采集/缓冲/分发）里流动，但承载事件的**对象本身**有另一套层级关系。理解这个层级有助于看懂 dispatch 链为什么是 `Tabs → Tab → BufPane` 三层转发。

```
action.Tabs           (TabList，全局唯一)
  │  List: []*Tab      平级 slice，tab 之间不嵌套
  ├── Tab "a.md"
  ├── Tab "b.md"        ← 当前 active = 这个
  └── Tab "c.md"
       │
       └── Tab 内部结构：
            │
            ├── Panes: []Pane            ← 平级 slice
            │   ├── Pane[0]  (active)
            │   └── Pane[1]
            │
            └── *views.Node (split tree)  ← pane 排版关系
                 │
                 ├── STVert (左右分)
                 │   ├── leaf → Pane[0]
                 │   └── leaf → Pane[1]
                 │
                 └── 这棵树决定 pane 在屏幕上的布局位置
```

两个关键点：

1. **`Tabs.List` 和 `Tab.Panes` 都是平级 slice**——同层之间不嵌套。Tab 之间是「同时打开的多个文档」的关系；Pane 之间是「同一个 tab 内的分屏」。

2. **Pane 的位置由 split tree 决定**，不由 `Panes` 数组顺序决定。想知道 pane 是左右分还是上下分，看 `Pane.GetView()` 返回的坐标——`Panes[0]` 在左就是 VSplit，在上就是 HSplit。

对应代码：

| 结构 | 位置 |
|---|---|
| `TabList` struct | `tab.go:17-20` |
| `MainTab()` 返回当前 active tab | `tab.go:223-225` |
| `Tab` struct（含 `Panes []Pane` 和 `*views.Node`） | `tab.go:231-243` |
| `views.Node`（split tree 节点类型） | `views/splits.go` |

### 父子链在 dispatch 中的体现

事件路由沿父子链向下：

```
Tabs.List[Active] (Tab)
  └─ Tab.Panes[active] (BufPane)  ← 事件最终落点
```

`Tabs.HandleEvent` 自己先处理 tab bar 鼠标，然后转发给当前 active Tab；`Tab.HandleEvent` 自己先处理 split resize / pane 切换 / wheel，然后转发给当前 active BufPane。详见 §六.4。

---

## 三、采集层：独立 goroutine

事件采集跑在独立 goroutine 里（`micro.go:488-496`），主循环不直接 poll 终端。goroutine 做的事很机械：

1. 拿 tcell 的 mutex（`screen.Lock()`，渲染时不能并发读）
2. `PollEvent()` 拉一个事件
3. 释放 mutex
4. 把事件塞进 `screen.Events` channel

```go
go func() {
    for {
        screen.Lock()
        e := screen.Screen.PollEvent()
        screen.Unlock()
        if e != nil {
            screen.Events <- e
        }
    }
}()
```

要点：

- 不区分事件类型——所有 `tcell.Event` 都走同一个 channel
- `screen.Events` 本身是无缓冲的，但因为 send 之前已经 unlock，所以不会跟渲染互锁
- goroutine 停不下来——terminal EOF 事件（`io.EOF`）由 main loop 的 `EventError` 处理（见 §4.3），goroutine 本身不退出

---

## 四、main loop

`micro.go:494` 是整个 editor 的心跳：

```go
for {
    DoEvent()
}
```

`DoEvent()` 每帧做四件事：渲染 → 等事件 → 处理错误 → 分发。

### 4.1 渲染顺序

先 `Fill(' ')` 清整屏，再依次画各个组件。顺序很重要，决定了**视觉上的上下层叠**：

1. `Fill(' ')` 清屏
2. `Tabs.Display()` — 顶部 tab bar（多 tab 时才显示）
3. `for ep in MainTab().Panes: ep.Display()` — 每个 pane 内部画 buffer + 自己的 statusLine
4. `MainTab().Display()` — split divider，**只画 VSplit 的 `|`**。HSplit 不画 `-`：因为每个 pane 自带 statusLine，上 pane 的 statusLine 已经天然把上下分开了，再画一根 `-` 是重复（`uiwindow.go:41` 只有 `STVert` 分支，不是漏写，是故意）
5. `InfoBar.Display()` — 底部 info bar
6. 如果 `NotePane` 开着，画它（次浮）
7. 如果 `FloatFrame` 开着，画它（最浮）
8. `Show()` flush 到终端

**最后画的在最上面**。FloatFrame 是最浮的层（modal 的视觉一致性要求），NotePane 次之，InfoBar 在底。

对应代码（`micro.go:511-530`）保持这个顺序，没插队。

### 4.2 等事件

渲染完进入 `select` 阻塞，等任意一个 channel 触发：

- `shell.Jobs` — 后台 shell 命令完成，跑回调
- `config.Autosave` — autosave 定时器到点，所有 buffer 跑一次 AutoSave
- `shell.CloseTerms` — shell 通知关终端（清理）
- `screen.Events` — 键盘 / 鼠标 / 粘贴 / resize 等用户/终端事件
- `screen.DrawChan()` — 外部请求重绘（drain 掉所有待处理的重绘请求）
- `timerChan` — 通用定时器队列
- `sighup` / `util.Sigterm` — 退出信号

空闲时整个循环就停在这里等，**一帧都不画**——这是事件驱动渲染的核心。

对应代码：`micro.go:532-548` 的 `select` 块。

### 4.3 错误处理

`DoEvent` 拿到事件后先做一次类型检查（`micro.go:550-557`）：如果是 `*tcell.EventError`，看错误原因：

- `io.EOF` — 终端被关了（SSH 断、X 窗口关），直接 `exit(0)`
- 其他 tcell 内部错误 — `log.Println` 记一下，继续

`EventError` 只在终端异常时产生，正常键盘/鼠标不会走这条。

### 4.4 分发

拿到正常事件后，进入分发层。逻辑见下一节。

---

## 五、分发层：priority chain

分发逻辑就两条规则：

**resize 例外**：广播四方，不走优先级。resize 是 terminal 状态变化（不是用户输入），每层都要重新布局。

**其他事件**：从上到下 match `IsOpen()`，第一个赢；都不 match 才落到 `Tabs` 默认路径。

代码（`micro.go:580-594`）：

```go
if resize {
    // broadcast: InfoBar, Tabs, NotePane?, FloatFrame?
} else if TheFloatFrame.IsOpen() {
    TheFloatFrame.HandleEvent(event)
} else if TheNotePane.IsOpen() {
    TheNotePane.HandleEvent(event)
} else if InfoBar.HasPrompt {
    InfoBar.HandleEvent(event)
} else {
    Tabs.HandleEvent(event)
}
```

四条优先级（从高到低）：

1. `TheFloatFrame` — 最浮的 modal
2. `TheNotePane` — 次浮
3. `InfoBar.HasPrompt` — 底部 prompt（命令行/搜索/y/n 确认/保存文件名）
4. `Tabs` — 默认。内部还要转发两层（TabList → Tab → BufPane）才到具体 pane

### 调度 vs 渲染顺序

有意思的是，**dispatch 顺序和渲染顺序大体一致，但不严格**：

| 调度优先级 | 渲染顺序 | 层级 |
|---|---|---|
| 最高 | 最后画（最上） | `TheFloatFrame` |
| 次高 | 倒数第二 | `TheNotePane` |
| 第三 | 第一（底部） | `InfoBar.HasPrompt` |
| 最低（默认） | 中间（编辑主体） | `Tabs` |

不一致处是 InfoBar：渲染在最底，dispatch 在 NotePane 之后——这是合理的，因为 InfoBar 是底部状态栏，prompt 弹起时只是临时接管输入，不抢 NotePane/FloatFrame 的优先级。

### Plugin hook

`micro.go:597` 在分发后跑：

```go
err := config.RunPluginFn("onAnyEvent")
```

**重要**：Lua 插件看到的是「已被 modal 吞掉或路由完的事件」，**不是 raw event**。如果你在 plugin 里期待「还没被处理的事件」，**那条时机不存在**——要么自己改 `DoEvent` 加 hook，要么在 handler 里写 log。

---

## 六、各 handler 内部

### 6.1 FloatFrame（`internal/action/floatframe.go:217-225`）

FloatFrame 是**最薄的 modal 容器**，自己不识别任何事件类型——纯 callback。

收到事件时分两条路：

- **resize**：自己处理。先 `Close()` 关容器，再触发 `onResize()` 业务回调；**不**转发给 widget。设计契约是：resize 对 FloatFrame 而言等价于「容器关了」，widget 应进入重置态。
- **其他事件**：直接丢给 widget 的 `handleEvent` 回调，由 widget 自己在回调里识别 `*tcell.EventKey` / `*tcell.EventMouse`。

要点：**不要假设 FloatFrame 自动支持 mouse/key**。要支持，由 widget 自己在 `handleEvent` 里识别事件类型并查 binding。

### 6.2 NotePane（`internal/action/notepane.go`）

跟 FloatFrame 完全不同的路子——NotePane 是**自管 modal**，不走 FloatFrame 框架：

- 自己注册完整的 mouse bindings（`MousePress / MouseDrag / MouseRelease / MouseMultiCursor`，`notepane.go:112-114`）
- 鼠标事件通过 `BufMouseActionGeneral(bufAction)` 复用 BufPane 的鼠标框架（`notepane.go:397-398`）
- 自带 `Display()` 自绘 border（border 上还能嵌入 receiver 名字）

**NotePane 和 FloatFrame 不是同一套机制**——它和 BufPane 是同一家族。设计新 widget 时想清楚走哪条路：

- 简单一次性交互（SelectPane、输入框、提示框）→ FloatFrame
- 复杂自管 UI（要 mouse、要自己画 border、要复用 BufPane 鼠标框架）→ 直接仿 NotePane，不走 FloatFrame

### 6.3 InfoBar.HasPrompt（`internal/info/infobuffer.go`）

底部状态栏的 prompt 模式。`HasPrompt` 是个 bool flag——prompt 期间 InfoBar 接管输入：

- 命令行输入（`:` 起）
- 搜索
- y/n 确认
- 保存文件名

HasPrompt 期间，下面的 Tabs 完全接收不到事件——和 FloatFrame / NotePane 同类的 modal 语义。

### 6.4 Tabs → Tab → BufPane（默认路径）

`Tabs.HandleEvent(event)` 不是直接到 BufPane，中间还要过两层（tab.go:102 / tab.go:276）：

1. **TabList.HandleEvent**（`tab.go:102`）自己吞掉 tab bar 的鼠标事件（点击切 tab、滚轮滚 tab bar），其他转发给当前 active Tab
2. **Tab.HandleEvent**（`tab.go:276`）自己吞掉 split 拖动、点击 pane 切换 active、wheel 转发到鼠标下的 pane，键盘事件全部转发给当前 active BufPane
3. **BufPane.HandleEvent**（`bufpane.go`）最终处理：键盘查 binding，鼠标查 BufMouseAction

**三层转发链**：

```
Tabs.HandleEvent
  ↓ 不是 tab bar 鼠标，转发
TabList.List[Active].HandleEvent
  ↓ 不是 split resize / pane 切换 / wheel，转发
Tab.Panes[active].HandleEvent
  ↓ 终于到这
BufPane.HandleEvent
```

**BufPane 完整键盘 + 鼠标处理**：

- `bufpane.go:476-506` 把 `*tcell.EventMouse` 转成 `MouseEvent`（区分 press/drag/release）
- `bufpane.go:605+` 的 `DoMouseEvent` 查 binding → 调 `BufMouseAction`
- `bufpane.go:25` 的 `BufMouseAction func(*BufPane, *tcell.EventMouse) bool` 是回调签名

如果想给某个 widget 加上鼠标，参考这套 BufMouseAction 框架。

---

## 七、容易踩坑的事实

1. **Modal 是真 modal** —— FloatFrame / NotePane / InfoBar.HasPrompt 期间，编辑器收不到事件。Widget 不处理的 event **就被吞掉**，**不会漏出到下层**。

2. **resize 例外** —— 是 broadcast 不是 priority。每层都要重新布局。FloatFrame **不会转发 resize 给 widget**，而是关容器 + 触发 `onResize()`。Widget 想响应 resize 必须在 `handleEvent` 里自己检查 `*tcell.EventResize`（不过当 FloatFrame 收到 resize 时它已经先关了，事件到不了 widget——除非 widget 自己管 modal 不用 FloatFrame）。

3. **FloatFrame 不识别事件类型** —— 是 callback 容器。要支持 mouse/key，由 widget 自己在 `handleEvent` 回调里识别类型。

4. **FileSelector 当前不响应 mouse** —— `grep -n Mouse fileselector.go` 0 匹配。只支持键盘。要加 mouse 处理，需在 widget 回调里识别 `*tcell.EventMouse` 并查 binding（或参考 NotePane 用 BufMouseActionGeneral 框架）。

5. **NotePane 是 BufPane 家族，不是 FloatFrame 家族** —— 设计 fileManager 时**不要假设 FloatFrame 内部会自动支持 mouse**。它不会。mouse 处理要 widget 自己写。

6. **Plugin 在分发后跑** —— Lua 插件 `onAnyEvent` 看到的是处理过的 event，不是 raw。如果你需要「还没人处理过」的信号，那条时机不存在。

7. **screen.Events 是无缓冲 channel** —— 但 `screen.Lock()/Unlock()` 把读取和发送分开，所以不会和渲染互锁。DoEvent 卡在 `select` 等事件，事件 goroutine 卡在 send 等渲染完成。

---

## 八、microNeo vs stock micro

stock micro 只有一层 dispatch：直接到 Tabs。microNeo 插入了三层 modal：

| 层级 | stock micro | microNeo |
|---|---|---|
| 渲染顺序最后 | — | `TheFloatFrame.Display()`（l.530-532） |
| dispatch 优先级最高 | — | `if TheFloatFrame.IsOpen()`（l.587） |
| 渲染顺序倒数第二 | — | `TheNotePane.Display()`（l.528-530） |
| dispatch 优先级次高 | — | `else if TheNotePane.IsOpen()`（l.589） |
| 底部 prompt | 已存在 | 同 stock，但 chain 位置不同 |

加上 `InfoBar.HasPrompt` 一层（stock 也有）和 `Tabs` 默认层，就是 microNeo 的完整四层 priority chain。

另外 `micro.go:486-490` 在启动后等第一个 resize、然后调 `OpenBirthSelector`——给刚启动的 noName pane 自动开 birth selector。**这条只在启动时跑一次**，不进 `for { DoEvent() }` 主循环。

---

## 九、后续扩展点

如果 fileManager 转自管 modal（参照 NotePane 路径），接入点只需两处：

**渲染顺序**：在 `micro.go:528` 附近、NotePane 后 / FloatFrame 前插一行。

**dispatch chain**：在 `micro.go:580` 附近的 priority chain 中、FloatFrame 和 NotePane 之间或之后——按视觉层级决定（视觉更浮的排前面）。

具体写法直接照抄 NotePane 那两行即可：

```go
// 渲染
if action.TheFileManager != nil && action.TheFileManager.IsOpen() {
    action.TheFileManager.Display()
}

// dispatch
else if action.TheFileManager != nil && action.TheFileManager.IsOpen() {
    action.TheFileManager.HandleEvent(event)
}
```

不需要重构现有 chain。NotePane 已经走过这条路了，照抄即可。

如果未来加更多 modal 层级（如同时打开 NotePane + FileManager），按视觉层级排在合适位置即可：

| 视觉层级 | dispatch 顺序 | 渲染顺序 |
|---|---|---|
| 最浮 | 最先匹配 | 最后画 |
| 较深 | 后匹配 | 先画 |
| 嵌入编辑器 | 不进 chain（依赖编辑器自身） | 不画 |

---

## 十、调试技巧

- `make build-dbg` 编译 debug 版本，事件进入 `DoEvent` 时会写 log
- log 文件：`/tmp/microNeo_debug.log`
- 看某事件被谁处理：在 handler 入口加 log，匹配 dispatch chain 即可
- 调试 plugin 时记住：`onAnyEvent` 拿到的不是 raw event；要看分发前的状态，要么改 DoEvent 加 hook，要么在 handler 里加 log