// rest_assets.go 资产治理域端点（REQ-340 A1 自 rest.go 拆分，80 号 A1）：质量门禁两族+配置/版本四连+发布状态机（REQ-239）/diff+impact 底座（REQ-278）/fork/artifacts/导出。
package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"

	"github.com/ontoagents/ontoagents/ontology-service/internal/importer"
	"github.com/ontoagents/ontoagents/ontology-service/internal/qualitygate"
	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

func (s *Server) qualityCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OntologyID string             `json:"ontology_id"`
		Spec       *pkgspec.Spec      `json:"spec"`
		Strict     bool               `json:"strict"`
		Config     qualitygate.Config `json:"config"`
		Save       *bool              `json:"save"`
		Reasoning  *bool              `json:"reasoning"` // REQ-255②：显式请求推理检查档（未带时读本 体配置 reasoning_check）
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	var sp *pkgspec.Spec
	if req.Spec != nil {
		sp = req.Spec
	} else if req.OntologyID != "" {
		raw, _, err := s.Store.GetArtifact(req.OntologyID, "spec_json")
		if err != nil {
			writeErr(w, err)
			return
		}
		sp = &pkgspec.Spec{}
		if err := json.Unmarshal([]byte(raw), sp); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "spec_json 解析失败: " + err.Error()})
			return
		}
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ontology_id 或 spec 必填其一"})
		return
	}
	rep := qualitygate.Check(sp, req.Config)
	rep.Strict = req.Strict
	if !req.Strict {
		rep.Pass = true // 宽松模式（默认）：仅告警不阻断
	}
	out := map[string]any{"report": rep}
	// REQ-255② 推理级检查档（owlrl OWL 2 RL 闭包一致性；默认关沿低侵入②）：
	// 本体配置 reasoning_check 开启或本次请求显式 reasoning=true 时执行——
	// 导出 TTL（sidecar export）→ sidecar reason → 命中以错误级独立检查项 reasoning_owlrl 并入报告。
	reasoningOn := req.Reasoning != nil && *req.Reasoning
	if !reasoningOn && req.OntologyID != "" {
		if o, gerr := s.Store.GetOntology(req.OntologyID); gerr == nil {
			reasoningOn = o.ReasoningCheck
		}
	}
	if reasoningOn && s.Sidecar != nil && req.OntologyID != "" {
		// 检查对象优先取 original 形态（owl/turtle——真实资产含公理才有检出意义；
		// spec_json 子集无公理，其导出基本恒一致），无 original 退回 spec 导出并标注来源。
		ttl, src, rdfFmt := "", "spec_export", "turtle"
		var terr error
		if c, _, gerr := s.Store.GetArtifact(req.OntologyID, "turtle"); gerr == nil && c != "" {
			ttl, src = c, "original_turtle"
		} else if c, _, gerr := s.Store.GetArtifact(req.OntologyID, "owl_rdfxml"); gerr == nil && c != "" {
			ttl, src, rdfFmt = c, "original_owl_rdfxml", "owl_rdfxml"
		} else {
			ttl, terr = importer.ExportTTL(req.OntologyID, sp)
		}
		if terr == nil {
			res, rerr := s.reasonCheck(ttl, rdfFmt)
			switch {
			case rerr != nil:
				out["reasoning_error"] = rerr.Error()
			case res.Consistent == nil || res.Error != "":
				rep.WarningCount++
				rep.Findings = append(rep.Findings, qualitygate.Finding{CheckID: "reasoning_owlrl", Title: "OWL 2 RL 推理一致性（owlrl）", Dimension: qualitygate.DimConsistency, Severity: qualitygate.SevWarning, Count: 1, Samples: []string{res.Error}})
			case !*res.Consistent:
				samples := res.Violations
				if len(samples) > 3 {
					samples = samples[:3]
				}
				rep.ErrorCount += len(res.Violations)
				rep.Findings = append(rep.Findings, qualitygate.Finding{CheckID: "reasoning_owlrl", Title: "OWL 2 RL 推理一致性（owlrl）", Dimension: qualitygate.DimConsistency, Severity: qualitygate.SevError, Count: len(res.Violations), Samples: samples})
			}
			if res != nil {
				out["reasoning"] = map[string]any{"consistent": res.Consistent, "violation_count": len(res.Violations), "source": src}
			}
		} else {
			out["reasoning_error"] = "TTL 导出失败: " + terr.Error()
		}
	}
	// 落库：已存本体默认写 quality-report artifact（可摘除=删该形态 artifact）；内联草稿不落
	if req.OntologyID != "" && (req.Save == nil || *req.Save) {
		if b, err := json.Marshal(rep); err == nil {
			if err := s.Store.PutArtifact(req.OntologyID, "quality-report", string(b), false); err != nil {
				writeJSON(w, http.StatusOK, map[string]any{"report": rep, "artifact_error": err.Error()})
				return
			}
		}
		out["artifact_saved"] = true
		// REQ-331/M131：评分历史追加一行（source=manual——质量卡手动生成时刻；失败不阻断）
		if o, gerr := s.Store.GetOntology(req.OntologyID); gerr == nil {
			_ = s.Store.SaveQualityScore(repo.QualityScoreRow{
				OntologyID: req.OntologyID, Version: o.Version, Source: "manual",
				Overall: rep.Score.Overall, Completeness: rep.Score.Completeness,
				Consistency: rep.Score.Consistency, Maintainability: rep.Score.Maintainability,
				ErrorCount: rep.ErrorCount, WarningCount: rep.WarningCount,
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// qualityRemediations GET /api/ontology/quality/remediations → 检查项修复指引全量
// （REQ-331②：finding 行按 check_id 取「怎么改」人话指引；只读静态，前端可长期缓存）。
func (s *Server) qualityRemediations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, qualitygate.AllRemediations())
}

// qualityHistory GET /api/ontologies/{id}/quality-history → 评分历史（REQ-331①：
// 版本×综合分趋势线+草稿→入库→当前三点对照取数源；时间正序）。
func (s *Server) qualityHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListQualityScores(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ontology_id": r.PathValue("id"), "history": rows, "count": len(rows)})
}

// qualityReport 读取最近一次质量报告 artifact（GET /api/ontology/quality/report?ontology_id=）。
func (s *Server) qualityReport(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("ontology_id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ontology_id 必填"})
		return
	}
	raw, importedAt, err := s.Store.GetArtifact(id, "quality-report")
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	writeJSON(w, http.StatusOK, map[string]any{"ontology_id": id, "imported_at": importedAt, "report": json.RawMessage(raw)})
}

