package chat

// external.go M13/D-O13（方案 §6.16）：外部推理后端执行路径。
//
// 外部 CLI（claude-code/opencode/aider）不走平台内 eino-adk 管线：无消息列表、无原生工具，
// 以单条组装提示运行子进程。能力降级语义（§6.16.4）：
//   - 对话/流式：✅（CLI stdout 行级转译为 message.delta / tool.call）
//   - 技能：→ instruction 注入 ⚠️
//   - MCP 工具：→ prompt 段落注入 ⚠️（仅告知存在，无法调用）
//   - AgentAsTool / 工作流：❌（P1 不暴露）
//   - 中断恢复：⚠️ 仅 Cancel（终止子进程）
// 模型连接（agent.model_conn_id）对外部后端不生效：模型由 CLI 自身配置决定。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/ontoagents/ontoagents/backend/internal/inference"
	"github.com/ontoagents/ontoagents/backend/internal/store"
	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

// historyLimit 外部提示中携带的历史轮数上限（CLI 无服务端记忆，单次拼入）。
const historyLimit = 10

// ErrNoPendingExtApproval REQ-206/M41 S2：无待决的外部权限请求（决议端点 409）。
var ErrNoPendingExtApproval = errors.New("该会话没有待决的外部权限请求")

// runExternal 外部推理后端一次运行的完整编排（镜像 Run 的 inprocess 路径骨架）。
func (s *Service) runExternal(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string, debug int, emit EmitFn) (*RunResult, error) {
	name := agent.InferenceBackend
	b := s.Inference.Get(name)
	if b == nil {
		err := fmt.Errorf("未知推理后端 %q", name)
		s.emitAndRecord(ctx, conv, runID, newEvent("run.error", runID, map[string]any{"code": "backend_not_found", "message": err.Error()}), emit)
		return nil, err
	}

	res := &RunResult{}

	// 1) 持久化用户消息 + 历史
	userMsg := &store.Message{ConversationID: conv.ID, Role: "user", Content: input}
	if _, err := s.Store.InsertMessage(userMsg); err != nil {
		return nil, fmt.Errorf("save user message: %w", err)
	}
	history, err := s.Store.ListMessages(conv.ID)
	if err != nil {
		return nil, err
	}
	histMsgs := BuildHistoryMessages(history)

	// 2) 可取消 ctx（Stop 复用 cancels 表 → 终止 CLI 子进程）
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancels[conv.ID] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.cancels, conv.ID)
		s.mu.Unlock()
		cancel()
	}()

	// 3) 降级注入内容：技能 instruction + MCP 段落 + run.started（backend/版本/降级警告）
	skills := s.loadAgentSkills(agent)
	caps := b.Capabilities()
	warnings := degradationWarnings(caps, len(skills), len(agent.MCPServers))

	start := time.Now()
	startData := map[string]any{
		"conversation_id": conv.ID,
		"agent_name":      agent.Name,
		"model":           fmt.Sprintf("%s（由 %s 自身配置决定）", name, name),
		"backend":         name,
	}
	// 生效审批档透出（与 in-process run.started 同语义；REQ-206 S2 起对 deepseek-harness-acp 实际生效）
	if apMode, apSrc := effectiveApprovalPolicy(agent, conv); apMode != "" || apSrc != "agent" {
		startData["tool_approval"] = map[string]any{"mode": apMode, "source": apSrc}
	}
	if len(warnings) > 0 {
		startData["warnings"] = warnings
	}
	s.emitAndRecord(runCtx, conv, runID, newEvent("run.started", runID, startData), emit)

	if len(skills) > 0 {
		sk := make([]map[string]string, 0, len(skills))
		for _, k := range skills {
			sk = append(sk, map[string]string{"id": k.ID, "name": k.Name})
		}
		s.emitAndRecord(runCtx, conv, runID, newEvent("skill.loaded", runID, map[string]any{"skills": sk}), emit)
	}

	// 4) 知识库召回（公共段；命中内容以 System 消息挂在 histMsgs 尾部）
	// baseLen 含本轮刚插入的用户输入（列表末位）——历史段取 prior = 剔除本轮输入
	//（顺修既有拼接把本轮输入与「## 用户输入」重复装配的缝隙）。
	baseLen := len(histMsgs)
	prior := histMsgs[:baseLen-1]
	histMsgs = s.recallKB(runCtx, conv, agent, runID, input, histMsgs, emit)
	// 伴生图检索源并入（REQ-170 P2：外部推理后端同口径；注入文本并入单条提示上下文）
	histMsgs = s.recallCompanion(runCtx, conv, agent, runID, input, histMsgs, emit)
	var kbCtx []string
	for _, m := range histMsgs[baseLen:] {
		if m.Role == schema.System {
			kbCtx = append(kbCtx, m.Content)
		}
	}

	// 5) 组装提示并执行（事件经转译回调流出并落库）
	req := &inference.RunRequest{UserInput: input, Debug: debug, SessionID: conv.ID}
	if caps.Resume {
		// REQ-206/M41 S2 会话延续：有前轮消息先试 session/resume 复用 dsh 持久化会话；
		// resumed=true 时历史去重（dsh 会话内已有前轮），false（首轮或回退）拼全量历史。
		req.AttemptResume = len(prior) > 0
		req.BuildPrompt = func(resumed bool) string {
			hist := prior
			if resumed {
				hist = nil
			}
			return assembleExternalPrompt(agent, skills, agent.MCPServers, kbCtx, hist, input)
		}
	} else {
		req.Prompt = assembleExternalPrompt(agent, skills, agent.MCPServers, kbCtx, prior, input)
	}
	if caps.MCPMode == "tools" {
		// REQ-206/M41 S2 MCP 挂载翻正：连接器实例 URL/headers 注入 session/new（dsh 直连）。
		// 每引用先做一次可达性预检（dsh 对初始连接失败回滚整个 session/new——不可达引用
		// 直接剔除并 connector.degraded，与 in-process 装配降级同口径）。
		req.MCPServers = s.externalMCPRefs(runCtx, conv, agent, runID, emit)
	}
	if policy, _ := effectiveApprovalPolicy(agent, conv); policy == "" {
		// 审批关：权限请求自动放行（首个 allow_once 选项；无 allow 选项时保守拒绝）
		req.Permission = func(pr inference.PermissionRequest) inference.PermissionDecision {
			for _, o := range pr.Options {
				if o.Kind == "allow_once" || o.Kind == "allow_always" {
					return inference.PermissionDecision{Outcome: "selected", OptionID: o.OptionID}
				}
			}
			return inference.PermissionDecision{Outcome: "cancelled"}
		}
	} else {
		// all/danger 转平台审批挂起（danger 档外部工具无法按平台危险清单分类，保守同 all：
		// 逐请求挂起询问；v1 无跨请求记忆，allow_always 不落盘——诚实边界）
		req.Permission = func(pr inference.PermissionRequest) inference.PermissionDecision {
			return s.waitExtApproval(runCtx, conv, agent, runID, pr, emit)
		}
	}
	var buf strings.Builder
	adaptEmit := func(ev inference.Event) {
		if ev.Type == "message.delta" {
			if d, ok := ev.Data["delta"].(string); ok {
				buf.WriteString(d)
			}
		}
		s.emitAndRecord(runCtx, conv, runID, newEvent(ev.Type, runID, ev.Data), emit)
	}
	err = b.Run(runCtx, req, adaptEmit)

	// 6) 收尾：取消/错误/正常 → assistant 落库 + run.finished/run.error
	if err != nil {
		if errors.Is(err, context.Canceled) {
			res.Stopped = true
		} else {
			res.Error = err.Error()
			s.emitAndRecord(runCtx, conv, runID, newEvent("run.error", runID, map[string]any{"code": "backend_error", "message": err.Error()}), emit)
			return res, err
		}
	}
	if text := strings.TrimSpace(buf.String()); text != "" {
		asg := &store.Message{ConversationID: conv.ID, Role: "assistant", Content: text}
		if m, ierr := s.Store.InsertMessage(asg); ierr == nil {
			res.AssistantMessageID = m.ID
		}
	}
	finish := "stop"
	if res.Stopped {
		finish = "stopped"
	}
	s.emitAndRecord(runCtx, conv, runID, newEvent("run.finished", runID, map[string]any{
		"elapsed_ms":    time.Since(start).Milliseconds(),
		"finish_reason": finish,
	}), emit)
	return res, nil
}

