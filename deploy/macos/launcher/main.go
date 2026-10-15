// mlabd MultiAgentLab macOS 桌面客户端服务编排器（REQ-324/M127）。
//
// 与 deploy/windows/launcher（REQ-321/M123）同构的 macOS 对称实现，职责：
//   - 编排三服务启动：ontologyd :8091 → runtimed :8090 → backend :8080，
//     环境变量与 run-dev.sh / Windows launcher 一一同构（见 16_部署与运行.md §7）；
//   - 数据根独立于程序目录：~/Library/Application Support/multiagent-lab（MLAB_DATA_ROOT 可覆盖，冒烟隔离用）；
//   - 退出回收 = 进程组 SIGTERM→SIGKILL 阶梯（Windows 为 Job Object）；并监视父进程：
//     本进程由 .app 壳脚本派生，壳（osascript 窗口）退出后本进程 getppid() 归 1，
//     立即回收整棵服务树——等价 Windows「关窗即整树回收」；-headless 冒烟模式关闭该监视；
//   - platform.pids 留痕 + `-stop` 幂等停服（兜底回收手段）；
//   - macOS 不按进程名兜底杀进程（避免误伤手动启动的开发实例），仅回收自身 PID 留痕。
//
// 窗口壳本体 = shell/window.applescript（AppleScriptObjC + WKWebView，零编译器依赖——
// 构建/运行仅需 go+node，与 run-dev.sh 工具面一致；本项目正身机 Xcode CLT 损坏亦不受影响）。
//
// 诚实边界（v1）：单机单实例（端口固定 8080/8091/8090）；SQLite WAL 停服为进程树终止；
// 壳被强制杀死（kill -9）时依赖本进程 ppid 监视回收，无系统级 Job Object 强保真；
// 不支持开机自启/托盘/自动更新（沿 Windows 口径 P2）。
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
)

// ShellVersion 客户端壳版本（与 Info.plist CFBundleShortVersionString、build.sh DMG 文件名保持一致）。
const ShellVersion = "1.0.0"

// appName 窗口标题与提示文案用名（Dock/菜单栏显示名取自 Info.plist CFBundleName）。
const appName = "智能体构建平台"

const (
	backendURL = "http://127.0.0.1:8080"
	healthPath = "/healthz"
)

type service struct {
	name string // 二进制名（logs 分文件 + 诊断文案用）
	exe  string
	env  [][2]string
	log  string
}

func main() {
	stop := flag.Bool("stop", false, "停止本数据根的三个平台服务并退出")
	headless := flag.Bool("headless", false, "无窗口编排：启动服务、等待就绪后打印 READY 并驻留（冒烟自动化；关闭父进程监视）")
	flag.Parse()

	root := appRoot()
	dataDir := dataRoot()
	logDir := filepath.Join(dataDir, "logs")

	switch {
	case *stop:
		os.Exit(stopServices(dataDir))
	default:
		os.Exit(run(root, dataDir, logDir, *headless))
	}
}

// appRoot 资源根：bundle 内（MultiAgentLab.app/Contents/MacOS/mlabd 落 Resources/bin 时取
// Resources）；裸二进制（开发调试）取可执行文件所在目录。
func appRoot() string {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法定位可执行文件:", err)
		os.Exit(1)
	}
	dir := filepath.Dir(exe)
	if base := filepath.Base(dir); base == "bin" {
		parent := filepath.Dir(dir)
		if strings.HasSuffix(parent, "Contents/Resources") {
			return parent
		}
	}
	return dir
}

// dataRoot 数据根：~/Library/Application Support/multiagent-lab（对齐 Windows %LOCALAPPDATA%\multiagent-lab
// 定案：独立于程序目录，删除/升级程序不触碰数据）；MLAB_DATA_ROOT 供冒烟/多实例隔离覆盖。
// 曾用名一次性迁移：eino-lab → multiagent-lab（旧目录存在且新目录不存在时整体改名；失败沿用旧目录）。
func dataRoot() string {
	if v := os.Getenv("MLAB_DATA_ROOT"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return appRoot()
	}
	cur := filepath.Join(home, "Library", "Application Support", "multiagent-lab")
	legacy := filepath.Join(home, "Library", "Application Support", "eino-lab")
	if st, err := os.Stat(legacy); err == nil && st.IsDir() {
		if _, err := os.Stat(cur); err != nil {
			if err := os.Rename(legacy, cur); err == nil {
				return cur
			}
		}
	}
	return cur
}

