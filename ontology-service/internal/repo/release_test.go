package repo

// REQ-541/M348 发布快照测试：冻结+digest+不可变语义（同内容重发布幂等/漂移拒绝）+
// 依赖账本 manifest（pin 解析闭包）。沿 external_refs_pin_test.go pinFixture 先例。

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func newReleaseStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir()+"/release.db", "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seedOntology 建本体+写 spec 工件+落版本快照（artifact 恒头版本内容，与 SaveVersion 同源）。
func seedOntology(t *testing.T, st *Store, id, spec string) {
	t.Helper()
	if _, err := st.CreateOntology(id, id, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact(id, "spec_json", spec, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveVersion(id, 1, spec, "", ""); err != nil {
		t.Fatal(err)
	}
}

func mustPublish(t *testing.T, st *Store, id, name, spec, ttl string) *Ontology {
	t.Helper()
	o, err := st.PublishRelease(id, name, spec, ttl, len(strings.Split(ttl, "\n")))
	if err != nil {
		t.Fatalf("PublishRelease %s 失败: %v", id, err)
	}
	return o
}

func sum256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestPublishRelease_FreezeDigestAndManifest_Req541(t *testing.T) {
	st := newReleaseStore(t)
	spec := `{"name":"A","concepts":[{"name":"缺陷"}]}`
	seedOntology(t, st, "A", spec)
	ttl := "# A snapshot ttl\n<urn:o:A:concept:缺陷> a owl:Class ."
	o := mustPublish(t, st, "A", "", spec, ttl)
	if o.Status != "published" || o.VersionName != "v1" {
		t.Fatalf("发布态期望 published/v1，实际 %s/%s", o.Status, o.VersionName)
	}
	if o.Release == nil {
		t.Fatal("发布响应应携带 release 元信息")
	}
	if o.Release.SpecSHA256 != sum256(spec) || o.Release.TTLSHA256 != sum256(ttl) {
		t.Fatalf("digest 应为内容 sha256：spec %s / ttl %s", o.Release.SpecSHA256, o.Release.TTLSHA256)
	}
	rel, err := st.GetRelease("A", 1)
	if err != nil {
		t.Fatal(err)
	}
	if rel.SpecJSON != spec || rel.TTL != ttl {
		t.Fatal("冻结快照内容应与发布输入逐字节一致")
	}
	if rel.Triples <= 0 {
		t.Fatalf("triples 应透传发布时计数，实际 %d", rel.Triples)
	}
	// 依赖账本：无仓内引用→依赖空表+合法 JSON
	var m ReleaseManifest
	if err := json.Unmarshal([]byte(rel.ManifestJSON), &m); err != nil {
		t.Fatalf("manifest 应为合法 JSON: %v", err)
	}
	if m.OntoID != "A" || m.Version != 1 || len(m.Dependencies) != 0 {
		t.Fatalf("manifest 期望 A/v1/空依赖，实际 %+v", m)
	}
}

func TestPublishRelease_RepublishSameContentIdempotent_Req541(t *testing.T) {
	st := newReleaseStore(t)
	spec, ttl := `{"name":"A","concepts":[{"name":"缺陷"}]}`, "<urn:o:A:concept:缺陷> a owl:Class ."
	seedOntology(t, st, "A", spec)
	first := mustPublish(t, st, "A", "", spec, ttl)
	again := mustPublish(t, st, "A", "renamed-tag", spec, ttl)
	if first.Release.SpecSHA256 != again.Release.SpecSHA256 || first.Release.TTLSHA256 != again.Release.TTLSHA256 {
		t.Fatal("同版本同内容重发布 digest 必须一致（验收锚点）")
	}
	if first.Release.PublishedAt != again.Release.PublishedAt {
		t.Fatalf("重发布不应刷新 published_at（不可变语义）：%s → %s", first.Release.PublishedAt, again.Release.PublishedAt)
	}
	if again.VersionName != "renamed-tag" {
		t.Fatalf("tag 标签应可随重发布刷新（OCI tag 可变名语义），实际 %q", again.VersionName)
	}
	rels, err := st.ListReleases("A")
	if err != nil || len(rels) != 1 {
		t.Fatalf("同版本重发布不应新增快照行: n=%d err=%v", len(rels), err)
	}
	if rels[0].VersionName != "renamed-tag" || rels[0].SpecSHA256 != sum256(spec) {
		t.Fatalf("清单行应保留冻结 digest+新 tag 标签: %+v", rels[0])
	}
}

func TestPublishRelease_ContentDriftRejected_Req541(t *testing.T) {
	st := newReleaseStore(t)
	spec := `{"name":"A","concepts":[{"name":"缺陷"}]}`
	seedOntology(t, st, "A", spec)
	ttl := "<urn:o:A:concept:缺陷> a owl:Class ."
	mustPublish(t, st, "A", "", spec, ttl)
	// 同版本内容漂移（ttl 变更——如引用登记后 TTL 物化面变化）：409 拒绝不改快照
	_, err := st.PublishRelease("A", "", spec, ttl+"\n# drifted", -1)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("内容漂移重发布应 ErrConflict，实际 %v", err)
	}
	rel, err := st.GetRelease("A", 1)
	if err != nil {
		t.Fatal(err)
	}
	if rel.TTL != ttl {
		t.Fatal("拒绝路径不得改写冻结快照内容")
	}
	// spec 漂移同样拒绝
	_, err = st.PublishRelease("A", "", spec+" ", ttl, -1)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("spec 漂移重发布应 ErrConflict，实际 %v", err)
	}
	// 漂移拒绝不改变发布态一致性（快照仍在、可按 tag 取）
	if _, err := st.GetReleaseMeta("A", 1); err != nil {
		t.Fatalf("漂移拒绝后快照仍应可取: %v", err)
	}
}

