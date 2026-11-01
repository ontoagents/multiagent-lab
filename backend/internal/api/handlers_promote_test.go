package api

// REQ-546/M353 learning proposal 暂存区 API 单测（全链桩——仿 REQ-216 夹具）：promote 改产
// 提案（勾选过滤）→清单→approve 重取 digest 取交集入库（spec PUT 捕获）→pending 守卫→
// 漂移 400→reject 留痕。运行平面/构建平面/引擎 SPARQL 均由 httptest 桩承载（零真实依赖）。

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/companion"
	"github.com/ontoagents/ontoagents/backend/internal/ontology"
	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// promoteStubs 全链桩状态（引擎图数据可变承载漂移用例）。
type promoteStubs struct {
	mu        sync.Mutex
	labels    []map[string]map[string]string // uri/label 绑定
	edges     []map[string]map[string]string // sLabel/relName/oLabel 绑定
	specJSON  string                         // 构建平面 spec GET 响应（目标本体）
	putBodies []string                       // spec PUT 捕获
}

func (st *promoteStubs) setGraph(labels, edges []map[string]map[string]string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.labels, st.edges = labels, edges
}

func (st *promoteStubs) setSpec(s string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.specJSON = s
}

func (st *promoteStubs) puts() []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return append([]string(nil), st.putBodies...)
}

func bind(k, v string) map[string]map[string]string {
	return map[string]map[string]string{k: {"value": v}}
}

// mergeBind 多键绑定合并（边绑定 sLabel/relName/oLabel 三键单行）。
func mergeBind(ms ...map[string]map[string]string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func newPromoteFixture(t *testing.T) (*Server, *promoteStubs) {
	t.Helper()
	t.Setenv("COMPANION_SNAPSHOT_DIR", t.TempDir()) // 快照/回灌探针不落 ./data
	stubs := &promoteStubs{specJSON: `{"name":"网络本体","concepts":[],"relations":[],"instances":[],"cq":[]}`}
	// 引擎桩：SPARQL /query 按语句形态分流（COUNT 探针/label 摘要/活跃边摘要）
	engineMux := http.NewServeMux()
	engineMux.HandleFunc("POST /query", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1<<16)
		n, _ := r.Body.Read(buf)
		q := string(buf[:n])
		stubs.mu.Lock()
		labels, edges := stubs.labels, stubs.edges
		stubs.mu.Unlock()
		w.Header().Set("Content-Type", "application/sparql-results+json")
		switch {
		case strings.Contains(q, "COUNT(*)"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{"bindings": []map[string]any{{"n": map[string]string{"value": "7"}}}}})
		case strings.Contains(q, "SELECT ?uri ?label"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{"bindings": labels}})
		case strings.Contains(q, "bot:Relation"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{"bindings": edges}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{"bindings": []any{}}})
		}
	})
	engineSrv := httptest.NewServer(engineMux)
	t.Cleanup(engineSrv.Close)
	enginePort := engineSrv.Listener.Addr().(*net.TCPAddr).Port
	// 运行+构建平面桩：running 宿主方案（端口指向引擎桩）+目标 spec GET/PUT 捕获
	planeMux := http.NewServeMux()
	planeMux.HandleFunc("GET /api/runtime-profiles", func(w http.ResponseWriter, r *http.Request) {
		// 运行平面 list 返回裸数组（rest.go list 直出 Store.List()——PlanEngines.listProfiles 同口径）
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id": "rp-1", "name": "伴生·网络", "engine": "oxigraph",
			"ontology_ids": []string{"onto_comp"}, "port": enginePort, "status": "running",
		}})
	})
	planeMux.HandleFunc("GET /api/ontologies/onto_main/spec", func(w http.ResponseWriter, r *http.Request) {
		stubs.mu.Lock()
		spec := stubs.specJSON
		stubs.mu.Unlock()
		_, _ = w.Write([]byte(spec))
	})
	planeMux.HandleFunc("PUT /api/ontologies/onto_main/spec", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1<<20)
		n, _ := r.Body.Read(buf)
		stubs.mu.Lock()
		stubs.putBodies = append(stubs.putBodies, string(buf[:n]))
		stubs.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	planeSrv := httptest.NewServer(planeMux)
	t.Cleanup(planeSrv.Close)
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	onto := &ontology.Service{BuildURL: planeSrv.URL, RuntimeURL: planeSrv.URL, DialTimeout: 2 * time.Second}
	s := &Server{
		Store: st, Box: box, Ontology: onto,
		Companion: companion.NewService(st, box, companion.NewPlanEngines(planeSrv.URL, planeSrv.URL)),
	}
	return s, stubs
}

