package manager

// REQ-540/M347 版本 pin 装载单测（httptest 假构建平面，零外部依赖——沿 closure_test.go 先例）：
// ①expandClosure 收集 pin（同目标多登记取最低、不限 ref_kind、无 pin 不入表）+装载日志呈现；
// ②FetchTTLWithCountAt 按 pin 附加 version 参数 + pin 快照缺失 404 硬失败（钉死语义显式可见）。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExpandClosure_PinCollect_Req540(t *testing.T) {
	mux := http.NewServeMux()
	// A：alignment pin B@3（非 import/reuse 也收集=登记即 pin 载体）；import C@5 与 reuse C@2 → 取最低 2
	mux.HandleFunc("/api/ontologies/A/external-refs", refStub(
		`[{"from_concept":"X","to_onto_id":"B","to_concept":"b1","ref_kind":"alignment","pin_version":3},
		  {"from_concept":"Y","to_onto_id":"C","to_concept":"c1","ref_kind":"import","pin_version":5},
		  {"from_concept":"Z","to_onto_id":"C","to_concept":"c2","ref_kind":"reuse","pin_version":2},
		  {"from_concept":"W","to_onto_id":"D","to_concept":"d1","ref_kind":"import"}]`))
	mux.HandleFunc("/api/ontologies/B/external-refs", refStub(`[]`))
	mux.HandleFunc("/api/ontologies/C/external-refs", refStub(
		`[{"from_concept":"(ontology)","to_onto_id":"B","to_concept":"(ontology)","ref_kind":"import"}]`))
	mux.HandleFunc("/api/ontologies/D/external-refs", refStub(`[]`))
	svc := httptest.NewServer(mux)
	defer svc.Close()
	m := &Manager{HTTP: &http.Client{}, BuildURL: svc.URL, LogDir: t.TempDir()}

	rep, err := m.expandClosure([]string{"A"})
	if err != nil {
		t.Fatalf("expandClosure: %v", err)
	}
	// B 经 C 的 import 进闭包（alignment 不走闭包边=既有语义；pin 收集不受 ref_kind 限制）
	if strings.Join(rep.Effective, ",") != "A,C,D,B" {
		t.Fatalf("闭包清单期望 A,C,D,B，实际 %v", rep.Effective)
	}
	if rep.Pins["B"] != 3 {
		t.Fatalf("B pin 期望 3，实际 %v", rep.Pins)
	}
	if rep.Pins["C"] != 2 {
		t.Fatalf("C pin 应取最低 2，实际 %v", rep.Pins)
	}
	if _, ok := rep.Pins["D"]; ok {
		t.Fatalf("无 pin 登记（D）不应入 pin 表: %v", rep.Pins)
	}
	// 评审修正：C 有 v5/v2 两条登记——冲突集显式呈现（悬空检测按各登记自身 pin 判、
	// 装载按最低取内容，分歧面不静默）
	if rep.PinConflicts["C"] != "v2/v5" {
		t.Fatalf("C pin 冲突集期望 v2/v5，实际 %q", rep.PinConflicts["C"])
	}
	if _, ok := rep.PinConflicts["B"]; ok {
		t.Fatalf("单版本 pin（B）不应入冲突集: %v", rep.PinConflicts)
	}
	joined := strings.Join(rep.logLines(), "\n")
	if !strings.Contains(joined, "[REQ-540 pin 装载] B → v3") || !strings.Contains(joined, "[REQ-540 pin 装载] C → v2") {
		t.Fatalf("装载日志应呈现 pin:\n%s", joined)
	}
	if !strings.Contains(joined, "pin 冲突: C 多登记 pin v2/v5——按最低 v2 装载") {
		t.Fatalf("装载日志应呈现 pin 冲突:\n%s", joined)
	}
}

// 二轮评审修正：PinConflicts 从访问集全部登记收集（不限 ref_kind）——纯 alignment 目标
// 不 import 不入闭包不装载，冲突行措辞不得断言「按最低装载」。
func TestExpandClosure_PinConflictNotLoaded_Req540(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ontologies/A/external-refs", refStub(
		`[{"from_concept":"P","to_onto_id":"E","to_concept":"e1","ref_kind":"alignment","pin_version":1},
		  {"from_concept":"Q","to_onto_id":"E","to_concept":"e2","ref_kind":"alignment","pin_version":4}]`))
	mux.HandleFunc("/api/ontologies/E/external-refs", refStub(`[]`))
	svc := httptest.NewServer(mux)
	defer svc.Close()
	m := &Manager{HTTP: &http.Client{}, BuildURL: svc.URL, LogDir: t.TempDir()}
	rep, err := m.expandClosure([]string{"A"})
	if err != nil {
		t.Fatalf("expandClosure: %v", err)
	}
	if rep.PinConflicts["E"] != "v1/v4" {
		t.Fatalf("E pin 冲突集期望 v1/v4，实际 %q", rep.PinConflicts["E"])
	}
	for _, oid := range rep.Effective {
		if oid == "E" {
			t.Fatalf("alignment 目标不应入装载清单: %v", rep.Effective)
		}
	}
	joined := strings.Join(rep.logLines(), "\n")
	if !strings.Contains(joined, "pin 冲突: E 多登记 pin v1/v4——目标不在本次装载清单") {
		t.Fatalf("未装载目标的冲突行应分流措辞不断言装载:\n%s", joined)
	}
	if strings.Contains(joined, "按最低 v2 装载；较高") && strings.Contains(joined, "E") {
		t.Fatalf("未装载目标不应出现「按最低装载」断言:\n%s", joined)
	}
}

func TestFetchTTLWithCountAt_Req540(t *testing.T) {
	var gotVersion string
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.URL.Query().Get("version")
		w.Header().Set("X-Onto-Triples", "7")
		_, _ = w.Write([]byte("<urn:pinned> <urn:b> <urn:c> ."))
	}))
	defer svc.Close()
	m := &Manager{HTTP: &http.Client{}, BuildURL: svc.URL}
	ttl, n, err := m.FetchTTLWithCountAt("o1", 3)
	if err != nil || n != 7 || ttl == "" {
		t.Fatalf("pin 版本导出期望成功: n=%d err=%v", n, err)
	}
	if gotVersion != "3" {
		t.Fatalf("pin 装载应附加 version=3 参数，实际 %q", gotVersion)
	}
	// version=0 退化头版本请求（无 version 参数，向后兼容）
	if _, _, err := m.FetchTTLWithCountAt("o1", 0); err != nil {
		t.Fatal(err)
	}
	if gotVersion != "" {
		t.Fatalf("浮动头版本不应带 version 参数，实际 %q", gotVersion)
	}
}

func TestFetchTTLAtPinSnapshotMissing_Req540(t *testing.T) {
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r) // pin 版本快照缺失
	}))
	defer svc.Close()
	m := &Manager{HTTP: &http.Client{}, BuildURL: svc.URL}
	_, _, err := m.FetchTTLWithCountAt("o1", 3)
	if err == nil || !strings.Contains(err.Error(), "pin 版本 v3") || !strings.Contains(err.Error(), "刷新 pin") {
		t.Fatalf("pin 快照缺失应硬失败且人话指引（不静默回落头版本）: %v", err)
	}
}
