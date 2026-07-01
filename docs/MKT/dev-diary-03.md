# microNeo 开发日记 #3：markdown的表格列宽怎么计算

既然我们自己给 microNeo 开发表格渲染器 tableRender，那么在渲染的时候正确的计算每列的宽度是一个很关键的问题：
- 一个 markdown 表格，3 列，最长一列 8 字符。
- 终端只有 20 字符宽。表格必须塞下，每列还得能看。
- 如何决定三列的宽度？

---

## 最初的想法：原始内容的宽度

最直观的选择：每列的宽度 = 这一列最长 cell 的字符数。
- 表头 `Age` 占 3 字符，这一列就是 3 字符；
- 正文 `Elizabeth` 占 9 字符，这一列就是 9 字符。

宽屏下没问题——`sum ≤ avail` 时直接用自然宽度。但如果内容长度合超过屏宽了，就溢出了。

v1.0.0 的 microNeo 就是这样：只算自然宽度，屏宽 < 总自然宽度时多出来的字符被截断，最右一列直接消失。这是「最简单的版本能跑起来」的代价。

**自然宽度是上限，不是答案。** 它假设屏宽永远够——可终端永远不够。

## 第二版的尝试：等比缩放

思路：每列按比例缩小。

```
ratio = avail / sum
new_width[i] = nat_width[i] * ratio
```

3 列自然宽度 [3, 8, 6] sum=17，avail=10，ratio=0.588：

```
[3*0.588, 8*0.588, 6*0.588] ≈ [1.76, 4.7, 3.53]
                                → [2, 5, 4]   sum ≈ 11
```

看起来每列都「按比例分摊」了。但仔细看——

- 第 1 列：3 → 2（-1 字符）
- 第 2 列：8 → 5（-3 字符）
- 第 3 列：6 → 4（-2 字符）

第 2 列得到了 3 字符，第 1 列只得到 1 字符——听起来很公平。

但**对短列来说，等比缩放就给缩没了**

第 1 列本来只有 3 字符宽，缩到 2 字符后能放什么？1 个汉字 + 1 个 padding，连 `id` 这种 2 字符 ID 都写不下。它**从「能看」直接变成「不能看」**。而第 2 列从 8 字符缩到 5 字符——还过得去。

长的cell内容折行后看上去还可以。但是短的cell内容再折行后就完全没法看了。

## 第三版：水位线算法：把列想成高低不同的楼房

怎么改？**保护短列，让长列多让。**

把这个思路想成物理场景：3 列是 3 栋高度不同的楼房——3、6、8 层。我们要削顶到3栋合并不超过 10 层。规则：

- **最高的先削**。第 2 栋从 8 削到 6。
- **两栋并排最高时一起削**。第 2 栋和第 3 栋都到了 6 层——下一刀开始它们一起被削。
- **最矮的永远最后才被削**。第 1 栋（3 层）最矮，它能撑到只剩它自己「高出水面」的时候才被削。

关键是「**同高度同时被削**」——长列先被罚，短列后被罚，**短的永远比长的活得久**。

这就是**水位线算法（waterline algorithm）**。它在信息论里叫 water-filling，在调度里叫 max-min fairness——本质是「**保护弱者**」的分配规则。当然，这不是我发明的，这是无数程序猿先烈们总结出来的。

## 最终代码

代码在 `internal/md/render_table.go` 的 `waterlineShrink` 函数里，约 50 行。核心循环 15 行：

```go
// entries 已按 width 降序排好（带原始列号）
for i := 0; i < len(entries)-1 && toShrink > 0; i++ {
    gap := entries[i].width - entries[i+1].width
    canRelease := gap * prefixCount   // 这一步最多能减多少

    if canRelease >= toShrink {
        // 够了：当前 prefixCount 列各缩 toShrink/prefixCount
        perCol := toShrink / prefixCount
        rem := toShrink % prefixCount
        for j := 0; j < prefixCount; j++ {
            result[entries[j].origIdx] -= perCol
            if j < rem {
                result[entries[j].origIdx]--   // 余数摊给前几列
            }
        }
        toShrink = 0
        break
    }

    // 不够：把 prefixCount 列都拉到 entries[i+1].width
    for j := 0; j < prefixCount; j++ {
        result[entries[j].origIdx] = entries[i+1].width
    }
    toShrink -= canRelease
    prefixCount++   // 下一轮多一列参与
}
```

走一遍刚才的 3 列 [3, 8, 6]、target=10、toShrink=7：

排好序的 entries（带原始列号）：

```
entries = [{idx:1, w:8}, {idx:2, w:6}, {idx:0, w:3}]
```

跑循环：

```
i=0, prefixCount=1
  gap = 8-6 = 2, canRelease = 2
  2 < 7，不够。idx:1 那列从 8 拉到 6。
  result = [3, 6, 6], toShrink = 5, prefixCount = 2

i=1, prefixCount=2
  gap = 6-3 = 3, canRelease = 6
  6 >= 5，够了。
  perCol = 5/2 = 2, rem = 1
  idx:1 那列 -= 2 再 -= 1（共 -3）：6 → 3
  idx:2 那列 -= 2（不减 1）：6 → 4
  result = [3, 3, 4], sum = 10  ✓
```

对比一下两种算法在 3 列 [3, 8, 6]、target=10 上的结果：

| 列 | 自然宽度 | 等比缩放 | 水位线 |
|---|---|---|---|
| 1（短）| 3 | 2 | **3** |
| 2（长）| 8 | 5 | **3** |
| 3（中）| 6 | 4 | **4** |
| **sum** | 17 | ≈11 | **10** |

等比缩放后最短那列只剩 2 字符。**水位线后，最短那列还是 3 字符——它没掉一字。**

> **长列多让、短列保护——这就是水位线算法的全部。**

## 边界条件：知道什么时候该放弃

水位线假设了一个最低水位（代码里钳到 2 字符）。如果终端窄到 `avail < 2 * numCols`——每列连 2 字符都分不到——硬塞只会得到一堆看不清的 `...`。

`allocColWidths` 的处理很直接：

```go
// 情况 C：极端窄屏（每列连 2 个字符都分不到）
if totalAvail <= len(natWidths)*2 {
    return nil
}
```

返回 `nil` 不是「我算了但没算出来」，是「**算出来也没用**」。`RenderTable` 看到这个 `nil` 就放弃——直接 fallback 渲染成原始文本（v1.0.0 的行为），让用户至少看到表格的原始 markdown，至少能编辑。

> **算法有尊严地认输，比硬塞出畸形结果体面。**

---
至此，TableRender 就算是完工了。这也是microNeo的几个渲染器里面相对比较复杂的一个Render了。

