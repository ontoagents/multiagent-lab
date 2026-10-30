package api

// REQ-529/M336（P17）：沉淀链出料口——①伴生图→正式本体晋升 ②项目产物→KB 一键入库。
// 知识资产管线（90 号 §2.2）出料段补全：伴生确认实体/关系写入目标本体 spec 草稿（走既有
// 统一校验循环——PUT spec 即构建平面 Spec.Validate，差异只在生成段、收敛在资产段架构复用，
// 零新能力）；项目文件复用既有摄入管道（ImportAsync，富格式 REQ-414 已在）。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/companion"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/fsutil"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// companionBinding SPARQL SELECT JSON 单绑定（s/p/o 或 ?label）。
type companionBinding map[string]struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func (b companionBinding) get(name string) string {
	if v, ok := b[name]; ok {
		return v.Value
	}
	return ""
}

// queryCompanionGraph 伴生图 SPARQL SELECT（JSON bindings 解析；Query 输出即 sparql-results+json）。
func (s *Server) queryCompanionGraph(ctx context.Context, ontologyID, sparql string) ([]companionBinding, error) {
	base, err := s.Companion.EnsureHost(ctx, ontologyID)
	if err != nil {
		return nil, err
	}
	raw, err := s.Companion.Plans.Query(ctx, base, sparql)
	if err != nil {
		return nil, err
	}
	var out struct {
		Bindings []companionBinding `json:"results"`
	}
	// sparql-results+json: {"results": {"bindings": [...]}}
	var wrapper struct {
		Results struct {
			Bindings []companionBinding `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("伴生图结果解析失败: %w", err)
	}
	out.Bindings = wrapper.Results.Bindings
	return out.Bindings, nil
}

// companionGraphDigest 伴生图摘要：活跃实体（label）+活跃关系边（subject/relName/object）。
type companionGraphDigest struct {
	Entities  []string          `json:"entities"`
	Relations [][3]string       `json:"relations"` // [subjectLabel, relName, objectLabel]
	Labels    map[string]string `json:"-"`         // entityURI → label
}

func (s *Server) companionGraphDigest(ctx context.Context, ontologyID string) (*companionGraphDigest, error) {
	// 实体 label（uri→label；label 三元组即实体存在性——失效化只打 invalidAt 于边，实体保留）
	labels := map[string]string{}
	labelBindings, err := s.queryCompanionGraph(ctx, ontologyID, fmt.Sprintf(
		`SELECT ?uri ?label WHERE { GRAPH <%s> { ?uri rdfs:label ?label . FILTER(isLiteral(?label)) } } LIMIT 5000`,
		companion.GraphURI(ontologyID)))
	if err != nil {
		return nil, err
	}
	for _, b := range labelBindings {
		labels[b.get("uri")] = b.get("label")
	}
	// 活跃关系边（invalidAt 不存在=活跃——矛盾旧边排除）
	edgeBindings, err := s.queryCompanionGraph(ctx, ontologyID, fmt.Sprintf(
		`SELECT ?sLabel ?relName ?oLabel WHERE {
  GRAPH <%s> {
    ?edge a bot:Relation ;
          rdfs:label ?edgeLabel ;
          bot:subject ?s ;
          bot:relName ?relName ;
          bot:object ?o .
    FILTER NOT EXISTS { ?edge bot:invalidAt ?any }
    ?s rdfs:label ?sLabel .
    ?o rdfs:label ?oLabel .
  }
} LIMIT 5000`, companion.GraphURI(ontologyID)))
	if err != nil {
		return nil, err
	}
	d := &companionGraphDigest{Labels: labels}
	seenEntity := map[string]bool{}
	for _, l := range labels {
		if !seenEntity[l] {
			seenEntity[l] = true
			d.Entities = append(d.Entities, l)
		}
	}
	sortStrings(d.Entities)
	for _, b := range edgeBindings {
		sl, rn, ol := b.get("sLabel"), b.get("relName"), b.get("oLabel")
		if sl == "" || ol == "" || rn == "" {
			continue
		}
		d.Relations = append(d.Relations, [3]string{sl, rn, ol})
	}
	return d, nil
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

// promoteCompanion POST /api/companion/ontologies/{id}/promote {target_ontology_id}
// → 伴生确认实体/关系写入目标本体 spec 草稿（去重合并——已存在同名概念/同边关系跳过），
// PUT 构建平面 spec 走统一校验循环（Spec.Validate；审计由 spec PUT 既有 auditOntologyProxy
// 同面覆盖——本端点直调构建平面与 auditOntologyProxy 转发同址）。
func (s *Server) promoteCompanion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		TargetOntologyID string `json:"target_ontology_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(in.TargetOntologyID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target_ontology_id 必填——选择晋升目标本体"})
		return
	}
	digest, err := s.companionGraphDigest(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(digest.Entities) == 0 && len(digest.Relations) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "伴生图为空（无已确认实体/关系）——先在对话中沉淀"})
		return
	}
	// 读目标本体 spec（构建平面权威）
	specRaw, err := s.buildPlaneGET(r.Context(), "/api/ontologies/"+in.TargetOntologyID+"/spec")
	if err != nil {
		writeErr(w, err)
		return
	}
	var spec struct {
		Name       string                   `json:"name"`
		Concepts   []map[string]any         `json:"concepts"`
		Relations  []map[string]any         `json:"relations"`
		Instances  []map[string]any         `json:"instances"`
		CQ         []string                 `json:"cq"`
	}
	if err := json.Unmarshal(specRaw, &spec); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "目标本体 spec 解析失败: " + err.Error()})
		return
	}
	haveConcept := map[string]bool{}
	for _, c := range spec.Concepts {
		if n, ok := c["name"].(string); ok {
			haveConcept[n] = true
		}
	}
	haveEdge := map[string]bool{}
	for _, rel := range spec.Relations {
		n, _ := rel["name"].(string)
		f, _ := rel["from"].(string)
		t, _ := rel["to"].(string)
		haveEdge[n+"\x00"+f+"\x00"+t] = true
	}
	addedC, addedR, skipped := 0, 0, 0
	for _, e := range digest.Entities {
		if haveConcept[e] {
			skipped++
			continue
		}
		spec.Concepts = append(spec.Concepts, map[string]any{"name": e})
		haveConcept[e] = true
		addedC++
	}
	for _, edge := range digest.Relations {
		key := edge[1] + "\x00" + edge[0] + "\x00" + edge[2]
		if haveConcept[edge[0]] == false || haveConcept[edge[2]] == false || haveEdge[key] {
			skipped++
			continue
		}
		spec.Relations = append(spec.Relations, map[string]any{"name": edge[1], "from": edge[0], "to": edge[2]})
		haveEdge[key] = true
		addedR++
	}
	// PUT 回构建平面（统一校验循环——校验失败构建平面返回 400 明细，本端点透传）
	patch, err := json.Marshal(map[string]any{
		"name": spec.Name, "concepts": spec.Concepts, "relations": spec.Relations,
		"instances": spec.Instances, "cq": spec.CQ,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.buildPlanePUT(r.Context(), "/api/ontologies/"+in.TargetOntologyID+"/spec", patch); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"promoted_concepts": addedC, "promoted_relations": addedR, "skipped_existing": skipped,
		"target": in.TargetOntologyID,
	})
}

