package ensure_agents

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

const aibpOpencodeSpec = "aibp-opencode" // 不 pin 版本（D5）

// OpencodeEnsurer 是 opencode 的 aibp 扩展自举实现。
type OpencodeEnsurer struct{}

var _ AgentEnsurer = OpencodeEnsurer{} // 编译期接口断言

func (OpencodeEnsurer) AgentName() string { return "opencode" }

func (OpencodeEnsurer) HasAgent() bool {
	_, err := exec.LookPath("opencode")
	return err == nil
}

// opencodeConfigDir 返回 opencode 的全局配置目录。
//
// 派生路径（与 opencode 1.17.9 的 XDG 行为一致）：
//   tui.json:           <configDir>/tui.json        （TUI 插件登记表；D19 §源码结构）
//   opencode.json:      <configDir>/opencode.json   （server 插件 + MCP；本计划不读）
//   package.json:       <configDir>/../cache/opencode/packages/aibp-opencode@latest/node_modules/aibp-opencode/package.json
//
// 注意：opencode 无 PI_CODING_AGENT_DIR 类环境变量覆盖机制——D8 硬编码。
func opencodeConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opencode")
}

func opencodeTuiPath() string {
	return filepath.Join(opencodeConfigDir(), "tui.json")
}

// opencodeCacheDir 返回 opencode 的包缓存目录。
//
// opencode 使用 xdg-basedir npm 包确定缓存路径：
//   - XDG_CACHE_HOME 设了 → $XDG_CACHE_HOME/opencode
//   - macOS fallback → ~/.cache/opencode
//   - Linux fallback → ~/.cache/opencode
//   - Windows fallback → %LOCALAPPDATA%/opencode
//
// Go os.UserCacheDir() 在 macOS 返回 ~/Library/Caches，与 xdg-basedir 不一致。
// 所以不能用 os.UserCacheDir()，必须自己实现相同的 fallback 逻辑。
func opencodeCacheDir() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "opencode")
}

// opencodeReadTui 读取 opencode tui.json 的 plugin[]。
func opencodeReadTui() []string {
	b, err := os.ReadFile(opencodeTuiPath())
	if err != nil {
		return nil
	}
	var s struct{ Plugin []string `json:"plugin"` }
	if err := json.Unmarshal(b, &s); err != nil {
		return nil
	}
	return s.Plugin
}

