package finder

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
)

// 显示宽度工具（CJK/全角字符占 2 列，rune-safe）
//
// 终端写屏必须按显示列宽而非 rune 数推进列坐标：
// 一个中文 rune 调 SetContent 后会占 2 列，若 col 只 +1，
// 下一个字符的 SetContent 会覆盖中文的第 2 列，表现为"隔一个少一个"。

// runeWidth 返回单个 rune 的显示列宽（CJK/全角=2，ASCII=1）。
// 宽度 0 的组合字符兑底为 1，避免原地覆盖。
func runeWidth(r rune) int {
	w := runewidth.RuneWidth(r)
	if w < 1 {
		return 1
	}
	return w
}

// stringWidth 返回字符串的显示列宽总和。
func stringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// tailByWidth 返回 s 的尾部子串，其显示宽度 ≤ maxW（按列宽从末尾向前取整字符）。
func tailByWidth(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	runes := []rune(s)
	w := 0
	start := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		rw := runeWidth(runes[i])
		if w+rw > maxW {
			break
		}
		w += rw
		start = i
	}
	return string(runes[start:])
}

// headByWidth 返回 s 的头部子串，其显示宽度 ≤ maxW（按列宽从头部向后取整字符）。
func headByWidth(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	runes := []rune(s)
	w := 0
	end := 0
	for i := 0; i < len(runes); i++ {
		rw := runeWidth(runes[i])
		if w+rw > maxW {
			break
		}
		w += rw
		end = i + 1
	}
	return string(runes[:end])
}

// isHiddenName 判断 Unix dotfile（以 . 开头的名字视为隐藏）。
func isHiddenName(name string) bool {
	return len(name) > 0 && name[0] == '.'
}

// truncateLeftPath 左截断路径到 maxW 列，恒保留尾段"当前目录/"。
//
// 在路径段（/）边界处截断，截断处用 "…/" 标记。
// path 须以路径分隔符结尾。
// maxW 是显示列宽预算（非 rune 数）：一个 CJK 字符占 2 列。
func truncateLeftPath(path string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if stringWidth(path) <= maxW {
		return path
	}
	sep := string(filepath.Separator)
	trimmed := strings.TrimSuffix(path, sep)
	parts := strings.Split(trimmed, sep) // 绝对路径 parts[0]==""（根）；相对路径无空首段
	if len(parts) == 0 {
		return path
	}
	ellW := runeWidth('…')
	if maxW <= ellW {
		return "…"
	}

	// "…/" 标记宽度 = … + 分隔符
	markerW := ellW + runeWidth(filepath.Separator)
	curSeg := parts[len(parts)-1] + sep
	curW := stringWidth(curSeg)

	// 连 "…/" 都放不下：只显示 … + 当前段尾
	if maxW < markerW {
		avail := maxW - ellW
		if avail <= 0 {
			return "…"
		}
		return "…" + tailByWidth(curSeg, avail)
	}

	budget := maxW - markerW // 给 kept 的总列宽预算

	// 当前段放不下 budget：… + 段尾
	if curW > budget {
		avail := maxW - ellW
		if avail <= 0 {
			return "…"
		}
		return "…" + tailByWidth(curSeg, avail)
	}

	// 从末尾向前累加完整段，直到下一段放不下
	kept := curSeg
	used := curW
	for i := len(parts) - 2; i >= 0; i-- {
		seg := parts[i] + sep
		segW := stringWidth(seg)
		if used+segW > budget {
			break
		}
		kept = seg + kept
		used += segW
	}
	return "…" + sep + kept
}

