package config

// MicroNeo: MD 渲染配置（buffer 本地，可被 ft: / glob: 覆盖）
func init() {
	defaultCommonSettings["mdrender"] = true
	defaultCommonSettings["mdtablealign"] = true
	defaultCommonSettings["mdtableborder"] = true
	defaultCommonSettings["mdbolditalic"] = true
	defaultCommonSettings["mdcodeblock"] = true
	defaultCommonSettings["mdheading"] = true
	defaultCommonSettings["mdlist"] = true
	defaultCommonSettings["mdlink"] = true

	// MicroNeo: MD 全局配置
	DefaultGlobalOnlySettings["mdrenderidle"] = float64(10) // 编辑模式超时秒数
}
