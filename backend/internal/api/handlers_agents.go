package api

import (
	"errors"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/ontoagents/ontoagents/backend/internal/inference"
	"github.com/ontoagents/ontoagents/backend/internal/runtime"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// normalizeInferenceBackend M13 §6.16：推理后端归一（空 = eino-adk 自研默认）+ 合法性校验。
// PUT 为全量更新（老客户端不带该字段时置空），置空一律归一为默认，不产生破坏。
func (s *Server) normalizeInferenceBackend(a *store.Agent) error {
	a.InferenceBackend = strings.TrimSpace(a.InferenceBackend)
	switch a.InferenceBackend {
	case "", inference.DefaultBackend:
		a.InferenceBackend = inference.DefaultBackend
	default:
		if s.Chat == nil || s.Chat.Inference == nil || !s.Chat.Inference.Known(a.InferenceBackend) {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "未知推理后端: " + a.InferenceBackend}
		}
	}
	return nil
}

// ---- Agents ----

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.Store.ListAgents()
	if err != nil {
		writeErr(w, err)
		return
	}
	if agents == nil {
		agents = []*store.Agent{}
	}
	writeJSON(w, http.StatusOK, agents)
}

// normalizeOnchatMode REQ-360/M161：对话建本体能力开关归一（''/guided/quick 之外 400——
// 非法值静默归''会伪装成「保存成功但开关失效」；显式报错让人话可懂）。
func normalizeOnchatMode(a *store.Agent) error {
	switch a.OnchatMode {
	case "", "guided", "quick":
		return nil
	}
	return &store.HTTPError{Status: http.StatusBadRequest, Msg: "onchat_mode 仅支持空（关）/ guided（访谈）/ quick（快速）"}
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	// REQ-131/M18：mcp_serve 开启时自动生成 Token 由 normalizeMcpServe 处理（见 updateAgent 同款逻辑）
	var a store.Agent
	if err := decodeJSON(r, &a); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(a.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if err := normalizeOnchatMode(&a); err != nil {
		writeErr(w, err)
		return
	}
	if a.MaxIteration <= 0 {
		a.MaxIteration = 25
	}
	if a.RuntimeBackend == "" {
		a.RuntimeBackend = "inprocess"
	}
	if err := s.normalizeInferenceBackend(&a); err != nil {
		writeErr(w, err)
		return
	}
	if verr := validateAgentHooks(a.Hooks); verr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": verr.Error()})
		return
	}
	if a.McpServe.Enabled && a.McpServe.Token == "" {
		a.McpServe.Token = RandToken()
	}
	created, err := s.Store.CreateAgent(&a)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.mcpSync()
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	a, err := s.Store.GetAgent(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// builtinToolsUnion REQ-213② 内置工具面基座并集：用户勾选 ∪ 内置必装十工具（L0 七只读
// + L1 平台知识/提案三件；REQ-451 doc_list/doc_grep 入基座）——L0/L1 为「授权更多」的
// 基座不可摘除；幂等保序（内置在前）。
func builtinToolsUnion(submitted []string) []string {
	required := []string{"doc_read", "doc_list", "doc_grep", "list_model_connections", "list_agents", "list_kbs",
		"get_assistant_config", "sync_platform_kb", "search_platform_kb", "propose_assistant_config"}
	seen := map[string]bool{}
	out := make([]string, 0, len(submitted)+len(required))
	for _, t := range required {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, t := range submitted {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request) {
	var a store.Agent
	if err := decodeJSON(r, &a); err != nil {
		writeErr(w, err)
		return
	}
	a.ID = r.PathValue("id")
	// REQ-400/M196：If-Match 乐观锁（头缺省不校验——存量兼容；前端主保存链路已透传）
	if prev, perr := s.Store.GetAgent(a.ID); perr == nil && prev != nil {
		if !checkIfMatch(w, r, prev.UpdatedAt) {
			return
		}
	}
	// REQ-186：内置助手行不可删除；REQ-192/M32②+REQ-213：编辑走**字段白名单**——
	// instruction/model_conn_id/temperature + tools/skills/mcp_servers/max_tokens/max_iteration
	// 放行（能力面与普通智能体统一，REQ-213②）；tools 经 builtinToolsUnion 做基座并集保护
	// （L0/L1 八工具不可摘除=「授权更多」）；身份（name/description/logo）与角色字段
	// （is_builtin/runtime_backend/tool_approval/mcp_serve/沙箱/伴生）仍锁死。
	if prev, perr := s.Store.GetAgent(a.ID); perr == nil && prev != nil && prev.IsBuiltin {
		merged := *prev
		if v := strings.TrimSpace(a.Instruction); v != "" {
			merged.Instruction = v
		}
		merged.ModelConnID = a.ModelConnID
		if a.Temperature != nil {
			if *a.Temperature < 0 || *a.Temperature > 2 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "temperature 取值 0~2"})
				return
			}
			merged.Temperature = a.Temperature
		}
		// REQ-213② 能力字段放行（tools 基座并集保护：用户勾选 ∪ 内置必装）
		merged.Tools = builtinToolsUnion(a.Tools)
		merged.Skills = a.Skills
		merged.MCPServers = a.MCPServers
		// REQ-360/M161：能力开关放行（内置行迁移已默认 guided——平台助手显式引导面定位，
		// 03 §2.54 C1 决策注；此处允许经配置面关闭/切换，与普通智能体统一）
		if err := normalizeOnchatMode(&a); err != nil {
			writeErr(w, err)
			return
		}
		merged.OnchatMode = a.OnchatMode
		// REQ-366/M164：本体操作能力开关放行（默认关无迁移默认；布尔值无归一需要）
		merged.OntoOps = a.OntoOps
		if a.MaxTokens != nil {
			merged.MaxTokens = a.MaxTokens
		}
		if a.MaxIteration > 0 {
			merged.MaxIteration = a.MaxIteration
		}
		updated, err := s.Store.UpdateAgent(&merged)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}
	if err := s.normalizeInferenceBackend(&a); err != nil {
		writeErr(w, err)
		return
	}
	if err := normalizeOnchatMode(&a); err != nil { // REQ-360/M161：能力开关归一（PUT 全量替换语义）
		writeErr(w, err)
		return
	}
	if verr := validateAgentHooks(a.Hooks); verr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": verr.Error()})
		return
	}
	var prevSnapshot *store.Agent
	if prev, perr := s.Store.GetAgent(a.ID); perr == nil && prev != nil {
		// 51 号 W5 定案④（REQ-206/M41 真机联调轮）：外部推理后端的 harness 治理面降维——
		// verify_command/runtime_backend/沙箱资源由外部 harness 自治，平台侧配置不生效；
		// UI 标注 + 保存校验都做：变更这些字段且后端为外部时 400（work_dir 仍平台侧文件
		// 视图根、tool_approval 由 deepseek-harness-acp 权限流承接，不拦截）。
		if s.Chat != nil && s.Chat.Inference != nil && s.Chat.Inference.IsExternal(a.InferenceBackend) {
			var conflicts []string
			if a.VerifyCommand != prev.VerifyCommand {
				conflicts = append(conflicts, "verify_command")
			}
			if a.RuntimeBackend != prev.RuntimeBackend {
				conflicts = append(conflicts, "runtime_backend")
			}
			if a.SandboxMemory != prev.SandboxMemory {
				conflicts = append(conflicts, "sandbox_memory")
			}
			if a.SandboxCPUs != prev.SandboxCPUs {
				conflicts = append(conflicts, "sandbox_cpus")
			}
			if len(conflicts) > 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("外部推理后端（%s）的 harness 治理由外部自治，平台侧字段不生效，已拒绝变更: %s", a.InferenceBackend, strings.Join(conflicts, "/"))})
				return
			}
		}
		prevSnapshot = prev
		if s.normalizeMcpServe(&a, *prev) {
			defer s.mcpSync()
		}
	} else if a.McpServe.Enabled && a.McpServe.Token == "" {
		a.McpServe.Token = RandToken()
		defer s.mcpSync()
	}
	updated, err := s.Store.UpdateAgent(&a)
	if err != nil {
		writeErr(w, err)
		return
	}
	// REQ-226/M54：保存即快照——「更新前」配置整包落版本表（一键回滚数据面；失败仅日志不阻断保存）
	if prevSnapshot != nil {
		if prevJSON, merr := json.Marshal(prevSnapshot); merr == nil {
			if _, verr := s.Store.InsertAgentConfigVersion(a.ID, string(prevJSON), "保存前自动快照"); verr != nil {
				log.Printf("[agents] 配置快照失败（%s）: %v", a.ID, verr)
			}
		}
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// REQ-186：内置助手行不可删除
	if prev, perr := s.Store.GetAgent(id); perr == nil && prev != nil && prev.IsBuiltin {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "内置助手不可删除（REQ-186）"})
		return
	}
	s.mcpSync()
	if err := s.Store.DeleteAgent(id); err != nil {
		writeErr(w, err)
		return
	}
	// REQ-216 增量②c：伴生数据随删清理（候选+游标；绑定行随 agent 级联消失）。
	// 图数据不动——伴生子图归属本体资产（同本体其他绑定者沉淀保留），本体详情页可管理。
	_ = s.Store.DeleteAgentCompanionData(id)
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

