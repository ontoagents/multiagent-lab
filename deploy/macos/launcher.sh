#!/bin/bash
# OntoAgents.app 主可执行（Contents/MacOS/OntoAgents，bundle 脚本形态）。
#
# 双击/open 时：后台拉起 mlabd（服务编排器）→ exec osascript 窗口壳（AppleScriptObjC，
# 同一 PID 承载 NSApplication → 单 Dock 图标；mlabd 以 ppid 监视壳存活，壳死整树回收）。
# 终端直跑：-stop / -headless 透传 mlabd（诊断/冒烟自动化）。
set -u
DIR="$(cd "$(dirname "$0")" && pwd)"
RES="$(cd "$DIR/../Resources" && pwd)"
MLABD="$RES/bin/mlabd"

case "${1:-}" in
  -stop|-headless)
    exec "$MLABD" "$@" ;;
esac

# 窗口导航地址与 mlabd 缺省 backend 端口同源（REQ-332 默认段 8180；MLAB_URL 可显式覆盖）
export MLAB_URL="${MLAB_URL:-http://127.0.0.1:8180}"
export MLAB_HEALTH_URL="${MLAB_HEALTH_URL:-$MLAB_URL/healthz}"
export MLAB_MLABD="$MLABD"

# mlabd 诊断输出随壳调试日志走（缺省 /dev/null；控制台/系统日志可见性由 launchd 决定）。
# 无 flag = 编排三服务 + 父进程监视（壳退出即整树回收）。
if [ -n "${MLAB_SHELL_LOG:-}" ]; then
  mkdir -p "$(dirname "$MLAB_SHELL_LOG")"
  "$MLABD" </dev/null >>"$MLAB_SHELL_LOG" 2>&1 &
else
  "$MLABD" </dev/null >/dev/null 2>&1 &
fi

exec /usr/bin/osascript "$RES/shell/window.applescript"