// loadAgentSkills 取 agent 挂载且启用的技能（降级为 instruction 注入的数据源）。
func (s *Service) loadAgentSkills(agent *store.Agent) []*store.Skill {
	var out []*store.Skill
	for _, id := range agent.Skills {
		sk, err := s.Store.GetSkill(id)
		if err != nil || sk == nil || !sk.Enabled {
			continue
		}
		if strings.TrimSpace(sk.Instruction) == "" {
			continue
		}
		out = append(out, sk)
	}
	return out
}

// degradationWarnings §6.16.4 能力降级提示（随 run.started.warnings 事件下发）。
func degradationWarnings(caps inference.Capabilities, skillN, mcpN int) []string {
	var w []string
	if skillN > 0 && caps.SkillsMode == "instruction" {
		w = append(w, fmt.Sprintf("外部推理后端不执行平台技能工具：已将 %d 个挂载技能降级为 instruction 注入", skillN))
	}
	if mcpN > 0 && caps.MCPMode == "instruction" {
		w = append(w, fmt.Sprintf("外部推理后端不可调用 MCP 工具：%d 个 MCP 服务仅作为说明段落注入提示", mcpN))
	}
	if !caps.AgentAsTool {
		w = append(w, "外部推理后端不支持多 Agent 编排（AgentAsTool/transfer），本次仅单 Agent 对话")
	}
	if !caps.Workflow {
		w = append(w, "外部推理后端不支持工作流编排")
	}
	if !caps.Resume {
		w = append(w, "外部推理后端仅支持中断（停止），不支持断点恢复")
	}
	w = append(w, "外部推理后端的模型由 CLI 自身配置决定，Agent 模型连接不生效")
	return w
}

