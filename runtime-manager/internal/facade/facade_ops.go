package facade

// REQ-366/M164 MCP 工具化本体操作——facade 工具面扩展 6→19（96 号规划档主线 A·O-1）。
//
// 五档中的 T2/T3/T5 落本文件（T1=sparql_query 扩 ASK 见 sparql.go；T4 推理挂钩 P2 触发驱动）：
//   T2 spec CRUD（9）：spec_get + concept/relation/attribute/instance 各 upsert/delete——
//     全部走构建平面既有 PUT spec If-Match 版本门禁与 Validate 校验管线，不经引擎直写
//     （版本快照/overlay 提取/入库评分随 PUT 免费获得）；工具内部自动「取版本→改 spec→
//     带 If-Match 提交」，400 校验明细（path: message 定位）与 409 版本冲突如实回喂不重试。
//   T3 校验封装（2）：spec_validate（结构+引用完整性）/ spec_quality_check（质量门禁三维分+findings）。
//   T5 CQ 评测联动（2）：cq_benchmark_datasets / cq_benchmark_run（202 job 工具内轮询，
//     沿 ontochat_turn 先例；超时回 job_id 续查）。
//
// 工具在本文件无条件注册（tools/list 全注册可枚举——外部 MCP 客户端同面；96 号 O-1 本意
// 「外部 Agent 可标准化操作本体」）。内部智能体的启用门控在平台 backend 装配层：13 件扩展
// 工具仅在智能体能力开关（agent.onto_ops）开启时并入装配清单，默认关（REQ-366 沿 REQ-360
// 能力开关先例）；写类 upsert/delete 在平台侧入 danger 档审批。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// buildAPIBase 构建平面基址（BUILD_SVC_URL，默认 http://127.0.0.1:8091）——与 runtimed
// main 及平台 backend 同 env 同缺省；单机同机互信（16 号 §11），无独立鉴权。
func buildAPIBase() string {
	if v := os.Getenv("BUILD_SVC_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:8091"
}

// specToolOutputCap spec_get 全文输出上限（字节）——超过回落「名称清单摘要」形态并诚实
// 标注（token 保护沿 REQ-354 export_ttl 截断先例）。
const specToolOutputCap = 48 * 1024

// addOpsTools REQ-366：T2/T3/T5 十三件扩展工具注册（无条件——MCP 标准面对外全量可枚举）。
func (f *Facade) addOpsTools(s *server.MCPServer) {
	oid := requiredString
	s.AddTool(mcp.NewTool("spec_get",
		mcp.WithDescription("读取本体资产当前 spec 全文与版本号（spec CRUD 的 If-Match 基准与只读查看面）。注意：本工具读构建平面资产（编辑态），查询运行引擎请用 get_concept/list_instances/sparql_query。超大 spec 自动回落名称清单摘要并标注 truncated。"),
		oid("ontology_id", "本体仓库 id"),
	), f.handleSpecGet())

	upsertCommon := "不存在则新增，已存在则更新（只更新传入的字段，未传字段保持原值）；保存走版本门禁与校验，失败回喂定位明细。"
	s.AddTool(mcp.NewTool("spec_concept_upsert",
		mcp.WithDescription("新增或更新本体中的概念（类）："+upsertCommon),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "概念名（唯一标识，如 计算节点）"),
		mcp.WithString("label", mcp.Description("显示名（可选；传入即覆盖，含空串）")),
		mcp.WithString("definition", mcp.Description("定义/描述（可选；传入即覆盖）")),
		mcp.WithString("parents", mcp.Description("父概念名列表（可选）：JSON 数组如 [\"网络设备\"]，或顿号/逗号分隔串；传入即整体覆盖，多继承传多个")),
	), f.handleConceptUpsert())

	s.AddTool(mcp.NewTool("spec_concept_delete",
		mcp.WithDescription("删除本体中的概念（类）。被其他元素引用（父概念/关系端点/实例类型等）时校验会拒绝并回喂悬空引用定位——先解除引用或一并删除。"),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "概念名"),
	), f.handleConceptDelete())

	s.AddTool(mcp.NewTool("spec_relation_upsert",
		mcp.WithDescription("新增或更新本体中的对象属性/关系（概念层，from/to 为概念名）："+upsertCommon),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "关系名（如 连接到）"),
		oid("from", "定义域概念名"),
		oid("to", "值域概念名"),
		mcp.WithString("label", mcp.Description("显示名（可选；传入即覆盖）")),
		mcp.WithString("definition", mcp.Description("定义/描述（可选；传入即覆盖）")),
		mcp.WithString("evidence", mcp.Description("证据分级（可选）：declared | name | values，空=未标注")),
	), f.handleRelationUpsert())

	s.AddTool(mcp.NewTool("spec_relation_delete",
		mcp.WithDescription("删除本体中的对象属性/关系。实例侧使用该关系的断言会随校验拒绝回喂定位。"),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "关系名"),
	), f.handleRelationDelete())

	s.AddTool(mcp.NewTool("spec_attribute_upsert",
		mcp.WithDescription("新增或更新本体中的数据属性（字面量属性的概念层声明，与实例 attributes 键同名关联）："+upsertCommon),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "数据属性名（如 重量）"),
		mcp.WithString("domain", mcp.Description("定义域概念名（可选=不限定义域；传入即覆盖）")),
		mcp.WithString("range", mcp.Description("数据类型短名：string/number/integer/boolean/date（可选，空=string）")),
		mcp.WithString("label", mcp.Description("显示名（可选；传入即覆盖）")),
		mcp.WithString("definition", mcp.Description("定义/描述（可选；传入即覆盖）")),
	), f.handleAttributeUpsert())

	s.AddTool(mcp.NewTool("spec_attribute_delete",
		mcp.WithDescription("删除本体中的数据属性声明（实例侧同名属性值不动）。"),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "数据属性名"),
	), f.handleAttributeDelete())

	s.AddTool(mcp.NewTool("spec_instance_upsert",
		mcp.WithDescription("新增或更新本体中的实例（个体）："+upsertCommon+"（实例整体覆盖语义：attributes/relations 传入即整体替换，未传保持原值）"),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "实例名"),
		oid("concept", "所属概念名"),
		mcp.WithString("attributes_json", mcp.Description("属性断言（可选）：JSON 对象如 {\"重量\":\"5kg\"}；传入即整体替换")),
		mcp.WithString("relations_json", mcp.Description("关系断言（可选）：JSON 数组如 [{\"rel\":\"连接到\",\"target\":\"端口1\"}]；传入即整体替换")),
	), f.handleInstanceUpsert())

	s.AddTool(mcp.NewTool("spec_instance_delete",
		mcp.WithDescription("删除本体中的实例（个体）及其属性/关系断言。"),
		oid("ontology_id", "本体仓库 id"),
		oid("name", "实例名"),
	), f.handleInstanceDelete())

	s.AddTool(mcp.NewTool("spec_validate",
		mcp.WithDescription("校验本体资产 spec（结构完整性+引用完整性），返回全部错误及定位（path 如 concepts[2].parents[0]）——保存前置自检或失败后定位用。"),
		oid("ontology_id", "本体仓库 id"),
	), f.handleSpecValidate())

	s.AddTool(mcp.NewTool("spec_quality_check",
		mcp.WithDescription("对本体资产跑质量门禁检查（完整性/一致性/可维护性三维评分+findings 明细，错误级命中在 strict 模式会阻断保存），返回完整质量报告。"),
		oid("ontology_id", "本体仓库 id"),
		mcp.WithBoolean("strict", mcp.Description("可选：临时按 strict 口径出报告（不影响资产上的 strict 开关）")),
	), f.handleSpecQualityCheck())

	s.AddTool(mcp.NewTool("cq_benchmark_datasets",
		mcp.WithDescription("列出可用的 CQ 基准集数据集（能力问题评测集，按种子域编写）——cq_benchmark_run 前探明可用数据集。"),
	), f.handleCQBenchDatasets())

	s.AddTool(mcp.NewTool("cq_benchmark_run",
		mcp.WithDescription("用 CQ 基准集「测我」：对指定本体逐条翻译基准能力问题为 SPARQL 并做只读白名单与 gold 对齐检查，返回三率（翻译/白名单/gold 对齐）报告并落质量档案历史。LLM 逐条翻译耗时约每条数秒；工具内等待至多 wait_seconds，超时回 job_id 可续查。同域本体高分=覆盖信号，跨域本体 gold 低为预期非缺陷。"),
		oid("ontology_id", "本体仓库 id"),
		oid("dataset", "基准集数据集名（cq_benchmark_datasets 列出）"),
		mcp.WithNumber("wait_seconds", mcp.Description("可选：工具内等待上限秒数（默认 300，上限 600）")),
		mcp.WithString("job_id", mcp.Description("可选：上次超时返回的 job_id——传则续查该任务不新发起")),
	), f.handleCQBenchRun())
}