// ontoextendListODPs M-O14 P2②（REQ-171 P2/26 号方案）：ODP 精选清单（人工 curated，编译期内嵌）

// reasonCheckEndpoint POST /api/ontologies/reason-check（REQ-480/M280）：运行平面组装载前
// 并置一致性检查的承载端——闭包全集 TTL 合并后经 sidecar reason（owlrl OWL 2 RL）跑检查，
// 结果透传（consistent/violations/error/note）。复用 REQ-255① reason 管线零新依赖；
// sidecar 未配置时返回错误（运行平面降级为日志注记跳过，不阻断）。
func (s *Server) reasonCheckEndpoint(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Content string `json:"content"`
		Format  string `json:"format"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(in.Content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content 必填（并置 TTL 全文）"})
		return
	}
	if in.Format == "" {
		in.Format = "turtle"
	}
	res, err := s.reasonCheck(in.Content, in.Format)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// materializeEndpoint POST /api/ontologies/materialize（REQ-481/M281）：运行平面组装载前
// 并置物化的承载端——闭包全集 TTL 合并经 sidecar materialize（owlrl OWL 2 RL 数据级闭包），
// 推断三元组 Turtle 直出正文 + X-Onto-Inferred-Triples 精确计数头（物化增量留痕）。
func (s *Server) materializeEndpoint(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Content string `json:"content"`
		Format  string `json:"format"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(in.Content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content 必填（并置 TTL 全文）"})
		return
	}
	if in.Format == "" {
		in.Format = "turtle"
	}
	ttl, n, err := importer.Materialize(s.Sidecar, in.Format, in.Content)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/turtle; charset=utf-8")
	w.Header().Set("X-Onto-Inferred-Triples", strconv.Itoa(n))
	_, _ = w.Write([]byte(ttl))
}

