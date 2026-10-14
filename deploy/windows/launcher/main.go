// MultiAgentLab Windows 桌面客户端壳与进程编排（REQ-321/M123）。
//
// 职责（对应 10 号跨端方案 R-X3/R-X11/SC-X1 的 Windows 单平台先行实现）：
//   - 编排三服务启动：本体两平面（ontologyd :8091 → runtimed :8090）→ 主平台 backend :8080，
//     环境变量与 run-dev.sh 一一同构（部署见 platform-knowledge/01_整体设计/16_部署与运行.md）；
//   - 数据根独立于程序目录：%LOCALAPPDATA%\eino-lab（卸载/升级程序目录时数据自然保留）；
//   - WebView2 窗口承载 Web UI（无浏览器依赖的桌面客户端形态；运行时缺失时回退系统默认浏览器）；
//   - Job Object（KILL_ON_JOB_CLOSE）保证壳退出（关窗）时整棵进程树回收，无孤儿进程；
//   - data\platform.pids 留痕 + `-stop` 幂等停服（壳异常退出后的兜底回收手段）；
//   - `-headless` 无窗口编排（冒烟自动化用）：就绪后打印 READY 并驻留。
//
// 诚实边界（v1）：单机单实例（端口 8080/8091/8090 固定）；SQLite 为 WAL 模式，停服为进程树
// 终止（崩溃安全由 WAL 承担，不持久化连接排空）；不支持开机自启/系统托盘/自动更新（R-X9/X10 后续，
// Tauri 2 壳定案项见 18 号 REQ-321 行）。
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

// ShellVersion 客户端壳版本（与 installer.iss 的 AppVersion 保持一致）。
const ShellVersion = "1.0.0"

// appName 窗口标题与提示文案用名。
const appName = "智能体构建平台"

const (
	backendURL = "http://127.0.0.1:8080"
	healthPath = "/healthz"
)

// CREATE_NO_WINDOW：子进程为控制台程序，GUI 进程派生时不弹黑窗。
const createNoWindow = 0x08000000

type service struct {
	name string // exe 名（backend.exe 等，-stop 兜底按名回收用）
	exe  string
	env  [][2]string
	log  string
}

func main() {
	stop := flag.Bool("stop", false, "停止本安装的三个平台服务并退出")
	headless := flag.Bool("headless", false, "无窗口编排：启动服务、等待就绪后打印 READY 并驻留（冒烟自动化）")
	flag.Parse()

	root := exeDir()
	dataDir := dataRoot()
	logDir := filepath.Join(dataDir, "logs")

	switch {
	case *stop:
		os.Exit(stopServices(dataDir))
	default:
		os.Exit(run(root, dataDir, logDir, *headless))
	}
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法定位可执行文件目录:", err)
		os.Exit(1)
	}
	return filepath.Dir(exe)
}

// dataRoot 数据根：%LOCALAPPDATA%\eino-lab（REQ-321 定案；环境异常退回安装目录自包含）。
func dataRoot() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return exeDir()
	}
	return filepath.Join(localAppData, "eino-lab")
}

// run 常规入口：清理残留 → 拉起三服务 → 等主平台就绪 → 窗口（或 headless 驻留）→ 退出回收。
func run(root, dataDir, logDir string, headless bool) int {
	for _, d := range []string{dataDir, logDir,
		filepath.Join(dataDir, "projects"), filepath.Join(dataDir, "engines"),
		filepath.Join(dataDir, "engine_logs"), filepath.Join(dataDir, "bin")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fail(headless, "无法创建数据目录 "+d+": "+err.Error())
		}
	}

	if code := cleanupStale(dataDir, headless); code != 0 {
		return code
	}

	svcs := buildServices(root, dataDir)
	job, err := newKillOnCloseJob()
	if err != nil {
		return fail(headless, "创建进程作业对象失败: "+err.Error())
	}
	defer windows.CloseHandle(job)

	pids := make([]int, 0, len(svcs))
	defer func() {
		// Job 对象关闭即整树终止（defer 顺序晚于显式 terminateJob 亦幂等）。
		windows.TerminateJobObject(job, 0)
		removePIDFile(dataDir)
	}()

	for _, svc := range svcs {
		cmd, err := spawn(svc, root, logDir)
		if err != nil {
			return fail(headless, "启动 "+svc.name+" 失败: "+err.Error())
		}
		if err := assignToJob(job, cmd.Process.Pid); err != nil {
			_ = cmd.Process.Kill()
			return fail(headless, svc.name+" 加入进程作业失败: "+err.Error())
		}
		pids = append(pids, cmd.Process.Pid)
	}
	writePIDFile(dataDir, pids)

	// 本体两平面就绪不阻塞主平台窗口（与 run-dev.sh 语义一致：失败仅留日志），
	// 但 backend 必须就绪——窗口要承载它。
	if !waitHealthy(backendURL+healthPath, 20*time.Second) {
		tail := tailFile(filepath.Join(logDir, "backend.log"), 800)
		return fail(headless, "主平台未在 20 秒内就绪（端口 8080）。backend.log 末尾：\n"+tail)
	}

	if headless {
		fmt.Printf("READY %s shell=%s data=%s\n", backendURL, ShellVersion, dataDir)
		waitInterrupt()
		return 0
	}

	return runShell(backendURL)
}

