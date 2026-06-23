package main

import (
	"fmt"
	"os"
	"path/filepath"

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
