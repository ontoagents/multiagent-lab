package knowledge

// REQ-372 P2：L2 图谱层单测——摄取（frontmatter 投影+prereq 解析+断链）/前置闭包拓扑序。
import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
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
		if len(s) >= len(sub) && (func() bool { for i := 0; i+len(sub) <= len(s); i++ { if s[i:i+len(sub)] == sub { return true } }; return false })() {
			return true
		}
	}
	return false
}