func TestPublishRelease_ManifestPinClosure_Req541(t *testing.T) {
	st := newReleaseStore(t)
	// B 两个版本（pin 钉 v1），C 头版本 v1；A import B(pin v1)+import C(浮动)；B import C。
	seedOntology(t, st, "A", `{"name":"A","concepts":[{"name":"缺陷"}]}`)
	specB1 := `{"name":"B","concepts":[{"name":"需求"}]}`
	specB2 := `{"name":"B","concepts":[{"name":"需求项"}]}`
	if _, err := st.CreateOntology("B", "B", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("B", "spec_json", specB1, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveVersion("B", 1, specB1, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutArtifact("B", "spec_json", specB2, true); err != nil {
		t.Fatal(err)
	}
	if v, err := st.BumpVersion("B"); err != nil || v != 2 {
		t.Fatalf("B 应 bump 到 v2: v=%d err=%v", v, err)
	}
	if err := st.SaveVersion("B", 2, specB2, "", ""); err != nil {
		t.Fatal(err)
	}
	seedOntology(t, st, "C", `{"name":"C","concepts":[{"name":"用例"}]}`)
	// A→B（import，pin v1）——发布后 B 演进到 v2，账本仍记 pin v1
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToOntoID: "B", ToConcept: "(本体)", RefKind: "import", PinVersion: 1}); err != nil {
		t.Fatal(err)
	}
	// A→C（import，浮动）→ 记发布时点头版本 v1
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToOntoID: "C", ToConcept: "(本体)", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	// B→C（import，浮动）→ 闭包传递
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "B", FromConcept: OntologyLevelRef, ToOntoID: "C", ToConcept: "(本体)", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	// A 外部 IRI 哨兵（远端不拉取，D-O27 → remote_skipped）
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToConcept: "http://xmlns.com/foaf/0.1/", RefKind: "import", Justification: "import_statement"}); err != nil {
		t.Fatal(err)
	}
	// 对齐登记（纯登记不产内容）不入闭包
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: "缺陷", ToOntoID: "C", ToConcept: "用例", RefKind: "alignment", Predicate: "exactMatch"}); err != nil {
		t.Fatal(err)
	}
	o := mustPublish(t, st, "A", "", `{"name":"A","concepts":[{"name":"缺陷"}]}`, "<urn:o:A:concept:缺陷> a owl:Class .")
	rel, err := st.GetRelease("A", o.Version)
	if err != nil {
		t.Fatal(err)
	}
	var m ReleaseManifest
	if err := json.Unmarshal([]byte(rel.ManifestJSON), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Dependencies) != 2 {
		t.Fatalf("闭包应恰含 B/C（alignment 不入闭包），实际 %+v", m.Dependencies)
	}
	byID := map[string]ManifestDependency{}
	for _, d := range m.Dependencies {
		byID[d.OntoID] = d
	}
	b, ok := byID["B"]
	if !ok || b.Resolution != "pin" || b.Version != 1 || b.SpecSHA256 != sum256(specB1) {
		t.Fatalf("B 应 pin v1 且指纹=该版本快照 digest，实际 %+v", b)
	}
	c, ok := byID["C"]
	if !ok || c.Resolution != "head" || c.Version != 1 || c.SpecSHA256 != sum256(`{"name":"C","concepts":[{"name":"用例"}]}`) {
		t.Fatalf("C 应 head v1 且指纹=头版本工件 digest，实际 %+v", c)
	}
	if len(m.RemoteSkipped) != 1 || m.RemoteSkipped[0] != "http://xmlns.com/foaf/0.1/" {
		t.Fatalf("外部 IRI 应记 remote_skipped，实际 %v", m.RemoteSkipped)
	}
}

