# 我的想法

现状：
- 当用户打开Finder的时候，打开的时候会读取当前目录里的列表，然后就不会再次读取了。

我想改成这样：
- finder 打开后，用户操作 finder 时（按键 / 鼠标），若距上次读取超过阈值（默认 1 秒），就重新读取一次最新列表，含每个文件的 git 标志
- finder 关闭后不再刷新

设计定位：
- microNeo 本质是 editor，finder 的职责是「用户操作时列表不陈旧」，不是「观察监视外部变化」
- 观察监视交给 yazi / lazygit 这类专业文件管理器
- 用户不操作 finder 时零后台开销：无定时器、无轮询

# 实施方案

## 需求边界

finder 打开期间：
- 用户每次操作 finder（键盘 / 鼠标）时检查距上次读取的时间差
- 超过阈值（默认 1 秒）就重新读取一次，读取含文件列表与 git 标志
- 用户不操作则不读取，零后台开销

finder 关闭后不再刷新：
- 所有关闭路径都走 reset()，在 reset 里统一停 filelistReader
- 关闭路径有四条：
  - Esc
  - 选中文件
  - Quit
  - Resize
- 停止后不留后台协程

## 现状调研

列表只读一次：
- `newFileList` 构造期调 `loadDir`，`readDirEntries` 读一次建好 `allEntries`
- 之后只有 `chdirTo`（切目录）会重读，列表本身不再主动更新

并发与重绘模型：
- 条目列表 `allEntries` / `showEntries`：`mu`（RWMutex）保护
- git 状态 `isRepo` / `gitBranch`：`mu`（RWMutex）保护
- Display 在锁内拷出可见行，锁外绘制
- 后台协程改状态后调 `screen.Redraw()` 推 drawChan，主循环醒来整屏重绘，`fetchGit` 已是这个模式
- 预览区 `preview.Load` 无锁，只允许主线程调（`syncPreview`），后台线程不能碰

## 方案

核心思路：不设闹钟（定时器），改派一个「后台小帮手」专门负责刷新列表。

后台小帮手是什么（代码里叫 filelistReader）：
- filelistReader 是 Go 里的一个协程（goroutine）：Go 在后台跑的小任务，不占界面，界面照常响应
- 这个小帮手平时歇着，什么也不干、零开销
- 收到「踢一脚」的通知才动手
- 动手 = 重新读一遍目录 + 拉一遍 git 状态

踢一脚是什么（代码里叫 kick）：
- 就是给帮手发个「该干活了」的提醒，帮手收完通知就去干一次活
- 有三件事会踢它：
  - finder 刚打开时
  - 用户切到别的目录时
  - 用户操作 finder 且离上次读取超过 1 秒时

单执行者是什么意思：
- 全世界只有这一个小帮手负责刷新，它一次只干一件活，干完再干下一件
- 所以同一时刻最多只有一个 git 进程在跑，两个 git 进程永远不会互相打架
- 这是整个设计最重要的目的：用「只有一个人干活」来保证不乱，而不是靠复杂的规则去防

为什么要后台干、而不是界面直接干：
- 读 git 状态可能很慢（最坏几秒）
- 界面直接干 = 按一下键界面就卡死几秒，没法用
- 后台干 = 界面照常响应，读完了再通知界面重画

复用原则：
- 不新写读列表逻辑：refreshNow 只组合现成函数，净增代码只在组合层
- 复用的现成函数（零新增）：
  - readDirEntries（读目录 + 排序）
  - rebuildShow()（重建可见列表）
  - ensureVisible()（光标可视区）
  - getGitStatus（git 拉取内核）
- 净新增与抽取：
  - refreshNow()：组合层，无新算法
  - entriesEqual()：纯比对
  - restoreCursor()：从 toggleHidden 抽取，不改逻辑
  - fetchGitSync()：fetchGit 改名去异步包装
- 改动行数：
  - 净新增主要是 refreshNow 组合 + entriesEqual，几十行
  - restoreCursor / fetchGitSync 是搬移不是新增
  - chdirTo 只改一行
- 原则定位：
  - 不发明新读取逻辑
  - 只调整触发时机与执行者
  - 「怎么做」全现成，改的是「什么时候做、谁来做」

改动分四块：

### 1. fileList 加 refreshNow 方法（唯一读取入口，全量）

