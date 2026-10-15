// MultiAgentLab Windows 桌面客户端壳与进程编排（REQ-321/M123）。
//
// 职责（对应 10 号跨端方案 R-X3/R-X11/SC-X1 的 Windows 单平台先行实现）：
//   - 编排三服务启动：本体两平面（ontologyd → runtimed）→ 主平台 backend，
//     端口经安装目录 launcher.json 可配置（端口被占用的共存出路），环境变量与 run-dev.sh 同构；
//   - 端口被占用时的分级处理：自身残留服务按 PID/exe 名回收 → 无法回收时透明诊断
//     （谁占的、PID、完整路径）+ 处置建议（停实例 / 检查 WSL2 / launcher.json 换端口）；
//   - 单实例：已有客户端窗口时聚焦唤起而非报错；
//   - 数据根：data.location（安装器写）→ %LOCALAPPDATA%\eino-lab → 安装目录 data\；
//   - Job Object（KILL_ON_JOB_CLOSE）关窗整树回收；PID 留痕 + -stop 兜底停服；
//   - WebView2 缺失回退默认浏览器。
//
// 诚实边界（v1）：端口固定单次运行内不变；SQLite WAL 停服为进程树终止；无系统托盘/自启/
// 自动更新（R-X9/X10 后续）；跨显示器拖动 DPI 动态重排未处理。
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

// appName 窗口标题与提示文案用名（单实例聚焦按此精确匹配窗口标题）。
const appName = "智能体构建平台"

// CREATE_NO_WINDOW：子进程为控制台程序，GUI 进程派生时不弹黑窗。
const createNoWindow = 0x08000000

type service struct {
	name string // exe 名（-stop 兜底按名回收用）
	exe  string
	env  [][2]string
	log  string
}

var (
	cfg        config
	backendURL string
)

func main() {
	stop := flag.Bool("stop", false, "停止本安装的三个平台服务并退出")
	headless := flag.Bool("headless", false, "无窗口编排：启动服务、等待就绪后打印 READY 并驻留（冒烟自动化）")
	flag.Parse()

	root := exeDir()
	cfg = loadConfig(root)
	backendURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.BackendPort)
	dataDir := dataRoot(root)
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

// dataRoot 数据根：安装器把所选数据根写入 <安装目录>\data.location（数据随安装目录走——
// C 盘紧张机器可整包装到其他盘）；无标记（旧版安装/手动部署）→ %LOCALAPPDATA%\eino-lab 兜底。
func dataRoot(root string) string {
	if b, err := os.ReadFile(filepath.Join(root, "data.location")); err == nil {
		if p := strings.TrimSpace(string(b)); p != "" {
			return p
		}
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		return filepath.Join(localAppData, "eino-lab")
	}
	return filepath.Join(root, "data")
}

// run 常规入口：单实例聚焦 → 清理残留 → 拉起三服务 → 等主平台就绪 → 窗口/驻留 → 退出回收。
func run(root, dataDir, logDir string, headless bool) int {
	// 单实例语义：已有客户端窗口时聚焦唤起并退出，而非双实例撞端口（headless 运维模式不适用）
	if !headless && focusExistingWindow() {
		return 0
	}

	for _, d := range []string{dataDir, logDir,
		filepath.Join(dataDir, "projects"), filepath.Join(dataDir, "engines"),
		filepath.Join(dataDir, "engine_logs"), filepath.Join(dataDir, "bin")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fail(headless, "无法创建数据目录 "+d+": "+err.Error())
		}
	}

	if msg := cleanupStale(dataDir, headless); msg != "" {
		return fail(headless, msg)
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
	if !waitHealthy(backendURL+"/healthz", 20*time.Second) {
		tail := tailFile(filepath.Join(logDir, "backend.log"), 800)
		return fail(headless, "主平台未在 20 秒内就绪（"+backendURL+"）。backend.log 末尾：\n"+tail)
	}

	if headless {
		fmt.Printf("READY %s shell=%s data=%s\n", backendURL, ShellVersion, dataDir)
		waitInterrupt()
		return 0
	}

	return runShell(backendURL)
}

