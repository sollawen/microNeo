//go:build !windows

package aibp

import "syscall"

// pidAlive — Unix 上用 kill(pid, 0) 旁证。
// 注册表 connect 失败时，PID 已死 → 视为僵尸由 Discover GC 删除注册文件；
// PID 仍活 → 保留但不推荐（socket 可能暂不可用）。
// Windows 实现见 pidAlive_windows.go。
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
