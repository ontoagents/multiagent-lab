package companion

// REQ-389/M185 D2 图状态实验件（2026-10-29，「按优先级逐件领取待开发需求」批）：
// 项目运行状态（会话/todo 清单/委派）投影为伴生图独立命名图 urn:state:run 的三元组，
// SPARQL 只读查询运行状态——「别人用 SQL 查状态，我们用 SPARQL 查状态」（38 号差异化
// 战略的 Graph 层实验：从「最薄层」走向「用本体当状态机」）。
// 形态=可选实验双轨起步（01 §3.10 拍板）：不接主线 run 收尾、不做主线切换——显式 sync
// 端点触发全量投影（DROP GRAPH+INSERT 幂等重建），查询面 SELECT 白名单。
// 诚实边界：①状态图是投影快照非实时（sync 时点语义）；②宿主方案=伴生绑定本体的宿主
// （EnsureHost 拉起，引擎依赖与伴生链路同源）；③实验词表 urn:state:* 非正式本体资产。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// RunStateGraph 运行状态独立命名图（与伴生子图隔离——DROP/INSERT 只动本图）。
const RunStateGraph = "urn:state:run"

// RunStateReport 一次投影的报告（sync 端点回执）。
type RunStateReport struct {
	OntologyID    string `json:"ontology_id"`
	Graph         string `json:"graph"`
	Conversations int    `json:"conversations"`
	Todos         int    `json:"todos"`
	Delegates     int    `json:"delegates"`
	Triples       int    `json:"triples"`
	ElapsedMs     int64  `json:"elapsed_ms"`
}

// stateTTL 一段 Turtle（INSERT DATA 载荷）。
type stateTTL struct {
	lines []string
}

func (t *stateTTL) add(format string, args ...any) {
	t.lines = append(t.lines, fmt.Sprintf(format, args...))
}

func (t *stateTTL) String() string { return strings.Join(t.lines, "\n") }