func (s *Server) reasonCheck(content, format string) (*sidecarReasonOut, error) {
	if s.Sidecar == nil || s.Sidecar.Python == "" || s.Sidecar.Script == "" {
		return nil, fmt.Errorf("sidecar 未配置（SIDECAR_SCRIPT）")
	}
	cmd := exec.Command(s.Sidecar.Python, s.Sidecar.Script, "reason", "--format", format)
	cmd.Stdin = strings.NewReader(content)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if strings.Contains(errb.String(), "owlrl 未安装") {
			return nil, fmt.Errorf("owlrl 未安装（pip install owlrl）——推理检查档依赖缺失")
		}
		return nil, fmt.Errorf("sidecar reason: %w; stderr: %s", err, strings.TrimSpace(errb.String()))
	}
	var res sidecarReasonOut
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("sidecar reason 输出解析失败: %w", err)
	}
	return &res, nil
}

// cqSparqlReq/cqSparqlItem CQ→SPARQL 翻译（60 号 H2：LLM 辅助翻译+人工确认模板）。
type cqSparqlItem struct {
	CQ     string `json:"cq"`
	Sparql string `json:"sparql"`
}

// cqSparql POST /api/ontologies/{id}/cq-sparql：spec.CQ 逐条翻译为 SPARQL SELECT
// （单次 LLM 批量翻译；IRI 模板按 sidecar 导出 urn:o:{oid}:concept:{name} 口径）。
// 返回 items 供前端人工确认/编辑后经运行方案 SPARQL 端点执行（通过率入质量卡）。

func (s *Server) qualityConfigGet(w http.ResponseWriter, r *http.Request) {
	o, err := s.Store.GetOntology(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ontology_id": o.ID, "strict": o.QualityStrict, "reasoning_check": o.ReasoningCheck})
}

// qualityConfigPut strict + reasoning_check（REQ-255②）两开关独立可写（指针判空=只改传入项）。
func (s *Server) qualityConfigPut(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Strict         *bool `json:"strict"`
		ReasoningCheck *bool `json:"reasoning_check"`
	}
	if err := decodeJSON(r, &body); err != nil || (body.Strict == nil && body.ReasoningCheck == nil) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "strict / reasoning_check（bool）至少填一项"})
		return
	}
	if body.Strict != nil {
		if err := s.Store.SetQualityStrict(id, *body.Strict); err != nil {
			writeErr(w, err)
			return
		}
	}
	if body.ReasoningCheck != nil {
		if err := s.Store.SetReasoningCheck(id, *body.ReasoningCheck); err != nil {
			writeErr(w, err)
			return
		}
	}
	o, _ := s.Store.GetOntology(id)
	writeJSON(w, http.StatusOK, map[string]any{"ontology_id": id, "strict": o.QualityStrict, "reasoning_check": o.ReasoningCheck})
}

// importLimits 导入防线阈值（REQ-305）：拒绝语义取代历史 64MB LimitReader 的静默截断。
// IMPORT_MAX_BODY_MB：请求体上限 MB（默认 64，与历史硬顶一致——只改「超了怎么办」不改「多大能进」；<=0 不限制）；
// IMPORT_MAX_SPEC_OBJECTS：归一化 spec 五层元素总数上限（默认 50000，真实资产误伤极小；<=0 不限制）。

func (s *Server) artifacts(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListArtifacts(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// putArtifactContent REQ-240⑥/M66：通用形态写入（图布局持久化 layout_json 等）。
// 限制为派生数据形态（layout_json），不收 spec_json——spec 走 PUT /spec 校验门控路径。
func (s *Server) putArtifactContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	format := r.PathValue("format")
	if format != "layout_json" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "仅接受 layout_json（图布局派生数据）；spec 写入走 PUT /spec 门控路径"})
		return
	}
	if _, err := s.Store.GetOntology(id); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeErr(w, err)
		return
	}
	if !json.Valid(body) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "layout_json 须为合法 JSON"})
		return
	}
	if err := s.Store.PutArtifact(id, format, string(body), false); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "format": format, "size": len(body)})
}

