package datasource

// REQ-595/M406 单测：内省→骨架确定性规则（表→概念/声明 FK→对象属性/junction→关系/列→数据属性）、
// 列画像（枚举/唯一性/日期格式/top 值）、A6 采样熔断、LLM 窄调用消毒与回退（桩 LLM）、
// Analyze 无 LLM 全链（LLMUsed=false 如实标注）。

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
)

// seedSampleDB 建样例库：customers/orders/products + order_products junction（纯 FK）。
func seedSampleDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE customers (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT UNIQUE)`,
		`CREATE TABLE products (id INTEGER PRIMARY KEY, title TEXT, price REAL, supplier_id INTEGER)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, customer_id INTEGER REFERENCES customers(id), amount REAL, status TEXT, created_at DATETIME)`,
		`CREATE TABLE order_products (order_id INTEGER REFERENCES orders(id), product_id INTEGER REFERENCES products(id), PRIMARY KEY (order_id, product_id))`,
		`INSERT INTO customers (id, name, email) VALUES (1,'张三','z@example.com'),(2,'李四','l@example.com'),(3,'王五','w@example.com')`,
		`INSERT INTO products (id, title, price) VALUES (1,'路由器',199.0),(2,'交换机',399.0)`,
		`INSERT INTO orders (id, customer_id, amount, status, created_at) VALUES
		 (1,1,199.0,'paid','2026-01-02 10:00:00'),(2,2,399.0,'paid','2026-01-03 11:30:00'),
		 (3,1,59.5,'pending','2026-01-05 09:15:00'),(4,3,120.0,'paid','2026-01-06 16:40:00')`,
		`INSERT INTO order_products (order_id, product_id) VALUES (1,1),(1,2),(2,2),(3,1),(4,1)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("建样例库失败 %s: %v", s, err)
		}
	}
	return path
}

func TestIntrospectAndSkeleton(t *testing.T) {
	path := seedSampleDB(t)
	db, err := openReadOnly(path)
	if err != nil {
		t.Fatalf("只读打开失败: %v", err)
	}
	defer db.Close()
	schema, err := introspect(context.Background(), db, 200)
	if err != nil {
		t.Fatalf("内省失败: %v", err)
	}
	if len(schema.Tables) != 4 {
		t.Fatalf("期望 4 表，得 %d", len(schema.Tables))
	}
	profiles, warns := profileSchema(context.Background(), db, schema, 1000)
	if len(warns) != 0 {
		t.Fatalf("样例库不应有采样告警: %v", warns)
	}
	sk := buildSkeleton(schema, profiles)
	if errs := sk.Spec.Validate(); len(errs) > 0 {
		t.Fatalf("骨架应通过 Validate: %v", errs)
	}
	// 表→概念（junction 不出概念）
	if len(sk.Spec.Concepts) != 3 {
		t.Fatalf("期望 3 概念（junction 不出），得 %d: %+v", len(sk.Spec.Concepts), sk.Spec.Concepts)
	}
	// 声明 FK → 对象关系（orders.customer_id→customers）+ junction 关系（order_products）
	if len(sk.Spec.Relations) != 2 {
		t.Fatalf("期望 2 关系（1 声明 FK+1 junction），得 %d: %+v", len(sk.Spec.Relations), sk.Spec.Relations)
	}
	var fkRel, junctionRel bool
	for _, r := range sk.Spec.Relations {
		if r.Evidence != "declared" {
			t.Fatalf("关系证据应为 declared: %+v", r)
		}
		if r.From == "orders" && r.To == "customers" {
			fkRel = true
		}
		// junction 两 FK 目标两两建关系（PRAGMA foreign_key_list 返回序与声明序相反，方向不敏感）
		if (r.From == "orders" && r.To == "products") || (r.From == "products" && r.To == "orders") {
			junctionRel = true
		}
	}
	if !fkRel || !junctionRel {
		t.Fatalf("声明 FK 关系或 junction 关系缺失: %+v", sk.Spec.Relations)
	}
	// 数据属性：customers name/email；orders amount/status/created_at；products title/price/supplier_id
	if len(sk.Spec.DataProperties) != 8 {
		t.Fatalf("期望 8 数据属性，得 %d: %+v", len(sk.Spec.DataProperties), sk.Spec.DataProperties)
	}
	rangeOf := map[string]string{}
	for _, dp := range sk.Spec.DataProperties {
		rangeOf[dp.Name] = dp.Range
	}
	if rangeOf["amount"] != "number" || rangeOf["created_at"] != "date" || rangeOf["status"] != "string" {
		t.Fatalf("range 推断不符: %v", rangeOf)
	}
}

