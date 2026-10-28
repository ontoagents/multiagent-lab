package api

// REQ-250/52 号 E6 余项：本体资产治理审计——构建平面四类治理动作（spec 保存/导入/
// fork/删除）在 backend 反代层落 onto_decision（subject_kind=ontology），使「谁在何时
// 改了本体、从哪导入」在消费与审计栏可追溯。此前 onto_decision 三写入点全在 KG/审计链，
// 本体资产侧零事件（52 号 A-3/58 号 E6 诚实遗留项）。
//
// 实现：精确路由压过 /api/ontologies/ 反代前缀，代理转发成功（2xx）后异步记事件
// （失败不记——审计的是已生效变更）。导入/合并为多形态写入，走同一段记录逻辑。
// 构建平面不可达（502）不记。

import (
	"net/http"
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// ontologyAuditAction 动作描述（title 模板）。
type ontologyAuditAction struct {
	kind  string // onto_decision action 语义位（存 title 前缀，subject_kind=ontology 统一）
	title string
}

func ontologyAuditFor(method, path string) (ontologyAuditAction, bool) {
	switch {
	case method == http.MethodPut && strings.HasSuffix(path, "/spec"):
		return ontologyAuditAction{kind: "spec.save", title: "保存 Spec（新版本）"}, true
	case method == http.MethodPost && strings.HasSuffix(path, "/import"):
		return ontologyAuditAction{kind: "import", title: "导入本体（多形态资产）"}, true
	case method == http.MethodPost && strings.HasSuffix(path, "/merge/apply"):
		return ontologyAuditAction{kind: "merge.apply", title: "导入合并入库（merge apply）"}, true
	case method == http.MethodPost && strings.HasSuffix(path, "/fork"):
		return ontologyAuditAction{kind: "fork", title: "Fork 派生新本体"}, true
	case method == http.MethodPost && strings.HasSuffix(path, "/ingest-csv"):
		return ontologyAuditAction{kind: "ingest.csv", title: "CSV 数据灌装入库"}, true
	// REQ-292/M96：对象级 overlay（编辑三层）——人工编辑与撤回进审计（audit 层承载）
	case method == http.MethodPut && strings.HasSuffix(path, "/overlay"):
		return ontologyAuditAction{kind: "overlay.edit", title: "对象级人工编辑（overlay）"}, true
	case method == http.MethodDelete && strings.HasSuffix(path, "/overlay"):
		return ontologyAuditAction{kind: "overlay.revert", title: "撤回人工编辑（revert to base）"}, true
	default:
		return ontologyAuditAction{}, false
	}
}

// extractOntologyID 从 /api/ontologies/{id}[/…] 形态路径取本体 id；import 无 id 返回空。
func extractOntologyID(path string) string {
	rest := strings.TrimPrefix(path, "/api/ontologies/")
	rest = strings.Trim(rest, "/")
	if rest == "" || strings.Contains(rest, "/") {
		// import（无 id）或更深路径取首段
		if i := strings.Index(rest, "/"); i > 0 {
			return rest[:i]
		}
		return rest
	}
	return rest
}

// auditOntologyProxy 构建平面治理动作审计代理：转发 → 2xx 后落 onto_decision。
// 放在反代 handler 外层（精确路由注册），响应体原样透传。
func (s *Server) auditOntologyProxy(w http.ResponseWriter, r *http.Request) {
	if s.Ontology == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "本体平面未配置"})
		return
	}
	act, ok := ontologyAuditFor(r.Method, r.URL.Path)
	if !ok {
		s.Ontology.BuildProxy().ServeHTTP(w, r)
		return
	}
	oid := extractOntologyID(r.URL.Path)
	proxy := s.Ontology.BuildProxy()
	rec := &passthroughRecorder{w: w}
	proxy.ServeHTTP(rec, r)
	if rec.code >= 200 && rec.code < 300 && s.Store != nil {
		// 异步落审计（失败仅日志，不阻断响应——响应体已透传）
		go func() {
			_, _ = s.Store.InsertDecision(&store.OntoDecision{
				SubjectKind: "ontology",
				SubjectID:   oid,
				Title:       "[" + act.kind + "] " + act.title,
				Rationale:   act.title + "（治理审计，REQ-250/52 号 E6）",
				MetaJSON:    `{"method":"` + r.Method + `","path":"` + r.URL.Path + `"}`,
			})
		}()
	}
}

// passthroughRecorder 捕获状态码的响应透传包装。
type passthroughRecorder struct {
	w    http.ResponseWriter
	code int
}

func (r *passthroughRecorder) Header() http.Header { return r.w.Header() }
func (r *passthroughRecorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.w.Write(b)
}
func (r *passthroughRecorder) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
	r.w.WriteHeader(code)
}

// Flush 透传（反代/流式响应需要）。
func (r *passthroughRecorder) Flush() {
	if f, ok := r.w.(http.Flusher); ok {
		f.Flush()
	}
}

// ontologyContribution GET /api/ontology/{id}/contribution?top_n=：REQ-224 H10 消费贡献画像
// ——onto_* facade 工具命中次数、按工具分布、被引用概念 TopN（仅成功命中计）、最近 20 条。
func (s *Server) ontologyContribution(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.OntologyContribution(r.PathValue("id"), atoiDefault(r.URL.Query().Get("top_n"), 10))
	if err != nil {
		writeErr(w, err)
		return
	}
	if c.ByTool == nil {
		c.ByTool = []store.OntologyToolStat{}
	}
	if c.TopTargets == nil {
		c.TopTargets = []store.OntologyTargetStat{}
	}
	if c.Recent == nil {
		c.Recent = []store.OntologyHit{}
	}
	writeJSON(w, http.StatusOK, c)
}

// ontologyContributionSummary GET /api/ontology/contribution/summary?limit=：跨本体命中概览
// （消费审计栏一览；倒序按命中数）。
func (s *Server) ontologyContributionSummary(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.OntologyContributionSummary(atoiDefault(r.URL.Query().Get("limit"), 20))
	if err != nil {
		writeErr(w, err)
		return
	}
	if rows == nil {
		rows = []*store.OntologyContributionSummaryRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

// ontologyRunGroups GET /api/ontology/contribution/run-groups?limit=：REQ-482/M282④ 组维度
// ——最近 N 次回答（run）各自命中的本体集合与方案归因（「一次回答命中哪些本体」）。
func (s *Server) ontologyRunGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.OntologyRunGroups(atoiDefault(r.URL.Query().Get("limit"), 10))
	if err != nil {
		writeErr(w, err)
		return
	}
	if rows == nil {
		rows = []*store.OntologyRunGroup{}
	}
	writeJSON(w, http.StatusOK, rows)
}