// getArtifactContent REQ-240⑥/M66：通用形态读取（未存返回 404，前端静默降级默认布局）。
func (s *Server) getArtifactContent(w http.ResponseWriter, r *http.Request) {
	content, _, err := s.Store.GetArtifact(r.PathValue("id"), r.PathValue("format"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

// ---- 导入 / 导出 ----

// exportSpecTTL spec_json → TTL 导出公共体（REQ-540 抽取自 exportOntology 头版本分支，
// 头版本与版本化导出共用）：桥投影（REQ-479）+ ExportTTLCount + 未投影公理透传（REQ-318）。
// includeRawAxioms=false（版本化导出）跳过 axioms-raw 拼接——该 artifact 为头版本级内容，
// 无版本快照，拼进历史版本导出会混入异版本内容（诚实边界，REQ-541 发布快照落库后可解）。
func (s *Server) exportSpecTTL(id, rawSpec string, includeRawAxioms bool) (string, int, error) {
	var sp pkgspec.Spec
	if err := json.Unmarshal([]byte(rawSpec), &sp); err != nil {
		return "", 0, err
	}
	// REQ-479/M279：external_refs 旁挂表 → 导出桥投影（读失败降级无桥导出，不阻断导出主链）。
	var bridges []importer.ExtRefBridge
	if refs, rerr := s.Store.ListExternalRefs(id); rerr != nil {
		log.Printf("[req479] external_refs 读取失败 ontology=%s: %v（降级无桥导出）", id, rerr)
	} else {
		for _, ref := range refs {
			bridges = append(bridges, importer.ExtRefBridge{
				FromConcept: ref.FromConcept, ToOntoID: ref.ToOntoID, ToConcept: ref.ToConcept,
				RefKind: ref.RefKind, Predicate: ref.Predicate,
			})
		}
	}
	ttl, triples, err := importer.ExportTTLCount(id, &sp, bridges...)
	if err != nil {
		return "", 0, err
	}
	// REQ-318/M120：未投影公理透传还原——axioms-raw artifact 片段原样拼接（77 号 §6）。
	// 拼接后精确计数未知：移除 X-Onto-Triples 头（运行平面装载对账 gate=skipped 诚实口径，
	// 防「头计数 < 实际行数」硬闸误中止）；片段引用被改名/删除的 IRI 时原样保留可能悬空（一期边界）。
	if includeRawAxioms {
		if raw_ax, _, axerr := s.Store.GetArtifact(id, "axioms-raw"); axerr == nil && raw_ax != "" {
			var envelope struct {
				Fragments []string `json:"fragments"`
			}
			if json.Unmarshal([]byte(raw_ax), &envelope) == nil && len(envelope.Fragments) > 0 {
				ttl += "\n\n# —— 未投影公理透传（REQ-318：导入时无法投影的公理原样还原；引用对象被改名/删除时可能悬空）——\n" +
					strings.Join(envelope.Fragments, "\n")
				triples = -1
			}
		}
	}
	return ttl, triples, nil
}

// lifecyclePlan REQ-155 阶段二：生命周期计划（期望=方案声明全部运行；实际=运行平面状态+加载版本快照；漂移=spec 版本超前）。

func (s *Server) exportOntology(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "turtle"
	}
	// REQ-540/M347：版本化导出——?version=N 取 ontology_version 该版本 spec_json 快照导出
	//（运行面 FetchTTL 有 pin 按 pin 解析的数据源；缺省/0=头版本，行为不变零迁移向后兼容）。
	// 快照不存在（含版本号超前）→ 404（pin 钉死语义：解析失败显式可见，不静默回落头版本）。
	specVersion := 0
	if vs := strings.TrimSpace(r.URL.Query().Get("version")); vs != "" {
		v, perr := strconv.Atoi(vs)
		if perr != nil || v < 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("version 参数须为非负整数（0/缺省=头版本），得到 %q", vs)})
			return
		}
		specVersion = v
	}
	o, err := s.Store.GetOntology(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	switch format {
	case "turtle":
		// REQ-540：版本化导出分支（pin 解析）——读指定版本 spec_json 快照重导出。
		// axioms-raw 透传还原不入本分支（artifact 为头版本级、无版本快照）——诚实边界：
		// pin 版本导出=该版本 spec 投影+桥三元组，未投影公理余量缺失（REQ-541 发布快照落库后可解）。
		if specVersion > 0 {
			raw, err := s.Store.GetVersionSpec(id, specVersion)
			if err != nil {
				writeErr(w, fmt.Errorf("本体 %s 无版本 %d 快照: %w", id, specVersion, err))
				return
			}
			ttl, triples, terr := s.exportSpecTTL(id, raw, false)
			if terr != nil {
				writeErr(w, terr)
				return
			}
			w.Header().Set("Content-Type", "text/turtle; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s_v%d.ttl", o.ID, specVersion))
			if triples >= 0 {
				w.Header().Set("X-Onto-Triples", strconv.Itoa(triples))
			}
			w.Header().Set("X-Onto-Version", strconv.Itoa(specVersion))
			w.Header().Set("X-Onto-Source", "spec_export") // 版本快照重导出（非头版本 original 直挂）
			_, _ = w.Write([]byte(ttl))
			return
		}
		// REQ-328/C 运行装载双源——未编辑直挂分支：导入源本体且导入后零编辑（version=1，
		// 任何内容变更动线都 bump 版本）→ original 全图 canonical Turtle 直出（rdflib 图同构
		// 100% 无损），X-Onto-Triples=原图精确计数（REQ-293① 硬闸照常）；直挂失败回落 spec 重导出。
		if o.Version == 1 {
			for _, af := range []string{"turtle", "owl_rdfxml"} {
				orig, _, aerr := s.Store.GetArtifact(id, af)
				if aerr != nil || orig == "" {
					continue
				}
				ttl, triples, cerr := importer.CanonicalTTL(s.Sidecar, af, orig)
				if cerr != nil {
					// 直挂失败诚实回落（原文件损坏等）——spec 重导出+余量账本拼接仍可装载
					log.Printf("[req328] canonical 直挂失败 ontology=%s format=%s: %v（回落 spec 导出）", id, af, cerr)
					break
				}
				w.Header().Set("Content-Type", "text/turtle; charset=utf-8")
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s_v%d.ttl", o.ID, o.Version))
				w.Header().Set("X-Onto-Triples", strconv.Itoa(triples))
				w.Header().Set("X-Onto-Source", "original") // REQ-328/C：数据源诚实标注（未编辑=100% 导入源）
				w.Header().Set("X-Onto-Version", strconv.Itoa(o.Version))
				_, _ = w.Write([]byte(ttl))
				return
			}
		}
		raw, _, err := s.Store.GetArtifact(id, "spec_json")
		if err != nil {
			writeErr(w, err)
			return
		}
		ttl, triples, terr := s.exportSpecTTL(id, raw, true)
		if terr != nil {
			writeErr(w, terr)
			return
		}
		w.Header().Set("Content-Type", "text/turtle; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s_v%d.ttl", o.ID, o.Version))
		// REQ-293①：三元组精确计数响应头（sidecar rdflib len(g)）——运行平面装载计数硬闸对账数据源
		if triples >= 0 {
			w.Header().Set("X-Onto-Triples", strconv.Itoa(triples))
		}
		w.Header().Set("X-Onto-Version", strconv.Itoa(o.Version))
		w.Header().Set("X-Onto-Source", "spec_export") // REQ-328/C：数据源诚实标注（编辑后=投影+余量还原）
		_, _ = w.Write([]byte(ttl))
	case "vowljson":
		// WebVOWL 对照视图数据源（2026-09-27 转换链重构）：spec → VOWL JSON 本服务直出，
		// 替代不可用的浏览器端 owl2vowl（Java-only 无浏览器分发，见 importer/vowljson.go）
		raw, _, err := s.Store.GetArtifact(id, "spec_json")
		if err != nil {
			writeErr(w, err)
			return
		}
		var sp pkgspec.Spec
		if err := json.Unmarshal([]byte(raw), &sp); err != nil {
			writeErr(w, err)
			return
		}
		vj, err := importer.ExportVOWLJSON(&sp)
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename*=UTF-8''%s_v%d_vowl.json", o.ID, o.Version))
		_, _ = w.Write(vj)
	default:
		raw, _, err := s.Store.GetArtifact(id, format)
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s_v%d.%s", o.ID, o.Version, format))
		_, _ = w.Write([]byte(raw))
	}
}