// buildServices 三服务环境面——与 run-dev.sh / 16 号部署文档 §7 一一同构；端口来自
// launcher.json（可配），路径全部钉绝对（正斜杠形态）。
func buildServices(root, dataDir string) []service {
	p := func(elem ...string) string { return filepath.ToSlash(filepath.Join(elem...)) }
	ontURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.OntologyPort)
	rtURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.RuntimePort)
	oxigraph := p(root, "bin", "oxigraph.exe")

	svcs := []service{
		{
			name: "ontologyd.exe", exe: p(root, "bin", "ontologyd.exe"), log: "ontology-service.log",
			env: [][2]string{
				{"ADDR", ontURL[strings.LastIndex(ontURL, ":"):] /* :8091 */},
				{"DB_PATH", p(dataDir, "ontology.db")},
				{"MIGRATIONS_DIR", p(root, "migrations", "ontology-service")},
				{"SIDECAR_SCRIPT", p(root, "tools", "rdf-sidecar", "sidecar.py")},
				{"PLATFORM_URL", backendURL},
			},
		},
		{
			name: "runtimed.exe", exe: p(root, "bin", "runtimed.exe"), log: "runtime-manager.log",
			env: [][2]string{
				{"ADDR", rtURL[strings.LastIndex(rtURL, ":"):]},
				{"DB_PATH", p(dataDir, "runtime.db")},
				{"MIGRATIONS_DIR", p(root, "migrations", "runtime-manager")},
				{"BUILD_SVC_URL", ontURL},
				{"ENGINE_DATA_DIR", p(dataDir, "engines")},
				{"ENGINE_LOG_DIR", p(dataDir, "engine_logs")},
				{"ENGINE_INSTALL_DIR", p(dataDir, "bin")},
			},
		},
		{
			name: "backend.exe", exe: p(root, "bin", "backend.exe"), log: "backend.log",
			env: [][2]string{
				{"ADDR", backendURL[strings.LastIndex(backendURL, ":"):]},
				{"DB_PATH", p(dataDir, "platform.db")},
				{"SECRET_KEY_FILE", p(dataDir, ".secret")},
				{"FILES_ROOT", p(dataDir, "projects")},
				{"WEB_DIST", p(root, "web", "dist")},
				{"KB_VECTOR_BACKEND", "sqlite"}, // 单机零依赖默认（README 注明 qdrant 覆盖方式）
				{"DOCS_ROOT", p(root, "docs")},
				{"RESEARCH_ROOT", p(root, "research")},
				{"KNOWLEDGE_ROOT", p(root, "platform-knowledge")},
				{"BUILD_SVC_URL", ontURL},
				{"RUNTIME_MGR_URL", rtURL},
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
	// AssignProcessToJobObject 要求 SET_QUOTA|TERMINATE 权限（QUERY_LIMITED 不够）
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.AssignProcessToJobObject(job, h)
}

// cleanupStale 启动前分级处理端口占用。返回非空 = 无法自动解决的诊断报告。
// 层级：①PID 留痕回收上次残留 ②本平台服务 exe 名回收 ③仍占用 → 透明诊断
// （端口/PID/进程名/完整路径 + 三条处置建议：停开发实例、检查 WSL2 内实例、launcher.json 换端口）。
func cleanupStale(dataDir string, headless bool) string {
	ports := []int{cfg.BackendPort, cfg.OntologyPort, cfg.RuntimePort}

	if pids := readPIDFile(dataDir); len(pids) > 0 {
		for _, pid := range pids {
			killPID(pid)
		}
		removePIDFile(dataDir)
		time.Sleep(300 * time.Millisecond) // 端口释放窗口
	}

	busy := false
	for _, port := range ports {
		if portBusy(port) {
			busy = true
		}
	}
	if !busy {
		return ""
	}

	// 按镜像名回收本平台服务（含 go run 开发实例——其临时 exe 基名同为 backend.exe 等）
	for _, name := range []string{"backend.exe", "ontologyd.exe", "runtimed.exe", "MultiAgentLab.exe"} {
		killImage(name)
	}
	time.Sleep(800 * time.Millisecond) // 进程树退出 + 端口释放窗口

	// 仍占用 → 收集占用者，输出透明诊断
	var report strings.Builder
	for _, port := range ports {
		owners := findPortOwners(port)
		if len(owners) == 0 {
			continue
		}
		fmt.Fprintf(&report, "\n· 端口 %d 被占用：", port)
		for _, o := range owners {
			if o.Path != "" {
				fmt.Fprintf(&report, "\n    PID %d  %s  (%s)", o.PID, o.Name, o.Path)
			} else {
				fmt.Fprintf(&report, "\n    PID %d  %s", o.PID, o.Name)
			}
		}
	}
	if report.Len() == 0 {
		return fmt.Sprintf("端口 %d/%d/%d 被占用且无法定位占用进程，请重启系统后重试。",
			ports[0], ports[2], ports[1])
	}
	return "启动所需端口被其他程序占用，为避免误杀未自动处理：\n" + report.String() + `

处理建议（任选其一）：
  1) 若是手动启动的平台开发实例（run-dev.sh / go run / WSL 内实例），请先停止它再启动本客户端；
     WSL2 内的实例 Windows 端 taskkill 不可见，需在 WSL 内停止；
  2) 在本安装目录新建 launcher.json 可更换客户端端口后共存，例如：
     {"backend_port":18080, "ontology_port":18091, "runtime_port":18090}`
}

// stopServices 幂等停服：PID 留痕优先，exe 镜像名兜底（覆盖 launcher 已损坏/无留痕场景）。
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
	setProcessDpiAware() // 高缩放屏清晰渲染的前提（必须在建窗前声明；125% 屏上不声明则整窗位图拉伸模糊）
	dpi := systemDPI()
	ww, wh := scaleSize(1152, 720, dpi) // 逻辑 1152×720 @96 → 125% 屏 = 1440×900 物理
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  appName,
			Width:  uint(ww),
			Height: uint(wh),
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

// ---- 数据根 / PID 留痕 ----

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

var procMessageBoxW = user32.NewProc("MessageBoxW") // user32 声明在 dpi.go

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
