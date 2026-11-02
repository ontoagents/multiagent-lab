package rest

// REQ-595/M406 datasource/analyze 端点单测：path 必填 400/文件缺失 400/目录 400/
// happy path（202 job→轮询 done→提案透出+LLM 未配置如实标注）。

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedDatasourceSQLite(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ds_rest_test.db")
	db, err := sql.Open("sqlite", path) // modernc 驱动经 repo 包 blank import 注册
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	defer db.Close()
	for _, s := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, role TEXT)`,
		`CREATE TABLE logs (id INTEGER PRIMARY KEY, user_id INTEGER REFERENCES users(id), action TEXT)`,
		`INSERT INTO users (id, name, role) VALUES (1,'a','admin'),(2,'b','user')`,
		`INSERT INTO logs (id, user_id, action) VALUES (1,1,'login'),(2,2,'view')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("建表失败 %s: %v", s, err)
		}
	}
	return path
}

func TestDatasourceAnalyzeValidation(t *testing.T) {
	s := newImportGuardServer(t)
	// path 必填
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ontology/datasource/analyze", strings.NewReader(`{"path":"  "}`))
	s.datasourceAnalyze(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("空 path 期望 400，得 %d", w.Code)
	}
	// 文件不存在
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/ontology/datasource/analyze", strings.NewReader(`{"path":"Z:/no/such/db.sqlite"}`))
	s.datasourceAnalyze(w2, req2)
	if w2.Code != http.StatusBadRequest || !strings.Contains(w2.Body.String(), "不存在") {
		t.Fatalf("缺失文件期望 400 人话，得 %d %s", w2.Code, w2.Body.String())
	}
	// 目录
	w3 := httptest.NewRecorder()
	dir := t.TempDir()
	b3, _ := json.Marshal(map[string]string{"path": dir})
	req3 := httptest.NewRequest("POST", "/api/ontology/datasource/analyze", strings.NewReader(string(b3)))
	s.datasourceAnalyze(w3, req3)
	if w3.Code != http.StatusBadRequest || !strings.Contains(w3.Body.String(), "目录") {
		t.Fatalf("目录路径期望 400 人话，得 %d %s", w3.Code, w3.Body.String())
	}
}

func TestDatasourceAnalyzeJobFlow(t *testing.T) {
	s := newImportGuardServer(t)
	path := seedDatasourceSQLite(t)
	body, _ := json.Marshal(map[string]string{"path": path, "name": "用户操作库"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ontology/datasource/analyze", strings.NewReader(string(body)))
	s.datasourceAnalyze(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("期望 202，得 %d: %s", w.Code, w.Body.String())
	}
	var ack struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ack); err != nil || ack.JobID == "" {
		t.Fatalf("202 应返回 job_id: %v %s", err, w.Body.String())
	}
	// 轮询（同 ai-draft-jobs 通道）至终态
	var last []byte
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		w2 := httptest.NewRecorder()
		req2 := httptest.NewRequest("GET", "/api/ai-draft-jobs/"+ack.JobID, nil)
		req2.SetPathValue("id", ack.JobID) // 裸 NewRequest 不过 mux，PathValue 须显式置
		s.aiDraftJob(w2, req2)
		var j draftJob
		if json.Unmarshal(w2.Body.Bytes(), &j) != nil {
			continue
		}
		last = w2.Body.Bytes()
		if j.Status == "done" {
			// 提案透出：LLM 未配置如实标注+骨架规模
			if j.Datasource == nil {
				t.Fatalf("done 应携带提案: %s", w2.Body.String())
			}
			if j.Datasource.LLMUsed || j.Datasource.LLMSkipped == "" {
				t.Fatalf("无 LLM 应如实标注: %+v", j.Datasource)
			}
			if len(j.Datasource.Spec.Concepts) != 2 || len(j.Datasource.Spec.Relations) != 1 {
				t.Fatalf("骨架规模不符（2 表 1 声明 FK）: %+v", j.Datasource.Spec)
			}
			if j.Datasource.Name != "用户操作库" {
				t.Fatalf("建议名应取请求 name: %q", j.Datasource.Name)
			}
			return
		}
		if j.Status == "failed" {
			t.Fatalf("job 失败: %s", j.Error)
		}
	}
	t.Fatalf("轮询超时未达终态: %s", string(last))
}
