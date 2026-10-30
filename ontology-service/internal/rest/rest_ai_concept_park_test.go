package rest

// REQ-508 ai-concept-park 单测：material 校验（必填/超长）+ LLM 未配置 503 + 桩 LLM 铺候选链
// （job done + concepts 透出 + 批内 name 去重 + 上限截断注记）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
)

// conceptParkJobJSON 构造桩 LLM 的 draft_json 字符串（conceptParkSchema 输出形态）。
func conceptParkJobJSON(topic string, concepts []map[string]string) string {
	b, _ := json.Marshal(map[string]any{"topic": topic, "concepts": concepts})
	return string(b)
}

func TestAiConceptParkValidation(t *testing.T) {
	s := newImportGuardServer(t)
	// material 必填
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ontologies/ai-concept-park", strings.NewReader(`{"material":"  "}`))
	req.Header.Set("Content-Type", "application/json")
	s.aiConceptPark(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("空 material 期望 400，得 %d", w.Code)
	}
	// 超长材料（ImportFileMaxChars+1 rune）→ 400 人话
	long := strings.Repeat("长", 30001)
	w2 := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"material": long})
	req2 := httptest.NewRequest("POST", "/api/ontologies/ai-concept-park", strings.NewReader(string(body)))
	req2.Header.Set("Content-Type", "application/json")
	s.aiConceptPark(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("超长 material 期望 400，得 %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "材料过长") {
		t.Fatalf("超长报错应为人话提示，得 %s", w2.Body.String())
	}
	// LLM 未配置 → 503
	s3 := New(s.Store, nil, nil)
	w3 := httptest.NewRecorder()
	body3, _ := json.Marshal(map[string]string{"material": "一段学习材料"})
	req3 := httptest.NewRequest("POST", "/api/ontologies/ai-concept-park", strings.NewReader(string(body3)))
	req3.Header.Set("Content-Type", "application/json")
	s3.aiConceptPark(w3, req3)
	if w3.Code != http.StatusServiceUnavailable {
		t.Fatalf("LLM 未配置期望 503，得 %d", w3.Code)
	}
}

func TestAiConceptParkJobLifecycle(t *testing.T) {
	s := newImportGuardServer(t)
	// 桩 LLM：35 条候选（32 唯一+3 重复）——验证上限 30 截断+truncated=3+topic 透出
	var concepts []map[string]string
	for i := 0; i < 32; i++ {
		concepts = append(concepts, map[string]string{
			"name": fmt.Sprintf("概念%d", i), "definition": fmt.Sprintf("第 %d 个概念定义", i),
			"origin": fmt.Sprintf("材料原句 %d", i),
		})
	}
	concepts = append(concepts,
		map[string]string{"name": "概念0", "origin": "重复 1"},
		map[string]string{"name": "概念1", "origin": "重复 2"},
		map[string]string{"name": "概念0", "origin": "重复 3"})
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"draft_json":` + strconv.Quote(conceptParkJobJSON("医院药物管理", concepts)) + `,"usage":null}`))
	}))
	t.Cleanup(llmSrv.Close)
	s2 := New(s.Store, nil, llmcreate.New(llmSrv.URL))
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"material": "医院药物管理学习笔记：处方药须凭医师处方购买……"})
	req := httptest.NewRequest("POST", "/api/ontologies/ai-concept-park", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	s2.aiConceptPark(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("期望 202，得 %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.JobID == "" {
		t.Fatal("应返回 job_id")
	}
	// 轮询 job 至 done（goroutine 毫秒级完成，上限 5s）
	var got struct {
		Status    string `json:"status"`
		Topic     string `json:"topic"`
		Truncated int    `json:"truncated"`
		Concepts  []struct {
			Name       string `json:"name"`
			Definition string `json:"definition"`
			Origin     string `json:"origin"`
		} `json:"concepts"`
		Error string `json:"error"`
	}
	for i := 0; i < 25; i++ {
		time.Sleep(200 * time.Millisecond)
		w3 := httptest.NewRecorder()
		r3 := httptest.NewRequest("GET", "/api/ai-draft-jobs/"+resp.JobID, nil)
		r3.SetPathValue("id", resp.JobID)
		s2.aiDraftJob(w3, r3)
		if w3.Code != http.StatusOK {
			continue
		}
		if err := json.Unmarshal(w3.Body.Bytes(), &got); err != nil {
			t.Fatalf("job 轮询解析失败：%v", err)
		}
		if got.Status == "done" || got.Status == "failed" {
			break
		}
	}
	if got.Status != "done" {
		t.Fatalf("job 应 done，得 %s（error=%s）", got.Status, got.Error)
	}
	if got.Topic != "医院药物管理" {
		t.Fatalf("topic 应透出，得 %q", got.Topic)
	}
	if len(got.Concepts) != 30 {
		t.Fatalf("候选应截断至上限 30，得 %d", len(got.Concepts))
	}
	if got.Truncated != 5 { // 3 重复 + 2 超上限（32 唯一中 30 之后 2 条被截）
		t.Fatalf("truncated 应为 5（3 重复+2 超限），得 %d", got.Truncated)
	}
	if got.Concepts[0].Name != "概念0" || got.Concepts[0].Origin != "材料原句 0" {
		t.Fatalf("首条候选应保留首现 origin，得 %+v", got.Concepts[0])
	}
}

func TestAiConceptParkParseRetry(t *testing.T) {
	s := newImportGuardServer(t)
	// 桩 LLM：首轮输出非 JSON → 回喂重试；次轮合法 → done
	calls := 0
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		if calls == 1 {
			_, _ = w.Write([]byte(`{"draft_json":"这不是 JSON"}`))
			return
		}
		_, _ = w.Write([]byte(`{"draft_json":` + strconv.Quote(conceptParkJobJSON("测试主题", []map[string]string{{"name": "容器", "origin": "容器是 Kubernetes 的核心原语"}})) + `,"usage":null}`))
	}))
	t.Cleanup(llmSrv.Close)
	s2 := New(s.Store, nil, llmcreate.New(llmSrv.URL))
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"material": "Kubernetes 学习笔记"})
	req := httptest.NewRequest("POST", "/api/ontologies/ai-concept-park", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	s2.aiConceptPark(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("期望 202，得 %d", w.Code)
	}
	var resp struct {
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	var got struct {
		Status   string `json:"status"`
		Concepts []struct {
			Name string `json:"name"`
		} `json:"concepts"`
		Error string `json:"error"`
	}
	for i := 0; i < 25; i++ {
		time.Sleep(200 * time.Millisecond)
		w3 := httptest.NewRecorder()
		r3 := httptest.NewRequest("GET", "/api/ai-draft-jobs/"+resp.JobID, nil)
		r3.SetPathValue("id", resp.JobID)
		s2.aiDraftJob(w3, r3)
		if w3.Code == http.StatusOK {
			_ = json.Unmarshal(w3.Body.Bytes(), &got)
			if got.Status == "done" || got.Status == "failed" {
				break
			}
		}
	}
	if got.Status != "done" {
		t.Fatalf("回喂重试后应 done，得 %s（error=%s）", got.Status, got.Error)
	}
	if len(got.Concepts) != 1 || got.Concepts[0].Name != "容器" {
		t.Fatalf("重试后候选应透出，得 %+v", got.Concepts)
	}
}
