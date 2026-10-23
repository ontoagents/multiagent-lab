package store

import (
	"strings"
	"testing"
)

// REQ-224①：状态族统一收口——setter 写入自动留痕 prev→next，no-op 不落，历史倒序可查。
func TestConversationStateLog(t *testing.T) {
	st := schedTestStore(t)
	defer st.Close()
	schedTestConv(t, st, "conv-log")

	// interrupt_state：写入→清除（清除到空值同样留痕）
	if err := st.SetConversationInterruptState("conv-log", `{"kind":"ask_human","run_id":"r1"}`, "interrupt:r1"); err != nil {
		t.Fatalf("set interrupt: %v", err)
	}
	if err := st.SetConversationInterruptState("conv-log", `{"kind":"ask_human","run_id":"r1"}`, "interrupt:r1-dup"); err != nil {
		t.Fatalf("set interrupt dup: %v", err)
	}
	if err := st.SetConversationInterruptState("conv-log", "", "resume:r1"); err != nil {
		t.Fatalf("clear interrupt: %v", err)
	}
	// todo_json 与 context_state
	if err := st.SaveConversationTodo("conv-log", `[{"content":"a","status":"pending"}]`); err != nil {
		t.Fatalf("todo: %v", err)
	}
	if err := st.SaveConversationTodo("conv-log", `[{"content":"a","status":"pending"}]`); err != nil {
		t.Fatalf("todo dup: %v", err) // no-op：同值不落第二条
	}
	if err := st.SaveContextState("conv-log", `{"summary":"s1"}`); err != nil {
		t.Fatalf("ctx: %v", err)
	}
	// schedule 族
	if err := st.UpsertSchedule("conv-log", 5, 10); err != nil {
		t.Fatalf("upsert sched: %v", err)
	}
	if err := st.DeleteSchedule("conv-log", "schedule.cancel"); err != nil {
		t.Fatalf("delete sched: %v", err)
	}

	logs, err := st.ListConversationStateLog("conv-log", 0)
	if err != nil {
		t.Fatalf("list log: %v", err)
	}
	if len(logs) != 6 { // interrupt 写/清 2 条 + todo 1 + ctx 1 + sched set/del 2
		t.Fatalf("期望 6 条留痕（no-op 去重后），实得 %d", len(logs))
	}
	first := logs[0] // 倒序：最新为 schedule 摘除
	if first.Field != "schedule" || first.Next != "" || first.Reason != "schedule.cancel" {
		t.Fatalf("最新留痕应为 schedule 摘除，实得 %+v", first)
	}
	fields := map[string]int{}
	for _, l := range logs {
		fields[l.Field]++
		if l.Prev == l.Next {
			t.Fatalf("prev==next 的 no-op 不应留痕: %+v", l)
		}
	}
	if fields["interrupt_state"] != 2 || fields["todo_json"] != 1 || fields["context_state"] != 1 || fields["schedule"] != 2 {
		t.Fatalf("各字段留痕计数不符: %v", fields)
	}
	// limit 透传
	if l2, _ := st.ListConversationStateLog("conv-log", 2); len(l2) != 2 {
		t.Fatalf("limit=2 应截断，实得 %d", len(l2))
	}
}

// REQ-224 H10：本体消费贡献度量——逐次落行、聚合（TopN 仅成功命中计）、跨本体概览。
func TestOntologyContribution(t *testing.T) {
	st := schedTestStore(t)
	defer st.Close()

	hits := []*OntologyHit{
		{OntologyID: "ont-a", ConversationID: "c1", RunID: "r1", Tool: "get_concept", Target: "计算节点", OK: true},
		{OntologyID: "ont-a", ConversationID: "c1", RunID: "r1", Tool: "get_concept", Target: "计算节点", OK: true},
		{OntologyID: "ont-a", ConversationID: "c2", RunID: "r2", Tool: "neighbors", Target: "node-1", OK: true},
		{OntologyID: "ont-a", ConversationID: "c2", RunID: "r2", Tool: "sparql_query", OK: false}, // 失败：不进 TopN
		{OntologyID: "ont-b", ConversationID: "c3", RunID: "r3", Tool: "list_concepts", OK: true},
	}
	for _, h := range hits {
		if err := st.InsertOntologyHit(h); err != nil {
			t.Fatalf("insert hit: %v", err)
		}
	}

	c, err := st.OntologyContribution("ont-a", 10)
	if err != nil {
		t.Fatalf("contribution: %v", err)
	}
	if c.TotalHits != 4 || c.OKHits != 3 {
		t.Fatalf("命中数不符: total=%d ok=%d", c.TotalHits, c.OKHits)
	}
	if len(c.ByTool) != 3 {
		t.Fatalf("按工具分布应为 3 种，实得 %d", len(c.ByTool))
	}
	if len(c.TopTargets) != 2 || c.TopTargets[0].Target != "计算节点" || c.TopTargets[0].Count != 2 {
		t.Fatalf("TopN 不符: %+v", c.TopTargets)
	}
	if len(c.Recent) != 4 {
		t.Fatalf("recent 应 4 条，实得 %d", len(c.Recent))
	}
	if c.FirstHitAt == "" || c.LastHitAt == "" {
		t.Fatalf("首末命中时间应非空")
	}

	// 空本体：零值画像不报错
	empty, err := st.OntologyContribution("ont-none", 0)
	if err != nil || empty.TotalHits != 0 {
		t.Fatalf("空本体画像: %+v err=%v", empty, err)
	}

	rows, err := st.OntologyContributionSummary(10)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("概览应 2 本体，实得 %d", len(rows))
	}
	if rows[0].OntologyID != "ont-a" || rows[0].TotalHits != 4 {
		t.Fatalf("概览排序不符: %+v", rows[0])
	}
	if !strings.Contains(rows[0].LastTool, "") || rows[0].LastTool == "" {
		t.Fatalf("最近工具应非空: %+v", rows[0])
	}
}