// ---- Projects ----

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	// REQ-550/M357：workspace_id 过滤参（工作区侧板 projects 视图按归属列项目；缺省=全量不变）
	if wsID := r.URL.Query().Get("workspace_id"); wsID != "" {
		ps, err := s.Store.ListProjectsByWorkspace(wsID)
		if err != nil {
			writeErr(w, err)
			return
		}
		if ps == nil {
			ps = []*store.Project{}
		}
		writeJSON(w, http.StatusOK, ps)
		return
	}
	ps, err := s.Store.ListProjects()
	if err != nil {
		writeErr(w, err)
		return
	}
	if ps == nil {
		ps = []*store.Project{}
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var p store.Project
	if err := decodeJSON(r, &p); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(p.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if p.CollabMode == "" {
		p.CollabMode = "agent_as_tool"
	}
	if p.WorkflowMode == "" {
		p.WorkflowMode = "free"
	}
	if !normalizeProjectLocalDir(w, &p) {
		return
	}
	created, err := s.Store.CreateProject(&p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.GetProject(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	var p store.Project
	if err := decodeJSON(r, &p); err != nil {
		writeErr(w, err)
		return
	}
	p.ID = r.PathValue("id")
	// REQ-400/M196：If-Match 乐观锁（同 updateAgent 口径）
	if prev, perr := s.Store.GetProject(p.ID); perr == nil && prev != nil {
		if !checkIfMatch(w, r, prev.UpdatedAt) {
			return
		}
	}
	if !normalizeProjectLocalDir(w, &p) {
		return
	}
	updated, err := s.Store.UpdateProject(&p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// setProjectWorkspace REQ-550/M357：归属/解除项目到工作区（PUT /api/projects/{id}/workspace，
// body {workspace_id}，空=解除）。归属变更是显式动作，不并入整行 PUT（UpdateProject SQL 不写
// workspace_id 列——防整行保存误清归属）。
func (s *Server) setProjectWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	updated, err := s.Store.SetProjectWorkspace(r.PathValue("id"), body.WorkspaceID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// applyProjectWorkspace REQ-550/M357：「应用到现有会话」幂等批量（POST /api/projects/{id}/workspace/apply）。
// 返回补齐会话数；updated_at 必 bump——开着的会话窗下一次 patchConv 409 为预期行为（106 号 obj-7 注记，
// 前端 toast 刷新兜底），冒烟不当 bug 抓。
func (s *Server) applyProjectWorkspace(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.ApplyWorkspaceDefaultsToProject(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": n})
}

func (s *Server) setProjectAgents(w http.ResponseWriter, r *http.Request) {
	var members []store.ProjectMember
	if err := decodeJSON(r, &members); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Store.SetProjectAgents(r.PathValue("id"), members); err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.Store.GetProject(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.DeleteProject(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

// ---- M10/10b：沙箱生命周期（状态/启动/停止，per Agent） ----

// sandboxAvailable 沙箱后端是否已启用（SANDBOX_IMAGE 配置即启用）。
func (s *Server) sandboxAvailable() bool {
	return s.Chat != nil && s.Chat.Runtime != nil
}

// sandboxStatus GET /api/agents/{id}/sandbox：容器状态 + 资源限制 + 端点。
func (s *Server) sandboxStatus(w http.ResponseWriter, r *http.Request) {
	if !s.sandboxAvailable() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	id := r.PathValue("id")
	agent, err := s.Store.GetAgent(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	st, err := s.Chat.Runtime.Status(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true, "state": st.State, "detail": st.Detail,
		"memory": agent.SandboxMemory, "cpus": agent.SandboxCPUs,
	})
}

// sandboxStart POST /api/agents/{id}/sandbox/start：拉起（或复用）agentd 容器。
func (s *Server) sandboxStart(w http.ResponseWriter, r *http.Request) {
	if !s.sandboxAvailable() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "沙箱后端未启用（需配置 SANDBOX_IMAGE）"})
		return
	}
	id := r.PathValue("id")
	agent, err := s.Store.GetAgent(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if agent.RuntimeBackend != "docker" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "该智能体运行后端不是 docker"})
		return
	}
	ep, err := s.Chat.Runtime.Start(r.Context(), runtime.StartSpec{AgentID: id, Memory: agent.SandboxMemory, CPUs: agent.SandboxCPUs})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoint": ep.URL})
}

// sandboxStop POST /api/agents/{id}/sandbox/stop：停止并移除容器。
func (s *Server) sandboxStop(w http.ResponseWriter, r *http.Request) {
	if !s.sandboxAvailable() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "沙箱后端未启用（需配置 SANDBOX_IMAGE）"})
		return
	}
	if err := s.Chat.Runtime.Stop(r.Context(), runtime.StopSpec{AgentID: r.PathValue("id")}); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stopped": r.PathValue("id")})
}

// listAgentConfigVersions GET /api/agents/{id}/config-versions（REQ-226/M54：版本列表）。
func (s *Server) listAgentConfigVersions(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListAgentConfigVersions(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []*store.AgentConfigVersion{}
	}
	writeJSON(w, http.StatusOK, list)
}

// agentConfigDiff GET /api/agents/{id}/config-versions/{version}/diff——该版与当前配置的差异。
func (s *Server) agentConfigDiff(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	var version int
	if _, err := fmt.Sscanf(r.PathValue("version"), "%d", &version); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version 须为数字"})
		return
	}
	v, err := s.Store.GetAgentConfigVersion(agentID, version)
	if err != nil {
		writeErr(w, err)
		return
	}
	cur, err := s.Store.GetAgent(agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	curJSON, _ := json.Marshal(cur)
	diffs, err := store.AgentConfigDiff(v.ConfigJSON, string(curJSON))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": version, "diffs": diffs})
}

// rollbackAgentConfig POST /api/agents/{id}/config-versions/{version}/rollback——
// 读历史版本整包写回当前配置（解出 store.Agent 走 UpdateAgent 全量更新；
// 回滚动作本身先快照「回滚前」状态，可再滚回；24 小时内无效操作不保护——教学尺度）。
func (s *Server) rollbackAgentConfig(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	var version int
	if _, err := fmt.Sscanf(r.PathValue("version"), "%d", &version); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version 须为数字"})
		return
	}
	v, err := s.Store.GetAgentConfigVersion(agentID, version)
	if err != nil {
		writeErr(w, err)
		return
	}
	cur, err := s.Store.GetAgent(agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	// 回滚前先快照当前（可再滚回）
	if curJSON, merr := json.Marshal(cur); merr == nil {
		_, _ = s.Store.InsertAgentConfigVersion(agentID, string(curJSON), "回滚前自动快照")
	}
	var target store.Agent
	if err := json.Unmarshal([]byte(v.ConfigJSON), &target); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "版本快照解析失败: " + err.Error()})
		return
	}
	target.ID = agentID
	updated, err := s.Store.UpdateAgent(&target)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.mcpSync()
	writeJSON(w, http.StatusOK, updated)
}

