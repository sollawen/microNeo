package finder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---- refreshNow 锁内 panic 路径 mu 不泄漏 ----

// TestRefreshNow_MuNotLeakedAfterPanic 验证 refreshNow 锁内逻辑意外 panic 时，
// defer Unlock 兜底解锁 + 局部 recover 不让 panic 逃逸。lock 内任何 panic 都不应
// 让 mu 永久持有，否则主线程 moveCursor / drawContent / chdirTo 全部死锁。
func TestRefreshNow_MuNotLeakedAfterPanic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	fm := &Session{}
	l := &fileList{
		fm:         fm,
		currentDir: dir,
		cursor:     1,
		listH:      10,
	}
	fm.list = l

	// 跳过首轮特判，让 refreshNow 走主路径（进入 Lock 段）
	l.firstRefresh.Store(true)

	// 强制 changed=true：磁盘有 a.txt，但 allEntries 为空 → entriesEqual → false
	l.allEntries = nil
	l.showEntries = nil

	// 注入 panic 源：fm=nil 让 previewPending.Store 触发 nil 指针 deref
	// 此时已持锁，panic 必须被 defer 捕获并释放锁。
	l.fm = nil

	l.refreshNow() // 不应逃逸 panic 到测试 goroutine

	// 验证 mu 没被永久持有：起一个独立 goroutine 尝试加锁，2s 内成功即通过。
	done := make(chan struct{})
	go func() {
		l.mu.Lock()
		l.mu.Unlock()
		close(done)
	}()
	select {
	case <-done:
		// 锁未泄漏
	case <-time.After(2 * time.Second):
		t.Fatal("mu leaked: refreshNow panic did not release the lock")
	}
}

// ---- sendKick 在 refreshKick==nil 时不 panic、不阻塞 ----

// TestSession_SendKickNilSafe 验证 reset 把 refreshKick 置 nil 后，主线程 sendKick
// 走 select default 分支：必须不 panic、不阻塞。否则 reset 后任何操作都会卡死主线程。
func TestSession_SendKickNilSafe(t *testing.T) {
	fm := &Session{} // refreshKick 为 nil
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("sendKick panicked on nil channel: %v", r)
			}
		}()
		fm.sendKick()
	}()
	select {
	case <-done:
		// 非阻塞、未 panic
	case <-time.After(1 * time.Second):
		t.Fatal("sendKick blocked when refreshKick is nil")
	}
}

// ---- restoreCursor 空列表时 cursor=0 ----

// TestRestoreCursor_EmptyList 验证空列表下 restoreCursor 把 cursor 归 0（与现有
// toggleHidden 行为一致）：clamp 到 0 是文档要求的「列表重建后光标有效范围」语义。
func TestRestoreCursor_EmptyList(t *testing.T) {
	l := &fileList{
		showEntries: nil, // 空
		cursor:      5,   // 任意初始值，不应是终值
		listH:       10,
	}
	l.restoreCursor("", 3)

	if l.cursor != 0 {
		t.Errorf("cursor on empty list: got %d, want 0", l.cursor)
	}
}

// TestRestoreCursor_NonEmptyFindsByName 验证非空列表下按名恢复光标到正确索引。
func TestRestoreCursor_NonEmptyFindsByName(t *testing.T) {
	e1 := &entry{name: "alpha"}
	e2 := &entry{name: "beta"}
	e3 := &entry{name: "gamma"}
	l := &fileList{
		showEntries: []*entry{e1, e2, e3},
		cursor:      0,
		listH:       10,
	}
	l.restoreCursor("gamma", 0)

	if l.cursor != 3 {
		t.Errorf("cursor: got %d, want 3 (gamma at index 2)", l.cursor)
	}
}

// TestRestoreCursor_NonEmptyClampsIndex 验证按名未命中时按 oldIdx clamp，不超界。
func TestRestoreCursor_NonEmptyClampsIndex(t *testing.T) {
	e1 := &entry{name: "alpha"}
	e2 := &entry{name: "beta"}
	l := &fileList{
		showEntries: []*entry{e1, e2},
		cursor:      0,
		listH:       10,
	}
	// oldName 不存在；oldIdx 越界 → clamp 到 len-1 = 1 → cursor = 2
	l.restoreCursor("nonexistent", 99)

	if l.cursor != 2 {
		t.Errorf("cursor: got %d, want 2 (clamped to last index)", l.cursor)
	}
}

// ---- applyGitFlags 回写顺序回归 ----

// TestApplyGitFlags_CharsPriorityOverDirState 验证 untracked 折叠目录
// （state=dirAllUntracked）内被 .gitignore 排除的 ignored 文件在 chars 中带 'I'，
// 写回时 chars 必须优先于 dir 级状态：显示 'I' 而非 'U'。
// 这是 fetchGit → fetchGitSync 重构的回归保护：chars 优先的循环顺序是硬约束。
// 直接驱动 applyGitFlags 避免拉起 git 子进程与修改 GlobalSettings。
func TestApplyGitFlags_CharsPriorityOverDirState(t *testing.T) {
	l := &fileList{
		allEntries: []entry{
			{name: "ignored.txt", isDir: false}, // chars 里有 'I'
			{name: "other.txt", isDir: false},   // chars 里没有，依赖 state 兑底
		},
	}
	chars := map[string]rune{"ignored.txt": 'I'}
	state := dirAllUntracked // 兑底给 'U'

	l.applyGitFlags(chars, state)

	if got := l.allEntries[0].gitChar; got != 'I' {
		t.Errorf("ignored.txt gitChar: got %q, want 'I' (chars must win over state)", got)
	}
	if got := l.allEntries[1].gitChar; got != 'U' {
		t.Errorf("other.txt gitChar: got %q, want 'U' (chars miss → state fallback)", got)
	}
}

// TestApplyGitFlags_DirAllIgnoredFallback 验证 state=dirAllIgnored 兌底打 'I'、
// state=dirNormal 不改 gitChar（保留干净 / 非仓库语义）。
func TestApplyGitFlags_DirAllIgnoredFallback(t *testing.T) {
	l := &fileList{
		allEntries: []entry{
			{name: "a", isDir: false},
			{name: "b", isDir: false},
			{name: "c", isDir: false},
		},
	}
	l.applyGitFlags(nil, dirAllIgnored)
	if got := l.allEntries[0].gitChar; got != 'I' {
		t.Errorf("a gitChar: got %q, want 'I' (dirAllIgnored fallback)", got)
	}

	l2 := &fileList{
		allEntries: []entry{{name: "a", isDir: false}},
	}
	l2.applyGitFlags(nil, dirNormal)
	if got := l2.allEntries[0].gitChar; got != 0 {
		t.Errorf("a gitChar: got %q, want 0 (dirNormal keeps clean)", got)
	}
}

// ---- entriesEqual 边界 ----

// TestEntriesEqual_DifferentLength 验证长度不同立即判不等，不深入比内容。
func TestEntriesEqual_DifferentLength(t *testing.T) {
	a := []entry{{name: "x"}}
	b := []entry{}
	if entriesEqual(a, b) {
		t.Errorf("different lengths should be unequal")
	}
}

// TestEntriesEqual_OrderMatters 验证顺序差异（同名不同位）被识别为变化。
func TestEntriesEqual_OrderMatters(t *testing.T) {
	a := []entry{{name: "alpha"}, {name: "beta"}}
	b := []entry{{name: "beta"}, {name: "alpha"}}
	if entriesEqual(a, b) {
		t.Errorf("order difference should be detected")
	}
}