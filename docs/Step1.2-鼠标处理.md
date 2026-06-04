# Step 1.2：鼠标处理

> 前置：Step 1.1 已完成（editMode 状态 + 键盘拦截 + per-segment 回退）
> 交付：阅读模式下鼠标点击进入编辑模式

---

## 验收标准

阅读模式下：
- 鼠标左键/右键点击 → 进入编辑模式
- 鼠标滚轮 → 正常滚动（不切换模式，已有 ScrollUp/ScrollDown 绑定）
- 鼠标拖选 → 进入编辑模式

编辑模式下：鼠标行为与原生一致。

---

## 改动清单

### 1. HandleEvent 鼠标拦截

**文件**：`internal/action/bufpane.go`

在 `case *tcell.EventMouse:` 分支开头加拦截：

```go
case *tcell.EventMouse:
    // MicroNeo Step 1.2: 阅读模式下鼠标点击 → 编辑模式
    if h.Buf.IsMD && !h.editMode {
        buttons := e.Buttons()
        wheel := tcell.WheelUp | tcell.WheelDown | tcell.WheelLeft | tcell.WheelRight
        if buttons != 0 && buttons&wheel == 0 {
            // 非滚轮的鼠标按键 → 切换到编辑模式
            h.enterEditMode()
            // 不 return，继续走原生鼠标处理
            // 光标会按原生逻辑定位到点击位置
            // 精确定位需要 Step 3 的渲染片坐标查找
        }
    }

    // ... 原有鼠标处理逻辑不变 ...
```

**关于光标定位**：点击后原生鼠标处理会把光标放在一个位置，但渲染模式下 screen↔buffer 的行映射和原生不同（渲染片可能改变行高）。Step 1.2 的简化处理：
- 点击进入编辑模式
- 光标定位到原生逻辑算出的位置（可能不精确）
- 精确定位留给 Step 3（渲染片坐标查找表）

### 2. EventPaste 门控

**文件**：`internal/action/bufpane.go`

某些终端支持直接粘贴（不经过按键），需要在 `EventPaste` 分支也加 editMode 门控：

```go
case *tcell.EventPaste:
    // MicroNeo Step 1.2: 阅读模式下 paste → 编辑模式
    if h.Buf.IsMD && !h.editMode {
        h.enterEditMode()
    }

    // ... 原有 paste 逻辑不变 ...
```

---

## 改动文件

| 文件 | 改动 |
|------|------|
| `internal/action/bufpane.go` | EventMouse 分支加点击拦截、EventPaste 分支加门控 |

## 风险

| 风险 | 缓解 |
|------|------|
| 点击后光标定位不精确 | Step 3 解决，1.2 先接受 |
| 拖选范围在渲染模式下可能不对应视觉区域 | Step 3 解决 |

**估计代码量**：~15-20 行
