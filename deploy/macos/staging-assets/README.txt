智能体构建平台 · macOS 桌面客户端（随包说明）

启动：双击 MultiAgentLab.app（或拖入「应用程序」后从启动台打开）。
首次启动自动拉起三个本地服务（ontology-service :8091 / runtime-manager :8090 / 主平台 :8080），
就绪后自动打开平台窗口；关闭窗口（或 Cmd+Q）即停止全部服务。

数据目录：~/Library/Application Support/eino-lab/
  （SQLite 三库、项目文件、引擎数据、日志、一键安装的引擎；删除本客户端不会触碰该目录。）

停止服务（兜底）：终端执行
  /Applications/MultiAgentLab.app/Contents/MacOS/MultiAgentLab -stop

诚实边界：
- 本体 TTL/OWL 导入导出需系统 Python3 + rdflib（未自动检测到时该功能降级，其余不受影响）；
  可 `pip3 install rdflib` 后重启客户端启用。
- oxigraph 引擎不随包分发：首启后在「本体 - 运行」页一键下载安装（落数据目录 bin，即时生效）。
- Docker 沙箱执行需自行安装 Docker Desktop；向量库默认 SQLite 后端（Qdrant 可选）。
- 应用图标为系统默认（P2）；未做公证（Gatekeeper 首次打开若拦截，请右键 → 打开）。
