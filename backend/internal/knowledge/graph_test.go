package knowledge

// REQ-372 P2：L2 图谱层单测——摄取（frontmatter 投影+prereq 解析+断链）/前置闭包拓扑序。
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func graphFixture(t *testing.T) (*store.Store, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	card := func(rel, fm string) {
		p := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(fm), 0o644)
	}
	card("01_本体线/cards/a.md", "---\nkind: concept\ndomain: ontology\ntopic: 概念A\nprereq: []\nsources: []\n---\n正文")
	card("01_本体线/cards/b.md", "---\nkind: decision\ndomain: ontology\ntopic: 决策B\nprereq:\n  - a.md\nsources: []\n---\n正文")
	card("01_本体线/cards/c.md", "---\nkind: practice\ndomain: ontology\ntopic: 实践C\nprereq:\n  - b.md\n  - 缺失卡.md\nsources: []\n---\n正文")
	return st, root
}

func TestIngestKnowledgeGraph(t *testing.T) {
	st, root := graphFixture(t)
	res, err := IngestKnowledgeGraph(root, "platform-knowledge/09_知识体系", st)
	if err != nil {
		t.Fatal(err)
	}
	if res.Nodes != 3 {
		t.Fatalf("节点应 3: %d", res.Nodes)
	}
	if res.Edges != 2 {
		t.Fatalf("边应 2（c→b、b→a）: %d %+v", res.Edges, res)
	}
	if len(res.Broken) != 1 || !contains(res.Broken, "缺失卡.md") {
		t.Fatalf("断链应记缺失卡: %+v", res.Broken)
	}
	nodes, edges, err := st.ListKnowledgeGraph()
	if err != nil || len(nodes) != 3 || len(edges) != 2 {
		t.Fatalf("投影读取: %v %d %d", err, len(nodes), len(edges))
	}
}

func TestLearningPathClosure(t *testing.T) {
	st, root := graphFixture(t)
	res, _ := IngestKnowledgeGraph(root, "platform-knowledge/09_知识体系", st)
	_ = res
	nodes, edges, _ := st.ListKnowledgeGraph()
	// 目标 c：闭包 = a → b → c（依赖先行）
	path, err := LearningPath(nodes, edges, "platform-knowledge/09_知识体系/01_本体线/cards/c.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 3 ||
		filepath.Base(path[0]) != "a.md" || filepath.Base(path[1]) != "b.md" || filepath.Base(path[2]) != "c.md" {
		t.Fatalf("拓扑序不符: %v", path)
	}
	// 目标 a：单卡
	path2, err := LearningPath(nodes, edges, "platform-knowledge/09_知识体系/01_本体线/cards/a.md")
	if err != nil || len(path2) != 1 {
		t.Fatalf("单卡路径: %v %v", path2, err)
	}
}

func contains(ss []string, sub string) bool {
	for _, s := range ss {
		if len(s) >= len(sub) && (func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})() {
			return true
		}
	}
	return false
}

// TestParseCardTextInlineArray 内联数组形态（现网真实卡形态）——prereq/sources 单行数组可解析。
func TestParseCardTextInlineArray(t *testing.T) {
	text := "---\nkind: decision\ndomain: ontology\ntopic: 决策卡\nprereq: [\"a.md\", \"b.md\"]\nsources:\n  - REQ-372\ncheck:\n  - 问题？\n---\n正文"
	m, err := ParseCardText(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Prereq) != 2 || m.Prereq[0] != "a.md" || m.Prereq[1] != "b.md" {
		t.Fatalf("内联 prereq 应解析为 [a.md b.md]: %+v", m.Prereq)
	}
	if m.Kind != "decision" || m.Topic != "决策卡" {
		t.Fatalf("标量字段不受内联数组解析影响: %+v", m)
	}
	// 空数组与标量 prereq 兜底
	m2, _ := ParseCardText("---\nkind: concept\nprereq: []\n---\n正文")
	if len(m2.Prereq) != 0 {
		t.Fatalf("空内联数组应为零条目: %+v", m2.Prereq)
	}
}