`refreshNow()` 做的事：
- 结尾记录本次刷新完成时间戳（fetchGit 跑完的时刻，atomic，供主线程判断是否超阈值）
- 锁内快照 `currentDir` / `sortMode` / `sortDesc`，锁外 `readDirEntries` 重读（复用现有排序）
- 加锁后先校验 `currentDir` 仍是快照值（与 `fetchGit` 同款守卫）：
  - 已被 chdir 抢跑则直接返回，chdir 自己重读过列表
  - 未变才继续
- 与现 `allEntries` 比对（name / isDir / size / mtime，不含 gitChar），得 `changed`
- `changed` 时更新列表，动作对齐 `chdirTo`：
  - 按名字从旧表拷贝 `gitChar`（保住旧标志，避免 fetch 回来前的空白闪烁）
  - 换 `allEntries` 并 `rebuildShow()`
  - `restoreCursor` 恢复光标（见第 3 点，内含 ensureVisible）
  - 置 `previewPending` 标记（见第 4 点：列表变了，preview 待主线程同步）
- 无论 `changed` 与否都拉 git（全量）：`fetchGitSync` 同步跑（见第 2 点），不 go 出去
- 收尾 `screen.Redraw()`，由主循环重绘
- 入口注释写死红线：本函数跑在 filelistReader 里，preview 无锁、主线程专属，禁止调 syncPreview / preview.Load

刷新时间戳的语义：
- 以「上次刷新完成」为基准，不是「上次刷新开始」
- 完成 = fetchGitSync 跑完的时刻
- 原因：git 慢时若按开始点算，上一次没跑完就满足阈值，会背靠背连续刷，git 几乎永远在跑；按完成点算，两次完整刷新之间保证有真正的休息间隔
- 守卫路径下时间戳仍记录：fetchGitSync 内部 dir 守卫返回（极小窗口被 chdir 抢跑）时，refreshNow 第 5 步仍会写时间戳
  - 这不是缺陷：chdir 自己会再 sendKick，正确性由后续那次完整刷新兜底，只是「完成」语义从「git 真跑完」放宽成「本次刷新流程走完」

全量拉 git 的成本模型：
- 触发频率受阈值限制（操作中最多每阈值一次），不会失控
- 换来 commit / add / checkout 等纯 git 变化在用户下次操作时立刻反映
- 列表无可见变化（仅 mode 变、或内容变但 size/mtime 不变）时 git 仍照跑：entriesEqual 判 unchanged 不重建列表，但 git 标志的实时性不依赖列表变化

首轮特判：
- `newFileList` 已同步 `loadDir` 建好首帧，`startFilelistReader` 发的首个 kick 若再全量重读就是白跑一次
- 用 `firstRefresh`（atomic bool）标记：首轮只补 git（fetchGitSync），跳过 readDirEntries / entriesEqual / rebuildShow

伪代码：

```go
// refreshNow 全量刷新一次：读目录 + 更新列表 + 拉 git。调用者只有 filelistReader。
func (l *fileList) refreshNow() {
	// 红线：跑在 filelistReader 里，禁止调 preview（无锁、主线程专属）；列表变更只置 previewPending
	var dir string
	defer func() {
		if r := recover(); r != nil {
			// 防锁内逻辑意外 panic 静默杀死 reader 或泄露 mu。
			// 死锁由 defer Unlock 兜底，这里只记日志，reader 继续等下一个 kick。
			display.DbgLog("finder refreshNow panic dir=%s: %v", dir, r)
		}
	}()

	// 0. 首轮特判：newFileList 已同步 loadDir 建好首帧，跳过重读，只补 git
	if l.firstRefresh.CompareAndSwap(false, true) {
		l.mu.RLock()
		dir = l.currentDir
		l.mu.RUnlock()
		l.fetchGitSync(dir)
		atomic.StoreInt64(&l.lastReadUnix, time.Now().UnixNano())
		screen.Redraw()
		return
	}

	// 1. 快照：锁内读目录状态，锁外干耗时活
	l.mu.RLock()
	dir, sm, desc := l.currentDir, l.sortMode, l.sortDesc
	l.mu.RUnlock()

	// 2. 锁外重读目录（读盘慢，不占锁）
	entries := readDirEntries(dir, sm, desc)

	// 3. 加锁后立刻 defer Unlock：锁内任何 panic 都保证解锁，不会死锁整个 finder
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.currentDir != dir {
		return // chdir 抢跑了，列表已被它重建；本次读取作废，不算完成一次刷新
	}

	// 4. 列表变了才更新：旧 git 标志按名拷贝，保住现有标志
	changed := !entriesEqual(l.allEntries, entries)
	if changed {
		oldChars := make(map[string]rune, len(l.allEntries))
		for _, e := range l.allEntries {
			oldChars[e.name] = e.gitChar
		}
		for i := range entries {
			entries[i].gitChar = oldChars[entries[i].name]
		}

		// 记录光标、重建列表、恢复光标
		curName := ""
		if l.cursor >= 1 && l.cursor-1 < len(l.showEntries) {
			curName = l.showEntries[l.cursor-1].name
		}
		curIdx := l.cursor - 1
		l.allEntries = entries
		l.rebuildShow()
		l.restoreCursor(curName, curIdx)

		// 列表变了 → 置 preview 待同步标记，主线程 Display 消费（后台不碰 preview）
		l.fm.previewPending.Store(true)
	}

	// 5. 放锁后：同步拉 git（getGitStatus 锁外，2s 超时兜底），无论列表变没变
	l.fetchGitSync(dir)
	atomic.StoreInt64(&l.lastReadUnix, time.Now().UnixNano()) // 刷新完成时间戳：以 fetchGit 跑完为准
	screen.Redraw() // 收尾重绘统一在此，fetchGitSync 不再各自 Redraw
}
```

