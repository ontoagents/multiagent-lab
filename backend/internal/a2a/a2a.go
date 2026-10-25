// Package a2a REQ-369/M167：A2A（Agent2Agent）协议端点暴露——平台智能体经 A2A 标准化对外，
// 外部 Agent 客户端可发现（Agent Card）/委派（message/send）/对话（contextId 多轮）。
// 协议面（v1 子集）：JSON-RPC 2.0 over HTTP——message/send（异步任务化：立即回 working 态 task，
// 后台跑完转 completed）/tasks/get（轮询）/tasks/cancel（chat.Stop 承接）；message/stream 与
// push notifications 不做（capabilities 如实标注 false）。Agent Card 按 A2A 规范形态
// （/.well-known/agent-card.json，每智能体一卡一端点）。
// 任务映射进程内（沿 ai-draft 口径：重启丢任务映射，tasks/get 诚实报 TaskNotFound）；
// contextId=平台会话 id——同一 contextId 连续 message/send 即多轮对话（平台历史承载）。
// 鉴权沿平台全局 accessAuth（PLATFORM_TOKEN 未设置=零开销透传，16 号 §11 口径）。
package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// Task A2A 任务（进程内生命周期：submitted→working→completed|failed|canceled）。
type Task struct {
	ID           string           `json:"id"`
	ContextID    string           `json:"contextId"` // =平台会话 id（多轮延续锚）
	AgentID      string           `json:"-"`
	State        string           `json:"state"`
	Timestamp    string           `json:"timestamp,omitempty"`
	ArtifactText string           `json:"-"`
	ErrorText    string           `json:"-"`
	History      []map[string]any `json:"-"`
}

// taskJSON A2A wire 形态（artifacts 内含最终文本产物）。
func (t *Task) taskJSON() map[string]any {
	out := map[string]any{
		"id":        t.ID,
		"contextId": t.ContextID,
		"status":    map[string]any{"state": t.State, "timestamp": t.Timestamp},
		"kind":      "task",
	}
	if t.History != nil {
		out["history"] = t.History
	}
	if t.ArtifactText != "" {
		out["artifacts"] = []map[string]any{{
			"taskId": t.ID,
			"parts":  []map[string]any{{"kind": "text", "text": t.ArtifactText}},
		}}
	}
	return out
}

// RunFunc 一次对话运行的可注入执行器（生产装配=chat.Run+伴生收尾+最终助手消息读取；
// 测试注入假执行器）。
type RunFunc func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error)

// Service A2A 端点服务。
type Service struct {
	Store   *store.Store
	BaseURL string // 卡片 url 对外基址（A2A_AGENT_BASE_URL，缺省 http://127.0.0.1:8080）
	RunFunc RunFunc

	// Stop 会话打断钩子（api 装配注入 chat.Stop——a2a 包不反向依赖 chat；nil=取消仅改状态）
	Stop func(conversationID string) bool
	// Busy 会话运行判据（api 装配注入 chat.IsRunning；nil=不设闸）——REQ-441/M237：
	// 同 contextId 并发 message/send 双跑防互踩（会话级单运行）。
	Busy func(conversationID string) bool
	// RunStreamFunc REQ-442/M238：流式执行器（message/stream 用）——onProgress 回调携带
	// 累积部分文本（服务端节流为对外快照）；nil=回落 RunFunc（仅首末快照）。
	RunStreamFunc func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string, onProgress func(partial string)) (string, error)
	// ResumeFunc REQ-443/M239：答案续跑执行器——同 contextId 再 send 且会话挂起 ask_human 时
	// input 作答走本执行器（api 装配注入 chat.Resume 管线）；nil=按新 Run 处理。
	ResumeFunc RunFunc
	mu         sync.Mutex
	tasks      map[string]*Task
}

// New 构造（始终可用；BaseURL 读 A2A_AGENT_BASE_URL）。
func New(st *store.Store) *Service {
	base := strings.TrimRight(os.Getenv("A2A_AGENT_BASE_URL"), "/")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	return &Service{Store: st, BaseURL: base, tasks: map[string]*Task{}}
}

