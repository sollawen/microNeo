# 多光标显示 Bug 修复记录

> 状态：✅ 已修复并提交（commit `1cad1088`）
> 影响版本：v1.0.6 ~ v1.0.9
> 文件：`internal/display/bufwindow_md.go` + `internal/display/bufwindow_md_test.go`

---

## Bug 现象

在 `.md` 文件中按 `Shift-Alt-↑/↓`（`SpawnMultiCursorUp/Down`）进入多光标状态后：

- ✅ **输入/删除**：在所有 cursor 位置同时生效（buffer 层多光标逻辑正常）
- ❌ **显示**：屏幕上只看到最后一个 cursor 的光标，其它次级光标不显示

## 定位过程

二分编译历史 tag 实测：

| 版本 | 多光标显示 | 说明 |
|---|---|---|
| v1.0.5 | ✅ 正常 | 二分确认的「最后一个正常」版本 |
| v1.0.6 | ❌ 复现 | 引入「方案 B：screenBuffer 离屏渲染」 |
| v1.0.7 / v1.0.8 / v1.0.9 | ❌ 持续复现 | bug 一直存在 |

`git diff v1.0.5..v1.0.6 -- internal/display/bufwindow_md.go` 显示该文件改动 800+ 行，正是方案 B 重构引入的 bug。

## 根因

v1.0.6 方案 B 把 MD 渲染从「直接画屏」改成「先画到离屏 `screenBuffer` → 再 blit 到屏」。光标绘制跟着走了两步走，但 `screenBuffer` 的光标存储是**单标量设计**，多光标信息在两步之间被丢失。

### 三处缺陷（都在 `internal/display/bufwindow_md.go`）

| # | 位置 | 问题 |
|---|---|---|
| 1 | `screenBuffer` 字段 | 只有 `cursorX / cursorY / cursorOK` 三个标量，存不下多个光标位置 |
| 2 | `ShowCursor(x, y, main)` 函数体 | 接收 `main` 参数但完全没用，每次调用覆盖前一个 cursor，最终只剩最后一个 |
| 3 | blit 路径（`showBuffer`）| 只调用一次 `w.showCursor(..., true)`，只画一个且强制当主光标，从未调用 `ShowFakeCursorMulti` |

### 两层光标（不要混淆）

| 层 | 存储 | 多光标状态 | 用途 |
|---|---|---|---|
| Buffer 层 | `buffer.Buffer.cursors []Cursor` | ✅ 完整保留 | 输入/删除遍历它 |
| 显示层 | `screenBuffer` 的光标字段 | ❌ 只存一个 | 画光标到屏幕 |

输入/删除走 buffer 层 → 一直正常。显示走 screenBuffer 层 → 丢失。所以 bug 现象是「输入删除正常但显示只有一个光标」。

### `main` 参数是什么

micro 里每个 `buffer.Cursor` 有 `Num` 字段，`Num == 0` 是主光标（用户最初那个），`Num > 0` 是 `SpawnMultiCursor` 加进来的次级光标。

终端里真光标只有一个，多光标靠两种机制拼出来（`bufwindow.go:416-424` 的 `showCursor`）：

| `main` 值 | 实现 | 数量 |
|---|---|---|
| `true`（`c.Num == 0`）| `screen.ShowCursor` → 终端硬件光标 | 全屏只有 1 个 |
| `false`（`c.Num > 0`）| `screen.ShowFakeCursorMulti` → cell 反转色模拟 | 可以任意多个 |

原生渲染入口（`bufwindow.go:724-728`）是「遍历 cursor + 就地画」，`main` 当场由 `c.Num == 0` 算出来，从来不存：

```go
for _, c := range cursors {
    if c.X == bloc.X && c.Y == bloc.Y && !c.HasSelection() {
        w.showCursor(w.X+vloc.X, w.Y+vloc.Y, c.Num == 0)  // ← main 当场算
    }
}
```

---

## 修复方案

**核心思路**：让 `screenBuffer` 存所有光标，blit 时全部画出来。最小侵入，只动 `screenBuffer` 的光标字段和 blit 路径，不动渲染入口。

### 设计原则：复用 micro 原生结构，不自己造字段

- 直接存 `*buffer.Cursor` 指针（复用 micro 原生类型，含 `Num`、`HasSelection` 等）
- **不自己造 `main bool` 字段** —— blit 时用 `c.Num == 0` 当场算，跟原生 `bufwindow.go:727` 写法一字不差
- 离屏必需的 sb 本地屏幕坐标单独存（原生没有这层概念，因为它是就地画）
- `buffer` 包已在 `bufwindow_md.go` import，无需新增依赖

### 新增数据结构

```go
// sbCursor 记录一个光标：复用 micro 原生 *buffer.Cursor（含 Num，blit 时算 main），
// 外加 sb 本地屏幕坐标（离屏渲染必需）。不自己造 main 字段，与原生就地画法保持一致。
type sbCursor struct {
    c       *buffer.Cursor // 复用 micro 原生 cursor（c.Num==0 即主光标）
    screenX int            // sb 本地 X（已减 originX）
    screenY int            // sb 本地 Y（已减 originY），= sb.rows 索引
}
```

### 改动清单（共 9 项 + 测试同步）

