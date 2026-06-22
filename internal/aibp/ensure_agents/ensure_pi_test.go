package ensure_agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasAIBP(t *testing.T) {
	t.Run("unpinned spec matches", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		settingsPath := filepath.Join(dir, "settings.json")
		settings := `{"packages":["npm:aibp-pi"]}`
		if err := os.WriteFile(settingsPath, []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}

		if !(PiEnsurer{}).HasAIBP() {
			t.Error("HasAIBP() = false, want true")
		}
	})

	t.Run("pinned spec matches", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		settingsPath := filepath.Join(dir, "settings.json")
		settings := `{"packages":["npm:aibp-pi@1.0.0"]}`
		if err := os.WriteFile(settingsPath, []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}

		if !(PiEnsurer{}).HasAIBP() {
			t.Error("HasAIBP() = false, want true")
		}
	})

	t.Run("no aibp-pi entry", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		settingsPath := filepath.Join(dir, "settings.json")
		settings := `{"packages":["npm:other-pkg"]}`
		if err := os.WriteFile(settingsPath, []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}

		if (PiEnsurer{}).HasAIBP() {
			t.Error("HasAIBP() = true, want false")
		}
	})

	t.Run("settings.json missing", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		if (PiEnsurer{}).HasAIBP() {
			t.Error("HasAIBP() = true, want false (settings.json missing)")
		}
	})

	t.Run("settings.json corrupt", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		settingsPath := filepath.Join(dir, "settings.json")
		if err := os.WriteFile(settingsPath, []byte("not-json"), 0644); err != nil {
			t.Fatal(err)
		}

		if (PiEnsurer{}).HasAIBP() {
			t.Error("HasAIBP() = true, want false (settings.json corrupt)")
		}
	})
}

func TestAIBPVersion(t *testing.T) {
	t.Run("normal package.json", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		pkgDir := filepath.Join(dir, "npm", "node_modules", "aibp-pi")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		pkgPath := filepath.Join(pkgDir, "package.json")
		pkg := `{"aibp":{"protocol":"aibp-1"}}`
		if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
			t.Fatal(err)
		}

		ver, err := (PiEnsurer{}).AIBPVersion()
		if err != nil {
			t.Fatalf("AIBPVersion() error = %v, want nil", err)
		}
		if ver != "aibp-1" {
			t.Errorf("AIBPVersion() = %q, want %q", ver, "aibp-1")
		}
	})

	t.Run("package.json missing", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		_, err = (PiEnsurer{}).AIBPVersion()
		if err == nil {
			t.Error("AIBPVersion() = nil, want error (package.json missing)")
		}
	})

	t.Run("package.json corrupt", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		pkgDir := filepath.Join(dir, "npm", "node_modules", "aibp-pi")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		pkgPath := filepath.Join(pkgDir, "package.json")
		if err := os.WriteFile(pkgPath, []byte("not-json"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err = (PiEnsurer{}).AIBPVersion()
		if err == nil {
			t.Error("AIBPVersion() = nil, want error (package.json corrupt)")
		}
	})

	t.Run("package.json missing aibp.protocol field", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		pkgDir := filepath.Join(dir, "npm", "node_modules", "aibp-pi")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		pkgPath := filepath.Join(pkgDir, "package.json")
		pkg := `{"name":"aibp-pi","version":"1.0.0"}`
		if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
			t.Fatal(err)
		}

		ver, err := (PiEnsurer{}).AIBPVersion()
		if err == nil {
			t.Errorf("AIBPVersion() error = nil, want error (aibp.protocol missing should trigger self-heal)")
		}
		if ver != "" {
			t.Errorf("AIBPVersion() = %q, want %q", ver, "")
		}
	})
}