// ---- 构建平面 HTTP 基元 ----

func (f *Facade) buildDo(ctx context.Context, method, path, contentType string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, buildAPIBase()+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("构建平面不可达（%s）: %v", buildAPIBase(), err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes(), nil
}

// fetchSpecAndVersion 取资产当前 spec 与版本号（If-Match 基准；两次读间的并发修改由
// PUT If-Match 409 兜底，不在此加锁）。
func (f *Facade) fetchSpecAndVersion(ctx context.Context, oid string) (*pkgspec.Spec, int, *mcp.CallToolResult) {
	code, body, err := f.buildDo(ctx, http.MethodGet, "/api/ontologies/"+url.PathEscape(oid), "", nil)
	if err != nil {
		return nil, 0, mcp.NewToolResultErrorf("SPEC_FETCH_FAILED: %v", err)
	}
	if code != http.StatusOK {
		return nil, 0, mcp.NewToolResultErrorf("SPEC_FETCH_FAILED: 本体 %s 不存在或构建平面返回 %d（%s）", oid, code, strings.TrimSpace(string(body)))
	}
	var detail struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, 0, mcp.NewToolResultErrorf("SPEC_FETCH_FAILED: 资产详情解析失败: %v", err)
	}
	code, body, err = f.buildDo(ctx, http.MethodGet, "/api/ontologies/"+url.PathEscape(oid)+"/spec", "", nil)
	if err != nil {
		return nil, 0, mcp.NewToolResultErrorf("SPEC_FETCH_FAILED: %v", err)
	}
	if code != http.StatusOK {
		return nil, 0, mcp.NewToolResultErrorf("SPEC_FETCH_FAILED: spec 读取返回 %d", code)
	}
	sp := &pkgspec.Spec{}
	if err := json.Unmarshal(body, sp); err != nil {
		return nil, 0, mcp.NewToolResultErrorf("SPEC_FETCH_FAILED: spec_json 解析失败: %v", err)
	}
	return sp, detail.Version, nil
}