// ---- guide（主平台 M8 注入用，§5 集成契约）----

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetOntology(id); err != nil {
		writeErr(w, err)
		return
	}
	vs, err := s.Store.ListVersions(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if vs == nil {
		vs = []repo.VersionMeta{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ontology_id": id, "versions": vs})
}

// versionOriginal GET /api/ontologies/{id}/versions/{version}/original：按版本读取原始源文件（REQ-93 源码视图）。
func (s *Server) versionOriginal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || v <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "版本号必须是正整数"})
		return
	}
	content, format, err := s.Store.GetVersionOriginal(id, v)
	if err != nil {
		writeErr(w, err)
		return
	}
	ct := "text/plain; charset=utf-8"
	switch format {
	case "turtle":
		ct = "text/turtle; charset=utf-8"
	case "owl_rdfxml":
		ct = "application/rdf+xml; charset=utf-8"
	case "spec_json":
		ct = "application/json; charset=utf-8"
	case "csv":
		ct = "text/csv; charset=utf-8"
	case "graphml":
		ct = "application/graphml; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Ontology-Version", strconv.Itoa(v))
	_, _ = w.Write([]byte(content))
}

// versionSpec GET /api/ontologies/{id}/versions/{version}/spec：按版本读取 spec_json 快照原文（REQ-145/M22 A3 前端文本 diff 的数据面；只读）。
func (s *Server) versionSpec(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetOntology(id); err != nil {
		writeErr(w, err)
		return
	}
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || v <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "版本号必须是正整数"})
		return
	}
	raw, err := s.Store.GetVersionSpec(id, v)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Ontology-Version", strconv.Itoa(v))
	_, _ = w.Write([]byte(raw))
}