func TestPromotionProposalFlow_Req546(t *testing.T) {
	s, stubs := newPromoteFixture(t)
	// 伴生图：3 实体 2 边（其中「隔离」边一端防火墙未被勾选——应被提案过滤）
	stubs.setGraph(
		[]map[string]map[string]string{
			mergeBind(bind("uri", "urn:e1"), bind("label", "交换机")),
			mergeBind(bind("uri", "urn:e2"), bind("label", "路由器")),
			mergeBind(bind("uri", "urn:e3"), bind("label", "防火墙")),
		},
		[]map[string]map[string]string{
			mergeBind(bind("sLabel", "交换机"), bind("relName", "级联"), bind("oLabel", "路由器")),
			mergeBind(bind("sLabel", "路由器"), bind("relName", "隔离"), bind("oLabel", "防火墙")),
		},
	)

	// ① promote（勾选 交换机+路由器）→ pending 提案：2 实体 1 边
	w := doReq216(s, s.promoteCompanion, "POST", "/api/companion/ontologies/onto_comp/promote",
		map[string]any{"target_ontology_id": "onto_main", "entity_ids": []string{"交换机", "路由器"}, "note": "对话沉淀网络子图"},
		map[string]string{"id": "onto_comp"})
	if w.Code != http.StatusOK {
		t.Fatalf("promote 应 200，got %d: %s", w.Code, w.Body.String())
	}
	var prop store.PromotionProposal
	_ = json.Unmarshal(w.Body.Bytes(), &prop)
	if prop.ID == "" || prop.Status != "pending" || prop.EntityCount != 2 || prop.RelationCount != 1 {
		t.Fatalf("提案失真: %+v", prop)
	}
	if len(prop.Payload.Relations) != 1 || prop.Payload.Relations[0][1] != "级联" {
		t.Fatalf("勾选过滤失真（隔离边应被滤）: %+v", prop.Payload.Relations)
	}
	if len(stubs.puts()) != 0 {
		t.Fatal("promote 不应直接 PUT spec（改产提案）")
	}

	// ② 清单：pending 命中
	w = doReq216(s, s.listPromotionProposals, "GET", "/api/companion/promotion-proposals?status=pending&source_ontology_id=onto_comp", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("清单应 200，got %d", w.Code)
	}
	var list []*store.PromotionProposal
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 || list[0].ID != prop.ID {
		t.Fatalf("清单失真: %v", list)
	}

	// ③ approve：重取 digest 取交集（图未漂移=全量命中）→ 去重合并 PUT → approved
	stubs.setSpec(`{"name":"网络本体","concepts":[{"name":"交换机"}],"relations":[],"instances":[],"cq":[]}`)
	w = doReq216(s, s.decidePromotionProposal, "POST", "/api/companion/promotion-proposals/"+prop.ID+"/decide",
		map[string]any{"action": "approve"}, map[string]string{"id": prop.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("approve 应 200，got %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Proposal  *store.PromotionProposal `json:"proposal"`
		Concepts  int                      `json:"promoted_concepts"`
		Relations int                      `json:"promoted_relations"`
		Skipped   int                      `json:"skipped_existing"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Proposal == nil || out.Proposal.Status != "approved" {
		t.Fatalf("approve 后提案态失真: %+v", out.Proposal)
	}
	// 交换机已在目标 spec（跳过）；路由器新增（+1）；级联边新增（+1）
	if out.Concepts != 1 || out.Relations != 1 || out.Skipped != 1 {
		t.Fatalf("入库计数失真: +c%d +r%d skip%d", out.Concepts, out.Relations, out.Skipped)
	}
	puts := stubs.puts()
	if len(puts) != 1 {
		t.Fatalf("approve 应恰好一次 spec PUT: %d", len(puts))
	}
	var spec struct {
		Concepts []map[string]any `json:"concepts"`
	}
	_ = json.Unmarshal([]byte(puts[0]), &spec)
	names := map[string]bool{}
	for _, c := range spec.Concepts {
		if n, ok := c["name"].(string); ok {
			names[n] = true
		}
	}
	if !names["交换机"] || !names["路由器"] || names["防火墙"] {
		t.Fatalf("合并后 spec 失真: %v", names)
	}

	// ④ pending 守卫：终态再裁 → 409
	w = doReq216(s, s.decidePromotionProposal, "POST", "/api/companion/promotion-proposals/"+prop.ID+"/decide",
		map[string]any{"action": "reject", "note": "改口"}, map[string]string{"id": prop.ID})
	if w.Code != http.StatusConflict {
		t.Fatalf("终态再裁应 409，got %d: %s", w.Code, w.Body.String())
	}

	// ⑤ 漂移防护：新提案后伴生图清空 → approve 400（提案保持 pending）
	w = doReq216(s, s.promoteCompanion, "POST", "/api/companion/ontologies/onto_comp/promote",
		map[string]any{"target_ontology_id": "onto_main"}, map[string]string{"id": "onto_comp"})
	if w.Code != http.StatusOK {
		t.Fatalf("二次 promote 应 200，got %d: %s", w.Code, w.Body.String())
	}
	var prop2 store.PromotionProposal
	_ = json.Unmarshal(w.Body.Bytes(), &prop2)
	stubs.setGraph(nil, nil)
	w = doReq216(s, s.decidePromotionProposal, "POST", "/api/companion/promotion-proposals/"+prop2.ID+"/decide",
		map[string]any{"action": "approve"}, map[string]string{"id": prop2.ID})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("漂移全空 approve 应 400，got %d: %s", w.Code, w.Body.String())
	}
	if pend, _ := s.Store.ListPromotionProposals("", "", "pending"); len(pend) != 1 || pend[0].ID != prop2.ID {
		t.Fatal("漂移拒绝后提案应保持 pending")
	}

	// ⑥ reject 留痕：拒绝带注记 → rejected + decided_note；rejected 清单可回看（审计半边）
	stubs.setGraph(
		[]map[string]map[string]string{mergeBind(bind("uri", "urn:e1"), bind("label", "交换机"))},
		[]map[string]map[string]string{})
	w = doReq216(s, s.promoteCompanion, "POST", "/api/companion/ontologies/onto_comp/promote",
		map[string]any{"target_ontology_id": "onto_main"}, map[string]string{"id": "onto_comp"})
	var prop3 store.PromotionProposal
	_ = json.Unmarshal(w.Body.Bytes(), &prop3)
	w = doReq216(s, s.decidePromotionProposal, "POST", "/api/companion/promotion-proposals/"+prop3.ID+"/decide",
		map[string]any{"action": "reject", "note": "与目标本体定位不符"}, map[string]string{"id": prop3.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("reject 应 200，got %d: %s", w.Code, w.Body.String())
	}
	var rj store.PromotionProposal
	_ = json.Unmarshal(w.Body.Bytes(), &rj)
	if rj.Status != "rejected" || rj.DecidedNote != "与目标本体定位不符" {
		t.Fatalf("拒绝留痕失真: %+v", rj)
	}
	w = doReq216(s, s.listPromotionProposals, "GET", "/api/companion/promotion-proposals?status=rejected", nil, nil)
	var rejected []*store.PromotionProposal
	_ = json.Unmarshal(w.Body.Bytes(), &rejected)
	if len(rejected) != 1 || rejected[0].ID != prop3.ID {
		t.Fatalf("rejected 清单失真: %v", rejected)
	}

	// ⑦ 负路径：缺 target 400 / 勾选全不在活跃集 400 / 未知提案 404 / 非法 action 400
	w = doReq216(s, s.promoteCompanion, "POST", "/api/companion/ontologies/onto_comp/promote",
		map[string]any{}, map[string]string{"id": "onto_comp"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺 target 应 400，got %d", w.Code)
	}
	w = doReq216(s, s.promoteCompanion, "POST", "/api/companion/ontologies/onto_comp/promote",
		map[string]any{"target_ontology_id": "onto_main", "entity_ids": []string{"不存在的实体"}}, map[string]string{"id": "onto_comp"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("勾选全不在活跃集应 400，got %d", w.Code)
	}
	w = doReq216(s, s.decidePromotionProposal, "POST", "/api/companion/promotion-proposals/nope/decide",
		map[string]any{"action": "approve"}, map[string]string{"id": "nope"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("未知提案 approve 应 404，got %d", w.Code)
	}
	w = doReq216(s, s.decidePromotionProposal, "POST", "/api/companion/promotion-proposals/nope/decide",
		map[string]any{"action": "explode"}, map[string]string{"id": "nope"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 action 应 400，got %d", w.Code)
	}
}