// elementsDo REQ-436/M232②：元素级操作透传——编排（GET spec→改→PUT If-Match+校验）已下沉
// 构建平面一等 REST（/api/ontologies/{id}/elements/{kind}[/name]，persistSpec 单源），本面只保留
// AX 适配（错误文案映射/包络透传）。200 包络直通；400 校验未过/409 版本冲突如实回喂（文案与
// 原客户端编排一致，工具输出零变化）。
func (f *Facade) elementsDo(ctx context.Context, method, oid, kind, name string, payload any) *mcp.CallToolResult {
	path := "/api/ontologies/" + url.PathEscape(oid) + "/elements/" + kind
	if name != "" {
		path += "/" + url.PathEscape(name)
	}
	var body io.Reader
	if payload != nil {
		bts, err := json.Marshal(payload)
		if err != nil {
			return mcp.NewToolResultErrorf("ELEMENT_MARSHAL_FAILED: %v", err)
		}
		body = bytes.NewReader(bts)
	}
	req, err := http.NewRequestWithContext(ctx, method, buildAPIBase()+path, body)
	if err != nil {
		return mcp.NewToolResultErrorf("ELEMENT_CALL_FAILED: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return mcp.NewToolResultErrorf("ELEMENT_CALL_FAILED: 构建平面不可达（%s）: %v", buildAPIBase(), err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	switch resp.StatusCode {
	case http.StatusOK:
		return jsonResult(json.RawMessage(buf.Bytes()))
	case http.StatusBadRequest:
		var e struct {
			Error   string                    `json:"error"`
			Details []pkgspec.ValidationError `json:"validation_errors"`
		}
		_ = json.Unmarshal(buf.Bytes(), &e)
		var b strings.Builder
		b.WriteString("SPEC_VALIDATION_REJECTED（校验未通过，未保存）: " + e.Error)
		for _, d := range e.Details {
			b.WriteString(fmt.Sprintf("\n- %s: %s", d.Path, d.Message))
		}
		if len(e.Details) == 0 {
			b.WriteString("\n（无结构化明细——按 error 文案排查）")
		}
		return mcp.NewToolResultErrorf("%s", b.String())
	case http.StatusConflict:
		var e struct {
			Error          string `json:"error"`
			CurrentVersion int    `json:"current_version"`
		}
		_ = json.Unmarshal(buf.Bytes(), &e)
		return mcp.NewToolResultErrorf("SPEC_VERSION_CONFLICT（版本冲突，未保存）: %s——重新发起本操作即可基于最新版本 v%d 重读再改", e.Error, e.CurrentVersion)
	default:
		return mcp.NewToolResultErrorf("ELEMENT_CALL_FAILED: 构建平面返回 %d（%s）", resp.StatusCode, strings.TrimSpace(buf.String()))
	}
}

// ---- 可选参量解析（缺席=保持原值；显式传入含空串=覆盖） ----

func argString(args map[string]any, key string) (string, bool) {
	v, ok := args[key].(string)
	return v, ok
}

// argNames 解析名称列表参（parents 等）：JSON 数组或顿号/逗号分隔串；空白项剔除。
func argNames(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}, nil
	}
	if strings.HasPrefix(raw, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(raw), &arr); err != nil {
			return nil, fmt.Errorf("名称列表须为 JSON 字符串数组（如 [\"A\",\"B\"]）: %v", err)
		}
		out := arr[:0]
		for _, n := range arr {
			if n = strings.TrimSpace(n); n != "" {
				out = append(out, n)
			}
		}
		return out, nil
	}
	split := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '、' || r == '，' })
	out := make([]string, 0, len(split))
	for _, n := range split {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out, nil
}

