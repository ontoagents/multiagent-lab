package chat

// REQ-553/M360 方案分歧结构化识别与人工裁决（结构化协作批 P0；107 号 §四）——
// 服务侧事件点规则评估与挂起状态扩展。识别=平台确定性规则档（零 LLM 裁量），
// 裁决=必经人工（resolve 两态），执行=复用既有挂起-恢复管线（Service.Resume 全链）。

import (
	"context"

	"github.com/ontoagents/ontoagents/backend/internal/store"
	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

// detectEventConflict event 点规则评估（verify 收束后边界——无活运行可挂起）：命中规则档
// event 条目即落 pending 裁决单+conflict.detected 事件；裁决经回注消息发起新运行（API 侧）。
// 默认档无 event 条目=零行为变化；规则档关闭/解析失败同样零命中（107 号 §4.3②）。
func (s *Service) detectEventConflict(ctx context.Context, conv *store.Conversation, runID, eventName, summary string, emit EmitFn) {
	if s.Store == nil {
		return
	}
	pid := ""
	if conv.ProjectID != nil {
		pid = *conv.ProjectID
	}
	rule, hit := tool.EvaluateConflictPolicy(s.Store, pid, "event", eventName)
	if !hit {
		return
	}
	cr := &store.ConflictReview{
		ConversationID: conv.ID,
		RunID:          runID,
		Kind:           "event",
		RuleID:         rule,
		State:          "pending",
		Summary:        summary,
	}
	if err := s.Store.CreateConflictReview(cr); err != nil {
		s.emitAndRecord(ctx, conv, runID, newEvent(EventRunWarning, runID, map[string]any{
			"message": "分歧裁决单落库失败（event 点规则命中）: " + err.Error(),
		}), emit)
		return
	}
	s.emitAndRecord(ctx, conv, runID, newEvent(EventConflictDete, runID, map[string]any{
		"conflict_id": cr.ID, "kind": "event", "rule_id": rule,
		"summary": summary, "state": "pending",
	}), emit)
}
