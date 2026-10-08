package companion

// REQ-281 伴生沉淀过程事件族单测：抽取决策点事件落 run_event（started/candidates/done/
// error/decision/ingest/reject）与 schema_version=2 契约。LLM 经 openai_compat httptest 桩；
// 入图事件走 smokeBase 冒烟引擎（无 oxigraph 二进制按既有约定 Skip）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// stubExtractLLM 启动 openai_compat 桩（固定抽取 JSON 响应），返回基址。
func stubExtractLLM(t *testing.T, payload string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// eino openai 客户端走 /chat/completions；内容经 strconv.Quote 安全内嵌
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + strconv.Quote(payload) + `}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// openEventsStore 每测试独立 sqlite（事件表随迁移就绪）。
func openEventsStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "ev.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func eventTypesByConv(t *testing.T, st *store.Store, convID string) []map[string]any {
	t.Helper()
	evs, err := st.ListEvents(convID)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, e := range evs {
		var d map[string]any
		_ = json.Unmarshal([]byte(e.Data), &d)
		out = append(out, map[string]any{"type": e.Type, "run_id": e.RunID, "schema": e.SchemaVersion, "data": d})
	}
	return out
}

// TestExtractEventsHappyPath 阈值 0（全人工审）抽取全链：started → candidates（含条目）→ done，
// run_id 贯通、schema_version=2；未触达图引擎（无宿主方案环境约束）。
func TestExtractEventsHappyPath(t *testing.T) {
	payload := `{"concepts":[{"name":"Pod驱逐","definition":"节点资源不足时清除并重建","confidence":0.95,"source":"Pod驱逐"}],"relations":[],"events":[]}`
	srv := stubExtractLLM(t, payload)
	st := openEventsStore(t)
	if _, err := st.CreateConnection(&store.ModelConnection{ID: "conn_stub", Name: "stub", ConnType: "chat", Protocol: "openai_compat", BaseURL: srv.URL, ModelName: "stub", Enabled: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(&store.Agent{ID: "agt_ev", Name: "ev", CompanionOntologyID: "ont_ev", CompanionExtractConnID: "conn_stub"}); err != nil {
		t.Fatal(err)
	}
	conv := &store.Conversation{ID: "conv_ev", Scope: "agent", AgentID: strPtr("agt_ev")}
	if _, err := st.CreateConversation(conv); err != nil {
		t.Fatal(err)
	}
	for _, m := range []*store.Message{
		{ConversationID: conv.ID, Role: "user", Content: "节点资源不足时 Pod 会被驱逐并重建。"},
		{ConversationID: conv.ID, Role: "assistant", Content: "Pod驱逐指节点资源不足时调度器清除并重建 Pod。"},
	} {
		if _, err := st.InsertMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	s := NewService(st, nil, nil)
	n, err := s.ExtractNew(context.Background(), conv.ID, "agt_ev", "run_9")
	if err != nil {
		t.Fatalf("抽取失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("新增候选数=%d want 1", n)
	}
	evs := eventTypesByConv(t, st, conv.ID)
	var gotStarted, gotCandidates, gotDone bool
	for _, e := range evs {
		if e["schema"].(int) != 2 {
			t.Fatalf("伴生事件 schema_version 应为 2: %v", e)
		}
		if e["run_id"] != "run_9" {
			t.Fatalf("run_id 应贯通触发轮: %v", e)
		}
		d := e["data"].(map[string]any)
		switch e["type"] {
		case "companion.extract":
			switch d["phase"] {
			case "started":
				gotStarted = true
				if d["fresh"].(float64) != 2 || d["windows"].(float64) != 1 {
					t.Fatalf("started 事件数据不符: %v", d)
				}
			case "done":
				gotDone = true
				if d["candidates"].(float64) != 1 || d["auto_ingested"].(float64) != 0 {
					t.Fatalf("done 事件数据不符: %v", d)
				}
			default:
				t.Fatalf("不应有其他 phase（阈值 0 无 error）: %v", d)
			}
		case "companion.candidates":
			gotCandidates = true
			if d["count"].(float64) != 1 {
				t.Fatalf("candidates count 不符: %v", d)
			}
			items := d["items"].([]any)
			first := items[0].(map[string]any)
			if first["name"] != "Pod驱逐" || first["kind"] != "concept" {
				t.Fatalf("候选条目不符: %v", first)
			}
		}
	}
	if !gotStarted || !gotCandidates || !gotDone {
		t.Fatalf("事件序列不全 started=%v candidates=%v done=%v", gotStarted, gotCandidates, gotDone)
	}
}

// TestExtractEventsErrorPath 无可用模型连接 → companion.extract error 事件留痕（治静默失败）。
func TestExtractEventsErrorPath(t *testing.T) {
	st := openEventsStore(t)
	if _, err := st.CreateAgent(&store.Agent{ID: "agt_err", Name: "err", CompanionOntologyID: "ont_err"}); err != nil {
		t.Fatal(err)
	}
	conv := &store.Conversation{ID: "conv_err", Scope: "agent", AgentID: strPtr("agt_err")}
	if _, err := st.CreateConversation(conv); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: "触发一条"}); err != nil {
		t.Fatal(err)
	}
	s := NewService(st, nil, nil)
	if _, err := s.ExtractNew(context.Background(), conv.ID, "agt_err", "run_e"); err == nil {
		t.Fatal("无模型连接应报错")
	}
	evs := eventTypesByConv(t, st, conv.ID)
	found := false
	for _, e := range evs {
		if e["type"] == "companion.extract" && e["data"].(map[string]any)["phase"] == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应有 companion.extract error 事件: %v", evs)
	}
}

// TestAutoIngestDecisionEvent 阈值 0.9 + 置信 0.95 → companion.decision 判定留痕 + ingest 事件
// （走冒烟引擎；无 oxigraph 二进制 Skip）。
func TestAutoIngestDecisionEvent(t *testing.T) {
	base := smokeBase(t)
	payload := `{"concepts":[{"name":"滚动更新","definition":"逐批替换实例","confidence":0.95,"source":"滚动更新"}],"relations":[],"events":[]}`
	srv := stubExtractLLM(t, payload)
	t.Setenv("COMPANION_SNAPSHOT_DIR", filepath.Join(t.TempDir(), "snaps"))
	st := openEventsStore(t)
	if _, err := st.CreateConnection(&store.ModelConnection{ID: "conn_stub2", Name: "stub", ConnType: "chat", Protocol: "openai_compat", BaseURL: srv.URL, ModelName: "stub", Enabled: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(&store.Agent{ID: "agt_auto", Name: "auto", CompanionOntologyID: "ont_smoke", CompanionExtractConnID: "conn_stub2", CompanionAutoThreshold: 0.9}); err != nil {
		t.Fatal(err)
	}
	conv := &store.Conversation{ID: "conv_auto", Scope: "agent", AgentID: strPtr("agt_auto")}
	if _, err := st.CreateConversation(conv); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: "滚动更新是逐批替换实例的策略。"}); err != nil {
		t.Fatal(err)
	}
	s := NewService(st, nil, smokePlans(base))
	n, err := s.ExtractNew(context.Background(), conv.ID, "agt_auto", "run_a")
	if err != nil {
		t.Fatalf("抽取失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("候选数=%d want 1", n)
	}
	evs := eventTypesByConv(t, st, conv.ID)
	var gotDecision, gotIngest bool
	for _, e := range evs {
		d := e["data"].(map[string]any)
		switch e["type"] {
		case "companion.decision":
			gotDecision = true
			if d["action"] != "auto_ingest" || d["threshold"].(float64) != 0.9 {
				t.Fatalf("decision 事件不符: %v", d)
			}
		case "companion.ingest":
			gotIngest = true
			if d["mode"] != "auto" || d["result"] != "insert" {
				t.Fatalf("ingest 事件不符: %v", d)
			}
		case "companion.extract":
			if d["phase"] == "done" && d["auto_ingested"].(float64) != 1 {
				t.Fatalf("done auto_ingested 不符: %v", d)
			}
		}
	}
	if !gotDecision || !gotIngest {
		t.Fatalf("自动入图判定/写入事件缺失 decision=%v ingest=%v", gotDecision, gotIngest)
	}
	// 候选应已被自动确认（不再 pending）
	cands, _ := st.ListCompanionCandidates(conv.ID, "agt_auto", "confirmed")
	if len(cands) != 1 {
		t.Fatalf("自动确认后 confirmed=%d want 1", len(cands))
	}
}

// TestRejectEvent 人工拒绝镜像 companion.reject 事件。
func TestRejectEvent(t *testing.T) {
	st := openEventsStore(t)
	if _, err := st.CreateAgent(&store.Agent{ID: "agt_rj", Name: "rj", CompanionOntologyID: "ont_rj"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateCompanionCandidates([]*store.CompanionCandidate{
		{AgentID: "agt_rj", ConversationID: "conv_rj", Kind: "concept", Name: "噪声实体", Status: "pending"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConversation(&store.Conversation{ID: "conv_rj", Scope: "agent", AgentID: strPtr("agt_rj")}); err != nil {
		t.Fatal(err)
	}
	s := NewService(st, nil, nil)
	if _, err := s.RejectCandidate(context.Background(), func() string {
		cands, _ := st.ListCompanionCandidates("conv_rj", "agt_rj", "pending")
		return cands[0].ID
	}()); err != nil {
		t.Fatal(err)
	}
	evs := eventTypesByConv(t, st, "conv_rj")
	found := false
	for _, e := range evs {
		if e["type"] == "companion.reject" {
			d := e["data"].(map[string]any)
			if d["name"] == "噪声实体" && d["mode"] == "manual" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("应有 companion.reject 事件: %v", evs)
	}
}

// TestListEventsTypePrefix 前缀过滤与 LIKE 通配转义（store 层）。
func TestListEventsTypePrefix(t *testing.T) {
	st := openEventsStore(t)
	if _, err := st.CreateAgent(&store.Agent{ID: "agt_c9", Name: "c9", CompanionOntologyID: "ont_c9"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConversation(&store.Conversation{ID: "c9", Scope: "agent", AgentID: strPtr("agt_c9")}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []*store.RunEvent{
		{ConversationID: "c9", Type: "companion.extract", Data: "{}"},
		{ConversationID: "c9", Type: "companion.candidates", Data: "{}"},
		{ConversationID: "c9", Type: "tool.call", Data: "{}"},
	} {
		if _, err := st.InsertEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	evs, total, err := st.ListEventsQ("c9", store.EventQuery{TypePrefix: "companion."})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(evs) != 2 || !strings.HasPrefix(evs[0].Type, "companion.") {
		t.Fatalf("前缀过滤不符: total=%d n=%d", total, len(evs))
	}
	// 通配符不逃逸即误命中——转义后应零命中
	_, total, err = st.ListEventsQ("c9", store.EventQuery{TypePrefix: "companion.%"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("LIKE 通配应被转义（total=%d want 0）", total)
	}
}
