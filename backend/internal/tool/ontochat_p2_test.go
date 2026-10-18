// REQ-354/P2 单测（tool 层）：①start 会话归属审计（tool.WithCaller 注入 → source=agent_chat+
// agent_id 随建会话落构建平面；无身份不传）；②ontochat_export_ttl 四路径——TTL 透传+计数头/
// 已入库回填指引/无草稿 400 透传/不可达诚实回执/超限截断；③注册与免审判定。
package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOntoChatStartAuditCaller(t *testing.T) {
	stub := newOntoStub()
	var gotSource, gotAgentID string
	stub.handle("POST /api/ontochat/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotSource, gotAgentID = req["source"], req["agent_id"]
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"sess-audit","title":"x","stage":"cq","round":0,"messages":[],"context":{}}`))
	})
	srv := stub.server()
	defer srv.Close()
	t.Setenv("BUILD_SVC_URL", srv.URL)

	// 有调用者身份（runner WithCaller 注入形态）：审计两列随建会话
	ctx := WithCaller(context.Background(), CallerInfo{AgentID: "ag_9", AgentName: "本体工程师", Scope: "agent"})
	if _, err := ontoStartFn(ctx, ontoStartIn{Topic: "缺陷管理", Mode: "quick"}); err != nil {
		t.Fatalf("start 失败: %v", err)
	}
	if gotSource != "agent_chat" || gotAgentID != "ag_9" {
		t.Fatalf("期望 source=agent_chat agent_id=ag_9，实际 %q/%q", gotSource, gotAgentID)
	}

	// 无身份（单测/直调形态）：不传，构建平面缺省归 page
	gotSource, gotAgentID = "", ""
	if _, err := ontoStartFn(context.Background(), ontoStartIn{Topic: "缺陷管理", Mode: "quick"}); err != nil {
		t.Fatalf("start 失败: %v", err)
	}
	if gotSource != "" || gotAgentID != "" {
		t.Fatalf("无身份不应传审计字段，实际 %q/%q", gotSource, gotAgentID)
	}
}

func TestOntoChatExportTTL(t *testing.T) {
	ttlBody := "# 导出测试\ncq:设备 a rdfs:Class .\n"
	stub := newOntoStub()
	stub.handle("GET /api/ontochat/sessions/sess-exp/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/turtle; charset=utf-8")
		w.Header().Set("X-Onto-Triples", "15")
		w.Header().Set("X-Onto-Source", "ontochat_draft")
		_, _ = w.Write([]byte(ttlBody))
	})
	stub.handle("GET /api/ontochat/sessions/sess-exp", func(w http.ResponseWriter, _ *http.Request) {
		// 未入库：ontology_id 缺省
		_, _ = w.Write([]byte(`{"id":"sess-exp","stage":"refine","round":3,"messages":[],"context":{}}`))
	})
	srv := stub.server()
	defer srv.Close()
	t.Setenv("BUILD_SVC_URL", srv.URL)

	out, err := ontoExportTTLFn(context.Background(), ontoExportIn{SessionID: "sess-exp"})
	if err != nil || out.Error != "" {
		t.Fatalf("export 失败: out=%+v err=%v", out, err)
	}
	if out.TTL != ttlBody || out.Triples != 15 || out.Truncated {
		t.Errorf("TTL/计数透传不符: %+v", out)
	}
	if out.SavedAssetID != "" || !strings.Contains(out.Hint, "ontochat_save") {
		t.Errorf("未入库会话应有入库指引且无资产回填: %+v", out)
	}

	// 已入库会话：SavedAssetID 回填 + 提示资产页权威导出
	stub.handle("GET /api/ontochat/sessions/sess-done", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"sess-done","stage":"done","round":5,"ontology_id":"ont_1","messages":[],"context":{}}`))
	})
	stub.handle("GET /api/ontochat/sessions/sess-done/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Onto-Triples", "9")
		_, _ = w.Write([]byte(ttlBody))
	})
	out2, _ := ontoExportTTLFn(context.Background(), ontoExportIn{SessionID: "sess-done"})
	if out2.SavedAssetID != "ont_1" || !strings.Contains(out2.Hint, "ont_1") {
		t.Errorf("已入库会话应回填资产指引: %+v", out2)
	}

	// 无草稿 400 人话报错透传
	stub.handle("GET /api/ontochat/sessions/sess-empty/export", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"会话尚无草稿：请先生成草稿再导出（TTL 导出对象是当前草稿）"}`))
	})
	out3, _ := ontoExportTTLFn(context.Background(), ontoExportIn{SessionID: "sess-empty"})
	if !strings.Contains(out3.Error, "尚无草稿") {
		t.Errorf("无草稿错误应透传: %+v", out3)
	}

	// 超限截断：>24KB TTL 截断并如实标注
	big := strings.Repeat("x", ontoExportTTLCap+1024)
	stub.handle("GET /api/ontochat/sessions/sess-big/export", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(big))
	})
	out4, _ := ontoExportTTLFn(context.Background(), ontoExportIn{SessionID: "sess-big"})
	if !out4.Truncated || len(out4.TTL) != ontoExportTTLCap {
		t.Errorf("超限应截断到上限: truncated=%v len=%d", out4.Truncated, len(out4.TTL))
	}
}

func TestOntoChatExportTTLUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // 立即关闭=不可达
	t.Setenv("BUILD_SVC_URL", srv.URL)
	out, err := ontoExportTTLFn(context.Background(), ontoExportIn{SessionID: "sess-x"})
	if err != nil {
		t.Fatalf("工具层不应回 err（诚实回执形态）: %v", err)
	}
	if !strings.Contains(out.Error, "构建平面暂不可达") || !strings.Contains(out.Error, "不得编造") {
		t.Errorf("不可达应诚实回执: %+v", out)
	}
}

func TestOntoChatP2Registered(t *testing.T) {
	r := NewRegistry()
	if err := registerOntoChatTools(r); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	for _, id := range []string{"ontochat_start", "ontochat_turn", "ontochat_save", "ontochat_export_ttl"} {
		if _, ok := r.Get(id); !ok {
			t.Errorf("工具 %s 应已注册", id)
		}
	}
	// export 只读免 danger 档（all 档照审）；save 写类保留
	if IsDangerousTool("ontochat_export_ttl") {
		t.Error("ontochat_export_ttl 为只读导出，不应入危险清单")
	}
	if !IsDangerousTool("ontochat_save") {
		t.Error("ontochat_save 应保留危险清单")
	}
}