不做什么：
- 不设定时器：用户不操作就不读取
- 不刷新预览：`preview.Load` 无锁，后台调会和主线程 Display 数据竞争；只置 previewPending 标记，由主线程下一次 Display 消费

### 2. fetchGit 改同步版 fetchGitSync

- 现有 `fetchGit` 的 git 标志回写逻辑保持不变，区别仅在三点：
  - 不 go 出去：`getGitStatus` 锁外跑（2 秒 ctx 超时兜底不变）
  - 循环顺序保持现状：chars 优先（命中 continue），dir 级 state 仅作 fallback
  - 去掉末尾 `screen.Redraw()`：收尾重绘统一在 refreshNow 第 5 步，避免重复推 drawChan
- 改名理由：chdir / newFileList 不再自己发 fetch 后，异步版没有调用者，唯一调用者是 filelistReader 里的 refreshNow（同步调用）
- 让「完成」自然成为 filelistReader 推进的一部分：跑完才轮到下一次 select，天然串行
- 注意：「同步」仅相对 filelistReader 自身
  - reader 等 git 跑完才处理下一个 kick，天然串行
  - 主线程照常响应按键与重绘，用户操作不受影响

chars 优先的顺序是硬约束，不是可选项：
- untracked 目录（state=dirAllUntracked）内仍可能有个别 ignored 条目
- `--ignored=traditional` 会逐个列出 ?? 目录里的 !! 文件，chars 里带 'I'
- 若先按 state 打 U 再查 chars，这些 !! 文件会被错打成 'U'，与现状行为回退
- 顺序必须是：chars 命中直接写并 continue，state 只兜底未命中项

伪代码（与现状 fetchGit 回写段逐行同构，仅去掉异步包装与末尾 Redraw）：

```go
// fetchGitSync 同步版：锁外 fork git，写回前过 dir 守卫。调用者只有 refreshNow。
func (l *fileList) fetchGitSync(dir string) {
	isRepo, branch, chars, state := getGitStatus(dir) // 锁外慢活，不占锁

	l.mu.Lock()
	if l.currentDir != dir {
		l.mu.Unlock()
		return // chdir 抢跑了，结果丢弃
	}
	l.isRepo = isRepo
	l.gitBranch = branch
	for i := range l.allEntries {
		// chars 优先于 dir 级标志：untracked 目录内仍可能有个别 ignored 条目
		// （--ignored=traditional 会逐个列出 ?? 目录里的 !! 文件），须显示 I 而非 U。
		if ch, ok := chars[l.allEntries[i].name]; ok {
			l.allEntries[i].gitChar = ch
			continue
		}
		switch state {
		case dirAllIgnored:
			l.allEntries[i].gitChar = 'I'
		case dirAllUntracked:
			l.allEntries[i].gitChar = 'U'
		}
	}
	l.mu.Unlock()
}
```

