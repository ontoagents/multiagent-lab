package rest

// REQ-308 ai-cq-extract 单测：description 校验 + 桩 LLM 抽取链（job done + cqs 透出）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
)

func TestAiCQExtract(t *testing.T) {
	s := newImportGuardServer(t)
	// description 必填
	w := httptest.NewRecorder()
	s.aiCQExtract(w, importJSONReq(""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("空 description 期望 400，得 %d", w.Code)
	}

	// 桩 LLM：Chat 走 doGenerate → draft_json 内 JSON 数组
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"draft_json":` + strconv.Quote(`[{"cq":"药物可治疗哪些疾病？","origin":"抽取"},{"cq":"处方由哪些人员开立？","origin":"抽象"}]`) + `,"usage":null}`))
	}))
	t.Cleanup(llmSrv.Close)
	s2 := New(s.Store, nil, llmcreate.New(llmSrv.URL))
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ontologies/ai-cq-extract", strings.NewReader(`{"description":"医院药物管理","hints":["药物分处方药与非处方药"]}`))
	req.Header.Set("Content-Type", "application/json")
	s2.aiCQExtract(w2, req)
	if w2.Code != http.StatusAccepted {
		t.Fatalf("期望 202，得 %d body=%s", w2.Code, w2.Body.String())
	}
	var resp struct {
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp.JobID == "" {
		t.Fatal("应返回 job_id")
	}
	// 轮询 job 至 done（goroutine 毫秒级完成，上限 5s）
	var done bool
	for i := 0; i < 25; i++ {
		time.Sleep(200 * time.Millisecond)
		w3 := httptest.NewRecorder()
		r3 := httptest.NewRequest("GET", "/api/ai-draft-jobs/"+resp.JobID, nil)
		r3.SetPathValue("id", resp.JobID)
		s2.aiDraftJob(w3, r3)
		if w3.Code != http.StatusOK {
			continue
		}
		var job struct {
			Status string `json:"status"`
			CQs    []struct {
				CQ     string `json:"cq"`
				Origin string `json:"origin"`
			} `json:"cqs"`
		}
		_ = json.Unmarshal(w3.Body.Bytes(), &job)
		if job.Status == "failed" {
			t.Fatalf("job failed")
		}
		if job.Status == "done" {
			if len(job.CQs) != 2 || job.CQs[0].CQ != "药物可治疗哪些疾病？" {
				t.Fatalf("cqs 不符: %+v", job.CQs)
			}
			done = true
			break
		}
	}
	if !done {
		t.Fatal("job 未在 5s 内完成")
	}
}
