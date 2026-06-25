package aibp

import "testing"

func TestParseProtocol(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMaj int
		wantMin int
		wantOk  bool
	}{
		// valid formats
		{"aibp-2", "aibp-2", 2, 0, true},
		{"aibp-2.0", "aibp-2.0", 2, 0, true},
		{"aibp-2.1", "aibp-2.1", 2, 1, true},
		{"aibp-1.10", "aibp-1.10", 1, 10, true},
		{"aibp-10.1", "aibp-10.1", 10, 1, true},
		{"aibp-0", "aibp-0", 0, 0, true},
		{"aibp-0.0", "aibp-0.0", 0, 0, true},
		// malformed formats
		{"empty string", "", 0, 0, false},
		{"no aibp- prefix", "invalid", 0, 0, false},
		{"aibp- only (no number)", "aibp-", 0, 0, false},
		{"aibp-2. (no minor)", "aibp-2.", 0, 0, false},
		{"aibp-.0 (no major)", "aibp-.0", 0, 0, false},
		{"aibp-XYZ (non-numeric)", "aibp-XYZ", 0, 0, false},
		{"aibp-2.ABC (non-numeric minor)", "aibp-2.ABC", 0, 0, false},
		{"aibp-2..0 (double dot)", "aibp-2..0", 0, 0, false},
		{"extra suffix", "aibp-2.0-extra", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maj, min, ok := ParseProtocol(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ParseProtocol(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
				return
			}
			if ok && (maj != tt.wantMaj || min != tt.wantMin) {
				t.Errorf("ParseProtocol(%q) = (%d,%d,_), want (%d,%d,_)", tt.input, maj, min, tt.wantMaj, tt.wantMin)
			}
		})
	}
}