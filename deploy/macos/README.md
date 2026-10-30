# macOS 桌面客户端（REQ-324 / M127）

双击即用的 macOS 桌面客户端：**WKWebView 壳窗口 + 三 Go 服务编排 + 零编译器依赖构建**。
与 `deploy/windows/`（REQ-321/M123）同构的 macOS 对称实现；对应 10 号《跨端改造方案》R-X3/R-X11/SC-X1 的 macOS 单平台实现（形态 A-lite 全量内嵌）。

## 产物

| 产物 | 说明 |
| --- | --- |
| `dist/OntoAgents.app` | 应用 bundle（拖入「应用程序」即完成安装）；双击 → 拉起三服务 → 打开平台窗口 |
| `dist/OntoAgents_<版本>.dmg` | 分发镜像（hdiutil UDZO 压缩）。**版本口径（2026-10-15 定）**：主版本 0，次版本随每次成功构建 +1（`deploy/macos/.build-number` 计数器，gitignored），后缀=构建时 git 提交 id short——形如 `OntoAgents_0.3-425f096.dmg`；semver 形态 `0.3.0-425f096` 注入 Info.plist 与 tauri.conf |
| 安装后体积 | 见构建输出实测（三服务二进制 + web/dist + docs/platform-knowledge/research 资源随包） |
| 数据目录 | `~/Library/Application Support/ontoagents/`（独立于程序目录，删除/升级不触碰；含 SQLite 三库、项目文件、引擎数据、日志）。**oxigraph 内置随包**（REQ-321 零捆绑口径 2026-10-15 反转；build.sh 下载官方 v0.5.11 双架构 lipo universal）：壳/编排器首启把随包副本种子到 `ontoagents/bin`（已存在不覆盖），运行页引擎即装即用；后续版本更新仍走「一键安装」 |

## 构建

```bash
deploy/macos/build.sh              # 全量（前端+三服务+编排器+组装+签名+DMG）
#                                   DMG 默认拖拽安装排版（应用图标+Applications 别名+背景引导+固定窗口布局，
#                                   dmg_layout.py 纯 Python 写 .DS_Store 零访达自动化权限；UDRW→UDZO 转换+
#                                   挂只读 codesign 校验硬闸；降级链=无 Chrome 纯色背景/排版失败纯镜像/-PlainDmg 退回；
#                                   依赖 pip install --user --break-system-packages dmgbuild〔带 ds_store/mac_alias〕）
deploy/macos/build.sh -SkipWeb     # 跳过前端构建
deploy/macos/build.sh -StageOnly   # 只重新组装/签名（复用 build/bin 与 web/dist）
deploy/macos/build.sh -SkipDmg     # 只出 .app
```

依赖：**Go 1.25+、Node 22+，零 Xcode CLT / 零 cgo / 零第三方 Go 依赖**——
窗口壳为系统自带 `osascript` 承载的 AppleScriptObjC + WKWebView（正身机 Xcode CLT 损坏亦不受影响）。

## 启动与停止

- **启动**：双击 OntoAgents.app → `Contents/MacOS/OntoAgents`（bundle 脚本）后台拉起编排器 `Resources/bin/mlabd`（按序 ontologyd(:8191)→runtimed(:8190)→backend(:8180)，REQ-332 默认段，环境面与 `run-dev.sh` 一一同构）→ `exec osascript shell/window.applescript` 承载 WKWebView 窗口（同一 PID，单 Dock 图标）。
- **关闭窗口 / Cmd+Q**：壳 `applicationWillTerminate` → `mlabd -stop` 整树回收；mlabd 另有 **ppid 监视**兜底（壳被强杀时 `getppid()` 归 1 即自回收）——等价 Windows Job Object「关窗整树回收」。
- **停止**（终端兜底）：`OntoAgents.app/Contents/MacOS/OntoAgents -stop`。
- **无窗口冒烟**：`.../Resources/bin/mlabd -headless`（就绪后打印 `READY` 并驻留，自动化用）。

## 壳架构（与 Windows 壳的对称与差异）

| 关注点 | Windows（REQ-321） | macOS（本轮） |
| --- | --- | --- |
| 窗口壳 | jchv/go-webview2（Go 内嵌 WebView2） | osascript + AppleScriptObjC + WKWebView（零 cgo） |
| 进程回收 | Job Object（KILL_ON_JOB_CLOSE） | Setpgid 进程组 + SIGTERM→SIGKILL 阶梯 + ppid 监视 |
| 数据根 | `%LOCALAPPDATA%\ontoagents` | `~/Library/Application Support/ontoagents`（`MLAB_DATA_ROOT` 可覆盖，冒烟隔离用） |
| 安装包 | NSIS/SFX per-user | .app + DMG（拖入「应用程序」） |
| 残留回收 | PID 留痕 + 进程名兜底 | **仅 PID 留痕**（不按进程名杀——避免误伤 run-dev.sh 开发实例，端口占用时明确报错） |
| Python sidecar | 零捆绑诚实降级 | 同左 + 自动探测含 rdflib 的 python3（沿 run-dev.sh 候选序，`SIDECAR_PYTHON` 优先） |

## 诚实边界（v1.0.0）

- 单机单实例；不支持开机自启/系统托盘/自动更新（沿 Windows 口径 P2）。
- **公证就绪形态（2026-10-20）**：默认 ad-hoc 签名——从网络下载的包会被 Gatekeeper 拦截（本机构建直接拷贝不触发），收包解除=`xattr -dr com.apple.quarantine /Applications/OntoAgents.app` 一行（macOS 15 起「右键→打开」旁路已移除）；构建端配置 `MACOS_SIGN_IDENTITY`（Developer ID Application 证书，`--options runtime` 强化运行时）+ `NOTARY_PROFILE`（`xcrun notarytool store-credentials` 一次性存储）即自动切换 Developer ID 签名 + `notarytool submit --wait` + `stapler staple`——公证版收包人下载即用零提示。CI 接线（P12/API 密钥入 Secrets + keychain 导入步）待开通开发者账号后随首跑验证落库。应用图标为系统默认（P2）。
- 菜单栏 App 名与 Dock 名取自 Info.plist（`OntoAgents`）；osascript 进程名在活动监视器可见（窗口/Dock/菜单形态不受影响）。
- 壳若被 `kill -9` 强杀，服务树由 mlabd ppid 监视在 ≤2s 内回收（非系统级 Job Object 强保真）。
- 本体 TTL/OWL 导入导出需 Python3+rdflib（探测失败自动降级，其余功能不受影响）；Docker 沙箱/Qdrant 需自行安装。
- 端口缺省 **8180/8191/8190**（REQ-332：与终端开发栈 run-dev.sh 的 8080/8091/8090 开箱即分工，两栈可并行互不干扰）；
  被占用时 mlabd 启动明确报错不代收（单机单实例口径，不做自动顺延——顺延兜底在 Tauri 壳默认形态）；
  个性化换口可在 `OntoAgents.app/Contents/MacOS/` 放 launcher.json（mlabd/Go 壳路径不读该文件，换口用 Tauri 壳）。

## macOS 平台移植注

REQ-321 已交付的跨平台基建（`procgroup_{unix,windows}` 进程组封装、`signal_windows` 桩、执行位判定等）使三服务 `GOOS=darwin` 零改动可用；本轮未新增平台分支代码，客户端面全部落在 `deploy/macos/`。
