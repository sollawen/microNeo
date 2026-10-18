# 我的想法

背景：
- F1（docs/F1-refreshFilelist.md）方案落地后，finder 自动打开时首帧正常，之后按任何键、点鼠标都无反应
- 排查确认为死锁：filelistReader 在 refreshNow 里自锁，主线程 Display 被锁挡死，整个 finder 无响应

设计定位：
- 这是 F1 实现的修正，不改变 F1 的任何设计决策，如下
  - filelistReader 单执行者
  - kick 驱动
  - 无定时器
- 只修「锁内段与锁外 git 的分离」，锁生命周期收窄

# 根因

现象链路：
- finder 自动打开 → startFilelistReader 首发 kick → filelistReader 跑 refreshNow
- 首轮走 firstRefresh 特判（RLock/RUnlock 后才 fetchGitSync），不死锁，首帧能画出来
- 之后任何一次完整刷新（用户操作超阈值触发）都走主路径，死锁

死锁点：
- refreshNow 第 3 步 `l.mu.Lock()` 加 `defer l.mu.Unlock()`，锁要到函数返回才释放
- 第 5 步在锁仍持有的情况下调 `l.fetchGitSync(dir)`，而 fetchGitSync 内部第一件事就是 `l.mu.Lock()`
- Go 的 Mutex 不可重入，filelistReader 卡死在第二次 Lock 上，永久持有 mu 写锁

传导到界面：
- filelistReader 握着写锁 → 主线程 Display 走 drawContent 要 `l.mu.RLock()`
- 写锁不释放，RLock 永远等不到 → 主循环卡死 → 按键、鼠标全部无响应

来源：
- 文档 F1 伪代码第 3 步写 defer Unlock、第 5 步注释写「放锁后」调 fetchGitSync，但 defer 生效前锁根本没放
- 伪代码自身就带这个 bug，实现照抄导致

定位：
- 这是实现细节错误，不是设计缺陷
- F1 整体架构全部可行，设计如下
  - 单执行者
  - kick 驱动
  - 无定时器
  - preview 红线
- 唯一问题是锁边界划错
- 修复只收窄锁生命周期，架构决策零改动

# 修复方案

核心思路：
- 把「锁内更新列表」与「锁外拉 git」彻底分离，fetchGitSync 必须在锁外跑
- 锁内段抽成私有方法 updateEntries，锁生命周期收窄到它内部

锁内段（原 refreshNow 第 3、4 步）抽成 updateEntries(dir, entries) bool，锁内做的事情：
- 内部 Lock + defer Unlock
- 锁内 panic 时兜底解锁，不泄漏 mu
- dir 守卫（chdir 抢跑直接返回 false）
- entriesEqual 判断
- 旧 gitChar 按名拷贝
- rebuildShow
- restoreCursor
- 列表变更时置 previewPending

refreshNow 改造后的主路径：
- 快照 dir / sortMode / sortDesc（RLock/RUnlock）
- 锁外 readDirEntries 重读
- 调 updateEntries（锁内更新，返回即解锁）
- 锁外 fetchGitSync
- 记录完成时间戳
- 收尾 screen.Redraw

不变的东西：
- firstRefresh 首轮特判原样保留
- 顶层 recover + display.DbgLog 原样保留（记日志，reader 继续等下一个 kick）
- fetchGitSync / applyGitFlags / entriesEqual / restoreCursor 全部不动
- 现有测试 TestRefreshNow_MuNotLeakedAfterPanic（靠 l.fm = nil 触发锁内 panic）依然适用
- F1 既有并发模型原样保留，如下
  - reset 时序（关 stop、nil 化 kick）
  - 双 reader 短暂并存的容忍
  - sendKick / startFilelistReader / maybeKickRefresh 接线

伪代码：