// validateAgentHooks REQ-232②：自定义 deny 规则校验（工具名模式必填；规则数上限 20）。
// 非法返回错误文案（400）。空 hooks 配置（零值）合法=全默认。
func validateAgentHooks(h store.AgentHooks) error {
	if len(h.DenyRules) > 20 {
		return errors.New("自定义 deny 规则数上限 20")
	}
	seen := map[string]bool{}
	for _, r := range h.DenyRules {
		if strings.TrimSpace(r.ToolPattern) == "" {
			return errors.New("deny 规则 " + r.Name + " 缺少工具名模式（tool_pattern）")
		}
		if strings.ContainsAny(r.ToolPattern, "\n\r") {
			return errors.New("deny 规则工具名模式含非法字符")
		}
		key := r.ToolPattern + "|" + r.ArgsPattern
		if seen[key] {
			return errors.New("deny 规则重复: " + r.Name)
		}
		seen[key] = true
	}
	return nil
}

// agentHarnessAudit GET /api/agents/{id}/harness-audit?limit=50（REQ-311 B2/M115）：
// 该智能体全部会话的 Harness 审计跨会话聚合（审批挂起/决议、hook 拦截、verify 背压），
// 倒序 limit 分页；Harness 页签「审计历史」段数据源。
func (s *Server) agentHarnessAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, total, err := s.Store.ListHarnessAudit(r.PathValue("id"), limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": rows, "total": total})
}
