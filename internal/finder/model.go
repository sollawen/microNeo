package finder

import (
	"os"
	"sort"
	"strings"
)

// sortMode 决定文件组内的排序键（目录组恒先、恒按名排）。
type sortMode uint8

const (
	sortName sortMode = iota // 字母（大小写不敏感）
	sortSize                 // size
	sortTime                 // mtime
)

// entry 是一个目录条目。gitChar 在 readDir 时为 0（干净/非仓库），后台 git 查询回来后填充。
// info 存全量 FileInfo（lstat，不跟随 symlink），恒非 nil（d.Info() 失败的条目直接跳过）。
type entry struct {
	name    string
	isDir   bool
	info    os.FileInfo
	gitChar rune // git 状态字符 'M'/'U'/'A'/'R'/'I'；0=干净/非仓库
}

// readDirEntries 读目录【全部】条目（含 hidden，不过滤）、建 entry（gitChar=0）、
// d.Info() 失败则跳过、排序返回。只在 chdir 时调一次（不再每次 toggle 重读）。
func readDirEntries(dir string, sm sortMode, desc bool) []entry {
	dirEntries, _ := os.ReadDir(dir)
	all := make([]entry, 0, len(dirEntries))
	for _, d := range dirEntries {
		info, err := d.Info() // lstat，不跟随 symlink，对齐 ls -l；失败跳过
		if err != nil {
			continue
		}
		all = append(all, entry{name: d.Name(), isDir: d.IsDir(), info: info /* gitChar=0 */})
	}
	sortEntries(all, sm, desc)
	return all
}

// sortEntries 原地排序：目录恒在文件前；目录组恒按名升序；文件组主键随 sm + desc，
// 并列恒回退字母升序（不受 desc 影响，保稳定）。readDirEntries 与 sort 切换都调它。
func sortEntries(all []entry, sm sortMode, desc bool) {
	lessName := func(a, b entry) bool { return strings.ToLower(a.name) < strings.ToLower(b.name) }
	sort.SliceStable(all, func(i, j int) bool {
		ai, aj := all[i], all[j]
		if ai.isDir != aj.isDir {
			return ai.isDir // 目录恒在文件前
		}
		if ai.isDir {
			return lessName(ai, aj) // 目录组恒按名升序
		}
		switch sm { // 文件组
		case sortSize:
			si, sj := ai.info.Size(), aj.info.Size()
			if si == sj {
				return lessName(ai, aj)
			}
			if desc {
				return si > sj
			}
			return si < sj
		case sortTime:
			ti, tj := ai.info.ModTime(), aj.info.ModTime()
			if ti.Equal(tj) {
				return lessName(ai, aj)
			}
			if desc {
				return ti.After(tj)
			}
			return ti.Before(tj)
		default: // sortName
			if strings.EqualFold(ai.name, aj.name) {
				return ai.name < aj.name // 大小写并列：原样升序
			}
			if desc {
				return lessName(aj, ai)
			}
			return lessName(ai, aj)
		}
	})
}
