//go:build !windows

// Package procgroup 子进程组启停的跨平台最小封装（REQ-321 Windows 移植）。
// unix 语义保持不变：Setpgid 独立进程组 + 负 PID 组信号（SIGTERM 宽限 → SIGKILL 兜底）。
package procgroup

import (
	"os/exec"
	"syscall"
)

// SetPgid 启动前将子进程置于独立进程组（负 PID 组信号的处置前提）。
func SetPgid(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// Kill 终止进程组：graceful=true 发 SIGTERM，false 发 SIGKILL。
func Kill(pid int, graceful bool) error {
	sig := syscall.SIGKILL
	if graceful {
		sig = syscall.SIGTERM
	}
	return syscall.Kill(-pid, sig)
}
