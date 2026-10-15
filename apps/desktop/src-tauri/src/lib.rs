// MultiAgentLab 桌面客户端（Tauri 2 壳，REQ-321/M123 壳技术切换交付；macOS 侧切换 2026-10-15）。
//
// 职责（对齐 10 号 §8 apps/desktop 与 18 号 REQ-321 定案）：
//   - 窗口系统 WebView（Win=WebView2 / macOS=WKWebView）直载 http://127.0.0.1:8080（backend 托管 SPA，免 CORS）；
//   - 三服务编排：ontologyd → runtimed → backend（env 与 run-dev.sh 同构；launcher.json 端口可配；
//     数据根机制与 Go launcher 双向兼容：Windows=data.location→%LOCALAPPDATA%\eino-lab；
//     macOS=data.location→MLAB_DATA_ROOT→~/Library/Application Support/eino-lab）；
//   - 托盘：打开主界面/服务状态/打开数据目录/退出；关窗收托盘；tooltip 实时服务状态；
//   - 单实例（tauri-plugin-single-instance）；退出整树回收（R-X11）：
//     Windows=Job Object（KILL_ON_JOB_CLOSE，随进程退出生效）；macOS=Setpgid 进程组 + SIGTERM→SIGKILL
//     阶梯（退出钩子）+ sh 兽护子进程（壳被 kill -9 后 ≤2s 轮询感知整树回收——Job Object 等价）；
//   - backend 就绪前窗口隐藏，就绪后展示（60s 兜底展示错误页）；
//   - **端口预检（共享路径，双平台）**：8080/8090/8091 任一被占即不拉服务、窗口落本地占位页并明示
//     占用态——防止把仍在运行的 run-dev 等他方实例渲染进本壳窗口（数据串台缺陷，macOS 切换轮顺修）。
//
// 诚实边界：macOS 强杀回收系 sh 兽护 2s 轮询（非系统级 Job Object 强保真）；--stop 无进程名兜底
// （防误伤开发实例）；launcher.json 换端口双平台同口径。
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

#[cfg(windows)]
use std::os::windows::process::CommandExt;
#[cfg(not(windows))]
use std::os::unix::process::CommandExt;

use std::io::{Read, Write};
use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::Duration;

