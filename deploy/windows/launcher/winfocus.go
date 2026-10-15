// 单实例聚焦（REQ-321 端口占用问题的用户侧出口之一）：
// 已有本客户端窗口在运行时，再次启动不做双实例撞端口，而是唤起既有窗口（还原+前置）并退出。
// 识别口径：窗口标题精确等于 appName 且属于其他进程（launcher 窗口标题固定，setup 向导标题带
// 「安装向导」后缀不会误中）。
package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procEnumWindows        = user32.NewProc("EnumWindows")
	procGetWindowTextW     = user32.NewProc("GetWindowTextW")
	procShowWindow         = user32.NewProc("ShowWindow")
	procSetForeground      = user32.NewProc("SetForegroundWindow")
	procIsIconic           = user32.NewProc("IsIconic")
	procGetWindowThreadPID = user32.NewProc("GetWindowThreadProcessId")
)

// focusExistingWindow 找到其他进程的客户端主窗口则还原+前置并返回 true（调用方应退出）。
func focusExistingWindow() bool {
	self := windows.GetCurrentProcessId()
	found := false
	cb := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		var pid uint32
		procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == 0 || pid == self {
			return 1
		}
		var buf [128]uint16
		n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n == 0 {
			return 1
		}
		if windows.UTF16ToString(buf[:n]) != appName {
			return 1
		}
		// 最小化则还原，再前置
		if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
			procShowWindow.Call(hwnd, 9 /*SW_RESTORE*/)
		}
		procSetForeground.Call(hwnd)
		found = true
		return 0
	})
	procEnumWindows.Call(cb, 0)
	return found
}