调用点：refreshNow 第 5 步 `l.fetchGitSync(dir)`，同步返回后才轮到下一次 select，天然串行。

### 3. 抽公共光标恢复方法

- 把 toggleHidden 里「记录 oldName / oldIdx → 按名恢复 → 找不到按索引 clamp → ensureVisible」抽成私有方法 `restoreCursor`
- `toggleHidden` 与 `refreshNow` 共用，行为保持一致
- 调用方须已持锁（restoreCursor 锁内改 cursor 与 showEntries）

抽出的方法（与 toggleHidden 现状恢复段逐行同构，内含 ensureVisible）：

```go
// restoreCursor 列表重建后恢复光标：先按名找，找不到按索引 clamp，最后 ensureVisible。调用方须持锁。
func (l *fileList) restoreCursor(oldName string, oldIdx int) {
	found := false
	if oldName != "" {
		for i, e := range l.showEntries {
			if e.name == oldName {
				l.cursor = i + 1
				found = true
				break
			}
		}
	}
	if !found {
		idx := oldIdx
		if idx >= len(l.showEntries) {
			idx = len(l.showEntries) - 1
		}
		if idx < 0 {
			idx = 0
		}
		if len(l.showEntries) == 0 {
			l.cursor = 0
		} else {
			l.cursor = idx + 1
		}
	}
	l.ensureVisible()
}
```

改造后的 toggleHidden（恢复段替换为一行调用）：

```go
func (l *fileList) toggleHidden() {
	l.mu.Lock()
	oldName := ""
	oldIdx := l.cursor - 1
	if l.cursor >= 1 && l.cursor-1 < len(l.showEntries) {
		oldName = l.showEntries[l.cursor-1].name
	}
	l.showHidden = !l.showHidden
	l.rebuildShow()
	l.restoreCursor(oldName, oldIdx)
	l.mu.Unlock()

	l.fm.syncPreview()
	screen.Redraw()
}
```

refreshNow 里的调用形态（见第 1 节伪代码第 4 步）：记录 curName / curIdx → 换 allEntries → rebuildShow → restoreCursor，不再单独 ensureVisible（restoreCursor 内部已处理）。

### 4. Session 生命周期接线 + kick 通道 + 操作 hook + preview 同步

Session 字段与常量：
- 加 `refreshStop chan struct{}` 与 `refreshKick chan struct{}`
- 加 `previewPending atomic.Bool`（列表被后台刷新改过，待主线程同步 preview）
- 加常量 `refreshThreshold = time.Second`（操作触发的最小读取间隔，可调）
- fileList 加 `lastReadUnix int64` 与 `firstRefresh atomic.Bool`（atomic 读写）

```go
const refreshThreshold = time.Second // 操作触发的最小读取间隔，可调

type Session struct {
	// ...现有字段...
	refreshStop    chan struct{} // 关闭信号：reset 里 close，filelistReader 退出
	refreshKick    chan struct{} // 踢一下：构造期 / chdir / 操作超阈值发，缓冲 1 合并多次
	previewPending atomic.Bool   // 列表被后台刷新改过：主线程 Display 消费一次，补 preview
}
```

`startFilelistReader()`（无定时器）：
- 建 stop channel 与 kick channel（kick 缓冲 1，多个 kick 自然合并为「至少一次」）
- 捕获 `list := fm.list`（filelistReader 不再碰 fm.list 字段，避免与 reset 的 nil 写入竞争）
- 起 filelistReader，两分支 select：stop 退出、kick 干活
- 启动前先发一个 kick：构造期首轮由 reader 跑

```go
// startFilelistReader 启动后台小帮手。Open 末尾调用，reset 关 stop 停止。
func (fm *Session) startFilelistReader() {
	stop := make(chan struct{})
	kick := make(chan struct{}, 1)
	fm.refreshStop = stop
	fm.refreshKick = kick
	list := fm.list // 捕获引用，reader 不碰 fm.list 字段

	// 构造期首轮由 reader 跑：此刻无人消费，缓冲 1 保证不丢
	kick <- struct{}{}

	go filelistReader(stop, kick, list)
}

// filelistReader 后台小帮手：一直待命，收到 kick 就全量刷新一次。单执行者。
func filelistReader(stop <-chan struct{}, kick <-chan struct{}, list *fileList) {
	for {
		select {
		case <-stop:
			return
		case <-kick: // 构造期 / chdir / 操作超阈值
			list.refreshNow()
		}
	}
}
```

