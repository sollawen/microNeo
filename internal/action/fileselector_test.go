package action

import "testing"

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
