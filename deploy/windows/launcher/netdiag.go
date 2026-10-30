// 端口占用诊断（REQ-321）：解析 netstat -ano 找到 LISTENING 套接字的持有 PID，
// 再经 QueryFullProcessImageName 取进程完整路径——错误提示里给用户「谁占的、在哪」，
// 而不是一句笼统的「端口被占用」。
package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

type portOwner struct {
	Port int
	PID  int
	Name string // 进程名（含扩展名）
	Path string // 完整路径（权限不足时为空）
}

// findPortOwners 返回指定端口上的所有 LISTENING 持有者（正常至多一个）。
func findPortOwners(port int) []portOwner {
	out, err := exec.Command("netstat", "-ano", "-p", "TCP").CombinedOutput()
	if err != nil {
		return nil
	}
	suffix := fmt.Sprintf(":%d", port)
	var owners []portOwner
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		if !strings.HasSuffix(fields[1], suffix) {
			continue
		}
		pid, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil || pid <= 0 {
			continue
		}
		name, path := processImage(pid)
		owners = append(owners, portOwner{Port: port, PID: pid, Name: name, Path: path})
	}
	return owners
}

// processImage 进程名与完整路径（OpenProcess 受限或短期进程 → 空串兜底）。
func processImage(pid int) (name, path string) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// 退化：tasklist 按名兜底（本地化输出不影响 CSV 形态）
		if out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").CombinedOutput(); err == nil {
			line := strings.TrimSpace(string(out))
			if i := strings.Index(line, `"`); i >= 0 {
				if j := strings.Index(line[i+1:], `"`); j >= 0 {
					return line[i+1 : i+1+j], ""
				}
			}
		}
		return fmt.Sprintf("PID %d", pid), ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return fmt.Sprintf("PID %d", pid), ""
	}
	full := strings.ReplaceAll(windows.UTF16ToString(buf[:size]), "\\", "/")
	return filepath.Base(full), full
}

// oursServiceName 判断占用者是否本平台服务（launcher/三服务同名 exe——回收安全）。
func oursServiceName(o portOwner) (string, bool) {
	for _, n := range []string{"backend.exe", "ontologyd.exe", "runtimed.exe", "OntoAgents.exe"} {
		if strings.EqualFold(o.Name, n) {
			return n, true
		}
	}
	return "", false
}
