是的，**每帧都调用**，但有两个关键前提。

## 调用链

```
main()  →  for { DoEvent() }          // 主循环
            ├─ screen.Fill(' ')        // 整屏清空
            ├─ action.Tabs.Display()   // ← 这里
            │    └─ TabList.Display()
            │         ├─ UpdateNames()
            │         └─ if len(t.List) > 1 {
            │                t.TabWindow.Display()   // 真正画 tabLine
            │            }
            ├─ 各 pane.Display()
            ├─ InfoBar.Display()
            └─ screen.Show()           // flush 到终端
```

## 两个要点

**1. 只有 tab 数 > 1 才画**

`tab.go:147` 有个守卫：
```go
if len(t.List) > 1 {
    t.TabWindow.Display()
}
```
单 tab 时整条 tabLine 直接跳过，连一行都不占。

**2. "帧"是事件驱动的，不是固定帧率**

`DoEvent()` 末尾有：
```go
select {
case f := <-shell.Jobs:    // 后台任务完成
case <-config.Autosave:    // 自动保存定时器
case event := <-...:        // 键盘/鼠标/resize 事件
}
```
也就是说**空闲时整个循环阻塞在 select 上，一帧都不画**。只有来事件（按键、鼠标、resize、autosave 到点、shell job 回来）才触发一次完整重绘。

## 推论：这是全量重绘模型

每帧都 `Fill(' ')` 清整屏 → 所有组件（tabLine、所有 pane、infobar、float）全部从头画一遍 → `Show()` 一次性 flush。没有脏区域/dirty line 优化。

所以对 tabLine 而言：只要 tab > 1 且有任何事件进来，`Display()` 里的那一串 `screen.SetContent` 就会全跑一遍——`Names`、`hscroll` 都没变也照画。开销不大（单行、字符数有限），但确实是"无脑全画"。