// buildServices 三服务环境面——与 run-dev.sh / 16 号部署文档 §7 一一同构；路径全部钉绝对
// （正斜杠形态，Windows API 原生接受且与仓库内 unix 风格相对路径习惯一致）。
// 数据类路径落 dataDir（%LOCALAPPDATA%\eino-lab）；程序类路径落安装根 root。
func buildServices(root, dataDir string) []service {
	p := func(elem ...string) string { return filepath.ToSlash(filepath.Join(elem...)) }
	oxigraph := p(root, "bin", "oxigraph.exe")

	svcs := []service{
		{
			name: "ontologyd.exe", exe: p(root, "bin", "ontologyd.exe"), log: "ontology-service.log",
			env: [][2]string{
				{"ADDR", ":8091"},
				{"DB_PATH", p(dataDir, "ontology.db")},
				{"MIGRATIONS_DIR", p(root, "migrations", "ontology-service")},
				{"SIDECAR_SCRIPT", p(root, "tools", "rdf-sidecar", "sidecar.py")},
				{"PLATFORM_URL", backendURL},
			},
		},
		{
			name: "runtimed.exe", exe: p(root, "bin", "runtimed.exe"), log: "runtime-manager.log",
			env: [][2]string{
				{"ADDR", ":8090"},
				{"DB_PATH", p(dataDir, "runtime.db")},
				{"MIGRATIONS_DIR", p(root, "migrations", "runtime-manager")},
				{"BUILD_SVC_URL", "http://127.0.0.1:8091"},
				{"ENGINE_DATA_DIR", p(dataDir, "engines")},
				{"ENGINE_LOG_DIR", p(dataDir, "engine_logs")},
				{"ENGINE_INSTALL_DIR", p(dataDir, "bin")},
			},
		},
		{
			name: "backend.exe", exe: p(root, "bin", "backend.exe"), log: "backend.log",
			env: [][2]string{
				{"ADDR", ":8080"},
				{"DB_PATH", p(dataDir, "platform.db")},
				{"SECRET_KEY_FILE", p(dataDir, ".secret")},
				{"FILES_ROOT", p(dataDir, "projects")},
				{"WEB_DIST", p(root, "web", "dist")},
				{"KB_VECTOR_BACKEND", "sqlite"}, // 单机零依赖默认（README 注明 qdrant 覆盖方式）
				{"DOCS_ROOT", p(root, "docs")},
				{"RESEARCH_ROOT", p(root, "research")},
				{"KNOWLEDGE_ROOT", p(root, "platform-knowledge")},
			},
		},
	}
	// oxigraph 按 REQ-321 定案不随包分发：缺失不阻塞——首启后在「本体-运行」页一键安装
	// （写入 data\bin 即时命中 ENGINE_INSTALL_DIR，REQ-146），或构建期 -IncludeOxigraph 预置。
	if fileExists(oxigraph) {
		svcs[1].env = append(svcs[1].env, [2]string{"OXIGRAPH_BIN", oxigraph})
	}
	return svcs
}

func spawn(svc service, root, logDir string) (*exec.Cmd, error) {
	f, err := os.OpenFile(filepath.Join(logDir, svc.log), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(f, "\n==== %s 启动 %s ====\n", svc.name, time.Now().Format("2006-01-02 15:04:05"))

	cmd := exec.Command(svc.exe)
	cmd.Dir = root
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.Env = os.Environ()
	for _, kv := range svc.env {
		cmd.Env = append(cmd.Env, kv[0]+"="+kv[1])
	}
	// HideWindow+CREATE_NO_WINDOW：windowsgui 壳派生控制台子进程不闪黑窗。
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		f.Close()
		return nil, err
	}
	// 生命周期归 Job/日志文件，进程句柄即刻释放。
	go func() { cmd.Wait(); f.Close() }()
	return cmd, nil
}