func TestProfileEvidence(t *testing.T) {
	path := seedSampleDB(t)
	db, err := openReadOnly(path)
	if err != nil {
		t.Fatalf("只读打开失败: %v", err)
	}
	defer db.Close()
	schema, err := introspect(context.Background(), db, 200)
	if err != nil {
		t.Fatalf("内省失败: %v", err)
	}
	profiles, _ := profileSchema(context.Background(), db, schema, 1000)
	byTable := map[string]TableProfile{}
	for _, tp := range profiles {
		byTable[tp.Table] = tp
	}
	// orders.status 枚举候选（paid/pending 两种）
	ord := byTable["orders"]
	if ord.SampledRows != 4 {
		t.Fatalf("orders 采样应 4 行，得 %d", ord.SampledRows)
	}
	var cp *ColumnProfile
	for i := range ord.Columns {
		if ord.Columns[i].Column == "status" {
			cp = &ord.Columns[i]
		}
	}
	if cp == nil || len(cp.EnumCandidates) != 2 {
		t.Fatalf("status 应有 2 枚举候选: %+v", cp)
	}
	// customers.id 采样内唯一；customer_id 非唯一
	cust := byTable["customers"]
	for _, c := range cust.Columns {
		if c.Column == "id" && !c.IsUnique {
			t.Fatalf("customers.id 应采样内唯一")
		}
	}
	// created_at 日期格式推断（modernc 驱动 DATETIME 扫为 RFC3339 字符串 → ISO8601 标签）
	var foundDate bool
	for _, c := range ord.Columns {
		if c.Column == "created_at" && len(c.DateFormats) > 0 && strings.Contains(c.DateFormats[0], "ISO8601") {
			foundDate = true
		}
	}
	if !foundDate {
		t.Fatalf("created_at 应推断出日期格式: %+v", ord.Columns)
	}
}

func TestProfileCircuitBreaker(t *testing.T) {
	path := seedSampleDB(t)
	db, err := openReadOnly(path)
	if err != nil {
		t.Fatalf("只读打开失败: %v", err)
	}
	defer db.Close()
	schema, err := introspect(context.Background(), db, 200)
	if err != nil {
		t.Fatalf("内省失败: %v", err)
	}
	// 内省后删 3 表 → 连续 3 次基础设施失败 → 熔断（products 跳过采样注记）
	// 删表需可写连接（被测只读连接 query_only 强制——正是边界所在）
	wdb, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("打开可写连接失败: %v", err)
	}
	for _, tb := range []string{"customers", "orders", "order_products"} {
		if _, err := wdb.Exec("DROP TABLE " + tb); err != nil {
			wdb.Close()
			t.Fatalf("删表失败: %v", err)
		}
	}
	wdb.Close()
	profiles, warns := profileSchema(context.Background(), db, schema, 1000)
	if len(warns) < 4 {
		t.Fatalf("应有 3 表失败注记+1 熔断注记，得 %v", warns)
	}
	broken := 0
	for _, tp := range profiles {
		if strings.Contains(tp.Error, "采样熔断") {
			broken++
		}
	}
	if broken != 1 { // products（第 4 表）被熔断跳过；customers/orders/order_products 各自失败
		t.Fatalf("期望 1 表被熔断跳过注记，得 %d: %+v", broken, profiles)
	}
}

const refineStubItems = `{"items":[
 {"key":"customers","label":"客户","definition":"购买商品的客户主体"},
 {"key":"orders","label":"订单","definition":"客户下的购买订单"},
 {"key":"products","label":"产品","definition":"在售商品"},
 {"key":"orders.status","definition":"订单支付状态"},
 {"key":"ghost_table","label":"幻觉表","definition":"不应被应用"}
]}`

const refineStubFK = `{"items":[
 {"table":"products","column":"supplier_id","target_table":"customers","reason":"未声明的引用列（样例）"},
 {"table":"orders","column":"customer_id","target_table":"customers","reason":"已是声明 FK——应被过滤"},
 {"table":"orders","column":"created_at","target_table":"no_such_table","reason":"目标不存在——应被过滤"}
]}`

