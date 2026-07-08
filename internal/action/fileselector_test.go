package action

import (
	"fmt"
	"testing"
	"time"
)

func TestTruncateNameKeepExt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		isDir    bool
		expected string
	}{
		{"fits as-is", "main.go", 8, false, "main.go"},
		{"fits exact", "ab.go", 5, false, "ab.go"},
		{"truncate keep ext", "verylongname.go", 10, false, "…ngname.go"},
		{"no ext keep tail", "verylongname", 8, false, "…ongname"},
		{"single char cap", "whatever.go", 1, false, "…"},
		{"zero cap", "x.go", 0, false, ""},
		{"dir keeps tail", "myproject/", 6, true, "…ject/"},
		{"dir with dot not treated as ext", "v2.3.4/", 6, true, "….3.4/"},
		{"ext too long degrades to tail", "a.superlongextension", 8, false, "…tension"},
		{"dotfile (leading dot) no real ext", ".gitignore", 5, false, "…nore"},
		{"chinese filename", "我的超长项目文件名.go", 8, false, "…件名.go"},
		{"chinese fits as-is", "项目.go", 7, false, "项目.go"},            // 显示宽 2+2+1+1+1=7
		{"chinese dir keeps tail", "我的项目目录/", 8, true, "…目目录/"},     // 目录宽 13，预算 8 → …+尾 7列（含两个目）
		{"chinese no ext keep tail", "我的超长项目文件名", 6, false, "…件名"},   // 全中文宽 18，无扩展名，预算 6 → …件名(5列)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateNameKeepExt(tt.input, tt.maxRunes, tt.isDir)
			if got != tt.expected {
				t.Errorf("truncateNameKeepExt(%q, %d, %v) = %q, want %q",
					tt.input, tt.maxRunes, tt.isDir, got, tt.expected)
			}
			// 保证结果显示宽度不超预算（CJK 占 2 列）
			if dw := stringWidth(got); dw > tt.maxRunes {
				t.Errorf("result %q (display width %d) exceeds max %d cols", got, dw, tt.maxRunes)
			}
		})
	}
}

func TestTruncateLeftPath(t *testing.T) {
	sep := "/"
	tests := []struct {
		name     string
		path     string
		maxW     int
		expected string
	}{
		{"fits", "/home/user/proj/", 17, "/home/user/proj/"},
		{"truncate keep current seg", "/home/user/proj/src/components/md/", 22, "…/src/components/md/"},
		{"truncate to parent only", "/home/user/proj/src/components/md/", 16, "…/components/md/"},
		{"truncate hard", "/home/user/proj/src/components/md/", 8, "…/md/"},
		{"root", "/", 1, "/"},
		{"root wide", "/", 10, "/"},
		{"current seg alone fits with ellipsis", "/a/verylongdirname/", 12, "…ongdirname/"},
		{"zero width", "/x/y/", 0, ""},
		{"single width", "/x/y/", 1, "…"},
		{"chinese path keeps current seg", "/home/我的项目/源码/", 16, "…/我的项目/源码/"},  // 16列刚好装下两段
		{"chinese path truncate hard", "/home/我的项目/源码/", 8, "…/源码/"},   // 仅够 …/ + 当前段
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 本测试用 / 作分隔符，统一输入
			got := truncateLeftPath(tt.path, tt.maxW)
			_ = sep
			if got != tt.expected {
				t.Errorf("truncateLeftPath(%q, %d) = %q, want %q",
					tt.path, tt.maxW, got, tt.expected)
			}
			if dw := stringWidth(got); dw > tt.maxW {
				t.Errorf("result %q (display width %d) exceeds max %d cols", got, dw, tt.maxW)
			}
		})
	}
}

// runeLen 已移除：断言改用包级 stringWidth（按 CJK 显示列宽计）。

// ---- F1d 元数据辅助函数测试 ----

