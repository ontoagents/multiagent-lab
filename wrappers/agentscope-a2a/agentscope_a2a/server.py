"""REQ-463/M263：AgentScope A2A 包装器——平台外 opt-in Python A2A server。

协议面对齐平台 internal/a2a server（REQ-442 形态）：
- GET  /.well-known/agent-card.json   Agent Card 发现（平台 client FetchAgentCard 消费）
- POST message/send                   异步任务化：立即回 working 态 task，后台跑完转终态
- POST tasks/get                      轮询（终态附 artifacts+history）
- POST message/stream                 SSE 订阅（每帧 data: JSON-RPC，result=任务快照）
- POST tasks/cancel                   working→canceled

两档运行模式（AGENTSCOPE_MODEL 环境变量切换）：
- stub（默认）：零第三方依赖（纯标准库），无凭证可跑全链——回显式回复+关键词路由；
  "ask:" 前缀演示 input-required 终态（对齐平台 REQ-443 语义：问题文本外发）。
- 真模型：AGENTSCOPE_MODEL 设置时经 agentscope ReActAgent 推理（凭证自备，端到端随凭证批；
  agentscope 未安装则启动期人话报错）。

D-O15 合规：本服务不进 run-dev.sh 主链（进程外可缺省，REQ-325 sidecar 先例）——
平台主链零 venv 零 Python 依赖；仅当用户显式运行本包装器时才需要 python3。
"""

from __future__ import annotations

import json
import os
import sys
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

TERMINAL_STATES = {"completed", "failed", "canceled", "rejected", "input-required"}


def _ts() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


class TaskStore:
    """进程内任务表（对齐平台 server 口径：重启丢映射，tasks/get 诚实报 TaskNotFound）。"""

    def __init__(self) -> None:
        self._mu = threading.Lock()
        self._tasks: dict[str, dict] = {}
        # contextId → 多轮历史（[{"role","text"}...]；stub 上下文即历史本身）
        self._contexts: dict[str, list[dict]] = {}

    def create(self, context_id: str) -> dict:
        task = {
            "id": uuid.uuid4().hex,
            "contextId": context_id,
            "status": {"state": "working"},
            "artifacts": [],
            "history": [],
            "timestamp": _ts(),
        }
        with self._mu:
            self._tasks[task["id"]] = task
        return task

    def get(self, task_id: str) -> dict | None:
        with self._mu:
            return self._tasks.get(task_id)

    def update(self, task_id: str, **fields) -> dict | None:
        with self._mu:
            t = self._tasks.get(task_id)
            if t is None:
                return None
            t.update(fields)
            return t

    def history_of(self, context_id: str) -> list[dict]:
        with self._mu:
            return list(self._contexts.get(context_id, []))

    def append_history(self, context_id: str, role: str, text: str) -> None:
        with self._mu:
            self._contexts.setdefault(context_id, []).append({"role": role, "text": text})

    def working_task_of(self, context_id: str) -> dict | None:
        with self._mu:
            for t in self._tasks.values():
                if t["contextId"] == context_id and t["status"]["state"] == "working":
                    return t
        return None


def _text_of_parts(parts: list) -> str:
    return "".join(p.get("text", "") for p in (parts or []) if p.get("kind", "text") == "text")


def _snapshot(task: dict, with_payload: bool) -> dict:
    """task → A2A wire 形态（终态附 artifacts+history，对齐平台 server 快照口径）。"""
    out = {
        "id": task["id"],
        "contextId": task["contextId"],
        "kind": "task",
        "status": dict(task["status"]),
        "timestamp": task["timestamp"],
    }
    if with_payload:
        out["artifacts"] = task["artifacts"]
        out["history"] = [
            {"role": h["role"], "kind": "message", "parts": [{"kind": "text", "text": h["text"]}]}
            for h in task["history"]
        ]
    return out


class AgentScopeRunner:
    """回复执行器：stub / agentscope 真模型两档。"""

    def __init__(self) -> None:
        self.model = os.environ.get("AGENTSCOPE_MODEL", "").strip()
        self._agent = None
        if self.model:
            try:
                from agentscope.agents import ReActAgent  # type: ignore
                from agentscope.models import DashScopeChatWrapper  # type: ignore
            except ImportError as e:  # 启动期人话报错（真模型档凭证/依赖自备）
                sys.exit(
                    f"AGENTSCOPE_MODEL={self.model} 需安装 agentscope 且配置凭证，导入失败: {e}\n"
                    "提示：pip install agentscope 且设置 DASHSCOPE_API_KEY；stub 模式请 unset AGENTSCOPE_MODEL"
                )
            model = DashScopeChatWrapper(model_name=self.model)
            self._agent = ReActAgent(
                name=os.environ.get("AGENTSCOPE_AGENT_NAME", "agentscope-wrapper"),
                sys_prompt=os.environ.get("AGENTSCOPE_SYS_PROMPT", "你是一个经由 A2A 接入平台的智能体。"),
                model=model,
                max_iters=int(os.environ.get("AGENTSCOPE_MAX_ITERS", "8")),
            )

    @property
    def mode(self) -> str:
        return "agentscope" if self._agent is not None else "stub"

    def reply(self, user_text: str, history: list[dict]) -> tuple[str, str | None]:
        """返回 (final_text, pending_question)。

        pending_question 非 None → 任务转 input-required 终态（问题文本外发，
        对齐平台 server REQ-443 ask_human 挂起语义）。
        """
        if self._agent is not None:
            from agentscope.message import Msg  # type: ignore
            msg = self._agent(Msg("user", user_text, "user"))
            return (getattr(msg, "content", "") or "", None)
        return self._stub_reply(user_text, history)

    @staticmethod
    def _stub_reply(user_text: str, history: list[dict]) -> tuple[str, str | None]:
        text = user_text.strip()
        # input-required 终态演示："ask: 问题" → 需要人工补答（stub 不臆造答案）
        if text.lower().startswith("ask:"):
            return ("", text[4:].strip() or "（空问题）")
        turns = len(history) + 1
        if "计划" in text or "plan" in text.lower():
            body = (
                f"【stub 计划】针对「{text[:60]}」的执行计划：\n"
                "1. 拆解任务为三步（stub 演示，无真实工具）\n"
                "2. 逐步执行并汇总\n"
                "3. 输出结论"
            )
        else:
            body = f"【stub 回显】已收到第 {turns} 轮消息：「{text[:80]}」。AgentScope 包装器运行正常（stub 模式，无模型凭证）。"
        return (body, None)