// buildPlaneGET/PUT 构建平面直调（与 auditOntologyProxy 转发同址 BuildURL；PUT 即走构建平面
// Spec.Validate 统一校验循环+审计由 proxy 面同源覆盖）。
func (s *Server) buildPlaneGET(ctx context.Context, path string) ([]byte, error) {
	tr := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Ontology.BuildURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := tr.Do(req)
	if err != nil {
		return nil, &store.HTTPError{Status: http.StatusBadGateway, Msg: "本体服务不可达: " + err.Error()}
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, &store.HTTPError{Status: resp.StatusCode, Msg: string(buf)}
	}
	return buf, nil
}

func (s *Server) buildPlanePUT(ctx context.Context, path string, body []byte) error {
	tr := &http.Client{Timeout: 60 * time.Second} // spec 校验可达 LLM 质量面——宽限
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.Ontology.BuildURL+path, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := tr.Do(req)
	if err != nil {
		return &store.HTTPError{Status: http.StatusBadGateway, Msg: "本体服务不可达: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(resp.Body)
		return &store.HTTPError{Status: http.StatusBadGateway, Msg: "spec 校验未通过: " + string(buf)}
	}
	return nil
}

// importProjectFileToKB POST /api/kb/{id}/import-project-file {project_id, file_id}
// → 项目产物一键入库（读绑定目录/FilesRoot 文件文本层——二进制走既有富格式上传通道）。
func (s *Server) importProjectFileToKB(w http.ResponseWriter, r *http.Request) {
	kbID := r.PathValue("id")
	var in struct {
		ProjectID string `json:"project_id"`
		FileID    string `json:"file_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	pf, err := s.Store.GetProjectFile(in.FileID)
	if err != nil || pf == nil || pf.ProjectID != in.ProjectID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "项目文件不存在或不属于该项目"})
		return
	}
	proj, err := s.Store.GetProject(pf.ProjectID)
	if err != nil {
		writeErr(w, err)
		return
	}
	fullPath, jerr := safeJoinPath(s.projectRoot(proj), pf.Path)
	if jerr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "路径越界"})
		return
	}
	full, err := os.ReadFile(fullPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "文件读取失败（绑定目录文件可能已移除）: " + err.Error()})
		return
	}
	if isBinaryContent(full) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "二进制文件请经知识库「上传文档」富格式通道（REQ-414）"})
		return
	}
	doc, err := s.KB.ImportAsync(r.Context(), kbID, pf.Name, string(full))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"doc_id": doc.ID, "title": doc.Title, "status": doc.Status})
}

// safeJoinPath 路径越界守卫（fsutil.SafeJoin 同款——handlers_projects 同源）。
func safeJoinPath(root, rel string) (string, error) {
	return fsutil.SafeJoin(root, rel)
}

// isBinaryContent 二进制判定（前 512B 含 NUL 即判二进制——Git 口径简化）。
func isBinaryContent(b []byte) bool {
	n := len(b)
	if n > 512 {
		n = 512
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}
