package tool

import "context"

// REQ-354/P2·G4：运行调用者身份（OntoChat 会话归属审计两列的数据源）。
// runner 在 Run/Resume 装配前把调用者信息放进 ctx；ontochat_start 建会话时读取，
// 以 source=agent_chat + agent_id 落构建平面审计列——「哪个智能体产了哪些本体」可审计。
// 纯上下文附加值：装配与编排行为零变化（沿 WithEventSink 注入先例，定义在此避免环）。

// CallerInfo 本次运行的调用者（会话宿主智能体；项目协作=协调者）。
type CallerInfo struct {
	AgentID        string
	AgentName      string
	ConversationID string
	Scope          string // agent | project
}

type callerKey struct{}

// WithCaller 在 ctx 上挂调用者身份（runner 专用写入点）。
func WithCaller(ctx context.Context, ci CallerInfo) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, callerKey{}, ci)
}

// CallerFrom 读取调用者身份；非运行期（单测/直调）时 ok=false。
func CallerFrom(ctx context.Context) (CallerInfo, bool) {
	if ctx == nil {
		return CallerInfo{}, false
	}
	ci, ok := ctx.Value(callerKey{}).(CallerInfo)
	return ci, ok
}
