// REQ-354/M154 单测（tool 层）：OntoChat 工具组三件——httptest 桩构建平面 REST。
// 覆盖：start 引导卡开场/mode 校验、turn 同步 200 与 202+job 轮询两形态、
// analyze/test 动作、超时 job_id 续查、save 确认纪律与门禁明细透传、构建平面不可达诚实回执。
package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ontoStubServer 按「METHOD 路径 → 响应」表桩构建平面。
type ontoStub struct {
	mu      atomic.Pointer[map[string]func(w http.ResponseWriter, r *http.Request)]
	missing []string
}

func newOntoStub() *ontoStub {
	m := map[string]func(w http.ResponseWriter, r *http.Request){}
	s := &ontoStub{}
	s.mu.Store(&m)
	return s
}

func (s *ontoStub) handle(key string, fn func(w http.ResponseWriter, r *http.Request)) {
	m := *s.mu.Load()
	m[key] = fn
}

func (s *ontoStub) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		m := *s.mu.Load()
		if fn, ok := m[key]; ok {
			fn(w, r)
			return
		}
		s.missing = append(s.missing, key)
		http.Error(w, "stub 未登记: "+key, http.StatusNotFound)
	}))
}

const ontoSessionJSON = `{
  "id": "sess-1", "title": "缺陷管理", "stage": "story", "round": 0,
  "messages": [{"role": "assistant", "content": "第 1 问：这个本体服务的领域和典型用户是谁？"}],
  "context": {"cqs": ["CQ：哪些缺陷属于哪个迭代？"]}
}`

