//go:build windows

package aibp

import "golang.org/x/sys/windows"

// pidAlive — Windows 上用 OpenProcess + GetExitCodeProcess 判活。
// connect 试探仍是权威判据（说明-AIBP §3.5）；本函数仅作 GC 旁证。
// STILL_ACTIVE (= 259，Windows SDK 常量；x/sys/windows 未导出，这里用字面量)
// 表示进程仍在运行；OpenProcess 失败（无权限或不存在）或取到退出码（非 259）表示进程已退出。
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false // OpenProcess 失败 = 进程不存在或无权限 → 视为已死
	}
	defer windows.CloseHandle(h)

	const stillActive = 259 // STILL_ACTIVE
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
