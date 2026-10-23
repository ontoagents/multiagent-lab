package chat

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// hitTestService 最小 Service+会话（recordOntologyHit 单测夹具）。
func hitTestService(t *testing.T) (*Service, *store.Conversation) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ag, err := st.CreateAgent(&store.Agent{Name: "hit-agent"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID})
	if err != nil {
		t.Fatalf("create conv: %v", err)
	}
	return &Service{Store: st, cancels: map[string]context.CancelFunc{}}, conv
}

// REQ-224 H10：recordOntologyHit——facade 工具命中落 ontology_hit + ontology.hit 事件；
// 无 ontology_id 入参不落；失败结果 ok=false 不进 TopN 口径（store 层单测已覆盖聚合）。
func TestRecordOntologyHit(t *testing.T) {
	s, conv := hitTestService(t)
	rc := &runConsumer{toolCallArg: map[string]string{
		"call-1": `{"ontology_id":"ont-x","name":"计算节点"}`,
		"call-2": `{"ontology_id":"ont-x","query":"SELECT ?s WHERE { ?s ?p ?o }"}`,
	}, ctx: t.Context()}
	var events []string
	emit := func(ev *Event) { events = append(events, ev.Type) }

	s.recordOntologyHit(rc, conv, "run-1", &schema.Message{ToolCallID: "call-1", ToolName: "get_concept", Content: `{"label":"计算节点"}`}, emit)

	hits, err := s.Store.OntologyContribution("ont-x", 10)
	if err != nil {
		t.Fatalf("contribution: %v", err)
	}
	if hits.TotalHits != 1 || hits.OKHits != 1 {
		t.Fatalf("命中应 1/1: %+v", hits)
	}
	if len(hits.TopTargets) != 1 || hits.TopTargets[0].Target != "计算节点" {
		t.Fatalf("TopN 应含概念名: %+v", hits.TopTargets)
	}
	if len(events) != 1 || events[0] != "ontology.hit" {
		t.Fatalf("应发一条 ontology.hit 事件: %v", events)
	}
	evs, _, err := s.Store.ListEventsQ(conv.ID, store.EventQuery{Type: "ontology.hit"})
	if err != nil || len(evs) != 1 {
		t.Fatalf("ontology.hit 事件应落库: %v %v", evs, err)
	}

	// 失败结果：ok=false 仍落行（命中口径=调用发生），事件照发
	s.recordOntologyHit(rc, conv, "run-1", &schema.Message{ToolCallID: "call-2", ToolName: "sparql_query", Content: "ONTOLOGY_SERVICE_UNAVAILABLE: x"}, emit)
	hits, _ = s.Store.OntologyContribution("ont-x", 10)
	if hits.TotalHits != 2 || hits.OKHits != 1 {
		t.Fatalf("失败命中应 ok=false: total=%d ok=%d", hits.TotalHits, hits.OKHits)
	}

	// 无 ontology_id：不落不报
	n := len(events)
	s.recordOntologyHit(rc, conv, "run-1", &schema.Message{ToolCallID: "call-3", ToolName: "get_concept", Content: "{}"}, emit)
	if len(events) != n {
		t.Fatalf("无本体归属不应落事件")
	}
}