`HandleEvent` 接线（操作驱动核心）：
- 每次键盘 / 鼠标事件进来，先检查距上次读取是否超阈值
- 超了就 `sendKick()`，filelistReader 醒来全量刷新

```go
func (fm *Session) HandleEvent(event tcell.Event) {
	if !fm.isOpen {
		return
	}
	fm.maybeKickRefresh(event) // 操作驱动：距上次读取超阈值则 kick
	// ...其余现有事件分发...
}
```

`maybeKickRefresh(event)` 与 `sendKick()`：
- 事件类型过滤：三类事件不 kick，避免发完就作废的浪费，逐类如下
  - Resize（紧接 close）
  - 关闭键（Esc / Ctrl-Q / q）
  - 纯鼠标 motion（Buttons()==0）
- `isCloseKey` 抽出来与 `handleGlobalKey` 共用同一判断

```go
// maybeKickRefresh 操作驱动刷新：距上次刷新完成超过阈值就 kick 一次。主线程调。
func (fm *Session) maybeKickRefresh(event tcell.Event) {
	if fm.list == nil {
		return // 防御：未来挪调用点时 list 可能为 nil
	}
	switch e := event.(type) {
	case *tcell.EventResize:
		return // 紧接 close(Resize)，kick 必然作废
	case *tcell.EventKey:
		if isCloseKey(e) {
			return // 紧接 close(Esc/Quit)，kick 必然作废
		}
	case *tcell.EventMouse:
		if e.Buttons() == 0 {
			return // 纯 mouse motion，不触发刷新
		}
	}
	if time.Now().UnixNano()-atomic.LoadInt64(&fm.list.lastReadUnix) >= int64(refreshThreshold) {
		fm.sendKick()
	}
}

// isCloseKey 判定是否为关闭 finder 的全局键（Esc / Ctrl-Q / q）。与 handleGlobalKey 共用。
func isCloseKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlQ:
		return true
	}
	return ev.Key() == tcell.KeyRune && ev.Rune() == 'q'
}

// sendKick 通知 filelistReader 立即干活。主线程调，非阻塞；通道已 buffered 时合并掉。
func (fm *Session) sendKick() {
	select {
	case fm.refreshKick <- struct{}{}:
	default: // kick 已排队，本次合并
	}
}
```

`Display` 接线（preview 同步消费点）：
- previewPending 置 true 后，由主线程下一次 Display 消费，补一次 syncPreview
- preview.Load 仍只在主线程调，后台红线不破

```go
func (fm *Session) Display() {
	if !fm.isOpen {
		return
	}
	fm.maybeSyncPreview() // 消费 previewPending：列表刚被后台刷新改过，补一次 preview 同步
	// ...现有 HideCursor / drawBorder / region Display...
}

// maybeSyncPreview 若上次后台刷新改了列表，则补一次 syncPreview。主线程调。
func (fm *Session) maybeSyncPreview() {
	if fm.previewPending.CompareAndSwap(true, false) {
		fm.syncPreview()
	}
}
```

`reset()` 改动：
- 开头关 `refreshStop` 并置 nil，`refreshKick` 一并置 nil
- Session 是 BufPane 复用的单例，每次开闭都要能重启
- `refreshKick` 永远不能 close，只能 nil 化：select 里 send 到已关闭 channel 会立即 panic，default 分支救不回来

```go
func (fm *Session) reset() {
	if fm.refreshStop != nil {
		close(fm.refreshStop) // filelistReader 从 select 退出
		fm.refreshStop = nil
	}
	// refreshKick 永远不能 close，只能 nil 化——sendKick 的 select 里 send 到 closed chan 会 panic
	fm.refreshKick = nil
	fm.list = nil
	// ...其余现有置 nil...
}
```

`chdirTo` 接线：
- 主线程同步换列表 + 定位 + preview（现状保留，保按键即时性）
- 删掉 `go l.fetchGit(target)`，末尾改发非阻塞 kick

```go
func (l *fileList) chdirTo(target, focusName string) {
	// ...锁内换列表 + 定位，现状不动...
	l.fm.syncPreview()
	screen.Redraw()
	// 时序：上面两步已让界面立即显示新目录列表（git 标志暂空）
	// 下面 sendKick 只塞命令不阻塞，git 标志由后台 reader 跑完再补
	l.fm.sendKick() // 替代 go l.fetchGit(target)：git 由 filelistReader 同步拉
}
```