// AgentCard A2A Agent Card（每智能体一卡；url=该智能体的 JSON-RPC 端点）。
func (s *Service) AgentCard(agent *store.Agent) map[string]any {
	desc := strings.TrimSpace(agent.Description)
	skillDesc := desc
	if skillDesc == "" {
		inst := strings.TrimSpace(agent.Instruction)
		if len(inst) > 160 {
			inst = inst[:160] + "…"
		}
		skillDesc = inst
	}
	return map[string]any{
		"name":            agent.Name,
		"description":     desc,
		"url":             fmt.Sprintf("%s/api/a2a/agents/%s", s.BaseURL, agent.ID),
		"version":         "0.1.0",
		"protocolVersion": "1.0",
		"capabilities": map[string]any{
			"streaming":              true, // REQ-442/M238：server 侧 message/stream SSE 已启用（与 client SendStream 对偶）
			"pushNotifications":      false,
			"stateTransitionHistory": false,
		},
		"defaultInputModes":  []string{"text/plain"},
		"defaultOutputModes": []string{"text/plain"},
		"skills": []map[string]any{{
			"id":          agent.ID,
			"name":        agent.Name,
			"description": skillDesc,
			"tags":        []string{"eino-multiagent-lab", "agent"},
		}},
		"preferredTransport": "JSONRPC",
	}
}

// ---- JSON-RPC ----

type rpcRequest struct {
	ID     any             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func rpcResult(id any, result any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
}
func rpcError(id any, code int, msg string) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}}
}

// A2A 规范错误码。
const (
	errTaskNotFound       = -32001
	errMethodNotSupported = -32601
	errInvalidParams      = -32602
	errTaskNotCancelable  = -32002
	errConversationBusy   = -32003 // REQ-441/M237：会话级单运行闸（同 contextId 并发 send 拒绝）
	errApprovalRequired   = -32004 // REQ-443/M239：审批档闸门（tool_approval≠off 对外面预拒绝）
)

// HandleRPC POST /api/a2a/agents/{id}——JSON-RPC 2.0 入口。
func (s *Service) HandleRPC(w http.ResponseWriter, r *http.Request, agentID string) {
	agent, err := s.Store.GetAgent(agentID)
	if err != nil || agent == nil {
		writeJSON(w, http.StatusNotFound, rpcError(nil, -32600, "智能体不存在: "+agentID))
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, rpcError(nil, -32700, "JSON 解析失败: "+err.Error()))
		return
	}
	switch req.Method {
	case "message/send":
		s.handleMessageSend(w, r, req, agent)
	case "message/stream":
		s.handleMessageStream(w, r, req, agent) // REQ-442/M238：SSE 流式应答
	case "tasks/get":
		s.handleTasksGet(w, req)
	case "tasks/cancel":
		s.handleTasksCancel(w, r, req)
	default:
		writeJSON(w, http.StatusOK, rpcError(req.ID, errMethodNotSupported, "方法未支持: "+req.Method))
	}
}

