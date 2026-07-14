# Alt-= / Alt-- 边界溢出到 :big / :small —— 方案

需求来源：`:big` / `:small` 已实现（见 `Z1c-pane-tab互转命令.md`），但只能靠命令或未来键位触发。本方案把 `Alt-=` / `Alt--` 的「边界溢出」作为 `:big` / `:small` 的天然入口——缩放到极限再按一次，自然过渡到 pane↔tab 转换，不新增键位。

---

## 1. 现状（已查证）

- `Alt-=` → `GrowPane`，`Alt--` → `ShrinkPane`（`defaults_darwin.go:58-59` / `defaults_other.go:61-62`）。
- `GrowPane` / `ShrinkPane` 调 `stepPaneRatio(grow)`（`command_neo.go:298`）算离散档位 `{0.25, 0.5, 0.75}`，像素空间比较。
- `stepPaneRatio` 有两种 `return -1`：
  - `p == nil`（单 pane 无 split）→ `InfoBar.Message("no other split")`
  - 已在最大 / 最小档位 → `InfoBar.Message("pane already at max")` / `"pane already at min")`
- `GrowPane` / `ShrinkPane` 拿到 `-1` 就 `return false`（no-op）。边界按键无效果，只弹消息。
- `:big` / `:small` 已实现（`command_neo.go`，`Z1c-pane-tab互转命令.md`）：`BigPane` 把当前 pane promote 成独立全屏 tab；`SmallPane` 路由 4 场景（demote / absorb / HSplit）。

---

## 2. 目标行为

| 键 | 当前档位 | 行为 |
|---|---|---|
| `Alt-=` | < 75% | 放大到下一档位（现状不变） |
| `Alt-=` | >= 75% | **溢出：`:big`**（promote 当前 pane 到独立全屏 tab） |
| `Alt-=` | 单 pane（无 split） | no-op（`BigPane` 单 pane 打 `"pane already at max"`） |
| `Alt--` | > 25% | 缩小到下一档位（现状不变） |
| `Alt--` | <= 25% | **溢出：`:small`**（demote 当前 pane 到新 tab） |
| `Alt--` | 本tab里单pane | :small |


核心语义：`Alt-=` 是「放大」，放到极限（75%）再按就「放成全屏 tab」；`Alt--` 是「缩小」，缩到极限（25%）再按就「缩走」（demote 到新 tab）。连续按键从档位 stepping 自然过渡到 pane↔tab 转换，无需新键位、无需记新命令。

单 pane 时没有 25% / 75% 概念，两键分流：`Alt-=` 已是全屏，no-op；`Alt--` 缩无可缩，溢出到 `:small`（有其他 tab 则吞一个 pane 进来配对，无则 HSplit 开配对 pane）。`Alt--` 单 pane 走场景 4 时与 `Ctrl-t`（HSplit）行为重合，但语义自洽——`Alt--` 连按从「缩小档位」自然过渡到「让出空间开配对」。

---

## 3. 设计

### 3.1 思路：区分两种 `-1`

当前 `stepPaneRatio` 把「单 pane」和「档位边界」都返回 `-1`，`GrowPane` / `ShrinkPane` 无法区分该 no-op 还是该溢出。

改 `stepPaneRatio` 返回 `(size int, limit paneLimit)`，`limit` 直接标明「到放大极限」还是「到缩小极限」，调用方按 `limitMax→:big` / `limitMin→:small` 直觉映射，不用靠自身 grow/shrink 身份推断：

```go
type paneLimit int

const (
	limitNone paneLimit = iota  // 正常档位，用 size resize
	limitMax                    // 已到放大极限（75% 或单 pane 全屏）
	limitMin                    // 已到缩小极限（25% 或单 pane）
)
```

| 情况 | size | limit | 调用方动作 |
|---|---|---|---|
| 单 pane + grow（`Alt-=`） | -1 | `limitMax` | `BigPane()` |
| 单 pane + shrink（`Alt--`） | -1 | `limitMin` | `SmallPane()` |
| ≥2 pane + grow（`Alt-=`） | -1 | `limitMax` | `BigPane()` |
| ≥2 pane + shrink（`Alt--`） | -1 | `limitMin` | `SmallPane()` |
| 正常档位 | 像素值 | `limitNone` | `ResizePane(size)` |

`stepPaneRatio` 变纯计算（去掉边界消息），边界决策上移到 `GrowPane` / `ShrinkPane`。

