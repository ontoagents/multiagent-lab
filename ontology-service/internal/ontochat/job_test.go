package ontochat

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newJobTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:/tmp/ontochat-job-test.db?_pragma=busy_timeout(3000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, ddl := range []string{
		`DROP TABLE IF EXISTS ontochat_job`,
		`DROP TABLE IF EXISTS ontochat_session`,
		`CREATE TABLE IF NOT EXISTS ontochat_session (
			id TEXT PRIMARY KEY, title TEXT, stage TEXT, round INTEGER,
			messages TEXT, context_json TEXT, ontology_id TEXT DEFAULT '',
			created_at TEXT, updated_at TEXT)`,
		`CREATE TABLE IF NOT EXISTS ontochat_job (
			id TEXT PRIMARY KEY, session_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'queued',
			error TEXT NOT NULL DEFAULT '', progress TEXT NOT NULL DEFAULT '', result_json TEXT,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_ontochat_job_session ON ontochat_job(session_id, status)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO ontochat_session (id, title, stage, round, messages, context_json, created_at, updated_at)
		VALUES ('sess-1', 'job 测试', 'domain', 1, '[]', '{}', '2026-10-08T00:00:00Z', '2026-10-08T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	return New(db)
}

// REQ-271/M80：job 生命周期 CRUD + 会话互斥 + 终态后释放。
func TestJobLifecycle(t *testing.T) {
	st := newJobTestStore(t)
	job, err := st.CreateJob("ontoj_1", "sess-1")
	if err != nil || job.Status != "queued" {
		t.Fatalf("CreateJob: %v %+v", err, job)
	}
	// 活跃判定：queued 占用会话
	if active, _ := st.ActiveJobBySession("sess-1"); !JobActive(active) {
		t.Fatal("queued 应为活跃任务")
	}
	// 进度与 running
	if err := st.UpdateJobStatus("ontoj_1", "running", ""); err != nil {
		t.Fatal(err)
	}
	_ = st.UpdateJobProgress("ontoj_1", "第 1/3 轮：调用模型生成中")
	j, err := st.GetJob("ontoj_1")
	if err != nil || j.Status != "running" || j.Progress == "" {
		t.Fatalf("running/progress 未落: %+v %v", j, err)
	}
	// 终态：result 落库 + 不再占用会话
	if err := st.SetJobResult("ontoj_1", []byte(`{"reply":"草稿已生成","stage":"draft"}`)); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateJobStatus("ontoj_1", "done", ""); err != nil {
		t.Fatal(err)
	}
	j, _ = st.GetJob("ontoj_1")
	if j.Status != "done" || !strings.Contains(string(j.Result), "草稿已生成") {
		t.Fatalf("终态/结果不符: %+v", j)
	}
	if active, _ := st.ActiveJobBySession("sess-1"); active != nil {
		t.Fatal("done 后会话应释放")
	}
}

// REQ-271 定案②：job 与 session 同库持久化——重开 Store（模拟进程重启）状态不丢；
// running 悬挂（updated_at 超时）自愈收敛为 error，会话释放。
func TestJobPersistenceAndStaleHeal(t *testing.T) {
	st := newJobTestStore(t)
	if _, err := st.CreateJob("ontoj_2", "sess-1"); err != nil {
		t.Fatal(err)
	}
	_ = st.UpdateJobStatus("ontoj_2", "running", "")

	// 模拟重启：新 Store 实例同库重开
	st2 := New(st.db)
	j, err := st2.GetJob("ontoj_2")
	if err != nil || j.Status != "running" {
		t.Fatalf("重启后 job 状态丢失: %+v %v", j, err)
	}

	// 悬挂自愈：把 updated_at 拨老 16min（> jobStaleRunning 15min）
	old := time.Now().UTC().Add(-16 * time.Minute).Format(time.RFC3339)
	if _, err := st2.db.Exec(`UPDATE ontochat_job SET updated_at = ? WHERE id = ?`, old, "ontoj_2"); err != nil {
		t.Fatal(err)
	}
	active, _ := st2.ActiveJobBySession("sess-1")
	if active != nil {
		t.Fatalf("悬挂 running 应自愈释放: %+v", active)
	}
	j, _ = st2.GetJob("ontoj_2")
	if j.Status != "error" || j.Error == "" {
		t.Fatalf("自愈未收敛为 error: %+v", j)
	}
}

// REQ-271：意图识别放宽（去 12 rune 硬阈）与 WillGenerate 同步/异步判定。
func TestWillGenerateAndIntent(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"生成草稿", true},
		{"请生成草稿", true},
		{"信息差不多了，帮我生成草稿吧", true}, // >12 rune + 生成 + 草稿 → 命中（旧 12 rune 硬阈漏判）
		{"生成关系的设计思路", true},          // ≤12 rune 含「生成」→ 命中（沿 REQ-246/G5 既有短句口径）
	}
	notHits := []string{
		"请帮我梳理一下生成概念之间关系的设计思路", // >12 rune 含「生成」无目标词 → 普通建模输入
		"",
		"now generate the draft please", // 无「生成」的英文长句不命中（精确 generate/draft 才命中）
	}
	for _, c := range cases {
		if !isGenerateIntent(c.text) {
			t.Fatalf("应命中生成意图: %q", c.text)
		}
	}
	for _, c := range notHits {
		if isGenerateIntent(c) {
			t.Fatalf("不应命中生成意图: %q", c)
		}
	}
	for _, s := range []struct {
		stage, text string
		want        bool
	}{
		{"cq", "生成草稿", false},          // cq 首轮纯本地，不做生成意图判断
		{"domain", "生成草稿", true},       // domain 生成意图 → 异步生成
		{"domain", "补充：还有 StatefulSet", false}, // domain 普通补充 → 同步归纳
		{"draft", "", true},             // draft 阶段空文本重生成 → 异步
		{"refine", "给 Deployment 增加副本数属性", true}, // 修正意见 → 异步
		{"done", "生成草稿", false},
	} {
		if got := WillGenerate(s.stage, s.text); got != s.want {
			t.Fatalf("WillGenerate(%q,%q)=%v want %v", s.stage, s.text, got, s.want)
		}
	}
}

// REQ-271⑥：提示词清单非空且字段完备（页面只读透出的数据源）。
func TestPromptsInventory(t *testing.T) {
	ps := Prompts()
	if len(ps) < 3 {
		t.Fatalf("提示词清单过少: %d", len(ps))
	}
	ids := map[string]bool{}
	for _, p := range ps {
		if p.ID == "" || p.Label == "" || p.Purpose == "" || p.Source == "" || p.Text == "" {
			t.Fatalf("提示词字段不全: %+v", p)
		}
		ids[p.ID] = true
	}
	for _, want := range []string{"spec_generation", "domain_sufficient"} {
		if !ids[want] {
			t.Fatalf("缺少核心提示词 %s", want)
		}
	}
}

// REQ-272：UpdateContext 定点更新（CQ 确认写入，不追加消息）。
func TestUpdateContextCQs(t *testing.T) {
	st := newJobTestStore(t)
	if err := st.UpdateContext("sess-1", func(c *Context) { c.CQs = []string{"药物可治疗哪些疾病？", "疾病簇由哪些症状组成？"} }); err != nil {
		t.Fatal(err)
	}
	back, err := st.Get("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Context.CQs) != 2 || back.Context.CQs[0] != "药物可治疗哪些疾病？" {
		t.Fatalf("CQs 未写入: %+v", back.Context.CQs)
	}
	if n := len(back.Messages); n != 0 {
		t.Fatalf("UpdateContext 不应追加消息: %d", n)
	}
}
