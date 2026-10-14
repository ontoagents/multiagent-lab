// MultiAgentLab Windows 安装器（REQ-321/M123）。
//
// 结构：setup.exe = 本程序 + 尾部附加 zip（build.ps1 拼接）。
// 双形态：
//   - GUI（默认双击）：WebView2 现代向导——安装位置可改、磁盘空间预检、解压直达安装目录
//     （无 %TEMP% 中转，目标盘瞬时占用减半）、进度条、完成后可一键启动平台；
//   - Console（-y 或 MLAB_SILENT=1，冒烟自动化）：同逻辑，进度走 stdout，无窗口。
//
// 数据根：安装器把所选安装目录下的 data\ 写入 <安装目录>\data.location，launcher 启动时读取
// （数据随安装目录走——C 盘紧张机器可选装其他盘；未写则 launcher 兜底 %LOCALAPPDATA%\eino-lab）。
package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

const (
	defaultDir     = `Programs\MultiAgentLab` // 相对 %LOCALAPPDATA%
	needBytesSlack = 80 << 20                 // 解压后预留量（zip 解压后 ~95MB + 余量）
)

type uiState struct {
	w      webview2.WebView
	appDir string // 安装目录（GUI 由用户输入，console 由默认/参数）
	self   []byte
	zr     *zip.Reader
	// 进度推送统一走 dispatch（WebView2 UI 线程）
}

func main() {
	silent := len(os.Args) > 1 && (os.Args[1] == "-y" || os.Args[1] == "-silent") ||
		os.Getenv("MLAB_SILENT") == "1"
	self, err := os.ReadFile(mustExe())
	if err != nil {
		failSilent("无法读取自身文件: " + err.Error())
	}
	zr, err := appendedZip(self)
	if err != nil {
		failSilent("安装包不完整或已损坏（附加载荷缺失）: " + err.Error())
	}

	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = `C:`
	}
	defDir := filepath.Join(localAppData, defaultDir)
	if env := os.Getenv("MLAB_DIR"); env != "" {
		defDir = env
	}

	if silent {
		st := &uiState{appDir: defDir, self: self, zr: zr}
		if msg := st.install(func(pct int, step string) { fmt.Printf("[%3d%%] %s\n", pct, step) }); msg != "" {
			fmt.Fprintln(os.Stderr, "[setup] "+msg)
			os.Exit(1)
		}
		fmt.Println("安装完成。开始菜单/桌面「智能体构建平台」启动。")
		return
	}
	runGUI(defDir, self, zr)
}

// ---- 安装核心（GUI/console 共用） ----

// install 执行完整安装；返回人话错误（空串=成功）。progress 回调可能来自任意 goroutine，
// 由调用方保证线程安全（console=直接打印；GUI=包一层 Dispatch）。
func (st *uiState) install(progress func(pct int, step string)) (errMsg string) {
	step := func(pct int, s string) {
		progress(pct, s)
	}

	// ① 磁盘空间预检（目标盘：解压后体量 + 预留）
	var need int64
	for _, f := range st.zr.File {
		need += int64(f.UncompressedSize64)
	}
	need += needBytesSlack
	if msg := checkDiskSpace(st.appDir, need); msg != "" {
		return msg
	}
	step(2, "磁盘空间检查通过")

	// ② 解压直达安装目录（含防穿越/目录条目归一——Compress-Archive 反斜杠条目见 probe 取证）。
	// zip 内程序文件带 payload\app\ 前缀（原 7zSD 流程由 installer.cmd robocopy 摊平，
	// 直解形态在此剥离；payload 外的根级文件〔uninstall/installer 脚本、README〕原位落 root）。
	if err := os.MkdirAll(st.appDir, 0o755); err != nil {
		return "无法创建安装目录 " + st.appDir + ": " + err.Error()
	}
	total := len(st.zr.File)
	for i, f := range st.zr.File {
		rel := strings.ReplaceAll(f.Name, "\\", "/")
		rel = strings.TrimPrefix(rel, "payload/app/")
		if strings.Contains(rel, "../") || strings.HasPrefix(rel, "/") {
			return "解压 " + f.Name + " 失败: 非法路径"
		}
		if err := extractEntry(f, st.appDir, rel); err != nil {
			return "解压 " + f.Name + " 失败: " + err.Error()
		}
		if i%24 == 0 || i == total-1 {
			step(2+80*(i+1)/total, fmt.Sprintf("释放程序文件 %d/%d", i+1, total))
		}
	}

	// ③ 数据根标记（launcher 启动时读取；数据随安装目录走）
	if err := os.WriteFile(filepath.Join(st.appDir, "data.location"),
		[]byte(filepath.Join(st.appDir, "data")), 0o644); err != nil {
		return "写入数据位置标记失败: " + err.Error()
	}

	// ④ 快捷方式 + 卸载注册项（payload 内的 install-shortcuts.ps1；隐藏窗口）
	step(88, "创建快捷方式与卸载项")
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(st.appDir, "install-shortcuts.ps1"), "-Install")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return "快捷方式/注册项创建失败: " + err.Error() + " " + strings.TrimSpace(string(out))
	}

	step(100, "安装完成")
	return ""
}

