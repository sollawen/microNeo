# F10c · 访问历史 · Step 2 实施计划

**性质**：实施计划。Step 1 已按 `F10b-底盘重构-step1-实施计划.md` 完成；本文以当前代码为基线，实现 `F10a-访问历史-讨论稿.md` 的 Step 2。

**范围**：一次完成访问历史的记录、持久化、读取、history region、布局、焦点与跳转，交付后端到端可用。本文完全取代 `访问历史-实现方案.md` 中的记录端方案：记录点从 `buffer.NewBufferFromFileWithCommand` 改为 `fileList.activate()`（见 §2.2），只负责 finder 内的访问行为。

**验收标准**：在 finder 中选中文件并按 Enter 后，当前目录进入 `$ConfigDir/history.json`；再次打开 finder，在高度足够时可看到最近目录，通过键盘或鼠标选择并切换 fileList 当前目录。原 fileList / preview 行为在 history 不显示时保持不变。

---

## 1. 当前基线与复用结论

### 1.1 Step 1 已落地的底盘

当前 `internal/finder/` 已有：

- `NoKeyboardRegion`：`Display`、`HandleMouse`、`Rect`
- `KeyboardRegion`：在前者上增加 `HandleKey`、`FocusOn`、`FocusLost`
- `Session.regions`：一般渲染与 mouse dispatch
- `Session.keyboardRegions`：键盘路由与 focus 的独立索引空间
- `hitTest`、`cycleFocus`、`switchFocus`、FFM 已实现
- `fileList.chdirTo(target, focusName)` 已完整实现目录校验、列表重载、preview 同步、重绘与 git 刷新

因此 Step 2 不新增事件总线、不新增 focus manager、不新增目录切换函数。history 直接实现既有 `KeyboardRegion`，激活后复用 `fileList.chdirTo`。

### 1.2 证伪式搜索结论

已搜索：

- `RecordDirHistory`、`DirHistory`、`history.json`：代码中尚无访问历史实现。
- `recent`、`openhistory`、`filehistory`：无可复用的文件/目录访问历史。
- `internal/info/history.go`：保存的是 InfoBar 命令历史，数据格式、生命周期和职责均不兼容。
- finder 的目录切换与滚动：`fileList.chdirTo`、`ensureVisible` 可分别复用行为与模式。
- 配置持久化：`config.ConfigDir` 继续作为统一配置目录来源，但访问历史的读写实现归属 finder，不放进 config package。

结论：访问历史的生产代码集中在 `internal/finder/history.go`，同时包含数据读写与 history region；其余接入现有机制。

---

## 2. 最终行为契约

### 2.1 数据

`$ConfigDir/history.json` 使用纯字符串数组：

```json
[
    "/home/user/project/internal/finder",
    "/home/user/project/docs"
]
```

规则：

- 元素是目录绝对路径。
- 队首最新。
- 同一路径去重；再次访问时移到队首。
- 最多保存 50 条。
- 文件不存在时读取结果视为空；空文件或 JSON 损坏时删除 `history.json` 后视为空，使后续记录可以从干净文件重新写入。清理失败也不得阻止 finder 打开。
- finder 打开时不对历史路径执行 `os.Stat`，避免 50 条路径或网络挂载拖慢 Open；路径有效性只在用户激活单条记录时检查。

### 2.2 记录时机

访问历史只描述 finder 内的文件选择行为，与 buffer、命令行及其他打开文件入口无关。

唯一挂钩点为 `fileList.activate()` 的文件分支：

- breadcrumb 与目录分支仍只负责切换目录，不记录。
- 光标确认落在文件条目后，先对 `l.currentDir` 执行 `os.Stat`，确认路径存在且仍是目录；检查失败则直接返回，不记录也不关闭 finder。
- stat 确认成功后，调用 `recordDirHistory(l.currentDir)`，再调用 `l.fm.closePicked(fileName)`。
- 传入值直接是 finder 当前目录；记录函数不接收文件路径，也不再调用 `filepath.Dir`。
- 通过命令行、`open`、`tabopen`、split、Lua 等 finder 之外的方式打开文件，一律与本功能无关。
- 仅在 finder 确认文件且当前目录仍有效时记录；只浏览目录、取消 finder、从 history 切换目录均不记录。

### 2.3 显示条件与高度

**布局计算全部由 `Session.Open` 负责。** `historyList` 不参与布局协商，也不计算其他 region 的大小；它只接收 Session 计算好的 `historyRect`，并在该矩形内绘制和处理交互。