// run 编排入口：建目录 → 回收残留 → 拉起三服务（进程组）→ 等主平台就绪 →
// 父进程监视驻留（或 headless 纯驻留）→ 退出时整树回收。
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

	pids := make([]int, 0, len(svcs))
	defer func() {
		terminateAll(pids)
		removePIDFile(dataDir)
	}()

	for _, svc := range svcs {
		pid, err := spawn(svc, root, logDir)
		if err != nil {
			return fail(headless, "启动 "+svc.name+" 失败: "+err.Error())
		}
		pids = append(pids, pid)
	}
	writePIDFile(dataDir, pids)

	// 本体两平面就绪不阻塞主平台（与 run-dev.sh 语义一致：失败仅留日志），
	// 但 backend 必须就绪——窗口壳要承载它。
	if !waitHealthy(backendURL+healthPath, 20*time.Second) {
		tail := tailFile(filepath.Join(logDir, "backend.log"), 800)
		return fail(headless, "主平台未在 20 秒内就绪（端口 8080）。backend.log 末尾：\n"+tail)
	}

	if headless {
		fmt.Printf("READY %s shell=%s data=%s\n", backendURL, ShellVersion, dataDir)
		_ = os.WriteFile("/tmp/mlab-launcher-last-error.txt", []byte(fmt.Sprintf("READY %s data=%s\n", backendURL, dataDir)), 0o666)
		waitInterrupt()
		return 0
	}

	// 父进程监视：壳脚本 exec 成 osascript 窗口后为本进程父；壳退出（正常 quit 或被杀）
	// 时 getppid() 变 1 → 立即回收整树（defer 兜底幂等）。
	watchParent()
	return 0
}

// buildServices 三服务环境面——与 run-dev.sh / Windows launcher / 16 号部署文档 §7 一一同构；
// 数据类路径落 dataDir，程序/资源类路径落 appRoot（bundle Resources）。
func buildServices(root, dataDir string) []service {
	svcs := []service{
		{
			name: "ontologyd", exe: filepath.Join(root, "bin", "ontologyd"), log: "ontology-service.log",
			env: [][2]string{
				{"ADDR", ":8091"},
				{"DB_PATH", filepath.Join(dataDir, "ontology.db")},
				{"MIGRATIONS_DIR", filepath.Join(root, "migrations", "ontology-service")},
				{"SIDECAR_SCRIPT", filepath.Join(root, "tools", "rdf-sidecar", "sidecar.py")},
				{"PLATFORM_URL", backendURL},
			},
		},
		{
			name: "runtimed", exe: filepath.Join(root, "bin", "runtimed"), log: "runtime-manager.log",
			env: [][2]string{
				{"ADDR", ":8090"},
				{"DB_PATH", filepath.Join(dataDir, "runtime.db")},
				{"MIGRATIONS_DIR", filepath.Join(root, "migrations", "runtime-manager")},
				{"BUILD_SVC_URL", "http://127.0.0.1:8091"},
				{"ENGINE_DATA_DIR", filepath.Join(dataDir, "engines")},
				{"ENGINE_LOG_DIR", filepath.Join(dataDir, "engine_logs")},
				{"ENGINE_INSTALL_DIR", filepath.Join(dataDir, "bin")},
			},
		},
		{
			name: "backend", exe: filepath.Join(root, "bin", "backend"), log: "backend.log",
			env: [][2]string{
				{"ADDR", ":8080"},
				{"DB_PATH", filepath.Join(dataDir, "platform.db")},
				{"SECRET_KEY_FILE", filepath.Join(dataDir, ".secret")},
				{"FILES_ROOT", filepath.Join(dataDir, "projects")},
				{"WEB_DIST", filepath.Join(root, "web", "dist")},
				{"KB_VECTOR_BACKEND", "sqlite"}, // 单机零依赖默认（README 注明 qdrant 覆盖方式）
				{"DOCS_ROOT", filepath.Join(root, "docs")},
				{"RESEARCH_ROOT", filepath.Join(root, "research")},
				{"KNOWLEDGE_ROOT", filepath.Join(root, "platform-knowledge")},
			},
		},
	}
	// 内置 oxigraph 种子（REQ-321 零捆绑口径 2026-10-15 反转：安装包内置引擎）：随包副本
	// （与 mlabd 同目录，build.sh 落 Resources/bin）首启复制到 dataDir/bin（一键安装同位）；
	// 已存在不覆盖（不回滚用户一键更新或手动放置的版本）。
	if exe, err := os.Executable(); err == nil {
		src := filepath.Join(filepath.Dir(exe), "oxigraph")
		dst := filepath.Join(dataDir, "bin", "oxigraph")
		if fileExists(src) && !fileExists(dst) {
			if b, err := os.ReadFile(src); err == nil {
				_ = os.WriteFile(dst, b, 0o755)
			}
		}
	}
	// REQ-146/307：oxigraph/fuseki 不随包分发（首启运行页一键安装落 dataDir/bin），
	// 此处存在即显式注入 env（即时命中，无需重启编排器）。
	if p := filepath.Join(dataDir, "bin", "oxigraph"); fileExists(p) {
		svcs[1].env = append(svcs[1].env, [2]string{"OXIGRAPH_BIN", p})
	}
	if p := filepath.Join(dataDir, "bin", "fuseki-server"); fileExists(p) {
		svcs[1].env = append(svcs[1].env, [2]string{"FUSEKI_BIN", p})
	}
	// REQ-286/run-dev.sh 同款 sidecar python 探测（macOS xcrun shim 损坏场景下
	// /usr/bin/python3 不可用，须探测含 rdflib 的解释器；已设 SIDECAR_PYTHON 则尊重）。
	if os.Getenv("SIDECAR_PYTHON") == "" {
		if py := probeSidecarPython(); py != "" {
			svcs[0].env = append(svcs[0].env, [2]string{"SIDECAR_PYTHON", py})
		}
	}
	return svcs
}

