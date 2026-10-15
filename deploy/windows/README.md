# Windows 桌面安装包（REQ-321 / M123）

双击即用的 Windows 桌面客户端：**WebView2 壳窗口 + 三 Go 服务编排 + per-user 免管理员安装**。
对应 10 号《跨端改造方案》R-X3/R-X11/SC-X1 的 Windows 单平台先行实现（形态 A 全量内嵌）。

## 产物

| 产物 | 说明 |
| --- | --- |
| `dist/MultiAgentLab_1.0.0_setup.exe`（约 36 MB） | 自解压安装包（Go 引导器 + 附加 zip）；双击 → 控制台解压安装到 `%LOCALAPPDATA%\Programs\MultiAgentLab` + 开始菜单/桌面快捷方式 + HKCU 卸载注册项（「应用与功能」可见）。`MLAB_SILENT=1` 环境变量静默（冒烟自动化用） |
| 安装后目录（约 93 MB） | `MultiAgentLab.exe`（壳）、`bin\`（backend/ontologyd/runtimed）、`web\dist`、`migrations\`、`docs\ + platform-knowledge\ + research\`（平台知识随包）、`tools\rdf-sidecar\`、`README.txt` |
| 数据目录 | `%LOCALAPPDATA%\eino-lab\`（REQ-321 定案：独立于程序目录，卸载/升级不触碰；含 SQLite 三库、项目文件、引擎数据、日志）。**oxigraph 不随包**（REQ-321 零捆绑定案）：首启在「本体-运行」页一键下载安装（落 `eino-lab\bin`，即时生效）；构建期 `-IncludeOxigraph` 可显式预置 |

## 构建

```powershell
powershell -ExecutionPolicy Bypass -File deploy/windows/build.ps1            # 全量（三服务+壳+前端+打包）
powershell -ExecutionPolicy Bypass -File deploy/windows/build.ps1 -SkipWeb   # 跳过前端构建
powershell -ExecutionPolicy Bypass -File deploy/windows/build.ps1 -StageOnly # 只组装 staging
```

依赖：Go 1.25+、Node 22+、网络（oxigraph 默认不抓取，`-IncludeOxigraph` 时抓官方 release）。
打包双路径：**检测到 ISCC（Inno Setup 6）走 `installer.iss`（推荐正式发布：中文向导/标准卸载器）；
否则走 Go 自解压引导器兜底**（`setup/main.go`：读自身尾部附加 zip → 解压临时目录 → 执行 installer.cmd；
曾用 7zSD.sfx 方案因其配置标记兼容性黑盒〔双击报「找不到 setup.exe」〕弃用）。

## 安装与卸载

- **安装**：双击 setup.exe（SFX：双击即装；Inno：中文向导）→ `MLAB_SILENT=1` 环境变量可静默（冒烟自动化用）。
- **启动**：开始菜单/桌面「智能体构建平台」→ 壳自动按序拉起 ontologyd(:8091)→runtimed(:8090)→backend(:8080)，就绪后打开 WebView2 窗口；关闭窗口即整树回收（Job Object）。
- **停止**：开始菜单「停止平台服务」（= `MultiAgentLab.exe -stop`）。
- **卸载**：「应用与功能」或开始菜单卸载项 → 停服 → 删快捷方式/注册项 → 删程序文件；**`data\` 目录有意保留**（用户数据），彻底清理请手动删除安装目录。

## 启动命令

```
MultiAgentLab.exe            # 桌面壳（WebView2 + 服务编排）
MultiAgentLab.exe -headless  # 无窗口编排（自动化冒烟：就绪后打印 READY 并驻留）
MultiAgentLab.exe -stop      # 停止全部平台服务
```

## 目录与数据

- 程序：`%LOCALAPPDATA%\Programs\MultiAgentLab`；数据：`%LOCALAPPDATA%\eino-lab\`（SQLite 三库、项目文件、引擎数据、伴生快照、日志 `logs\`、一键安装的 oxigraph `bin\`）。
- 备份/迁移：停服后复制 `eino-lab\`（换机连 `eino-lab\.secret` 一起带走，否则模型连接密钥无法解密）。
- 端口：8080 / 8091 / 8090 固定，oxigraph 子进程 9201+；被占用时启动会明确报错。
- 服务环境变量与 `run-dev.sh` 一一同构（全量清单见 `platform-knowledge/01_整体设计/16_部署与运行.md` §7）；
  安装态差异：`KB_VECTOR_BACKEND=sqlite`（单机零依赖默认，需 Qdrant 时在 launcher `buildServices` 覆盖）。

## 诚实边界（v1.0.0）

- 单机单实例；不支持开机自启/系统托盘/自动更新（跨端方案 R-X9/X10，后续按需）。
- WebView2 Runtime 需系统预装（Win11 自带；缺失时壳自动回退默认浏览器并提示安装）。
- 本体 TTL/OWL 导入导出需系统 Python3+rdflib（未装自动降级，其余功能不受影响）。
- Docker 沙箱执行需自行安装 Docker Desktop；Qdrant 向量库不在包内（默认 SQLite 向量后端）。
- SFX 兜底安装包的安装向导为英文/简洁形态（中文向导走 Inno Setup 路径）；卸载完成提示为英文。
- 壳窗口图标为系统默认（应用图标 P2）；oxigraph 由用户首启一键下载（Apache-2.0/MIT，源 https://github.com/oxigraph/oxigraph）。
- 壳技术为 Go + WebView2 直载（jchv/go-webview2）；18 号 REQ-321 立项定案为 Tauri 2 壳（含托盘/关窗收托盘/单实例）——
  Rust 工具链未就绪，本版以同验收（WebView2 直载 127.0.0.1:8080 + 进程编排回收）等价实现先行，Tauri 切换挂待办由开发者裁决。

## Windows 平台移植（随本需求顺修的仓库级缺口）

- `backend/internal/inference/signal_windows.go`：关闭阶梯第二档信号的 Windows 桩（`!windows` 隔离文件缺失致 Windows 无法编译）。
- `backend/internal/companion/procgroup_{unix,windows}.go`、`runtime-manager/internal/procgroup/`：进程组启停跨平台封装（unix 负 PID 组信号语义零变化；Windows taskkill /T 等价）。
- `runtime-manager/cmd/runtimed/main.go`：DB_PATH 父目录 `filepath.Dir` 替换手写 basename（反斜杠路径切出空串）。
- `backend/internal/fsutil.IsExecutableFile` + `runtime-manager/internal/engine` 同名助手：Go 在 Windows 对 .exe 不置执行位（Mode()=-rw-rw-rw-），原 `Mode()&0o111` 检查在 Windows 恒假（引擎恒判「未安装」、docker 探测失效）。
- `backend/internal/tool/runcommand.go`：work_dir 绝对路径判定改 `fsutil.IsAbsDir`（原仅认 `/` 前缀）。
- 测试面：companion/store/chat 的 SQLite 测试库补 `t.Cleanup(Close)`（Windows 句柄不释放会卡死 TempDir 清理）；
  docker/engine/manager 安装链测试的执行位断言加 GOOS 分支；~~fuseki windows 一键安装拒绝沿 REQ-307 边界跳过~~
  （REQ-325 反转：一键安装改 zip 发行包形态全平台可用，测试不再 skip——见下节）。
  （仍红的测试=dsh/kubectl/docker 夹具依赖 unix 语义，见 20 号 S8 边界注；unix 侧完整回归在 WSL go1.25 实测通过。）

## Fuseki 引擎（REQ-325，2026-10-15）

Windows 上 Fuseki 引擎全链可用：**运行页「一键下载安装」**（zip 发行包 apache-jena-fuseki-6.2.0，镜像序北大→华为云→清华→阿里云→官方→archive，写入数据根 `data\bin` 即时生效）或「上传发行包安装」（.tar.gz/.zip 均可）。启动不经 shell 脚本/bat——runtimed 直接 `java -jar fuseki-server.jar`（config 路径已做正斜杠消毒）。**运行依赖（产品不代装，引擎卡 Hint 指引）**：

- **JDK 17+**（Temurin 等发行版均可；仅 JRE 时推理形态的规则解出不可用——依赖 JDK 的 jar 工具）。
- **python3 + rdflib**（本体导入/导出 sidecar，非 fuseki 专属）：python.org 安装器只装 `python.exe`，Windows 自带 `python3` App 执行别名是空壳（调用 exit 9009）——装真 python 后可在其目录复制一份 `python3.exe`，或设 `SIDECAR_PYTHON` 指向 `python.exe`。
- 一键安装的文件落在**数据根** `data\bin`（数据随安装目录时即 `<安装目录>\data\bin`）；`FUSEKI_BIN` 可显式覆盖。