// SyncRunState 全量投影项目运行状态到伴生图 urn:state:run（幂等：先 DROP 再 INSERT）。
// ontologyID=伴生宿主本体（显式传入——实验语义不猜绑定）。
func (s *Service) SyncRunState(ctx context.Context, projectID, ontologyID string) (*RunStateReport, error) {
	start := time.Now()
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(ontologyID) == "" {
		return nil, fmt.Errorf("project_id 与 ontology_id 均必填（实验语义显式指定宿主本体）")
	}
	convs, err := s.Store.ListConversations(store.ConversationFilter{Scope: "project", ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("列出项目会话失败: %w", err)
	}

	ttl := &stateTTL{lines: []string{statePrefixes()}}
	nConv, nTodo, nDel := 0, 0, 0
	for _, conv := range convs {
		nConv++
		convIRI := fmt.Sprintf("<urn:state:conv/%s>", conv.ID)
		ttl.add("%s a state:Conversation ; state:id %s ; state:title %s ; state:updatedAt %s .",
			convIRI, ttlLiteral(conv.ID), ttlLiteral(conv.Title), ttlLiteral(conv.UpdatedAt))
		// todo 清单投影（conversation.todo_json，REQ-202/M38 机器侧产物）
		if conv.TodoJSON != "" {
			var items []struct{ Title, Status string }
			if json.Unmarshal([]byte(conv.TodoJSON), &items) == nil {
				for i, it := range items {
					nTodo++
					iri := fmt.Sprintf("<urn:state:todo/%s/%d>", conv.ID, i)
					ttl.add("%s a state:Todo ; state:title %s ; state:status %s ; state:ofConv %s .",
						iri, ttlLiteral(it.Title), ttlLiteral(it.Status), convIRI)
				}
			}
		}
		// 委派状态投影（parent=本会话）
		if ds, err := s.Store.ListDelegatesByParent(conv.ID); err == nil {
			for _, d := range ds {
				nDel++
				iri := fmt.Sprintf("<urn:state:delegate/%s>", d.ID)
				ttl.add("%s a state:Delegate ; state:member %s ; state:status %s ; state:task %s ; state:ofConv %s .",
					iri, ttlLiteral(d.MemberName), ttlLiteral(d.Status), ttlLiteral(truncateStr(d.Task, 200)), convIRI)
			}
		}
	}
	if nConv == 0 {
		return &RunStateReport{OntologyID: ontologyID, Graph: RunStateGraph, ElapsedMs: time.Since(start).Milliseconds()}, nil
	}

	// 幂等重建：DROP GRAPH + INSERT DATA（宿主方案引擎两连发）
	if err := s.graphUpdate(ctx, ontologyID, fmt.Sprintf("DROP GRAPH <%s>", RunStateGraph)); err != nil {
		return nil, fmt.Errorf("清理状态图失败: %w", err)
	}
	insert := fmt.Sprintf("INSERT DATA { GRAPH <%s> {\n%s\n} }", RunStateGraph, ttl.String())
	if err := s.graphUpdate(ctx, ontologyID, insert); err != nil {
		return nil, fmt.Errorf("写入状态图失败: %w", err)
	}
	return &RunStateReport{
		OntologyID: ontologyID, Graph: RunStateGraph,
		Conversations: nConv, Todos: nTodo, Delegates: nDel,
		Triples: len(ttl.lines) - 1, // 首行为 prefixes 不计
		ElapsedMs: time.Since(start).Milliseconds(),
	}, nil
}

// QueryRunState 运行状态 SPARQL 只读查询（SELECT/ASK 白名单；默认查 urn:state:run 图内——
// 查询未带 GRAPH 子句时服务端 union 语义下可见伴生子图，实验面不做查询改写，透明直传）。
func (s *Service) QueryRunState(ctx context.Context, ontologyID, sparql string) ([]byte, error) {
	q := strings.TrimSpace(sparql)
	if q == "" {
		return nil, fmt.Errorf("SPARQL 查询为空")
	}
	// 只读白名单（词法级轻量复刻——backend 与 pkg/sparqlguard 跨 Go module 不互引，D-O26）
	if kind := firstQueryKind(q); kind != "select" && kind != "ask" {
		return nil, fmt.Errorf("状态查询面仅支持 SELECT/ASK（实测 %s）——变更操作不开放", kind)
	}
	return s.graphQuery(ctx, ontologyID, q)
}

// statePrefixes 实验词表前缀（urn:state:*，非正式本体资产——诚实标注）。
func statePrefixes() string {
	return "@prefix state: <urn:state:> ."
}

// ttlLiteral Turtle 字面量转义（反斜杠/引号/换行/回车/制表）。
func ttlLiteral(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`,
	)
	return `"` + r.Replace(s) + `"`
}

// firstQueryKind 跳过注释与 PREFIX/BASE 后取首关键词（词法级轻量判定）。
func firstQueryKind(q string) string {
	stripped := q
	for {
		stripped = strings.TrimSpace(stripped)
		switch {
		case strings.HasPrefix(stripped, "#"):
			if i := strings.IndexByte(stripped, '\n'); i >= 0 {
				stripped = stripped[i+1:]
				continue
			}
			return ""
		case strings.HasPrefix(strings.ToUpper(stripped), "PREFIX "), strings.HasPrefix(strings.ToUpper(stripped), "BASE "):
			if i := strings.IndexByte(stripped, '\n'); i >= 0 {
				stripped = stripped[i+1:]
				continue
			}
			return ""
		}
		break
	}
	for _, kw := range []string{"SELECT", "ASK", "CONSTRUCT", "DESCRIBE", "INSERT", "DELETE", "LOAD", "CLEAR", "CREATE", "DROP", "MOVE", "COPY", "ADD"} {
		if len(stripped) >= len(kw) && strings.EqualFold(stripped[:len(kw)], kw) {
			return strings.ToLower(kw)
		}
	}
	return ""
}

// RunStateSample 示例查询（端点空参引导用）。
var RunStateSample = strings.Join([]string{
	"PREFIX state: <urn:state:>",
	"SELECT ?title ?status WHERE { GRAPH <urn:state:run> { ?t a state:Todo ; state:title ?title ; state:status ?status } }",
}, "\n")

// sortedKeys 诊断辅助（保持投影确定性——map 遍历序不定时报告排序稳定）。
func sortedKeys(m map[string]int) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
