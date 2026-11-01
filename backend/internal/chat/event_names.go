package chat

// REQ-537/M344：SSE 事件协议注册表常量单源——事件名唯一在此定义（02 §8 事件一表为文档镜像，
// doccheck 后续可校验两者一致）。此前事件名散落 newEvent 字符串字面量，新增事件靠里程碑行携带。

// 事件类型常量（SSE event 名=落库 run_event.type）。
const (
	EventRunStarted      = "run.started"
	EventRunFinished     = "run.finished"
	EventRunError        = "run.error"
	EventRunWarning      = "run.warning"
	EventRunInterrupted  = "run.interrupted"
	EventRunPreempted    = "run.preempted"    // REQ-385/M181：steer 抢占生效
	EventSteerPushed     = "steer.pushed"     // REQ-385/M181：插话受理（late=运行已收束）
	EventSteerApplied    = "steer.applied"    // REQ-385/M181：插话轮界消费
	EventMessageDelta    = "message.delta"
	EventMessageSegment  = "message.segment"
	EventReasoningDelta  = "reasoning.delta"
	EventModelStep       = "model.step"
	EventToolCall        = "tool.call"
	EventToolResult      = "tool.result"
	EventSkillLoaded     = "skill.loaded"
	EventSubagentEnter   = "subagent.enter"
	EventSubagentExit    = "subagent.exit"
	EventRetrieval       = "retrieval"
	EventContextStats    = "context.stats"
	EventConnectionLost  = "connection.lost" // REQ-514/M321：client 层合成（后端不发出）
	EventOntologyQuery   = "ontology.query"
	EventOntologyHit     = "ontology.hit"
	EventOntologyUnavail = "ontology.unavailable"
	EventCompanionPrefix = "companion." // 族前缀（extract/candidates/ingest/hit…）
	EventArtifactSaved   = "artifact.saved"
	EventApprovalReq     = "approval.request"
	EventApprovalGranted = "approval.granted"
	EventApprovalDenied  = "approval.denied"
	EventHookDenied      = "hook.denied"
	EventVerifyCompleted = "verify.completed"
	EventVerifyFailed    = "verify.failed"
	EventConnectorDegr   = "connector.degraded"
	EventDebugCLI        = "debug.cli"
	EventSubagentDel     = "subagent.delegate"
	EventWorkflowNode    = "workflow.node"     // REQ-557/M364 补录缺口：工作流节点开始/完成（phase=started|completed——runner 字面量在案）
	EventStateChanged    = "state.changed"     // REQ-557/M364 补录缺口：会话状态族变更（interrupt/resume/abandon）
	EventRunPaused       = "run.paused"       // REQ-558/M365：pause 安全点挂起（对称 interrupted）
	EventWFNodeStarted   = "wf.node.started"   // REQ-563/M370：WF 一等节点事件族（node_id/status/phase/attempt——113 号归一轮单源口径）
	EventDagPlanCreated  = "dag.plan.created"  // REQ-552/M377：DAG 计划层事件（113 号归一轮口径——仅计划层两事件，node 级以 wf.node.* 为基准）
	EventDagPlanFinished = "dag.plan.finished" // REQ-552/M377
	EventWFNodeCompleted = "wf.node.completed" // REQ-563/M370
	EventWFNodeFailed    = "wf.node.failed"    // REQ-563/M370
	EventWFNodeSkipped   = "wf.node.skipped"   // REQ-563/M370：条件边不满足/上游失败传播
	EventRunResumed      = "run.resumed"      // REQ-558/M365：unpause 续跑（新 runID 沿 checkpoint）
	EventRedoAnchored    = "redo.anchored"    // REQ-558/M365：redo 锚点（截断计数+redo_of 溯源）
	EventReviewSubmitted = "review.submitted"  // REQ-553/M360：评审 verdict 提交（submit_review 工具体内）
	EventConflictDete    = "conflict.detected" // REQ-553/M360：规则档命中分歧（review 点挂起/event 点 pending）
	EventConflictReso    = "conflict.resolved" // REQ-553/M360：人工裁决落库（decision_source=manual）
)
