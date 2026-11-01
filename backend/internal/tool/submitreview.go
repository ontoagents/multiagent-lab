// submit_review 内置工具（REQ-553/M360 方案分歧结构化识别与人工裁决；107 号 §4.3①）：
// 审查成员按结构化评审协议提交 verdict JSON {position, plan_summary, objections[], alternatives[]}，
// 平台侧纯 Go 规则档确定性评估（默认档：position=disagree → 分歧）——命中即 StatefulInterrupt
// 挂起运行（同构 ask_human，零 LLM 裁量），裁决文本经定向恢复回喂工具返回值续跑；
// 未命中返回 ack、协调者自然续跑。verdict 解析失败 fail-closed 保守按 disagree（judge.go 兜底口径）。
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// gob 注册：checkpoint（gob 编码）会携带 Info/State（any），跨编解码需具名注册。
func init() {
	schema.RegisterName[*ConflictInfo]("eino_multiagent_lab_tool_conflict_info")
	schema.RegisterName[*ConflictState]("eino_multiagent_lab_tool_conflict_state")
}

// ConflictInfo 分歧挂起的用户可读信息（run.interrupted kind=conflict 外显；裁决卡数据源）。
type ConflictInfo struct {
	ConflictID string   `json:"conflict_id"`
	Question   string   `json:"question"` // 方案对比摘要（卡面主文本）
	Choices    []string `json:"choices,omitempty"`
}

// ConflictState 分歧挂起持久化状态（checkpoint gob；恢复重入时取回）。
type ConflictState struct {
	ConflictID string `json:"conflict_id"`
}

type submitReviewIn struct {
	Position     string   `json:"position" jsonschema_description:"评审立场：agree（同意当前方案）或 disagree（有分歧，需要人工裁决）"`
	PlanSummary  string   `json:"plan_summary" jsonschema_description:"本方方案一句话摘要（裁决卡对比展示）"`
	Objections   []string `json:"objections,omitempty" jsonschema_description:"对当前方案的反对要点（逐条）"`
	Alternatives []string `json:"alternatives,omitempty" jsonschema_description:"本方替代方案要点（逐条；disagree 时建议给出）"`
}

type submitReviewOut struct {
	Ack        string `json:"ack" jsonschema_description:"平台受理结果（agree=已记录续跑；disagree 命中分歧=已挂起待人工裁决）"`
	ConflictID string `json:"conflict_id,omitempty" jsonschema_description:"命中分歧时的裁决单 id"`
}

// 事件名（SSE 事件名=run_event.type；chat/event_names.go 为镜像单源——tool 包反向引用成环，
// 沿 approval.go 字面量先例，chat 侧常量同值承载）。
const (
	eventReviewSubmitted  = "review.submitted"
	eventConflictDetected = "conflict.detected"
)

// SubmitReviewDeps 工具装配依赖（装配层注入——评审协议同缝 member_profile）。
type SubmitReviewDeps struct {
	Store          *store.Store
	ConversationID string
	ProjectID      string // 项目会话归属（规则档读取；空=默认档）
	AgentID        string
	AgentName      string
	RunID          string // 发起运行（审计列；装配期已知则带，空容忍）
}

// NewSubmitReviewTool 构造 submit_review 工具（REQ-553；Store 缺失不装配）。
func NewSubmitReviewTool(deps SubmitReviewDeps) (einotool.BaseTool, error) {
	if deps.Store == nil || deps.ConversationID == "" {
		return nil, fmt.Errorf("submit_review: store/conversation required")
	}
	return utils.InferTool("submit_review",
		"提交结构化评审结论（多成员协作分歧识别）。position=agree 表示同意当前方案并说明理由；position=disagree 表示有分歧并给出反对要点与替代方案——平台将按规则档判定，命中分歧会挂起运行等待人工裁决（裁决结果自动回喂本工具返回值）。",
		func(ctx context.Context, in submitReviewIn) (*submitReviewOut, error) {
			return submitReviewFn(ctx, in, deps)
		})
}