// ---- REQ-239/M65 版本发布状态机 ----

// publishOntology POST /api/ontologies/{id}/publish {version_name?}：发布当前版本为命名快照终态。
// 空命名默认 v{N}；历史快照回滚=对恢复后的新版本再次发布（审计友好，版本号单调）。
func (s *Server) publishOntology(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VersionName string `json:"version_name"`
	}
	_ = decodeJSON(r, &req) // 空请求体（Content-Length 0）允许：默认命名
	o, err := s.Store.Publish(r.PathValue("id"), req.VersionName)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// unpublishOntology POST /api/ontologies/{id}/unpublish：撤回发布回 draft（命名清空）。
func (s *Server) unpublishOntology(w http.ResponseWriter, r *http.Request) {
	o, err := s.Store.Unpublish(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// restoreVersion POST /api/ontologies/{id}/versions/{version}/restore：历史快照回滚——
// 指定版本内容恢复为**新版本**（BumpVersion+写版本历史），当前态回 draft（REQ-239⑤ 重发布动作口径）。
func (s *Server) restoreVersion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetOntology(id); err != nil {
		writeErr(w, err)
		return
	}
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || v <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "版本号必须是正整数"})
		return
	}
	newV, err := s.Store.RestoreVersion(id, v)
	if err != nil {
		writeErr(w, err)
		return
	}
	// REQ-292 v1 语义：版本恢复=人工钉定历史态——overlay 全清（此后重算按恢复态为 base，预期行为）
	cleared, _ := s.Store.ClearOverlays(id)
	o, _ := s.Store.GetOntology(id)
	writeJSON(w, http.StatusOK, map[string]any{"restored_from": v, "new_version": newV, "overlays_cleared": cleared, "ontology": o})
}

// ---- REQ-95 版本 diff ----

type diffFieldDelta struct {
	From any `json:"from,omitempty"`
	To   any `json:"to,omitempty"`
}

type diffChangedItem struct {
	Name   string                    `json:"name"`
	Fields map[string]diffFieldDelta `json:"fields"`
}

type diffSet struct {
	Added   []any             `json:"added"`
	Removed []any             `json:"removed"`
	Changed []diffChangedItem `json:"changed"`
}

type diffImpact struct {
	Name         string `json:"name"`
	ReferencedBy int    `json:"referenced_by"`
}

func newDiffSet() diffSet {
	return diffSet{Added: []any{}, Removed: []any{}, Changed: []diffChangedItem{}}
}

func strDelta(from, to string) diffFieldDelta {
	d := diffFieldDelta{}
	if from != "" {
		d.From = from
	}
	if to != "" {
		d.To = to
	}
	return d
}

func strSliceDelta(from, to []string) diffFieldDelta {
	d := diffFieldDelta{}
	if len(from) > 0 {
		d.From = from
	}
	if len(to) > 0 {
		d.To = to
	}
	return d
}

func attrsDelta(from, to map[string]any) diffFieldDelta {
	d := diffFieldDelta{}
	if len(from) > 0 {
		d.From = from
	}
	if len(to) > 0 {
		d.To = to
	}
	return d
}

func relsDelta(from, to []pkgspec.InstanceRel) diffFieldDelta {
	d := diffFieldDelta{}
	if len(from) > 0 {
		d.From = from
	}
	if len(to) > 0 {
		d.To = to
	}
	return d
}

