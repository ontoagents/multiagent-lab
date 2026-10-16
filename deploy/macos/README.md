# macOS 桌面客户端（REQ-324 / M127）

双击即用的 macOS 桌面客户端：**WKWebView 壳窗口 + 三 Go 服务编排 + 零编译器依赖构建**。
与 `deploy/windows/`（REQ-321/M123）同构的 macOS 对称实现；对应 10 号《跨端改造方案》R-X3/R-X11/SC-X1 的 macOS 单平台实现（形态 A-lite 全量内嵌）。

## 产物

| 产物 | 说明 |
| --- | --- |
| `dist/MultiAgentLab.app` | 应用 bundle（拖入「应用程序」即完成安装）；双击 → 拉起三服务 → 打开平台窗口 |
| `dist/MultiAgentLab_1.0.0.dmg` | 分发镜像（hdiutil UDZO 压缩） |
| 安装后体积 | 见构建输出实测（三服务二进制 + web/dist + docs/platform-knowledge/research 资源随包） |
| 数据目录 | `~/Library/Application Support/multiagent-lab/`（独立于程序目录，删除/升级不触碰；含 SQLite 三库、项目文件、引擎数据、日志）。**oxigraph 内置随包**（REQ-321 零捆绑口径 2026-10-15 反转；build.sh 下载官方 v0.5.11 双架构 lipo universal）：壳/编排器首启把随包副本种子到 `multiagent-lab/bin`（已存在不覆盖），运行页引擎即装即用；后续版本更新仍走「一键安装」 |

## 构建

```bash
deploy/macos/build.sh              # 全量（前端+三服务+编排器+组装+签名+DMG）
deploy/macos/build.sh -SkipWeb     # 跳过前端构建
deploy/macos/build.sh -StageOnly   # 只重新组装/签名（复用 build/bin 与 web/dist）
deploy/macos/build.sh -SkipDmg     # 只出 .app
```

依赖：**Go 1.25+、Node 22+，零 Xcode CLT / 零 cgo / 零第三方 Go 依赖**——
窗口壳为系统自带 `osascript` 承载的 AppleScriptObjC + WKWebView（正身机 Xcode CLT 损坏亦不受影响）。

## 启动与停止

- **启动**：双击 MultiAgentLab.app → `Contents/MacOS/MultiAgentLab`（bundle 脚本）后台拉起编排器 `Resources/bin/mlabd`（按序 ontologyd(:8191)→runtimed(:8190)→backend(:8180)，REQ-332 默认段，环境面与 `run-dev.sh` 一一同构）→ `exec osascript shell/window.applescript` 承载 WKWebView 窗口（同一 PID，单 Dock 图标）。
- **关闭窗口 / Cmd+Q**：壳 `applicationWillTerminate` → `mlabd -stop` 整树回收；mlabd 另有 **ppid 监视**兜底（壳被强杀时 `getppid()` 归 1 即自回收）——等价 Windows Job Object「关窗整树回收」。
- **停止**（终端兜底）：`MultiAgentLab.app/Contents/MacOS/MultiAgentLab -stop`。
- **无窗口冒烟**：`.../Resources/bin/mlabd -headless`（就绪后打印 `READY` 并驻留，自动化用）。

## 壳架构（与 Windows 壳的对称与差异）

| 关注点 | Windows（REQ-321） | macOS（本轮） |
| --- | --- | --- |
| 窗口壳 | jchv/go-webview2（Go 内嵌 WebView2） | osascript + AppleScriptObjC + WKWebView（零 cgo） |
| 进程回收 | Job Object（KILL_ON_JOB_CLOSE） | Setpgid 进程组 + SIGTERM→SIGKILL 阶梯 + ppid 监视 |
| 数据根 | `%LOCALAPPDATA%\multiagent-lab` | `~/Library/Application Support/multiagent-lab`（`MLAB_DATA_ROOT` 可覆盖，冒烟隔离用） |
| 安装包 | NSIS/SFX per-user | .app + DMG（拖入「应用程序」） |
| 残留回收 | PID 留痕 + 进程名兜底 | **仅 PID 留痕**（不按进程名杀——避免误伤 run-dev.sh 开发实例，端口占用时明确报错） |
| Python sidecar | 零捆绑诚实降级 | 同左 + 自动探测含 rdflib 的 python3（沿 run-dev.sh 候选序，`SIDECAR_PYTHON` 优先） |

## 诚实边界（v1.0.0）

- 单机单实例；不支持开机自启/系统托盘/自动更新（沿 Windows 口径 P2）。
- 应用图标为系统默认（P2）；未做 Apple 公证（Gatekeeper 首次打开若拦截，右键 → 打开；本机构建无 quarantine 问题）。
- 菜单栏 App 名与 Dock 名取自 Info.plist（`MultiAgentLab`）；osascript 进程名在活动监视器可见（窗口/Dock/菜单形态不受影响）。
- 壳若被 `kill -9` 强杀，服务树由 mlabd ppid 监视在 ≤2s 内回收（非系统级 Job Object 强保真）。
- 本体 TTL/OWL 导入导出需 Python3+rdflib（探测失败自动降级，其余功能不受影响）；Docker 沙箱/Qdrant 需自行安装。
- 端口缺省 **8180/8191/8190**（REQ-332：与终端开发栈 run-dev.sh 的 8080/8091/8090 开箱即分工，两栈可并行互不干扰）；
  被占用时 mlabd 启动明确报错不代收（单机单实例口径，不做自动顺延——顺延兜底在 Tauri 壳默认形态）；
  个性化换口可在 `MultiAgentLab.app/Contents/MacOS/` 放 launcher.json（mlabd/Go 壳路径不读该文件，换口用 Tauri 壳）。

## macOS 平台移植注

REQ-321 已交付的跨平台基建（`procgroup_{unix,windows}` 进程组封装、`signal_windows` 桩、执行位判定等）使三服务 `GOOS=darwin` 零改动可用；本轮未新增平台分支代码，客户端面全部落在 `deploy/macos/`。
