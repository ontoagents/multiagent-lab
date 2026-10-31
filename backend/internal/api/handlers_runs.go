package api

import (
	"context"
	"strconv"
	"time"

	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/ontoagents/ontoagents/backend/internal/api/sse"
	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// resolveRunTarget 解析会话的运行目标 Agent（agent 直聊 / 项目协调者），返回会话与智能体。
func (s *Server) resolveRunTarget(r *http.Request) (*store.Conversation, *store.Agent, string, error) {
	conv, err := s.Store.GetConversation(r.PathValue("id"))
	if err != nil {
		return nil, nil, "", err
	}
	if conv.Scope == "agent" {
		if conv.AgentID == nil {
			return nil, nil, "", &store.HTTPError{Status: 400, Msg: "conversation is not bound to an agent"}
		}
		agent, err := s.Store.GetAgent(*conv.AgentID)
		if err != nil {
			return nil, nil, "", err
		}
		return conv, agent, "", nil
	}
	// 项目会话：解析运行成员——主智能体优先，缺省取第一个成员
	if conv.ProjectID == nil || *conv.ProjectID == "" {
		return nil, nil, "", &store.HTTPError{Status: 400, Msg: "conversation is not bound to a project"}
	}
	p, err := s.Store.GetProject(*conv.ProjectID)
	if err != nil {
		return nil, nil, "", err
	}
	agentID := p.Coordinator
	if agentID == "" && len(p.AgentIDs) > 0 {
		agentID = p.AgentIDs[0]
	}
	if agentID == "" {
		return nil, nil, "", &store.HTTPError{Status: 400, Msg: "项目还没有成员智能体，请先在项目配置中添加成员"}
	}
	agent, err := s.Store.GetAgent(agentID)
	if err != nil {
		return nil, nil, "", err
	}
	return conv, agent, p.Name, nil
}

// runConversation 发起一次运行（SSE 流式返回平台事件，方案 §7 统一协议）。
func (s *Server) runConversation(w http.ResponseWriter, r *http.Request) {
	var in chat.RunInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	conv, agent, projectName, err := s.resolveRunTarget(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	// REQ-291：会话级模型覆盖（conversation.model_conn_id，''=跟随）先落基座；REQ-174 请求级单路
	// 覆盖在其后应用故仍最高。对比模式窗格 ''（继承）经基座继承会话级，窗格显式选择不变最高。
	agent = chat.ApplyConversationModelOverride(conv, agent)
	// REQ-174：单路模型覆盖（对话输入区模型快捷切换逐次下发；对比模式走窗格级覆盖，此处不介入）
	if in.ModelConnID != "" && len(in.Panes) < 2 {
		agent = chat.WithModelOverride(agent, in.ModelConnID)
	}

	sw, err := sse.NewWriter(w)
	if err != nil {
		writeErr(w, err)
		return
	}
	runID := store.NewID()

	// REQ-19e/19f：对比模式 N 路事件并发写同一条 SSE 流，http.ResponseWriter 非并发安全——统一串行化
	var swMu sync.Mutex
	writeEvent := func(event string, data any) {
		swMu.Lock()
		defer swMu.Unlock()
		_ = sw.Event(event, data)
	}

	emit := func(ev *chat.Event) {
		if ev == nil {
			return
		}
		writeEvent(ev.Type, ev)
	}

	// REQ-19e/19f 对比模式：一次提问 N 路（2~4 窗格）——meta 附窗格 run_id 映射，
	// 前端按事件 run_id 路由到对应窗格（SSE 契约与单路一致）。
	if len(in.Panes) >= 2 {
		paneIDs := make([]string, len(in.Panes))
		panes := make([]map[string]any, len(in.Panes))
		for i, pc := range in.Panes {
			paneIDs[i] = store.NewID()
			panes[i] = map[string]any{
				"pane": i, "run_id": paneIDs[i],
				"model_conn_id": pc.ModelConnID, "kb_id": pc.KBID, "runtime_profile_id": pc.RuntimeProfileID,
			}
		}
		meta := map[string]any{"run_id": runID, "conversation_id": conv.ID, "agent_name": agent.Name, "compare": true, "panes": panes}
		if projectName != "" {
			meta["project_name"] = projectName
		}
		writeEvent("meta", meta)
		if _, err := s.Chat.RunCompare(r.Context(), conv, agent, runID, paneIDs, in, emit); err != nil {
			ev := chat.NewErrorEvent(runID, "run_failed", err.Error())
			writeEvent(ev.Type, ev)
		}
		return
	}

	// 元信息事件（供前端校验会话与运行归属）
	meta := map[string]any{"run_id": runID, "conversation_id": conv.ID, "agent_name": agent.Name}
	if projectName != "" {
		meta["project_name"] = projectName
	}
	writeEvent("meta", meta)

	// REQ-527/M334（P16）：运行 ctx 与请求 ctx 解耦——r.Context() 在客户端断连（刷新/断网/
	// 合盖）即取消整场运行，与 C3 LongRun 主打能力直接相悖。detached ctx（Background 派生+
	// 30min 超时护栏防泄漏）：断连后 SSE 写失效但不取消运行，事件照常落库——刷新后经
	// GET /api/conversations/{id}/events?run_id= 回放续看（run_event 逐条落库基础设施现成）。
	// Stop 语义不变（cancels 表显式取消）。对比模式维持请求 ctx（组级取消语义保持）。
	runCtx, runCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer runCancel()
	res, runErr := s.Chat.Run(runCtx, conv, agent, runID, in.Input, in.DebugLevel, in.DebugPersist, emit)
	if runErr != nil {
		ev := chat.NewErrorEvent(runID, "run_failed", runErr.Error())
		writeEvent(ev.Type, ev)
	}
	if res != nil {
		s.Chat.RecordOntologyCited(conv.ID, runID, res.AssistantMessageID) // REQ-530/M337：cited 归因收尾
	}
	// REQ-170/M28：伴生本体收尾触发（旁路 goroutine；开关关/未绑定 Agent 静默返回，不影响 SSE 收尾）
	s.Companion.OnRunComplete(conv, agent, runID)
	s.Memory.OnRunComplete(conv, agent, runID) // REQ-551/M358：记忆候选生成（无工作区会话静默短路）
	// REQ-228①：召回消费追踪（本次 run 的 companion 命中实体是否在回答中出现 → companion.hit 事件）
	s.Companion.RecordHits(conv.ID, runID)
}

// runEvents REQ-527/M334（P16 后半）：事件回放——run_id 过滤升序（run_event 逐条落库现成）。
// 断连/刷新后按此轮询续看推进中运行（offset 增量取新事件）；配合运行 ctx 解耦构成
// 「任务仍推进+可重挂续看」闭环（live SSE 重挂以轮询承载——单用户轮询开销可忽略）。
func (s *Server) runEvents(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	q := store.EventQuery{
		RunID: r.URL.Query().Get("run_id"),
		Order: "asc",
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n >= 0 {
			q.Offset = n
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 && n <= 500 {
			q.Limit = n
		}
	}
	if q.Offset > 0 && q.Limit == 0 {
		q.Limit = 500 // OFFSET 须配 LIMIT 才拼接（ListEventsQ 语义）——增量轮询默认窗
	}
	events, total, err := s.Store.ListEventsQ(convID, q)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "total": total})
}

// stopConversation 停止运行。
func (s *Server) stopConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok := s.Chat.Stop(id)
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": ok})
}

