package api

// REQ-529/M336（P17）：沉淀链出料口——伴生图→正式本体晋升 + 项目产物→KB 一键入库。
// REQ-546/M353（决策结构化借鉴批③；105 号 P1-5）：晋升补「评审」半边——promote 改产
// pending 提案（learning proposal 暂存区，原直接 PUT 退役），裁决端点 approve 执行入库
// （重取 digest 取交集防伴生图漂移）/ reject 留痕（decided_note=reject route 素材，
// 衔接 REQ-544 decisions[]）；promotion-candidates 清单端点供裁决卡与审计回看。
// 审计半边：spec PUT 既有 auditOntologyProxy 同面覆盖（与 M336 直推同址同源）。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/companion"
	"github.com/ontoagents/ontoagents/backend/internal/fsutil"
	"github.com/ontoagents/ontoagents/backend/internal/store"
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
	// sparql-results+json: {"results": {"bindings": [...]}}
	var wrapper struct {
		Results struct {
			Bindings []companionBinding `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("伴生图结果解析失败: %w", err)
	}
	return wrapper.Results.Bindings, nil
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

// filterDigestByLabels 实体勾选过滤（REQ-529 挂账「逐条勾选」合流）：实体取勾选集与
// 活跃集交集；关系保留两端均在勾选集的活跃边（缺端即悬空，不随提案）。
func filterDigestByLabels(d *companionGraphDigest, selected []string) *companionGraphDigest {
	sel := map[string]bool{}
	for _, l := range selected {
		if l != "" {
			sel[l] = true
		}
	}
	out := &companionGraphDigest{Labels: d.Labels}
	for _, e := range d.Entities {
		if sel[e] {
			out.Entities = append(out.Entities, e)
		}
	}
	for _, edge := range d.Relations {
		if sel[edge[0]] && sel[edge[2]] {
			out.Relations = append(out.Relations, edge)
		}
	}
	return out
}

// intersectDigestPayload 裁决时点漂移防护（105 号 E2「写回=validate+评审」：入库以
// 当前活跃集为准）——提案快照 ∩ 当前活跃 digest：实体取双集合交，边取原样仍活跃者。
func intersectDigestPayload(payload *store.PromotionPayload, cur *companionGraphDigest) *companionGraphDigest {
	inCur := map[string]bool{}
	for _, e := range cur.Entities {
		inCur[e] = true
	}
	out := &companionGraphDigest{Labels: cur.Labels}
	for _, e := range payload.Entities {
		if inCur[e] {
			out.Entities = append(out.Entities, e)
		}
	}
	edgeSet := map[[3]string]bool{}
	for _, edge := range cur.Relations {
		edgeSet[edge] = true
	}
	for _, edge := range payload.Relations {
		if edgeSet[edge] {
			out.Relations = append(out.Relations, edge)
		}
	}
	return out
}

// promoteCompanion POST /api/companion/ontologies/{id}/promote
// {target_ontology_id, entity_ids?[], note?} → 产 pending 提案（REQ-546 改产提案——
// 原直接 PUT 退役）：entity_ids 非空按勾选过滤（REQ-529 挂账「逐条勾选」合流），
// 缺省=全量活跃集；提案快照冻结发起时点 digest，实际入库在 approve 裁决时点执行。
func (s *Server) promoteCompanion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		TargetOntologyID string   `json:"target_ontology_id"`
		EntityIDs        []string `json:"entity_ids"`
		Note             string   `json:"note"`
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
	if len(in.EntityIDs) > 0 {
		digest = filterDigestByLabels(digest, in.EntityIDs)
		if len(digest.Entities) == 0 && len(digest.Relations) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "勾选实体均不在伴生图活跃集——请核对后重选"})
			return
		}
	}
	p := &store.PromotionProposal{
		SourceOntologyID: id,
		TargetOntologyID: in.TargetOntologyID,
		Note:             truncNote300(in.Note),
	}
	for _, e := range digest.Entities {
		p.Payload.Entities = append(p.Payload.Entities, e)
	}
	p.Payload.Relations = append(p.Payload.Relations, digest.Relations...)
	p.EntityCount, p.RelationCount = len(p.Payload.Entities), len(p.Payload.Relations)
	if err := s.Store.CreatePromotionProposal(p); err != nil {
		writeErr(w, err)
		return
	}
	saved, err := s.Store.GetPromotionProposal(p.ID)
	if err != nil || saved == nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// listPromotionProposals GET /api/companion/promotion-proposals
// ?source_ontology_id=&target_ontology_id=&status= ——提案清单（裁决卡数据源+审计回看；
// 被拒提案即 reject route 素材，衔接 REQ-544 decisions[]）。
func (s *Server) listPromotionProposals(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListPromotionProposals(
		r.URL.Query().Get("source_ontology_id"),
		r.URL.Query().Get("target_ontology_id"),
		r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []*store.PromotionProposal{}
	}
	writeJSON(w, http.StatusOK, list)
}

// decidePromotionProposal POST /api/companion/promotion-proposals/{id}/decide
// {action: "approve"|"reject", note?} ——裁决入口（仅 pending；终态不可再裁）。
// approve=重取源伴生图 digest 取交集（漂移防护）→既有去重合并→PUT 统一校验循环入库
// →置 approved；reject=留痕（note=拒绝理由，reject route 素材）。
func (s *Server) decidePromotionProposal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	switch in.Action {
	case "reject":
		p, err := s.Store.DecidePromotionProposal(id, "rejected", truncNote300(in.Note))
		if err != nil {
			writeDecideErr(w, p, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	case "approve":
		proposal, err := s.Store.GetPromotionProposal(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		if proposal == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "提案不存在"})
			return
		}
		if proposal.Status != "pending" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "提案已裁决（" + proposal.Status + "）——仅 pending 可裁决"})
			return
		}
		cur, err := s.companionGraphDigest(r.Context(), proposal.SourceOntologyID)
		if err != nil {
			writeErr(w, err)
			return
		}
		d := intersectDigestPayload(&proposal.Payload, cur)
		if len(d.Entities) == 0 && len(d.Relations) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "提案内容已不在伴生图活跃集（伴生图已漂移）——可拒绝留痕后重新发起"})
			return
		}
		applied, err := s.applyDigestToTarget(r.Context(), proposal.TargetOntologyID, d)
		if err != nil {
			writeErr(w, err) // 提案保持 pending——可修复后重试
			return
		}
		p, err := s.Store.DecidePromotionProposal(id, "approved", truncNote300(in.Note))
		if err != nil {
			writeDecideErr(w, p, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"proposal": p, "promoted_concepts": applied.Concepts,
			"promoted_relations": applied.Relations, "skipped_existing": applied.Skipped,
			"target": proposal.TargetOntologyID,
		})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action 仅 approve|reject"})
	}
}

// writeDecideErr 裁决守卫错误映射：不存在 404 / 已裁决 409。
func writeDecideErr(w http.ResponseWriter, p *store.PromotionProposal, err error) {
	if err == store.ErrPromotionAlreadyDecided {
		status := "approved"
		if p != nil {
			status = p.Status
		}
		writeJSON(w, http.StatusConflict, map[string]string{"error": "提案已裁决（" + status + "）——仅 pending 可裁决"})
		return
	}
	writeErr(w, err)
}

// truncNote300 注记截断（沿 store.truncateNote 300 rune 口径——防滥用）。
func truncNote300(s string) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) > 300 {
		return string(rs[:300])
	}
	return string(rs)
}

// promoteResult 入库计数（approve 响应与原 M336 响应同形）。
type promoteResult struct {
	Concepts int `json:"promoted_concepts"`
	Relations int `json:"promoted_relations"`
	Skipped  int `json:"skipped_existing"`
}

// applyDigestToTarget digest 写入目标本体 spec 草稿（去重合并——已存在同名概念/同边关系
// 跳过），PUT 构建平面走统一校验循环（Spec.Validate；校验失败构建平面 400 明细透传，
// 调用方提案保持 pending 可重试）。M336 原直推链原样迁此。
func (s *Server) applyDigestToTarget(ctx context.Context, targetOntologyID string, d *companionGraphDigest) (*promoteResult, error) {
	specRaw, err := s.buildPlaneGET(ctx, "/api/ontologies/"+targetOntologyID+"/spec")
	if err != nil {
		return nil, err
	}
	var spec struct {
		Name      string           `json:"name"`
		Concepts  []map[string]any `json:"concepts"`
		Relations []map[string]any `json:"relations"`
		Instances []map[string]any `json:"instances"`
		CQ        []string         `json:"cq"`
	}
	if err := json.Unmarshal(specRaw, &spec); err != nil {
		return nil, &store.HTTPError{Status: http.StatusBadGateway, Msg: "目标本体 spec 解析失败: " + err.Error()}
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
	out := &promoteResult{}
	for _, e := range d.Entities {
		if haveConcept[e] {
			out.Skipped++
			continue
		}
		spec.Concepts = append(spec.Concepts, map[string]any{"name": e})
		haveConcept[e] = true
		out.Concepts++
	}
	for _, edge := range d.Relations {
		key := edge[1] + "\x00" + edge[0] + "\x00" + edge[2]
		if !haveConcept[edge[0]] || !haveConcept[edge[2]] || haveEdge[key] {
			out.Skipped++
			continue
		}
		spec.Relations = append(spec.Relations, map[string]any{"name": edge[1], "from": edge[0], "to": edge[2]})
		haveEdge[key] = true
		out.Relations++
	}
	patch, err := json.Marshal(map[string]any{
		"name": spec.Name, "concepts": spec.Concepts, "relations": spec.Relations,
		"instances": spec.Instances, "cq": spec.CQ,
	})
	if err != nil {
		return nil, err
	}
	if err := s.buildPlanePUT(ctx, "/api/ontologies/"+targetOntologyID+"/spec", patch); err != nil {
		return nil, err
	}
	return out, nil
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