// argUpsertNames 可缺席版 argNames（缺席返回 ok=false 语义交给调用方保持原值）。
func argUpsertNames(args map[string]any, key string) ([]string, bool, error) {
	raw, present := argString(args, key)
	if !present {
		return nil, false, nil
	}
	names, err := argNames(raw)
	if err != nil {
		return nil, true, err
	}
	return names, true, nil
}

// ---- T2 handlers ----

func (f *Facade) handleSpecGet() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		if oid == "" {
			return mcp.NewToolResultErrorf("ontology_id 必填"), nil
		}
		sp, version, errRes := f.fetchSpecAndVersion(ctx, oid)
		if errRes != nil {
			return errRes, nil
		}
		full, merr := json.Marshal(map[string]any{
			"ontology_id": oid, "version": version, "name": sp.Name, "description": sp.Description,
			"counts": map[string]int{
				"concepts": len(sp.Concepts), "relations": len(sp.Relations),
				"data_properties": len(sp.DataProperties), "instances": len(sp.Instances),
				"axioms": len(sp.Axioms), "cq": len(sp.CQ),
			},
			"spec": sp,
		})
		if merr != nil {
			return mcp.NewToolResultErrorf("spec 序列化失败: %v", merr), nil
		}
		if len(full) <= specToolOutputCap {
			return mcp.NewToolResultText(string(full)), nil
		}
		// 超限回落名称清单摘要（诚实标注 truncated——全文走平台资产页/导出，token 保护）
		conceptNames := make([]string, 0, len(sp.Concepts))
		for _, c := range sp.Concepts {
			conceptNames = append(conceptNames, c.Name)
		}
		relationNames := make([]string, 0, len(sp.Relations))
		for _, r := range sp.Relations {
			relationNames = append(relationNames, r.Name)
		}
		attributeNames := make([]string, 0, len(sp.DataProperties))
		for _, d := range sp.DataProperties {
			attributeNames = append(attributeNames, d.Name)
		}
		instanceNames := make([]string, 0, len(sp.Instances))
		for _, i := range sp.Instances {
			instanceNames = append(instanceNames, i.Name)
		}
		summary, _ := json.Marshal(map[string]any{
			"ontology_id": oid, "version": version, "name": sp.Name,
			"counts": map[string]int{
				"concepts": len(sp.Concepts), "relations": len(sp.Relations),
				"data_properties": len(sp.DataProperties), "instances": len(sp.Instances),
				"axioms": len(sp.Axioms), "cq": len(sp.CQ),
			},
			"truncated":       true,
			"hint":            fmt.Sprintf("spec 全文约 %d 字节超过工具输出上限——此处仅列元素名称；增删改用 spec_*_upsert/delete 按名操作无需全文，查看全文请到平台本体资产页或导出", len(full)),
			"concept_names":   conceptNames,
			"relation_names":  relationNames,
			"attribute_names": attributeNames,
			"instance_names":  instanceNames,
		})
		return mcp.NewToolResultText(string(summary)), nil
	}
}