// assembleExternalPrompt 组装外部后端单条提示：指令 + 技能/MCP 注入 + 知识库上下文 + 历史 + 用户输入。
func assembleExternalPrompt(agent *store.Agent, skills []*store.Skill, mcps []store.MCPServer, kbCtx []string, history []*schema.Message, input string) string {
	var b strings.Builder
	if ins := strings.TrimSpace(agent.Instruction); ins != "" {
		b.WriteString("## 系统指令\n\n" + ins + "\n\n")
	}
	if len(skills) > 0 {
		b.WriteString("## 已挂载技能（以说明形式注入，非工具调用）\n\n")
		for _, sk := range skills {
			b.WriteString("### 技能：" + sk.Name + "\n" + strings.TrimSpace(sk.Instruction) + "\n\n")
		}
	}
	if len(mcps) > 0 {
		b.WriteString("## 可用 MCP 服务（仅说明，本次无法调用）\n\n")
		for _, m := range mcps {
			b.WriteString("- " + m.Name + "：" + m.URL + "\n")
		}
		b.WriteString("\n")
	}
	for _, c := range kbCtx {
		b.WriteString(c + "\n\n")
	}
	if len(history) > historyLimit {
		history = history[len(history)-historyLimit:]
	}
	if len(history) > 0 {
		b.WriteString("## 对话历史（较新在前/在后按时间序）\n\n")
		for _, m := range history {
			switch m.Role {
			case schema.User:
				b.WriteString("用户：" + m.Content + "\n")
			case schema.Assistant:
				b.WriteString("助手：" + m.Content + "\n")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("## 用户输入\n\n" + input)
	return b.String()
}

// externalMCPRefs REQ-206/M41 S2 MCP 挂载翻正：解析 agent 授权连接器为 ACP 会话级 MCP 引用
// （HTTP 条目：mcp 直通走 config.url+认证头；kubernetes/ssh 走平台托管插件服务端点）。
// 与 assembleTools 装配同防线的两类拒绝：本平台 /mcp 自引用、mcp 直通指向插件服务（绕过授权）。
// 逐引用可达性预检（FetchMCPToolsWithHeaders 同一握手）：dsh 对初始连接失败回滚整个 session/new，
// 不可达引用剔除并 connector.degraded（与 in-process 装配降级同口径），不阻断运行。
func (s *Service) externalMCPRefs(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID string, emit EmitFn) []inference.MCPServerRef {
	asm := s.Assembler
	if asm == nil || asm.Store == nil {
		return nil
	}
	pluginBase := strings.TrimRight(asm.PluginEndpoint, "/")
	refs := make([]inference.MCPServerRef, 0, len(agent.Connectors))
	for _, id := range agent.Connectors {
		c, cerr := asm.Store.GetConnector(id)
		if cerr != nil {
			continue
		}
		var target string
		var headers map[string]string
		switch c.Kind {
		case store.ConnectorKindMCP:
			target, _ = c.Config["url"].(string)
			headers = asm.mcpHeaders(c)
		case store.ConnectorKindKubernetes, store.ConnectorKindSSH:
			if pluginBase == "" {
				continue
			}
			target = pluginBase + "/connectors/" + c.ID + "/mcp"
		}
		if target == "" {
			continue
		}
		if isSelfMCPEndpoint(target) || (c.Kind == store.ConnectorKindMCP && pluginBase != "" && strings.HasPrefix(target, pluginBase+"/")) {
			s.emitAndRecord(ctx, conv, runID, newEvent("run.warning", runID, map[string]any{"message": fmt.Sprintf("连接器 %s 指向本平台端点（自引用/绕过授权），未注入外部会话", c.Name)}), emit)
			continue
		}
		if _, err := tool.FetchMCPToolsWithHeaders(ctx, c.Name, target, headers, mcpFetchTimeout); err != nil {
			tool.EmitEvent(ctx, "connector.degraded", map[string]any{"connector": c.Name, "target": target, "reason": err.Error()})
			s.emitAndRecord(ctx, conv, runID, newEvent("run.warning", runID, map[string]any{"message": fmt.Sprintf("连接器 %s 不可达（含幂等重试），本次未注入外部会话: %v", c.Name, err)}), emit)
			continue
		}
		refs = append(refs, inference.MCPServerRef{Name: c.Name, URL: target, Headers: headers})
	}
	return refs
}

// waitExtApproval REQ-206/M41 S2 权限转审批：approval.request 事件透出（SSE+落库）后挂起等待
// 人工决议（POST /api/conversations/{id}/ext-approval）。超时取 agent.ApprovalTimeoutHours
//（REQ-231③ 挂起超时可选，未配置则不设超时——挂到 run 结束）；ctx 取消（停止运行）即拒绝。
// 决议映射：approve → 首个 allow_once/allow_always 选项；deny/超时/取消 → cancelled（dsh 语义=拒绝本轮）。
func (s *Service) waitExtApproval(runCtx context.Context, conv *store.Conversation, agent *store.Agent, runID string, pr inference.PermissionRequest, emit EmitFn) inference.PermissionDecision {
	ch := make(chan string, 1)
	s.mu.Lock()
	s.extApprovals[conv.ID] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.extApprovals, conv.ID)
		s.mu.Unlock()
	}()
	opts := make([]map[string]string, 0, len(pr.Options))
	for _, o := range pr.Options {
		opts = append(opts, map[string]string{"option_id": o.OptionID, "name": o.Name, "kind": o.Kind})
	}
	toolName := pr.Title
	data := map[string]any{"tool_name": toolName, "kind": pr.Kind, "options": opts}
	s.emitAndRecord(runCtx, conv, runID, newEvent("approval.request", runID, data), emit)
	var timeoutCh <-chan time.Time
	if agent.ApprovalTimeoutHours > 0 {
		t := time.NewTimer(time.Duration(agent.ApprovalTimeoutHours * float64(time.Hour)))
		defer t.Stop()
		timeoutCh = t.C
	}
	pickAllow := func() string {
		for _, o := range pr.Options {
			if o.Kind == "allow_once" || o.Kind == "allow_always" {
				return o.OptionID
			}
		}
		return ""
	}
	select {
	case d := <-ch:
		if d == "approve" {
			if id := pickAllow(); id != "" {
				return inference.PermissionDecision{Outcome: "selected", OptionID: id}
			}
			return inference.PermissionDecision{Outcome: "cancelled"}
		}
		return inference.PermissionDecision{Outcome: "cancelled"}
	case <-timeoutCh:
		s.emitAndRecord(runCtx, conv, runID, newEvent("run.warning", runID, map[string]any{"message": fmt.Sprintf("外部权限审批挂起超时（%.2f 小时），已拒绝本轮工具调用", agent.ApprovalTimeoutHours)}), emit)
		return inference.PermissionDecision{Outcome: "cancelled"}
	case <-runCtx.Done():
		return inference.PermissionDecision{Outcome: "cancelled"}
	}
}