func TestRefineWithStubLLM(t *testing.T) {
	path := seedSampleDB(t)
	db, err := openReadOnly(path)
	if err != nil {
		t.Fatalf("只读打开失败: %v", err)
	}
	defer db.Close() // Windows 文件锁：不关则 TempDir 清理失败
	schema, err := introspect(context.Background(), db, 200)
	if err != nil {
		t.Fatalf("内省失败: %v", err)
	}
	profiles, _ := profileSchema(context.Background(), db, schema, 1000)
	sk := buildSkeleton(schema, profiles)

	// 桩 LLM：命名批与 FK 批共用端点，各取所需字段（消毒兜住幻觉）
	var sawFKPrompt bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := refineStubItems
		if strings.Contains(req.Prompt, "疑似外键") {
			sawFKPrompt = true
			resp = refineStubFK
		}
		b, _ := json.Marshal(map[string]any{"draft_json": resp, "usage": nil})
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	creator := llmcreate.New(srv.URL)

	st := refine(context.Background(), creator, schema, profiles, sk, nil)
	if !st.Used || st.Fallback {
		t.Fatalf("桩 LLM 全批应成功: used=%v fallback=%v warns=%v", st.Used, st.Fallback, st.Warnings)
	}
	if !sawFKPrompt {
		t.Fatalf("应发起隐含 FK 窄调用")
	}
	// 显示名应用（消毒命中）
	if sk.Spec.Concepts[0].Label != "客户" || sk.Spec.Concepts[0].Definition != "购买商品的客户主体" {
		t.Fatalf("概念显示名/描述未应用: %+v", sk.Spec.Concepts[0])
	}
	// 幻觉 key 不应用（ghost_table 无对应概念——概念 Label 仍为原表名）
	for _, c := range sk.Spec.Concepts {
		if c.Label == "幻觉表" {
			t.Fatalf("幻觉条目不应被应用")
		}
	}
	// 属性描述应用（orders.status）
	for _, dp := range sk.Spec.DataProperties {
		if dp.Name == "status" && dp.Definition != "订单支付状态" {
			t.Fatalf("属性描述未应用: %+v", dp)
		}
	}
	// FK 建议：白名单过滤后仅 1 条（声明 FK 重叠/不存在目标被滤）
	if len(st.FKSuggestions) != 1 {
		t.Fatalf("期望 1 条 FK 建议（2 条被消毒过滤），得 %d: %+v", len(st.FKSuggestions), st.FKSuggestions)
	}
	if st.FKSuggestions[0].Column != "supplier_id" || st.FKSuggestions[0].TargetTable != "customers" {
		t.Fatalf("FK 建议内容不符: %+v", st.FKSuggestions[0])
	}
	// FK 建议不进骨架（必人审——不静默落库）
	for _, r := range sk.Spec.Relations {
		if r.From == "orders" && r.To == "customers" && r.Evidence != "declared" {
			t.Fatalf("FK 建议不应静默进骨架")
		}
	}
}

func TestRefineFallbackOnBadLLM(t *testing.T) {
	path := seedSampleDB(t)
	db, _ := openReadOnly(path)
	defer db.Close()
	schema, err := introspect(context.Background(), db, 200)
	if err != nil {
		t.Fatalf("内省失败: %v", err)
	}
	profiles, _ := profileSchema(context.Background(), db, schema, 1000)
	sk := buildSkeleton(schema, profiles)
	// 桩 LLM 恒返回非法 JSON → 两轮修复失败 → 批回退（不中断，骨架保持确定性结果）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		b, _ := json.Marshal(map[string]any{"draft_json": "not-json{{", "usage": nil})
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	creator := llmcreate.New(srv.URL)
	st := refine(context.Background(), creator, schema, profiles, sk, nil)
	if !st.Fallback {
		t.Fatalf("坏 LLM 输出应注记回退: %+v", st)
	}
	if len(sk.Spec.Concepts) == 0 || sk.Spec.Concepts[0].Definition != "" {
		t.Fatalf("回退后骨架应为确定性结果: %+v", sk.Spec.Concepts)
	}
	if len(st.FKSuggestions) != 0 {
		t.Fatalf("FK 批失败应跳过不产建议: %+v", st.FKSuggestions)
	}
}

func TestAnalyzeWithoutLLM(t *testing.T) {
	path := seedSampleDB(t)
	prop, err := Analyze(context.Background(), Options{Path: path}, nil, nil)
	if err != nil {
		t.Fatalf("无 LLM 全链失败: %v", err)
	}
	if prop.LLMUsed || prop.LLMSkipped == "" {
		t.Fatalf("无 LLM 应如实标注: used=%v skipped=%q", prop.LLMUsed, prop.LLMSkipped)
	}
	if prop.Name != "shop" {
		t.Fatalf("建议名应取文件名去扩展: %q", prop.Name)
	}
	if errs := prop.Spec.Validate(); len(errs) > 0 {
		t.Fatalf("提案 spec 应通过 Validate: %v", errs)
	}
	if len(prop.Spec.Concepts) != 3 || len(prop.Spec.Relations) != 2 || len(prop.Spec.DataProperties) != 8 {
		t.Fatalf("提案骨架规模不符: concepts=%d relations=%d dataprops=%d", len(prop.Spec.Concepts), len(prop.Spec.Relations), len(prop.Spec.DataProperties))
	}
	// 非法路径人话报错
	if _, err := Analyze(context.Background(), Options{Path: filepath.Join(t.TempDir(), "missing.db")}, nil, nil); err == nil || !strings.Contains(err.Error(), "只读") {
		t.Fatalf("缺失文件应人话报错: %v", err)
	}
}