`newFileList` 接线：
- 同步 `loadDir` 保留（首帧硬需求）
- 删掉 `go l.fetchGit(cwd)`：首轮由 startFilelistReader 的首个 kick 覆盖

```go
// newFileList 构造 fileList region。
func newFileList(fm *Session, rect Rect, cwd, file string, pickerW int) *fileList {
	l := &fileList{
		// ...现有字段初始化，不动...
	}
	l.loadDir(cwd, file) // 同步读目录 + 建列表 + 定位光标，首帧硬需求
	// 删掉 go l.fetchGit(cwd)：git 标志由 startFilelistReader 的首个 kick 覆盖
	return l
}
```

`Open` 接线：
- `fm.list = newFileList(...)` 之后、所有初始化完成处调 `fm.startFilelistReader()`
- 删除 newFileList 内部原有的 `go l.fetchGit(cwd)`

```go
func (fm *Session) Open(rect Rect, cwd, file string, isQuit bool, onClose func(Result)) bool {
	// ...现有校验与几何计算，不动...
	fm.list = newFileList(fm, listRect, cwd, file, pickerW)
	// ...其余 region 构造与 syncPreview / FocusOn，不动...
	fm.startFilelistReader() // 所有初始化完成后才起 reader，首个 kick 补 git 标志
	return true
}
```

错误处理：
- refreshNow 内局部 recover + `display.DbgLog` 写日志
- 防锁内逻辑（entriesEqual / rebuildShow / restoreCursor）意外 panic 静默杀死 filelistReader，导致 finder 永久停止自动刷新
- 死锁由 defer Unlock 兜底：锁内 panic 时 mu 不会永久持有，主线程 moveCursor / drawContent / chdirTo 不受影响
- 日志带 dir 快照，便于定位是哪次刷新触发

## 改动点

- `internal/finder/filelist.go`：改动如下
  - 加 `refreshNow()`
  - 加 `entriesEqual()`
  - 加 `lastReadUnix int64`、`firstRefresh atomic.Bool` 字段（atomic 读写）
  - `fetchGit` 改同步版 `fetchGitSync`，回写顺序保持 chars 优先，去末尾 Redraw
  - 删两处 `go l.fetchGit(...)`（newFileList、chdirTo）改 `sendKick`
  - 抽私有方法 `restoreCursor()`
  - 补 import time、sync/atomic 与 internal/display
- `internal/finder/session.go`：改动如下
  - 加 `refreshStop` / `refreshKick` / `previewPending` 字段
  - 加 `startFilelistReader()`（无定时器）与 `filelistReader()`
  - 加 `maybeKickRefresh(event)`（含事件类型过滤）
  - 加 `maybeSyncPreview()`（Display 消费 previewPending）
  - 抽 `isCloseKey()`（与 handleGlobalKey 共用）
  - `HandleEvent` 开头接 `maybeKickRefresh(event)`，`Display` 开头接 `maybeSyncPreview()`
  - 加常量 `refreshThreshold = time.Second`
  - 补 import time 与 sync/atomic

## 边界情况

- 纯观察场景（打开 finder 不操作）不刷新：这是刻意的设计取舍，editor 定位下监视交给 yazi / lazygit
- commit / add / checkout 等纯 git 操作：用户下次操作 finder 时全量拉 git，标志立刻更新
- 用户持续操作：每阈值一次全量读（目录 + git），频率受阈值限制，不会失控
- 慢 git（接近 2s 超时）：filelistReader 串行处理 kick，操作产生的刷新排队，最坏延迟 = 前一个 git 剩余 + 本次 git；主线程不卡
- 慢 git 不会背靠背：以完成点为基准冷却，上次刷新跑完歇满阈值才轮到下一次
- 连续切目录：中间目录的刷新在 refreshNow 锁内守卫处直接作废（chdir 已重建列表），只有最终目录会完整跑一遍 git
- 目录无可见变化（仅 mode 变、或内容变但 size/mtime 不变）：entriesEqual 判 unchanged 不重建列表，git 仍照跑（保证标志实时）
- 对话框（rename / delete）开着时 kick 照跑：回调持有的 name / dir 是局部值拷贝，结束后 `chdirTo` 重读，无冲突
- 当前目录被删：`os.ReadDir` 报错 → 空列表，光标归 0，可接受
- kick 合并：缓冲 1，用户连续操作产生的多次 kick 合并为一次读取，读完一次再等阈值
- sendKick 对 nil 通道安全：reset 后 refreshKick 为 nil，非阻塞 select 走 default 分支
- refreshKick 永远不能 close，只能 nil 化：sendKick 的 select 里 send 到 closed chan 会立即 panic，default 救不回来
- close 后旧 filelistReader 仍在跑当前 refreshNow：reset 关 stop 后，reader 若正卡在 getGitStatus（最坏 2s）会先跑完再退出；期间重 Open 起的新 reader 独立运行，短暂两份后台但互不干扰（旧 reader 拿的是 startFilelistReader 时捕获的旧 list）
- 测试影响：无定时器后 filelistReader 无 kick 时永久阻塞、零开销
- Open 后不 Close 的测试无额外行为

