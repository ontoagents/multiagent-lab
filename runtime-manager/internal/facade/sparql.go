package facade

// REQ-151（D-O17，M20）：facade 第 5 工具 sparql_query——受控开放查询面。
// 只读边界：词法级校验（跳过字符串字面量/IRI/注释后）要求前导声明后首关键词
// 为 SELECT，且全文裸词不得命中 SPARQL 1.1 Update 变更关键字；单次查询超时
// 与结果行数上限防失控查询。翻译透视（REQ-94）随 exec 照常记录。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/xiaoyao/eino-multiagent-lab/pkg/sparqlguard"
	"github.com/mark3labs/mcp-go/server"
	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/store"
)

const (
	sparqlQueryTimeout = 10 * time.Second
	defaultQueryLimit  = 50
	maxQueryLimit      = 200
)

// platformAPIBase 平台 backend 基址（REQ-211 会话→所属 agent 解析；REQ-216 升级为
// 解析绑定本体——本地同机默认）。
func platformAPIBase() string {
	if v := os.Getenv("PLATFORM_API_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:8080"
}

// execAsk 执行 ASK 查询（REQ-366/T1：布尔判定入只读查询面），随 exec 同口径记翻译透视
// （ResultCount 记 0/1）。
func (f *Facade) execAsk(ctx context.Context, tool, ontologyID, profileID, endpoint, sparql string) (bool, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(sparql))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/sparql-query")
	req.Header.Set("Accept", "application/sparql-results+json")
	resp, err := f.HTTP.Do(req)
	var ask bool
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("SPARQL 执行失败: %s", resp.Status)
		} else {
			var res struct {
				Boolean bool `json:"boolean"`
			}
			if jerr := json.NewDecoder(resp.Body).Decode(&res); jerr != nil {
				err = jerr
			}
			ask = res.Boolean
		}
	}
	if f.TraceSparql {
		n := 0
		if ask {
			n = 1
		}
		tr := &store.Trace{Tool: tool, ProfileID: profileID, OntologyID: ontologyID,
			Sparql: sparql, TookMS: time.Since(start).Milliseconds(), ResultCount: n, Ok: err == nil}
		if err != nil {
			tr.Error = err.Error()
		}
		_ = f.Store.SaveTrace(tr)
	}
	return ask, err
}

// companionOntologyOf 会话/智能体 → 绑定伴生本体 id（REQ-216：伴生图=本体伴生子图，
// 归属解析收敛到平台单点 /api/companion/graph-owner；结果进程内缓存——agent 绑定关系
// 变更低频，缓存内 stale 影响仅限进程生命周期）。
func (f *Facade) companionOntologyOf(agentID, convID string) (string, error) {
	if strings.TrimSpace(agentID) == "" && strings.TrimSpace(convID) == "" {
		return "", fmt.Errorf("agent_id / conversation_id 至少传一")
	}
	q := "agent_id=" + url.QueryEscape(agentID)
	if strings.TrimSpace(agentID) == "" {
		q = "conversation_id=" + url.QueryEscape(convID)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(platformAPIBase() + "/api/companion/graph-owner?" + q)
	if err != nil {
		return "", fmt.Errorf("平台不可达（%v）", err)
	}
	defer resp.Body.Close()
	var body struct {
		AgentID    string `json:"agent_id"`
		OntologyID string `json:"ontology_id"`
		Error      string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s", body.Error)
	}
	if body.OntologyID == "" {
		return "", fmt.Errorf("该智能体未绑定伴生本体")
	}
	return body.OntologyID, nil
}

// firstQueryKind 前导 PREFIX/BASE 声明后的首关键词（SELECT/ASK 判别用；无=空串）。
// REQ-380/M176：词法面单源化 pkg/sparqlguard。
func firstQueryKind(query string) string { return sparqlguard.FirstQueryKind(query) }

func (f *Facade) addSparqlQueryTool(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("sparql_query",
		mcp.WithDescription("对本体或伴生图执行自定义只读 SPARQL 查询（开放问题查询面，如「哪些概念没有注释」）：SELECT 返回表格列与行，ASK 返回布尔判定。仅允许 SELECT/ASK，禁任何变更操作；SELECT 默认返回 50 行，可用 limit 调整（上限 200）。graph=companion 时查询指定智能体绑定本体的伴生子图（对话动态沉淀的知识，REQ-216 资产化后归属本体资产）。"),
		requiredString("ontology_id", "本体仓库 id（graph=companion 时可留空）"),
		mcp.WithString("query", mcp.Required(), mcp.Description("SPARQL SELECT 或 ASK 查询全文（可带 PREFIX 声明）")),
		mcp.WithNumber("limit", mcp.Description("返回行数上限（可选，默认 50，最大 200；仅 SELECT 生效）")),
		mcp.WithString("graph", mcp.Description("查询目标（可选）：default=本体运行方案默认图（缺省）；companion=伴生本体子图（对话生长的动态知识，挂本体伴生子图）")),
		mcp.WithString("agent_id", mcp.Description("智能体 id（graph=companion 时与 conversation_id 二选一：解析其绑定本体的伴生子图）")),
		mcp.WithString("conversation_id", mcp.Description("会话 id（graph=companion 时可选：解析所属智能体绑定的伴生子图；项目会话无唯一归属须改传 agent_id）")),
	), f.handleSparqlQuery())
}

