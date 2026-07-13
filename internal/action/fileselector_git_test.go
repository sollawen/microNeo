package action

import (
	"reflect"
	"testing"
)

func TestParsePorcelain(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		input       []byte
		want        map[string]rune
		branch      string
		allIgnored  bool
	}{
		{
			name:   "empty",
			input:  []byte{},
			want:   map[string]rune{},
			branch: "",
		},
		{
			name:   "single modified file",
			input:  []byte(" M main.go\x00"),
			want:   map[string]rune{"main.go": 'M'},
			branch: "",
		},
		{
			name:   "untracked file",
			input:  []byte("?? newfile.txt\x00"),
			want:   map[string]rune{"newfile.txt": 'U'},
			branch: "",
		},
		{
			name:   "staged added file",
			input:  []byte("A  test.go\x00"),
			want:   map[string]rune{"test.go": 'A'},
			branch: "",
		},
		{
			name:   "deleted file",
			input:  []byte(" D old.go\x00"),
			want:   map[string]rune{"old.go": 'D'},
			branch: "",
		},
		{
			name:   "renamed file",
			input:  []byte("R  oldname.go\x00"),
			want:   map[string]rune{"oldname.go": 'R'},
			branch: "",
		},
		{
			name:   "multiple files with spaces",
			input:  []byte(" M my file.go\x00?? file with spaces.txt\x00"),
			want:   map[string]rune{"my file.go": 'M', "file with spaces.txt": 'U'},
			branch: "",
		},
		{
			name:   "Chinese filename",
			input:  []byte("?? 中文文件.txt\x00 M 我的项目.go\x00"),
			want:   map[string]rune{"中文文件.txt": 'U', "我的项目.go": 'M'},
			branch: "",
		},
		{
			name:   "subdirectory propagation (browsing sub/)",
			prefix: "sub/",
			input:  []byte(" M sub/dir/file.go\x00"),
			want:   map[string]rune{"dir": 'M'},
			branch: "",
		},
		{
			name:   "MM (staged and modified)",
			input:  []byte("MM both.go\x00"),
			want:   map[string]rune{"both.go": 'M'},
			branch: "",
		},
		{
			name:   "AM (added in index, modified in worktree)",
			input:  []byte("AM mixed.go\x00"),
			want:   map[string]rune{"mixed.go": 'M'}, // M 优先级更高
			branch: "",
		},
		{
			name:   "ignored short record",
			input:  []byte("M\x00"),
			want:   map[string]rune{},
			branch: "",
		},
		{
			name:   "complex mixed output",
			input:  []byte("M  modified.go\x00?? untracked.md\x00A  added.go\x00 D deleted.go\x00"),
			want:   map[string]rune{"modified.go": 'M', "untracked.md": 'U', "added.go": 'A', "deleted.go": 'D'},
			branch: "",
		},
		// F1c 新增 case
		{
			name:   "root view three levels deep",
			input:  []byte(" M a/b/c/deep.go\x00"),
			want:   map[string]rune{"a": 'M'},
			branch: "",
		},
		{
			name:   "subdirectory view (internal/)",
			prefix: "internal/",
			input:  []byte(" M internal/action/a.go\x00"),
			want:   map[string]rune{"action": 'M'},
			branch: "",
		},
		{
			name:   "file directly in subdirectory (internal/flat.go)",
			prefix: "internal/",
			input:  []byte(" M internal/flat.go\x00"),
			want:   map[string]rune{"flat.go": 'M'},
			branch: "",
		},
		{
			name:   "multi-status aggregation in root view (M wins over A)",
			input:  []byte("A  x/added.go\x00 M x/mod.go\x00"),
			want:   map[string]rune{"x": 'M'},
			branch: "",
		},
		{
			name:   "backslash filename not mis-split (Unix \\ is valid char)",
			input:  []byte(" M a\\b.go\x00"),
			want:   map[string]rune{"a\\b.go": 'M'},
			branch: "",
		},
		// —— 分支头捕获（-b 产生的 "## " 首记录）——
		{
			name:   "branch header normal",
			input:  []byte("## main\x00 M main.go\x00"),
			want:   map[string]rune{"main.go": 'M'},
			branch: "main",
		},
		{
			name:   "branch with upstream (main...origin/main)",
			input:  []byte("## main...origin/main\x00 M a\x00"),
			want:   map[string]rune{"a": 'M'},
			branch: "main",
		},
		{
			name:   "branch with ahead/behind suffix",
			input:  []byte("## main [ahead 1]\x00"),
			want:   map[string]rune{},
			branch: "main",
		},
		{
			name:   "detached HEAD → empty branch",
			input:  []byte("## HEAD (no branch)\x00?? a\x00"),
			want:   map[string]rune{"a": 'U'},
			branch: "",
		},
		{
			name:   "unborn branch (No commits yet on main) → main",
			input:  []byte("## No commits yet on main\x00?? a\x00"),
			want:   map[string]rune{"a": 'U'},
			branch: "main",
		},
		// —— ignored（!!）状态字符（fix "I 冒泡" bug）——
		{
			name:        "ignored file in subdir → no bubble (fix)",
			input:       []byte("!! build/artifact.o\x00"),
			want:        map[string]rune{}, // 深处单文件不冒泡到父目录
			branch:      "",
			allIgnored:  false,
		},
		{
			name:        "ignored single file in current dir → I on file",
			input:       []byte("!! secret.md\x00"),
			want:        map[string]rune{"secret.md": 'I'},
			branch:      "",
			allIgnored:  false,
		},
		{
			name:        "ignored direct subdir (node_modules/) → I on name",
			input:       []byte("!! node_modules/\x00"),
			want:        map[string]rune{"node_modules": 'I'},
			branch:      "",
			allIgnored:  false,
		},
		{
			name:        "ignored deep subdir (aaa/bbb/ccc/) from bbb/ level → I on ccc",
			prefix:      "aaa/bbb/",
			input:       []byte("!! aaa/bbb/ccc/\x00"),
			want:        map[string]rune{"ccc": 'I'},
			branch:      "",
			allIgnored:  false,
		},
		{
			name:        "ignored deep subdir from root → no bubble (too deep)",
			prefix:      "aaa/",
			input:       []byte("!! aaa/bbb/ccc/\x00"),
			want:        map[string]rune{},
			branch:      "",
			allIgnored:  false,
		},
		{
			name:        "browsing into ignored dir itself → allIgnored",
			prefix:      "aaa/bbb/ccc/",
			input:       []byte("!! aaa/bbb/ccc/\x00"),
			want:        map[string]rune{},
			branch:      "",
			allIgnored:  true, // rel==""：当前目录本身被 ignore
		},
		{
			name:        "M + ignored file in subdir → M not polluted by I bubble",
			input:       []byte(" M x/mod.go\x00!! x/ignored.log\x00"),
			want:        map[string]rune{"x": 'M'}, // ignored 深处单文件被丢弃
			branch:      "",
			allIgnored:  false,
		},
		{
			name:        "ignored file deep in subtree from root → no bubble",
			prefix:      "aaa/",
			input:       []byte("!! aaa/bbb/ccc/secret.md\x00"),
			want:        map[string]rune{},
			branch:      "",
			allIgnored:  false,
		},
		{
			name:        "ignored file deep in subtree from immediate parent → I on file",
			prefix:      "aaa/bbb/ccc/",
			input:       []byte("!! aaa/bbb/ccc/secret.md\x00"),
			want:        map[string]rune{"secret.md": 'I'},
			branch:      "",
			allIgnored:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chars, branch, allIgnored := parsePorcelain(tt.input, tt.prefix)
			if !reflect.DeepEqual(chars, tt.want) {
				t.Errorf("chars = %#v, want %#v", chars, tt.want)
			}
			if branch != tt.branch {
				t.Errorf("branch = %q, want %q", branch, tt.branch)
			}
			if allIgnored != tt.allIgnored {
				t.Errorf("allIgnored = %v, want %v", allIgnored, tt.allIgnored)
			}
		})
	}
}