func TestOntoChatStart(t *testing.T) {
	stub := newOntoStub()
	var gotMode string
	stub.handle("POST /api/ontochat/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotMode = req["mode"]
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(ontoSessionJSON))
	})
	stub.handle("GET /api/ontochat/story-templates", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"label":"领域","text":"本本体服务的领域是 **[领域名称]**"},{"label":"角色","text":"典型用户是…"}]`))
	})
	srv := stub.server()
	defer srv.Close()
	t.Setenv("BUILD_SVC_URL", srv.URL)

	out, err := ontoStartFn(context.Background(), ontoStartIn{Topic: "缺陷管理"})
	if err != nil || out.Error != "" {
		t.Fatalf("start 失败: out=%+v err=%v", out, err)
	}
	if out.SessionID != "sess-1" || out.Stage != "story" || out.Mode != "guided" {
		t.Errorf("start 基本字段不符: %+v", out)
	}
	if !strings.Contains(out.Opening, "第 1 问") || !strings.Contains(out.Opening, "参考引导卡·领域") {
		t.Errorf("开场应含首问与首卡: %q", out.Opening)
	}
	if out.Hint == "" {
		t.Error("guided 模式应有纪律提示")
	}
	if gotMode != "guided" {
		t.Errorf("缺省 mode 应为 guided，收到 %q", gotMode)
	}

	// quick 模式：无开场、hint 指引直接给领域描述
	out2, _ := ontoStartFn(context.Background(), ontoStartIn{Topic: "缺陷管理", Mode: "quick"})
	if out2.Opening != "" || !strings.Contains(out2.Hint, "领域描述") {
		t.Errorf("quick 模式形态不符: %+v", out2)
	}

	// 参数校验
	if o, _ := ontoStartFn(context.Background(), ontoStartIn{}); o.Error == "" {
		t.Error("topic 空应报错")
	}
	if o, _ := ontoStartFn(context.Background(), ontoStartIn{Topic: "x", Mode: "bogus"}); o.Error == "" {
		t.Error("mode 非法应报错")
	}
}

func TestOntoChatTurnSyncAndAsync(t *testing.T) {
	// 恢复轮询节奏（后续超时用例会改）
	origPoll := ontoChatPollInterval
	defer func() { ontoChatPollInterval = origPoll }()
	ontoChatPollInterval = 5 * time.Millisecond

	turnResult := map[string]any{
		"reply": "已归纳本轮信息。", "stage": "domain", "round": 3,
		"quality": map[string]any{
			"score":        map[string]any{"overall": 88.5, "completeness": 90, "consistency": 87, "maintainability": 88},
			"error_count":  0, "warning_count": 2,
		},
		"draft": map[string]any{
			"name": "缺陷管理本体", "description": "管理软件缺陷的领域本体",
			"concepts": []any{map[string]any{"name": "缺陷"}}, "relations": []any{},
		},
	}
	resultJSON, _ := json.Marshal(turnResult)

	stub := newOntoStub()
	stub.handle("POST /api/ontochat/sessions/sess-1/turn", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["text"] == "" && req["feedback"] == "" {
			t.Error("turn 请求 text/feedback 不应同时为空")
		}
		if req["feedback"] != "" { // refine 轮走异步
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"job_id": "job-9"}`))
			return
		}
		_, _ = w.Write(append([]byte{}, resultJSON...))
	})
	stub.handle("GET /api/ontochat/jobs/job-9", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"job-9","status":"done","result":` + string(resultJSON) + `}`))
	})
	stub.handle("GET /api/ontochat/sessions/sess-1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(ontoSessionJSON))
	})
	srv := stub.server()
	defer srv.Close()
	t.Setenv("BUILD_SVC_URL", srv.URL)

	// 同步 200：reply/quality/draft/CQs 全解析
	out, err := ontoTurnFn(context.Background(), ontoTurnIn{SessionID: "sess-1", Message: "开发人员提缺陷"})
	if err != nil || out.Error != "" {
		t.Fatalf("turn 失败: %+v %v", out, err)
	}
	if out.Reply != "已归纳本轮信息。" || out.Stage != "domain" || out.Round != 3 {
		t.Errorf("turn 基本字段不符: %+v", out)
	}
	if out.Quality == nil || out.Quality.Overall != 88.5 || out.Quality.WarningCount != 2 {
		t.Errorf("质量摘要未解析: %+v", out.Quality)
	}
	if out.Draft == nil || out.Draft.Name != "缺陷管理本体" || out.Draft.Concepts != 1 {
		t.Errorf("草稿摘要未解析: %+v", out.Draft)
	}
	if len(out.CQs) != 1 || !strings.Contains(out.CQs[0], "迭代") {
		t.Errorf("CQ 清单未随会话透出: %v", out.CQs)
	}

	// 202+job 轮询至 done
	out2, _ := ontoTurnFn(context.Background(), ontoTurnIn{SessionID: "sess-1", Feedback: "补充实例维度"})
	if out2.Error != "" || out2.Reply != "已归纳本轮信息。" {
		t.Errorf("异步轮询结果不符: %+v", out2)
	}
	if out2.SessionID == "" {
		t.Error("异步路径应从 result.session 补出 session_id")
	}

	// 参数校验：chat 动作 message/feedback 双空
	if o, _ := ontoTurnFn(context.Background(), ontoTurnIn{SessionID: "sess-1"}); o.Error == "" {
		t.Error("chat 双空应报错")
	}
	if o, _ := ontoTurnFn(context.Background(), ontoTurnIn{SessionID: "sess-1", Action: "bogus"}); o.Error == "" {
		t.Error("action 非法应报错")
	}
}

func TestOntoChatTurnAnalyzeAndTest(t *testing.T) {
	origPoll := ontoChatPollInterval
	defer func() { ontoChatPollInterval = origPoll }()
	ontoChatPollInterval = 5 * time.Millisecond

	stub := newOntoStub()
	var running atomic.Bool
	running.Store(true)
	stub.handle("POST /api/ontochat/sessions/sess-1/cq-analyze", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req) // 空对象体
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"job_id": "job-a"}`))
	})
	stub.handle("POST /api/ontochat/sessions/sess-1/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"job_id": "job-t", "total": 3}`))
	})
	stub.handle("GET /api/ontochat/jobs/job-a", func(w http.ResponseWriter, _ *http.Request) {
		if running.Load() { // 先 running 一拍，验证轮询不止一次
			_, _ = w.Write([]byte(`{"id":"job-a","status":"running","progress":"分析中"}`))
			running.Store(false)
			return
		}
		_, _ = w.Write([]byte(`{"id":"job-a","status":"done","result":{"clusters":2,"dedup_count":1}}`))
	})
	stub.handle("GET /api/ontochat/jobs/job-t", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"job-t","status":"done","result":{"passed":2,"total":3,"pass_rate":0.667}}`))
	})
	stub.handle("GET /api/ontochat/sessions/sess-1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(ontoSessionJSON))
	})
	srv := stub.server()
	defer srv.Close()
	t.Setenv("BUILD_SVC_URL", srv.URL)

	out, _ := ontoTurnFn(context.Background(), ontoTurnIn{SessionID: "sess-1", Action: "analyze"})
	if out.Error != "" || out.Clusters != 2 || out.DedupCount != 1 {
		t.Errorf("analyze 结果不符: %+v", out)
	}
	if out.Reply == "" {
		t.Error("analyze 应从会话补拉最新 assistant 留痕作为 reply")
	}

	out2, _ := ontoTurnFn(context.Background(), ontoTurnIn{SessionID: "sess-1", Action: "test"})
	if out2.Error != "" || out2.TestReport == nil || out2.TestReport.Passed != 2 || out2.TestReport.Total != 3 {
		t.Errorf("test 结果不符: %+v", out2)
	}
}

func TestOntoChatTurnTimeoutJobIDResume(t *testing.T) {
	origPoll := ontoChatPollInterval
	defer func() { ontoChatPollInterval = origPoll }()
	ontoChatPollInterval = 5 * time.Millisecond

	stub := newOntoStub()
	stub.handle("GET /api/ontochat/jobs/job-stuck", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"job-stuck","status":"running","progress":"生成中"}`))
	})
	srv := stub.server()
	defer srv.Close()
	t.Setenv("BUILD_SVC_URL", srv.URL)

	// 直接以 1s 预算上下文轮询（生产路径由 ontoTurnFn 包预算，此处等价构造）
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := ontoPollJob(ctx, "job-stuck", "sess-1")
	if err != nil {
		t.Fatalf("poll err: %v", err)
	}
	if out.JobID != "job-stuck" || !strings.Contains(out.Error, "续查") {
		t.Errorf("超时应返回 job_id 与续查指引: %+v", out)
	}

	// 续查模式：job 转 done → 正常出结果
	stub.handle("GET /api/ontochat/jobs/job-stuck", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"job-stuck","status":"done","result":{"reply":"完成了","stage":"refine","round":2}}`))
	})
	out2, _ := ontoTurnFn(context.Background(), ontoTurnIn{SessionID: "sess-1", JobID: "job-stuck"})
	if out2.Error != "" || out2.Reply != "完成了" || out2.Stage != "refine" {
		t.Errorf("续查结果不符: %+v", out2)
	}

	// error 终态透传
	stub.handle("GET /api/ontochat/jobs/job-stuck", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"job-stuck","status":"error","error":"LLM 未配置"}`))
	})
	out3, _ := ontoTurnFn(context.Background(), ontoTurnIn{SessionID: "sess-1", JobID: "job-stuck"})
	if !strings.Contains(out3.Error, "LLM 未配置") {
		t.Errorf("error 终态应透传: %+v", out3)
	}
}