// newKillOnCloseJob R-X11 退出回收载体：作业对象最后一个句柄关闭时整树终止。
func newKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func assignToJob(job windows.Handle, pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.AssignProcessToJobObject(job, h)
}

// cleanupStale 启动前回收：上次壳异常退出遗留的服务进程（pids 留痕优先，exe 名兜底）。
func cleanupStale(dataDir string, headless bool) int {
	if pids := readPIDFile(dataDir); len(pids) > 0 {
		for _, pid := range pids {
			killPID(pid)
		}
		removePIDFile(dataDir)
		time.Sleep(300 * time.Millisecond) // 端口释放窗口
	}
	if portBusy(8080) || portBusy(8090) || portBusy(8091) {
		for _, name := range []string{"backend.exe", "ontologyd.exe", "runtimed.exe"} {
			killImage(name)
		}
		time.Sleep(500 * time.Millisecond)
		if portBusy(8080) {
			return fail(headless, "端口 8080 被占用且无法回收（可能为手动启动的开发实例），请先停止后重试。")
		}
	}
	return 0
}

func stopServices(dataDir string) int {
	pids := readPIDFile(dataDir)
	if len(pids) == 0 {
		// 无留痕（如手动启动）：按镜像名兜底，与本安装的固定 exe 名一一对应。
		for _, name := range []string{"backend.exe", "ontologyd.exe", "runtimed.exe", "MultiAgentLab.exe"} {
			killImage(name)
		}
		fmt.Println("已按进程名停止平台服务（无 PID 留痕）。")
		return 0
	}
	for _, pid := range pids {
		killPID(pid)
	}
	removePIDFile(dataDir)
	fmt.Printf("已停止 %d 个平台服务进程。\n", len(pids))
	return 0
}

// runShell WebView2 桌面窗口；运行时缺失时回退系统默认浏览器（诚实降级）。
func runShell(url string) int {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  appName,
			Width:  1440,
			Height: 900,
			Center: true,
		},
	})
	if w == nil {
		// WebView2 Runtime 未安装（Win10 老版本可能缺失）：回退浏览器并提示安装运行时。
		openInBrowser(url)
		msgBox("WebView2 运行时缺失", "未检测到 Microsoft WebView2 运行时，已在系统浏览器中打开平台。\n如需桌面窗口形态，请安装 WebView2 Runtime：\nhttps://developer.microsoft.com/microsoft-edge/webview2/")
		return 0
	}
	defer w.Destroy()
	w.Navigate(url)
	w.Run() // 阻塞至窗口关闭；返回后由调用方 defer 回收进程树
	return 0
}

// ---- PID 留痕 / 进程回收 / 探活 / 小工具 ----

func pidFile(dataDir string) string { return filepath.Join(dataDir, "platform.pids") }

func writePIDFile(dataDir string, pids []int) {
	_ = os.MkdirAll(dataDir, 0o755)
	lines := make([]string, 0, len(pids))
	for _, p := range pids {
		lines = append(lines, strconv.Itoa(p))
	}
	_ = os.WriteFile(pidFile(dataDir), []byte(strings.Join(lines, "\n")), 0o644)
}

func readPIDFile(dataDir string) []int {
	b, err := os.ReadFile(pidFile(dataDir))
	if err != nil {
		return nil
	}
	var out []int
	for _, line := range strings.Split(string(b), "\n") {
		if p, err := strconv.Atoi(strings.TrimSpace(line)); err == nil && p > 0 {
			out = append(out, p)
		}
	}
	return out
}

func removePIDFile(dataDir string) { _ = os.Remove(pidFile(dataDir)) }

func killPID(pid int) {
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	_ = cmd.Run()
}

func killImage(image string) {
	cmd := exec.Command("taskkill", "/F", "/T", "/IM", image)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	_ = cmd.Run()
}

func portBusy(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitHealthy(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func waitInterrupt() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
}

func tailFile(path string, max int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(无日志)"
	}
	s := string(b)
	if len(s) > max {
		s = s[len(s)-max:]
	}
	return s
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func openInBrowser(url string) {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	_ = cmd.Start()
}

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

func msgBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0)
}

func fail(headless bool, msg string) int {
	fmt.Fprintln(os.Stderr, "[MultiAgentLab] "+msg)
	if !headless {
		msgBox(appName, msg)
	}
	return 1
}