仅当下列条件同时满足时，Session 才构造 history region：

1. finder 入口 `rect.H >= 15`；
2. history 原始列表非空。

显示时 history 占左栏底部 4 行：

- 1 行由 session 绘制的上分隔线；
- 3 行 history 内容；
- 下边界复用 finder 原底边，不额外占行。

history 不显示时，Step 1 的 `listRect` 几何与渲染必须一字不变。

以下公式由 `Session.Open` 在构造 region 前执行。Session 先按 Step 1 现有代码计算 `listRect`，再在需要 history 时从这个 `listRect` 的底部切出 history 区域：

```text
historyContentH = 3
historyBlockH   = 1 + historyContentH
historyRect     = Rect{
    X: listRect.X,
    Y: listRect.Y + listRect.H - historyContentH,
    W: listRect.W,
    H: historyContentH,
}
historyY = historyRect.Y - 1
listRect.H -= historyBlockH
```

Session 随后使用缩短后的 `listRect` 构造 fileList，并将 `historyRect` 注入 `newHistoryList`。`historyList` 不知道 finder 总高度、status line、preview 或 fileList 的原始高度，也不负责决定自己是否构造。

输入高度 `rect.H == 15` 时：原 `listRect.H == 12`，切分后 `fileList.rect.H == 8`，其 `listH == 6`，符合 F10a 的最低可用布局。

### 2.4 history 交互

history 内部维护：

- `dirs []string`
- `cursor int`，使用 0-based 索引
- `topIdx int`
- `focused bool`
- 固定可见行数取 `rect.H`，本期为 3

键盘：

- Up / Down：光标 ±1，边界 clamp，不跨 region。
- Enter / Right：对当前路径完成 stat 后，调用 `h.fm.ActivateFromHistory(dir)`，由 Session 负责完成 `chdirTo` 并切回 fileList focus。
- 其他键返回 `false`，让 Esc / Ctrl-Q / `q` 继续走 session 全局路由。
- Tab / Shift-Tab 仍由 session 拦截，在 fileList 与 history 间轮转；preview 不在 `keyboardRegions` 中。

鼠标：

- WheelUp / WheelDown：光标 ±1并确保可见。
- Button1：按点击行更新 cursor，但不切目录。
- 纯 mouse move（`Buttons() == 0`）：history 不做任何处理，`HandleMouse` 直接返回 `false`，不移动 cursor、不修改 `topIdx`、不重绘。
- 分隔线不属于任何 region，`hitTest` 返回 -1。

focus：

- `FocusOn` 设置 `focused=true` 并重绘。
- `FocusLost` 设置 `focused=false` 并重绘。
- 只有 focused region 的当前行使用 reverse 样式；失焦 region 保留 cursor 位置但不反白。
- finder 初始 focus 仍是 fileList。
- Enter / Right 从 history 选择有效目录后，调用 `fileList.chdirTo` 完成目录切换，并将 focus 切换给 fileList；无效目录只从 history 中删除，不切换目录。

---

## 3. 改动步骤

### 3.1 新增 `internal/finder/history.go`

访问历史属于 finder 功能，读写实现放在现有 `finder` package 中。该文件使用 `config.ConfigDir` 定位配置目录，但不把 finder 专属能力放进 config package。

提供两个 package 内部函数，不导出 finder 专属细节：

```go
func readDirHistory() []string
func recordDirHistory(dir string)
```

实现要求：

1. 文件路径统一为 `filepath.Join(ConfigDir, "history.json")`。
2. `readDirHistory` 读取并反序列化 `[]string`：
   - 文件不存在时返回空 slice；
   - 文件为空或 `json.Unmarshal` 失败时，调用 `os.Remove` 删除损坏的 `history.json`，然后返回空 slice；
   - 其他读取错误返回空 slice，不删除文件；
   - 删除失败不向上阻断 finder。
3. 返回新 slice，不把可变的包内状态暴露给调用方。
4. 提取包内 `writeDirHistory(dirs []string)`，统一给记录新增和失效条目删除复用，负责 `MarshalIndent + WriteFile`。
5. `recordDirHistory`：
   - 空路径直接返回；
   - `dir = filepath.Clean(dir)`；
   - 读取现有列表；
   - 删除等于 `dir` 的旧项；
   - 把 `dir` 放到队首；
   - 截断为 50；
   - 调 `writeDirHistory` 整体写回。
6. 不新增配置项、不新增 package、不新增退出钩子。
7. 写入失败静默降级；本功能不能阻止 finder 选中文件和关闭。