func (f *Facade) handleSparqlQuery() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		oid, _ := args["ontology_id"].(string)
		q, _ := args["query"].(string)
		graph, _ := args["graph"].(string)
		convID, _ := args["conversation_id"].(string)
		agentID, _ := args["agent_id"].(string)
		if strings.TrimSpace(q) == "" {
			return mcp.NewToolResultErrorf("query 必填"), nil
		}
		if graph != "" && graph != "default" && graph != "companion" {
			return mcp.NewToolResultErrorf("graph 仅支持 default | companion，得到 %q", graph), nil
		}
		// REQ-216：伴生图查询——智能体绑定本体伴生子图（ont-{ontologyID}），宿主 = 含该本体
		// 的 running 方案引擎（route 同源复用；COMPANION_GRAPH_ENDPOINT/:9199 独立实例退役）。
		if graph == "companion" {
			return f.sparqlCompanion(ctx, oid, convID, agentID, q, args), nil
		}
		if oid == "" {
			return mcp.NewToolResultErrorf("ontology_id 必填（graph=default 时）"), nil
		}
		if err := vetReadonlySelect(q); err != nil {
			return mcp.NewToolResultErrorf("QUERY_REJECTED: %v", err), nil
		}
		p, ep, errRes := f.route(oid)
		if errRes != nil {
			return errRes, nil
		}
		limit := clampLimit(args["limit"])
		ctx, cancel := context.WithTimeout(ctx, sparqlQueryTimeout)
		defer cancel()
		// REQ-366/T1：ASK 布尔判定走引擎通道同封装（行数上限不适用；透视照记）
		if firstQueryKind(q) == "ASK" {
			ask, err := f.execAsk(ctx, "sparql_query", oid, p.ID, ep, q)
			if err != nil {
				return mcp.NewToolResultErrorf("查询失败: %v", err), nil
			}
			return jsonResult(map[string]any{"ask": ask, "query": q}), nil
		}
		bindings, err := f.exec(ctx, "sparql_query", oid, p.ID, ep, q)
		if err != nil {
			return mcp.NewToolResultErrorf("查询失败: %v", err), nil
		}
		cols, rows, truncated := toTable(bindings, limit)
		return jsonResult(map[string]any{
			"columns": cols, "rows": rows, "count": len(rows),
			"total": len(bindings), "truncated": truncated, "limit": limit,
		}), nil
	}
}

