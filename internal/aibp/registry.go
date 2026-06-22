package aibp

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// RegistryDir — 说明-AIBP §3.1。MNAB_REG_DIR 覆盖优先（调试便利说明在该节末尾）
func RegistryDir() string {
	if d := os.Getenv("MNAB_REG_DIR"); d != "" {
		return d
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.Getenv("TMPDIR")
	}
	if base == "" {
		base = "/tmp"
	}
	return filepath.Join(base, fmt.Sprintf("microneo-agent-bridge-%d", os.Getuid()))
}

// RegFile — 说明-AIBP §3.2 注册文件内容
type RegFile struct {
	Name      string   `json:"name"`
	PID       int      `json:"pid"`
	Transport string   `json:"transport"` // "unix"
	Socket    string   `json:"socket"`
	Protocol  string   `json:"protocol"` // "aibp-1"
	StartedAt int64    `json:"startedAt"`
	Cwd       string   `json:"cwd,omitempty"`
	Labels    []string `json:"labels,omitempty"`
}

// Discover — 扫描注册表，返回存活 receiver。目录不存在/空目录均返回空 slice，不报错。
//   顺手 GC 僵尸（说明-AIBP §3.5）。
//   存活判据：connect socket 为权威；connect 成功 → 活。connect 失败 → 再看 PID：
//   PID 已死 → GC（删注册文件），不计入。PID 仍活 → 保留但不推荐（socket 可能暂不可用）。
func Discover() ([]RegFile, error) {
	dir := RegistryDir()
	entries, err := filepath.Glob(filepath.Join(dir, "ai-*.json"))
	if err != nil {
		return nil, err
	}
	var live []RegFile
	for _, p := range entries {
		var rf RegFile
		if b, e := os.ReadFile(p); e != nil {
			continue
		} else if json.Unmarshal(b, &rf) != nil {
			continue
		}
		// 主版本不符 → 跳过（说明-AIBP §7.2）。字符串形如 "aibp-1"
		if MajorVersion(rf.Protocol) != MajorVersion(Protocol) {
			continue
		}
		if alive(rf.Socket) {
			live = append(live, rf)
			continue
		}
		if !pidAlive(rf.PID) {
			_ = os.Remove(p) // GC 僵尸
		}
	}
	return live, nil
}

// alive — connect 试探为权威判据（说明-AIBP §3.5）
func alive(socket string) bool {
	c, err := net.DialTimeout("unix", socket, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// pidAlive — 跨平台 kill(0) 旁证。Windows 上 syscall 用法不同，v1 仅 Unix
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// MajorVersion — 解析 "aibp-1" 取 1（说明-AIBP §7.3）
// 被 ensure_agents 子包使用，导出为公共 API
func MajorVersion(protocol string) int {
	i := strings.LastIndexByte(protocol, '-')
	if i < 0 {
		return -1
	}
	n, err := strconv.Atoi(protocol[i+1:])
	if err != nil {
		return -1
	}
	return n
}