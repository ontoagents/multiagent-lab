// REQ-355 R1/M155 缝合测试：KBSource 接口反转后的 recallKB 行为——
// ①stub 源检索命中→retrieval 事件（hits/spans/sources/mode 透传）+ 注入文本 System 消息；
// ②graphrag 模式明细透传；③nil 源=未接入降级（跳过、零事件、消息原样）；④检索错误降级 warning。
package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

type stubKBSource struct {
	text     string
	detail   *KBGraphragDetail
	mode     string
	degraded bool
	err      error
	calls    int
}

func (s *stubKBSource) RetrieveContext(_ context.Context, _ *store.KnowledgeBase, _ string, _ int, _ float64) (string, *KBGraphragDetail, string, bool, error) {
	s.calls++
	return s.text, s.detail, s.mode, s.degraded, s.err
}

func newRecallFixture(t *testing.T) (*Service, *store.Conversation) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/recall.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	kb, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "缝合库", Mode: "rag"})
	if err != nil {
		t.Fatalf("create kb: %v", err)
	}
	svc := NewService(st, nil, nil)
	conv := &store.Conversation{ID: "conv-recall", Scope: "agent", EnableKB: true, KBID: strPtr(kb.ID)}
	return svc, conv
}

func TestRecallKBSourceSeam(t *testing.T) {
	svc, conv := newRecallFixture(t)
	stub := &stubKBSource{
		text: "【知识库】缝合命中内容",
		detail: &KBGraphragDetail{
			Hits: []KBHit{
				{Doc: "docs/a.md", Seq: 2, Score: 0.83, Excerpt: "命中片段", Spans: []map[string]int{{"start": 0, "end": 4}}, Sources: []map[string]any{{"doc": "docs/a.md", "seq": 2}}},
			},
			Entities: []map[string]any{{"name": "实体A"}},
			Claims:   []map[string]any{{"text": "断言1"}},
		},
		mode: "graphrag", degraded: true,
	}
	svc.KB = stub
	var events []*Event
	msgs := svc.recallKB(context.Background(), conv, nil, "run-1", "问题", nil, func(ev *Event) { events = append(events, ev) })

	if stub.calls != 1 {
		t.Fatalf("检索源应被调用 1 次，got %d", stub.calls)
	}
	if len(events) != 1 || events[0].Type != "retrieval" {
		t.Fatalf("应恰好发一条 retrieval 事件，got %+v", events)
	}
	var data map[string]any
	if err := json.Unmarshal(events[0].Data, &data); err != nil {
		t.Fatalf("retrieval data 解析: %v", err)
	}
	if data["mode"] != "graphrag" || data["degraded"] != true {
		t.Errorf("mode/degraded 透传不符: %v", data)
	}
	hits, _ := data["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("hits 应 1 条: %v", data["hits"])
	}
	h0, _ := hits[0].(map[string]any)
	if h0["doc"] != "docs/a.md" || h0["spans"] == nil || h0["sources"] == nil {
		t.Errorf("命中投影字段不符（doc/spans/sources 透传）: %v", h0)
	}
	if data["entities"] == nil || data["claims"] == nil {
		t.Errorf("graphrag 明细应透传: %v", data)
	}
	if len(msgs) != 1 || !strings.Contains(msgs[0].Content, "缝合命中内容") {
		t.Fatalf("注入文本应为 System 消息: %+v", msgs)
	}
	if msgs[0].Role != "system" {
		t.Errorf("注入消息 role=system，got %s", msgs[0].Role)
	}
}

func TestRecallKBNilSourceSkip(t *testing.T) {
	svc, conv := newRecallFixture(t) // KB=nil（未接入）
	var events []*Event
	msgs := svc.recallKB(context.Background(), conv, nil, "run-1", "问题", nil, func(ev *Event) { events = append(events, ev) })
	if len(events) != 0 || len(msgs) != 0 {
		t.Fatalf("nil 源应零事件零注入，got events=%d msgs=%d", len(events), len(msgs))
	}
}

func TestRecallKBSourceErrorDegraded(t *testing.T) {
	svc, conv := newRecallFixture(t)
	svc.KB = &stubKBSource{err: context.DeadlineExceeded}
	var events []*Event
	msgs := svc.recallKB(context.Background(), conv, nil, "run-1", "问题", nil, func(ev *Event) { events = append(events, ev) })
	if len(events) != 1 || events[0].Type != "run.warning" {
		t.Fatalf("检索失败应降级 run.warning，got %+v", events)
	}
	if len(msgs) != 0 {
		t.Fatalf("失败不注入，got %d 条", len(msgs))
	}
}