// sparqlCompanion 伴生图分支（REQ-216 资产化）：SELECT 白名单照旧 → 解析智能体绑定的
// 伴生本体（agent_id 直传优先；conversation_id 经平台 graph-owner 解析）→ 按本体路由到
// 宿主方案引擎（RunningByOntology 同 default 图同源）→ default-graph-uri=<ont-{id}> 限定
// 查询伴生子图（oxigraph SPARQL 协议标准参数）。宿主方案未 running 给可操作提示
// （读侧兜底拉起由平台 backend 承担，facade 只读）。
// 语义账目：ontology_id 入参（若有）仅作提示，路由以解析出的伴生本体为准；透视 ProfileID 记 companion:ont。
func (f *Facade) sparqlCompanion(ctx context.Context, oid, convID, agentID, q string, args map[string]any) *mcp.CallToolResult {
	ontID, errRes := f.companionOntology(agentID, convID)
	if errRes != nil {
		return errRes
	}
	if err := vetReadonlySelect(q); err != nil {
		return mcp.NewToolResultErrorf("QUERY_REJECTED: %v", err)
	}
	p, err := f.Store.RunningByOntology(ontID)
	if err != nil || p == nil {
		return mcp.NewToolResultErrorf("COMPANION_HOST_UNAVAILABLE: 本体 %s 未挂载到任何运行中的方案（伴生宿主方案未运行——backend 读路径会自动拉起，或到本体运行页手动启动）", ontID)
	}
	ep, err := f.Endpoint(p.ID)
	if err != nil {
		return mcp.NewToolResultErrorf("COMPANION_HOST_UNAVAILABLE: %v", err)
	}
	limit := clampLimit(args["limit"])
	ctx, cancel := context.WithTimeout(ctx, sparqlQueryTimeout)
	defer cancel()
	graphURI := fmt.Sprintf("http://eino-lab/graph/ont-%s", ontID)
	endpoint := ep + "?default-graph-uri=" + url.QueryEscape(graphURI)
	start := time.Now()
	// REQ-366/T1：伴生图 ASK 与 default 图同口径（布尔判定；透视 ProfileID 记 companion:ont）
	if firstQueryKind(q) == "ASK" {
		ask, aerr := f.execAsk(ctx, "sparql_query", ontID, "companion:"+ontID, endpoint, q)
		if aerr != nil {
			return mcp.NewToolResultErrorf("COMPANION_GRAPH_QUERY_FAILED: 伴生子图查询失败（%v）", aerr)
		}
		return jsonResult(map[string]any{"ask": ask, "query": q, "graph": graphURI})
	}
	bindings, err := f.query(ctx, endpoint, q)
	if f.TraceSparql {
		tr := &store.Trace{Tool: "sparql_query", ProfileID: "companion:" + ontID, OntologyID: ontID,
			Sparql: q, TookMS: time.Since(start).Milliseconds(), ResultCount: len(bindings), Ok: err == nil}
		if err != nil {
			tr.Error = err.Error()
		}
		_ = f.Store.SaveTrace(tr)
	}
	if err != nil {
		return mcp.NewToolResultErrorf("COMPANION_GRAPH_QUERY_FAILED: 伴生子图查询失败（%v）", err)
	}
	cols, rows, truncated := toTable(bindings, limit)
	return jsonResult(map[string]any{
		"columns": cols, "rows": rows, "count": len(rows),
		"total": len(bindings), "truncated": truncated, "limit": limit,
		"graph": graphURI,
	})
}

// companionOntology 解析伴生本体 id（agent_id 直传优先，conversation_id 经平台解析；
// 进程内缓存——绑定关系低频变更）。
func (f *Facade) companionOntology(agentID, convID string) (string, *mcp.CallToolResult) {
	direct := strings.TrimSpace(agentID)
	cacheKey := direct
	if cacheKey == "" {
		cacheKey = "conv:" + strings.TrimSpace(convID)
	}
	f.ownerMu.Lock()
	if e, ok := f.ownerCache[cacheKey]; ok && time.Since(e.at) < ownerCacheTTL {
		f.ownerMu.Unlock()
		return e.oid, nil
	}
	f.ownerMu.Unlock()
	oid, err := f.companionOntologyOf(agentID, convID)
	if err != nil {
		return "", mcp.NewToolResultErrorf("OWNER_RESOLVE_FAILED: %v", err)
	}
	f.ownerMu.Lock()
	f.ownerCache[cacheKey] = ownerEntry{oid: oid, at: time.Now()}
	f.ownerMu.Unlock()
	return oid, nil
}

// ValidateReadonlySelect 词法级只读校验的导出面（REQ-236⑥/M63：SPARQL 工作台代理端点
// 服务端兜底与 facade tools/call 同一权威口径；此前校验仅在前端拦截+facade MCP 面）。
func ValidateReadonlySelect(query string) error { return vetReadonlySelect(query) }

// vetReadonlySelect 内部调用面（薄包装，REQ-380/M176 单源化 pkg/sparqlguard——
// 本包原实现为四处副本之一；统一 SELECT|ASK 最宽只读口径，REQ-366/T1 语义不变）。
func vetReadonlySelect(query string) error { return sparqlguard.Vet(query) }


// clampLimit 行数上限：缺省/非正数回默认值，超 maxQueryLimit 截到上限。
func clampLimit(v any) int {
	n := defaultQueryLimit
	if f, ok := v.(float64); ok && f > 0 {
		n = int(f)
	}
	if n > maxQueryLimit {
		n = maxQueryLimit
	}
	return n
}

// toTable 把 SPARQL JSON bindings 投影为表格：列名字母序稳定，缺键补空串，
// 超出 limit 的行截断并置 truncated（诚实标注）。
func toTable(bindings []map[string]any, limit int) ([]string, []map[string]string, bool) {
	colSet := map[string]bool{}
	for _, b := range bindings {
		for k := range b {
			colSet[k] = true
		}
	}
	cols := make([]string, 0, len(colSet))
	for k := range colSet {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	truncated := len(bindings) > limit
	if truncated {
		bindings = bindings[:limit]
	}
	rows := make([]map[string]string, 0, len(bindings))
	for _, b := range bindings {
		row := make(map[string]string, len(cols))
		for _, c := range cols {
			if v, ok := b[c]; ok {
				row[c] = lit(v)
			} else {
				row[c] = ""
			}
		}
		rows = append(rows, row)
	}
	return cols, rows, truncated
}