// steerConversation REQ-385/M181：主会话运行中介入（steer）——插话经 TurnLoop.Push(WithPreempt)
// 在安全点打断当前轮、下一轮以全量历史（含插话）接续。无进行中 TurnLoop 运行 → 409
// （引导走常规新消息运行）；引擎关闭（CHAT_TURNLOOP=0）同样 409（无抢占通道）。
func (s *Server) steerConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Message string `json:"message"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Message == "" {
		writeErr(w, errors.New("message 不能为空"))
		return
	}
	ok, err := s.Chat.Steer(id, in.Message)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "该会话当前无 TurnLoop 运行（引擎关闭或已收束）——请走常规新消息", "steered": false})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"steered": true, "preempt": "safe_point"})
}

// extApproval REQ-206/M41 S2：外部推理后端（deepseek-harness-acp）权限转审批的人工决议。
// body {decision: "approve"|"deny"}；approve 映射首个 allow 选项，deny/无待决 → cancelled/409。
// 与 in-process resume（checkpoint 挂起恢复）不同：外部通道的权限挂起活在当前 run 内，
// 决议即时生效（SSE 保持打开，turn 继续）。
func (s *Server) extApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Decision string `json:"decision"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Decision != "approve" && in.Decision != "deny" {
		writeErr(w, errors.New("decision 仅支持 approve|deny"))
		return
	}
	if err := s.Chat.ResolveExtApproval(id, in.Decision); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "decision": in.Decision})
}

// resumeConversation 恢复挂起的中断（M11 收尾 · 中断恢复）：以用户答复定向续跑，SSE 事件与运行同构。
func (s *Server) resumeConversation(w http.ResponseWriter, r *http.Request) {
	var in chat.RunInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	conv, agent, projectName, err := s.resolveRunTarget(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	// REQ-291：恢复路径同样吃会话级模型覆盖（请求级覆盖恢复语义维持不带，沿 REQ-174 现状）
	agent = chat.ApplyConversationModelOverride(conv, agent)
	sw, err := sse.NewWriter(w)
	if err != nil {
		writeErr(w, err)
		return
	}
	runID := store.NewID()
	meta := map[string]any{"run_id": runID, "conversation_id": conv.ID, "agent_name": agent.Name, "resumed": true}
	if projectName != "" {
		meta["project_name"] = projectName
	}
	_ = sw.Event("meta", meta)
	emit := func(ev *chat.Event) {
		if ev == nil {
			return
		}
		_ = sw.Event(ev.Type, ev)
	}
	if _, err := s.Chat.Resume(r.Context(), conv, agent, runID, in.Input, in.DebugLevel, in.DebugPersist, emit); err != nil {
		ev := chat.NewErrorEvent(runID, "resume_failed", err.Error())
		_ = sw.Event(ev.Type, ev)
	}
	// REQ-170/M28：恢复收尾同样触发伴生抽取（方案：Run/Resume 收尾事件触发）
	s.Companion.OnRunComplete(conv, agent, runID)
	s.Memory.OnRunComplete(conv, agent, runID) // REQ-551/M358
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
