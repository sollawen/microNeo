package ensure_agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpencodeHasAIBP(t *testing.T) {
	t.Run("unpinned spec matches", func(t *testing.T) {
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
		if err := os.WriteFile(tuiPath, []byte(`{"plugin":["aibp-opencode"]}`), 0644); err != nil {
			t.Fatal(err)
		}

		if !(OpencodeEnsurer{}).HasAIBP() {
			t.Error("HasAIBP() = false, want true")
		}
	})

	t.Run("pinned spec matches", func(t *testing.T) {
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
		if err := os.WriteFile(tuiPath, []byte(`{"plugin":["aibp-opencode@1.0.1"]}`), 0644); err != nil {
			t.Fatal(err)
		}

		if !(OpencodeEnsurer{}).HasAIBP() {
			t.Error("HasAIBP() = false, want true")
		}
	})

	t.Run("no aibp entry", func(t *testing.T) {
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
		if err := os.WriteFile(tuiPath, []byte(`{"plugin":["some-other-plugin"]}`), 0644); err != nil {
			t.Fatal(err)
		}

		if (OpencodeEnsurer{}).HasAIBP() {
			t.Error("HasAIBP() = true, want false")
		}
	})

	t.Run("tui.json missing", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CONFIG_HOME", dir)

		if (OpencodeEnsurer{}).HasAIBP() {
			t.Error("HasAIBP() = true, want false (tui.json missing)")
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

		if (OpencodeEnsurer{}).HasAIBP() {
			t.Error("HasAIBP() = true, want false (tui.json corrupt)")
		}
	})
}

func TestOpencodeAIBPVersion(t *testing.T) {
	// opencodeCacheDir() 用 XDG_CACHE_HOME 覆盖，不用 os.UserCacheDir()。
	// 所以所有测试都设 XDG_CACHE_HOME 到临时目录，隔离真实缓存。

	t.Run("normal package.json", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-cache")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CACHE_HOME", dir)

		pkgDir := filepath.Join(dir, "opencode", "packages", "aibp-opencode@latest", "node_modules", "aibp-opencode")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}

		pkgPath := filepath.Join(pkgDir, "package.json")
		if err := os.WriteFile(pkgPath, []byte(`{"aibp":{"protocol":"aibp-1"}}`), 0644); err != nil {
			t.Fatal(err)
		}

		ver, err := (OpencodeEnsurer{}).AIBPVersion()
		if err != nil {
			t.Fatalf("AIBPVersion() error = %v, want nil", err)
		}
		if ver != "aibp-1" {
			t.Errorf("AIBPVersion() = %q, want %q", ver, "aibp-1")
		}
	})

	t.Run("package.json missing", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-cache")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CACHE_HOME", dir)

		// AIBPVersion should fail — no cache dir created
		_, err = (OpencodeEnsurer{}).AIBPVersion()
		if err == nil {
			t.Error("AIBPVersion() = nil, want error (package.json missing)")
		}
	})

	t.Run("package.json corrupt", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-cache")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CACHE_HOME", dir)

		pkgDir := filepath.Join(dir, "opencode", "packages", "aibp-opencode@latest", "node_modules", "aibp-opencode")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}

		pkgPath := filepath.Join(pkgDir, "package.json")
		if err := os.WriteFile(pkgPath, []byte("not-json"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err = (OpencodeEnsurer{}).AIBPVersion()
		if err == nil {
			t.Error("AIBPVersion() = nil, want error (package.json corrupt)")
		}
	})

	t.Run("package.json missing aibp.protocol field", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "opencode-cache")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("XDG_CACHE_HOME", dir)

		pkgDir := filepath.Join(dir, "opencode", "packages", "aibp-opencode@latest", "node_modules", "aibp-opencode")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}

		pkgPath := filepath.Join(pkgDir, "package.json")
		if err := os.WriteFile(pkgPath, []byte(`{"name":"aibp-opencode","version":"1.0.0"}`), 0644); err != nil {
			t.Fatal(err)
		}

		var ver string
		ver, err = (OpencodeEnsurer{}).AIBPVersion()
		if err == nil {
			t.Errorf("AIBPVersion() error = nil, want error (aibp.protocol missing should trigger self-heal)")
		}
		if ver != "" {
			t.Errorf("AIBPVersion() = %q, want %q", ver, "")
		}
	})
}