这里优先采用“每次读文件”的无缓存实现，而不是旧方案中的 `recentDirs + loaded` 包级缓存：

- finder 每次 Open 都应看到磁盘上的最新内容；
- 无需增加 cache reset、并发同步和测试隔离状态；
- 最多 50 条，整体读写成本可忽略。

I/O 同步权衡：`recordDirHistory` 与 `removeCurrent` 中的 `writeDirHistory` 都在主线程同步调用，50 条记录在普通 SSD 上仅微秒级；若后续在慢盘或网络盘上实测出可感知延迟，再考虑后台 goroutine 写入，但本期不引入该复杂度。

### 3.2 修改 `internal/finder/filelist.go`

在 `fileList.activate()` 已确认 cursor 对应文件条目后，先校验当前目录，再记录并关闭：

```go
info, err := os.Stat(l.currentDir)
if err != nil || !info.IsDir() {
    return
}
recordDirHistory(l.currentDir)
l.fm.closePicked(l.showEntries[idx].name)
```

这里检查的是将要记录的目录，而不是重新检查文件条目本身。目录在列表加载后可能被删除或替换；此时不应写入失效 history，也不应继续完成 finder 选择。

必须在 `closePicked` 前调用，因为关闭会话后不应再依赖 region 状态。目录激活、breadcrumb、Left/Right 切目录等路径不记录。

`fileList.activate()` 是本计划中唯一额外产生持久化副作用的 region 方法。调用 `recordDirHistory` 不需要任何新机制：`history.go` 与 `filelist.go` 同属 `internal/finder` package，`recordDirHistory` 是包内私有函数，`fileList` 直接调用即可，不需要 import、接口或注册。同包内的轻量耦合是可接受的，但未来如果 history 被替换或拆出独立包，需要重新评估该依赖。

### 3.3 在 `internal/finder/history.go` 中实现 history region

在同一个 `history.go` 中新增 `historyList`，实现现有 `KeyboardRegion`，不扩展 `region.go` 接口。持久化与 region 都是 finder history 的私有实现，当前规模无需拆文件。

建议结构：

```go
type historyList struct {
    fm      *Session
    rect    Rect
    dirs    []string
    cursor  int
    topIdx  int
    focused bool
}
```

只实现必要方法：

- `newHistoryList(fm, rect, dirs)`
- `Rect`
- `Display`
- `HandleMouse`
- `HandleKey`
- `FocusOn` / `FocusLost`
- `moveCursor(delta)`
- `ensureVisible()`
- `activate()`

显示要求：

- 每帧完整清理 history 的 3 行，避免短路径覆盖长路径后的残字。
- 路径采用现有 `truncateLeftPath(path, rect.W-1)`，为右侧 1 列滚动指示预留空间。
- cursor 所在行仅在 `focused` 时用 `config.DefStyle.Reverse(true)`。
- 空余行填空格。
- 列表超过可见行数时，Up / Down 配合 `topIdx` 滚动；右侧固定 1 列作为滚动指示，模仿 `fileList` 的 scroll 表现：
  - `topIdx > 0` 时在可见区首行右侧画 `▲`；
  - `topIdx + visibleH < total` 时在可见区末行右侧画 `▼`；
  - 该字符位于 cursor 行时使用 reverse 样式，否则使用默认样式；
  - 路径绘制宽度为 `rect.W - 1`，为滚动指示预留 1 列。

目录激活流程：

```go
dir := h.dirs[h.cursor]
info, err := os.Stat(dir)
switch {
case err == nil && info.IsDir():
    h.fm.ActivateFromHistory(dir)
case os.IsNotExist(err) || err == nil:
    h.removeCurrent()
default:
    return
}
```

`historyList` 不直接访问 fileList，也不知道 keyboardRegions 中 fileList 的索引；只调用 `Session` 暴露的最小动作 `ActivateFromHistory(dir)`。

`Session.ActivateFromHistory` 负责完成实际目录切换与 focus 回切；查找 fileList 索引不依赖构造顺序：

```go
func (fm *Session) ActivateFromHistory(dir string) {
    fm.list.chdirTo(dir, "")
    for i, kr := range fm.keyboardRegions {
        if _, ok := kr.(*fileList); ok {
            fm.switchFocus(i)
            return
        }
    }
}
```

这样 `keyboardRegions` 的先后顺序、新增其他 keyboard region 都不会破坏 history 激活逻辑。

`removeCurrent` 负责：