func checkDiskSpace(dir string, need int64) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	root := filepath.VolumeName(abs)
	if root == "" {
		root = `C:`
	}
	var free, total uint64
	p, _ := syscall.UTF16PtrFromString(root)
	err = windows.GetDiskFreeSpaceEx(p, nil, &total, &free)
	if err != nil {
		return "" // 探测失败不阻塞（写入时报错兜底）
	}
	if uint64(need) > free {
		return fmt.Sprintf("磁盘空间不足：%s 盘剩余 %.0f MB，安装需要约 %.0f MB。请更换安装目录或清理磁盘后重试。",
			strings.TrimSuffix(root, "\\"), float64(free)/1e6, float64(need)/1e6)
	}
	return ""
}

// ---- GUI 向导 ----

func runGUI(defDir string, self []byte, zr *zip.Reader) {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "智能体构建平台 安装向导",
			Width:  620,
			Height: 480,
			Center: true,
		},
	})
	if w == nil {
		// WebView2 缺失：回退 console 流程 + 提示
		msgBox("WebView2 运行时缺失", "未检测到 WebView2 运行时，将以简洁模式继续安装。")
		st := &uiState{appDir: defDir, self: self, zr: zr}
		if msg := st.install(func(pct int, step string) { fmt.Printf("[%3d%%] %s\n", pct, step) }); msg != "" {
			failSilent(msg)
		}
		return
	}
	st := &uiState{w: w, appDir: defDir, self: self, zr: zr}

	dirJSON, _ := json.Marshal(defDir)
	need := int64(0)
	for _, f := range zr.File {
		need += int64(f.UncompressedSize64)
	}
	w.Bind("goStartInstall", func(dir string) string {
		go func() {
			st.appDir = dir
			msg := st.install(func(pct int, stepText string) {
				w.Dispatch(func() {
					w.Eval(fmt.Sprintf("uiProgress(%d,%q)", pct, stepText))
				})
			})
			w.Dispatch(func() {
				w.Eval(fmt.Sprintf("uiDone(%q)", msg))
			})
		}()
		return "started"
	})
	w.Bind("goLaunch", func() string {
		exe := filepath.Join(st.appDir, "MultiAgentLab.exe")
		c := exec.Command(exe)
		c.Dir = st.appDir
		c.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
		_ = c.Start()
		// 启动器接管服务编排与窗口，安装器功成身退
		w.Dispatch(func() { w.Terminate() })
		return "ok"
	})
	w.Bind("goClose", func() string {
		w.Dispatch(func() { w.Terminate() })
		return "ok"
	})

	w.Init(fmt.Sprintf(`document.addEventListener('DOMContentLoaded',function(){uiInit(%s,%d)});`, dirJSON, need/1e6))
	// 接口未暴露 NavigateToString：以 data: URL 承载向导页（Bind 的 RPC 注入走 Init 同文档生效）
	w.Navigate("data:text/html;charset=utf-8," + url.QueryEscape(pageHTML))
	w.Run()
}

