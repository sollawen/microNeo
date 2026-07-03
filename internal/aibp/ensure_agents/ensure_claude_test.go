package ensure_agents

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// TestClaudeAIBPVersion
// ---------------------------------------------------------------------------

func TestClaudeAIBPVersion(t *testing.T) {
	// installPath 在 setup 内部算（dir 在其作用域内），用 %s 占位让 setup 把绝对路径
	// 替换进 installedJSON——避免调用方引用 dir（Lisa review Issue #1：闭包拿不到 dir）。
	const installedTpl = `{"version":2,"plugins":{"aibp-claude@microNeo-plugins":[{"scope":"user","installPath":"%s","version":"1.0.0"}]}}`
	const enabledSettings  = `{"enabledPlugins":{"aibp-claude@microNeo-plugins":true}}`
	const disabledSettings = `{"enabledPlugins":{"aibp-claude@microNeo-plugins":false}}`

	setup := func(t *testing.T, installedJSON, settingsJSON, pkgProtocol string) string {
		t.Helper()
		dir, _ := os.MkdirTemp("", "claude-test")
		t.Cleanup(func() { os.RemoveAll(dir) })
		t.Setenv("CLAUDE_CONFIG_DIR", dir)

		installPath := filepath.Join(dir, "cache", "microNeo-plugins", "aibp-claude", "1.0.0")

		if installedJSON != "" { // 含 %s → Sprintf 替换成绝对 installPath
			ip := filepath.Join(dir, "plugins", "installed_plugins.json")
			os.MkdirAll(filepath.Dir(ip), 0755)
			os.WriteFile(ip, []byte(fmt.Sprintf(installedJSON, installPath)), 0644)
		}
		if settingsJSON != "" {
			os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settingsJSON), 0644)
		}
		if pkgProtocol != "__omit__" {
			os.MkdirAll(installPath, 0755)
			pkg := `{"name":"aibp-claude","version":"1.0.0","aibp":{"protocol":"` + pkgProtocol + `"}}`
			os.WriteFile(filepath.Join(installPath, "package.json"), []byte(pkg), 0644)
		}
		return dir
	}

	t.Run("marketplace installed + enabled", func(t *testing.T) {
		setup(t, installedTpl, enabledSettings, "aibp-2.0")
		maj, min, isSrc := (ClaudeEnsurer{}).AIBPVersion()
		if maj != 2 || min != 0 || isSrc {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (2,0,false)", maj, min, isSrc)
		}
	})

	t.Run("not installed", func(t *testing.T) {
		setup(t, "", "", "__omit__")
		maj, min, isSrc := (ClaudeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSrc {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSrc)
		}
	})

	t.Run("entry missing", func(t *testing.T) {
		setup(t, `{}`, "", "__omit__")
		maj, min, isSrc := (ClaudeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSrc {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSrc)
		}
	})

	t.Run("installed but disabled", func(t *testing.T) {
		setup(t, installedTpl, disabledSettings, "aibp-2.0")
		maj, min, isSrc := (ClaudeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSrc {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSrc)
		}
	})

	t.Run("corrupt installed json", func(t *testing.T) {
		dir, _ := os.MkdirTemp("", "claude-test")
		t.Cleanup(func() { os.RemoveAll(dir) })
		t.Setenv("CLAUDE_CONFIG_DIR", dir)
		ip := filepath.Join(dir, "plugins", "installed_plugins.json")
		os.MkdirAll(filepath.Dir(ip), 0755)
		os.WriteFile(ip, []byte("not-json"), 0644)

		maj, min, isSrc := (ClaudeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSrc {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSrc)
		}
	})

	t.Run("cache package missing", func(t *testing.T) {
		setup(t, installedTpl, enabledSettings, "__omit__")
		maj, min, isSrc := (ClaudeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSrc {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSrc)
		}
	})

	t.Run("missing protocol field", func(t *testing.T) {
		dir, _ := os.MkdirTemp("", "claude-test")
		t.Cleanup(func() { os.RemoveAll(dir) })
		t.Setenv("CLAUDE_CONFIG_DIR", dir)

		installPath := filepath.Join(dir, "cache", "microNeo-plugins", "aibp-claude", "1.0.0")
		os.MkdirAll(installPath, 0755)

		ip := filepath.Join(dir, "plugins", "installed_plugins.json")
		os.MkdirAll(filepath.Dir(ip), 0755)
		os.WriteFile(ip, []byte(fmt.Sprintf(installedTpl, installPath)), 0644)

		os.WriteFile(filepath.Join(dir, "settings.json"), []byte(enabledSettings), 0644)

		// package.json 没有 aibp 键
		pkg := `{"name":"aibp-claude","version":"1.0.0"}`
		os.WriteFile(filepath.Join(installPath, "package.json"), []byte(pkg), 0644)

		maj, min, isSrc := (ClaudeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSrc {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSrc)
		}
	})
}