func (f *Facade) handleConceptUpsert() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		if oid == "" || strings.TrimSpace(name) == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		parents, parentsPresent, perr := argUpsertNames(args, "parents")
		if perr != nil {
			return mcp.NewToolResultErrorf("parents: %v", perr), nil
		}
		label, hasLabel := argString(args, "label")
		definition, hasDefinition := argString(args, "definition")
		// REQ-436②：编排下沉构建平面——透传元素级端点（语义服务端单源）
		payload := map[string]any{"name": name}
		if hasLabel {
			payload["label"] = label
		}
		if hasDefinition {
			payload["definition"] = definition
		}
		if parentsPresent {
			payload["parents"] = parents
		}
		return f.elementsDo(ctx, http.MethodPost, oid, "concept", "", payload), nil
	}
}

func (f *Facade) handleConceptDelete() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		if oid == "" || name == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		return f.elementsDo(ctx, http.MethodDelete, oid, "concept", name, nil), nil
	}
}

func (f *Facade) handleRelationUpsert() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		from, hasFrom := argString(args, "from")
		to, hasTo := argString(args, "to")
		if oid == "" || strings.TrimSpace(name) == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		label, hasLabel := argString(args, "label")
		definition, hasDefinition := argString(args, "definition")
		evidence, hasEvidence := argString(args, "evidence")
		if hasEvidence && evidence != "" && !pkgspec.ValidEvidence(evidence) {
			return mcp.NewToolResultErrorf("evidence 仅支持 declared | name | values（或留空=未标注）"), nil
		}
		// REQ-436②：编排下沉构建平面——透传元素级端点（presence 语义随载荷原样）
		payload := map[string]any{"name": name}
		if hasFrom {
			payload["from"] = from
		}
		if hasTo {
			payload["to"] = to
		}
		if hasLabel {
			payload["label"] = label
		}
		if hasDefinition {
			payload["definition"] = definition
		}
		if hasEvidence {
			payload["evidence"] = evidence
		}
		return f.elementsDo(ctx, http.MethodPost, oid, "relation", "", payload), nil
	}
}

func (f *Facade) handleRelationDelete() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		if oid == "" || name == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		return f.elementsDo(ctx, http.MethodDelete, oid, "relation", name, nil), nil
	}
}

func (f *Facade) handleAttributeUpsert() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		if oid == "" || strings.TrimSpace(name) == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		domain, hasDomain := argString(args, "domain")
		drange, hasRange := argString(args, "range")
		label, hasLabel := argString(args, "label")
		definition, hasDefinition := argString(args, "definition")
		// REQ-436②：编排下沉构建平面——透传元素级端点
		payload := map[string]any{"name": name}
		if hasDomain {
			payload["domain"] = domain
		}
		if hasRange {
			payload["range"] = drange
		}
		if hasLabel {
			payload["label"] = label
		}
		if hasDefinition {
			payload["definition"] = definition
		}
		return f.elementsDo(ctx, http.MethodPost, oid, "attribute", "", payload), nil
	}
}

func (f *Facade) handleAttributeDelete() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		if oid == "" || name == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		return f.elementsDo(ctx, http.MethodDelete, oid, "attribute", name, nil), nil
	}
}

