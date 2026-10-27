# 桌面客户端「选目录」修复（REQ-133 bugfix）构建与验证说明

## 问题回顾

Windows 客户端创建项目时点「选目录」无反应/报错，web 端正常。

根因：两端走同一条链路（同一份 web 构建产物 + 同一个后端 handler
`POST /api/projects/pick-dir`），唯一差异在**后端进程的运行环境**——
桌面壳以 `CREATE_NO_WINDOW` 把 backend 拉起为无控制台、无前台会话的
无头服务进程，其代为唤起的 `powershell FolderBrowserDialog` 在这种
进程链下弹窗不可靠（创建失败被归一为 400「目录选择已取消或不可用
（远程部署请手填路径）」，或弹到 WebView 窗口之后不可见）。

## 修复内容（改动文件清单）

| 文件 | 改动 |
|---|---|
| `web/src/api/native.ts` | **新增**：运行时补丁。检测到 Tauri 环境（`window.__TAURI__.dialog`）时改写 `api.pickProjectDir` 走壳进程原生对话框；用户取消返回 `{ dir:'', cancelled:true }` |
| `web/src/main.tsx` | 引导时调用 `installNativePickDir()`（渲染前安装，所有调用点自动生效，无需逐个改） |
| `web/src/components/ProjectModal.tsx` | 取消静默处理（不清空已填路径、不弹误导 toast）；按钮 tooltip 文案修正 |
| `apps/desktop/src-tauri/tauri.conf.json` | `app.withGlobalTauri: true`（暴露 `window.__TAURI__`，插件 JS API 随之注入） |
| `apps/desktop/src-tauri/capabilities/default.json` | **新增**：`core:default` + `dialog:default` 授权；**含 `remote.urls`（关键，见下）** |
| `apps/desktop/src-tauri/Cargo.toml` | 新增依赖 `tauri-plugin-dialog = "2"` |
| `apps/desktop/src-tauri/src/lib.rs` | ① builder 链注册 `tauri_plugin_dialog::init()`；② `set_hidden` 双平台统一：服务 stdout/stderr 落数据根 `logs/<svc>.log`（原 Windows 分支 `Stdio::null()` 是排障黑洞）；③ 超时文案/托盘 tooltip 改 String 拼接防 format 笔误 |

## 关键点：capabilities 的 `remote.urls` 为什么必须有

Tauri 2 的 IPC 初始化脚本（`window.__TAURI__`）默认**只注入本地协议源**
（`tauri://localhost` / Windows 的 `http://tauri.localhost`）。而桌面壳
就绪后会把窗口导航到 backend 托管的 SPA：`http://127.0.0.1:8180`——
这是**外部 http 源**。若 capabilities 不声明：

```json
"remote": { "urls": ["http://127.0.0.1:*", "http://localhost:*"] }
```

则 backend 页面里 `window.__TAURI__` 根本不存在，`native.ts` 的环境
检测静默失败、回落旧的后端路径，修复无效。端口用通配 `:*` 是为了覆盖
REQ-332 的顺延端口档（8180 被占时自动 +100 顺延）。

安全边界：remote IPC 是窗口级信任声明；本 capability 只授予
`core:default` + `dialog:default`，且窗口只加载本机 backend 页面，
不扩大到任意远程站点。

## 构建步骤（Windows）

```powershell
# 1. 构建 web 前端（native.ts / main.tsx / ProjectModal.tsx 改动随包）
cd web
npm run build
cd ..

# 2. 同步 web 产物到桌面包资源（与包内旧产物同源，务必刷新）
Remove-Item -Recurse -Force apps/desktop/src-tauri/resources/web-dist
Copy-Item -Recurse web/dist apps/desktop/src-tauri/resources/web-dist

# 3. 构建桌面安装包（首次会 cargo 拉取 tauri-plugin-dialog，需联网）
cd apps/desktop
npm install          # 首次或 CI
npm run build        # = tauri build --bundles nsis
```

macOS 侧：`npm run build:macos`（`tauri build --bundles app`），前端
同步步骤相同；native.ts 双平台通用（WKWebView 同样经 remote IPC 注入）。

## 验证清单

1. **桌面客户端**：新建项目 → 点「选目录」→ 弹出系统目录对话框
   （owner 为主窗口，必在前台，不会被 WebView 压住）；
2. **取消**：对话框点取消 → 无任何 toast，已填路径保留；
3. **选择**：绝对路径回填并自动触发目录检测（DirCheckResult 展示）；
4. **web 浏览器（同机 run-dev）**：行为不变（仍走后端 powershell
   路径，REQ-133 原语义）；
5. **web 远程部署**：后端 400 → 提示手填路径（不变）；
6. **日志黑洞修复**：`%LOCALAPPDATA%\multiagent-lab\logs\` 下
   `backend.log`、`ontology-service.log`、`runtime-manager.log`
   现在应有实际内容（排障可用）；
7. **回归**：托盘 tooltip 仍为「应用名 · 状态」；启动超时诊断页
   文案仍带实际端口。

## 备注

- `capabilities/default.json` 是本仓库首个 capability 文件：仅授予
  core/dialog 默认权限，不影响既有托盘/单实例（均为 Rust 侧实现，
  不经 IPC）。
- 壳窗口先加载本地过渡页、后 `location.replace` 到 backend 地址——
  两类页面均有 `window.__TAURI__`（本地源自带；backend 源经 remote
  声明注入），故 backend 托管的 SPA 同样可用。
- 后端 `POST /api/projects/pick-dir` 保留未动：它是浏览器同机部署的
  路径；其「取消/失败归一 400」的文案可区分优化（退出码/超时细分）
  可作后续独立项。
- 工具教训（仓库协作注意）：本仓库内 grep 对部分代码文件返回空不可信
  （本次 lib.rs 验证即误报），核对文件内容以 read_file 为准。
