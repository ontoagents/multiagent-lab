package vocabctx

// REQ-543/M350 单测：仓内词表上下文组装——RepoList/RefVocab 出料、注入上限三重截断、
// 中性文案、pin 解析（REQ-540 同目标取最低）、同版本缓存（指纹命中/失配）。

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/ontology-service/internal/repo"
)

func vctxFixture(t *testing.T) *repo.Store {
	t.Helper()
	st, err := repo.Open(filepath.Join(t.TempDir(), "vocabctx.db"), "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func mustOnto(t *testing.T, st *repo.Store, id, name, spec string) {
	t.Helper()
	if _, err := st.CreateOntology(id, name, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact(id, "spec_json", spec, true); err != nil {
		t.Fatal(err)
	}
}

func TestRepoListEmptyAndPopulated_Req543(t *testing.T) {
	st := vctxFixture(t)
	b := New(st)
	if got := b.RepoList(); got != "" {
		t.Fatalf("空仓应返回空串（整段不进 prompt），得 %q", got)
	}
	mustOnto(t, st, "defects", "缺陷管理", `{"concepts":[{"name":"缺陷","label":"缺陷单"},{"name":"模块"}]}`)
	mustOnto(t, st, "orgs", "组织人员", `{"concepts":[{"name":"部门"}]}`)
	got := b.RepoList()
	for _, want := range []string{"仓内已有本体", "缺陷管理（defects） v1", "组织人员（orgs） v1", "缺陷单、缺陷、模块"} {
		if !strings.Contains(got, want) {
			t.Fatalf("RepoList 应含 %q，得:\n%s", want, got)
		}
	}
	// label 优先、名兜底双投影 + 不强制中性文案
	if !strings.Contains(got, "不强制") {
		t.Fatal("RepoList 应含中性文案（是否采用不强制）")
	}
}

func TestRepoListCap_Req543(t *testing.T) {
	st := vctxFixture(t)
	for i := 0; i < MaxRepoOntos+2; i++ {
		mustOnto(t, st, string(rune('a'+i)), "本体"+string(rune('A'+i)), `{"concepts":[{"name":"C1"}]}`)
	}
	got := New(st).RepoList()
	// 每条目一行（\n 前缀）+ 截断注记一行
	if n := strings.Count(got, "\n"); n != MaxRepoOntos+1 {
		t.Fatalf("至多列 %d 个本体+1 行注记，实得 %d 行:\n%s", MaxRepoOntos, n, got)
	}
	if !strings.Contains(got, "其余 2 个本体略——注入上限护栏") {
		t.Fatalf("超出上限应如实注记，得:\n%s", got)
	}
}

func TestRepoListCache_Req543(t *testing.T) {
	st := vctxFixture(t)
	b := New(st)
	mustOnto(t, st, "d1", "一号", `{"concepts":[{"name":"缺陷"}]}`)
	first := b.RepoList()
	second := b.RepoList()
	if first != second {
		t.Fatal("同版本两次组装应逐字一致（缓存命中）")
	}
	hits, misses := b.CacheStats()
	if hits != 1 || misses != 1 {
		t.Fatalf("缓存统计应 hits=1 misses=1，得 %d/%d", hits, misses)
	}
	// 版本 bump → 指纹变化 → 重建（内容随版本刷新）
	if _, err := st.BumpVersion("d1"); err != nil {
		t.Fatal(err)
	}
	third := b.RepoList()
	if !strings.Contains(third, "一号（d1） v2") {
		t.Fatalf("版本 bump 后应重建并显示 v2:\n%s", third)
	}
	if hits2, misses2 := b.CacheStats(); hits2 != 1 || misses2 != 2 {
		t.Fatalf("bump 后应 miss 重建，得 hits=%d misses=%d", hits2, misses2)
	}
}

func TestRefVocabBasic_Req543(t *testing.T) {
	st := vctxFixture(t)
	b := New(st)
	mustOnto(t, st, "src", "源本体", `{"concepts":[{"name":"缺陷"}]}`)
	// 悬空目标：先建后删（external_refs 无 FK 级联，孤儿登记行存活=049 迁移实证）
	mustOnto(t, st, "ghost", "将被删除的本体", `{"concepts":[{"name":"幽灵"}]}`)
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToOntoID: "ghost", ToConcept: "幽灵", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteOntology("ghost"); err != nil {
		t.Fatal(err)
	}
	if got := b.RefVocab("src"); got != "" {
		t.Fatalf("无仓内可解析被引目标应返回空串（悬空如实跳过），得 %q", got)
	}
	mustOnto(t, st, "dst", "缺陷管理库", `{"concepts":[{"name":"缺陷","label":"缺陷单"},{"name":"模块"},{"name":"迭代"}],"relations":[{"name":"属于"}]}`)
	mustOnto(t, st, "orgs", "组织本体", `{"concepts":[{"name":"部门"},{"name":"员工"}]}`)
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToOntoID: "dst", ToConcept: "缺陷", RefKind: "reuse"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToOntoID: "dst", ToConcept: "模块", RefKind: "alignment"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToOntoID: "orgs", ToConcept: "部门", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	// 外部 IRI 登记态（to_onto_id 空）不入词表
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToConcept: "http://ex.org/v", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	got := b.RefVocab("src")
	for _, want := range []string{
		"已登记被引本体词表",
		"缺陷管理库（dst） v1〔alignment/reuse〕",
		"缺陷单、缺陷、模块、迭代",
		"｜关系：属于",
		"组织本体（orgs） v1〔import〕",
		"不是可引用名册",             // 中性文案+名册边界（保住两阶段校验硬约束）
		"登记 external_refs 落地", // 复用/对齐建议可落地到登记动作
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RefVocab 应含 %q，得:\n%s", want, got)
		}
	}
	if strings.Contains(got, "http://ex.org/v") {
		t.Fatal("外部 IRI 登记态不应入词表（D-O27 不拉取远端）")
	}
	if strings.Contains(got, "ghost") {
		t.Fatal("悬空目标（已删/不存在）不应入词表")
	}
	// 同目标多登记去重：dst 只出现一次（种类合并标签）
	if n := strings.Count(got, "缺陷管理库（dst）"); n != 1 {
		t.Fatalf("同目标应收敛为一节，dst 出现 %d 次", n)
	}
}

func TestRefVocabPinResolution_Req543(t *testing.T) {
	st := vctxFixture(t)
	b := New(st)
	mustOnto(t, st, "src", "源本体", `{"concepts":[{"name":"缺陷"}]}`)
	specV1 := `{"concepts":[{"name":"需求"}]}`
	specV2 := `{"concepts":[{"name":"需求项"}]}`
	mustOnto(t, st, "dst", "目标本体", specV1)
	if err := st.SaveVersion("dst", 1, specV1, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("dst", "spec_json", specV2, true); err != nil {
		t.Fatal(err)
	}
	if v, err := st.BumpVersion("dst"); err != nil || v != 2 {
		t.Fatalf("dst 应 bump 到 v2: v=%d err=%v", v, err)
	}
	if err := st.SaveVersion("dst", 2, specV2, "", ""); err != nil {
		t.Fatal(err)
	}
	r, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToOntoID: "dst", ToConcept: "需求", RefKind: "import", PinVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := b.RefVocab("src")
	if !strings.Contains(got, "目标本体（dst） v1〔import · pin v1〕") || !strings.Contains(got, "需求") || strings.Contains(got, "需求项") {
		t.Fatalf("pin v1 应显示 v1 词表内容（pin 钉死语义）:\n%s", got)
	}
	// 显式清除 pin → 浮动头版本 → 指纹变化重建出 v2 内容
	if _, err := st.SetExternalRefPin("src", r.RefID, 0); err != nil {
		t.Fatal(err)
	}
	got2 := b.RefVocab("src")
	if !strings.Contains(got2, "需求项") || strings.Contains(got2, "pin v") {
		t.Fatalf("清除 pin 后应浮动头版本 v2:\n%s", got2)
	}
}

func TestRefVocabCacheSameVersion_Req543(t *testing.T) {
	st := vctxFixture(t)
	b := New(st)
	mustOnto(t, st, "src", "源本体", `{"concepts":[{"name":"缺陷"}]}`)
	specV1 := `{"concepts":[{"name":"需求"}]}`
	mustOnto(t, st, "dst", "目标本体", specV1)
	if err := st.SaveVersion("dst", 1, specV1, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToOntoID: "dst", ToConcept: "需求", RefKind: "import", PinVersion: 1}); err != nil {
		t.Fatal(err)
	}
	first := b.RefVocab("src")
	if h1, m1 := b.CacheStats(); h1 != 0 || m1 != 1 {
		t.Fatalf("首次应 miss，得 hits=%d misses=%d", h1, m1)
	}
	// 被引方头版本演进（v2/v3）而 pin 仍 v1：指纹不变 → 缓存命中（同版本不重复花费，
	// 且内容仍为 pin v1 词表=REQ-540 pin 钉死语义在注入面成立）
	specV3 := `{"concepts":[{"name":"全新概念"}]}`
	if err := st.PutArtifact("dst", "spec_json", specV3, true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.BumpVersion("dst"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveVersion("dst", 2, specV3, "", ""); err != nil {
		t.Fatal(err)
	}
	second := b.RefVocab("src")
	if second != first {
		t.Fatalf("pin 未变时头版本演进应命中缓存（同版本不重复花费）:\n首:%s\n次:%s", first, second)
	}
	if h2, m2 := b.CacheStats(); h2 != 1 || m2 != 1 {
		t.Fatalf("第二次应命中缓存，得 hits=%d misses=%d", h2, m2)
	}
	// 新增指向同一目标的登记（同目标取最低 pin 语义下指纹不变——词表内容不变，命中缓存正确）
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToOntoID: "dst", ToConcept: "需求", RefKind: "reuse"}); err != nil {
		t.Fatal(err)
	}
	if _, m3 := b.CacheStats(); m3 != 1 {
		t.Fatalf("同目标同版本新增登记不应触发重建，得 misses=%d", m3)
	}
}

func TestRefVocabNone_Req543(t *testing.T) {
	st := vctxFixture(t)
	b := New(st)
	mustOnto(t, st, "src", "源本体", `{"concepts":[{"name":"缺陷"}]}`)
	if got := b.RefVocab("src"); got != "" {
		t.Fatalf("无登记应返回空串，得 %q", got)
	}
	// 仅有外部 IRI 登记态 → 空串
	if _, err := st.CreateExternalRef(&repo.ExternalRef{FromOntoID: "src", FromConcept: "缺陷", ToConcept: "http://ex.org/v", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	if got := b.RefVocab("src"); got != "" {
		t.Fatalf("纯外部 IRI 登记应返回空串，得 %q", got)
	}
}

func TestTotalCharCap_Req543(t *testing.T) {
	st := vctxFixture(t)
	// 词条条数上限（MaxListTerms=8）挡不住单条超长名——总字符兜底（护栏①第三重）的真实触发面
	var terms []string
	for i := 0; i < MaxListTerms; i++ {
		terms = append(terms, `{"name":"超长概念名`+strings.Repeat("长", 600)+string(rune('甲'+i))+`"}`)
	}
	mustOnto(t, st, "big", "巨型词表", `{"concepts":[`+strings.Join(terms, ",")+`]}`)
	got := New(st).RepoList()
	if !strings.Contains(got, "已截断——注入上限护栏") {
		t.Fatalf("超总字符上限应截断并注记，得前 200 rune:\n%s", string([]rune(got)[:200]))
	}
	if n := len([]rune(strings.TrimSuffix(got, "\n"+capNote))); n > MaxTotalChars {
		t.Fatalf("截断后应 ≤%d rune，得 %d", MaxTotalChars, n)
	}
}
