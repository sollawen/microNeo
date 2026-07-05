# `isTableRow` 奇数 / 未闭合配对符号导致表格行漏判

> 状态：**计划中**（PLAN 模式产出，未实现）
> 关联文件：`internal/md/detect.go`（`isTableRow()`，约 189–235 行）
> 关联现象：表格某行内含奇数个反引号（或其它「配对符号」未闭合）时，该行不再被识别为表格行，导致整个表格在该行处提前收尾、被切成多段。

---

## 1. 问题

### 现象

`docs/sample.md` line 61–66 是一个完整表格：

````markdown
| 场景 | 修复前（bug） | 修复后 |
|---|---|---|
| list 下一行紧跟 ` ```go ` | list 覆盖 codeblock，codeblock 错位/丢失 | list 正常收尾，codeblock 独立成块、顺序正确 |
| blockquote 紧跟 codeblock | 同上 | blockquote 收尾，codeblock 独立 |
| table 紧跟 codeblock | 同上 | table 收尾，codeblock 独立 |
| 中间有空行分隔 | 本就正常 | 仍正常（回归） |
````

实际渲染时，表格在 **line 63**（含 ` ```go ` 的那行）就提前结束，line 63–66 不再作为同一个表格渲染。

用户实测：把 line 63 里那几个反引号删掉，表格就能完整渲染 line 61–66。

### 直接原因

`isTableRow(line63)` 返回了 `false`。

`DetectSegments()` 在 `stateTable` 分支里，对每一行调用 `isTableRow(trimmed)` 判断是否「继续收集表格行」。一旦某行返回 `false`，表格段就在该行**前一行收尾**，当前行被回退成 `stateNormal` 重新处理（`detect.go` 约 165–172 行）：

```go
case stateTable:
    if isTableRow(trimmed) {
        // 继续收集表格行
    } else {
        segments = append(segments, Segment{
            BufStartLine: startLine,
            BufEndLine:   y - 1,   // ← 在前一行收尾
            Render:       RenderTable,
        })
        state = stateNormal
        reprocess = true
    }
```

所以 line 63 一旦漏判，表格就被切成「line 61–62」+「line 63–66 各自单独处理」的若干段，渲染框架自然拼不出一张完整的表。

---

## 2. 根因：`isTableRow` 的配对符号扫描会吞掉剩余行

### 当前实现（`detect.go` 约 189–235 行）

```go
func isTableRow(s string) bool {
    if len(s) == 0 || s[0] != '|' {
        return false
    }
    i := 1
    for i < len(s) {
        if s[i] == '|' {
            return true
        }
        // 跳过配对符号
        var close byte
        switch s[i] {
        case '`':  close = '`'
        case '"':  close = '"'
        case '\'': close = '\''
        case '(':  close = ')'
        case '[':  close = ']'
        case '{':  close = '}'
        default:
            i++
            continue
        }
        i++
        for i < len(s) && s[i] != close {   // ★ 找闭符号
            i++
        }
        i++                                  // ★ 越过闭符号
    }
    return false
}
```

设计意图：单元格内可能出现 `` `a|b` `` 这种「`|` 被包在配对符号里」的情况，这种 `|` 不应被当成列分隔符；所以扫描时遇到开符号就跳到闭符号之后。

### Bug：闭符号找不到时，i 直接冲到字符串末尾

内层循环 `for i < len(s) && s[i] != close` **找不到闭符号时**，`i` 会停在 `len(s)`，紧接着的 `i++` 让 `i = len(s)+1`，外层 `for i < len(s)` 直接退出 → **返回 `false`**。

也就是说：**只要出现一个找不到配对的开符号，整行剩余部分（包括真正的 `|` 列分隔符）全部被吞掉**。

### 逐字符 trace：line 63

```
| list 下一行紧跟 ` ```go ` | list 覆盖 codeblock ... |
                   ↑ ↑↑↑    ↑
                   1 23 4   5      ← 共 5 个反引号（奇数）
```

| 步骤 | i 指向 | 动作 |
|---|---|---|
| 起始 | `\|`（i=0） | 首字符是 `\|`，进入扫描，i=1 |
| 扫普通字符 | ` list 下一行紧跟 ` | 普通字符，i++ |
| 反引号 1 | `` ` `` | 开符号，close=`` ` ``，i++，扫下一个 `` ` `` |
| 反引号 2 | `` ` `` | 命中闭符号，i++（配对成功） |
| 反引号 3 | `` ` `` | 开符号，close=`` ` ``，i++，扫下一个 `` ` `` |
| 反引号 4 | `` ` `` | 命中闭符号，i++（配对成功） |
| 扫普通字符 | `go ` | i++ |
| **反引号 5** | `` ` `` | **开符号，close=`` ` ``，i++，扫闭符号……** |
| —— | （后面再无反引号） | **内层循环一路扫到 len(s)，i++ → 越界** |
| 外层退出 | — | **`return false`** ❌ |

后面的 ` | list 覆盖 ... |` 里那两个真正的列分隔符 `|` 完全没机会被检查。

> 同理，`'`、`"`、`(`、`[`、`{` 出现奇数次（或方向不对，比如只有 `)` 没有 `(`）也会触发同一 bug。

---

## 3. 修复方案

### 改动点

**仅** `internal/md/detect.go` 的 `isTableRow()` 内层循环一处。

### 修复逻辑

找不到闭符号时，把开符号当作**普通字符**，从 `start+1` 继续扫描——而不是吞掉剩余行。