// handleMessageSend message/send：text parts 拼接为用户输入——contextId 存在且会话在=多轮延续，
// 否则新建平台会话；任务后台执行，立即回 working 态（外部客户端 tasks/get 轮询）。
func (s *Service) handleMessageSend(w http.ResponseWriter, r *http.Request, req rpcRequest, agent *store.Agent) {
	var p struct {
		Message struct {
			Role      string `json:"role"`
			Kind      string `json:"kind"`
			MessageID string `json:"messageId"`
			ContextID string `json:"contextId"`
			Parts     []struct {
				Kind string `json:"kind"`
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"message"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errInvalidParams, "params 解析失败: "+err.Error()))
		return
	}
	var b strings.Builder
	for _, part := range p.Message.Parts {
		if part.Kind == "text" || part.Kind == "" {
			b.WriteString(part.Text)
		}
	}
	input := strings.TrimSpace(b.String())
	if input == "" {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errInvalidParams, "message.parts 缺 text 文本"))
		return
	}
	// REQ-443/M239：审批档闸门——对外面无交互面，tool_approval≠off 预拒绝（沿 /mcp server
	// 模式先例；治「审批挂起等死 10min 按 failed 收场」）
	if agent.ToolApproval != "" && agent.ToolApproval != "off" {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errApprovalRequired, "该智能体开启工具审批（"+agent.ToolApproval+"），A2A 对外面无交互面不可委派（沿 /mcp 先例预拒绝）"))
		return
	}
	// contextId=平台会话 id：存在且归属本智能体则多轮延续，否则新建
	var conv *store.Conversation
	if cid := strings.TrimSpace(p.Message.ContextID); cid != "" {
		if c, gerr := s.Store.GetConversation(cid); gerr == nil && c != nil && c.Scope == "agent" && c.AgentID != nil && *c.AgentID == agent.ID {
			conv = c
		}
	}
	if conv == nil {
		c, cerr := s.Store.CreateConversation(&store.Conversation{
			Scope: "agent", AgentID: &agent.ID, Title: "[A2A] " + agent.Name,
		})
		if cerr != nil {
			writeJSON(w, http.StatusInternalServerError, rpcError(req.ID, -32603, "会话创建失败: "+cerr.Error()))
			return
		}
		conv = c
	}
	// REQ-441/M237：会话级单运行闸——同 contextId 并发 send 双跑互踩（chat.IsRunning 判据注入）
	if s.Busy != nil && s.Busy(conv.ID) {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errConversationBusy, "会话已有进行中的任务（A2A 单会话串行），请待终态后重发"))
		return
	}
	// REQ-443/M239：会话挂起 ask_human 时 input 作答——走 ResumeFunc 续跑（chat.Resume 管线）
	resuming := s.ResumeFunc != nil && s.pendingHumanQuestion(conv.ID) != ""
	task := &Task{
		ID: store.NewID(), ContextID: conv.ID, AgentID: agent.ID,
		State: "working", Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	s.mu.Lock()
	s.tasks[task.ID] = task
	s.mu.Unlock()
	s.persistTask(task) // REQ-387/M183：任务写穿落库（重启 tasks/get 可查）

	// 后台执行（外部客户端 tasks/get 轮询；沿 ai-draft 202 异步口径）——ctx 脱离请求生命周期
	//（message/send 响应返回后请求 ctx 即取消，任务必须独立存活）
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		var text string
		var rerr error
		if resuming {
			text, rerr = s.ResumeFunc(ctx, conv, agent, store.NewID(), input)
		} else {
			text, rerr = s.RunFunc(ctx, conv, agent, store.NewID(), input)
		}
		s.mu.Lock()
		cur, ok := s.tasks[task.ID]
		s.mu.Unlock()
		if !ok {
			return
		}
		// REQ-441/M237：canceled 为终态——后台完成不得覆写（调用方已收 canceled 应答，对外契约自洽）
		if cur.State == "canceled" {
			return
		}
		cur.Timestamp = time.Now().UTC().Format(time.RFC3339)
		switch {
		case rerr != nil:
			cur.State, cur.ErrorText = "failed", rerr.Error()
		default:
			// REQ-443/M239：ask_human 挂起→input-required 终态（问题文本外发——外部调用方
			// 可同 contextId 携答案再委派续跑；A2A 协议语义本为此设计）
			if q := s.pendingHumanQuestion(conv.ID); q != "" {
				cur.State, cur.ErrorText = "input-required", q
			} else {
				cur.State, cur.ArtifactText = "completed", text
				cur.History = []map[string]any{
					{"role": "user", "parts": []map[string]any{{"kind": "text", "text": input}}, "kind": "message"},
					{"role": "agent", "parts": []map[string]any{{"kind": "text", "text": text}}, "kind": "message"},
				}
			}
		}
		s.persistTask(cur) // REQ-387/M183：终态写穿
	}()
	writeJSON(w, http.StatusOK, rpcResult(req.ID, task.taskJSON()))
}