- 从 `h.dirs` 删除 cursor 对应项；
- 调 `writeDirHistory(h.dirs)` 同步覆盖 `history.json`；
- 调整 cursor，确保删除末项后不越界；
- 调整 `topIdx` 并重新 `ensureVisible`；
- 触发重绘；
- 列表删空后保留本次 session 的空 history region，避免运行中重排全部 region；下次打开 finder 时因 history 为空不再构造。

只对用户当前激活的一条记录执行 `os.Stat`。有效目录仍交给 `fileList.chdirTo`，不复制重读目录、preview、git 等逻辑。`chdirTo` 的再次校验作为 TOCTOU 兜底。

### 3.4 修改 `internal/finder/session.go`

#### Session 字段

Step 2 实施时，将 Step 1 已有的 `Session.divX` 重命名为 `Session.previewX`。这不是新增布局数据，而是把现有字段改成能表达其含义的名称：它表示左右栏分隔线以及 preview 区的起始 X 坐标。

新增 history 横线坐标字段 `historyY`：

```go
history *historyList
previewX int // 左右栏分隔线的 X 坐标；preview 区从该位置开始
historyY int // history 上边界横线的 Y 坐标
```

`previewX` 在 Step 1 已由现有左右栏布局计算；Step 2 只复用重命名后的字段。`historyY` 只在 `history != nil` 时有效：`drawBorder` 读取 `historyY` 前必须 assert `fm.history != nil`；不在该分支里使用魔法初值 0 画横线。

#### Open 布局与构造顺序

1. 先按 Step 1 现有代码计算 `listRect`（构造一个局部 `listRect` 变量，超越当前代码的“一次行内构造”后立即使用的写法，为可能的 history 缩短预留可能性）。
2. 调 `readDirHistory()`。
3. 不做批量 `os.Stat` 或路径过滤，保持读取顺序直接构造 history。
4. 若 `rect.H >= 15 && len(dirs) > 0`：
   - 在 Session 内按 §2.3 从 `listRect` 底部切出 `historyRect`，并将 `listRect.H` 减少 4 行；
   - 用缩短后的 `listRect` 构造 fileList；
   - 用 `historyRect` 和原始 `dirs` 构造 historyList。
5. 否则完全沿用 Step 1 的 `listRect` 构造 fileList，`history=nil`。
6. slice 顺序固定：
   - `regions`: fileList、history（若有）、preview（若有）；
   - `keyboardRegions`: fileList、history（若有）。
7. 设置 `focus=0` 后调用 `fm.list.FocusOn()`，明确初始化视觉焦点。

不要把 preview 放进 keyboard slice；不要改变 preview rect。

#### drawBorder

保留 Step 1 外框和纵向分隔符的全部现有绘制，仅在 `history != nil` 时增加左栏横分隔线：

- Y 坐标为 `historyY`；
- 只覆盖左栏范围，不穿过 preview；
- 与 `previewX` 的交点使用正确的 box-drawing 连接字符；
- scrollCol = `historyRect.X + historyRect.W - 1`，正好位于 `previewX` 列左侧 1 位，不会越界。
- 分隔线由 session 绘制，history `Display` 不画框线。

需要针对交点字符写精确测试，避免把纵向分隔符覆盖成普通 `─`。

#### close / reset

关闭时对当前有效的 `keyboardRegions[focus]` 调一次 `FocusLost`，再 reset，满足 focus 生命周期协议。`reset` 不仅是 Step 1 的 stub，Step 2 必须补全为：

```go
func (fm *Session) reset() {
    fm.list = nil
    fm.prev = nil
    fm.history = nil
    fm.regions = nil
    fm.keyboardRegions = nil
    fm.focus = 0
    fm.previewX = 0
    fm.historyY = 0
    fm.rect = Rect{}
    fm.isOpen = false
    fm.onClose = nil
    fm.isQuit = nil
}
```

避免 Session 被误复用时残留上次 region；外部 API 不变。

### 3.5 修改 `internal/finder/filelist.go`

把 Step 1 的空 focus hook 落实为视觉状态：

1. `fileList` 增加 `focused bool`。
2. `FocusOn` / `FocusLost` 更新该字段并 `screen.Redraw()`。
3. `drawContent` 中：
   - breadcrumb reverse 条件改为 `focused && cursorOnBc`；
   - entry 选中条件改为 `focused && rowIsCursor`；
   - scroll indicator reverse 条件也加 `focused`。
4. fileList 的 cursor、topIdx、键盘和鼠标逻辑不变。