use tauri::{
    menu::{Menu, MenuItem, PredefinedMenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Emitter, Manager,
};

const APP_TITLE: &str = "智能体构建平台";
#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

#[derive(Clone)]
struct Ports {
    backend: u16,
    ontology: u16,
    runtime: u16,
}

impl Ports {
    fn load(exe_dir: &std::path::Path) -> Self {
        let mut p = Ports { backend: 8080, ontology: 8091, runtime: 8090 };
        if let Ok(b) = std::fs::read_to_string(exe_dir.join("launcher.json")) {
            if let Ok(v) = serde_json::from_str::<serde_json::Value>(&b) {
                if let Some(n) = v["backend_port"].as_u64() { p.backend = n as u16; }
                if let Some(n) = v["ontology_port"].as_u64() { p.ontology = n as u16; }
                if let Some(n) = v["runtime_port"].as_u64() { p.runtime = n as u16; }
            }
        }
        p
    }
    fn backend_url(&self) -> String { format!("http://127.0.0.1:{}", self.backend) }
    fn ontology_url(&self) -> String { format!("http://127.0.0.1:{}", self.ontology) }
    fn runtime_url(&self) -> String { format!("http://127.0.0.1:{}", self.runtime) }
}

// ---- 数据根（与 Go launcher 双向兼容） ----

fn data_root(exe_dir: &std::path::Path) -> PathBuf {
    if let Ok(b) = std::fs::read_to_string(exe_dir.join("data.location")) {
        let p = b.trim();
        if !p.is_empty() {
            return PathBuf::from(p);
        }
    }
    // 数据根更名（0524fef 曾用名清理）：multiagent-lab；旧 eino-lab 存在且新目录不存在时一次性整体改名
    //（改名失败——如目录被占用——沿用旧目录不丢数据；data.location 机制不变）
    #[cfg(windows)]
    {
        if let Some(base) = std::env::var_os("LOCALAPPDATA") {
            return migrate_legacy_dir(PathBuf::from(base));
        }
    }
    #[cfg(not(windows))]
    {
        // 与 deploy/macos mlabd 同口径：MLAB_DATA_ROOT 冒烟隔离覆盖 → 用户数据根
        if let Some(root) = std::env::var_os("MLAB_DATA_ROOT") {
            if !root.is_empty() {
                return PathBuf::from(root);
            }
        }
        if let Some(home) = std::env::var_os("HOME") {
            return migrate_legacy_dir(
                PathBuf::from(home).join("Library").join("Application Support"),
            );
        }
    }
    exe_dir.join("data")
}

// 曾用名一次性迁移：eino-lab → multiagent-lab（旧目录存在且新目录不存在时整体改名；
// 改名失败——如目录被占用——沿用旧目录不丢数据）
fn migrate_legacy_dir(base: PathBuf) -> PathBuf {
    let cur = base.join("multiagent-lab");
    let legacy = base.join("eino-lab");
    if legacy.is_dir() && !cur.is_dir() {
        if std::fs::rename(&legacy, &cur).is_err() {
            return legacy;
        }
    }
    cur
}

// ---- 资源路径：兼容 resources 子目录与安装根两种落位 ----

fn resolve_resource(app: &tauri::AppHandle, exe_dir: &std::path::Path, rel: &str) -> Option<PathBuf> {
    if let Ok(rd) = app.path().resource_dir() {
        let a = rd.join("resources").join(rel);
        if a.exists() {
            return Some(a);
        }
        let b = rd.join(rel);
        if b.exists() {
            return Some(b);
        }
    }
    let c = exe_dir.join(rel);
    if c.exists() {
        return Some(c);
    }
    None
}

// ---- 平台差异小面：二进制名 / 无窗口派生（Windows）/ 日志重定向（macOS） ----

fn svc_path(exe_dir: &std::path::Path, name: &str) -> PathBuf {
    #[cfg(windows)]
    {
        let _ = name;
        exe_dir.join(format!("{}.exe", name))
    }
    #[cfg(not(windows))]
    {
        // bundled .app：externalBin 与主程序同落 Contents/MacOS；钉绝对路径（Finder 启动 CWD=/ 不可依赖 PATH）
        exe_dir.join(name)
    }
}

fn set_hidden(cmd: &mut Command, log: &std::path::Path) {
    #[cfg(windows)]
    {
        let _ = log;
        cmd.creation_flags(CREATE_NO_WINDOW);
        cmd.stdout(Stdio::null());
        cmd.stderr(Stdio::null());
    }
    #[cfg(not(windows))]
    {
        let f = std::fs::OpenOptions::new().create(true).append(true).open(log).ok();
        cmd.stdout(f.as_ref().map_or(Stdio::null(), |f| Stdio::from(f.try_clone().expect("log fd"))));
        cmd.stderr(f.map_or(Stdio::null(), Stdio::from));
    }
}

// ---- 服务编排 ----

fn spawn_services(
    app: &tauri::AppHandle,
    exe_dir: &std::path::Path,
    data_dir: &std::path::Path,
    ports: &Ports,
    kids: &mut Vec<Child>,
) -> Result<(), String> {
    let abs = |base: &std::path::Path, elem: &[&str]| -> String {
        let mut p = base.to_path_buf();
        for e in elem {
            p = p.join(e);
        }
        p.to_string_lossy().replace('\\', "/")
    };
    let res = |rel: &str| -> String {
        match resolve_resource(app, exe_dir, rel) {
            Some(p) => p.to_string_lossy().replace('\\', "/"),
            None => abs(exe_dir, &[rel]),
        }
    };
    let log = |name: &str| data_dir.join("logs").join(name);

    let mut ont = Command::new(svc_path(exe_dir, "ontologyd"));
    ont.env("ADDR", format!(":{}", ports.ontology))
        .env("DB_PATH", abs(data_dir, &["ontology.db"]))
        .env("MIGRATIONS_DIR", res("migrations/ontology-service"))
        .env("SIDECAR_SCRIPT", res("tools/rdf-sidecar/sidecar.py"))
        .env("PLATFORM_URL", ports.backend_url());
    #[cfg(not(windows))]
    if let Some(py) = probe_sidecar_python() {
        // run-dev.sh/mlabd 同款：含 rdflib 的解释器显式透传（xcrun shim 损坏机自动绕开）
        ont.env("SIDECAR_PYTHON", py);
    }
    set_hidden(&mut ont, &log("ontology-service.log"));

    let mut rt = Command::new(svc_path(exe_dir, "runtimed"));
    rt.env("ADDR", format!(":{}", ports.runtime))
        .env("DB_PATH", abs(data_dir, &["runtime.db"]))
        .env("MIGRATIONS_DIR", res("migrations/runtime-manager"))
        .env("BUILD_SVC_URL", ports.ontology_url())
        .env("ENGINE_DATA_DIR", abs(data_dir, &["engines"]))
        .env("ENGINE_LOG_DIR", abs(data_dir, &["engine_logs"]))
        .env("ENGINE_INSTALL_DIR", abs(data_dir, &["bin"]));
    // oxigraph 路径：一键安装落 data/bin（REQ-321 零捆绑，首启一键安装）
    let oxi_name = if cfg!(windows) { "oxigraph.exe" } else { "oxigraph" };
    let oxi = data_dir.join("bin").join(oxi_name);
    if oxi.exists() {
        rt.env("OXIGRAPH_BIN", oxi.to_string_lossy().replace('\\', "/"));
    }
    set_hidden(&mut rt, &log("runtime-manager.log"));

    let mut be = Command::new(svc_path(exe_dir, "backend"));
    be.env("ADDR", format!(":{}", ports.backend))
        .env("DB_PATH", abs(data_dir, &["platform.db"]))
        .env("SECRET_KEY_FILE", abs(data_dir, &[".secret"]))
        .env("FILES_ROOT", abs(data_dir, &["projects"]))
        .env("WEB_DIST", res("web-dist"))
        .env("KB_VECTOR_BACKEND", "sqlite")
        .env("DOCS_ROOT", res("docs"))
        .env("RESEARCH_ROOT", res("research"))
        .env("KNOWLEDGE_ROOT", res("platform-knowledge"))
        .env("BUILD_SVC_URL", ports.ontology_url())
        .env("RUNTIME_MGR_URL", ports.runtime_url());
    set_hidden(&mut be, &log("backend.log"));

    let order: [(&str, &mut Command); 3] = [
        ("ontologyd", &mut ont),
        ("runtimed", &mut rt),
        ("backend", &mut be),
    ];
    let mut started = Vec::new();
    for (name, cmd) in order {
        // macOS：子进程自成进程组（pgid=pid），整树回收按 -pgid 投递（Windows 为 Job Object）
        #[cfg(not(windows))]
        cmd.process_group(0);
        match cmd.spawn() {
            Ok(child) => {
                SERVICE_PIDS.lock().unwrap().push(child.id());
                kids.push(child);
                started.push(name.to_string());
            }
            Err(e) => {
                return Err(format!(
                    "启动 {} 失败: {}（本轮已启动: {}）",
                    name,
                    e,
                    started.join(", ")
                ))
            }
        }
    }
    // pid 留痕（与 Go launcher 口径一致）：--stop 兜底回收的数据源
    let pf = format!(
        "{}\n",
        SERVICE_PIDS
            .lock()
            .unwrap()
            .iter()
            .map(|p| p.to_string())
            .collect::<Vec<_>>()
            .join("\n")
    );
    let _ = std::fs::write(data_dir.join("platform.pids"), pf);
    Ok(())
}

#[cfg(not(windows))]
fn probe_sidecar_python() -> Option<String> {
    let candidates = [
        "python3",
        "/usr/local/bin/python3.12",
        "/usr/local/bin/python3",
        "/opt/homebrew/bin/python3",
        "/usr/local/bin/python3.10",
    ];
    for py in candidates {
        let ok = Command::new(py)
            .arg("-c")
            .arg("import rdflib")
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()
            .map(|s| s.success())
            .unwrap_or(false);
        if ok {
            let resolved = Command::new("which")
                .arg(py)
                .output()
                .ok()
                .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
                .filter(|s| !s.is_empty())
                .unwrap_or_else(|| py.to_string());
            return Some(resolved);
        }
    }
    None
}

fn port_open(port: u16) -> bool {
    std::net::TcpStream::connect_timeout(
        &format!("127.0.0.1:{}", port).parse().unwrap(),
        Duration::from_millis(400),
    )
    .is_ok()
}

fn http_status_body(port: u16) -> Option<String> {
    let mut stream = std::net::TcpStream::connect(("127.0.0.1", port)).ok()?;
    stream.set_read_timeout(Some(Duration::from_secs(2))).ok()?;
    stream
        .write_all(format!("GET /healthz HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\n\r\n").as_bytes())
        .ok()?;
    let mut buf = String::new();
    stream.read_to_string(&mut buf).ok()?;
    Some(buf)
}

fn backend_healthy(ports: &Ports) -> bool {
    http_status_body(ports.backend)
        .map(|b| b.contains("\"status\":\"ok\""))
        .unwrap_or(false)
}

// ---- --stop（与 Go launcher pidfile 口径一致） ----

fn stop_services(exe_dir: &std::path::Path) {
    let data = data_root(exe_dir);
    let pf = data.join("platform.pids");
    match std::fs::read_to_string(&pf) {
        Ok(b) => {
            let mut n = 0;
            for line in b.lines() {
                if let Ok(pid) = line.trim().parse::<u32>() {
                    kill_group(pid, false);
                    n += 1;
                }
            }
            std::thread::sleep(Duration::from_secs(2));
            for line in b.lines() {
                if let Ok(pid) = line.trim().parse::<u32>() {
                    kill_group(pid, true);
                }
            }
            let _ = std::fs::remove_file(&pf);
            println!("已停止 {} 个平台服务进程。", n);
        }
        Err(_) => {
            #[cfg(windows)]
            {
                for name in ["backend.exe", "ontologyd.exe", "runtimed.exe", "MultiAgentLab.exe"] {
                    let _ = Command::new("taskkill")
                        .args(["/F", "/T", "/IM", name])
                        .creation_flags(CREATE_NO_WINDOW)
                        .output();
                }
                println!("已按进程名停止平台服务（无 PID 留痕）。");
            }
            #[cfg(not(windows))]
            {
                // 不按进程名兜底杀（backend/ontologyd 与 run-dev 开发实例同名，防误伤）
                println!("无 PID 留痕（可能未由本客户端启动）；不按进程名兜底回收——如需停止开发实例请用其启动脚本。");
            }
        }
    }
}

// kill 负 pid = 进程组（unix，配合 process_group(0)）；force=false 先 TERM 优雅、true 补 KILL
#[cfg(not(windows))]
fn kill_group(pid: u32, force: bool) {
    let sig = if force { "-KILL" } else { "-TERM" };
    let _ = Command::new("/bin/kill")
        .arg(sig)
        .arg(format!("-{}", pid))
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
}

// sh 兽护：壳被 kill -9 等异常退出后，2s 轮询感知并整树回收（Job Object 的 macOS 等价，
// best-effort；正常退出路径由 RunEvent::Exit 钩子先回收，兽护随后自然消亡）
#[cfg(not(windows))]
fn spawn_reaper() {
    let pids: Vec<String> = SERVICE_PIDS
        .lock()
        .unwrap()
        .iter()
        .map(|p| format!("-{}", p))
        .collect();
    if pids.is_empty() {
        return;
    }
    let groups = pids.join(" ");
    let script = format!(
        "while kill -0 {} 2>/dev/null; do sleep 2; done; kill -TERM {} 2>/dev/null; sleep 2; kill -KILL {} 2>/dev/null",
        std::process::id(),
        groups,
        groups
    );
    let _ = Command::new("/bin/sh")
        .arg("-c")
        .arg(script)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn();
}

// 退出钩子（macOS）：整组 SIGTERM → 2s → SIGKILL；Windows 无需（Job Object 随进程退出生效）
#[cfg(not(windows))]
fn shutdown_tree() {
    let pids: Vec<u32> = SERVICE_PIDS.lock().unwrap().clone();
    for pid in &pids {
        kill_group(*pid, false);
    }
    std::thread::sleep(Duration::from_secs(2));
    for pid in &pids {
        kill_group(*pid, true);
    }
}

static SERVICE_PIDS: Mutex<Vec<u32>> = Mutex::new(Vec::new());
static SHUTTING_DOWN: AtomicBool = AtomicBool::new(false);
static PORTS_BUSY: AtomicBool = AtomicBool::new(false);

pub fn run() {
    let args: Vec<String> = std::env::args().collect();
    if args.iter().any(|a| a == "--stop") {
        let exe_dir = exe_dir_of();
        stop_services(&exe_dir);
        return;
    }

    let exe_dir = exe_dir_of();
    let ports = Ports::load(&exe_dir);

    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _argv, _cwd| {
            if let Some(win) = app.get_webview_window("main") {
                let _ = win.show();
                let _ = win.set_focus();
            }
        }))
        // 关窗收托盘（CloseRequested 拦截；退出经托盘菜单触发，退出钩子整树回收）
        .on_window_event(move |window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                if !SHUTTING_DOWN.load(Ordering::SeqCst) {
                    api.prevent_close();
                    let _ = window.hide();
                }
            }
        })
        .setup(move |app| {
            let handle = app.handle().clone();
            let data_dir = data_root(&exe_dir);
            for d in [
                data_dir.clone(),
                data_dir.join("logs"),
                data_dir.join("projects"),
                data_dir.join("engines"),
                data_dir.join("engine_logs"),
                data_dir.join("bin"),
            ] {
                let _ = std::fs::create_dir_all(&d);
            }

            #[cfg(windows)]
            let job = job_handle();
            #[cfg(windows)]
            {
                let _ = job;
            }

            // 启动前残留回收：上次壳异常退出遗留的服务（仅按本数据根 PID 留痕，防误伤他方实例）
            #[cfg(not(windows))]
            {
                let pf = data_dir.join("platform.pids");
                if let Ok(b) = std::fs::read_to_string(&pf) {
                    let mut any = false;
                    for line in b.lines() {
                        if let Ok(pid) = line.trim().parse::<u32>() {
                            let _ = Command::new("/bin/kill")
                                .arg("-KILL")
                                .arg(format!("-{}", pid))
                                .stdout(Stdio::null())
                                .stderr(Stdio::null())
                                .status();
                            any = true;
                        }
                    }
                    if any {
                        std::thread::sleep(Duration::from_millis(800));
                    }
                    let _ = std::fs::remove_file(&pf);
                }
            }

            // 端口预检（共享路径）：任一端口被占 → 不拉服务，窗口落本地占位页明示占用态，
            // 防止把他方实例（如 run-dev 开发栈）渲染进本壳窗口造成数据串台
            let busy = port_open(ports.backend) || port_open(ports.ontology) || port_open(ports.runtime);            PORTS_BUSY.store(busy, Ordering::SeqCst);
            if busy {
                let msg = format!(
                    "<h2 style='font-family:sans-serif'>端口被占用，平台服务未启动</h2>\
                     <p style='font-family:sans-serif'>端口 {} / {} / {} 已被其他实例占用（可能是手动启动的开发实例）。\
                     请先停止占用端口的服务后重启本客户端。</p>",
                    ports.backend, ports.ontology, ports.runtime
                );
                if let Some(win) = handle.get_webview_window("main") {
                    // 前端占位页协议随平台（Windows=http://tauri.localhost / macOS·Linux=tauri://localhost）
                    #[cfg(windows)]
                    let home = "http://tauri.localhost/";
                    #[cfg(not(windows))]
                    let home = "tauri://localhost/";
                    let _ = win.eval(&format!(
                        "location.replace({:?});setTimeout(function(){{document.body.innerHTML={:?};}},400);",
                        home, msg
                    ));
                    let _ = win.show();
                    let _ = win.set_focus();
                }
                let _ = handle.emit("service-error", "端口被占用".to_string());
            }

            let ports_for_spawn = ports.clone();
            let data_for_spawn = data_dir.clone();
            let handle_for_spawn = handle.clone();
            std::thread::spawn(move || {
                if !PORTS_BUSY.load(Ordering::SeqCst) {
                    let mut kids = Vec::new();
                    if let Err(e) = spawn_services(
                        &handle_for_spawn,
                        &exe_dir_of(),
                        &data_for_spawn,
                        &ports_for_spawn,
                        &mut kids,
                    ) {
                        eprintln!("[desktop] {}", e);
                        let _ = handle_for_spawn.emit("service-error", e);
                    }
                    drop(kids);
                    #[cfg(not(windows))]
                    spawn_reaper();
                }

                let ready = {
                    let deadline = std::time::Instant::now() + Duration::from_secs(60);
                    loop {
                        if backend_healthy(&ports_for_spawn) {
                            break true;
                        }
                        if std::time::Instant::now() >= deadline {
                            break false;
                        }
                        std::thread::sleep(Duration::from_millis(300));
                    }
                };
                if PORTS_BUSY.load(Ordering::SeqCst) {
                    return; // 占用态：窗口已落本地占位页，不刷新重载
                }
                if let Some(win) = handle_for_spawn.get_webview_window("main") {
                    let _ = win.show();
                    let _ = win.set_focus();
                    let _ = win.emit("backend-ready", ready);
                    let _ = win.eval("setTimeout(function(){try{location.reload()}catch(e){}},600)");
                }
            });

            // 托盘（打开主界面 / 服务状态 / 打开数据目录 / 退出）
            let exe_dir_for_menu = exe_dir.clone();
            let open_item = MenuItem::with_id(app, "open", "打开主界面", true, None::<&str>)?;
            let status_item = MenuItem::with_id(
                app,
                "status",
                if busy { "服务状态：端口被占用" } else { "服务状态：启动中…" },
                false,
                None::<&str>,
            )?;
            let data_item = MenuItem::with_id(app, "data", "打开数据目录", true, None::<&str>)?;
            let quit_item =
                MenuItem::with_id(app, "quit", "退出（停止平台服务）", true, None::<&str>)?;
            let sep1 = PredefinedMenuItem::separator(app)?;
            let sep2 = PredefinedMenuItem::separator(app)?;
            let menu = Menu::with_items(
                app,
                &[&open_item, &status_item, &sep1, &data_item, &sep2, &quit_item],
            )?;
            let tray = TrayIconBuilder::with_id("main-tray")
                .icon(app.default_window_icon().unwrap().clone())
                .menu(&menu)
                .show_menu_on_left_click(false)
                .tooltip(APP_TITLE)
                .on_menu_event(move |app, event| match event.id().as_ref() {
                    "open" => {
                        if let Some(win) = app.get_webview_window("main") {
                            let _ = win.show();
                            let _ = win.set_focus();
                        }
                    }
                    "data" => {
                        #[cfg(windows)]
                        {
                            let _ = Command::new("explorer").arg(data_root(&exe_dir_for_menu)).spawn();
                        }
                        #[cfg(not(windows))]
                        {
                            let _ = Command::new("open").arg(data_root(&exe_dir_for_menu)).spawn();
                        }
                    }
                    "quit" => {
                        // 退出 → RunEvent::Exit 钩子整树回收（macOS 进程组阶梯 / Windows Job 关闭生效）
                        app.exit(0);
                    }
                    _ => {}
                })
                .on_tray_icon_event(|tray, event| {
                    if let TrayIconEvent::Click {
                        button: MouseButton::Left,
                        button_state: MouseButtonState::Up,
                        ..
                    } = event
                    {
                        let app = tray.app_handle();
                        if let Some(win) = app.get_webview_window("main") {
                            let _ = win.show();
                            let _ = win.set_focus();
                        }
                    }
                })
                .build(app)?;
            let _ = tray.set_visible(true);

            // 服务状态轮询 → 托盘菜单项 + tooltip；backend 崩溃自愈 ≤1 次（R-X11 口径）
            let app_for_poll = handle.clone();
            let status_for_poll = status_item.clone();
            let ports_for_poll = ports.clone();
            let exe_dir_for_poll = exe_dir.clone();
            let data_dir_for_poll = data_dir.clone();
            #[cfg(not(windows))]
            let _ = (&exe_dir_for_poll, &data_dir_for_poll); // 自愈仅 Windows（见下）
            std::thread::spawn(move || {
                let mut restarted = false;
                loop {
                    std::thread::sleep(Duration::from_secs(3));
                    let busy = PORTS_BUSY.load(Ordering::SeqCst);
                    let healthy = backend_healthy(&ports_for_poll);
                    let text = if busy {
                        "服务状态：端口被占用（他方实例）".to_string()
                    } else if healthy {
                        format!("服务状态：运行中（:{}）", ports_for_poll.backend)
                    } else {
                        "服务状态：启动中/异常".to_string()
                    };
                    let _ = status_for_poll.set_text(&text);
                    if let Some(t) = app_for_poll.tray_by_id("main-tray") {
                        let _ = t.set_tooltip(Some(&format!("{} · {}", APP_TITLE, text)));
                    }
                    if !busy && !healthy && !restarted && !SHUTTING_DOWN.load(Ordering::SeqCst) {
                        restarted = true;
                        #[cfg(windows)]
                        std::thread::spawn(move || {
                            // 自愈 ≤1 次（R-X11；Windows Job Object 覆盖该子进程的回收）。
                            // macOS 不自愈：与 mlabd 无自愈口径一致——`--stop` 兜底语义与自愈打架
                            // 且重生进程游离于回收名册外（实测泄漏），异常态由托盘如实呈现。
                            let exe = svc_path(&exe_dir_for_poll, "backend");
                            let data = data_dir_for_poll.clone();
                            let mut cmd = Command::new(exe);
                            cmd.env("ADDR", format!(":{}", ports_for_poll.backend))
                                .env("DB_PATH", data.join("platform.db"))
                                .env("SECRET_KEY_FILE", data.join(".secret"))
                                .env("FILES_ROOT", data.join("projects"))
                                .env("KB_VECTOR_BACKEND", "sqlite");
                            set_hidden(&mut cmd, &data.join("logs").join("backend.log"));
                            if let Ok(child) = cmd.spawn() {
                                let pid = child.id();
                                SERVICE_PIDS.lock().unwrap().push(pid);
                                use std::io::Write as _;
                                if let Ok(mut f) = std::fs::OpenOptions::new()
                                    .create(true)
                                    .append(true)
                                    .open(data.join("platform.pids"))
                                {
                                    let _ = writeln!(f, "{}", pid);
                                }
                                drop(child); // 仅释放句柄，进程继续运行
                            }
                        });
                    }
                }
            });

            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building MultiAgentLab desktop")
        .run(|_app, event| {
            if let tauri::RunEvent::Exit = event {
                SHUTTING_DOWN.store(true, Ordering::SeqCst);
                #[cfg(not(windows))]
                shutdown_tree();
                // Windows：Job Object 随进程退出整树终止（R-X11），无需显式 kill
                // pid 留痕清理（两平台一致；Windows 侧 stop_services 依赖该文件兜底）
                let _ = std::fs::remove_file(data_root(&exe_dir_of()).join("platform.pids"));
            }
        });
}

// ---- Job Object（Windows：关窗/退出整树回收，R-X11） ----

#[cfg(windows)]
fn job_handle() -> Option<isize> {
    use windows::Win32::System::JobObjects::{
        AssignProcessToJobObject, CreateJobObjectW, JobObjectExtendedLimitInformation,
        SetInformationJobObject, JOBOBJECT_EXTENDED_LIMIT_INFORMATION,
    };
    use windows::Win32::System::Threading::GetCurrentProcess;
    unsafe {
        let job = CreateJobObjectW(None, None).ok()?;
        let mut info = JOBOBJECT_EXTENDED_LIMIT_INFORMATION::default();
        info.BasicLimitInformation.LimitFlags =
            windows::Win32::System::JobObjects::JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
        let info_ptr = &info as *const JOBOBJECT_EXTENDED_LIMIT_INFORMATION as *const core::ffi::c_void;
        let _ = SetInformationJobObject(
            job,
            JobObjectExtendedLimitInformation,
            unsafe { &*info_ptr },
            std::mem::size_of_val(&info) as u32,
        );
        // 本进程与后续子进程全部纳入 Job：进程退出/被杀时 Job 句柄关闭 → 整树终止（R-X11）
        let me = GetCurrentProcess();
        let _ = AssignProcessToJobObject(job, me);
        Some(job.0 as isize)
    }
}

fn exe_dir_of() -> PathBuf {
    std::env::current_exe()
        .ok()
        .and_then(|p| p.parent().map(|d| d.to_path_buf()))
        .unwrap_or_else(|| PathBuf::from("."))
}
