"""REQ-463/M263 wrapper 入口：python3 -m agentscope_a2a [--port N]。

--port 0 时绑定随机端口并把实际端口打到 stdout 一行 `PORT=xxxx`（平台侧回环验证用）。
"""

from __future__ import annotations

import argparse
import os
import sys

from .http import serve
from .server import A2AServer


def main() -> None:
    ap = argparse.ArgumentParser(description="AgentScope A2A 包装器（平台外 opt-in，REQ-463/M263）")
    ap.add_argument("--port", type=int, default=int(os.environ.get("AGENTSCOPE_PORT", "8099")))
    ap.add_argument("--name", default=os.environ.get("AGENTSCOPE_AGENT_NAME", "AgentScope Wrapper Agent"))
    ap.add_argument("--description", default="经 AgentScope（或 stub）运行时处理的 A2A 外部智能体（平台外独立进程）")
    args = ap.parse_args()

    app = A2AServer(name=args.name, description=args.description, port=args.port)
    httpd = serve(app)
    print(f"PORT={httpd.server_address[1]}", flush=True)
    print(f"agentscope-a2a listening on http://127.0.0.1:{httpd.server_address[1]} mode={app.runner.mode}", flush=True)
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        pass
    return


if __name__ == "__main__":
    main()