// sortedStrings 返回副本排序结果（parents 视为无序集合比较）。
func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as, bs := sortedStrings(a), sortedStrings(b)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

// jsonValueEqual 以确定性 JSON 序列化比较任意 JSON 值（map 键有序）。
func jsonValueEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ab) == string(bb)
}

func attrsEqual(a, b map[string]any) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return jsonValueEqual(a, b)
}

func sortedRels(in []pkgspec.InstanceRel) []pkgspec.InstanceRel {
	out := append([]pkgspec.InstanceRel(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rel != out[j].Rel {
			return out[i].Rel < out[j].Rel
		}
		return out[i].Target < out[j].Target
	})
	return out
}

func instanceRelsEqual(a, b []pkgspec.InstanceRel) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return jsonValueEqual(sortedRels(a), sortedRels(b))
}

func diffConcepts(from, to []pkgspec.Concept) diffSet {
	out := newDiffSet()
	fm := make(map[string]pkgspec.Concept, len(from))
	for _, c := range from {
		fm[c.Name] = c
	}
	tm := make(map[string]pkgspec.Concept, len(to))
	for _, c := range to {
		tm[c.Name] = c
	}
	for _, c := range to {
		if _, ok := fm[c.Name]; !ok {
			out.Added = append(out.Added, c)
		}
	}
	for _, c := range from {
		if _, ok := tm[c.Name]; !ok {
			out.Removed = append(out.Removed, c)
		}
	}
	for _, c := range to {
		o, ok := fm[c.Name]
		if !ok {
			continue
		}
		fields := map[string]diffFieldDelta{}
		if o.Label != c.Label {
			fields["label"] = strDelta(o.Label, c.Label)
		}
		if o.Definition != c.Definition {
			fields["definition"] = strDelta(o.Definition, c.Definition)
		}
		if !stringSlicesEqual(o.Parents, c.Parents) {
			fields["parents"] = strSliceDelta(o.Parents, c.Parents)
		}
		if len(fields) > 0 {
			out.Changed = append(out.Changed, diffChangedItem{Name: c.Name, Fields: fields})
		}
	}
	return out
}

func diffRelations(from, to []pkgspec.Relation) diffSet {
	out := newDiffSet()
	fm := make(map[string]pkgspec.Relation, len(from))
	for _, r := range from {
		fm[r.Name] = r
	}
	tm := make(map[string]pkgspec.Relation, len(to))
	for _, r := range to {
		tm[r.Name] = r
	}
	for _, r := range to {
		if _, ok := fm[r.Name]; !ok {
			out.Added = append(out.Added, r)
		}
	}
	for _, r := range from {
		if _, ok := tm[r.Name]; !ok {
			out.Removed = append(out.Removed, r)
		}
	}
	for _, r := range to {
		o, ok := fm[r.Name]
		if !ok {
			continue
		}
		fields := map[string]diffFieldDelta{}
		if o.Label != r.Label {
			fields["label"] = strDelta(o.Label, r.Label)
		}
		if o.Definition != r.Definition {
			fields["definition"] = strDelta(o.Definition, r.Definition)
		}
		if o.From != r.From {
			fields["from"] = strDelta(o.From, r.From)
		}
		if o.To != r.To {
			fields["to"] = strDelta(o.To, r.To)
		}
		if len(fields) > 0 {
			out.Changed = append(out.Changed, diffChangedItem{Name: r.Name, Fields: fields})
		}
	}
	return out
}

func diffInstances(from, to []pkgspec.Instance) diffSet {
	out := newDiffSet()
	fm := make(map[string]pkgspec.Instance, len(from))
	for _, it := range from {
		fm[it.Name] = it
	}
	tm := make(map[string]pkgspec.Instance, len(to))
	for _, it := range to {
		tm[it.Name] = it
	}
	for _, it := range to {
		if _, ok := fm[it.Name]; !ok {
			out.Added = append(out.Added, it)
		}
	}
	for _, it := range from {
		if _, ok := tm[it.Name]; !ok {
			out.Removed = append(out.Removed, it)
		}
	}
	for _, it := range to {
		o, ok := fm[it.Name]
		if !ok {
			continue
		}
		fields := map[string]diffFieldDelta{}
		if o.Concept != it.Concept {
			fields["concept"] = strDelta(o.Concept, it.Concept)
		}
		if !attrsEqual(o.Attributes, it.Attributes) {
			fields["attributes"] = attrsDelta(o.Attributes, it.Attributes)
		}
		if !instanceRelsEqual(o.Relations, it.Relations) {
			fields["relations"] = relsDelta(o.Relations, it.Relations)
		}
		if len(fields) > 0 {
			out.Changed = append(out.Changed, diffChangedItem{Name: it.Name, Fields: fields})
		}
	}
	return out
}

