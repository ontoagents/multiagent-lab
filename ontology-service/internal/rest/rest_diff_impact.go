package rest

// REQ-278/M103 变更影响分析端点（66 号 §2⑤ 唯一真缺口；oo plan/blast radius 借鉴自研）：
// GET /api/ontologies/{id}/impact?from={v}&to={v}——两版本 spec 结构 diff + 引用闭包波及面
// + 风险分级 + B1 疑似蕴含边检测（67 号扩注，measurement 口径只检测不自动删）。
// from/to 缺省=to 当前版本、from=上一版本（发布前影响报告的直接形态）。

import (
	"encoding/json"
	"net/http"
	"strconv"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

func (s *Server) impactVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetOntology(id); err != nil {
		writeErr(w, err)
		return
	}
	versions, err := s.Store.ListVersions(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(versions) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "该本体无版本快照"})
		return
	}
	cur, _ := s.Store.GetOntology(id)
	toV := cur.Version
	fromV := toV - 1
	if q := r.URL.Query().Get("to"); q != "" {
		if n, perr := strconv.Atoi(q); perr == nil && n > 0 {
			toV = n
		}
	}
	if q := r.URL.Query().Get("from"); q != "" {
		if n, perr := strconv.Atoi(q); perr == nil && n > 0 {
			fromV = n
		}
	}
	if fromV <= 0 || toV <= 0 || fromV == toV {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from/to 必须为不同的正整数版本号"})
		return
	}
	specAt := func(v int) (*pkgspec.Spec, error) {
		raw, err := s.Store.GetVersionSpec(id, v)
		if err != nil {
			return nil, err
		}
		sp := &pkgspec.Spec{}
		if err := json.Unmarshal([]byte(raw), sp); err != nil {
			return nil, err
		}
		return sp, nil
	}
	fromSp, err := specAt(fromV)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "from 版本不存在: " + err.Error()})
		return
	}
	toSp, err := specAt(toV)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "to 版本不存在: " + err.Error()})
		return
	}
	rep := pkgspec.AnalyzeImpact(fromSp, toSp)
	writeJSON(w, http.StatusOK, map[string]any{"ontology_id": id, "from_version": fromV, "to_version": toV, "report": rep})
}