| # | 改动 | 位置 |
|---|---|---|
| 1 | 新增 `sbCursor` 结构体 | `screenRow` 后 |
| 2 | `screenBuffer` 字段：`cursorX/Y/OK` → `cursors []sbCursor` | 结构体定义 |
| 3 | `reset` 用 `s.cursors[:0]` 清空（复用底层数组）| `reset` 方法 |
| 4 | `ShowCursor` 签名：`(x, y, main bool)` → `(x, y, c *buffer.Cursor)`，改 append | `ShowCursor` 方法 |
| 5 | `setShowCursor` 签名同步，原生分支 `c.Num == 0` | `setShowCursor` 方法 |
| 6 | 渲染入口调用点：`c.Num == 0` → `c` | `renderSegmentNative` 内 |
| 7 | **blit 路径遍历画全部**（核心修复）| `showBuffer` 内 |
| 8 | 删除 `displayToBuffer` 入口的 `w.sb.cursorOK = false` | `displayToBuffer` 内 |
| 9 | blit 注释更新（`cursorY` → `sbCursor.screenY` + 多光标说明）| `showBuffer` 注释 |

### 核心修复代码

blit 路径（`showBuffer`）改前：

```go
if w.sb.cursorOK {
    curScreenY := w.sb.cursorY - startVY
    if curScreenY >= 0 && curScreenY < bufHeight {
        w.showCursor(w.X+w.sb.cursorX, w.Y+curScreenY, true)  // ← 只画一个，强制 main
    }
}
```

改后：

```go
for _, sc := range w.sb.cursors {
    curScreenY := sc.screenY - startVY
    if curScreenY >= 0 && curScreenY < bufHeight {
        w.showCursor(w.X+sc.screenX, w.Y+curScreenY, sc.c.Num == 0)  // ← 遍历，main 当场算
    }
}
```

### 坐标系（与 v1.0.6+ 原单光标逻辑完全一致）

- `ShowCursor(x, y)` 入参 = 绝对屏幕坐标（`w.X+vloc.X, w.Y+vloc.Y`）
- sb 内部存 `x - originX, y - originY`（相对窗口原点；`y` = sb.rows 索引）
- blit 时 `curScreenY = sc.screenY - startVY`（blit 起始偏移），`w.X + sc.screenX` 还原绝对 X

---

## 测试

### `TestScreenBuffer_ShowCursor` 重写

```go
func TestScreenBuffer_ShowCursor(t *testing.T) {
    s := &screenBuffer{originX: 10, originY: 5, width: 80}
    s.reset(50, 80, 10, 5)

    main := &buffer.Cursor{Num: 0}
    s.ShowCursor(15, 10, main) // 绝对 → 本地 (5, 5)
    if len(s.cursors) != 1 {
        t.Fatalf("len(cursors) = %d, want 1", len(s.cursors))
    }
    // ... 单 cursor 验证

    // 多光标追加：次级 cursor 应保留在 cursors[1]，main 标记不丢失
    sub := &buffer.Cursor{Num: 1}
    s.ShowCursor(25, 20, sub)
    if len(s.cursors) != 2 {
        t.Fatalf("len(cursors) = %d, want 2（次级 cursor 应追加）", len(s.cursors))
    }
    // ... 追加行为验证
}
```

### `TestScreenBuffer_NilReceiver` 签名同步

`ShowCursor(0, 0, true)` → `ShowCursor(0, 0, nil)`。

### 验证结果

```
✅ make build-quick      编译通过
✅ make build-dbg        带 debug 日志版本也通过
✅ TestScreenBuffer_ShowCursor     PASS
✅ TestScreenBuffer_NilReceiver    PASS
✅ 全量 internal/display 测试通过
```

---

## 边界情况

| 场景 | 行为 | 说明 |
|---|---|---|
| 单光标（默认）| `cursors` 长度 1，main=true | 与原行为一致 |
| 多光标全在可见区 | 全部画出 | 修复目标 |
| 多光标部分在可见区 | 只画可见的 | 渲染入口 `c.X==bloc.X && c.Y==bloc.Y` 天然过滤不可见 cursor |
| 多光标全不可见 | `cursors` 为空，不画 | 罕见（主光标一般总在可见区）|
| 同位置多个 cursor | 主光标走真光标、次级走 fake | `ShowFakeCursorMulti` 幂等，无需去重 |
| reset 复用底层数组 | `s.cursors[:0]` | `cap` 只增不减，`len` 不超 `cap`，安全 |

---

## 不改动的地方

- `buffer.Buffer.cursors`：buffer 层多光标，与显示无关
- `SpawnMultiCursorUpN` 等动作：输入/删除逻辑一直正常
- 渲染入口遍历 cursors 的判断逻辑（`c.X==bloc.X && c.Y==bloc.Y && !c.HasSelection()`）：本来正确
- `relocateVerticalMD` / `lastRowIndexOf`（v1.0.9 新增）：与光标显示无关

## 项目原则核查

- ✅ 所有改动在 `bufwindow_md.go`（microNeo 自有）+ `bufwindow_md_test.go`（测试）
- ✅ 不碰 `bufwindow.go`、`buffer.go`、`cursor.go` 等 micro 原生代码
- ✅ 不碰 `buffer.Buffer.cursors`、`SpawnMultiCursorUpN` 等输入逻辑
- ✅ 复用 micro 原生 `*buffer.Cursor`，不自己造字段

---

## 保留的原生行为（未改）

ESC 取消多光标后，光标落在**第一个/最早**的 cursor 位置，而非最后一个。

这是 micro 原生 `ClearCursors`（`buffer.go:1144`）的设计 —— 一直保留 `cursors[0]`：

```go
func (b *Buffer) ClearCursors() {
    for i := 1; i < len(b.cursors); i++ {
        b.cursors[i] = nil
    }
    b.cursors = b.cursors[:1]   // ← 保留第一个
    b.UpdateCursors()
    b.curCursor = 0             // ← 激活第一个
    ...
}
```

`git log -L` 确认这函数从 micro 上游 `5c8bf6b3` 以来核心逻辑没变。这跟我们这次修的多光标**显示** bug 完全无关。决定保持 micro 原生行为一致，不改。

---

## Commit

```
1cad1088 fix(md): multi-cursor display lost after screenBuffer refactor
```
