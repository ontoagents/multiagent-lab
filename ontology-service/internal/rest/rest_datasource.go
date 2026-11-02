package rest

// REQ-595/M406 由数据源构建本体（第八构建路径）：POST /api/ontology/datasource/analyze →
// 202 {job_id}（异步 job 沿 ai-draft-async 同型基建——进程内 map+async_job 写穿+重启回读）；
// 轮询复用 GET /api/ai-draft-jobs/{id}（job.datasource 字段透出提案）。
// 提案落库人工确认后由前端走 createOntology+saveSpec 既有链（qualitygate/strict 门禁单源不旁路）。

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/ontology-service/internal/datasource"
)

// datasourceAnalyze POST /api/ontology/datasource/analyze。
func (s *Server) datasourceAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"` // SQLite 数据库文件路径（服务端本地；只读打开）
		Name string `json:"name"` // 建议本体名（可空=文件名去扩展名）
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path 必填（SQLite 数据库文件的服务端本地路径）"})
		return
	}
	// 文件存在性前置校验（快速失败人话；打开探活在管线内 query_only 只读强制）
	if st, err := os.Stat(req.Path); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件不存在或不可访问（请确认是服务端本机路径）：" + req.Path})
		return
	} else if st.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "路径是目录而非 SQLite 数据库文件"})
		return
	}
	job := &draftJob{ID: fmt.Sprintf("job_%d", time.Now().UnixNano()), Status: "running", CreatedAt: time.Now()}
	draftJobsMu.Lock()
	draftJobs[job.ID] = job
	draftJobsMu.Unlock()
	s.persistDraftJob(job)
	go func() {
		opts := datasource.Options{Path: req.Path, Name: req.Name}
		// 凭据不落库边界：LLM 走平台代理通道（构建平面不自持凭据）；连接信息仅本地文件路径，
		// 提案结果不含 path——重启回读也无连接串残留。
		prop, err := datasource.Analyze(context.Background(), opts, s.LLM,
			func(msg string) {
				draftJobsMu.Lock()
				job.Progress = msg
				draftJobsMu.Unlock()
				s.persistDraftJob(job)
			})
		draftJobsMu.Lock()
		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
		} else {
			job.Status = "done"
			job.Datasource = prop
		}
		draftJobsMu.Unlock()
		s.persistDraftJob(job)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID})
}
