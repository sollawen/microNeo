package ensure_agents

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// TestOpencodeAIBPVersion
// ---------------------------------------------------------------------------

func TestOpencodeAIBPVersion(t *testing.T) {
	t.Run("source install (absolute path)", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CONFIG_HOME", dir)

		tuiPath := filepath.Join(dir, "opencode", "tui.json")
		if err := os.MkdirAll(filepath.Dir(tuiPath), 0755); err != nil {
			t.Fatal(err)
		}
		// absolute path containing "aibp-agents" → source
		if err := os.WriteFile(tuiPath, []byte(`{"plugin":["/abs/path/to/aibp-agents/opencode"]}`), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (OpencodeEnsurer{}).AIBPVersion()
		if !isSource {
			t.Errorf("AIBPVersion() isSource = false, want true")
		}
		if maj != 0 || min != 0 {
			t.Errorf("AIBPVersion() = (%d,%d,_,true), want (0,0,_,true)", maj, min)
		}
	})

	t.Run("npm package", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-cache")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("XDG_CACHE_HOME", dir)

		tuiPath := filepath.Join(dir, "opencode", "tui.json")
		if err := os.MkdirAll(filepath.Dir(tuiPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tuiPath, []byte(`{"plugin":["aibp-opencode"]}`), 0644); err != nil {
			t.Fatal(err)
		}

		pkgDir := filepath.Join(dir, "opencode", "packages", "aibp-opencode@latest", "node_modules", "aibp-opencode")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		pkgPath := filepath.Join(pkgDir, "package.json")
		if err := os.WriteFile(pkgPath, []byte(`{"aibp":{"protocol":"aibp-2.0"}}`), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (OpencodeEnsurer{}).AIBPVersion()
		if isSource {
			t.Errorf("AIBPVersion() isSource = true, want false")
		}
		if maj != 2 || min != 0 {
			t.Errorf("AIBPVersion() = (%d,%d,_,false), want (2,0,_,false)", maj, min)
		}
	})

	t.Run("npm pinned version", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-cache")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("XDG_CACHE_HOME", dir)

		tuiPath := filepath.Join(dir, "opencode", "tui.json")
		if err := os.MkdirAll(filepath.Dir(tuiPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tuiPath, []byte(`{"plugin":["aibp-opencode@1.0.1"]}`), 0644); err != nil {
			t.Fatal(err)
		}

		pkgDir := filepath.Join(dir, "opencode", "packages", "aibp-opencode@1.0.1", "node_modules", "aibp-opencode")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		pkgPath := filepath.Join(pkgDir, "package.json")
		if err := os.WriteFile(pkgPath, []byte(`{"aibp":{"protocol":"aibp-1.5"}}`), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (OpencodeEnsurer{}).AIBPVersion()
		if maj != 1 || min != 5 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (1,5,false)", maj, min, isSource)
		}
	})

	t.Run("npm package missing", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-cache")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("XDG_CACHE_HOME", dir)

		tuiPath := filepath.Join(dir, "opencode", "tui.json")
		if err := os.MkdirAll(filepath.Dir(tuiPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tuiPath, []byte(`{"plugin":["aibp-opencode"]}`), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (OpencodeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})

	t.Run("npm package corrupt json", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-cache")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("XDG_CACHE_HOME", dir)

		tuiPath := filepath.Join(dir, "opencode", "tui.json")
		if err := os.MkdirAll(filepath.Dir(tuiPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tuiPath, []byte(`{"plugin":["aibp-opencode"]}`), 0644); err != nil {
			t.Fatal(err)
		}

		pkgDir := filepath.Join(dir, "opencode", "packages", "aibp-opencode@latest", "node_modules", "aibp-opencode")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte("not-json"), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (OpencodeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})

	t.Run("npm package missing aibp.protocol field", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-cache")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("XDG_CACHE_HOME", dir)

		tuiPath := filepath.Join(dir, "opencode", "tui.json")
		if err := os.MkdirAll(filepath.Dir(tuiPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tuiPath, []byte(`{"plugin":["aibp-opencode"]}`), 0644); err != nil {
			t.Fatal(err)
		}

		pkgDir := filepath.Join(dir, "opencode", "packages", "aibp-opencode@latest", "node_modules", "aibp-opencode")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		pkgPath := filepath.Join(pkgDir, "package.json")
		if err := os.WriteFile(pkgPath, []byte(`{"name":"aibp-opencode","version":"1.0.0"}`), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (OpencodeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})

	t.Run("empty plugin list", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CONFIG_HOME", dir)

		tuiPath := filepath.Join(dir, "opencode", "tui.json")
		if err := os.MkdirAll(filepath.Dir(tuiPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tuiPath, []byte(`{"plugin":[]}`), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (OpencodeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})

	t.Run("tui.json missing", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CONFIG_HOME", dir)

		maj, min, isSource, _ := (OpencodeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})

	t.Run("tui.json corrupt", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CONFIG_HOME", dir)

		tuiPath := filepath.Join(dir, "opencode", "tui.json")
		if err := os.MkdirAll(filepath.Dir(tuiPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tuiPath, []byte("not-json"), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (OpencodeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})
}
