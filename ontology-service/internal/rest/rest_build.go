// rest_build.go 构建域端点（REQ-340 A1 自 rest.go 拆分，80 号 A1）：导入+merge 审查（REQ-157）/ai-draft 族/种子与引导/ontoextend/CSV 灌装（REQ-96）。
// 六段构建管线见 internal/buildflow（A2）；路由挂载统一在 rest.go Mount。
package rest

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/buildflow"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/importer"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/ontoextend"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/qualitygate"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/seed"
)

func (s *Server) ontoextendListODPs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"odps": ontoextend.List(), "count": len(ontoextend.List())})
}

// ontoextendDraft M-O14 P2②：按 ODP 生成扩展草稿片段（spec_json 形态；LOV 附加术语由前端并入后再走 merge/preview 审查）
func (s *Server) ontoextendDraft(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OdpID string `json:"odp_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	odp := ontoextend.Get(in.OdpID)
	if odp == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ODP 不存在: " + in.OdpID})
		return
	}
	// REQ-340 A2：管线段留痕（source→draft；ODP 片段为静态装配无抽取段；gate/审查/入库走通用 merge 端点 strict 门禁）
	_ = buildflow.Run(r.Context(), "ontoextend-draft",
		buildflow.Segment{Stage: buildflow.StageSource, Run: func(context.Context) (string, error) {
			return "ODP " + odp.Name + "（" + odp.Source + "）", nil
		}},
		buildflow.Segment{Stage: buildflow.StageDraft, Run: func(context.Context) (string, error) {
			return fmt.Sprintf("概念 %d · 关系 %d（静态片段装配）", len(odp.Spec.Concepts), len(odp.Spec.Relations)), nil
		}},
	)
	draft := *odp.Spec
	draft.Name = odp.Name + " 扩展片段（OntoExtend）"
	writeJSON(w, http.StatusOK, map[string]any{"odp": odp.ID, "name": odp.Name, "description": odp.Description, "draft_spec": &draft})
}

// vocabSearch REQ-171 P1 LOV 词表搜索薄层（GET /api/ontology/vocabularies/search?q=）。

func (s *Server) mergeIncoming(r *http.Request) (filename, content string, spec *pkgspec.Spec, strategy, prefix string, imp *importer.Report, baseVersion int, err error) {
	strategy = r.FormValue("strategy")
	prefix = r.FormValue("prefix")
	if bv := strings.TrimSpace(r.FormValue("base_version")); bv != "" {
		baseVersion, _ = strconv.Atoi(bv)
	}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		f, h, ferr := r.FormFile("file")
		if ferr != nil {
			err = fmt.Errorf("file 字段必填（multipart）")
			return
		}
		defer f.Close()
		b, _ := io.ReadAll(io.LimitReader(f, 64<<20))
		filename, content = h.Filename, string(b)
	} else {
		var body struct {
			Filename    string        `json:"filename"`
			Content     string        `json:"content"`
			Spec        *pkgspec.Spec `json:"spec"`
			Strategy    string        `json:"strategy"`
			Prefix      string        `json:"prefix"`
			BaseVersion int           `json:"base_version"`
		}
		if derr := decodeJSON(r, &body); derr != nil {
			err = derr
			return
		}
		filename, content = body.Filename, body.Content
		if body.Spec != nil {
			spec = body.Spec
		}
		if strategy == "" {
			strategy = body.Strategy
		}
		if prefix == "" {
			prefix = body.Prefix
		}
		if body.BaseVersion > 0 {
			baseVersion = body.BaseVersion
		}
	}
	if spec == nil {
		if strings.TrimSpace(content) == "" {
			err = fmt.Errorf("content 或 spec 必填其一")
			return
		}
		if filename == "" {
			filename = "incoming.md"
		}
		spec, imp, err = importer.Import(s.Sidecar, filename, content)
		if err != nil {
			return
		}
	}
	return
}

// mergePreview REQ-157：导入合并冲突预览（字段级冲突/新增/重命名/合并结果 spec）。
func (s *Server) mergePreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, _, spec, strategy, prefix, imp, _, err := s.mergeIncoming(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strategy == "" {
		strategy = importer.StrategyReplace
	}
	raw, _, gerr := s.Store.GetArtifact(id, "spec_json")
	if gerr != nil {
		writeErr(w, gerr)
		return
	}
	target := &pkgspec.Spec{}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "现行 spec_json 解析失败: " + err.Error()})
		return
	}
	pv, err := importer.BuildMerged(target, spec, strategy, prefix)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// M-O14 P2③ + REQ-235/H5：有损导入报告内嵌 preview（lossy 清单+warnings 呈现给审查方，
	// 支撑「补录→重跑对账」动线；此前 wrapper 形态与前端裸 MergePreview 消费错位，一并归一）
	pv.ImportReport = imp
	// REQ-293②：回预览时点版本号——apply 回传做三重防陈旧（任一不符 409 要求重新验证）
	if o, gerr := s.Store.GetOntology(id); gerr == nil {
		pv.BaseVersion = o.Version
	}
	writeJSON(w, http.StatusOK, pv)
}

// mergeApply REQ-157：按策略应用合并（结构校验 + strict 门禁 + 版本快照）。
func (s *Server) mergeApply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, _, spec, strategy, prefix, imp, baseVersion, err := s.mergeIncoming(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strategy == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "strategy 必填（replace | merge-overwrite | merge）"})
		return
	}
	// REQ-293②（67 号 B7）防陈旧：预览时点版本不符 → 409 要求重新预览（不静默应用过期合并）
	if !s.guardBaseVersion(w, id, baseVersion) {
		return
	}
	raw, _, gerr := s.Store.GetArtifact(id, "spec_json")
	if gerr != nil {
		writeErr(w, gerr)
		return
	}
	target := &pkgspec.Spec{}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "现行 spec_json 解析失败: " + err.Error()})
		return
	}
	pv, err := importer.BuildMerged(target, spec, strategy, prefix)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	pv.BaseVersion = baseVersion
	// REQ-292① 编辑三层：机器合并后重放 overlay（人工编辑不随重算蒸发；Foundry manualObjects 跨代保留口径）
	overlays, gerr := s.Store.ListOverlays(id)
	applied, skipped := 0, 0
	var notes []string
	machineJSON := map[string]string{}
	if gerr == nil && len(overlays) > 0 {
		machineJSON = specMachineJSON(pv.MergedSpec) // 重放前快照机器值——revert 目标（base）不能被人造值污染
		applied, skipped, notes = reapplyOverlays(pv.MergedSpec, overlays)
	}
	if errs := pv.MergedSpec.Validate(); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "合并结果结构校验未通过", "validation_errors": errs})
		return
	}
	// REQ-156：strict 门禁（本体级开关）——合并结果错误级命中阻断
	if o, gerr := s.Store.GetOntology(id); gerr == nil && o.QualityStrict {
		if rep := qualitygate.Check(pv.MergedSpec, nil); rep.ErrorCount > 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("strict 门禁拦截：合并结果存在 %d 处错误级质量命中", rep.ErrorCount), "quality_report": rep})
			return
		}
	}
	bts, _ := json.Marshal(pv.MergedSpec)
	if err := s.Store.PutArtifact(id, "spec_json", string(bts), true); err != nil {
		writeErr(w, err)
		return
	}
	// overlay base 刷新：revert 目标=重放前的机器产出值
	if len(machineJSON) > 0 {
		_, _ = s.Store.RefreshOverlayBase(id, machineJSON)
	}
	v, _ := s.Store.BumpVersion(id)
	_ = s.Store.SaveVersion(id, v, string(bts), "", "")
	s.recordIngestScore(id, v, pv.MergedSpec) // REQ-331/M131：入库评分随版本追加（失败不阻断）
	pv.ImportReport = imp                     // REQ-235/H5：应用后报告随结果透出（lossy 补录动线入口）
	writeJSON(w, http.StatusOK, map[string]any{"applied": true, "version": v, "preview": pv,
		"overlays_applied": applied, "overlays_skipped": skipped, "notes": notes})
}

// ---- REQ-255/M62 批次（60 号 H2+H3）：推理级检查档 + CQ→SPARQL 验收闭环 ----

// sidecarReasonOut sidecar reason 子命令输出（owlrl OWL 2 RL 闭包一致性）。
type sidecarReasonOut struct {
	Consistent *bool    `json:"consistent"` // nil=推理器内部错误（error 字段承载）
	Violations []string `json:"violations"`
	Error      string   `json:"error,omitempty"`
	Note       string   `json:"note"`
}

// reasonCheck 导出 TTL 内容经 sidecar reason 跑一致性检测（不落库，报告旁路）。

func importLimits() (bodyLimit int64, maxObjects int) {
	mb := 64
	if v := os.Getenv("IMPORT_MAX_BODY_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			mb = n
		}
	}
	objs := 50_000
	if v := os.Getenv("IMPORT_MAX_SPEC_OBJECTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			objs = n
		}
	}
	var limit int64 = -1
	if mb > 0 {
		limit = int64(mb) << 20
	}
	return limit, objs
}

// captureMinTriples REQ-328/A 哨兵阈值：余量账本达到该条数且 spec 投影全空时 400 拒绝
// （IMPORT_CAPTURE_MIN_TRIPLES，默认 10；<=0 关闸——全空小文件多为测试空壳，拒绝反而碍事）。
func captureMinTriples() int {
	if v := os.Getenv("IMPORT_CAPTURE_MIN_TRIPLES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 10
}

func importTooLarge(w http.ResponseWriter, bodyLimit int64) {
	writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
		"error": fmt.Sprintf("导入内容超过体积上限（%d MB）：请拆分或清洗后重试", bodyLimit>>20),
	})
}

func specObjectCount(sp *pkgspec.Spec) int {
	return len(sp.Concepts) + len(sp.Relations) + len(sp.Instances) + len(sp.DataProperties) + len(sp.Axioms)
}

func (s *Server) importOntology(w http.ResponseWriter, r *http.Request) {
	bodyLimit, maxObjects := importLimits()
	if bodyLimit > 0 {
		// Content-Length 预检：超限快速拒绝（省一次无效读体；chunked 无长度由 MaxBytesReader 兜底）
		if r.ContentLength > bodyLimit {
			importTooLarge(w, bodyLimit)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
	}
	var filename, content, name string
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			var mbErr *http.MaxBytesError
			if errors.As(err, &mbErr) && bodyLimit > 0 {
				importTooLarge(w, bodyLimit)
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "multipart 解析失败: " + err.Error()})
			return
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 file 字段"})
			return
		}
		defer f.Close()
		bts, err := io.ReadAll(f)
		if err != nil {
			var mbErr *http.MaxBytesError
			if errors.As(err, &mbErr) && bodyLimit > 0 {
				importTooLarge(w, bodyLimit)
				return
			}
			writeErr(w, err)
			return
		}
		filename, content = hdr.Filename, string(bts)
		name = strings.TrimSuffix(r.FormValue("name"), "")
	} else {
		var req struct{ Filename, Content, Name string }
		// 体积上限由开头 MaxBytesReader 统一承载（REQ-305 拒绝语义；历史 decodeJSON 是 64MB 静默截断）
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var mbErr *http.MaxBytesError
			if errors.As(err, &mbErr) && bodyLimit > 0 {
				importTooLarge(w, bodyLimit)
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要 multipart file 或 JSON {filename, content}"})
			return
		}
		if req.Content == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要 multipart file 或 JSON {filename, content}"})
			return
		}
		filename, content, name = req.Filename, req.Content, req.Name
	}
	if filename == "" {
		filename = "paste.ttl"
	}
	sp, rep, err := importer.Import(s.Sidecar, filename, content)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// REQ-328/A 捕获率哨兵：RDF 形态投影全空且余量账本非空 → 400 拒绝（防「静默全空空壳库」——
	// 77 号触发案例：fastq 纯 ABox 曾 0/0/0 零警告 201 建库）；人话成因+建议随行。
	// 阈值：余量 ≥ IMPORT_CAPTURE_MIN_TRIPLES（默认 10，<=0 关闸）。<50% 捕获率 warning 由 sidecar 报告自带。
	if rc := rep.Reconciliation; rc != nil && rc.Remainder >= captureMinTriples() &&
		len(sp.Concepts) == 0 && len(sp.Relations) == 0 && len(sp.DataProperties) == 0 && len(sp.Instances) == 0 {
		causes := rep.Warnings
		if len(causes) == 0 {
			causes = []string{"文件中没有可识别的本体结构声明（owl:Class/属性声明/实例标记均缺失）"}
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":          fmt.Sprintf("导入已拒绝：文件含 %d 条语义三元组，但未识别出任何可编辑本体结构（用法推断亦无法补齐）", rc.Remainder),
			"causes":         causes,
			"hint":           "常见成因：①非 RDF 文件被误判为本体格式 ②全部内容为不支持的形态（如复杂限制表达式无具名结构）——可先在 Protégé 等工具为类型/个体补显式声明后重试",
			"reconciliation": rc,
		})
		return
	}
	// 规模校验（REQ-305）：归一化 spec 五层元素总数上限——在 CreateOntology 之前拒绝，无脏数据
	if maxObjects > 0 && specObjectCount(sp) > maxObjects {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("导入规模超过上限（%d 个对象 > %d）：请拆分成本体或先行清洗合并后导入", specObjectCount(sp), maxObjects),
		})
		return
	}
	// 本体记录
	id := "onto_" + newID()
	if name == "" {
		name = sp.Name
	}
	if name == "" {
		name = strings.TrimSuffix(filename, extensionOf(filename))
	}
	if _, err := s.Store.CreateOntology(id, name, sp.Description); err != nil {
		writeErr(w, err)
		return
	}
	// ①原样存档（original 不可变）②归一化 spec_json
	origFormat := originalFormatOf(rep.Format)
	if origFormat != "" {
		_ = s.Store.PutArtifact(id, origFormat, content, false)
	}
	bts, _ := json.Marshal(sp)
	if err := s.Store.PutArtifact(id, "spec_json", string(bts), true); err != nil {
		writeErr(w, err)
		return
	}
	// REQ-318/M120：未投影公理透传存档（artifact axioms-raw）——77 号 §6 设计；
	// 导出 TTL 时拼接还原（三要素编辑无损 + 高级公理透传）；spec_json 结构零改动（LLM 契约零影响）
	if len(rep.Unprojected) > 0 {
		if ab, err := json.Marshal(map[string]any{"fragments": rep.Unprojected}); err == nil {
			_ = s.Store.PutArtifact(id, "axioms-raw", string(ab), false)
		}
	}
	// 导入即首次入库：CreateOntology 初始 version=1，此处不再 Bump（REQ-93 版本语义）
	_ = s.Store.SaveVersion(id, 1, string(bts), origFormat, content)
	o, _ := s.Store.GetOntology(id)
	writeJSON(w, http.StatusCreated, map[string]any{"ontology": o, "report": rep})
}

func extensionOf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}

func originalFormatOf(format string) string {
	switch format {
	case importer.FormatTurtle:
		return "turtle"
	case importer.FormatOWLRDF:
		return "owl_rdfxml"
	case importer.FormatCSV:
		return "csv"
	case importer.FormatGraphML:
		return "graphml"
	}
	return "" // spec_json 无 original
}

// exportOntology 导出：?format=turtle（spec 经 sidecar 导出并校验）；其他形态直接回原文。

func (s *Server) guide(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	o, err := s.Store.GetOntology(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	raw, _, err := s.Store.GetArtifact(id, "spec_json")
	if err != nil {
		writeErr(w, err)
		return
	}
	var sp pkgspec.Spec
	_ = json.Unmarshal([]byte(raw), &sp)

	var b strings.Builder
	// REQ-215：紧凑口径——只给身份+规模+工具用法，不枚举概念/关系全量清单。
	// 概念发现职责移交 facade 工具 list_concepts 运行时按需拉取（本体消费=工具查询，非预载 TBox 进系统提示词）。
	fmt.Fprintf(&b, "可用本体【%s】(ontology_id=%s, v%d)：%s\n", o.Name, o.ID, o.Version, o.Description)
	fmt.Fprintf(&b, "规模：概念 %d 个、关系 %d 个、实例 %d 个。\n", len(sp.Concepts), len(sp.Relations), len(sp.Instances))
	// REQ-357/M157：IRI 模板一行透出——attr: 数据属性断言约定此前仅 cq-sparql/NL2SPARQL 两处
	// UI prompt 可见，对话动线 agent 侧零可发现来源（facade 工具结果 nameFromURI 全还原裸名，
	// sparql_query 写数据属性查询只能靠猜）。与 NL2SPARQL prompt 同源文案；一行模板非概念
	// 枚举，紧凑口径不破（TestGuideCompactNoEnumeration 语义不变）。
	fmt.Fprintf(&b, "IRI 模板：概念=<urn:o:%s:concept:名称>，关系=<urn:o:%s:relation:名称>，实例=<urn:o:%s:instance:名称>，数据属性断言=<urn:o:%s:attr:名称>（sparql_query 按此构造 IRI；名称保持原文，URL 无需转义）。\n", o.ID, o.ID, o.ID, o.ID)
	b.WriteString("查询工具使用：先用 list_concepts 获取概念名清单（指引不再预列概念），再 get_concept/get_instance 按名称精确查，list_instances 列出某概念全部实例，neighbors 查实例关系邻居，sparql_query 可执行自定义只读 SPARQL SELECT 查询（开放性问题如「哪些概念没有任何注释」，入参 query + 可选 limit，仅允许 SELECT 禁变更操作；graph=companion 时传 agent_id（或 conversation_id 自动解析所属智能体）可查询该智能体的伴生本体图——对话中沉淀的动态知识，全部会话与项目协作共享（跨会话记忆查询，REQ-211））。所有工具入参 ontology_id 固定为 " + o.ID + "。")
	writeJSON(w, http.StatusOK, map[string]string{"ontology_id": o.ID, "guide": b.String()})
}

// ---- seed / AI 草稿 ----

func (s *Server) seedSample(w http.ResponseWriter, r *http.Request) {
	sp := seed.K8sOpsWithRelations()
	if _, err := s.Store.GetOntology(sp.ID); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"id": sp.ID, "seeded": false, "note": "示例已存在"})
		return
	}
	if _, err := s.Store.CreateOntology(sp.ID, sp.Name, sp.Description); err != nil {
		writeErr(w, err)
		return
	}
	bts, _ := json.Marshal(sp)
	if err := s.Store.PutArtifact(sp.ID, "spec_json", string(bts), true); err != nil {
		writeErr(w, err)
		return
	}
	o, _ := s.Store.GetOntology(sp.ID)
	writeJSON(w, http.StatusCreated, o)
}

// aiClarify AI 创建澄清轮（REQ-306 A1/M110）：生成前基于描述+已补材料输出「范围复述+五维缺口清单+行动引导」。
// 同步 RawChat（沿 OntoChat domain 归纳轮先例——单次调用秒~十秒级，生成才走异步 job）；回复不落库无副作用。
func (s *Server) aiClarify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description string   `json:"description"`
		Hints       []string `json:"hints"` // 逐轮澄清累积的补充材料
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Description) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description 必填"})
		return
	}
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM 辅助创建未配置（PLATFORM_URL）"})
		return
	}
	var b strings.Builder
	b.WriteString(llmcreate.ClarifyPrompt)
	b.WriteString("\n\n领域描述：\n" + strings.TrimSpace(req.Description))
	for i, h := range req.Hints {
		b.WriteString(fmt.Sprintf("\n\n已补充材料 %d：\n%s", i+1, h))
	}
	reply, _, err := s.LLM.RawChat(r.Context(), b.String())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"reply": reply})
}

func (s *Server) aiDraft(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM 辅助创建未配置（PLATFORM_URL）"})
		return
	}
	var req struct {
		Description         string   `json:"description"`
		ExtraHint           string   `json:"extra_hint"`
		CapabilityQuestions []string `json:"capability_questions"` // 可选：AI 创建的能力问题引导（§4.8.3）
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Description) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description 必填"})
		return
	}
	// REQ-248/G2：CQ 显式入参（DraftWithCQ 并入 prompt 且回写 spec.CQ 入资产）
	res, err := s.LLM.DraftWithCQ(r.Context(), req.Description, req.ExtraHint, req.CapabilityQuestions)
	if res == nil {
		writeErr(w, err)
		return
	}
	out := map[string]any{"spec": res.Spec, "rounds": res.Rounds}
	if res.Usage != nil {
		out["usage"] = res.Usage
	}
	// REQ-247/G4：草案质量报告透出（此前被丢弃——用户看不到质量分）
	if res.Quality != nil {
		out["quality"] = res.Quality
	}
	if err != nil {
		out["warning"] = err.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- utils ----

// listLearning GET /api/ontologies/seed-learning：列出内置学习示例。
func (s *Server) listLearning(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, seed.LearningExamples())
}

// seedLearning POST /api/ontologies/seed-learning {"key":"defects"}：灌装内置学习示例本体。
func (s *Server) seedLearning(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Key) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key 必填"})
		return
	}
	sp, err := seed.LoadLearningExample(strings.TrimSpace(req.Key))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if _, err := s.Store.GetOntology(sp.ID); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"id": sp.ID, "seeded": false, "note": "示例已存在"})
		return
	}
	if _, err := s.Store.CreateOntology(sp.ID, sp.Name, sp.Description); err != nil {
		writeErr(w, err)
		return
	}
	bts, _ := json.Marshal(sp)
	if err := s.Store.PutArtifact(sp.ID, "spec_json", string(bts), true); err != nil {
		writeErr(w, err)
		return
	}
	_ = s.Store.SaveVersion(sp.ID, 1, string(bts), "", "")
	o, _ := s.Store.GetOntology(sp.ID)
	writeJSON(w, http.StatusCreated, o)
}

// listVersions GET /api/ontologies/{id}/versions：版本历史列表（REQ-93）。

// ---- REQ-96 数据灌装（CSV → 实例） ----

// csvList 兼容 JSON 数组与逗号分隔字符串两种列清单写法。
type csvList []string

func (l *csvList) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*l = splitCSVList(s)
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	*l = csvList(arr)
	return nil
}

func splitCSVList(s string) csvList {
	out := csvList{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ingestConfig 灌装配置（REQ-96）：P2a 同名映射 + P2b 映射向导增量。
// TypeRules 列名→转换类型（int/number/date/bool，缺省 string 原样）；
// MultiValueSep 关系列多值分隔符（空 = 整格单值，保持 P2a 语义）。
type ingestConfig struct {
	Concept          string            `json:"concept"`
	KeyColumn        string            `json:"key_column"`
	RelationColumns  csvList           `json:"relation_columns"`
	AttributeColumns csvList           `json:"attribute_columns"`
	SkipRows         int               `json:"skip_rows"`
	TypeRules        map[string]string `json:"type_rules,omitempty"`
	MultiValueSep    string            `json:"multi_value_sep,omitempty"`
	// SkipInstances REQ-294①（67 号 B2）：疑似活动流时不物化实例（仅出报告）——灌装取舍的自动建议承接
	SkipInstances bool `json:"skip_instances,omitempty"`
}

// ingestNote REQ-295②（67 号 A3）：人话 notes 四类——merged/unmatched/case/unlinked，每条带具体数字。
type ingestNote = importer.MergeNote

// convertCell P2b 类型转换：合法返回转换值，非法返回 error（由调用方记 warning 并降级为原字符串）。
func convertCell(col, v string, rules map[string]string) (any, error) {
	switch rules[col] {
	case "int":
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("不是整数")
		}
		return n, nil
	case "number":
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("不是数值")
		}
		return f, nil
	case "bool":
		switch strings.ToLower(v) {
		case "true", "1", "yes", "y", "是":
			return true, nil
		case "false", "0", "no", "n", "否":
			return false, nil
		}
		return nil, fmt.Errorf("不是布尔值")
	case "date":
		for _, layout := range []string{"2006-01-02", "2006/01/02", "2006-01-02 15:04:05", "20060102"} {
			if t, err := time.Parse(layout, v); err == nil {
				return t.Format("2006-01-02"), nil
			}
		}
		return nil, fmt.Errorf("不是日期（支持 2006-01-02 / 2006/01/02 / 20060102）")
	}
	return v, nil // string / 未配置：原样
}

// ingestStats 灌装报告（REQ-295② 扩展：大小写归一命中数 + 活动流判据提示）。
type ingestStats struct {
	RowsRead              int  `json:"rows_read"`
	InstancesGenerated    int  `json:"instances_generated"`
	SkippedEmptyKey       int  `json:"skipped_empty_key"`
	CaseInsensitive       int  `json:"case_insensitive_matches"` // REQ-292③（B6）：仅在忽略大小写时命中的去重数
	ActivityHint          bool `json:"activity_hint"`            // REQ-294①（B2）：疑似活动流
	InstancesMaterialized int  `json:"instances_materialized"`   // 实际物化数（skip_instances 时为 0）
}

// ingestDraft 预览条目：attributes/relations 恒为非空容器，保证响应形状稳定。
type ingestDraft struct {
	Name       string                `json:"name"`
	Concept    string                `json:"concept"`
	Attributes map[string]any        `json:"attributes"`
	Relations  []pkgspec.InstanceRel `json:"relations"`
}

// ingestCSV POST /api/ontologies/{id}/ingest-csv?mode=preview|apply：CSV → 实例草稿/入库（REQ-96 P2a）。
// REQ-292/293/294/295 批次：mode 兼容 form 字段（顺修前端漏传 ?mode=apply 落 preview 的真 bug）；
// base_version 防陈旧 409；B6 归一化键去重+case note；B9 属性列大小写冲突 _2 后缀；
// B2 活动流五条件判据+skip_instances；notes 四类人话报告；悬空关系目标降级 note 不阻断（A5）。
func (s *Server) ingestCSV(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = strings.TrimSpace(r.FormValue("mode")) // 兼容 form 字段（REQ-295 批次顺修：apply 不再静默落 preview）
	}
	if mode == "" {
		mode = "preview"
	}
	if mode != "preview" && mode != "apply" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode 只能是 preview 或 apply"})
		return
	}
	if _, err := s.Store.GetOntology(id); err != nil {
		writeErr(w, err)
		return
	}
	// REQ-293②（67 号 B7）防陈旧：预览时点版本不符 → 409（form 字段 base_version，缺省跳过）
	if bv := strings.TrimSpace(r.FormValue("base_version")); bv != "" {
		if n, perr := strconv.Atoi(bv); perr == nil && !s.guardBaseVersion(w, id, n) {
			return
		}
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要 multipart/form-data（csv 文件 + 配置字段）"})
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "multipart 解析失败: " + err.Error()})
		return
	}

	// 配置：优先 config JSON part，否则逐字段 form value。
	var cfg ingestConfig
	cfgRaw := strings.TrimSpace(r.FormValue("config"))
	if cfgRaw == "" && r.MultipartForm != nil {
		if fhs := r.MultipartForm.File["config"]; len(fhs) > 0 {
			if cf, err := fhs[0].Open(); err == nil {
				if cb, err := io.ReadAll(io.LimitReader(cf, 1<<20)); err == nil {
					cfgRaw = strings.TrimSpace(string(cb))
				}
				_ = cf.Close()
			}
		}
	}
	if cfgRaw != "" {
		if err := json.Unmarshal([]byte(cfgRaw), &cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config JSON 解析失败: " + err.Error()})
			return
		}
	} else {
		cfg.Concept = strings.TrimSpace(r.FormValue("concept"))
		cfg.KeyColumn = strings.TrimSpace(r.FormValue("key_column"))
		cfg.RelationColumns = splitCSVList(r.FormValue("relation_columns"))
		cfg.AttributeColumns = splitCSVList(r.FormValue("attribute_columns"))
		if sr := strings.TrimSpace(r.FormValue("skip_rows")); sr != "" {
			n, err := strconv.Atoi(sr)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "skip_rows 必须是整数"})
				return
			}
			cfg.SkipRows = n
		}
	}
	cfg.Concept = strings.TrimSpace(cfg.Concept)
	cfg.KeyColumn = strings.TrimSpace(cfg.KeyColumn)
	if cfg.Concept == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "concept 必填"})
		return
	}
	if cfg.KeyColumn == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key_column 必填"})
		return
	}
	if cfg.SkipRows < 0 {
		cfg.SkipRows = 0
	}

	// CSV 文件（字段名 csv，兼容 file）。
	f, _, err := r.FormFile("csv")
	if err != nil {
		f, _, err = r.FormFile("file")
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 csv 文件字段"})
		return
	}
	defer f.Close()
	bts, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		writeErr(w, err)
		return
	}
	cr := csv.NewReader(strings.NewReader(string(bts)))
	cr.FieldsPerRecord = -1 // 容忍参差不齐的行
	cr.LazyQuotes = true
	cr.TrimLeadingSpace = true
	records, err := cr.ReadAll()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CSV 解析失败: " + err.Error()})
		return
	}
	if cfg.SkipRows >= len(records) {
		records = nil
	} else {
		records = records[cfg.SkipRows:]
	}
	if len(records) < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CSV 无表头/数据行"})
		return
	}
	header, data := records[0], records[1:]

	colIdx := map[string]int{}
	for i, h := range header {
		h = strings.TrimSpace(h)
		if i == 0 {
			h = strings.TrimPrefix(h, "\ufeff") // 去 BOM
		}
		if h != "" {
			if _, dup := colIdx[h]; !dup {
				colIdx[h] = i
			}
		}
	}
	keyIdx, ok := colIdx[cfg.KeyColumn]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("主键列 %s 不存在", cfg.KeyColumn)})
		return
	}

	// 目标概念必须已存在（preview/apply 同样校验）。
	rawSpec, _, err := s.Store.GetArtifact(id, "spec_json")
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("概念 %s 不存在", cfg.Concept)})
			return
		}
		writeErr(w, err)
		return
	}
	var sp pkgspec.Spec
	if err := json.Unmarshal([]byte(rawSpec), &sp); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "spec 解析失败: " + err.Error()})
		return
	}
	conceptSet := map[string]bool{}
	for _, c := range sp.Concepts {
		conceptSet[c.Name] = true
	}
	if !conceptSet[cfg.Concept] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("概念 %s 不存在", cfg.Concept)})
		return
	}

	warnings := []string{}
	relCols := []string{}
	for _, c := range cfg.RelationColumns {
		if _, ok := colIdx[c]; !ok {
			warnings = append(warnings, fmt.Sprintf("关系列 %s 不存在，已忽略", c))
			continue
		}
		relCols = append(relCols, c)
	}
	attrCols := []string{}
	for _, c := range cfg.AttributeColumns {
		if _, ok := colIdx[c]; !ok {
			warnings = append(warnings, fmt.Sprintf("属性列 %s 不存在，已忽略", c))
			continue
		}
		attrCols = append(attrCols, c)
	}

	stats := ingestStats{}
	draft := []pkgspec.Instance{}
	seenKey := map[string]bool{}
	seenKeyNorm := map[string]string{} // REQ-292③（B6）：归一化键 lower(trim) → 首个原键（大小写仅在归一后命中也视为同一实例，显式记数）
	for ri, row := range data {
		rowNo := cfg.SkipRows + 2 + ri // 1-based（跳过行 + 表头）
		stats.RowsRead++
		cell := func(i int) string {
			if i >= 0 && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		name := cell(keyIdx)
		if name == "" {
			stats.SkippedEmptyKey++
			warnings = append(warnings, fmt.Sprintf("第 %d 行主键为空，已跳过", rowNo))
			continue
		}
		norm := strings.ToLower(name)
		if seenKey[name] {
			warnings = append(warnings, fmt.Sprintf("主键重复 %s（第 %d 行），已跳过", name, rowNo))
			continue
		}
		if first, hit := seenKeyNorm[norm]; hit {
			stats.CaseInsensitive++
			warnings = append(warnings, fmt.Sprintf("主键 %s（第 %d 行）与 %s 仅大小写不同，视为同一实例已跳过（REQ-292③ 归一化口径）", name, rowNo, first))
			continue
		}
		seenKey[name] = true
		seenKeyNorm[norm] = name
		inst := pkgspec.Instance{Name: name, Concept: cfg.Concept}
		attrKeyUsed := map[string]string{} // REQ-292④（B9）：属性键大小写不敏感去重——先到先得，冲突者 _2 后缀
		for _, c := range attrCols {
			if v := cell(colIdx[c]); v != "" {
				attrKey := c
				// REQ-292④（B9）：属性键大小写不敏感去重——先到先得，冲突列 _2 后缀（确定性消解）
				if first, clash := attrKeyUsed[strings.ToLower(c)]; clash && first != c {
					attrKey = fmt.Sprintf("%s_2", c)
					warnings = append(warnings, fmt.Sprintf("属性列 %s 与 %s 仅大小写不同，后者落为 %s（B9 先到先得）", c, first, attrKey))
				}
				attrKeyUsed[strings.ToLower(attrKey)] = attrKey
				cv, err := convertCell(c, v, cfg.TypeRules)
				if err != nil {
					cv = v // 降级：非法值按原字符串保留
					warnings = append(warnings, fmt.Sprintf("第 %d 行属性列 %s 值 %q %v，按原字符串保留", rowNo, c, v, err))
				}
				if inst.Attributes == nil {
					inst.Attributes = map[string]any{}
				}
				inst.Attributes[attrKey] = cv
			}
		}
		for _, c := range relCols {
			v := cell(colIdx[c])
			if v == "" {
				warnings = append(warnings, fmt.Sprintf("第 %d 行关系列 %s 目标为空，已跳过", rowNo, c))
				continue
			}
			// P2b 多值分隔符：一格多个目标（如 "D1;D2"）拆成多条断言。
			targets := []string{v}
			if cfg.MultiValueSep != "" {
				targets = nil
				for _, p := range strings.Split(v, cfg.MultiValueSep) {
					if p = strings.TrimSpace(p); p != "" {
						targets = append(targets, p)
					}
				}
				if len(targets) == 0 {
					warnings = append(warnings, fmt.Sprintf("第 %d 行关系列 %s 拆分后无有效目标，已跳过", rowNo, c))
					continue
				}
			}
			for _, t := range targets {
				inst.Relations = append(inst.Relations, pkgspec.InstanceRel{Rel: c, Target: t})
			}
		}
		draft = append(draft, inst)
		stats.InstancesGenerated++
	}

	// REQ-294①（67 号 B2）活动流五条件判据（单文件=单成员天然成立）：
	// 行数>2000 + 有时间列 + 概念名词命中事件词表 + 无关系列（只出不入）→ hint，可 skip_instances 不物化
	hasTimeCol := false
	for _, h := range header {
		hl := strings.ToLower(strings.TrimSpace(h))
		if hl == "" {
			continue
		}
		if strings.Contains(hl, "date") || strings.Contains(hl, "time") || strings.Contains(hl, "timestamp") ||
			strings.Contains(hl, "created") || strings.Contains(hl, "updated") || cfg.TypeRules[h] == "date" {
			hasTimeCol = true
			break
		}
	}
	eventNoun := false
	for _, w := range strings.FieldsFunc(strings.ToLower(cfg.Concept), func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
		switch w {
		case "event", "activity", "log", "track", "tracking", "click", "pageview", "visit", "session", "audit":
			eventNoun = true
		}
	}
	if stats.RowsRead > 2000 && hasTimeCol && eventNoun && len(relCols) == 0 {
		stats.ActivityHint = true
		warnings = append(warnings, fmt.Sprintf("疑似活动流：%d 行、有时间列、概念名含事件类词且无关系引用——大事件流建议不物化实例（勾选「跳过实例物化」或 config skip_instances，REQ-294①）", stats.RowsRead))
	}

	notes := []ingestNote{}
	if stats.SkippedEmptyKey > 0 {
		notes = append(notes, ingestNote{Kind: "unmatched", Text: fmt.Sprintf("%d 行主键为空未生成实例（unplaced 口径：退化但不停工，REQ-295④）", stats.SkippedEmptyKey)})
	}

	if mode == "preview" {
		limit := len(draft)
		if limit > 20 {
			limit = 20
		}
		preview := make([]ingestDraft, 0, limit)
		for _, it := range draft[:limit] {
			attrs := it.Attributes
			if attrs == nil {
				attrs = map[string]any{}
			}
			rels := it.Relations
			if rels == nil {
				rels = []pkgspec.InstanceRel{}
			}
			preview = append(preview, ingestDraft{Name: it.Name, Concept: it.Concept, Attributes: attrs, Relations: rels})
		}
		if o, gerr := s.Store.GetOntology(id); gerr == nil {
			writeJSON(w, http.StatusOK, map[string]any{"stats": stats, "warnings": warnings, "draft": preview, "notes": notes, "base_version": o.Version})
		} else {
			writeJSON(w, http.StatusOK, map[string]any{"stats": stats, "warnings": warnings, "draft": preview, "notes": notes})
		}
		return
	}

	// apply：合并进当前 spec → 校验 → 存为新版本。
	existing := map[string]bool{}
	existingBefore := map[string]bool{} // skip_instances 撤回时的「本批之前已有」基线
	existingNorm := map[string]string{} // B6：与既有实例的大小写归一对账
	for _, it := range sp.Instances {
		existing[it.Name] = true
		existingBefore[it.Name] = true
		existingNorm[strings.ToLower(it.Name)] = it.Name
	}
	materialized := 0
	for _, inst := range draft {
		if existing[inst.Name] {
			warnings = append(warnings, fmt.Sprintf("实例 %s 已存在，已跳过", inst.Name))
			continue
		}
		if first, hit := existingNorm[strings.ToLower(inst.Name)]; hit {
			stats.CaseInsensitive++
			warnings = append(warnings, fmt.Sprintf("实例 %s 与既有实例 %s 仅大小写不同，视为同一实例已跳过（REQ-292③）", inst.Name, first))
			continue
		}
		sp.Instances = append(sp.Instances, inst)
		existing[inst.Name] = true
		existingNorm[strings.ToLower(inst.Name)] = inst.Name
		materialized++
	}
	// REQ-295④（A5 退化不停工）：悬空关系目标（目标实例既不在库内也不在本批）降级为丢弃+显式注记，
	// 不再让整条灌装线卡死在结构校验（此前 400 硬失败）
	dangling := 0
	for i := range sp.Instances {
		kept := sp.Instances[i].Relations[:0]
		for _, ir := range sp.Instances[i].Relations {
			if _, ok := existing[ir.Target]; ok {
				kept = append(kept, ir)
			} else {
				dangling++
			}
		}
		sp.Instances[i].Relations = kept
		if len(kept) == 0 {
			sp.Instances[i].Relations = nil
		}
	}
	if dangling > 0 {
		notes = append(notes, ingestNote{Kind: "unmatched", Text: fmt.Sprintf("%d 条关系断言目标实例不存在，已丢弃（可先灌目标实例再重灌本文件补齐）", dangling)})
		warnings = append(warnings, fmt.Sprintf("%d 条关系断言因目标实例不存在被丢弃（REQ-295④ 降级注记）", dangling))
	}
	unlinked := 0
	for _, it := range sp.Instances {
		if len(it.Relations) == 0 {
			unlinked++
		}
	}
	if unlinked > 0 {
		notes = append(notes, ingestNote{Kind: "unlinked", Text: fmt.Sprintf("库内 %d 个实例无任何关系断言（孤立实例清单可在资产详情查看）", unlinked)})
	}
	if materialized > 0 {
		notes = append(notes, ingestNote{Kind: "merged", Text: fmt.Sprintf("本批物化实例 %d 个（读入 %d 行，空键跳过 %d）", materialized, stats.RowsRead, stats.SkippedEmptyKey)})
	}
	// B6 case note 在去重后汇算（批内碰撞与既有库碰撞两路都计入后再出 note）
	if stats.CaseInsensitive > 0 {
		notes = append(notes, ingestNote{Kind: "case", Text: fmt.Sprintf("%d 条主键仅在忽略大小写时视为同一实例（已按归一化口径去重并注记，REQ-292③）", stats.CaseInsensitive)})
	}
	if cfg.SkipInstances {
		if stats.ActivityHint {
			notes = append(notes, ingestNote{Kind: "unlinked", Text: fmt.Sprintf("活动流判定成立：%d 行未物化为实例（仅本报告承载计数，REQ-294①）", stats.RowsRead)})
		}
		materialized = 0
		drop := map[string]bool{}
		for _, inst := range draft {
			drop[inst.Name] = true
		}
		kept := make([]pkgspec.Instance, 0, len(sp.Instances))
		for _, it := range sp.Instances {
			if !drop[it.Name] || existingBefore[it.Name] {
				kept = append(kept, it)
			}
		}
		sp.Instances = kept
	}
	stats.InstancesMaterialized = materialized

	// REQ-292① 编辑三层：机器灌装后重放 overlay（人工编辑的实例属性/概念描述不随灌装蒸发）
	overlays, gerr := s.Store.ListOverlays(id)
	applied, skippedOv := 0, 0
	var overlayNotes []string
	machineJSON := map[string]string{}
	if gerr == nil && len(overlays) > 0 {
		machineJSON = specMachineJSON(&sp) // 重放前快照机器值——revert 目标不能被人造值污染
		applied, skippedOv, overlayNotes = reapplyOverlays(&sp, overlays)
		for _, n := range overlayNotes {
			notes = append(notes, ingestNote{Kind: "merged", Text: n})
		}
	}
	if errs := sp.Validate(); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "校验未通过，不允许保存坏本体", "validation_errors": errs})
		return
	}
	bts2, err := json.Marshal(sp)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Store.PutArtifact(id, "spec_json", string(bts2), true); err != nil {
		writeErr(w, err)
		return
	}
	if len(machineJSON) > 0 {
		_, _ = s.Store.RefreshOverlayBase(id, machineJSON)
	}
	v, _ := s.Store.BumpVersion(id)
	_ = s.Store.SaveVersion(id, v, string(bts2), "", "")
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "version": v, "stats": stats, "warnings": warnings,
		"notes": notes, "overlays_applied": applied, "overlays_skipped": skippedOv})
}

// ---- REQ-96 P2b 映射配置存取 ----

// getIngestMapping GET /api/ontologies/{id}/ingest-mapping：读取已保存的映射配置（无则 404）。
// 存 artifact format=ingest_mapping_json：不 bump version、不进产物列表（ListArtifacts 过滤）、不随 fork 复制。
func (s *Server) getIngestMapping(w http.ResponseWriter, r *http.Request) {
	raw, _, err := s.Store.GetArtifact(r.PathValue("id"), "ingest_mapping_json")
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(raw))
}

// putIngestMapping PUT /api/ontologies/{id}/ingest-mapping：保存映射配置（结构即 ingestConfig 的 JSON）。
func (s *Server) putIngestMapping(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetOntology(id); err != nil {
		writeErr(w, err)
		return
	}
	var cfg ingestConfig
	if err := decodeJSON(r, &cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON 解析失败: " + err.Error()})
		return
	}
	if strings.TrimSpace(cfg.Concept) == "" || strings.TrimSpace(cfg.KeyColumn) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "concept 与 key_column 必填"})
		return
	}
	bts, err := json.Marshal(cfg)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Store.PutArtifact(id, "ingest_mapping_json", string(bts), true); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

// ---- REQ-83 fork ----

// fork POST /api/ontologies/{id}/fork：复制 spec 与全部形态资产派生新本体（REQ-83）。

// ---------------------------------------------------------------------------
// REQ-340 A2：构建管线段留痕聚合口——跨服务路径（kb-extract 在 backend ontobuild 编排）
// 把段边界 SegmentTrace 回传到构建平面统一出口（Tracer 单点）；REQ-328 对账持久化
// 只需 SetTracer 接管一处覆盖三路径。best-effort：调用方 fire-and-forget，本端点恒 204。
// ---------------------------------------------------------------------------

func (s *Server) buildflowTraces(w http.ResponseWriter, r *http.Request) {
	var in []buildflow.SegmentTrace
	if err := decodeJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON 解析失败: " + err.Error()})
		return
	}
	for _, t := range in {
		if t.Pipeline == "" || t.Stage == "" {
			continue
		}
		if t.StartedAt.IsZero() {
			t.StartedAt = time.Now().UTC()
		}
		buildflow.EmitAt(t)
	}
	w.WriteHeader(http.StatusNoContent)
}
