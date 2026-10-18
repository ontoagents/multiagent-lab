package rest

// REQ-354/P2 交付测试：①OntoChat 会话归属审计两列（G4，迁移 046——source/agent_id 建会话传入、
// 缺省 page、白名单外值归 page、List 透出）；②会话草稿 TTL 导出端点（ontochat_export_ttl 工具
// 数据源——无草稿 400 / 不存在 404 / 草稿 TTL 直出带 X-Onto-Triples 与 X-Onto-Source: ontochat_draft）。
// 临时 SQLite + httptest，零外部依赖（草稿经 Store.UpdateContext 注入，不经 LLM）。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/ontochat"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
)

func newOntoChatP2TestServer(t *testing.T) (*httptest.Server, *repo.Store) {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "ontochat_p2_test.db"), "../../migrations")
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mux := http.NewServeMux()
	New(st, nil, nil).Mount(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

func postSession(t *testing.T, srv *httptest.Server, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/ontochat/sessions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestOntoChatSessionAuditColumns(t *testing.T) {
	srv, _ := newOntoChatP2TestServer(t)

	// agent_chat 来源：ontochat_start 工具形态（source+agent_id 显式传入）
	status, body := postSession(t, srv, `{"title":"智能体访谈","mode":"quick","source":"agent_chat","agent_id":"ag_123"}`)
	var created struct {
		ID      string `json:"id"`
		Source  string `json:"source"`
		AgentID string `json:"agent_id"`
	}
	_ = json.Unmarshal([]byte(body), &created)
	if status != http.StatusCreated || created.ID == "" {
		t.Fatalf("期望 201 带 id，实际 %d %s", status, body)
	}
	if created.Source != "agent_chat" || created.AgentID != "ag_123" {
		t.Fatalf("审计两列期望 agent_chat/ag_123，实际 %q/%q", created.Source, created.AgentID)
	}

	// GET 回读一致
	get, _ := http.Get(srv.URL + "/api/ontochat/sessions/" + created.ID)
	var got struct {
		Source  string `json:"source"`
		AgentID string `json:"agent_id"`
	}
	_ = json.NewDecoder(get.Body).Decode(&got)
	get.Body.Close()
	if got.Source != "agent_chat" || got.AgentID != "ag_123" {
		t.Fatalf("GET 回读审计两列不匹配: %+v", got)
	}

	// 页面流程缺省：不传 source → page；白名单外值归 page
	for i, reqBody := range []string{`{"title":"页面会话"}`, `{"title":"坏值","source":"hacker"}`} {
		status, body := postSession(t, srv, reqBody)
		var s2 struct {
			Source string `json:"source"`
		}
		_ = json.Unmarshal([]byte(body), &s2)
		if status != http.StatusCreated || s2.Source != "page" {
			t.Fatalf("用例 %d 期望 201 且缺省归 page，实际 %d %q", i, status, s2.Source)
		}
	}

	// List 透出审计两列（审计查询「哪个智能体产了哪些本体」的取数口）
	lr, _ := http.Get(srv.URL + "/api/ontochat/sessions")
	var list []map[string]any
	_ = json.NewDecoder(lr.Body).Decode(&list)
	lr.Body.Close()
	var sawAgentChat bool
	for _, it := range list {
		if it["source"] == "agent_chat" && it["agent_id"] == "ag_123" {
			sawAgentChat = true
		}
	}
	if !sawAgentChat {
		t.Fatalf("列表未透出 agent_chat 审计行: %v", list)
	}
}

func TestOntoChatSessionExport(t *testing.T) {
	srv, st := newOntoChatP2TestServer(t)

	status, body := postSession(t, srv, `{"title":"导出测试","source":"agent_chat","agent_id":"ag_export"}`)
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &created)
	if status != http.StatusCreated || created.ID == "" {
		t.Fatalf("建会话失败: %d %s", status, body)
	}

	// 无草稿 → 400 人话报错
	r400, _ := http.Get(srv.URL + "/api/ontochat/sessions/" + created.ID + "/export")
	b400, _ := io.ReadAll(r400.Body)
	r400.Body.Close()
	if r400.StatusCode != http.StatusBadRequest || !strings.Contains(string(b400), "尚无草稿") {
		t.Fatalf("无草稿期望 400 尚无草稿，实际 %d %q", r400.StatusCode, string(b400))
	}

	// 404：不存在会话
	r404, _ := http.Get(srv.URL + "/api/ontochat/sessions/no_such/export")
	r404.Body.Close()
	if r404.StatusCode != http.StatusNotFound {
		t.Fatalf("不存在会话期望 404，实际 %d", r404.StatusCode)
	}

	// 注入草稿（绕 LLM：UpdateContext 直写 DraftSpec）→ TTL 直出
	draft := map[string]any{
		"name":        "导出测试本体",
		"description": "P2 导出端点夹具",
		"concepts":    []map[string]any{{"name": "设备", "definition": "物理装置"}, {"name": "传感器", "parents": []string{"设备"}}},
		"relations":   []map[string]any{{"name": "包含", "from": "设备", "to": "传感器"}},
		"instances":   []map[string]any{},
	}
	raw, _ := json.Marshal(draft)
	ocs := ontochat.New(st.DB())
	if err := ocs.UpdateContext(created.ID, func(c *ontochat.Context) {
		rawCopy := json.RawMessage(append([]byte(nil), raw...))
		c.DraftSpec = &rawCopy
	}); err != nil {
		t.Fatalf("注入草稿失败: %v", err)
	}

	rex, err := http.Get(srv.URL + "/api/ontochat/sessions/" + created.ID + "/export")
	if err != nil {
		t.Fatalf("导出请求失败: %v", err)
	}
	defer rex.Body.Close()
	if rex.StatusCode != http.StatusOK {
		t.Fatalf("导出期望 200，实际 %d", rex.StatusCode)
	}
	if ct := rex.Header.Get("Content-Type"); !strings.Contains(ct, "text/turtle") {
		t.Fatalf("Content-Type 期望 text/turtle，实际 %q", ct)
	}
	if src := rex.Header.Get("X-Onto-Source"); src != "ontochat_draft" {
		t.Fatalf("X-Onto-Source 期望 ontochat_draft，实际 %q", src)
	}
	if rex.Header.Get("X-Onto-Triples") == "" {
		t.Fatalf("缺 X-Onto-Triples 计数头")
	}
	ttlBytes, _ := io.ReadAll(rex.Body)
	ttl := string(ttlBytes)
	for _, want := range []string{"设备", "传感器", "包含"} {
		if !strings.Contains(ttl, want) {
			t.Fatalf("TTL 缺元素 %q：\n%s", want, ttl)
		}
	}
}
