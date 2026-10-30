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
)