### 3.2 代码

`stepPaneRatio`（改签名 + 去掉边界消息，边界 return `(-1, limitMax/limitMin)`）：

```go
// stepPaneRatio 算当前 pane 的下一个离散档位。
// grow=true 放大，grow=false 缩小。档位 {0.25, 0.5, 0.75}，按像素比较。
// 返回 (targetSize, limit)：
//   - 单 pane + grow：(-1, limitMax)
//   - 单 pane + shrink：(-1, limitMin)
//   - 已到放大极限（grow，75%）：(-1, limitMax)
//   - 已到缩小极限（shrink，25%）：(-1, limitMin)
//   - 正常档位：(像素值, limitNone)
func (h *BufPane) stepPaneRatio(grow bool) (int, paneLimit) {
	ratios := []float64{0.25, 0.5, 0.75}

	n := h.tab.GetNode(h.splitID)
	p := n.Parent()
	if p == nil {
		// 单 pane 全屏：grow 视为已到放大极限，shrink 视为已到缩小极限
		if grow {
			return -1, limitMax
		}
		return -1, limitMin
	}

	children := p.Children()
	isFirst := len(children) > 0 && children[0] == n

	var cur, total int
	if p.Kind == views.STVert {
		cur = n.H
		total = p.H
	} else {
		cur = n.W
		total = p.W
	}

	// 档位用「像素」比较，不用比例 cur/total：pane 尺寸是整数像素，按 resize 实际产生的
	// 截断方式算每个档位对应的像素，cur 就能精确命中某个档位，无需 epsilon 容差。resize 对
	// 第一个子节点取 int(ratio*total)（截断），对第二个是 total - int((1-ratio)*total)，
	// stepPixel 必须照搬这套——否则 cur 和档位像素会差 1，导致 grow/shrink 卡在原地。
	stepPixel := func(r float64) int {
		if isFirst {
			return int(r * float64(total))
		}
		return total - int((1-r)*float64(total))
	}

	var target float64
	found := false
	if grow {
		for _, s := range ratios {
			if stepPixel(s) > cur {
				target = s
				found = true
				break
			}
		}
		if !found {
			return -1, limitMax   // 已到放大极限（75%）
		}
	} else {
		for i := len(ratios) - 1; i >= 0; i-- {
			if stepPixel(ratios[i]) < cur {
				target = ratios[i]
				found = true
				break
			}
		}
		if !found {
			return -1, limitMin   // 已到缩小极限（25%）
		}
	}

	// size 是传给 ResizeSplit 的「第一个子节点」尺寸：当前 pane 若是第一个直接用 target，
	// 若是第二个取 1-target（vResizeSplit/hResizeSplit 恒把 c1=children[0] 设成 size）。
	ratio := target
	if !isFirst {
		ratio = 1.0 - target
	}
	return int(ratio * float64(total)), limitNone
}
```

`GrowPane` / `ShrinkPane`（边界分流）：

```go
// GrowPane 放大当前 pane 到下一档位；到放大极限（75% 或单 pane）时溢出到 :big。
// 单 pane 时 BigPane 自身 no-op（"pane already at max"），无需在此特判。
func (h *BufPane) GrowPane() bool {
	size, lim := h.stepPaneRatio(true)
	if lim == limitMax {
		return h.BigPane()
	}
	h.ResizePane(size)
	return true
}

// ShrinkPane 缩小当前 pane 到下一档位；到缩小极限（25% 或单 pane）时溢出到 :small。
func (h *BufPane) ShrinkPane() bool {
	size, lim := h.stepPaneRatio(false)
	if lim == limitMin {
		return h.SmallPane()
	}
	h.ResizePane(size)
	return true
}
```

### 3.3 语义验证

- **75% 按 `Alt-=`**：`limitMax` → `BigPane` → `extractPaneToNewTab(tab, h, true)`，当前 pane promote 成独立全屏 tab，焦点跟随到新 tab。promote 后该 tab 单 pane，再按 `Alt-=` → `limitMax` → `BigPane` 单 pane no-op（"pane already at max"）。✓
- **25% 按 `Alt--`**：`limitMin` → `SmallPane` → 场景 1（≥2 pane）→ `extractPaneToNewTab(tab, h, false)`，当前 pane demote 到新 tab，焦点留在原 tab 的剩余 pane。✓
- **单 pane 按 `Alt-=`**：`limitMax` → `BigPane` → 单 pane no-op（"pane already at max"，已全屏没法再大）。✓
- **单 pane 按 `Alt--`**：`limitMin` → `SmallPane` → 路由场景 2/3（有其他 tab → 吞一个 pane 进来配对）或场景 4（无其他 tab → HSplit 开配对 pane）。✓
- **正常档位**（如 50% 按 `Alt-=`）：`limitNone` → `ResizePane(size)`，行为与现状完全一致。✓

