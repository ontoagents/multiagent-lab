// REQ-370/M168 多 Agent 协作本体工程流水线（96 号规划档主线 B·O-5 交汇项——O-1 MCP 工具面 ✅ /
// O-2 CQ 基准 ✅/O-4 A2A ✅ 三线交汇）：一键创建「领域专家 → 本体工程师 → 质量评审」三阶段流水线——
// 四个角色智能体（幂等：同名存在即复用不覆盖，沿 REQ-360 本体工程师预设先例）+ 一个项目
// （agent_as_tool 协作模式，协调者编排三阶段）。运行面=既有项目多 Agent 装配（§6.4）与
// REQ-366 spec 工具面/REQ-367 基准联动承载；协作轨迹=既有 tool.call/subagent 事件时间线。
package api

import (
	"net/http"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// 流水线角色固定名（幂等锚——同名存在即复用）。
const (
	pipeCoordName    = "本体工程协调者"
	pipeExpertName   = "本体领域专家"
	pipeEngineerName = "本体工程师" // 与 REQ-360 预设同名——存在即复用（含用户自建版）
	pipeReviewerName = "本体质量评审"
	pipeProjectName  = "本体工程流水线"
)

// pipelineInstructions 三阶段角色指令（建模/评审两角色依赖 REQ-366 spec 工具面——onto_ops 开启
// 后挂载本体的会话自动并入 spec 工具；评审门禁=error 级命中即 FAIL，结论必须引用实测数字）。
func pipelineInstructions() map[string]string {
	return map[string]string{
		pipeCoordName: "你是本体工程流水线协调者。你编排三阶段流水线（评审阶段不可跳过）：\n" +
			"阶段一：委派「本体领域专家」产出核心概念/关系/CQ 清单；\n" +
			"阶段二：把清单完整传递给「本体工程师」建模并自检（目标本体 id 由用户给定，未给则先询问）；\n" +
			"阶段三：委派「本体质量评审」跑质量门禁并给 PASS/FAIL/CONDITIONAL 结论。\n" +
			"纪律：一次只委派一个阶段；每阶段把上一阶段产物完整传给下一阶段；评审 FAIL 时把明细转给工程师修复并重评（至多两轮）；" +
			"最终向用户汇报：资产规模、质量三维分、评审结论与改进项。",
		pipeExpertName: "你是本体领域专家。职责：承担流水线「领域分析」阶段——\n" +
			"1. 接到领域主题后梳理核心概念（5~10 个）、概念层级、关键关系（含端点概念）与值得沉淀的实例类型；\n" +
			"2. 提出 5~8 条能力问题（CQ），每条标注考察点（概念覆盖/关系覆盖/推理链）；\n" +
			"3. 以结构化清单（概念/关系/CQ 三节）交回协调者，不直接修改本体资产；\n" +
			"4. 不确定处显式标注「待用户澄清」，不编造领域事实。",
		pipeEngineerName: "你是本体工程师。职责：承担流水线「建模」阶段——\n" +
			"1. 依据领域专家的概念/关系/CQ 清单，用 spec_* 工具在目标本体上构建或扩展（先 spec_get 了解现状，再逐条 upsert；挂载本体的会话工具自动可用）；\n" +
			"2. 建模完成必须 spec_validate 自检，错误逐条修复；\n" +
			"3. CQ 清单交评审方用于基准对照；\n" +
			"4. 写操作经版本门禁与审批，失败按回喂明细修正，不得编造保存成功。",
		pipeReviewerName: "你是本体质量评审。职责：承担流水线「评审门禁」阶段——\n" +
			"1. 用 spec_quality_check 跑质量门禁（三维分+findings），用 spec_validate 校验结构；\n" +
			"2. 若存在同域 CQ 基准集，用 cq_benchmark_run「测我」并如实转述三率（跨域本体 gold 低为预期非缺陷）；\n" +
			"3. 门禁结论：error 级命中>0 → FAIL（逐条列出）；无 error 且综合分≥80 → PASS；否则 CONDITIONAL（列出改进项）；\n" +
			"4. 结论必须引用实测数字，不得编造；评审报告交回协调者。",
	}
}

// upsertPipelineAgent 同名幂等创建/复用角色智能体（工程师/评审 onto_ops 开——spec 工具面随挂载并入；
// modelConnID 仅对新建生效——复用既有角色不覆盖其模型配置）。
func (s *Server) upsertPipelineAgent(name string, ontoOps bool, modelConnID string) (*store.Agent, bool, error) {
	agents, err := s.Store.ListAgents()
	if err != nil {
		return nil, false, err
	}
	for _, a := range agents {
		if a.Name == name {
			return a, false, nil // 存在即复用不覆盖（沿 REQ-360 同名幂等回落先例）
		}
	}
	inst := pipelineInstructions()[name]
	a := &store.Agent{Name: name, Description: "REQ-370 本体工程流水线角色（幂等创建，可按需编辑）", Instruction: inst, OntoOps: ontoOps}
	if modelConnID != "" {
		a.ModelConnID = &modelConnID
	}
	created, err := s.Store.CreateAgent(a)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

// createOntologyPipeline POST /api/ontology/collab-pipeline——一键创建/复用流水线。
// 可选 body {model_conn_id}：显式落到新建角色（复用的既有角色不动——不静默改配置）。
func (s *Server) createOntologyPipeline(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ModelConnID string `json:"model_conn_id"`
	}
	_ = decodeJSON(r, &body)
	roles := []struct {
		name    string
		ontoOps bool
	}{
		{pipeCoordName, false},
		{pipeExpertName, false},
		{pipeEngineerName, true},
		{pipeReviewerName, true},
	}
	members := map[string]*store.Agent{}
	createdAgents := []string{}
	for _, role := range roles {
		a, created, err := s.upsertPipelineAgent(role.name, role.ontoOps, body.ModelConnID)
		if err != nil {
			writeErr(w, err)
			return
		}
		members[role.name] = a
		if created {
			createdAgents = append(createdAgents, nameOrEmpty(a))
		}
	}
	// 项目（同名幂等复用）：成员=协调者+三角色，agent_as_tool 协作
	projects, err := s.Store.ListProjects()
	if err != nil {
		writeErr(w, err)
		return
	}
	var project *store.Project
	projectCreated := false
	for _, p := range projects {
		if p.Name == pipeProjectName {
			project = p
			break
		}
	}
	if project == nil {
		project, err = s.Store.CreateProject(&store.Project{
			Name:        pipeProjectName,
			Description: "REQ-370 多 Agent 协作本体工程流水线：领域分析 → 建模 → 评审门禁（agent_as_tool 协作）",
			CollabMode:  "agent_as_tool",
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		projectCreated = true
	}
	// 成员与协调者每次落齐（幂等——复用场景下成员可能被手动移除；project_agent 表全量重设）
	memberRows := []store.ProjectMember{
		{AgentID: members[pipeCoordName].ID, Role: "coordinator"},
		{AgentID: members[pipeExpertName].ID, Role: "member"},
		{AgentID: members[pipeEngineerName].ID, Role: "member"},
		{AgentID: members[pipeReviewerName].ID, Role: "member"},
	}
	if err := s.Store.SetProjectAgents(project.ID, memberRows); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id": project.ID,
		"project_created": projectCreated,
		"coordinator_id":  members[pipeCoordName].ID,
		"members": map[string]string{
			"coordinator":      members[pipeCoordName].ID,
			"domain_expert":    members[pipeExpertName].ID,
			"ontology_engineer": members[pipeEngineerName].ID,
			"quality_reviewer": members[pipeReviewerName].ID,
		},
		"agents_created": createdAgents,
		"collab_mode":    project.CollabMode,
		"hint":           "新建项目会话（scope=project，挂载目标本体方案）并向协调者提出领域主题即启动流水线；协作轨迹见对话过程时间线",
	})
}

func nameOrEmpty(a *store.Agent) string {
	if a == nil {
		return ""
	}
	return a.Name
}