func (f *Facade) handleInstanceUpsert() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		concept, hasConcept := argString(args, "concept")
		if oid == "" || strings.TrimSpace(name) == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		var attributes map[string]any
		hasAttributes := false
		if raw, present := argString(args, "attributes_json"); present {
			hasAttributes = true
			attributes = map[string]any{}
			if strings.TrimSpace(raw) != "" {
				if err := json.Unmarshal([]byte(raw), &attributes); err != nil {
					return mcp.NewToolResultErrorf("attributes_json 须为 JSON 对象（如 {\"重量\":\"5kg\"}）: %v", err), nil
				}
			}
		}
		var relations []pkgspec.InstanceRel
		hasRelations := false
		if raw, present := argString(args, "relations_json"); present {
			hasRelations = true
			relations = []pkgspec.InstanceRel{}
			if strings.TrimSpace(raw) != "" {
				if err := json.Unmarshal([]byte(raw), &relations); err != nil {
					return mcp.NewToolResultErrorf("relations_json 须为 JSON 数组（如 [{\"rel\":\"连接到\",\"target\":\"端口1\"}]）: %v", err), nil
				}
			}
		}
		// REQ-436②：编排下沉构建平面——透传元素级端点（attributes/relations 已解析为结构化载荷）
		payload := map[string]any{"name": name}
		if hasConcept {
			payload["concept"] = concept
		}
		if hasAttributes {
			payload["attributes"] = attributes
		}
		if hasRelations {
			payload["relations"] = relations
		}
		return f.elementsDo(ctx, http.MethodPost, oid, "instance", "", payload), nil
	}
}

func (f *Facade) handleInstanceDelete() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		name, _ := args["name"].(string)
		if oid == "" || name == "" {
			return mcp.NewToolResultErrorf("ontology_id 与 name 必填"), nil
		}
		return f.elementsDo(ctx, http.MethodDelete, oid, "instance", name, nil), nil
	}
}

// ---- T3 handlers ----

func (f *Facade) handleSpecValidate() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		if oid == "" {
			return mcp.NewToolResultErrorf("ontology_id 必填"), nil
		}
		code, body, err := f.buildDo(ctx, http.MethodPost, "/api/ontologies/"+url.PathEscape(oid)+"/validate", "application/json", nil)
		if err != nil {
			return mcp.NewToolResultErrorf("VALIDATE_FAILED: %v", err), nil
		}
		if code != http.StatusOK {
			return mcp.NewToolResultErrorf("VALIDATE_FAILED: 构建平面返回 %d（%s）", code, strings.TrimSpace(string(body))), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}

func (f *Facade) handleSpecQualityCheck() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		if oid == "" {
			return mcp.NewToolResultErrorf("ontology_id 必填"), nil
		}
		payload := map[string]any{"ontology_id": oid}
		if strict, ok := args["strict"].(bool); ok {
			payload["strict"] = strict
		}
		bts, _ := json.Marshal(payload)
		code, body, err := f.buildDo(ctx, http.MethodPost, "/api/ontology/quality/check", "application/json", bts)
		if err != nil {
			return mcp.NewToolResultErrorf("QUALITY_CHECK_FAILED: %v", err), nil
		}
		if code != http.StatusOK {
			return mcp.NewToolResultErrorf("QUALITY_CHECK_FAILED: 构建平面返回 %d（%s）", code, strings.TrimSpace(string(body))), nil
		}
		if len(body) > specToolOutputCap {
			// 超限回落计数摘要（findings 全量走平台质量卡，token 保护诚实标注）
			var rep struct {
				ErrorCount   int `json:"error_count"`
				WarningCount int `json:"warning_count"`
			}
			_ = json.Unmarshal(body, &rep)
			return jsonResult(map[string]any{
				"truncated": true, "error_count": rep.ErrorCount, "warning_count": rep.WarningCount,
				"hint": fmt.Sprintf("质量报告全文约 %d 字节超过工具输出上限——明细请到平台资产详情质量卡查看", len(body)),
			}), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}

// ---- T5 handlers ----

func (f *Facade) handleCQBenchDatasets() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		code, body, err := f.buildDo(ctx, http.MethodGet, "/api/ontology/cq-benchmark/datasets", "", nil)
		if err != nil {
			return mcp.NewToolResultErrorf("CQ_DATASETS_FAILED: %v", err), nil
		}
		if code != http.StatusOK {
			return mcp.NewToolResultErrorf("CQ_DATASETS_FAILED: 构建平面返回 %d（%s）", code, strings.TrimSpace(string(body))), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}

// cqRunWaitDefault / cqRunWaitMax 工具内等待预算（沿 ontochat_turn「202+工具内轮询、超时回
// job_id 续查」先例；cqbench 逐条 LLM 翻译，15 条实测约 1~3 分钟）。
const (
	cqRunWaitDefault = 300 * time.Second
	cqRunWaitMax     = 600 * time.Second
)

// cqRunPollStep 轮询步长（var 便于测试收紧）。
var cqRunPollStep = 1500 * time.Millisecond

