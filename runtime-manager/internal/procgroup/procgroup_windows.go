//go:build windows

// Package procgroup 子进程组启停的跨平台最小封装（REQ-321 Windows 移植）。
// Windows 无进程组信号：graceful 用 taskkill /PID（投递 WM_CLOSE，控制台程序通常忽略），
// 非优雅用 taskkill /F /T（等价 unix 负 PID 组杀的整树终止）。引擎停止语义由调用方的
// 「宽限等待 + 超时强杀」阶梯承载，与 unix 行为对齐。
package procgroup

import (
	"os/exec"
	"strconv"
	"syscall"
)

// CREATE_NO_WINDOW：服务进程派生 taskkill 不闪控制台窗。
const createNoWindow = 0x08000000

// SetPgid Windows 无进程组概念，空操作（整树终止由 Kill 的 taskkill /T 承载）。
func SetPgid(cmd *exec.Cmd) {}

// Kill 终止进程树：graceful=true 试 taskkill（WM_CLOSE），false 直接 taskkill /F /T。
func Kill(pid int, graceful bool) error {
	args := []string{"/PID", strconv.Itoa(pid)}
	if !graceful {
		args = append([]string{"/F", "/T"}, args...)
	}
	cmd := exec.Command("taskkill", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd.Run()
}
