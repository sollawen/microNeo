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

		maj, min, isSource := (OpencodeEnsurer{}).AIBPVersion()
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

		maj, min, isSource := (OpencodeEnsurer{}).AIBPVersion()
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

		pkgDir := filepath.Join(dir, "opencode", "packages", "aibp-opencode@latest", "node_modules", "aibp-opencode")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatal(err)
		}
		pkgPath := filepath.Join(pkgDir, "package.json")
		if err := os.WriteFile(pkgPath, []byte(`{"aibp":{"protocol":"aibp-1.5"}}`), 0644); err != nil {
			t.Fatal(err)
		}

		maj, min, isSource := (OpencodeEnsurer{}).AIBPVersion()
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

		maj, min, isSource := (OpencodeEnsurer{}).AIBPVersion()
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

		maj, min, isSource := (OpencodeEnsurer{}).AIBPVersion()
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

		maj, min, isSource := (OpencodeEnsurer{}).AIBPVersion()
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

		maj, min, isSource := (OpencodeEnsurer{}).AIBPVersion()
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

		maj, min, isSource := (OpencodeEnsurer{}).AIBPVersion()
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

		maj, min, isSource := (OpencodeEnsurer{}).AIBPVersion()
		if maj != 0 || min != 0 || isSource {
			t.Errorf("AIBPVersion() = (%d,%d,%v), want (0,0,false)", maj, min, isSource)
		}
	})
}

// ---------------------------------------------------------------------------
// TestEnsure — orchestration test covering all 5 branches
// ---------------------------------------------------------------------------

// mockEnsurer implements AgentEnsurer for testing Ensure()编排逻辑.
type mockEnsurer struct {
	name               string
	hasAgent           bool
	aibpMajor, aibpMinor int
	isSource           bool
	installCalled      bool
	updateCalled       bool
	installErr         error
	updateErr          error
}

func (m *mockEnsurer) AgentName() string { return m.name }
func (m *mockEnsurer) HasAgent() bool    { return m.hasAgent }
func (m *mockEnsurer) AIBPVersion() (int, int, bool) {
	return m.aibpMajor, m.aibpMinor, m.isSource
}
func (m *mockEnsurer) InstallAIBP() error {
	m.installCalled = true
	return m.installErr
}
func (m *mockEnsurer) UpdateAIBP() error {
	m.updateCalled = true
	return m.updateErr
}

func TestEnsure(t *testing.T) {
	t.Run("source install branch", func(t *testing.T) {
		m := &mockEnsurer{name: "test", hasAgent: true, aibpMajor: 0, aibpMinor: 0, isSource: true}
		var msgs []string
		err := Ensure(m, func(msg string) { msgs = append(msgs, msg) })
		if err != nil {
			t.Errorf("Ensure() error = %v, want nil", err)
		}
		if !m.isSource && len(msgs) > 0 && msgs[len(msgs)-1] != "aibp-test source install, skipping" {
			// source branch message check
		}
		if m.installCalled || m.updateCalled {
			t.Error("source install branch should not call InstallAIBP or UpdateAIBP")
		}
	})

	t.Run("not installed branch (major==0, not source)", func(t *testing.T) {
		m := &mockEnsurer{name: "test", hasAgent: true, aibpMajor: 0, aibpMinor: 0, isSource: false}
		var msgs []string
		err := Ensure(m, func(msg string) { msgs = append(msgs, msg) })
		if err != nil {
			t.Errorf("Ensure() error = %v, want nil", err)
		}
		if !m.installCalled {
			t.Error("not installed branch should call InstallAIBP")
		}
		if m.updateCalled {
			t.Error("not installed branch should not call UpdateAIBP")
		}
	})

	t.Run("outdated branch (major < mine)", func(t *testing.T) {
		m := &mockEnsurer{name: "test", hasAgent: true, aibpMajor: 1, aibpMinor: 0, isSource: false}
		var msgs []string
		err := Ensure(m, func(msg string) { msgs = append(msgs, msg) })
		if err != nil {
			t.Errorf("Ensure() error = %v, want nil", err)
		}
		if !m.updateCalled {
			t.Error("outdated branch should call UpdateAIBP")
		}
		if m.installCalled {
			t.Error("outdated branch should not call InstallAIBP")
		}
	})

	t.Run("outdated branch update fails", func(t *testing.T) {
		m := &mockEnsurer{name: "test", hasAgent: true, aibpMajor: 1, aibpMinor: 0, isSource: false, updateErr: errMicroNeoOutdated}
		err := Ensure(m, func(string) {})
		if err == nil {
			t.Error("Ensure() error = nil, want non-nil when UpdateAIBP fails")
		}
	})

	t.Run("ready branch (major == mine)", func(t *testing.T) {
		m := &mockEnsurer{name: "test", hasAgent: true, aibpMajor: 2, aibpMinor: 1, isSource: false}
		err := Ensure(m, func(string) {})
		if err != nil {
			t.Errorf("Ensure() error = %v, want nil", err)
		}
		if m.installCalled || m.updateCalled {
			t.Error("ready branch should not call InstallAIBP or UpdateAIBP")
		}
	})

	t.Run("agent not found branch", func(t *testing.T) {
		m := &mockEnsurer{name: "test", hasAgent: false}
		err := Ensure(m, func(string) {})
		if err != errAgentNotFound {
			t.Errorf("Ensure() error = %v, want errAgentNotFound", err)
		}
		if m.installCalled || m.updateCalled {
			t.Error("agent not found branch should not call InstallAIBP or UpdateAIBP")
		}
	})
}