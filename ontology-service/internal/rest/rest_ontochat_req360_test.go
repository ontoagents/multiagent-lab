package rest

// REQ-360/M161 D 产出清单数据源：GET /api/ontochat/sessions?agent_id= 按归属智能体过滤
// （REQ-354/P2·G4 审计两列的消费闭环）；缺省全量（页面流程列表行为零变化）。

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestListOntoChatSessionsByAgent(t *testing.T) {
	srv, _ := newOntoChatP2TestServer(t)

	// 两个归属智能体各建一会话 + 一条页面流程会话（无归属）
	if status, body := postSession(t, srv, `{"title":"甲的会话","source":"agent_chat","agent_id":"ag_A"}`); status != 201 {
		t.Fatalf("建会话失败 %d %s", status, body)
	}
	if status, body := postSession(t, srv, `{"title":"乙的会话","source":"agent_chat","agent_id":"ag_B"}`); status != 201 {
		t.Fatalf("建会话失败 %d %s", status, body)
	}
	if status, body := postSession(t, srv, `{"title":"页面会话"}`); status != 201 {
		t.Fatalf("建会话失败 %d %s", status, body)
	}

	// agent_id 过滤：只归还该智能体的会话
	resp, err := http.Get(srv.URL + "/api/ontochat/sessions?agent_id=ag_A")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("过滤列表应 200，实际 %d %s", resp.StatusCode, string(raw))
	}
	var list []struct {
		Title   string `json:"title"`
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Title != "甲的会话" || list[0].AgentID != "ag_A" {
		t.Fatalf("agent_id=ag_A 应恰一条归属会话，实际 %v", list)
	}

	// 缺省全量：三条（页面流程列表零回归）
	resp2, _ := http.Get(srv.URL + "/api/ontochat/sessions")
	var all []json.RawMessage
	_ = json.NewDecoder(resp2.Body).Decode(&all)
	resp2.Body.Close()
	if len(all) != 3 {
		t.Fatalf("缺省应全量 3 条，实际 %d", len(all))
	}
}
