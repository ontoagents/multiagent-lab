//go:build !windows

package companion

import (
	"os/exec"
	"syscall"
)

// setPgid 子进程置于独立进程组（负 PID 组信号的处置前提；Windows 移植见 procgroup_windows.go，REQ-321）。
func setPgid(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// killTree 组信号终止：graceful=true 发 SIGTERM，false 发 SIGKILL。
func killTree(pid int, graceful bool) error {
	sig := syscall.SIGKILL
	if graceful {
		sig = syscall.SIGTERM
	}
	return syscall.Kill(-pid, sig)
}
