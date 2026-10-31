package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// REQ-557/M364 多智能体协作过程账本（结构化协作批 P1；107 号 §八）——
// run 级指标派生聚合：数据全部从既有 run_event 事件流派生（零新增事件类型/零迁移/D-O5），
// 与 agenteval 离线评测互补（指标记账、评测打分——数据源与口径互不混用）。
// 三出料：参与度（token/工具调用/在位时长）/介入频次（审批·挂起·插话·拦截·抢占计数+决议比）/
// 阶段耗时（成员在位区间+工作流节点区间——纯派生不加事件）。
// ---------------------------------------------------------------------------

// AgentParticipation 单成员参与度（token 归因沿 usage_by_agent 发射点口径；在位时长=enter/exit 差）。
type AgentParticipation struct {
	Agent        string `json:"agent"`
	TotalTokens  int    `json:"total_tokens"`
	ToolCalls    int    `json:"tool_calls"`
	DurationMS   int64  `json:"duration_ms"` // 成员在位时长（root=run 全程；成员=enter/exit 对差；未闭合计到 run 末事件）
}

// RunInterventions 介入频次（口径=approval.request+run.interrupted〔kind 分列〕+steer.pushed+
// hook.denied+run.preempted 计数；granted/denied 决议结构比；委派沿 subagent.delegate phase 分布）。
type RunInterventions struct {
	ApprovalRequests int            `json:"approval_requests"`
	ApprovalGranted  int            `json:"approval_granted"`
	ApprovalDenied   int            `json:"approval_denied"`
	InterruptedBy    map[string]int `json:"interrupted_by_kind,omitempty"` // ask_human|approval|conflict
	SteerPushed      int            `json:"steer_pushed"`
	HookDenied       int            `json:"hook_denied"`
	Preempted        int            `json:"run_preempted"`
	DelegatePhases   map[string]int `json:"delegate_phases,omitempty"` // started|completed|failed|stopped|steered
	DelegateSuccess  float64        `json:"delegate_success_rate,omitempty"`
}

// RunPhases 阶段耗时（纯派生）。
type RunPhases struct {
	ElapsedMS      int64             `json:"elapsed_ms"`                  // run.finished elapsed_ms
	AgentSpans     []PhaseSpan       `json:"agent_spans,omitempty"`       // 成员在位区间
	WorkflowNodes  []PhaseSpan       `json:"workflow_nodes,omitempty"`    // 工作流节点区间（started/completed 差）
}

// PhaseSpan 单区间。
type PhaseSpan struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
	Open       bool   `json:"open,omitempty"` // 未闭合（开始无完成）——如实标注
}

// RunMetrics 单 run 过程账本三出料。
type RunMetrics struct {
	RunID         string               `json:"run_id"`
	ConversationID string              `json:"conversation_id,omitempty"`
	EventCount    int                  `json:"event_count"`
	Participation []AgentParticipation `json:"participation"`
	Interventions RunInterventions     `json:"interventions"`
	Phases        RunPhases            `json:"phases"`
}