---

## 4. 涉及文件

- `internal/action/command_neo.go`：新增 `paneLimit` 类型（包级定义）；`stepPaneRatio`（签名 + 去消息）、`GrowPane`、`ShrinkPane`。
- 不碰 `defaults_darwin.go` / `defaults_other.go`（键位绑定不变）。
- 不碰 `bufpane.go`（`GrowPane`/`ShrinkPane` 方法签名 `func (h *BufPane) bool` 不变，`BufKeyActions` 注册不动；`ResizePane` 原子不动）。

---

## 5. 边界 / 风险

1. **promote / demote 后焦点去向**：`:big` 用 `activate=true`（焦点跟到新 tab），`:small` 场景 1 用 `activate=false`（焦点留原 tab 剩余 pane）。符合「放大跟随、缩小留下」的直觉——放大是「我要更大的空间」，焦点跟着走；缩小是「我让出这个 pane」，焦点留给留下来的。
2. **单 pane `Alt--` 溢出到 `:small`**：单 pane 无更小档位可缩，直接溢出——有其他 tab 则吞一个 pane 进来配对（场景 2/3），无其他 tab 则 HSplit 开配对 pane（场景 4）。场景 4 与 `Ctrl-t`（HSplit）行为重合，但语义自洽：`Alt--` 连按从缩小档位自然过渡到让出空间开配对。`Alt-=` 单 pane 则 no-op（已全屏），两键不对称——放大到头是终点，缩小到头是转换。
3. **边界消息移除**：`stepPaneRatio` 不再打任何消息（`"already at max/min"`、`"no other split"` 都去掉）——边界时直接返回 `limitMax`/`limitMin`，由 `BigPane`/`SmallPane` 的结果接管。单 pane `Alt-=` 走 `limitMax` → `BigPane`，BigPane 单 pane 时打 `"pane already at max"`（比原来的 `"no other split"` 更贴切：单 pane = 已全屏）。
4. **不改键位**：`Alt-=` / `Alt--` 仍绑 `GrowPane` / `ShrinkPane`，溢出是 action 内部行为，`bindings.json` 无感知，用户自定义键位不受影响。
5. **`SmallPane` 各场景的入口**：25% 边界（2-pane tab）→ 场景 1（demote 当前 pane）；单 pane `Alt--` → 场景 2/3/4（吞 pane 或 HSplit）。即 `Alt--` 溢出现在覆盖场景 1/2/3/4 全部，`Alt-=` 溢出只覆盖 `:big`（单场景）。场景 2/3/4 也可由 `:small` 命令直接触发。

---

## 6. 实施顺序

1. 新增 `paneLimit` 类型（`limitNone`/`limitMax`/`limitMin`）；改 `stepPaneRatio` 签名返回 `(int, paneLimit)`，去掉所有边界消息，边界 `return (-1, limitMax/limitMin)`，单 pane 按 grow 返回 `limitMax`/`limitMin`。
2. 改 `GrowPane` / `ShrinkPane`：`limitMax` → `BigPane`、`limitMin` → `SmallPane`，其余 `ResizePane`（去掉 `size < 0` 特判分支，单 pane 由 `BigPane` 自身 no-op）。
3. `make build-quick` 编译。
4. 冒烟测试：
   - 2-pane tab，`Alt-=` 连按到 75% 再按一次 → 当前 pane promote 成全屏新 tab，焦点跟随。
   - 2-pane tab，`Alt--` 连按到 25% 再按一次 → 当前 pane demote 到新 tab，焦点留原 tab。
   - 单 pane tab，`Alt-=` → `"pane already at max"` no-op；`Alt--` → `:small`（有其他 tab 吞 pane，无则 HSplit 开配对）。
   - promote / demote 后再按 `Alt-=` / `Alt--` → 行为符合预期（单 pane `Alt-=` no-op，`Alt--` 继续 `:small`，或新 2-pane 继续 stepping）。
5. 更新 `CHANGELOG.md`（用户可见的键位行为变化）。
