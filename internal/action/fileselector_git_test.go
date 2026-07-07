package action

import (
	"testing"
)

func TestParsePorcelain(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected map[string]statusKind
	}{
		{
			name:  "empty",
			input: []byte{},
			expected: map[string]statusKind{},
		},
		{
			name:  "single modified file",
			input: []byte(" M main.go\x00"),
			expected: map[string]statusKind{
				"main.go": stModified,
			},
		},
		{
			name:  "untracked file",
			input: []byte("?? newfile.txt\x00"),
			expected: map[string]statusKind{
				"newfile.txt": stUntracked,
			},
		},
		{
			name:  "staged added file",
			input: []byte("A  test.go\x00"),
			expected: map[string]statusKind{
				"test.go": stAdded,
			},
		},
		{
			name:  "deleted file",
			input: []byte(" D old.go\x00"),
			expected: map[string]statusKind{
				"old.go": stDeleted,
			},
		},
		{
			name:  "renamed file",
			input: []byte("R  oldname.go\x00"),
			expected: map[string]statusKind{
				"oldname.go": stRenamed,
			},
		},
		{
			name:  "multiple files with spaces",
			input: []byte(" M my file.go\x00?? file with spaces.txt\x00"),
			expected: map[string]statusKind{
				"my file.go":          stModified,
				"file with spaces.txt": stUntracked,
			},
		},
		{
			name:  "Chinese filename",
			input: []byte("?? 中文文件.txt\x00 M 我的项目.go\x00"),
			expected: map[string]statusKind{
				"中文文件.txt": stUntracked,
				"我的项目.go":   stModified,
			},
		},
		{
			name:  "file in subdirectory (base only)",
			input: []byte(" M sub/dir/file.go\x00"),
			expected: map[string]statusKind{
				"file.go": stModified,
			},
		},
		{
			name:  "MM (staged and modified)",
			input: []byte("MM both.go\x00"),
			expected: map[string]statusKind{
				"both.go": stModified,
			},
		},
		{
			name:  "AM (added in index, modified in worktree)",
			input: []byte("AM mixed.go\x00"),
			expected: map[string]statusKind{
				"mixed.go": stModified, // M 优先级更高
			},
		},
		{
			name:  "ignored short record",
			input: []byte("M\x00"),
			expected: map[string]statusKind{},
		},
		{
			name:  "complex mixed output",
			input: []byte("M  modified.go\x00?? untracked.md\x00A  added.go\x00 D deleted.go\x00"),
			expected: map[string]statusKind{
				"modified.go":  stModified,
				"untracked.md": stUntracked,
				"added.go":     stAdded,
				"deleted.go":   stDeleted,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePorcelain(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entries, got %d", len(tt.expected), len(result))
				return
			}
			for name, expectedSt := range tt.expected {
				gotSt, ok := result[name]
				if !ok {
					t.Errorf("missing entry for %q", name)
					continue
				}
				if gotSt != expectedSt {
					t.Errorf("for %q: expected %v, got %v", name, expectedSt, gotSt)
				}
			}
		})
	}
}
