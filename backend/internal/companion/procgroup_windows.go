//go:build windows

package companion

import (
	"os/exec"
	"strconv"
	"syscall"
)

// CREATE_NO_WINDOW：服务进程派生 taskkill 不闪控制台窗。
const killNoWindow = 0x08000000

// setPgid Windows 无进程组概念，空操作（整树终止由 killTree 的 taskkill /T 承载，REQ-321）。
func setPgid(cmd *exec.Cmd) {}

// killTree taskkill 整树终止。graceful 参数仅 unix 有意义（Windows 的 oxigraph 为控制台进程
// 不响应 WM_CLOSE，宽限投递恒无效），故 Windows 侧一律 taskkill /F /T 硬终止；
// 旧伴生实例为迁移期一次性进程，硬终止语义由 launcher 的 Job Object 兜底覆盖。
func killTree(pid int, graceful bool) error {
	args := []string{"/F", "/T", "/PID", strconv.Itoa(pid)}
	cmd := exec.Command("taskkill", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: killNoWindow}
	return cmd.Run()
}
