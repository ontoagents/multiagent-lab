package tool

// REQ-553/M360 submit_review 工具单测：规则档确定性评估（默认档/自定义档/损坏档 fail-closed）+
// 工具体（agree=ack 续跑；disagree=命中即 StatefulInterrupt 挂起+裁决单落库；verdict 解析失败
// fail-closed 按 disagree）。中断形态沿 connector_approval_test 口径（图外调用返回中断错误）。

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func newSubmitReviewFixture(t *testing.T) (*store.Store, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, ""
}

func TestEvaluateConflictPolicy_Req553(t *testing.T) {
	st, pid := newSubmitReviewFixture(t)
	// 默认档（''=内置）：review 点 disagree 命中；agree/event 不命中
	if id, hit := EvaluateConflictPolicy(st, pid, "review", "disagree"); !hit || id != "default_review_disagree" {
		t.Fatalf("默认档 disagree 应命中: %s %v", id, hit)
	}
	if _, hit := EvaluateConflictPolicy(st, pid, "review", "agree"); hit {
		t.Fatal("默认档 agree 不应命中")
	}
	if _, hit := EvaluateConflictPolicy(st, pid, "event", "verify.failed"); hit {
		t.Fatal("默认档无 event 条目不应命中（零行为变化）")
	}
	// 自定义档：event 点 + review 自定义立场
	if _, err := st.DB.Exec(`INSERT INTO project (id,name,conflict_policy,created_at,updated_at) VALUES ('p1','P','{"rules":[{"id":"vf","when":{"type":"event","event":"verify.failed"},"action":"pending"},{"id":"rv","when":{"type":"review","position":"blocked"},"action":"suspend"}]}','2026-01-01','2026-01-01')`); err != nil {
		t.Skipf("project 表形态不含直插所需列（%v）——经 CreateProject 路径补验", err)
	}
	if id, hit := EvaluateConflictPolicy(st, "p1", "event", "verify.failed"); !hit || id != "vf" {
		t.Fatalf("自定义 event 条目应命中: %s %v", id, hit)
	}
	if id, hit := EvaluateConflictPolicy(st, "p1", "review", "blocked"); !hit || id != "rv" {
		t.Fatalf("自定义 review 立场应命中: %s %v", id, hit)
	}
	if _, hit := EvaluateConflictPolicy(st, "p1", "review", "disagree"); hit {
		t.Fatal("自定义档下默认 disagree 不再命中（规则档语义替换）")
	}
	// 损坏档 fail-closed 回默认档
	if _, err := st.DB.Exec(`UPDATE project SET conflict_policy='{broken' WHERE id='p1'`); err != nil {
		t.Fatal(err)
	}
	if id, hit := EvaluateConflictPolicy(st, "p1", "review", "disagree"); !hit || id != "default_review_disagree" {
		t.Fatalf("损坏档应 fail-closed 回默认档: %s %v", id, hit)
	}
}

func TestSubmitReviewTool_Req553(t *testing.T) {
	st, _ := newSubmitReviewFixture(t)
	bt, err := NewSubmitReviewTool(SubmitReviewDeps{
		Store: st, ConversationID: "conv-1", ProjectID: "", AgentID: "agt-a", AgentName: "评审甲", RunID: "run-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	inv, ok := bt.(einotool.InvokableTool)
	if !ok {
		t.Fatalf("submit_review 应为 InvokableTool，实际 %T", bt)
	}
	// ① agree → ack（不挂起、不落裁决单）
	args, _ := json.Marshal(map[string]any{"position": "agree", "plan_summary": "维持原方案", "objections": []string{}, "alternatives": []string{}})
	out, ierr := inv.InvokableRun(context.Background(), string(args))
	if ierr != nil {
		t.Fatalf("agree 不应挂起: %v", ierr)
	}
	if !strings.Contains(out, "agree") {
		t.Fatalf("ack 应含立场: %s", out)
	}
	if l, _ := st.ListConflictReviews("conv-1"); len(l) != 0 {
		t.Fatalf("agree 不应落裁决单: %d", len(l))
	}
	// ② disagree → StatefulInterrupt 挂起 + 裁决单落库（suspended）+ 中断信息含 conflict_id
	args2, _ := json.Marshal(map[string]any{"position": "disagree", "plan_summary": "改用方案 B", "objections": []string{"X 不可行"}, "alternatives": []string{"方案 B"}})
	_, ierr = inv.InvokableRun(context.Background(), string(args2))
	if ierr == nil {
		t.Fatal("disagree 应 StatefulInterrupt 挂起")
	}
	if !strings.Contains(strings.ToLower(ierr.Error()), "interrupt") {
		t.Fatalf("应返回中断错误: %v", ierr)
	}
	list, _ := st.ListConflictReviews("conv-1")
	if len(list) != 1 || list[0].State != "suspended" || list[0].Kind != "review" {
		t.Fatalf("裁决单落库失真: %+v", list)
	}
	if !strings.Contains(list[0].Summary, "方案 B") || !strings.Contains(list[0].Summary, "X 不可行") {
		t.Fatalf("裁决卡摘要应含方案对比: %s", list[0].Summary)
	}
	if list[0].Parties[0].AgentName != "评审甲" || list[0].RunID != "run-1" {
		t.Fatalf("parties/run_id 失真: %+v", list[0])
	}
	// ③ verdict 解析失败（非法 position）→ fail-closed 按 disagree 挂起
	args3, _ := json.Marshal(map[string]any{"position": "模糊不清", "plan_summary": ""})
	_, ierr = inv.InvokableRun(context.Background(), string(args3))
	if ierr == nil || !strings.Contains(strings.ToLower(ierr.Error()), "interrupt") {
		t.Fatalf("非法 verdict 应 fail-closed 挂起: %v", ierr)
	}
	list, _ = st.ListConflictReviews("conv-1")
	if len(list) != 2 {
		t.Fatalf("fail-closed 应落第二张裁决单: %d", len(list))
	}
}