const pageHTML = `<!doctype html><html><head><meta charset="utf-8"><style>
*{margin:0;padding:0;box-sizing:border-box;font-family:"Segoe UI","Microsoft YaHei",sans-serif}
body{height:100vh;display:flex;flex-direction:column;background:#f6f7fb;color:#1f2937}
.hero{background:linear-gradient(135deg,#4f46e5,#7c3aed);color:#fff;padding:28px 32px}
.hero h1{font-size:22px;font-weight:600;letter-spacing:.5px}
.hero p{opacity:.85;font-size:13px;margin-top:6px}
.body{flex:1;padding:24px 32px;display:flex;flex-direction:column;gap:14px}
label{font-size:13px;color:#4b5563;font-weight:600}
.dirrow{display:flex;gap:8px}
input{flex:1;padding:9px 12px;border:1px solid #d1d5db;border-radius:8px;font-size:13px;outline:none}
input:focus{border-color:#6366f1}
.hint{font-size:12px;color:#6b7280}
#btn{align-self:flex-start;background:#4f46e5;color:#fff;border:0;border-radius:8px;
padding:11px 34px;font-size:14px;font-weight:600;cursor:pointer}
#btn:disabled{background:#c7d2fe;cursor:default}
#prog{display:none}#bar{height:8px;background:#e5e7eb;border-radius:6px;overflow:hidden}
#fill{height:100%;width:0;background:linear-gradient(90deg,#4f46e5,#7c3aed);transition:width .2s}
#step{font-size:12px;color:#4b5563;margin-top:6px}
#done{display:none;background:#ecfdf5;border:1px solid #a7f3d0;border-radius:10px;padding:16px}
#done h3{color:#047857;font-size:15px}#done p{font-size:12px;color:#4b5563;margin:8px 0 12px}
#done button{margin-right:10px;padding:8px 18px;border:0;border-radius:8px;font-size:13px;
font-weight:600;cursor:pointer}
#launch{background:#059669;color:#fff}#fin{background:#e5e7eb;color:#374151}
.err{background:#fef2f2;border:1px solid #fecaca;color:#b91c1c;border-radius:8px;padding:10px 12px;font-size:12px;display:none}
.foot{padding:12px 32px;font-size:11px;color:#9ca3af;border-top:1px solid #e5e7eb}
</style></head><body>
<div class="hero"><h1>智能体构建平台</h1><p>智能体工程 × 本体工程 · 本地单机版（当前用户安装，无需管理员）</p></div>
<div class="body">
  <div id="form">
    <label>安装位置</label>
    <div class="dirrow"><input id="dir" spellcheck="false"></div>
    <p class="hint" id="space"></p>
    <div style="height:10px"></div>
    <button id="btn" onclick="go()">立即安装</button>
  </div>
  <div id="prog"><div id="bar"><div id="fill"></div></div><p id="step">准备中…</p></div>
  <div id="err" class="err"></div>
  <div id="done"><h3>✓ 安装完成</h3><p>已创建开始菜单与桌面快捷方式。数据目录位于安装目录内，卸载时自动保留。</p>
    <button id="launch" onclick="goLaunch()">立即启动平台</button><button id="fin" onclick="goClose()">完成</button></div>
</div>
<div class="foot">Go + WebView2 · oxigraph 等原生依赖在首次使用对应功能时引导下载 · 数据目录卸载时保留</div>
<script>
function uiInit(dir,needMB){document.getElementById('dir').value=dir;
document.getElementById('space').textContent='安装体积约 '+needMB+' MB（下载内容为当前盘瞬时占用，不影响其他盘）';}
function uiProgress(pct,step){document.getElementById('prog').style.display='block';
document.getElementById('form').style.display='none';document.getElementById('err').style.display='none';
document.getElementById('fill').style.width=pct+'%';document.getElementById('step').textContent=step;}
function uiDone(err){document.getElementById('prog').style.display='none';
if(err){var e=document.getElementById('err');e.style.display='block';e.textContent=err;
document.getElementById('form').style.display='block';document.getElementById('btn').disabled=false;return;}
document.getElementById('form').style.display='none';document.getElementById('done').style.display='block';}
function go(){document.getElementById('btn').disabled=true;goStartInstall(document.getElementById('dir').value);}
</script></body></html>`

// ---- 附加 zip 定位 / 解压 / 小工具 ----

// appendedZip 从自身字节尾部还原附加 zip：定位 EOCD（PK\x05\x06，无注释形态），
// 反推 archive 起点 = fileLen - 22 - cdSize - cdOffset。
func appendedZip(self []byte) (*zip.Reader, error) {
	const eocdSig = "PK\x05\x06"
	if len(self) < 22 {
		return nil, fmt.Errorf("文件过小")
	}
	eocd := -1
	floor := len(self) - 65536
	if floor < 0 {
		floor = 0
	}
	for i := len(self) - 22; i >= floor; i-- {
		if bytes.Equal(self[i:i+4], []byte(eocdSig)) {
			eocd = i
			break
		}
	}
	if eocd < 0 {
		return nil, fmt.Errorf("未找到 EOCD")
	}
	cdSize := binary.LittleEndian.Uint32(self[eocd+12 : eocd+16])
	cdOffset := binary.LittleEndian.Uint32(self[eocd+16 : eocd+20])
	start := len(self) - 22 - int(cdSize) - int(cdOffset)
	if start < 0 {
		return nil, fmt.Errorf("附加段越界")
	}
	return zip.NewReader(bytes.NewReader(self[start:]), int64(len(self)-start))
}

func extractEntry(f *zip.File, dir, rel string) error {
	// Compress-Archive（PS5.1）以反斜杠存条目名且目录条目不带 zip 规范的 / 结尾——
	// Go zip 据此误判为文件，会先创建出「payload\app」零字节文件使后续 mkdir 全线撞墙。
	// isDir 必须在 Clean 之前判定（Clean 会剥掉尾斜杠，反斜杠目录条目此时已无法识别）。
	isDir := strings.HasSuffix(rel, "/") || f.FileInfo().IsDir()
	name := filepath.Clean(rel)
	target := filepath.Join(dir, name)
	if isDir {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func mustExe() string {
	exe, err := os.Executable()
	if err != nil {
		failSilent("无法定位自身: " + err.Error())
	}
	return exe
}

func failSilent(msg string) {
	fmt.Fprintln(os.Stderr, "[setup] "+msg)
	msgBox("智能体构建平台 安装", msg)
	os.Exit(1)
}

func msgBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	proc.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0)
}