## 验证

- `make build-dbg` 起调试版，开 finder，操作（按键 / 鼠标）观察列表刷新，不操作时不刷新
- 外部建 / 删文件后操作 finder，1 秒内反映（超过阈值才读）
- 外部 commit 后操作 finder，git 标志更新
- 切目录后 git 标志正常出现，狂按导航不产生并发 git 进程
- 关闭 finder 后确认不再刷新（无后台协程残留）
- 外部删除光标所在文件后操作 finder：列表高亮落到邻近条目，右侧 preview 同步显示新光标文件（previewPending 生效）
- `make build` 后跑 `go test ./internal/finder/...`

单元测试补充：
- refreshNow 锁内 panic 路径 mu 不泄漏：defer Unlock 兜底后，recover 完 mu 可再次获取
- sendKick 在 `fm.refreshKick == nil` 时不 panic、不阻塞（走 default）
- restoreCursor 在 showEntries 为空时 cursor=0（与现有 toggleHidden 一致）
- fetchGitSync 回写顺序回归：untracked 折叠目录（state=dirAllUntracked）内 ignored 文件显示 'I' 而非 'U'


# 实施计划

按依赖顺序分 5 步，每步完成后可编译，前两步零行为变化。

- [ ] 第 1 步：搬移与字段（无新逻辑）
  - 抽 `restoreCursor()`，toggleHidden 恢复段改为一行调用
  - `fetchGit` 改同步版 `fetchGitSync`（去异步包装与末尾 Redraw，回写顺序不变）
  - fileList 加 `lastReadUnix` / `firstRefresh` 字段
  - Session 加 `refreshStop` / `refreshKick` / `previewPending` 字段与 `refreshThreshold` 常量
  - 编译验证：现状行为零变化
- [ ] 第 2 步：refreshNow 与 entriesEqual
  - 加 `entriesEqual()`（name / isDir / size / mtime 比对）
  - 加 `refreshNow()`（首轮特判 / defer Unlock / previewPending 置位 / fetchGitSync 同步调用 / 收尾 Redraw）
  - 此刻 refreshNow 尚无调用者，只保证编译通过
- [ ] 第 3 步：Session 通道与 hook
  - 加 `startFilelistReader()` / `filelistReader()` / `sendKick()`
  - 加 `maybeKickRefresh(event)` / `maybeSyncPreview()` / `isCloseKey()`
  - `reset()` 关 refreshStop、nil 化 refreshKick
  - `HandleEvent` 开头接 `maybeKickRefresh`，`Display` 开头接 `maybeSyncPreview`
- [ ] 第 4 步：触发点接线
  - `newFileList` 删 `go l.fetchGit(cwd)`
  - `chdirTo` 删 `go l.fetchGit(target)` 改 `l.fm.sendKick()`
  - `Open` 末尾调 `fm.startFilelistReader()`
  - 全量功能接通，进入验证
- [ ] 第 5 步：测试与验证
  - 补四项单元测试，逐项如下
    - mu 不泄漏
    - nil 通道 sendKick
    - restoreCursor 空列表
    - fetchGitSync 顺序回归
  - `make build` 后跑 `go test ./internal/finder/...`
  - `make build-dbg` 手工验证，逐项如下
    - 操作时列表刷新、不操作时不刷新
    - git 标志更新
    - preview 同步
    - 无协程残留
