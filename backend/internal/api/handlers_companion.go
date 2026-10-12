package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/companion"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// ---------------------------------------------------------------------------
// REQ-170/M28 伴生本体 API（候选确认流）+ REQ-216/M47 伴生本体资产化：
//   GET  /api/companion/candidates?conversation_id=&agent_id=&ontology_id=&status=  候选列表
//        （agent 维度铺平=侧板；ontology_id 维度=资产详情页「伴生候选」页签——双入口同表）
//   POST /api/companion/candidates/{id}/confirm                        确认入图（写本体伴生子图）
//   POST /api/companion/candidates/{id}/reject                          拒绝（不触达图）
//   GET  /api/companion/status?agent_id=                                agent 视角管线状态（宿主方案可观测）
//   GET  /api/companion/graph?agent_id=                                 本体伴生子图（成长可视化）
//   POST /api/companion/agents/{id}/bind                                绑定伴生本体（选择/一键创建空本体；确保宿主方案）
//   POST /api/companion/agents/{id}/reset                               agent 级解绑（清该 agent 候选游标；图数据留本体）
//   GET  /api/companion/ontologies/{id}/candidates                      本体视角候选（跨 agent）
//   GET  /api/companion/ontologies/{id}/agents                          绑定该本体的 agent 清单
//   POST /api/companion/ontologies/{id}/reset                           本体级伴生图清空（DROP 子图+清全部绑定 agent 候选游标）
//   GET  /api/companion/bound-ontologies                                伴生绑定本体 id 清单（资产列表「对话生长」徽标）
//   GET  /api/companion/ontologies/{id}/export-ttl                      伴生子图 TTL 导出（REQ-285④ 资产出门）
//   GET  /api/companion/graph-owner?conversation_id=|agent_id=          解析伴生图归属（runtime-manager facade 兼容消费）
// ---------------------------------------------------------------------------