### 完整的新 `isTableRow()`

```go
// isTableRow 判断是否为表格行。
// 判定规则：第一个字符必须是 |，且在配对符号之外至少还有一个 |
func isTableRow(s string) bool {
	if len(s) == 0 || s[0] != '|' {
		return false
	}

	// 从第一个 | 之后扫描，跳过配对符号包裹的内容，
	// 在跳过范围之外找至少一个 |
	i := 1
	for i < len(s) {
		if s[i] == '|' {
			return true
		}

		// 跳过配对符号
		var close byte
		switch s[i] {
		case '`':
			close = '`'
		case '"':
			close = '"'
		case '\'':
			close = '\''
		case '(':
			close = ')'
		case '[':
			close = ']'
		case '{':
			close = '}'
		default:
			i++
			continue
		}

		// 跳过开符号本身，然后找闭符号
		start := i
		i++
		found := false
		for i < len(s) {
			if s[i] == close {
				found = true
				i++ // 跳过闭符号
				break
			}
			i++
		}

		// 未找到闭符号：把开符号当普通字符，从下一个位置继续扫描。
		// 否则奇数个反引号（或其它未闭合配对符号）会让 i 冲到字符串末尾，
		// 吞掉剩余行里真正的 | 列分隔符，导致整行漏判为非表格行。
		if !found {
			i = start + 1
		}
	}
	return false
}
```

**与旧版的 diff**（仅内层循环）：

```diff
-		// 跳过开符号本身，然后找闭符号
-		i++
-		for i < len(s) && s[i] != close {
+		// 跳过开符号本身，然后找闭符号
+		start := i
+		i++
+		found := false
+		for i < len(s) {
+			if s[i] == close {
+				found = true
+				i++ // 跳过闭符号
+				break
+			}
 			i++
 		}
-		// 跳过闭符号
-		i++
+		// 未找到闭符号：把开符号当普通字符，从下一个位置继续扫描
+		if !found {
+			i = start + 1
+		}
```

### 语义说明

- Markdown 的 inline code span 必须配对。一个**没有**配对的开反引号，本就是字面文本（`` unresolved ` backtick ``），不应影响行类型判断。
- 这样改之后，配对成功时行为完全不变（`` `a|b` `` 里的 `|` 仍被跳过）；只有配对失败时才回退成「把开符号当普通字符」，符合直觉。

---

## 4. 验证

### 临时脚本结果（修复前 vs 修复后）

| 用例 | 修复前 | 修复后 |
|---|---|---|
| `\| col A \| col B \|` | ✅ true | ✅ true |
| **line 63**：`\| list 下一行紧跟 \` \`\`\`go \` \| list 覆盖 ... \|`（5 反引号） | ❌ **false** | ✅ **true** |
| `\| 场景 \| 修复前（bug） \| 修复后 \|` | ✅ true | ✅ true |
| `\|---\|---\|---\|` | ✅ true | ✅ true |
| `\| \`a\|b\` \| c \|`（code span 内有 `\|`，配对正常） | ✅ true | ✅ true（回归通过） |
| `\| unmatched \` backtick \| data \|`（单个未配对反引号） | ❌ false | ✅ true |
| `\| \`unmatched end`（只有一个 `\|`） | ✅ false | ✅ false（回归通过） |

### 不受影响的部分

- **`splitCells`（render_table.go）**：按 `\|` 转义拆分单元格，line 63 的反引号内**不含** `|`，拆分正常，无需改动。
- **`renderInline`**：line 63 第一个 cell `list 下一行紧跟 ` ```go `` 含奇数反引号，会被当成未闭合的 code span 起始，可能显示样式略有出入；但这属于**显示层**的独立小问题，不影响「整行被识别为表格」，不在本次修复范围。

---

## 5. 测试计划

在 `internal/md/detect_test.go` 增加一条 table 用例，覆盖「表格行内含奇数反引号」的场景，防回归：

```go
func TestDetectTableRowWithOddBackticks(t *testing.T) {
    buf := &mockBuffer{lines: []string{
        "| 场景 | 修复前 | 修复后 |",
        "|---|---|---|",
        "| list 紧跟 ` ```go ` | 覆盖 | 独立 |",   // 5 个反引号
        "| blockquote 紧跟 | 同上 | 独立 |",
    }}
    segments := DetectSegments(buf, 0, 3)
    if len(segments) != 1 {
        t.Fatalf("expected 1 table segment, got %d", len(segments))
    }
    if segType := getSegmentType(segments[0]); segType != typeTable {
        t.Fatalf("expected table, got %v", segType)
    }
    if segments[0].BufStartLine != 0 || segments[0].BufEndLine != 3 {
        t.Fatalf("expected [0,3], got [%d,%d]",
            segments[0].BufStartLine, segments[0].BufEndLine)
    }
}
```

并可直接加一个 `isTableRow` 单测表（仿 `TestIsListItem` / `TestDetectHR` 的 table-driven 写法），把第 4 节的用例固化下来。

---

## 6. 影响面

- **改动范围**：`internal/md/detect.go` 的 `isTableRow()` 一处，约 6 行。
- **风险**：极低。配对成功路径行为不变；只有原本被错误判为 `false` 的「未闭合配对符号」行才会改变结果（从 `false` → `true`），且这些行本就是合法表格行。
- **对 micro 原生代码的侵入**：零（改动隔离在 `internal/md/` 内）。
