"""REQ-463/M263 wrapper HTTP 层：标准库 ThreadingHTTPServer 承载 JSON-RPC 与 SSE。

消息驱动的协议逻辑在 server.py（A2AServer）；本模块只做 HTTP 编排：
- GET  /.well-known/agent-card.json → 卡片
- POST / → JSON-RPC（message/send、tasks/get、tasks/cancel）或 message/stream（SSE 响应）
流式订阅的后续过程帧由 StreamHub 广播（任务状态每次迁移即推一帧快照）。
"""

from __future__ import annotations

import json
import queue
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from .server import A2AServer, TERMINAL_STATES, _snapshot


class StreamHub:
    """任务状态迁移的 SSE 订阅器登记表（task_id → 队列列表）。"""

    def __init__(self) -> None:
        self._mu = threading.Lock()
        self._subs: dict[str, list[queue.Queue]] = {}

    def subscribe(self, task_id: str) -> queue.Queue:
        q: queue.Queue = queue.Queue()
        with self._mu:
            self._subs.setdefault(task_id, []).append(q)
        return q

    def unsubscribe(self, task_id: str, q: queue.Queue) -> None:
        with self._mu:
            lst = self._subs.get(task_id) or []
            if q in lst:
                lst.remove(q)

    def publish(self, task_id: str, snapshot: dict) -> None:
        with self._mu:
            for q in (self._subs.get(task_id) or []):
                q.put(snapshot)


def make_handler(app: A2AServer, hub: StreamHub) -> type[BaseHTTPRequestHandler]:
    class Handler(BaseHTTPRequestHandler):
        server_version = "agentscope-a2a/0.1"
        protocol_version = "HTTP/1.1"  # 平台 Go client keep-alive 复用语义对齐（每响应恒带 Content-Length）

        def log_message(self, fmt, *args):  # noqa: N802 —— 静默访问日志（冒烟输出干净）
            pass

        def _json(self, code: int, payload: dict) -> None:
            body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
            self.send_response(code)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):  # noqa: N802
            if self.path.rstrip("/") == "/.well-known/agent-card.json" or self.path.rstrip("/") == "/.well-known/agent.json":
                self._json(200, app.card())
                return
            self._json(404, {"error": "not found（卡地址：/.well-known/agent-card.json）"})

        def do_POST(self):  # noqa: N802
            try:
                length = int(self.headers.get("Content-Length", "0"))
                req = json.loads(self.rfile.read(length) or b"{}")
            except Exception as e:  # noqa: BLE001
                self._json(200, {"jsonrpc": "2.0", "id": None, "error": {"code": -32700, "message": f"解析失败: {e}"}})
                return
            rid = req.get("id")
            method = req.get("method", "")
            params = req.get("params") or {}

            if method == "message/stream":
                self._handle_stream(rid, params)
                return

            handlers = {
                "message/send": app.rpc_message_send,
                "tasks/get": app.rpc_tasks_get,
                "tasks/cancel": app.rpc_tasks_cancel,
            }
            fn = handlers.get(method)
            if fn is None:
                self._json(200, {"jsonrpc": "2.0", "id": rid, "error": {"code": -32601, "message": f"未知方法 {method}（支持 message/send|tasks/get|tasks/cancel|message/stream）"}})
                return
            result, err = fn(params)
            if err is not None:
                self._json(200, {"jsonrpc": "2.0", "id": rid, "error": err})
                return
            self._json(200, {"jsonrpc": "2.0", "id": rid, "result": result})

        def _handle_stream(self, rid, params) -> None:
            """SSE：send 建任务后持续订阅状态迁移推帧，终态关流（对齐平台 SendStream 消费）。"""
            # 先建任务（working 帧立即回）
            result, err = app.rpc_message_send(params)
            if err is not None:
                self._json(200, {"jsonrpc": "2.0", "id": rid, "error": err})
                return
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream; charset=utf-8")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "close")  # 流式无 Content-Length：以关流收尾
            self.end_headers()
            self.close_connection = True

            def emit(snapshot: dict) -> None:
                frame = json.dumps({"jsonrpc": "2.0", "id": rid, "result": snapshot}, ensure_ascii=False)
                self.wfile.write(f"data: {frame}\n\n".encode("utf-8"))
                self.wfile.flush()

            emit(result)
            task_id = result["id"]
            q = hub.subscribe(task_id)
            try:
                while True:
                    snap = q.get(timeout=120)
                    emit(snap)
                    if (snap.get("status") or {}).get("state") in TERMINAL_STATES:
                        break
            except queue.Empty:
                pass
            finally:
                hub.unsubscribe(task_id, q)
                # 兜底：订阅期内未拿到终态（超时）补一帧当前快照，让消费端收尾
                t = app.store.get(task_id)
                if t is not None and t["status"]["state"] not in TERMINAL_STATES:
                    emit(_snapshot(t, with_payload=False))

    return Handler


def serve(app: A2AServer) -> ThreadingHTTPServer:
    """启动任务状态迁移→SSE 的桥（store.update 后由 runner 显式 publish 简化为轮询桥）。"""
    httpd = ThreadingHTTPServer(("127.0.0.1", app.port), make_handler(app, hub := StreamHub()))
    app.port = httpd.server_address[1]

    # 状态迁移广播桥：轻量轮询任务表（200ms），状态变化即 publish——
    # runner 不感知订阅面（解耦），对齐「状态迁移即帧」语义。
    def bridge() -> None:
        last: dict[str, str] = {}
        while True:
            with app.store._mu:
                items = [(tid, dict(t["status"]), t) for tid, t in app.store._tasks.items()]
            for tid, status, t in items:
                state = status.get("state", "")
                if last.get(tid) != state:
                    last[tid] = state
                    hub.publish(tid, _snapshot(t, with_payload=state in TERMINAL_STATES))

    threading.Thread(target=bridge, daemon=True).start()
    return httpd