class A2AServer:
    """JSON-RPC 2.0 over HTTP（标准库实现，线程化）。"""

    def __init__(self, name: str, description: str, port: int) -> None:
        self.runner = AgentScopeRunner()
        self.store = TaskStore()
        self.name = name
        self.description = description
        self.port = port

    # ---- Agent Card ----
    def card(self) -> dict:
        return {
            "name": self.name,
            "description": self.description,
            "url": f"http://127.0.0.1:{self.port}/",
            "protocolVersion": "0.2.9",
            "preferredTransport": "JSONRPC",
            "skills": [{
                "id": "agentscope-chat",
                "name": "对话（AgentScope 包装）",
                "description": f"经 AgentScope 运行时处理的通用对话能力（mode={self.runner.mode}）。",
                "tags": ["chat", "agentscope", self.runner.mode],
            }],
            "capabilities": {"streaming": True},
            "properties": {
                "mode": self.runner.mode,
                "model": self.runner.model or "(stub 无模型)",
                "runtime": "agentscope-wrapper/REQ-463",
                "costTier": "对端自管（外部成员通用口径）",
            },
        }

    # ---- JSON-RPC 方法 ----
    def rpc_message_send(self, params: dict) -> tuple[dict | None, dict | None]:
        msg = params.get("message") or {}
        text = _text_of_parts(msg.get("parts") or []).strip()
        if not text:
            return None, {"code": -32602, "message": "message.parts 缺 text 文本"}
        context_id = (msg.get("contextId") or "").strip() or uuid.uuid4().hex
        # 会话级单运行闸（对齐平台 REQ-441：同 contextId 并发 send 拒绝）
        if self.store.working_task_of(context_id) is not None:
            return None, {"code": -32002, "message": "会话已有进行中的任务（单会话串行），请待终态后重发"}
        task = self.store.create(context_id)
        self.store.append_history(context_id, "user", text)
        threading.Thread(target=self._run_task, args=(task["id"], context_id, text), daemon=True).start()
        return _snapshot(task, with_payload=False), None

    def _run_task(self, task_id: str, context_id: str, text: str) -> None:
        task = self.store.get(task_id)
        if task is None or task["status"]["state"] != "working":
            return  # 已被 cancel
        try:
            answer, pending = self.runner.reply(text, self.store.history_of(context_id)[:-1])
        except Exception as e:  # noqa: BLE001 —— 执行失败转 failed（人话透传）
            self.store.update(task_id, status={"state": "failed", "message": {"parts": [{"kind": "text", "text": str(e)}]}})
            return
        if task["status"]["state"] != "working":
            return  # 竞态防覆写（对齐平台 REQ-441 cancel 竞态修复语义）
        if pending:
            # input-required 终态：问题文本外发（对齐平台 REQ-443），不臆造答案
            self.store.append_history(context_id, "agent", f"[input-required] {pending}")
            self.store.update(
                task_id,
                status={"state": "input-required", "message": {"parts": [{"kind": "text", "text": pending}]}},
                history=self.store.history_of(context_id),  # raw 形态，wire 变换统一在 _snapshot
            )
            return
        self.store.append_history(context_id, "agent", answer)
        self.store.update(
            task_id,
            status={"state": "completed"},
            artifacts=[{"name": "reply", "parts": [{"kind": "text", "text": answer}]}],
            history=self.store.history_of(context_id),
        )

    def rpc_tasks_get(self, params: dict) -> tuple[dict | None, dict | None]:
        task = self.store.get((params or {}).get("id", ""))
        if task is None:
            return None, {"code": -32001, "message": "TaskNotFound（进程内任务表重启即失，沿平台同口径）"}
        terminal = task["status"]["state"] in TERMINAL_STATES
        return _snapshot(task, with_payload=terminal), None

    def rpc_tasks_cancel(self, params: dict) -> tuple[dict | None, dict | None]:
        task = self.store.get((params or {}).get("id", ""))
        if task is None:
            return None, {"code": -32001, "message": "TaskNotFound"}
        if task["status"]["state"] in TERMINAL_STATES:
            return _snapshot(task, with_payload=True), None
        self.store.update(task_id := task["id"], status={"state": "canceled"})
        return _snapshot(self.store.get(task_id), with_payload=True), None

    def sse_message_stream(self, params: dict) -> "list[dict] | None":
        """message/stream：SSE 帧序列（本实现先落任务再产帧——语义与 send 等价+过程帧）。"""
        send_result, err = self.rpc_message_send(params)
        if err is not None:
            return None
        return [send_result]
