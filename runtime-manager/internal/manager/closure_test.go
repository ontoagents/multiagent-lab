package manager

// REQ-480/M280 闭包装载与门禁单测（httptest 假构建平面，零外部依赖——沿 gate_test.go 先例）：
// 闭包 BFS 补全+互引环呈现+远端跳过 / 悬空阻断（missing_ontology/missing_concept）/
// imports 登记目标悬空阻断 / 装载前并置一致性检查降级策略。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func refStub(refsJSON string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"refs":` + refsJSON + `,"dangling":[]}`))
	}
}

func TestExpandClosure_CycleAddedAndRemoteSkip(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ontologies/A/external-refs", refStub(
		`[{"from_concept":"(ontology)","to_concept":"urn:o:B:","ref_kind":"import"},
		  {"from_concept":"X","to_onto_id":"C","to_concept":"c1","ref_kind":"import"}]`))
	mux.HandleFunc("/api/ontologies/B/external-refs", refStub(
		`[{"from_concept":"(ontology)","to_concept":"urn:o:A:","ref_kind":"import"}]`))
	mux.HandleFunc("/api/ontologies/C/external-refs", refStub(
		`[{"from_concept":"(ontology)","to_concept":"http://ex.org/voc#x","ref_kind":"import"}]`))
	svc := httptest.NewServer(mux)
	defer svc.Close()
	m := &Manager{HTTP: &http.Client{}, BuildURL: svc.URL, LogDir: t.TempDir()}

	rep, err := m.expandClosure([]string{"A"})
	if err != nil {
		t.Fatalf("expandClosure: %v", err)
	}
	if strings.Join(rep.Effective, ",") != "A,B,C" {
		t.Fatalf("有效清单期望 A,B,C，实际 %v", rep.Effective)
	}
	if strings.Join(rep.Added, ",") != "B,C" {
		t.Fatalf("闭包补全期望 B,C，实际 %v", rep.Added)
	}
	if len(rep.Cycles) != 1 || strings.Join(rep.Cycles[0], "→") != "A→B→A" {
		t.Fatalf("互引环期望 [A→B→A]，实际 %v", rep.Cycles)
	}
	if len(rep.RemoteSkipped) != 1 || rep.RemoteSkipped[0] != "http://ex.org/voc#x" {
		t.Fatalf("远端跳过期望 [http://ex.org/voc#x]，实际 %v", rep.RemoteSkipped)
	}
	if tag := rep.cycleTags()["A"]; !strings.Contains(tag, "A") || !strings.Contains(tag, "B") {
		t.Fatalf("环标注期望含 A/B，实际 %q", tag)
	}
	logs := rep.logLines()
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "闭包补全: B, C") || !strings.Contains(joined, "互引环: A → B → A") || !strings.Contains(joined, "远端 IRI 跳过") {
		t.Fatalf("装载报告行不符:\n%s", joined)
	}
}

func TestExpandClosure_DanglingOntologyBlock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ontologies/A/external-refs", refStub(
		`[{"from_concept":"X","to_onto_id":"D","to_concept":"d1","ref_kind":"import","dangling":"missing_ontology"}]`))
	svc := httptest.NewServer(mux)
	defer svc.Close()
	m := &Manager{HTTP: &http.Client{}, BuildURL: svc.URL, LogDir: t.TempDir()}
	_, err := m.expandClosure([]string{"A"})
	if err == nil || !strings.Contains(err.Error(), "装载门禁阻断") || !strings.Contains(err.Error(), "本体已删除") {
		t.Fatalf("悬空本体期望阻断，实际 err=%v", err)
	}
}

func TestExpandClosure_DanglingConceptBlock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ontologies/A/external-refs", refStub(
		`[{"from_concept":"X","to_onto_id":"D","to_concept":"d1","ref_kind":"reuse","dangling":"missing_concept"}]`))
	svc := httptest.NewServer(mux)
	defer svc.Close()
	m := &Manager{HTTP: &http.Client{}, BuildURL: svc.URL, LogDir: t.TempDir()}
	_, err := m.expandClosure([]string{"A"})
	if err == nil || !strings.Contains(err.Error(), "概念不存在") {
		t.Fatalf("悬空概念期望阻断，实际 err=%v", err)
	}
}

func TestExpandClosure_ImportsTargetMissingBlock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ontologies/A/external-refs", refStub(
		`[{"from_concept":"(ontology)","to_concept":"urn:o:ghost:","ref_kind":"import"}]`))
	// ghost 未注册处理器 → 404
	svc := httptest.NewServer(mux)
	defer svc.Close()
	m := &Manager{HTTP: &http.Client{}, BuildURL: svc.URL, LogDir: t.TempDir()}
	_, err := m.expandClosure([]string{"A"})
	if err == nil || !strings.Contains(err.Error(), "悬空") {
		t.Fatalf("imports 目标悬空期望阻断，实际 err=%v", err)
	}
}

func TestPreloadReasonCheck(t *testing.T) {
	mk := func(body string, code int, wantParts ...string) *Manager {
		svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/ontologies/reason-check" {
				http.NotFound(w, r)
				return
			}
			var in struct {
				Content string `json:"content"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			for _, want := range wantParts {
				if !strings.Contains(in.Content, want) {
					t.Errorf("并置内容应含 %q，实际 %.80s", want, in.Content)
				}
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(svc.Close)
		return &Manager{HTTP: &http.Client{}, BuildURL: svc.URL, LogDir: t.TempDir()}
	}
	m := mk(`{"consistent":true}`, 200, "ttlA", "ttlB")
	if line := m.preloadReasonCheck(map[string]string{"b": "ttlB", "a": "ttlA"}); !strings.Contains(line, "一致（owlrl") {
		t.Fatalf("一致期望日志行，实际 %q", line)
	}
	m2 := mk(`{"consistent":false,"violations":["v1","v2","v3","v4"]}`, 200, "ttlA")
	if line := m2.preloadReasonCheck(map[string]string{"a": "ttlA"}); !strings.Contains(line, "不一致（强告警不阻断）") || !strings.Contains(line, "4 违例") {
		t.Fatalf("不一致期望强告警行，实际 %q", line)
	}
	m3 := mk(`{"error":"boom"}`, 500)
	if line := m3.preloadReasonCheck(map[string]string{"a": "ttlA"}); !strings.Contains(line, "跳过（HTTP 500") {
		t.Fatalf("端点失败期望降级跳过，实际 %q", line)
	}
	_ = context.Background()
}