// opencodeRemoveTuiEntries 从 tui.json 删除 npm 形态的 aibp-opencode 条目
// （"aibp-opencode" 与 "aibp-opencode@<version>"）。
// 不动源码路径形态条目（源码↔npm 迁移由用户手动完成，见 README）。
// tui.json 不存在/损坏 → 静默返回 nil，不阻塞 install。
//
// 关键陷阱：tui.json 含其它键（theme/keybinds/…），不能只写回 Plugin 字段，
// 否则会丢用户配置。这里用 map[string]json.RawMessage 读全文件，只重写 "plugin" 键，
// 其它键字节级保留。
func opencodeRemoveTuiEntries() error {
	p := opencodeTuiPath()
	b, err := os.ReadFile(p)
	if err != nil {
		return nil // 不存在：无条目可删
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil // 损坏：跳过，避免破坏
	}
	pluginRaw, ok := doc["plugin"]
	if !ok {
		return nil
	}
	var plugins []string
	if err := json.Unmarshal(pluginRaw, &plugins); err != nil {
		return nil
	}
	kept := make([]string, 0, len(plugins))
	removed := false
	for _, e := range plugins {
		if e == aibpOpencodeSpec || strings.HasPrefix(e, aibpOpencodeSpec+"@") || strings.Contains(e, "aibp-agents/") {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	if !removed {
		return nil
	}
	out, _ := json.Marshal(kept)
	doc["plugin"] = out
	final, _ := json.MarshalIndent(doc, "", "  ")
	// 0644 会覆盖用户可能设的 0600 权限——tui.json 通常无 secrets，0644 是合理默认。
	return os.WriteFile(p, final, 0o644)
}

// AIBPVersion：识别本 agent 已装的 aibp。
//   - 包含 "aibp-agents" → 源码路径，返回 (0, 0, true, "")（不读盘；信任假设）
//   - 包含 "aibp-opencode" → npm 包，读取缓存的 package.json 的协议 + 版本
//   - 未找到 / 损坏 → (0, 0, false, "")
func (OpencodeEnsurer) AIBPVersion() (int, int, bool, string) {
	for _, entry := range opencodeReadTui() {
		if strings.Contains(entry, "aibp-agents") {
			return 0, 0, true, "" // 源码路径：不读盘
		}
		if strings.Contains(entry, "aibp-opencode") {
			return opencodeNpmAIBPVersion() // npm 包：读版本号
		}
	}
	return 0, 0, false, "" // 没找到 aibp 条目
}

// opencodeNpmAIBPVersion 读取 aibp-opencode 的 cache 安装版本的协议。
// 协议来源：package.json 的 aibp.protocol 交 aibp.ParseProtocol 解析。
func opencodeNpmAIBPVersion() (int, int, bool, string) {
	pkgPath := filepath.Join(
		opencodeNpmCacheSubdir(),
		"node_modules", "aibp-opencode", "package.json",
	)
	b, err := os.ReadFile(pkgPath)
	if err != nil {
		return 0, 0, false, ""
	}
	var pkg struct {
		AIBP    struct{ Protocol string `json:"protocol"` } `json:"aibp"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return 0, 0, false, ""
	}
	if pkg.AIBP.Protocol == "" {
		return 0, 0, false, ""
	}
	maj, min, ok := aibp.ParseProtocol(pkg.AIBP.Protocol)
	if !ok {
		return 0, 0, false, ""
	}
	return maj, min, false, pkg.Version // npm 安装：isSource 恒为 false；version 来自 package.json
}

// opencodeNpmCacheSubdir 按 tui.json 条目推导 cache 子目录名：
//   "aibp-opencode"        → "aibp-opencode@latest"
//   "aibp-opencode@1.0.2"  → "aibp-opencode@1.0.2"
// 找不到 aibp 条目 → 回退 @latest（兼容）。
//
// 注意 Go 的 break 作用于最内层 for/switch：若 break 写在 switch 体内，只跳出 switch。
// 这里要的是「跳过非 aibp 条目、命中第一个 aibp 条目即停」，故用 matched 标志
// 把 break 放到 for 体内、switch 之外（修 D3）。
func opencodeNpmCacheSubdir() string {
	suffix := "latest"
	for _, e := range opencodeReadTui() {
		matched := true
		switch {
		case strings.HasPrefix(e, aibpOpencodeSpec+"@"):
			suffix = strings.TrimPrefix(e, aibpOpencodeSpec+"@")
		case e == aibpOpencodeSpec:
			// suffix 默认即 "latest"
		default:
			matched = false // 非 aibp 条目（其它插件名 / 源码路径），继续看下一个
		}
		if matched {
			break // 命中第一个 aibp 条目即停（for 体内 break 跳出整个 for）
		}
	}
	return filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@"+suffix)
}

// uninstallAIBP 清理 aibp-opencode，保证「安装/卸载前是干净状态」：
//   1. 规范化 tui.json（删 aibp 条目）—— 修 D2
//   2. 自检（防 opencode 运行中持有锁，WriteFile 静默失败 → 旧条目仍在 → 后续 Already configured → 假成功）
//   3. glob 清所有版本 cache（@* 覆盖 @latest 与 pinned @1.0.x）—— 修 D1
// 被 UninstallAIBP 和 installOrUpdate 共用。
func uninstallAIBP() error {
	// 1. 规范化 tui.json（opencodeRemoveTuiEntries 对文件不存在/损坏返回 nil，不阻塞）
	if err := opencodeRemoveTuiEntries(); err != nil {
		return fmt.Errorf("规范化 tui.json 失败: %w", err)
	}
	// 2. 自检：tui.json 里不应还有 aibp-opencode 条目
	for _, e := range opencodeReadTui() {
		if strings.HasPrefix(e, aibpOpencodeSpec) {
			return fmt.Errorf("tui.json 规范化失败：条目仍存在 %q（请先完全退出 opencode TUI 再试）", e)
		}
	}
	// 3. glob @* 清所有版本 cache
	pkgGlob := filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@*")
	if matches, _ := filepath.Glob(pkgGlob); matches != nil {
		for _, m := range matches {
			_ = os.RemoveAll(m)
		}
	}
	return nil
}

// installOrUpdate = uninstall + 重装（D1/D2/自检由 uninstallAIBP 集中解决）。
// opencode 无原生 update 命令，清干净 + 重装是唯一强制刷新路径（D24 调研结论）。
func (e OpencodeEnsurer) installOrUpdate() error {
	if err := uninstallAIBP(); err != nil {
		return err
	}
	cmd := exec.Command("opencode", "plugin", aibpOpencodeSpec, "-g")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("opencode plugin 失败（可能 opencode 过旧，请升级 opencode）: %w", err)
	}
	return nil
}

func (OpencodeEnsurer) InstallAIBP() error {
	return OpencodeEnsurer{}.installOrUpdate()
}

func (OpencodeEnsurer) UpdateAIBP() error {
	return OpencodeEnsurer{}.installOrUpdate()
}

func (OpencodeEnsurer) UpdateToLatest(report Reporter) error {
	prefix := "aibp-opencode"
	_, _, isSource, installed := (OpencodeEnsurer{}).AIBPVersion() // 一次拿到 isSource + version

	// npm 已装 → 查 registry，新了才装；已是最新则跳过
	if !isSource && installed != "" {
		latest, err := latestNpmVersion(aibpOpencodeSpec)
		if err != nil {
			report(prefix + " couldn't check npm registry (" + err.Error() + "), skipping — retry later")
			return err
		}
		if !semverLess(installed, latest) {
			report(prefix + " already at latest (" + installed + ")")
			return nil
		}
		report(prefix + " updating " + installed + " → " + latest)
		return (OpencodeEnsurer{}).installOrUpdate() // 复用
	}

	// source / 未装 → 复用 installOrUpdate 拉最新 npm
	if isSource {
		report(prefix + " source install → converting to npm latest")
	} else {
		report(prefix + " not installed, installing latest")
	}
	return (OpencodeEnsurer{}).installOrUpdate() // 复用
}

// UninstallAIBP 清理 aibp-opencode（规范化 tui.json + 删所有版本 cache）。
// 暂未被 Ensure 编排调用（AgentEnsurer 接口未含卸载），为未来 uninstall 功能预留；
// 逻辑与 installOrUpdate 共用 uninstallAIBP。
func (OpencodeEnsurer) UninstallAIBP() error {
	return uninstallAIBP()
}