```go
// updateEntries 锁内更新列表：dir 守卫 + 变更判断 + 重建 + 恢复光标。调用方须锁外。
// 锁内任何 panic 由 defer Unlock 兜底解锁，不泄漏 mu；返回 changed 供调用方判断。
func (l *fileList) updateEntries(dir string, entries []entry) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.currentDir != dir {
		return false // chdir 抢跑了，本次读取作废，不重建列表
	}

	// 列表变了才更新：旧 git 标志按名拷贝，保住现有标志
	changed := !entriesEqual(l.allEntries, entries)
	if !changed {
		return false
	}
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
	return true
}

// refreshNow 全量刷新一次：读目录 + 更新列表 + 拉 git。调用者只有 filelistReader。
func (l *fileList) refreshNow() {
	var dir string
	defer func() {
		if r := recover(); r != nil {
			// 防锁内逻辑意外 panic 静默杀死 reader 或泄露 mu，只记日志，reader 继续等下一个 kick
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

	// 3. 锁内更新列表：updateEntries 内部 Lock + defer Unlock，返回即已解锁
	l.updateEntries(dir, entries)

	// 4. 锁外同步拉 git：此刻锁已放，fetchGitSync 内部 Lock 不会自锁（修复核心）
	l.fetchGitSync(dir)
	atomic.StoreInt64(&l.lastReadUnix, time.Now().UnixNano()) // 刷新完成时间戳：以 fetchGit 跑完为准
	screen.Redraw() // 收尾重绘统一在此
}
```

对比 F1 伪代码的差异：
- 锁内段（原第 3、4 步）整体搬进 updateEntries，defer Unlock 随方法返回即生效
- fetchGitSync 从锁内移到锁外，第 4 步调用时锁必然已放，自锁消除
- 原第 3 步的 defer Unlock 不再出现在 refreshNow 里，refreshNow 自身不再持有锁

实现约束（两条铁律）：
- 锁内段必须是独立方法，不能内联回 refreshNow，原因如下
  - defer 的生效点是函数返回，不是代码块结束
  - 内联会让锁留到函数尾，fetchGitSync 再次在锁内 Lock
  - 结果与 F1 原 bug 相同，死锁复现
- 不选显式 Unlock（去掉 defer、fetchGitSync 前手动放锁），原因如下
  - 显式 Unlock 改动虽小，但锁内逻辑（entriesEqual / rebuildShow / restoreCursor）panic 时 mu 永久泄漏
  - 抽方法同时满足「锁内段结束即解锁」和「panic 时兑底解锁」
  - 这是保住 F1 设计属性的最小改动

# 改动点

- internal/finder/filelist.go：改动如下
  - 新增私有方法 updateEntries(dir, entries) bool，内容从 refreshNow 第 3、4 步搬移
  - refreshNow 主路径改为「快照 → 锁外重读 → updateEntries → 锁外 fetchGitSync → 时间戳 → Redraw」
  - refreshNow 里删掉原来的 Lock / defer Unlock 与内联列表更新段
  - 其余方法不动

不变项清单（实施时对照检查）：
- firstRefresh 首轮特判路径原样保留，不参与 updateEntries 抽取
- refreshNow 收尾的 screen.Redraw 不下沉到 updateEntries（fetchGitSync 末尾 Redraw 已删，updateEntries 也不能补，避免重复推 drawChan）
- previewPending 写入点从 refreshNow 改至 updateEntries 内 changed 分支

# 验证

- make build-quick 编译通过
- go test ./internal/finder/... 全过（含既有 9 个新增测试）
- go test -race 仅作参考，原因如下
  - F1 引入的 syncPreview 数据竞争是已知项（见「已知问题」O1）
  - 不在 F1a 修复范围，需后续单独 PR 处理
- make build-dbg 手工验证，逐项如下
  - finder 自动打开后按键、鼠标立即响应，不卡死
  - 操作时列表刷新正常
  - git 标志更新正常
  - preview 同步正常
  - 关闭 finder 无协程残留

# 已知问题（后续项，不在本次范围）

O1 syncPreview 数据竞争：
- filelistReader 持锁改 cursor / showEntries
- 主线程 syncPreview 无锁读这些字段
- -race 会报
- 后续单独 PR 修复（selectedFilePath 加 RLock 或快照方法）

O2 fetchGitSync 无 defer Unlock：
- 极端 panic 路径锁永久持有
- 后续统一与 updateEntries 风格一致

O3 首轮特判 panic 测试缺失：
- TestRefreshNow_MuNotLeakedAfterPanic 只覆盖主路径
- 首轮特判路径（CAS → fetchGitSync）未验证
- 后续补测试

O5 panic 日志 dir 为空：
- 极端 CAS panic 时 dir 未赋值
- 日志记 dir= 可读性微差
- 可设默认值
