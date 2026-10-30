package companion

// REQ-389/M185 单测：图状态实验件——TTL 转义合法性/词法判定/投影三元组构造与幂等 UPDATE 序列
//（ Plans 桩捕获 DROP/INSERT 语句，宿主引擎不启动——引擎链路由 companion 既有测试承载）。

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestTTLLiteralEscapes(t *testing.T) {
	got := ttlLiteral("说\"引\"与\\反斜\n换行\t制表")
	want := `"说\"引\"与\\反斜\n换行\t制表"`
	if got != want {
		t.Fatalf("TTL 转义不符: got %s want %s", got, want)
	}
}

func TestFirstQueryKind(t *testing.T) {
	cases := map[string]string{
		"SELECT ?s WHERE {}":                                             "select",
		"# 注释\nPREFIX state: <urn:state:>\nASK {}":                       "ask",
		"PREFIX e: <urn:e>\nDELETE WHERE { ?s ?p ?o }":                   "delete",
		"INSERT DATA { <a> <b> <c> }":                                    "insert",
		"  \nSELECT * { ?s ?p ?o }":                                      "select",
	}
	for q, want := range cases {
		if got := firstQueryKind(q); got != want {
			t.Fatalf("firstQueryKind(%q)=%s want %s", q, got, want)
		}
	}
}

func TestSyncRunStateProjection(t *testing.T) {
	st, err := store.Open(t.TempDir()+"/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	// 项目+成员 agent+会话（带 todo）+委派
	proj, err := st.CreateProject(&store.Project{Name: "gs-proj"})
	if err != nil {
		t.Fatal(err)
	}
	member, err := st.CreateAgent(&store.Agent{Name: "gs-worker"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Scope: "project", ProjectID: &proj.ID, AgentID: &member.ID, Title: "状态会话"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveConversationTodo(conv.ID, `[{"title":"写代码","status":"done"},{"title":"写测试","status":"in_progress"}]`); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateDelegate(&store.Delegate{ParentConversationID: conv.ID, ChildConversationID: conv.ID, MemberAgentID: member.ID, MemberName: "gs-worker", Task: "长任务一", Status: "running"}); err != nil {
		t.Fatal(err)
	}

	// httptest 引擎桩：/update 捕获 SPARQL；/query 返回空结果集（ensureInflated 探针走此路径）
	var updates []string
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/update":
			b, _ := io.ReadAll(r.Body)
			updates = append(updates, string(b))
			w.WriteHeader(http.StatusOK)
		case "/query":
			w.Header().Set("Content-Type", "application/sparql-results+json")
			_, _ = w.Write([]byte(`{"results":{"bindings":[]}}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(engine.Close)
	svc := NewService(st, nil, nil)
	pe := NewPlanEngines(engine.URL, engine.URL)
	pe.HTTP = engine.Client()
	pe.mu.Lock()
	pe.eps["onto_state_x"] = engine.URL // 预置引擎缓存——跳过 EnsureHostPlan 建方案链路
	pe.mu.Unlock()
	svc.Plans = pe
	rep, err := svc.SyncRunState(context.Background(), proj.ID, "onto_state_x")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Conversations != 1 || rep.Todos != 2 || rep.Delegates != 1 {
		t.Fatalf("投影计数不符: %+v", rep)
	}
	if len(updates) != 2 || !strings.HasPrefix(updates[0], "DROP GRAPH <urn:state:run>") || !strings.HasPrefix(updates[1], "INSERT DATA { GRAPH <urn:state:run>") {
		t.Fatalf("应先 DROP 后 INSERT 幂等重建: %d 条 %q", len(updates), updates[0])
	}
	insert := updates[1]
	for _, frag := range []string{"state:Conversation", "state:Todo ; state:title \"写代码\" ; state:status \"done\"", "state:Delegate", "urn:state:conv/"} {
		if !strings.Contains(insert, frag) {
			t.Fatalf("INSERT 应含 %q", frag)
		}
	}
	// 空项目：零投影且不触发 UPDATE（诚实空态）
	rep2, err := svc.SyncRunState(context.Background(), "proj_empty", "onto_state_x")
	if err != nil || rep2.Conversations != 0 || len(updates) != 2 {
		t.Fatalf("空项目应零投影零写: %+v err=%v", rep2, err)
	}
}

func TestQueryRunStateRejectsUpdate(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := &Service{Store: st, Plans: &PlanEngines{}}
	if _, err := svc.QueryRunState(context.Background(), "o", "DELETE WHERE { ?s ?p ?o }"); err == nil {
		t.Fatal("UPDATE 应被白名单拒绝")
	}
	if _, err := svc.QueryRunState(context.Background(), "o", ""); err == nil {
		t.Fatal("空查询应拒绝")
	}
}