func submitReviewFn(ctx context.Context, in submitReviewIn, deps SubmitReviewDeps) (*submitReviewOut, error) {
	// 恢复重入路径：定向恢复携带裁决文本 → 作为工具返回值回喂（续跑语义同 ask_human）
	if was, hasState, st := compose.GetInterruptState[*ConflictState](ctx); was {
		if hasData, answer := resumeAnswer(ctx); hasData {
			return &submitReviewOut{Ack: answer}, nil
		}
		if hasState && st != nil {
			// 未被定向恢复（如恢复的是别的中断点）→ 原样再中断，保持状态等待
			return nil, compose.StatefulInterrupt(ctx, conflictInfoOf(st.ConflictID, deps), st)
		}
	}
	position := strings.ToLower(strings.TrimSpace(in.Position))
	if position != "agree" && position != "disagree" {
		// verdict 解析失败 fail-closed 保守按 disagree（107 号 §4.4；judge.go 兜底口径）
		position = "disagree"
	}
	// review.submitted 评审留痕（事件契约三新增之一）
	EmitEvent(ctx, eventReviewSubmitted, map[string]any{
		"agent_id": deps.AgentID, "agent_name": deps.AgentName,
		"position": position, "plan_summary": in.PlanSummary,
		"objections": len(in.Objections), "alternatives": len(in.Alternatives),
	})
	// 平台侧纯 Go 规则档确定性评估（默认档：review 点 position=disagree → 挂起裁决）
	rule, hit := EvaluateConflictPolicy(deps.Store, deps.ProjectID, "review", position)
	if !hit {
		return &submitReviewOut{Ack: "评审已记录（" + position + "）——未命中分歧规则，继续执行"}, nil
	}
	// 命中分歧：落审计行 + 挂起（StatefulInterrupt 同构 ask_human）
	summary := conflictCardText(position, in)
	cr := &store.ConflictReview{
		ConversationID: deps.ConversationID,
		RunID:          deps.RunID,
		Kind:           "review",
		RuleID:         rule,
		Parties:        []store.ConflictParty{{AgentID: deps.AgentID, AgentName: deps.AgentName, Position: position}},
		Verdicts:       []map[string]any{{"position": position, "plan_summary": in.PlanSummary, "objections": in.Objections, "alternatives": in.Alternatives}},
		State:          "suspended",
		Summary:        summary,
	}
	if err := deps.Store.CreateConflictReview(cr); err != nil {
		return nil, fmt.Errorf("分歧裁决单落库失败: %w", err)
	}
	EmitEvent(ctx, eventConflictDetected, map[string]any{
		"conflict_id": cr.ID, "kind": "review", "rule_id": rule,
		"agent_id": deps.AgentID, "agent_name": deps.AgentName, "position": position,
		"summary": summary,
	})
	info := &ConflictInfo{ConflictID: cr.ID, Question: summary, Choices: []string{"采纳 A 案", "采纳 B 案", "提供第三案"}}
	return nil, compose.StatefulInterrupt(ctx, info, &ConflictState{ConflictID: cr.ID})
}

// conflictCardText 裁决卡主文本（方案对比摘要——结构化回注只带摘要不注入全量轨迹，107 §4.8）。
func conflictCardText(position string, in submitReviewIn) string {
	var b strings.Builder
	if position == "disagree" {
		b.WriteString("评审成员对当前方案提出分歧")
	} else {
		b.WriteString("评审成员提交评审")
	}
	if strings.TrimSpace(in.PlanSummary) != "" {
		b.WriteString("——本方方案：" + strings.TrimSpace(in.PlanSummary))
	}
	if len(in.Objections) > 0 {
		b.WriteString("；反对要点：" + strings.Join(in.Objections, "；"))
	}
	if len(in.Alternatives) > 0 {
		b.WriteString("；替代方案：" + strings.Join(in.Alternatives, "；"))
	}
	return b.String()
}

// conflictInfoOf 恢复重入再中断时重建 Info（Summary 自审计行回读——正文不进 checkpoint gob）。
func conflictInfoOf(conflictID string, deps SubmitReviewDeps) *ConflictInfo {
	q := "方案分歧待裁决"
	if deps.Store != nil {
		if cr, err := deps.Store.GetConflictReview(conflictID); err == nil && cr != nil && cr.Summary != "" {
			q = cr.Summary
		}
	}
	return &ConflictInfo{ConflictID: conflictID, Question: q, Choices: []string{"采纳 A 案", "采纳 B 案", "提供第三案"}}
}

// EvaluateConflictPolicy 冲突规则档确定性评估（REQ-553；纯 Go 零 LLM 裁量）。
// 规则档=项目级 JSON（project.conflict_policy，''=默认档）：
//
//	{"rules":[{"id":"...","when":{"type":"review","position":"disagree"},"action":"suspend"},
//	          {"id":"...","when":{"type":"event","event":"verify.failed"},"action":"pending"}]}
//
// 返回 (命中规则 id, 是否命中)。解析失败/空=默认档（review 点 position=disagree → suspend）。
func EvaluateConflictPolicy(st *store.Store, projectID, trigger, position string) (string, bool) {
	type whenSpec struct {
		Type     string `json:"type"`
		Position string `json:"position"`
		Event    string `json:"event"`
	}
	type ruleSpec struct {
		ID     string   `json:"id"`
		When   whenSpec `json:"when"`
		Action string   `json:"action"`
	}
	var spec struct {
		Rules []ruleSpec `json:"rules"`
	}
	raw := ""
	if st != nil {
		raw = st.GetProjectConflictPolicy(projectID)
	}
	if strings.TrimSpace(raw) == "" {
		// 默认档：评审 disagree → 挂起裁决（零配置可用）
		return "default_review_disagree", trigger == "review" && position == "disagree"
	}
	if err := json.Unmarshal([]byte(raw), &spec); err != nil || len(spec.Rules) == 0 {
		return "default_review_disagree", trigger == "review" && position == "disagree" // 规则档损坏 fail-closed 回默认档
	}
	for _, r := range spec.Rules {
		switch {
		case r.When.Type == "review" && trigger == "review":
			pos := strings.ToLower(strings.TrimSpace(r.When.Position))
			if pos == "" {
				pos = "disagree" // review 条目缺省立场=disagree
			}
			if pos == position {
				return ruleIDOf(r.ID), true
			}
		case r.When.Type == "event" && trigger == "event":
			if ev := strings.TrimSpace(r.When.Event); ev != "" && ev == position { // event 点：position 位传事件名
				return ruleIDOf(r.ID), true
			}
		}
	}
	return "", false
}

func ruleIDOf(id string) string {
	if strings.TrimSpace(id) == "" {
		return "unnamed_rule"
	}
	return id
}