func scanRunEventsByRun(runID string, s *Store) ([]*RunEvent, error) {
	rows, err := s.DB.Query(`SELECT id,conversation_id,run_id,type,data,created_at,COALESCE(schema_version,1) FROM run_event WHERE run_id = ? ORDER BY created_at, id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*RunEvent
	for rows.Next() {
		var e RunEvent
		var data sql.NullString
		if err := rows.Scan(&e.ID, &e.ConversationID, &e.RunID, &e.Type, &data, &e.CreatedAt, &e.SchemaVersion); err != nil {
			return nil, err
		}
		e.Data = data.String
		out = append(out, &e)
	}
	return out, rows.Err()
}

// RunMetrics 派生单 run 过程账本（无事件返回 nil——诚实空态）。
func (s *Store) RunMetrics(runID string) (*RunMetrics, error) {
	evs, err := scanRunEventsByRun(runID, s)
	if err != nil {
		return nil, err
	}
	if len(evs) == 0 {
		return nil, nil
	}
	m := &RunMetrics{RunID: runID, EventCount: len(evs)}
	partByAgent := map[string]*AgentParticipation{}
	agentOf := func(e *RunEvent) string {
		var d struct {
			Agent string `json:"agent"`
		}
		if json.Unmarshal([]byte(e.Data), &d) == nil && d.Agent != "" {
			return d.Agent
		}
		return ""
	}
	enterAt := map[string]string{} // 成员 → enter 时间（未闭合计到末事件）
	// 工作流节点区间
	nodeStart := map[string]string{}
	nodeOpen := map[string]bool{}

	for _, e := range evs {
		if m.ConversationID == "" {
			m.ConversationID = e.ConversationID
		}
		var d map[string]any
		_ = json.Unmarshal([]byte(e.Data), &d)
		numOf := func(k string) float64 { v, _ := d[k].(float64); return v }
		strOf := func(k string) string { v, _ := d[k].(string); return v }

		switch e.Type {
		case "run.finished":
			m.Phases.ElapsedMS = int64(numOf("elapsed_ms"))
			// usage_by_agent 参与度 token（发射点归因口径单源——前端不复算）
			if arr, ok := d["usage_by_agent"].([]any); ok {
				for _, it := range arr {
					o, _ := it.(map[string]any)
					name, _ := o["agent"].(string)
					tok, _ := o["total_tokens"].(float64)
					if name == "" {
						continue
					}
					p := partByAgent[name]
					if p == nil {
						p = &AgentParticipation{Agent: name}
						partByAgent[name] = p
					}
					p.TotalTokens += int(tok)
				}
			}
			// 未闭合成员在位计到 run 末
			for name, at := range enterAt {
				p := partByAgent[name]
				if p != nil {
					p.DurationMS += msBetween(at, e.CreatedAt)
				}
			}
			enterAt = map[string]string{}
		case "tool.call":
			if a := agentOf(e); a != "" {
				p := partByAgent[a]
				if p == nil {
					p = &AgentParticipation{Agent: a}
					partByAgent[a] = p
				}
				p.ToolCalls++
			}
		case "subagent.enter":
			name := strOf("agent")
			if name != "" {
				enterAt[name] = e.CreatedAt
			}
		case "subagent.exit":
			name := strOf("agent")
			if at, ok := enterAt[name]; ok {
				p := partByAgent[name]
				if p == nil {
					p = &AgentParticipation{Agent: name}
					partByAgent[name] = p
				}
				p.DurationMS += msBetween(at, e.CreatedAt)
				delete(enterAt, name)
			}
		case "approval.request":
			m.Interventions.ApprovalRequests++
		case "approval.granted":
			m.Interventions.ApprovalGranted++
		case "approval.denied":
			m.Interventions.ApprovalDenied++
		case "run.interrupted":
			kind := strOf("kind")
			if kind == "" {
				kind = "unknown"
			}
			if m.Interventions.InterruptedBy == nil {
				m.Interventions.InterruptedBy = map[string]int{}
			}
			m.Interventions.InterruptedBy[kind]++
		case "steer.pushed":
			m.Interventions.SteerPushed++
		case "hook.denied":
			m.Interventions.HookDenied++
		case "run.preempted":
			m.Interventions.Preempted++
		case "subagent.delegate":
			ph := strOf("phase")
			if ph == "" {
				ph = "started"
			}
			if m.Interventions.DelegatePhases == nil {
				m.Interventions.DelegatePhases = map[string]int{}
			}
			m.Interventions.DelegatePhases[ph]++
		case "workflow.node":
			name := strOf("node")
			ph := strOf("phase")
			key := "started\x00" + name // started/completed 同名配对（键统一 started 前缀）
			if ph == "started" {
				nodeStart[key] = e.CreatedAt
				nodeOpen[key] = true
			} else if ph == "completed" {
				if at, ok := nodeStart[key]; ok {
					m.Phases.WorkflowNodes = append(m.Phases.WorkflowNodes, PhaseSpan{Name: name, DurationMS: msBetween(at, e.CreatedAt)})
					delete(nodeStart, key)
					nodeOpen[key] = false
				}
			}
		}
	}
	// 收尾：仍在位的成员区间（enter 无 exit——计到末事件）
	last := evs[len(evs)-1]
	for name, at := range enterAt {
		p := partByAgent[name]
		if p == nil {
			p = &AgentParticipation{Agent: name}
			partByAgent[name] = p
		}
		p.DurationMS += msBetween(at, last.CreatedAt)
	}
	// 未闭合工作流节点（started 无 completed——Open 如实标注）
	for key, at := range nodeStart {
		if nodeOpen[key] {
			name := key[strings.IndexByte(key, 0)+1:]
			m.Phases.WorkflowNodes = append(m.Phases.WorkflowNodes, PhaseSpan{Name: name, DurationMS: msBetween(at, last.CreatedAt), Open: true})
		}
	}
	// 委派成功率
	if m.Interventions.DelegatePhases != nil {
		done := m.Interventions.DelegatePhases["completed"]
		fail := m.Interventions.DelegatePhases["failed"]
		if done+fail > 0 {
			m.Interventions.DelegateSuccess = float64(done) / float64(done+fail)
		}
	}
	// 参与度输出（token 降序）
	for _, p := range partByAgent {
		m.Participation = append(m.Participation, *p)
	}
	if len(m.Participation) > 1 {
		for i := 1; i < len(m.Participation); i++ {
			for j := i; j > 0 && m.Participation[j].TotalTokens > m.Participation[j-1].TotalTokens; j-- {
				m.Participation[j], m.Participation[j-1] = m.Participation[j-1], m.Participation[j]
			}
		}
	}
	return m, nil
}

// msBetween 两 RFC3339/Nano 时间戳差值毫秒（解析失败按 0——诚实不计）。
func msBetween(a, b string) int64 {
	ta, err1 := parseTimePtr(a)
	tb, err2 := parseTimePtr(b)
	if err1 != nil || err2 != nil {
		return 0
	}
	d := tb.Sub(ta).Milliseconds()
	if d < 0 {
		return 0
	}
	return d
}

// MemberUsage 成员维度聚合行（group_by=member——数据源=run.finished.usage_by_agent）。
type MemberUsage struct {
	Member       string `json:"member"`
	Runs         int    `json:"runs"`
	TotalTokens  int    `json:"total_tokens"`
}

// MemberUsageStats 按「过程账本成员」聚合（107 号 §8.3 group_by=member——与既有 agent 维度
// 〔会话归属智能体〕正交：member=run 内成员（含委派子成员），agent=发起 run 的智能体）。
func (s *Store) MemberUsageStats(rng UsageRange) ([]*MemberUsage, error) {
	// run.finished 的 usage_by_agent 在事件 data JSON 内——SQLite 无 JSON1 保证，应用层聚合
	rows, err := s.DB.Query(`SELECT data, created_at FROM run_event WHERE type='run.finished' ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agg := map[string]*MemberUsage{}
	for rows.Next() {
		var data, createdAt sql.NullString
		if err := rows.Scan(&data, &createdAt); err != nil {
			return nil, err
		}
		day := createdAt.String
		if len(day) > 10 {
			day = day[:10]
		}
		if rng.From != "" && day < rng.From {
			continue
		}
		if rng.To != "" && day > rng.To {
			continue
		}
		var d struct {
			UsageByAgent []struct {
				Agent       string `json:"agent"`
				TotalTokens int    `json:"total_tokens"`
			} `json:"usage_by_agent"`
		}
		if json.Unmarshal([]byte(data.String), &d) != nil {
			continue
		}
		seen := map[string]bool{}
		for _, a := range d.UsageByAgent {
			if a.Agent == "" {
				continue
			}
			m := agg[a.Agent]
			if m == nil {
				m = &MemberUsage{Member: a.Agent}
				agg[a.Agent] = m
			}
			m.TotalTokens += a.TotalTokens
			if !seen[a.Agent] {
				m.Runs++
				seen[a.Agent] = true
			}
		}
	}
	out := make([]*MemberUsage, 0, len(agg))
	for _, m := range agg {
		out = append(out, m)
	}
	// token 降序
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].TotalTokens > out[j-1].TotalTokens; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// parseTimePtr RFC3339/Nano 宽容解析（SQLite 时间戳形态）。
func parseTimePtr(v string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, strings.TrimSpace(v))
}
