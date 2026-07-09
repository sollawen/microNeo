package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/aibp/ensure_agents"
	"github.com/micro-editor/micro/v2/internal/config"
	rt "github.com/micro-editor/micro/v2/runtime"
)

// ResetSettings copies the embedded runtime/settings.json to the user's
// config directory as settings.json. If an existing settings.json is found,
// it is renamed to settings.json.backup first (overwriting any prior backup).
func ResetSettings() {
	data, _ := rt.Asset("runtime/settings.json")
	dst := filepath.Join(config.ConfigDir, "settings.json")

	if _, err := os.Stat(dst); err == nil {
		os.Remove(dst + ".backup")
		os.Rename(dst, dst+".backup")
		fmt.Println("Backed up", dst, "to", dst+".backup")
	}
	os.WriteFile(dst, data, 0644)
	fmt.Println("Wrote", dst)
}

// DoCheckAgent executes -check-agent: runs aibp extension self-heal for all
// installed agents, printing progress to stdout. Exits when done, never enters TUI.
// Placed before config/screen init — ensure_agents has zero dependency on either,
// so it works even if the config is corrupted.
func DoCheckAgent() {
	if !*flagCheckAgent {
		return
	}
	hadErr := ensure_agents.EnsureAll(func(msg string) { fmt.Println(msg) })
	if hadErr {
		exit(1)
	}
	exit(0)
}

// DoUpdateAIBP 执行 -update-aibp：把无自更新能力的 agent（opencode/claude）
// 的 aibp 扩展升到最新发布版，进度打到 stdout。跑完 exit，不进 TUI。
func DoUpdateAIBP() {
	if !*flagUpdateAIBP {
		return
	}
	hadErr := ensure_agents.UpdateAll(func(msg string) { fmt.Println(msg) })
	if hadErr {
		exit(1)
	}
	exit(0)
}