// handleMessageStream REQ-442/M238：message/stream——SSE 流式应答（与 client SendStream wire
// 对偶：每条 data: 行=JSON-RPC 响应 envelope，result=任务快照）。运行**内联**于请求生命周期
// （客户端断连即 r.Context() 取消——与 message/send 的后台执行相反）：初始 working 快照 →
// RunStreamFunc onProgress 累积文本的过程快照（500ms 节流）→ 终态快照；canceled 守则沿 REQ-441。
func (s *Service) handleMessageStream(w http.ResponseWriter, r *http.Request, req rpcRequest, agent *store.Agent) {
	var p struct {
		Message struct {
			Role      string `json:"role"`
			Kind      string `json:"kind"`
			MessageID string `json:"messageId"`
			ContextID string `json:"contextId"`
			Parts     []struct {
				Kind string `json:"kind"`
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"message"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errInvalidParams, "params 解析失败: "+err.Error()))
		return
	}
	var b strings.Builder
	for _, part := range p.Message.Parts {
		if part.Kind == "text" || part.Kind == "" {
			b.WriteString(part.Text)
		}
	}
	input := strings.TrimSpace(b.String())
	if input == "" {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errInvalidParams, "message.parts 缺 text 文本"))
		return
	}
	// REQ-443/M239：审批档闸门（同 message/send——沿 /mcp 先例预拒绝）
	if agent.ToolApproval != "" && agent.ToolApproval != "off" {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errApprovalRequired, "该智能体开启工具审批（"+agent.ToolApproval+"），A2A 对外面无交互面不可委派（沿 /mcp 先例预拒绝）"))
		return
	}
	var conv *store.Conversation
	if cid := strings.TrimSpace(p.Message.ContextID); cid != "" {
		if c, gerr := s.Store.GetConversation(cid); gerr == nil && c != nil && c.Scope == "agent" && c.AgentID != nil && *c.AgentID == agent.ID {
			conv = c
		}
	}
	if conv == nil {
		c, cerr := s.Store.CreateConversation(&store.Conversation{
			Scope: "agent", AgentID: &agent.ID, Title: "[A2A] " + agent.Name,
		})
		if cerr != nil {
			writeJSON(w, http.StatusInternalServerError, rpcError(req.ID, -32603, "会话创建失败: "+cerr.Error()))
			return
		}
		conv = c
	}
	if s.Busy != nil && s.Busy(conv.ID) {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errConversationBusy, "会话已有进行中的任务（A2A 单会话串行），请待终态后重发"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusOK, rpcError(req.ID, -32603, "传输层不支持流式（无 Flusher）"))
		return
	}
	task := &Task{
		ID: store.NewID(), ContextID: conv.ID, AgentID: agent.ID,
		State: "working", Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	s.mu.Lock()
	s.tasks[task.ID] = task
	s.mu.Unlock()
	s.persistTask(task)

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	snap := func(t *Task) {
		data, _ := json.Marshal(rpcResult(req.ID, t.taskJSON()))
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	snap(task)

	// 过程增量快照（节流 500ms；仅 working 态透出，终态由收尾统一发）
	var lastEmit time.Time
	progress := func(partial string) {
		now := time.Now()
		if now.Sub(lastEmit) < 500*time.Millisecond {
			return
		}
		lastEmit = now
		s.mu.Lock()
		if task.State == "working" {
			task.ArtifactText = partial
		}
		cur := *task
		s.mu.Unlock()
		snap(&cur)
	}

	ctx := r.Context() // 客户端断连即取消
	var text string
	var rerr error
	resuming := s.ResumeFunc != nil && s.pendingHumanQuestion(conv.ID) != "" // REQ-443/M239：携答案续跑
	switch {
	case resuming:
		text, rerr = s.ResumeFunc(ctx, conv, agent, store.NewID(), input)
	case s.RunStreamFunc != nil:
		text, rerr = s.RunStreamFunc(ctx, conv, agent, store.NewID(), input, progress)
	default:
		text, rerr = s.RunFunc(ctx, conv, agent, store.NewID(), input)
	}
	s.mu.Lock()
	if task.State == "canceled" { // tasks/cancel 并发路径已落库——终态不被覆写（REQ-441 同守则）
		cur := *task
		s.mu.Unlock()
		snap(&cur)
		return
	}
	task.Timestamp = time.Now().UTC().Format(time.RFC3339)
	switch {
	case rerr != nil:
		task.State, task.ErrorText = "failed", rerr.Error()
	default:
		if q := s.pendingHumanQuestion(conv.ID); q != "" { // REQ-443/M239：ask_human 挂起→input-required
			task.State, task.ErrorText = "input-required", q
		} else {
			task.State, task.ArtifactText = "completed", text
			task.History = []map[string]any{
				{"role": "user", "parts": []map[string]any{{"kind": "text", "text": input}}, "kind": "message"},
				{"role": "agent", "parts": []map[string]any{{"kind": "text", "text": text}}, "kind": "message"},
			}
		}
	}
	cur := *task
	s.mu.Unlock()
	s.persistTask(task)
	snap(&cur)
}

// pendingHumanQuestion REQ-443/M239：会话中断态中的 ask_human 追问文本（""=无挂起）——
// input-required 生产与答案续跑判定的共用判据。
func (s *Service) pendingHumanQuestion(convID string) string {
	c, err := s.Store.GetConversation(convID)
	if err != nil || c == nil || strings.TrimSpace(c.InterruptState) == "" {
		return ""
	}
	var st struct {
		Kind     string   `json:"kind"`
		Question string   `json:"question"`
		Choices  []string `json:"choices"`
	}
	if json.Unmarshal([]byte(c.InterruptState), &st) != nil || st.Kind != "ask_human" {
		return ""
	}
	q := st.Question
	if len(st.Choices) > 0 {
		q += "（可选：" + strings.Join(st.Choices, " / ") + "）"
	}
	return q
}

// persistTask REQ-387/M183：任务写穿落库（旁路失败仅日志不阻断响应）。
func (s *Service) persistTask(t *Task) {
	if s.Store == nil {
		return
	}
	if err := s.Store.UpsertA2ATask(&store.A2ATaskRow{
		ID: t.ID, ContextID: t.ContextID, AgentID: t.AgentID, State: t.State, Timestamp: t.Timestamp,
		Payload: store.MarshalA2APayload(store.A2APayload{ArtifactText: t.ArtifactText, ErrorText: t.ErrorText, History: t.History}),
	}); err != nil {
		log.Printf("[a2a] 任务落库失败 %s: %v", t.ID, err)
	}
}

// handleTasksGet tasks/get：任务快照（终态附 artifacts+history）。
func (s *Service) handleTasksGet(w http.ResponseWriter, req rpcRequest) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil || strings.TrimSpace(p.ID) == "" {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errInvalidParams, "tasks/get 需 id"))
		return
	}
	s.mu.Lock()
	t, ok := s.tasks[p.ID]
	s.mu.Unlock()
	if !ok {
		// REQ-387/M183：进程内未命中回读 DB（重启后终态任务可查）
		if row, gerr := s.Store.GetA2ATask(p.ID); gerr == nil && row != nil {
			var pl store.A2APayload
			_ = json.Unmarshal([]byte(row.Payload), &pl)
			t = &Task{ID: row.ID, ContextID: row.ContextID, AgentID: row.AgentID,
				State: row.State, Timestamp: row.Timestamp,
				ArtifactText: pl.ArtifactText, ErrorText: pl.ErrorText, History: pl.History}
			s.mu.Lock()
			s.tasks[t.ID] = t
			s.mu.Unlock()
		}
	}
	if t == nil {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errTaskNotFound, "task 不存在（或服务重启丢进程内任务映射——请重新委派）"))
		return
	}
	writeJSON(w, http.StatusOK, rpcResult(req.ID, t.taskJSON()))
}