func TestOntoChatSave(t *testing.T) {
	stub := newOntoStub()
	var gotNote string
	stub.handle("POST /api/ontochat/sessions/sess-1/save", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotNote = req["name"]
		if req["name"] == "gate-fail" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"草稿校验未通过","validation_errors":[{"path":"concepts[0].name","message":"name 必填"}]}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ontology":{"id":"onto-1","name":"缺陷管理本体","description":"领域本体"},"session":{"id":"sess-1","stage":"done","round":5,"messages":[]}}`))
	})
	srv := stub.server()
	defer srv.Close()
	t.Setenv("BUILD_SVC_URL", srv.URL)

	// 确认纪律：confirm_note 必填
	out, _ := ontoSaveFn(context.Background(), ontoSaveIn{SessionID: "sess-1"})
	if out.Error == "" || !strings.Contains(out.Error, "confirm_note") {
		t.Errorf("confirm_note 缺失应拦截: %+v", out)
	}

	// 成功入库
	out2, _ := ontoSaveFn(context.Background(), ontoSaveIn{SessionID: "sess-1", Name: "缺陷管理本体", ConfirmNote: "用户已确认入库"})
	if out2.Error != "" || !out2.Saved || out2.AssetID != "onto-1" || out2.Stage != "done" {
		t.Errorf("save 成功形态不符: %+v", out2)
	}
	if gotNote != "缺陷管理本体" {
		t.Errorf("name 未透传: %q", gotNote)
	}

	// 门禁失败：明细逐条透传不静默
	out3, _ := ontoSaveFn(context.Background(), ontoSaveIn{SessionID: "sess-1", Name: "gate-fail", ConfirmNote: "已确认"})
	if out3.Error == "" || !strings.Contains(out3.Error, "逐条转述") || len(out3.Validations) != 1 {
		t.Errorf("门禁失败应带明细: %+v", out3)
	}
	if out3.Validations[0].Path != "concepts[0].name" {
		t.Errorf("validation 明细解析不符: %+v", out3.Validations)
	}
}