func TestPublishRelease_ManifestCycleAndMissing_Req541(t *testing.T) {
	st := newReleaseStore(t)
	// 互引环 A⇄B：visited 收敛=并集语义，A 不自列入依赖
	seedOntology(t, st, "A", `{"name":"A","concepts":[{"name":"缺陷"}]}`)
	seedOntology(t, st, "B", `{"name":"B","concepts":[{"name":"需求"}]}`)
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToOntoID: "B", ToConcept: "(本体)", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "B", FromConcept: OntologyLevelRef, ToOntoID: "A", ToConcept: "(本体)", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	o := mustPublish(t, st, "A", "", `{"name":"A","concepts":[{"name":"缺陷"}]}`, "<urn:o:A:concept:缺陷> a owl:Class .")
	rel, _ := st.GetRelease("A", o.Version)
	var m ReleaseManifest
	if err := json.Unmarshal([]byte(rel.ManifestJSON), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Dependencies) != 1 || m.Dependencies[0].OntoID != "B" || m.Dependencies[0].Resolution != "head" {
		t.Fatalf("环闭包应恰含 B（A 不自列），实际 %+v", m.Dependencies)
	}
	// 悬空目标：B 发布后被删 → 重发布 C 记 missing（不阻断发布）
	seedOntology(t, st, "C", `{"name":"C","concepts":[{"name":"用例"}]}`)
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "C", FromConcept: OntologyLevelRef, ToOntoID: "B", ToConcept: "(本体)", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteOntology("B"); err != nil {
		t.Fatal(err)
	}
	o2 := mustPublish(t, st, "C", "", `{"name":"C","concepts":[{"name":"用例"}]}`, "<urn:o:C:concept:用例> a owl:Class .")
	rel2, err := st.GetRelease("C", o2.Version)
	if err != nil {
		t.Fatal(err)
	}
	var m2 ReleaseManifest
	if err := json.Unmarshal([]byte(rel2.ManifestJSON), &m2); err != nil {
		t.Fatal(err)
	}
	if len(m2.Dependencies) != 1 || m2.Dependencies[0].OntoID != "B" || m2.Dependencies[0].Resolution != "missing" || m2.Dependencies[0].Version != 0 {
		t.Fatalf("已删目标应记 missing/v0，实际 %+v", m2.Dependencies)
	}
}

func TestPublishRelease_HeadPinContentFingerprint_Req541(t *testing.T) {
	st := newReleaseStore(t)
	// head 解析指纹=发布时点被引方头版本 artifact 内容——发布后被引方升级不影响已冻结账本
	seedOntology(t, st, "A", `{"name":"A","concepts":[{"name":"缺陷"}]}`)
	specB1 := `{"name":"B","concepts":[{"name":"需求"}]}`
	seedOntology(t, st, "B", specB1)
	if _, err := st.CreateExternalRef(&ExternalRef{FromOntoID: "A", FromConcept: OntologyLevelRef, ToOntoID: "B", ToConcept: "(本体)", RefKind: "import"}); err != nil {
		t.Fatal(err)
	}
	o := mustPublish(t, st, "A", "", `{"name":"A","concepts":[{"name":"缺陷"}]}`, "<urn:o:A:concept:缺陷> a owl:Class .")
	// B 演进 v2（发布后）
	if err := st.PutArtifact("B", "spec_json", `{"name":"B","concepts":[{"name":"需求项"}]}`, true); err != nil {
		t.Fatal(err)
	}
	if v, err := st.BumpVersion("B"); err != nil || v != 2 {
		t.Fatal(err)
	}
	if err := st.SaveVersion("B", 2, `{"name":"B","concepts":[{"name":"需求项"}]}`, "", ""); err != nil {
		t.Fatal(err)
	}
	rel, _ := st.GetRelease("A", o.Version)
	var m ReleaseManifest
	if err := json.Unmarshal([]byte(rel.ManifestJSON), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Dependencies) != 1 || m.Dependencies[0].SpecSHA256 != sum256(specB1) || m.Dependencies[0].Version != 1 {
		t.Fatalf("账本应冻结发布时点 B v1 指纹，实际 %+v", m.Dependencies)
	}
}