// handleTasksCancel tasks/cancel：working 态经 chat.Stop 打断→canceled；终态任务回 TaskNotCancelable。
func (s *Service) handleTasksCancel(w http.ResponseWriter, r *http.Request, req rpcRequest) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil || strings.TrimSpace(p.ID) == "" {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errInvalidParams, "tasks/cancel 需 id"))
		return
	}
	s.mu.Lock()
	t, ok := s.tasks[p.ID]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errTaskNotFound, "task 不存在（或服务重启丢进程内任务映射）"))
		return
	}
	if t.State != "working" && t.State != "submitted" {
		writeJSON(w, http.StatusOK, rpcError(req.ID, errTaskNotCancelable, "task 已终态（"+t.State+"），不可取消"))
		return
	}
	_ = r // chat.Stop 以会话为粒度打断（api 装配注入；未注入=任务跑完自然终态）
	if s.Stop != nil {
		s.Stop(t.ContextID)
	}
	s.mu.Lock()
	t.State = "canceled"
	t.Timestamp = time.Now().UTC().Format(time.RFC3339)
	s.mu.Unlock()
	s.persistTask(t) // REQ-441/M237：cancel 写穿（重启对账不再 working→failed 语义漂移）
	writeJSON(w, http.StatusOK, rpcResult(req.ID, t.taskJSON()))
}

