package companion

// REQ-281 伴生沉淀过程事件族单测：抽取决策点事件落 run_event（started/candidates/done/
// error/decision/ingest/reject）与 schema_version=2 契约。LLM 经 openai_compat httptest 桩；
// 入图事件走 smokeBase 冒烟引擎（无 oxigraph 二进制按既有约定 Skip）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

// ---- REQ-282：超时治理与长抽取不卡顿 ----

// mustAgent 测试辅助：取 agent（OnRunComplete 需 *store.Agent）。
func mustAgent(t *testing.T, st *store.Store, id string) *store.Agent {
	t.Helper()
	a, err := st.GetAgent(id)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestExtractTimeoutEnv A1：COMPANION_EXTRACT_TIMEOUT 秒解析与默认值。
func TestExtractTimeoutEnv(t *testing.T) {
	t.Setenv("COMPANION_EXTRACT_TIMEOUT", "")
	if got := extractTimeout(); got != 300*time.Second {
		t.Fatalf("默认预算=%v want 300s", got)
	}
	t.Setenv("COMPANION_EXTRACT_TIMEOUT", "600")
	if got := extractTimeout(); got != 600*time.Second {
		t.Fatalf("env 预算=%v want 600s", got)
	}
	t.Setenv("COMPANION_EXTRACT_TIMEOUT", "abc")
	if got := extractTimeout(); got != 300*time.Second {
		t.Fatalf("非法 env 应回落默认: %v", got)
	}
	t.Setenv("COMPANION_EXTRACT_TIMEOUT", "-5")
	if got := extractTimeout(); got != 300*time.Second {
		t.Fatalf("非正数 env 应回落默认: %v", got)
	}
}

// TestStartedCarriesWindowBudget B4：started 事件带每窗预算秒数。
func TestStartedCarriesWindowBudget(t *testing.T) {
	payload := `{"concepts":[],"relations":[],"events":[]}`
	srv := stubExtractLLM(t, payload)
	st := openEventsStore(t)
	if _, err := st.CreateConnection(&store.ModelConnection{ID: "conn_b4", Name: "stub", ConnType: "chat", Protocol: "openai_compat", BaseURL: srv.URL, ModelName: "stub", Enabled: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(&store.Agent{ID: "agt_b4", Name: "b4", CompanionOntologyID: "ont_b4", CompanionExtractConnID: "conn_b4"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConversation(&store.Conversation{ID: "conv_b4", Scope: "agent", AgentID: strPtr("agt_b4")}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertMessage(&store.Message{ConversationID: "conv_b4", Role: "user", Content: "一句话事实。"}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(st, nil, nil).ExtractNew(context.Background(), "conv_b4", "agt_b4", "run_b4"); err != nil {
		t.Fatal(err)
	}
	evs := eventTypesByConv(t, st, "conv_b4")
	for _, e := range evs {
		if e["type"] == "companion.extract" && e["data"].(map[string]any)["phase"] == "started" {
			if _, ok := e["data"].(map[string]any)["window_budget"]; !ok {
				t.Fatalf("started 事件应带 window_budget: %v", e["data"])
			}
			return
		}
	}
	t.Fatal("缺 started 事件")
}

// TestPendingExtractCoalesce B1：抽取进行中的新收尾置补抽标记（不再静默丢弃），
// extractRounds 收尾后按标记再跑一轮（游标保证只抽增量），限补抽 1 轮。
func TestPendingExtractCoalesce(t *testing.T) {
	var calls int32
	callCount := &calls
	payload := `{"concepts":[],"relations":[],"events":[]}`
	srv := stubExtractLLM(t, payload)
	st := openEventsStore(t)
	if _, err := st.CreateConnection(&store.ModelConnection{ID: "conn_co", Name: "stub", ConnType: "chat", Protocol: "openai_compat", BaseURL: srv.URL, ModelName: "stub", Enabled: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(&store.Agent{ID: "agt_co", Name: "co", CompanionOntologyID: "ont_co", CompanionExtractConnID: "conn_co"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConversation(&store.Conversation{ID: "conv_co", Scope: "agent", AgentID: strPtr("agt_co")}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertMessage(&store.Message{ConversationID: "conv_co", Role: "user", Content: "第一轮消息。"}); err != nil {
		t.Fatal(err)
	}
	s := NewService(st, nil, nil)
	// 模拟抽取进行中：running 占位 + 新收尾到达 → 应置补抽标记并静默返回
	s.mu.Lock()
	s.running["conv_co"] = true
	s.mu.Unlock()
	conv := &store.Conversation{ID: "conv_co", Scope: "agent", AgentID: strPtr("agt_co")}
	s.OnRunComplete(conv, mustAgent(t, st, "agt_co"), "run_skip")
	s.mu.Lock()
	pending, hasPending := s.pendingExtract["conv_co"]
	s.mu.Unlock()
	if !hasPending || pending != "run_skip" {
		t.Fatalf("冲突收尾应置补抽标记: %v %v", pending, hasPending)
	}
	// 第二次冲突收尾覆盖标记（取最新触发轮）
	s.OnRunComplete(conv, mustAgent(t, st, "agt_co"), "run_skip2")
	// extractRounds：主轮（游标已空 → 抽取第一条消息）+ 补抽轮，至多 2 轮
	_ = callCount
	s.extractRounds(conv, mustAgent(t, st, "agt_co"), "run_first")
	s.mu.Lock()
	_, still := s.pendingExtract["conv_co"]
	s.mu.Unlock()
	if still {
		t.Fatal("extractRounds 收尾应消费补抽标记")
	}
}

// TestConflictCheckTimeoutNote A3/B3：冲突检测超时 → 候选 note 如实标注（保留双方语义不变）。
func TestConflictCheckTimeoutNote(t *testing.T) {
	base := smokeBase(t)
	old := conflictCheckBudget
	conflictCheckBudget = 300 * time.Millisecond
	t.Cleanup(func() { conflictCheckBudget = old })
	// 慢 LLM 桩：冲突判定请求挂起超过收缩后的预算
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	t.Cleanup(srv.Close)
	st := openEventsStore(t)
	if _, err := st.CreateConnection(&store.ModelConnection{ID: "conn_slow", Name: "slow", ConnType: "chat", Protocol: "openai_compat", BaseURL: srv.URL, ModelName: "slow", Enabled: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(&store.Agent{ID: "agt_ct", Name: "ct", CompanionOntologyID: "ont_smoke", CompanionExtractConnID: "conn_slow"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateCompanionCandidates([]*store.CompanionCandidate{
		{AgentID: "agt_ct", ConversationID: "conv_ct", Kind: "relation", Name: "滚动更新", RelName: "引发", RelTarget: "告警", Confidence: 0.9, Status: "pending"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConversation(&store.Conversation{ID: "conv_ct", Scope: "agent", AgentID: strPtr("agt_ct")}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(st, nil, smokePlans(base))
	// 首次确认：建边（无活跃旧边 → insert，语义检测对空清单跳过）
	cands, _ := st.ListCompanionCandidates("conv_ct", "agt_ct", "pending")
	if _, err := svc.ConfirmCandidate(context.Background(), cands[0].ID, "manual"); err != nil {
		t.Fatalf("首次确认失败: %v", err)
	}
	// 第二次确认同主体+不同关系名（部署≠引发）→ 无确定性冲突，语义检测对既有「引发」活跃边
	// 做 LLM 二分类（慢桩将超时 → note 标注 + 入图继续不受阻）
	if err := st.CreateCompanionCandidates([]*store.CompanionCandidate{
		{AgentID: "agt_ct", ConversationID: "conv_ct", Kind: "relation", Name: "滚动更新", RelName: "部署", RelTarget: "回滚", Confidence: 0.9, Status: "pending"},
	}); err != nil {
		t.Fatal(err)
	}
	cands2, _ := st.ListCompanionCandidates("conv_ct", "agt_ct", "pending")
	got, err := svc.ConfirmCandidate(context.Background(), cands2[0].ID, "manual")
	if err != nil {
		t.Fatalf("超时不应阻断入图: %v", err)
	}
	if got.Status != "confirmed" {
		t.Fatalf("超时候选应照常确认: %s", got.Status)
	}
	if !strings.Contains(got.Note, "矛盾检测超时未判定") {
		t.Fatalf("候选应带超时 note: %q", got.Note)
	}
}

// ---- REQ-283：伴生抽取质量加固与持久化兜底 ----

// TestPruneCompanionCandidates A：自指关系丢弃/批内去重保最高置信/悬空端点注记。
func TestPruneCompanionCandidates(t *testing.T) {
	cands := []*store.CompanionCandidate{
		{Kind: "concept", Name: "Pod驱逐", Confidence: 0.9},
		{Kind: "concept", Name: "调度器", Confidence: 0.8},
		// 自指：source==target → 丢弃
		{Kind: "relation", Name: "Pod驱逐", RelName: "终止且清除", RelTarget: "Pod驱逐", Confidence: 0.95},
		// 批内重复：同名同关系同客体 → 保留更高置信（0.92），丢 0.80
		{Kind: "relation", Name: "Pod驱逐", RelName: "依赖", RelTarget: "调度器", Confidence: 0.80},
		{Kind: "relation", Name: "Pod驱逐", RelName: "依赖", RelTarget: "调度器", Confidence: 0.92},
		// 悬空端点：ReplicaSet 既不在本批概念也不在 known → note 注记（不丢弃）
		{Kind: "relation", Name: "调度器", RelName: "编排", RelTarget: "ReplicaSet", Confidence: 0.7},
	}
	kept, detail := pruneCompanionCandidates(cands, []string{"HPA"})
	if len(kept) != 4 {
		t.Fatalf("存活候选=%d want 4（自指 1+重复 1 被剪）", len(kept))
	}
	for _, c := range kept {
		if c.Kind == "relation" && c.Name == c.RelTarget {
			t.Fatal("自指关系应被剪除")
		}
	}
	// 重复组保留 0.92 那条
	foundHigh, foundLow := false, false
	for _, c := range kept {
		if c.Kind == "relation" && c.RelName == "依赖" {
			if c.Confidence == 0.92 {
				foundHigh = true
			}
			if c.Confidence == 0.80 {
				foundLow = true
			}
		}
	}
	if !foundHigh || foundLow {
		t.Fatalf("去重应保留最高置信: high=%v low=%v", foundHigh, foundLow)
	}
	// 悬空端点注记（HPA 在 known → 非悬空；ReplicaSet 悬空）
	noted := 0
	for _, c := range kept {
		if c.RelTarget == "ReplicaSet" && strings.Contains(c.Note, "端点由关系薄建") {
			noted++
		}
	}
	if noted != 1 {
		t.Fatalf("悬空端点应恰有一条注记: %d", noted)
	}
	// 剪除明细含两类原因
	reasons := map[string]bool{}
	for _, d := range detail {
		reasons[d["reason"].(string)] = true
	}
	if !reasons["自指关系（source==target）"] {
		t.Fatalf("明细应含自指原因: %v", detail)
	}
	hasDup := false
	for r := range reasons {
		if strings.HasPrefix(r, "批内重复") {
			hasDup = true
		}
	}
	if !hasDup {
		t.Fatalf("明细应含批内重复原因: %v", detail)
	}
}

// TestPromptFewShot B：范例与硬性标准恒在（hint 为空也不缺）。
func TestPromptFewShot(t *testing.T) {
	p := companionPrompt("语料", "", "")
	if !strings.Contains(p, "抽取范例") || !strings.Contains(p, "自指关系") || !strings.Contains(p, "time_scope") {
		t.Fatal("few-shot 范例与硬性标准应恒在")
	}
	// 既有 hint/alignment 行为不回归
	p2 := companionPrompt("语料", "聚焦K8s", "已有实体清单约束节")
	if !strings.Contains(p2, "聚焦K8s") || !strings.Contains(p2, "已有实体清单约束节") {
		t.Fatal("hint/alignment 注入不回归")
	}
}

// TestSnapshotBackfillOnMissing D：快照缺失而子图非空 → EnsureHost 读路径补拍快照。
func TestSnapshotBackfillOnMissing(t *testing.T) {
	base := smokeBase(t)
	t.Setenv("COMPANION_SNAPSHOT_DIR", filepath.Join(t.TempDir(), "snaps"))
	st := openEventsStore(t)
	if _, err := st.CreateAgent(&store.Agent{ID: "snap-agt2", Name: "snap2", CompanionOntologyID: "ont_smoke"}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(st, nil, smokePlans(base))
	// 确认入图（写路径快照已落）→ 删快照 + 清基址标记 → 模拟「数据只在引擎里」
	cands := []*store.CompanionCandidate{
		{AgentID: "snap-agt2", ConversationID: "c1", Kind: "concept", Name: "滚动更新", Definition: "逐批替换", Confidence: 0.9, Status: "pending"},
	}
	if err := st.CreateCompanionCandidates(cands); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmCandidate(context.Background(), cands[0].ID, "manual"); err != nil {
		t.Fatal(err)
	}
	snap := snapshotPath("ont_smoke")
	if _, err := os.Stat(snap); err != nil {
		t.Fatal("确认后应有快照")
	}
	if err := os.Remove(snap); err != nil {
		t.Fatal(err)
	}
	svc.infMu.Lock()
	svc.inflatedBase = map[string]string{}
	svc.infMu.Unlock()
	// 读路径 EnsureHost → 快照缺失而子图非空 → 补拍
	if _, err := svc.EnsureHost(context.Background(), "ont_smoke"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(snap); err != nil {
		t.Fatal("读路径应已补拍快照（D 兜底）")
	}
}
