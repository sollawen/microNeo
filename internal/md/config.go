package md

import (
	"path/filepath"
	"strings"

	"github.com/micro-editor/tcell/v2"
)

// MDConfig 持有所有 MD 渲染相关的配置。
// 由 BufPane 构造时从 config/buffer 层读取，塞到 BufWindow 上。
// display 和 md 包通过这个结构解耦，不直接 import config.

// MDColorscheme 持有渲染器需要的颜色方案。
// 由 BufPane 构造时从 config.Colorscheme 填入，md 包只读不写。
type MDColorscheme struct {
	DefStyle tcell.Style            // config.DefStyle 快照
	Styles   map[string]tcell.Style // "default", "comment", "statement" 等
}

type MDConfig struct {
	Colorscheme MDColorscheme // 颜色方案（BufPane 构造时填入）

	MDRender      bool    // 功能总开关
	MDRenderIdle  float64 // 编辑模式超时秒数（Step 1 用）
	MDTableAlign  bool    // 表格对齐
	MDTableBorder bool    // 表格外框
	MDBoldItalic  bool    // 加粗斜体渲染
	MDCodeBlock   bool    // 代码块渲染
	MDHeading     bool    // 标题渲染
	MDList        bool    // 列表渲染
	MDLink        bool    // 链接渲染
}

// DefaultMDConfig 返回默认配置。
func DefaultMDConfig() MDConfig {
	return MDConfig{
		Colorscheme:  MDColorscheme{}, // 由 BufPane 构造时填入
		MDRender:      true,
		MDRenderIdle:  10,
		MDTableAlign:  true,
		MDTableBorder: false,
		MDBoldItalic:  true,
		MDCodeBlock:   true,
		MDHeading:     true,
		MDList:        true,
		MDLink:        true,
	}
}

// IsMarkdownFile 判断文件路径是否为 Markdown 文件。
// BufPane 创建 BufWindow 时调用一次。
func IsMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}