// truncateNameKeepExt 把 name 截断到 maxCols 显示列宽，保留文件扩展名。
//
//	超长 → 右侧加 …，保留 basename 头部 + "扩展名"（最后一个 . 之后）可见（对齐 yazi）。
//	无扩展名/目录 → 保留头部 + …。
//	isDir=true 时不按扩展名处理（目录名里的 . 不是扩展名），直接保留头部。
//	按 CJK 显示列宽计算（中文占 2 列），rune-safe。
func truncateNameKeepExt(name string, maxCols int, isDir bool) string {
	if maxCols <= 0 {
		return ""
	}
	if stringWidth(name) <= maxCols {
		return name
	}
	ellW := runeWidth('…')
	if maxCols <= ellW {
		return "…"
	}
	r := []rune(name)
	headBudget := maxCols - ellW
	if headBudget <= 0 {
		return "…"
	}

	keepHead := func() string {
		return headByWidth(name, headBudget) + "…"
	}

	if isDir {
		return keepHead()
	}
	// 找扩展名（basename 内最后一个 .）
	dotIdx := -1
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] == '.' {
			dotIdx = i
			break
		}
	}
	if dotIdx <= 0 {
		return keepHead() // 无扩展名
	}
	ext := string(r[dotIdx:]) // 含 .
	extW := stringWidth(ext)
	basenameBudget := headBudget - extW
	if basenameBudget <= 0 {
		return keepHead() // 扩展名放不下，退化保头
	}
	head := string(r[:dotIdx])
	return headByWidth(head, basenameBudget) + "…" + ext
}

// humanSize 把字节数格式化为人类可读字符串（输出恒 ≤5 列）。
// 对齐 ls -lh 的 1024 进制规则。
func humanSize(n int64) string {
	if n < 0 {
		n = 0
	}

	const unit1024 = int64(1024)
	units := []string{"B", "K", "M", "G", "T", "P"}

	if n < unit1024 {
		return fmt.Sprintf("%dB", n)
	}

	// 除到 mantissa < unit1024
	mantissa := float64(n)
	unitIdx := 0
	for mantissa >= float64(unit1024) && unitIdx < len(units)-1 {
		mantissa /= float64(unit1024)
		unitIdx++
	}

	unit := units[unitIdx]
	if mantissa < 100 {
		s := fmt.Sprintf("%.1f%s", mantissa, unit)
		if stringWidth(s) <= 5 {
			return s
		}
		// 否则 99.9x 进位到 100.0 → 落到下面的整数分支
	}
	// mantissa ≥ 100：整数，去小数（按四舍五入取最近整数）
	// 1023.6→1024K → 进位 1.0M；999.9→1000K；1023.0→1023K
	intPart := int64(mantissa + 0.5) // 四舍五入
	// 进位：进位后值 ≥ 1024（如 1024K → 1.0M）
	if intPart >= unit1024 && unitIdx < len(units)-1 {
		return fmt.Sprintf("1.0%s", units[unitIdx+1])
	}
	return fmt.Sprintf("%d%s", intPart, unit)
}

// formatMtime 把修改时间格式化为对齐 ls -lh 的字符串。
// 同年 → "MM-DD HH:MM"（11 列）；跨年 → "YYYY-MM-DD"（10 列）。
func formatMtime(t time.Time) string {
	now := time.Now()
	year, _, _ := t.Date()
	nowYear, _, _ := now.Date()
	if year == nowYear {
		return t.Format("01-02 15:04") // 11 列
	}
	return t.Format("2006-01-02") // 10 列（跨年省略时分）
}

// fitMeta 按 w 宽挑能放下的组合；砍字段优先级 权限→size，mtime 保底。
func fitMeta(w int, perms, size, mtime string) string {
	type f struct{ k, v string }
	order := []f{{"p", perms}, {"s", size}, {"m", mtime}}
	keep := map[string]bool{"p": true, "s": true, "m": true}
	drop := []string{"p", "s"} // m 永不砍
	for {
		var parts []string
		for _, x := range order {
			if keep[x.k] {
				parts = append(parts, x.v)
			}
		}
		line := strings.Join(parts, "  ")
		if stringWidth(line) <= w {
			return line
		}
		killed := false
		for _, d := range drop {
			if keep[d] {
				keep[d] = false
				killed = true
				break
			}
		}
		if !killed {
			return tailByWidth(line, w) // 只剩 mtime 仍超宽（极窄 pane），左截断保底
		}
	}
}