func probeSidecarPython() string {
	for _, py := range []string{"python3", "/usr/local/bin/python3.12", "/usr/local/bin/python3",
		"/opt/homebrew/bin/python3", "/usr/local/bin/python3.10"} {
		if _, err := exec.LookPath(py); err != nil {
			continue
		}
		cmd := exec.Command(py, "-c", "import rdflib")
		if err := cmd.Run(); err == nil {
			if abs, err := filepath.Abs(py); err == nil {
				return abs
			}
			return py
		}
	}
	return ""
}

func spawn(svc service, root, logDir string) (int, error) {
	f, err := os.OpenFile(filepath.Join(logDir, svc.log), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
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
	// Setpgid：子进程自成进程组（pgid=pid），整树回收以 -pgid 信号投递；
	// 引擎子进程（oxigraph serve 等）继承同组，随组回收。
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		f.Close()
		return 0, err
	}
	// 生命周期归进程组/日志文件，进程句柄即刻释放。
	go func() { cmd.Wait(); f.Close() }()
	return cmd.Process.Pid, nil
}

// terminateAll 进程组回收阶梯：SIGTERM（优雅，inference quitSignal 消费）→ 2s 后对残存组 SIGKILL。
// SQLite WAL 下硬终止崩溃安全。
func terminateAll(pids []int) {
	for _, pid := range pids {
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
	time.Sleep(2 * time.Second)
	for _, pid := range pids {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

// watchParent 壳退出监视：getppid() 归 1（osascript 壳终止、本进程被 launchd 收养）即整树回收退出。
func watchParent() {
	for {
		time.Sleep(2 * time.Second)
		if os.Getppid() == 1 {
			return
		}
	}
}

// cleanupStale 启动前回收：上次壳异常退出遗留的服务进程（仅按本数据根 PID 留痕）。
// macOS 端不做进程名兜底杀（backend/ontologyd 与开发实例同名，避免误伤 run-dev.sh 栈）。
func cleanupStale(dataDir string, headless bool) int {
	if pids := readPIDFile(dataDir); len(pids) > 0 {
		for _, pid := range pids {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
		removePIDFile(dataDir)
		time.Sleep(300 * time.Millisecond) // 端口释放窗口
	}
	if portBusy(8080) || portBusy(8090) || portBusy(8091) {
		return fail(headless, "端口 8080/8090/8091 被占用（可能为手动启动的开发实例，run-dev.sh）。\n"+
			"客户端不代收其他实例的进程——请先停止占用端口的服务后重试。")
	}
	return 0
}

func stopServices(dataDir string) int {
	pids := readPIDFile(dataDir)
	if len(pids) == 0 {
		fmt.Println("无 PID 留痕（可能未由本客户端启动）。macOS 端不按进程名兜底回收（避免误伤开发实例）——如需停止开发栈请用其启动脚本。")
		return 0
	}
	for _, pid := range pids {
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
	time.Sleep(2 * time.Second)
	for _, pid := range pids {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
	removePIDFile(dataDir)
	fmt.Printf("已停止 %d 个平台服务进程组。\n", len(pids))
	return 0
}

// ---- PID 留痕 / 探活 / 小工具 ----

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

func fail(headless bool, msg string) int {
	fmt.Fprintln(os.Stderr, "[MultiAgentLab] "+msg)
	// 状态文件：窗口壳（window.applescript）就绪超时弹窗时读取示因（固定路径，个人机单实例语义）。
	_ = os.WriteFile("/tmp/mlab-launcher-last-error.txt", []byte("[MultiAgentLab] "+msg), 0o666)
	if !headless {
		dialog(appName, msg)
	}
	return 1
}

// dialog 错误弹窗（best-effort：osascript display dialog；headless/无 GUI 时静默）。
func dialog(title, text string) {
	script := fmt.Sprintf(`display dialog %s with title %s buttons {"好"} default button 1`,
		strconv.Quote(text), strconv.Quote(title))
	cmd := exec.Command("osascript", "-e", script)
	_ = cmd.Run()
}