func TestHumanSize(t *testing.T) {
	tests := []struct {
		name     string
		n        int64
		expected string
	}{
		// §3.2 边界值
		{"零字节", 0, "0B"},
		{"小于1KB", 1, "1B"},
		{"1023B上限", 1023, "1023B"},
		{"刚好1KB", 1024, "1.0K"},
		{"略超1KB", 1025, "1.0K"},

		// mantissa < 100 → 1 位小数
		{"99.9K量级", 102297, "99.9K"},   // 99.9×1024
		{"99.96K量级", 102358, "100K"},  // 99.96×1024 → 100.0K → 进位 100K（BugA 修复验证）
		{"99.99K量级", 102389, "100K"},  // 99.99×1024 → 100.0K → 进位 100K
		{"12.3K量级", 12595, "12.3K"},   // 12.3×1024
		{"3.4M量级", 3565158, "3.4M"},   // 3.4×1024²
		{"1.5M量级", 1572864, "1.5M"},   // 1.5×1024²

		// mantissa ≥ 100 → 整数（舍入后进位）
		{"1023K未进位", 1047552, "1023K"},    // 1023×1024
		{"1023.6K进位到1.0M", 1048166, "1.0M"}, // 1023.6×1024
		{"100K", 102400, "100K"},
		{"999K", 1022976, "999K"},
		{"999.9K不超1024不进位", 1023935, "1000K"}, // 999.9K mantissa → 1000K，1000<1024 不进位
		{"100M", 104857600, "100M"},
		{"1G", 1073741824, "1.0G"}, // 1×1024³
		{"10G", 10737418240, "10.0G"},
		{"123G", 132070244352, "123G"},
		{"123T（≥100规则）", 135619697049600, "123T"},   // mantissa 123.4 → 整数 123T（<100 才显示小数）

		// int64 上限
		{"int64最大", 9223372036854775807, "8192P"}, // 8192×1024^5
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := humanSize(tt.n)
			if got != tt.expected {
				t.Errorf("humanSize(%d) = %q, want %q", tt.n, got, tt.expected)
			}
			// §3.2 硬约束：输出恒 ≤5 列
			if dw := stringWidth(got); dw > 5 {
				t.Errorf("humanSize(%d)=%q display width %d exceeds 5 cols", tt.n, got, dw)
			}
		})
	}
}

func TestHumanSizeWidthBound(t *testing.T) {
	// 全区间扫描：从 0 步进到约 1TB（1024^4），覆盖 [99.96K, 100K) 窄窗口
	step := int64(1234)
	maxVal := int64(1024 * 1024 * 1024 * 1024) // 1TB
	var failures []string
	for n := int64(0); n <= maxVal; n += step {
		got := humanSize(n)
		if dw := stringWidth(got); dw > 5 {
			failures = append(failures, fmt.Sprintf("humanSize(%d)=%q (width %d)", n, got, dw))
		}
	}
	// 补扫最后零头
	for n := maxVal + 1; n <= maxVal+step; n++ {
		got := humanSize(n)
		if dw := stringWidth(got); dw > 5 {
			failures = append(failures, fmt.Sprintf("humanSize(%d)=%q (width %d)", n, got, dw))
		}
	}
	if len(failures) > 0 {
		for _, f := range failures {
			t.Errorf("Width bound violation: %s", f)
		}
	}
}

func TestFormatMtime(t *testing.T) {
	tests := []struct {
		name     string
		t        time.Time
		sameYear bool
	}{
		{"同年内", time.Date(2026, 7, 8, 14, 30, 0, 0, time.UTC), true},
		{"跨年", time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), false},
		{"十年前", time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMtime(tt.t)
			wantLen := 10
			if tt.sameYear {
				wantLen = 11 // 同年格式：MM-DD HH:MM
			}
			if len(got) != wantLen {
				t.Errorf("formatMtime result %q len=%d, want %d (sameYear=%v)",
					got, len(got), wantLen, tt.sameYear)
			}
			// 同年格式内容合理性（MM-DD HH:MM）
			if tt.sameYear {
				if got[2] != '-' || got[5] != ' ' || got[8] != ':' {
					t.Errorf("formatMtime 同年 format wrong: %q", got)
				}
			} else {
				if got[4] != '-' || got[7] != '-' {
					t.Errorf("formatMtime 跨年 format wrong: %q", got)
				}
			}
		})
	}
}
