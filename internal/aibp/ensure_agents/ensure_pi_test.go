package ensure_agents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

// ---------------------------------------------------------------------------
// TestParseProtocol
// ---------------------------------------------------------------------------

func TestParseProtocol(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMaj int
		wantMin int
		wantOk  bool
	}{
		{"aibp-2", "aibp-2", 2, 0, true},
		{"aibp-2.0", "aibp-2.0", 2, 0, true},
		{"aibp-2.1", "aibp-2.1", 2, 1, true},
		{"aibp-1.10", "aibp-1.10", 1, 10, true},
		{"aibp-10.1", "aibp-10.1", 10, 1, true},
		// malformed
		{"empty prefix", "", 0, 0, false},
		{"no dash", "invalid", 0, 0, false},
		{"aibp- only", "aibp-", 0, 0, false},
		{"aibp-2. (no minor)", "aibp-2.", 0, 0, false},
		{"aibp-.0 (no major)", "aibp-.0", 0, 0, false},
		{"aibp-XYZ (non-numeric)", "aibp-XYZ", 0, 0, false},
		{"aibp-2.ABC (non-numeric minor)", "aibp-2.ABC", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maj, min, ok := aibp.ParseProtocol(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ParseProtocol(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
			if ok && (maj != tt.wantMaj || min != tt.wantMin) {
				t.Errorf("ParseProtocol(%q) = (%d,%d,_), want (%d,%d,_)", tt.input, maj, min, tt.wantMaj, tt.wantMin)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPiAIBPVersion
// ---------------------------------------------------------------------------

func TestPiAIBPVersion(t *testing.T) {
	t.Run("source install", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		settingsPath := filepath.Join(dir, "settings.json")
		settings := `{"packages":["../../pi-dev/microNeo/aibp-agents/pi"]}`
		if err := os.WriteFile(settingsPath, []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (PiEnsurer{}).AIBPVersion()
		if !isSource {
			t.Errorf("AIBPVersion() isSource = false, want true")
		}
		if maj != 0 || min != 0 {
			t.Errorf("AIBPVersion() = (%d,%d,_,true), want (0,0,_,true)", maj, min)
		}
	})

	t.Run("npm package", func(t *testing.T) {
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

		pkgDir := filepath.Join(dir, "npm", "node_modules", "aibp-pi")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		pkgPath := filepath.Join(pkgDir, "package.json")
		pkg := `{"aibp":{"protocol":"aibp-2.0"}}`
		if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (PiEnsurer{}).AIBPVersion()
		if isSource {
			t.Errorf("AIBPVersion() isSource = true, want false")
		}
		if maj != 2 || min != 0 {
			t.Errorf("AIBPVersion() = (%d,%d,_,false), want (2,0,_,false)", maj, min)
		}
	})

	t.Run("npm pinned version", func(t *testing.T) {
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

		pkgDir := filepath.Join(dir, "npm", "node_modules", "aibp-pi")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		pkgPath := filepath.Join(pkgDir, "package.json")
		pkg := `{"aibp":{"protocol":"aibp-1.5"}}`
		if err := os.WriteFile(pkgPath, []byte(pkg), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (PiEnsurer{}).AIBPVersion()
		if maj != 1 || min != 5 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (1,5,false)", maj, min, isSource)
		}
	})

	t.Run("npm package missing", func(t *testing.T) {
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

		maj, min, isSource, _ := (PiEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})

	t.Run("npm package corrupt json", func(t *testing.T) {
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

		pkgDir := filepath.Join(dir, "npm", "node_modules", "aibp-pi")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte("not-json"), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (PiEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})

	t.Run("npm package missing aibp.protocol field", func(t *testing.T) {
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

		pkgDir := filepath.Join(dir, "npm", "node_modules", "aibp-pi")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		pkgPath := filepath.Join(pkgDir, "package.json")
		if err := os.WriteFile(pkgPath, []byte(`{"name":"aibp-pi","version":"1.0.0"}`), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (PiEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})

	t.Run("empty packages", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		settingsPath := filepath.Join(dir, "settings.json")
		settings := `{"packages":[]}`
		if err := os.WriteFile(settingsPath, []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource, _ := (PiEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})

	t.Run("settings.json missing", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "pi-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		t.Setenv("PI_CODING_AGENT_DIR", dir)

		maj, min, isSource, _ := (PiEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
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

		maj, min, isSource, _ := (PiEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})
}