func (f *Facade) handleCQBenchRun() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		dataset, _ := args["dataset"].(string)
		jobID, _ := args["job_id"].(string)
		if oid == "" {
			return mcp.NewToolResultErrorf("ontology_id 必填（续查 job_id 时可仅传 job_id）"), nil
		}
		wait := cqRunWaitDefault
		if v, ok := args["wait_seconds"].(float64); ok && v > 0 {
			wait = time.Duration(v) * time.Second
			if wait > cqRunWaitMax {
				wait = cqRunWaitMax
			}
		}
		if strings.TrimSpace(jobID) == "" {
			payload, _ := json.Marshal(map[string]string{"ontology_id": oid, "dataset": dataset})
			code, body, err := f.buildDo(ctx, http.MethodPost, "/api/ontology/cq-benchmark/run", "application/json", payload)
			if err != nil {
				return mcp.NewToolResultErrorf("CQ_RUN_FAILED: %v", err), nil
			}
			if code == http.StatusServiceUnavailable {
				return mcp.NewToolResultErrorf("CQ_RUN_FAILED: 构建平面模型通道未配置（LLM Creator 未装配）——评测需 LLM 翻译"), nil
			}
			if code != http.StatusAccepted {
				return mcp.NewToolResultErrorf("CQ_RUN_FAILED: 构建平面返回 %d（%s）", code, strings.TrimSpace(string(body))), nil
			}
			var out struct {
				JobID string `json:"job_id"`
				Total int    `json:"total"`
			}
			if err := json.Unmarshal(body, &out); err != nil || out.JobID == "" {
				return mcp.NewToolResultErrorf("CQ_RUN_FAILED: 202 响应解析失败（%s）", strings.TrimSpace(string(body))), nil
			}
			jobID = out.JobID
		}
		deadline := time.Now().Add(wait)
		for {
			code, body, err := f.buildDo(ctx, http.MethodGet, "/api/ai-draft-jobs/"+url.PathEscape(jobID), "", nil)
			if err != nil || code != http.StatusOK {
				return mcp.NewToolResultErrorf("CQ_RUN_POLL_FAILED: job %s 状态查询失败（code=%d err=%v；服务重启会丢进程内任务——请重新发起）", jobID, code, err), nil
			}
			var job struct {
				Status   string `json:"status"`
				Progress string `json:"progress"`
				Error    string `json:"error"`
			}
			_ = json.Unmarshal(body, &job)
			switch job.Status {
			case "done":
				return f.cqRunResult(ctx, oid, jobID, job.Progress)
			case "failed":
				return mcp.NewToolResultErrorf("CQ_RUN_JOB_FAILED: job %s 失败：%s", jobID, job.Error), nil
			}
			if time.Now().After(deadline) {
				return jsonResult(map[string]any{
					"status": "running", "job_id": jobID, "ontology_id": oid,
					"progress": job.Progress,
					"hint":     fmt.Sprintf("工具等待 %.0f 秒超时任务仍在跑——稍后用本工具传 job_id=\"%s\" 续查（报告也会落平台质量卡基准历史）", wait.Seconds(), jobID),
				}), nil
			}
			select {
			case <-ctx.Done():
				return mcp.NewToolResultErrorf("CQ_RUN_CANCELLED: 调用方取消（job %s 仍在后台执行，可用 job_id 续查）", jobID), nil
			case <-time.After(cqRunPollStep):
			}
		}
	}
}

// cqRunResult 任务完成后取报告历史最新一条（runs 追加式——最新即本次）随进度摘要回喂。
func (f *Facade) cqRunResult(ctx context.Context, oid, jobID, progress string) (*mcp.CallToolResult, error) {
	out := map[string]any{"status": "done", "job_id": jobID, "ontology_id": oid, "summary": progress}
	code, body, err := f.buildDo(ctx, http.MethodGet, "/api/ontologies/"+url.PathEscape(oid)+"/cq-benchmark-report", "", nil)
	if err == nil && code == http.StatusOK {
		var hist struct {
			Runs []json.RawMessage `json:"runs"`
		}
		if json.Unmarshal(body, &hist) == nil && len(hist.Runs) > 0 {
			var latest any
			if json.Unmarshal(hist.Runs[len(hist.Runs)-1], &latest) == nil {
				out["latest_report"] = latest
			}
		}
	}
	return jsonResult(out), nil
}