// conceptReferences 统计 TO 版本中被引用的概念名（relations.from/to + instances.concept + 其他概念 parents）。
func conceptReferences(to pkgspec.Spec) map[string]int {
	refs := map[string]int{}
	for _, r := range to.Relations {
		refs[r.From]++
		refs[r.To]++
	}
	for _, it := range to.Instances {
		refs[it.Concept]++
	}
	for _, c := range to.Concepts {
		for _, p := range c.Parents {
			refs[p]++
		}
	}
	return refs
}

func parseVersionQuery(r *http.Request, key string) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// diffVersions GET /api/ontologies/{id}/diff?from={v1}&to={v2}：版本 spec 三集结构 diff（REQ-95）。
func (s *Server) diffVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetOntology(id); err != nil {
		writeErr(w, err)
		return
	}
	fromV, ok := parseVersionQuery(r, "from")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from 参数必须是正整数"})
		return
	}
	toV, ok := parseVersionQuery(r, "to")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to 参数必须是正整数"})
		return
	}
	vs, err := s.Store.ListVersions(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	known := make(map[int]bool, len(vs))
	for _, m := range vs {
		known[m.Version] = true
	}
	load := func(v int) (*pkgspec.Spec, bool) {
		if !known[v] {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("版本 %d 不存在", v)})
			return nil, false
		}
		raw, err := s.Store.GetVersionSpec(id, v)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("版本 %d 无 spec 快照", v)})
			} else {
				writeErr(w, err)
			}
			return nil, false
		}
		var sp pkgspec.Spec
		if err := json.Unmarshal([]byte(raw), &sp); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("版本 %d 的 spec 快照解析失败: %s", v, err.Error())})
			return nil, false
		}
		return &sp, true
	}
	fromSpec, ok := load(fromV)
	if !ok {
		return
	}
	toSpec, ok := load(toV)
	if !ok {
		return
	}

	d := buildSpecDiff(fromSpec, toSpec)

	writeJSON(w, http.StatusOK, map[string]any{
		"from_version": fromV,
		"to_version":   toV,
		"concepts":     d.Concepts,
		"relations":    d.Relations,
		"instances":    d.Instances,
		"impact":       d.Impact,
	})
}

func (s *Server) fork(w http.ResponseWriter, r *http.Request) {
	srcID := r.PathValue("id")
	src, err := s.Store.GetOntology(srcID)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req struct{ Name, Description string }
	if err := decodeJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON 解析失败: " + err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = src.Name + "（副本）"
	}
	desc := req.Description
	if strings.TrimSpace(desc) == "" {
		desc = src.Description
	}
	newOntoID := "onto_" + newID()
	if _, err := s.Store.CreateOntologyFork(newOntoID, name, desc, srcID); err != nil {
		writeErr(w, err)
		return
	}
	arts, err := s.Store.ListArtifactContents(srcID)
	if err != nil {
		writeErr(w, err)
		return
	}
	specJSON := ""
	for _, a := range arts {
		if a.Format == "spec_json" {
			specJSON = a.Content
			continue
		}
		if err := s.Store.PutArtifact(newOntoID, a.Format, a.Content, a.Normalized); err != nil {
			writeErr(w, err)
			return
		}
	}
	if specJSON == "" {
		// 回退：最新版本快照
		if vs, e := s.Store.ListVersions(srcID); e == nil && len(vs) > 0 {
			if raw, e2 := s.Store.GetVersionSpec(srcID, vs[len(vs)-1].Version); e2 == nil {
				specJSON = raw
			}
		}
	}
	if specJSON == "" {
		empty := pkgspec.Spec{Name: name, Description: desc}
		b, _ := json.Marshal(empty)
		specJSON = string(b)
	}
	if err := s.Store.PutArtifact(newOntoID, "spec_json", specJSON, true); err != nil {
		writeErr(w, err)
		return
	}
	_ = s.Store.SaveVersion(newOntoID, 1, specJSON, "", "")
	o, err := s.Store.GetOntology(newOntoID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}