// ---- 共用 ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// AgentsExposed 平台级发现面（非 A2A 规范件）：可被委派的智能体清单+各自卡片地址。
// REQ-434/M230：扩成员目录透出——外部成员（kind=cli/a2a 连接器授权面）与协作拓扑元数据
// 一并透出（外部智能体反向发现平台资产；与 ANS 类通用 registry 区隔=仅平台自身端点增强）。
func (s *Service) AgentsExposed(w http.ResponseWriter, _ *http.Request) {
	agents, err := s.Store.ListAgents()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(agents))
	externalMembers := make([]map[string]any, 0)
	topology := map[string]any{}
	for _, a := range agents {
		out = append(out, map[string]any{
			"id": a.ID, "name": a.Name, "is_builtin": a.IsBuiltin,
			"card_url": fmt.Sprintf("%s/api/a2a/agents/%s/.well-known/agent-card.json", s.BaseURL, a.ID),
			"endpoint": fmt.Sprintf("%s/api/a2a/agents/%s", s.BaseURL, a.ID),
		})
		// REQ-434：成员目录——外部成员（连接器授权）随智能体透出（kind/名称/端点元数据；
		// 凭据/配置细节不透出——只读目录形态）
		for _, cid := range a.Connectors {
			c, cerr := s.Store.GetConnector(cid)
			if cerr != nil || c == nil {
				continue
			}
			if c.Kind != "cli" && c.Kind != "a2a" {
				continue
			}
			entry := map[string]any{
				"agent_id": a.ID, "agent_name": a.Name,
				"connector_id": c.ID, "connector_name": c.Name,
				"kind": c.Kind, "status": c.Status,
			}
			if c.Kind == "a2a" {
				if u, ok := c.Config["url"].(string); ok {
					entry["endpoint"] = u
				}
			} else {
				if b, ok := c.Config["backend"].(string); ok {
					entry["backend"] = b
				}
			}
			externalMembers = append(externalMembers, entry)
		}
		// REQ-434：协作拓扑元数据（协调者+成员清单——反向发现「该智能体在哪个团队」）
		projects, perr := s.Store.ListProjects()
		if perr == nil {
			topoList := make([]map[string]any, 0)
			for _, pr := range projects {
				members, merr := s.Store.ListProjectAgents(pr.ID)
				if merr != nil {
					continue
				}
				for _, pm := range members {
					if pm.AgentID == a.ID {
						entry := map[string]any{"project_id": pr.ID, "project_name": pr.Name, "role": pm.Role}
						topoList = append(topoList, entry)
						break
					}
				}
			}
			if len(topoList) > 0 {
				topology[a.ID] = topoList
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"agents": out, "count": len(out),
		"protocol": "A2A v1.0（JSON-RPC 子集，REQ-369）",
		// REQ-434/M230：成员目录与拓扑透出（只读；凭据/配置细节不透出）
		"external_members": externalMembers,
		"external_count":   len(externalMembers),
		"topology":         topology,
	})
}

// HandleCard GET /api/a2a/agents/{id}/.well-known/agent-card.json。
func (s *Service) HandleCard(w http.ResponseWriter, _ *http.Request, agentID string) {
	agent, err := s.Store.GetAgent(agentID)
	if err != nil || agent == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "智能体不存在: " + agentID})
		return
	}
	writeJSON(w, http.StatusOK, s.AgentCard(agent))
}