// companionGraph GET /api/companion/graph?agent_id=（REQ-154 成长可视化；REQ-216 图=本体伴生子图）。
func (s *Server) companionGraph(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id 必填"})
		return
	}
	out, err := s.Companion.Graph(r.Context(), agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listCompanionCandidates(w http.ResponseWriter, r *http.Request) {
	convID := r.URL.Query().Get("conversation_id")
	agentID := r.URL.Query().Get("agent_id") // REQ-193/M33：agent 维度铺平过滤
	ontID := r.URL.Query().Get("ontology_id") // REQ-216⑥：本体维度跨 agent（资产详情页）
	status := r.URL.Query().Get("status")
	if ontID != "" {
		list, err := s.Companion.Store.ListCompanionCandidatesByOntology(ontID, status)
		if err != nil {
			writeErr(w, err)
			return
		}
		// REQ-216 增量③：本体视角批量——group_by=entity 按实体归组（详情页「全部入图/拒绝」，
		// 与侧板 agent 视角批量同构；REQ-216⑥「confirm/reject+批量」范围补齐）
		if r.URL.Query().Get("group_by") == "entity" {
			writeJSON(w, http.StatusOK, map[string]any{"groups": companion.GroupCandidatesByEntity(list)})
			return
		}
		if list == nil {
			list = []*store.CompanionCandidate{}
		}
		writeJSON(w, http.StatusOK, list)
		return
	}
	list, err := s.Companion.Store.ListCompanionCandidates(convID, agentID, status)
	if err != nil {
		writeErr(w, err)
		return
	}
	// REQ-194⑥确认桶聚类：group_by=entity 按实体 slug 归组（代表候选=组内置信最高+组内计数）
	if r.URL.Query().Get("group_by") == "entity" {
		writeJSON(w, http.StatusOK, map[string]any{"groups": companion.GroupCandidatesByEntity(list)})
		return
	}
	if list == nil {
		list = []*store.CompanionCandidate{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) confirmCompanionCandidate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.Companion.ConfirmCandidate(r.Context(), id, "manual")
	if err != nil {
		writeErr(w, err)
		return
	}
	graph := ""
	if agent, aerr := s.Store.GetAgent(c.AgentID); aerr == nil {
		graph = companion.GraphURI(agent.CompanionOntologyID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidate": c, "graph": graph})
}

func (s *Server) rejectCompanionCandidate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.Companion.RejectCandidate(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) companionStatus(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeErr(w, errBadRequest("agent_id 必填（REQ-211 起状态按智能体聚合）"))
		return
	}
	st, err := s.Companion.StatusByAgent(r.Context(), agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// resetCompanionAgent POST /api/companion/agents/{id}/reset（REQ-216 摘除→解绑语义）：
// 清该 agent 全部候选与游标 + 断开绑定；本体伴生子图与同本体其他 agent 沉淀保留。
func (s *Server) resetCompanionAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Companion.ResetAgent(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"reset": true})
}

// bindCompanionAgent POST /api/companion/agents/{id}/bind（REQ-216②绑定交互）：
// {"ontology_id": "..."} 选择既有本体；{"create": {"name"?: "..."}} 一键创建空本体
// （缺省名「{agent 名}的伴生本体」）。绑定成功即确保宿主方案 running（读侧立即可查）。
func (s *Server) bindCompanionAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	agent, err := s.Store.GetAgent(agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		OntologyID string `json:"ontology_id"`
		Create     *struct {
			Name string `json:"name"`
		} `json:"create"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	ontID := strings.TrimSpace(body.OntologyID)
	if ontID != "" {
		// REQ-216 增量②a：绑定校验——本体必须真实存在（防脏绑定悬挂：宿主方案 start 永远失败）
		if _, err := s.Companion.Plans.OntologyName(r.Context(), ontID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "本体不存在或构建平面不可达（" + ontID + "）: " + err.Error()})
			return
		}
	}
	if ontID == "" {
		if body.Create == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ontology_id 必填，或传 create 创建空本体"})
			return
		}
		name := strings.TrimSpace(body.Create.Name)
		if name == "" {
			name = companion.CompanionOntologyName(agent.Name)
		}
		if ontID, err = s.Companion.Plans.CreateEmptyOntology(context.Background(), name, "伴生本体（智能体 "+agent.Name+" 对话生长产物归属容器）"); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := s.Store.SetAgentCompanionBinding(agentID, ontID); err != nil {
		writeErr(w, err)
		return
	}
	// 绑定即确保宿主方案（REQ-216③：查找复用/自动创建并 start）——失败不回滚绑定，
	// 错误透出由读侧兜底拉起兜底（诚实呈现）
	planErr := ""
	if _, err := s.Companion.Plans.EnsureHostPlan(r.Context(), ontID); err != nil {
		planErr = err.Error()
	}
	updated, _ := s.Store.GetAgent(agentID)
	out := map[string]any{"agent": updated, "ontology_id": ontID, "graph": companion.GraphURI(ontID)}
	if planErr != "" {
		out["plan_error"] = planErr
	}
	writeJSON(w, http.StatusOK, out)
}

// reseedCompanionAgent POST /api/companion/agents/{id}/reseed（REQ-229③ 全量重沉淀）：
// 清该 agent pending 候选与抽取游标（图数据与绑定不动），下次对话收尾自然全量重抽；
// 同事实重入图经印证聚合（REQ-227①）计数递增不炸图。
func (s *Server) reseedCompanionAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if err := s.Companion.ReseedAgent(r.Context(), agentID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"reseed": true})
}

// ontologyCompanionCandidates GET /api/companion/ontologies/{id}/candidates（REQ-216⑥）。
func (s *Server) ontologyCompanionCandidates(w http.ResponseWriter, r *http.Request) {
	ontID := r.PathValue("id")
	status := r.URL.Query().Get("status")
	list, err := s.Companion.Store.ListCompanionCandidatesByOntology(ontID, status)
	if err != nil {
		writeErr(w, err)
		return
	}
	// REQ-216 增量③：本体视角批量——group_by=entity 按实体归组（详情页「全部入图/拒绝」）
	if r.URL.Query().Get("group_by") == "entity" {
		writeJSON(w, http.StatusOK, map[string]any{"groups": companion.GroupCandidatesByEntity(list)})
		return
	}
	if list == nil {
		list = []*store.CompanionCandidate{}
	}
	writeJSON(w, http.StatusOK, list)
}

// ontologyCompanionAgents GET /api/companion/ontologies/{id}/agents（REQ-216：绑定者清单）。
func (s *Server) ontologyCompanionAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.Companion.Store.ListAgentsByCompanionOntology(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []*store.Agent{}
	}
	writeJSON(w, http.StatusOK, list)
}

// resetCompanionOntology POST /api/companion/ontologies/{id}/reset（REQ-216 本体级清空）：
// DROP 本体伴生子图 + 清全部绑定 agent 候选与游标；本体资产本身不受影响。
func (s *Server) resetCompanionOntology(w http.ResponseWriter, r *http.Request) {
	ontID := r.PathValue("id")
	if err := s.Companion.ResetOntology(r.Context(), ontID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"reset": true})
}

// companionBoundOntologies GET /api/companion/bound-ontologies（REQ-216⑦：对话生长徽标数据源）。
func (s *Server) companionBoundOntologies(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListCompanionBoundOntologies()
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ontology_ids": list})
}

// companionGraphOwner 解析伴生图归属（REQ-216：facade 兼容消费——返回 agent 与其绑定本体）。
// agent_id 直传优先；conversation_id 经 conv.AgentID 解析（项目会话无唯一归属 → 400 提示直传）。
func (s *Server) companionGraphOwner(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	convID := r.URL.Query().Get("conversation_id")
	if agentID == "" && convID == "" {
		writeErr(w, errBadRequest("agent_id / conversation_id 至少传一"))
		return
	}
	if agentID == "" {
		conv, err := s.Store.GetConversation(convID)
		if err != nil {
			writeErr(w, err)
			return
		}
		if conv.Scope == "project" || conv.AgentID == nil || *conv.AgentID == "" {
			writeErr(w, errBadRequest("项目会话无唯一所属智能体（多成员各自伴生本体）——请按 agent_id 直传"))
			return
		}
		agentID = *conv.AgentID
	}
	agent, err := s.Store.GetAgent(agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	ontID := strings.TrimSpace(agent.CompanionOntologyID)
	if ontID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "该智能体未绑定伴生本体"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"agent_id":    agentID,
		"ontology_id": ontID,
		"graph":       companion.GraphURI(ontID),
	})
}

// errBadRequest 简单 400 错误（与既有 writeErr 语义对齐）。
func errBadRequest(msg string) error {
	return &store.HTTPError{Status: http.StatusBadRequest, Msg: msg}
}

// exportCompanionTTL GET /api/companion/ontologies/{id}/export-ttl（REQ-285④：伴生子图 → Turtle 下载
// ——对话生长产物可进 Protégé 等外部工具，与普通本体互操作面对齐）。
func (s *Server) exportCompanionTTL(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ontology id 必填"})
		return
	}
	data, err := s.Companion.ExportTTL(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-turtle; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="companion-%s.ttl"`, id))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ---------------------------------------------------------------------------
// REQ-286/M91 伴生图质量三维度 API（本体视角；编辑动作落 onto_decision 审计+快照同步）：
//   POST /api/companion/ontologies/{id}/mine-relations     关系挖掘补抽（A2，同步单次 LLM）
//   POST /api/companion/ontologies/{id}/entities/edit      实体编辑（C1：改 label/定义）
//   POST /api/companion/ontologies/{id}/entities/delete    删除实体（C1：主体+关联边）
//   POST /api/companion/ontologies/{id}/entities/merge     人工合并（B4：from 并入 to）
//   POST /api/companion/ontologies/{id}/relations/add      补关系（C1：两端须为图内实体）
//   POST /api/companion/ontologies/{id}/relations/delete   删关系（C1：按边 URI）
// ---------------------------------------------------------------------------

// mineCompanionRelations POST …/mine-relations（REQ-286 A2；REQ-297 异步化——202+job_id，
// 挖掘转后台分批执行，前端轮询 mine-jobs/{id}）。
func (s *Server) mineCompanionRelations(w http.ResponseWriter, r *http.Request) {
	jobID, err := s.Companion.MineRelationsAsync(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID})
}

// getMineJob GET /api/companion/mine-jobs/{id}（REQ-297：挖掘 job 状态轮询；
// 404=任务已失效〔进程重启丢内存态〕——前端提示重新触发）。
func (s *Server) getMineJob(w http.ResponseWriter, r *http.Request) {
	j := s.Companion.GetMineJob(r.PathValue("id"))
	if j == nil {
		writeErr(w, &store.HTTPError{Status: 404, Msg: "任务已失效（服务重启）——请重新触发挖掘"})
		return
	}
	writeJSON(w, http.StatusOK, j)
}

// editCompanionEntity POST …/entities/edit（REQ-286 C1）。
func (s *Server) editCompanionEntity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label         string `json:"label"`
		NewLabel      string `json:"new_label"`
		NewDefinition string `json:"new_definition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, errBadRequest("请求体非法"))
		return
	}
	if err := s.Companion.EditEntity(r.Context(), r.PathValue("id"), body.Label, body.NewLabel, body.NewDefinition); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// deleteCompanionEntity POST …/entities/delete（REQ-286 C1）。
func (s *Server) deleteCompanionEntity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Label) == "" {
		writeErr(w, errBadRequest("label 必填"))
		return
	}
	if err := s.Companion.DeleteEntity(r.Context(), r.PathValue("id"), body.Label); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// mergeCompanionEntity POST …/entities/merge（REQ-286 B4）。
func (s *Server) mergeCompanionEntity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, errBadRequest("请求体非法"))
		return
	}
	if err := s.Companion.MergeEntity(r.Context(), r.PathValue("id"), body.From, body.To); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// addCompanionRelation POST …/relations/add（REQ-286 C1）。
func (s *Server) addCompanionRelation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source     string `json:"source"`
		RelName    string `json:"rel_name"`
		Target     string `json:"target"`
		Definition string `json:"definition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, errBadRequest("请求体非法"))
		return
	}
	if err := s.Companion.AddRelationManual(r.Context(), r.PathValue("id"), body.Source, body.RelName, body.Target, body.Definition); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// deleteCompanionRelation POST …/relations/delete（REQ-286 C1）。
func (s *Server) deleteCompanionRelation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EdgeURI string `json:"edge_uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.EdgeURI) == "" {
		writeErr(w, errBadRequest("edge_uri 必填"))
		return
	}
	if err := s.Companion.DeleteRelationManual(r.Context(), r.PathValue("id"), body.EdgeURI); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// diagnoseCompanionGraph POST …/diagnose（REQ-299：伴生图只读诊断——重复平行边/自指边/
// 孤立实体/重复实体/缺失定义/失效边计数）。
func (s *Server) diagnoseCompanionGraph(w http.ResponseWriter, r *http.Request) {
	d, err := s.Companion.DiagnoseGraph(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// cleanupCompanionGraph POST …/cleanup（REQ-299：有限自动清理——actions 可选 duplicateEdges/selfLoops）。
func (s *Server) cleanupCompanionGraph(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Actions []string `json:"actions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Actions) == 0 {
		writeErr(w, errBadRequest("actions 必填（duplicateEdges/selfLoops）"))
		return
	}
	out, err := s.Companion.CleanupGraph(r.Context(), r.PathValue("id"), body.Actions)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
