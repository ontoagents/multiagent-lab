#!/usr/bin/env python3
"""REQ-463/M263 全链冒烟示例：card 发现 → send/轮询 → input-required → message/stream。

零凭证可跑（stub 默认）：先起一个 wrapper（--port 0），本脚本直连消费。
运行：python3 examples/smoke.py
"""

import json
import sys
import time
import urllib.request
import subprocess
import os

BASE = None  # 启动后回填


def rpc(method, params):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}).encode()
    req = urllib.request.Request(BASE, data=body, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.loads(resp.read())


def wait_terminal(task_id, timeout=15):
    deadline = time.time() + timeout
    while time.time() < deadline:
        task = rpc("tasks/get", {"id": task_id})["result"]
        if task["status"]["state"] != "working":
            return task
        time.sleep(0.2)
    raise SystemExit("超时未终态")


def main() -> None:
    global BASE
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    proc = subprocess.Popen(
        [sys.executable, "-m", "agentscope_a2a", "--port", "0"],
        cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
    )
    try:
        for line in proc.stdout:
            if line.startswith("PORT="):
                BASE = f"http://127.0.0.1:{line.strip().split('=', 1)[1]}/"
                break
        assert BASE, "wrapper 未宣告端口"

        # ① card
        with urllib.request.urlopen(BASE + ".well-known/agent-card.json", timeout=5) as r:
            card = json.loads(r.read())
        print(f"① card: name={card['name']} mode={card['properties']['mode']} streaming={card['capabilities']['streaming']}")

        # ② send → completed
        task = rpc("message/send", {"message": {"role": "user", "kind": "message", "messageId": "m1",
                                                 "parts": [{"kind": "text", "text": "帮我做个计划"}]}})["result"]
        task = wait_terminal(task["id"])
        text = "".join(p["text"] for a in task["artifacts"] for p in a["parts"])
        print(f"② send→{task['status']['state']}: {text[:60]}…")

        # ③ ask: → input-required
        task = rpc("message/send", {"message": {"role": "user", "kind": "message", "messageId": "m2",
                                                 "parts": [{"kind": "text", "text": "ask: 需要确认什么？"}]}})["result"]
        task = wait_terminal(task["id"])
        q = "".join(p["text"] for p in (task["status"].get("message") or {}).get("parts", []))
        print(f"③ 终态={task['status']['state']} 问题外发={q!r}")

        # ④ tasks/cancel 回环
        t = rpc("message/send", {"message": {"role": "user", "kind": "message", "messageId": "m3",
                                             "parts": [{"kind": "text", "text": "cancel demo"}]}})["result"]
        rpc("tasks/cancel", {"id": t["id"]})
        print("④ cancel 已发（任务快，可能已终态——回环不报错即过）")
        print("SMOKE PASS")
    finally:
        proc.kill()


if __name__ == "__main__":
    main()