// TestIngestSkipsNonCardDocs 导读/主档（有 frontmatter 无 kind）不入图。
func TestIngestSkipsNonCardDocs(t *testing.T) {
	st, root := graphFixture(t)
	extra := func(rel, content string) {
		p := filepath.Join(root, rel)
		_ = os.WriteFile(p, []byte(content), 0o644)
	}
	extra("00_导读.md", "---\nmodule: 知识体系\nrole: home\ntopic: 导读\n---\n导读正文")
	extra("01_本体线/00_本体线学习路径.md", "---\nmodule: 知识体系\ntopic: 学习路径\n---\n主档正文")
	extra("README.md", "无 frontmatter 文档")
	if _, err := IngestKnowledgeGraph(root, "platform-knowledge/09_知识体系", st); err != nil {
		t.Fatal(err)
	}
	nodes, _, err := st.ListKnowledgeGraph()
	if err != nil || len(nodes) != 3 {
		t.Fatalf("非卡文档应跳过（节点仍 3）: %v %d", err, len(nodes))
	}
	for _, n := range nodes {
		if n.Kind == "" {
			t.Fatalf("空 kind 节点不应入图: %+v", n)
		}
	}
}

// TestIngestNoDoublePrefix root 含 platform-knowledge 段（生产形态）时 id 不得双前缀。
func TestIngestNoDoublePrefix(t *testing.T) {
	st, _ := graphFixture(t)
	root := filepath.Join(t.TempDir(), "platform-knowledge", "09_知识体系")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// 在带前缀 root 下重建 fixture（生产形态：root 路径含 platform-knowledge 段）
	card := func(rel, fm string) {
		p := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(fm), 0o644)
	}
	card("01_本体线/cards/a.md", "---\nkind: concept\ntopic: 概念A\nprereq: []\n---\n正文")
	card("01_本体线/cards/b.md", "---\nkind: decision\ntopic: 决策B\nprereq: [\"a.md\"]\n---\n正文")
	if _, err := IngestKnowledgeGraph(root, "platform-knowledge", st); err != nil {
		t.Fatal(err)
	}
	nodes, edges, err := st.ListKnowledgeGraph()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || len(edges) != 1 {
		t.Fatalf("生产形态 root 应摄取 2 节点 1 边: %d %d", len(nodes), len(edges))
	}
	for _, n := range nodes {
		if strings.Contains(n.ID, "platform-knowledge/platform-knowledge") {
			t.Fatalf("节点 id 双前缀（修复回归）: %s", n.ID)
		}
		want := "platform-knowledge/09_知识体系/01_本体线/cards/" + filepath.Base(n.ID)
		if filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(n.ID)))) != "09_知识体系" {
			t.Fatalf("id 应为 platform-knowledge 前缀单层形态: %s（期望形如 %s）", n.ID, want)
		}
	}
	// 边解析在单前缀 id 空间内成立
	if edges[0].Src == edges[0].Dst || (filepath.Base(edges[0].Src) != "b.md" || filepath.Base(edges[0].Dst) != "a.md") {
		t.Fatalf("prereq 边应为 b→a: %+v", edges[0])
	}
}

// TestCardRelIDSegmentRoot root 以 platform-knowledge/ 开头（KNOWLEDGE_ROOT=./platform-knowledge
// 经 Join 清 ./ 后的形态）时 id 不得丢段——repoRootOf i=0 边界（2026-10-28 冒烟实锤修复回归：
// 全部节点 id 丢 platform-knowledge/09_知识体系 两段、跨线 path 查询不命中）。
func TestCardRelIDSegmentRoot(t *testing.T) {
	root := filepath.ToSlash(filepath.Join("platform-knowledge", "09_知识体系")) // 无父段形态
	path := filepath.ToSlash(filepath.Join(root, "03_结合线", "cards", "x.md"))
	got := cardRelID(root, "platform-knowledge", path)
	want := "platform-knowledge/09_知识体系/03_结合线/cards/x.md"
	if got != want {
		t.Fatalf("单段 root 形态 id 应保留全路径: got %q want %q", got, want)
	}
	// 双段形态（生产常态）回归不变
	root2 := filepath.ToSlash(filepath.Join("..", "platform-knowledge", "09_知识体系"))
	path2 := filepath.ToSlash(filepath.Join(root2, "01_本体线", "cards", "y.md"))
	if got2 := cardRelID(root2, "platform-knowledge", path2); got2 != "platform-knowledge/09_知识体系/01_本体线/cards/y.md" {
		t.Fatalf("双段 root 形态 id 回归: got %q", got2)
	}
}

// TestScanBroken 断链只读扫描（不写库）与摄取断链同口径。
func TestScanBroken(t *testing.T) {
	_, root := graphFixture(t)
	broken, err := ScanBroken(root, "platform-knowledge/09_知识体系")
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) != 1 || !contains(broken, "缺失卡.md") {
		t.Fatalf("ScanBroken 应检出缺失卡断链: %+v", broken)
	}
}