这样 fileList 失焦后只取消反白，不丢选择；重新聚焦时恢复原行反白。

### 3.6 测试

#### `internal/finder/history_test.go`（新文件）

同一个测试文件覆盖持久化与 history region 两部分。

持久化测试使用临时目录替换并在测试结束恢复 `config.ConfigDir`，覆盖：

- history.json 不存在 → 空列表；
- 首次记录写入目录而非文件；
- 再次记录同目录 → 去重并移到队首；
- 超过 50 条 → 保留最近 50 条；
- 空文件或损坏 JSON → 读取为空并删除原文件，后续记录重建有效文件；
- 其他读取错误 → 返回空且不误删文件；
- 空路径 → 不创建文件；
- 返回 slice 可被调用方修改而不污染下一次读取。

#### `internal/finder/session_test.go`

在既有测试基础上增加：

- 高度 14，即使 history 非空也不构造；
- 高度 15、history 为空不构造；
- 高度 15、history 非空：fileList/history rect 与 `listH==6` 精确符合公式；
- `regions` / `keyboardRegions` 顺序与数量正确，preview 仍只在前者；
- history 内容区与分隔线、外框均不被 hitTest 错误命中；
- Tab / Backtab 在 fileList 与 history 间往返；
- mouse move / click 命中 history 后切 focus，命中 preview 不切 focus；
- 初始 fileList focused，切到 history 后两者 focused 状态互斥；
- close 时当前 region 失焦。

history region 的纯状态和激活行为同样写在 `history_test.go`，覆盖：

- Up/Down clamp；
- 超过 3 条时 `ensureVisible` 正确更新 `topIdx`；
- wheel 等价于 Up/Down；
- Button1 按可见行选择，点空白不改变 cursor；
- Enter / Right 激活合法目录并让 fileList 切换目录；
- 激活不存在或已变成文件的路径，会同时从内存列表和 history.json 删除，并正确修正 cursor/topIdx；
- `os.Stat` 遇到权限或临时错误时保留条目且不切目录；
- 删除最后一条后当前 region 安全显示为空，下次 Open 不再构造 history；
- 其他键返回 false；
- FocusOn / FocusLost 只改变视觉状态，不改变 cursor/topIdx。

对 screen 像素的测试沿用项目现有 mock screen；至少断言 history 分隔线交点和 fileList 失焦后不再 reverse。

---

## 4. 实施顺序

1. 在 `internal/finder/history.go` 中实现持久化函数与 `historyList`，并统一写入 `history_test.go` 测试。
2. 在 `fileList.activate()` 的文件分支挂唯一记录点。
3. 单测 `historyList` 的 cursor、滚动、mouse、key 和 activate。
4. 修改 `Session.Open` 完成条件构造、rect 切分和双 slice 注册。
5. 修改 `drawBorder` 增加 history 分隔线。
6. 落实 fileList/history 的 focus 视觉与 close 生命周期。
7. 补齐 finder 路由、布局和像素测试。
8. 运行 `go test ./internal/finder/`。
9. 运行 `make build`；不得直接使用 `go build`。
10. 按 §5 做端到端手测。

---

## 5. 验收清单

### 5.1 记录与持久化

- [ ] finder 中选中文件时先 stat 当前目录；目录已不存在或不再是目录时不记录、不关闭。
- [ ] stat 成功后才写入当前目录并完成文件选择。
- [ ] 重启 microNeo 后 history 仍可见。
- [ ] 只浏览或切换目录、从 history 激活目录、取消 finder 均不记录。
- [ ] finder 之外的文件打开路径不参与访问历史。
- [ ] 空文件或损坏的 history.json 在读取时被删除，下一次记录可重建。
- [ ] history.json 的其他读取错误不误删文件，也不影响 finder 选中文件和关闭。

### 5.2 布局与视觉

- [ ] finder 高度 `<15` 时无 history，fileList/preview 与 Step 1 完全一致。
- [ ] history 为空时无空白占位和分隔线。
- [ ] 高度 `15` 时 history 显示 3 行，fileList 仍有面包屑 + 6 个条目行 + hint。
- [ ] history 分隔线仅位于左栏，和竖分隔符连接正确，不覆盖 preview。
- [ ] fileList/history 只有当前 focus 区域反白；切 focus 不丢各自 cursor。
- [ ] 长路径左截断，切换到短路径后无残字。

### 5.3 键盘与鼠标