func TestOntoChatUnreachableHonest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	srv.Close() // 立即关闭：构造不可达地址
	t.Setenv("BUILD_SVC_URL", srv.URL)

	o1, _ := ontoStartFn(context.Background(), ontoStartIn{Topic: "x"})
	if !strings.Contains(o1.Error, "构建平面暂不可达") || !strings.Contains(o1.Error, "不得编造结果") {
		t.Errorf("不可达应诚实回执: %q", o1.Error)
	}
	o2, _ := ontoTurnFn(context.Background(), ontoTurnIn{SessionID: "s", Message: "hi"})
	if !strings.Contains(o2.Error, "构建平面暂不可达") {
		t.Errorf("turn 不可达应诚实回执: %q", o2.Error)
	}
	o3, _ := ontoSaveFn(context.Background(), ontoSaveIn{SessionID: "s", ConfirmNote: "ok"})
	if !strings.Contains(o3.Error, "构建平面暂不可达") {
		t.Errorf("save 不可达应诚实回执: %q", o3.Error)
	}
}

func TestOntoChatRegistered(t *testing.T) {
	reg := NewRegistry()
	if err := registerOntoChatTools(reg); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	for _, id := range []string{"ontochat_start", "ontochat_turn", "ontochat_save"} {
		e, ok := reg.Get(id)
		if !ok {
			t.Errorf("工具 %s 未注册", id)
			continue
		}
		if e.Source != SourceBuiltin {
			t.Errorf("%s source=%s 应为 builtin", id, e.Source)
		}
		if _, err := e.New(context.Background()); err != nil {
			t.Errorf("%s 实例化失败: %v", id, err)
		}
	}
	// 描述纪律抽查：save 工具描述必须带确认纪律（92 号 §8 R2 双保险之工具面）
	if e, _ := reg.Get("ontochat_save"); !strings.Contains(e.Description, "确认") {
		t.Error("save 工具描述应带确认纪律")
	}
	// REQ-231① 危险清单：ontochat_save 写类内置入 danger 档审批范围；start/turn 会话过程免审
	if !IsDangerousTool("ontochat_save") {
		t.Error("ontochat_save 应入危险清单（写类内置）")
	}
	if IsDangerousTool("ontochat_start") || IsDangerousTool("ontochat_turn") {
		t.Error("ontochat_start/turn 为会话过程工具，不应入危险清单")
	}
}