- [ ] Tab / Shift-Tab 在 fileList ↔ history 间循环，永不进入 preview。
- [ ] fileList 末行按 Down 仍 clamp，不自动流入 history。
- [ ] history Up/Down clamp；超过 3 条时窗口跟随 cursor。
- [ ] finder Open 不批量 stat 历史路径。
- [ ] history Enter / Right 只检查当前条目；有效目录切换 fileList。
- [ ] 激活不存在或已不是目录的条目时，该条立即从显示列表和 history.json 删除，不执行 chdir。
- [ ] 删除末项后 cursor/topIdx 不越界；删空后当前 finder 保持稳定，下次打开不显示 history。
- [ ] 权限或临时 I/O 错误不误删历史条目。
- [ ] history Enter / Right 激活有效目录后由 `fileList.chdirTo` 切换，并将 focus 切回 fileList。
- [ ] history 单击只选择不切目录；滚轮移动选择。
- [ ] 鼠标滑入 history 触发 FFM，滑入 preview 不改变键盘 focus。
- [ ] Esc / Ctrl-Q / `q` 在 history focus 下仍关闭 finder。

### 5.4 回归

- [ ] fileList 的打开、返回上级、增删改、hidden、排序、git 状态均不变。
- [ ] preview 的显示、清空和滚轮均不变。
- [ ] resize / blur / pick / quit 的 Result 与回调不变。
- [ ] `go test ./internal/finder/` 全绿。
- [ ] `make build` 通过。

---

## 6. 文件清单

### 新文件

- `internal/finder/history.go`：访问目录历史的 JSON 读写与 history `KeyboardRegion`。
- `internal/finder/history_test.go`：统一覆盖持久化、去重、上限、容错、cursor、滚动、输入与激活。

### 修改文件

- `internal/finder/session.go`：
  - `divX` 重命名为 `previewX`，更新 `session.go` 与 `drawBorder` 内部所有引用；
  - 新增 `historyY`；
  - 补全 `reset()` 清理全部 region 与几何字段；
  - 新增 `ActivateFromHistory(dir)` ；
  - 完成 history 条件构造、布局、slice 注册、border 横线绘制、focus 生命周期。
- `internal/finder/filelist.go`：在 finder 选中文件时记录当前目录，并落实 focus 视觉。
- `internal/finder/session_test.go`：补布局、路由、focus、hit-test 与边框测试。

### 不修改

- `internal/finder/region.go`：现有接口足够。
- `internal/finder/preview.go`：仍是 `NoKeyboardRegion`。
- `internal/finder/add.go`、`delete.go`、`rename.go`：history 激活复用 `fileList.chdirTo`，无需改动。

---

## 7. 风险与控制

1. **高度口径混淆**：显示门槛使用 `Open` 收到的原始 `rect.H`，不是减去 statusLine 后的 `fm.rect.H`。用 H=14/15 边界测试固定。
2. **双 slice 下标混淆**：`focus` 始终索引 `keyboardRegions`；history 同时进入两个 slice，preview 只进入 `regions`。禁止用 `regions` 下标调用 `switchFocus`。
3. **fileList 高度派生错误**：必须先切 rect，再调用 `newFileList`，让既有构造函数从最终 rect 算 `listH`；不要构造后再改 rect。
4. **边框覆盖**：history 横线必须在 `drawBorder` 内画，并正确处理 `previewX` 交点。region 不画线。
5. **失焦后仍反白**：fileList 的 breadcrumb、entry、scroll indicator 三处选中样式必须同时受 `focused` 限制。
6. **坏路径**：finder Open 不批量检查路径；history 激活时只 stat 当前项。明确不存在或已非目录才删除，其他错误保留；有效项交给 `chdirTo` 再校验一次，防止检查后路径发生变化。
7. **持久化失败影响主流程**：记录函数不得让 finder 选择文件失败；history 是辅助功能，失败必须静默降级。
8. **测试污染用户配置**：history 测试必须保存并恢复 `config.ConfigDir`，只在 `t.TempDir()` 中读写。

---

## 8. 本期不做

- history 条数或保存上限配置化；保存上限固定 50，显示窗口固定 3。
- `{path, lastAccess}` 对象模型、热度榜、时间衰减排序。
- history 条目的删除、固定、搜索、手工编辑。
- 选中 history 后自动把该目录提升到队首；只有在 fileList 中确认选择文件才记录。
- preview 键盘化或升级为 `KeyboardRegion`。
- active panel 边框高亮；本期只用当前行 reverse 表达 focus。
- 原子临时文件替换、跨进程锁或文件监听；当前数据量与单进程使用场景不需要。
