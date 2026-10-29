package kb

import (
	"math"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// KB-4③/M287 PPR 单测：纯函数零网络，断言质量守恒/单调性/无向传播/收敛与降级路径。

func relEdges(pairs ...[3]string) []*store.KGRelationship {
	out := make([]*store.KGRelationship, 0, len(pairs))
	for i, p := range pairs {
		out = append(out, &store.KGRelationship{ID: "r" + string(rune('a'+i)), Source: p[0], Target: p[1], Type: p[2]})
	}
	return out
}

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s = %v, want ~%v", name, got, want)
	}
}

// 1) 种子质量守恒：Σ=1；单节点图种子自得满分。
func TestPPRMassConservation(t *testing.T) {
	if got := PersonalizedPageRank([]string{"A"}, nil, map[string]float64{"A": 1}, pprDamping, 0, pprTol); got == nil {
		t.Fatal("single node should rank")
	} else {
		assertClose(t, "A", got["A"], 1)
	}
	// 链 A-B-C，种子 A：分数和守恒
	ppr := PersonalizedPageRank([]string{"A", "B", "C"}, relEdges([3]string{"A", "B", "依赖"}), map[string]float64{"A": 1}, pprDamping, 0, pprTol)
	sum := 0.0
	for _, v := range ppr {
		sum += v
	}
	assertClose(t, "sum", sum, 1)
}

// 2) 链式结构序：种子 A、链 A-B-C → 无向 PPR 稳态 r(B) > r(A) > r(C)。
// 注意种子节点不必然最高分：B 双邻接收 A+C 两路传播，A 只有 restart 注入——
// 这是「全局证据分布」语义（HippoRAG）与逐跳衰减的本质差异，枢纽吸分非缺陷。
// （稳态手推：r(B)=0.85·(r(A)+r(C))，r(C)=0.425·r(B)，r(A)=0.15+0.425·r(B)。）
func TestPPRChainStructure(t *testing.T) {
	ppr := PersonalizedPageRank([]string{"A", "B", "C"}, relEdges(
		[3]string{"A", "B", "r1"}, [3]string{"B", "C", "r2"},
	), map[string]float64{"A": 1}, pprDamping, 0, pprTol)
	if !(ppr["B"] > ppr["A"] && ppr["A"] > ppr["C"] && ppr["C"] > 0) {
		t.Fatalf("chain structure unexpected: A=%v B=%v C=%v", ppr["A"], ppr["B"], ppr["C"])
	}
}

// 3) 对称等分：星形 A-B/A-C/A-D 全对称（同深度同度数），B/C/D 分数严格相等——
// PPR 实现正确性的最强结构检验（任何遍历序偏差都会破坏等分）。
func TestPPRSymmetricEqual(t *testing.T) {
	ppr := PersonalizedPageRank([]string{"A", "B", "C", "D"}, relEdges(
		[3]string{"A", "B", "r1"}, [3]string{"A", "C", "r2"}, [3]string{"A", "D", "r3"},
	), map[string]float64{"A": 1}, pprDamping, 0, pprTol)
	assertClose(t, "B=C", ppr["B"], ppr["C"])
	assertClose(t, "C=D", ppr["C"], ppr["D"])
	assertClose(t, "leaves sum", ppr["B"]+ppr["C"]+ppr["D"], 1-ppr["A"])
}

// 3b) 无向传播与种子权重单调：单边 A-B 时 B 必得分；种子权重高者分高。
func TestPPRUndirectedAndSeedWeight(t *testing.T) {
	edges := relEdges([3]string{"A", "B", "关联"})
	p1 := PersonalizedPageRank([]string{"A", "B"}, edges, map[string]float64{"A": 1}, pprDamping, 0, pprTol)
	if p1["B"] <= 0 {
		t.Fatalf("undirected propagation failed: B=%v", p1["B"])
	}
	p2 := PersonalizedPageRank([]string{"A", "B"}, edges, map[string]float64{"A": 3, "B": 1}, pprDamping, 0, pprTol)
	if !(p2["A"] > p2["B"]) {
		t.Fatalf("seed weight not respected: A=%v B=%v", p2["A"], p2["B"])
	}
}

// 5) dangling（孤立实体无关系边）：质量按 personalization 重分布，守恒不丢；孤立节点得 (1-d)+d·p 份额。
func TestPPRDanglingRedistribution(t *testing.T) {
	ppr := PersonalizedPageRank([]string{"A", "B", "Iso"}, relEdges([3]string{"A", "B", "r1"}),
		map[string]float64{"A": 1, "Iso": 1}, pprDamping, 0, pprTol)
	sum := 0.0
	for _, v := range ppr {
		sum += v
	}
	assertClose(t, "sum", sum, 1)
	if ppr["Iso"] <= 0 {
		t.Fatalf("dangling node lost mass: %+v", ppr)
	}
}

// 6) 降级：空种子/全零种子返回 nil（调用方回落对照口径）；空图返回 nil。
func TestPPRDegenerate(t *testing.T) {
	if got := PersonalizedPageRank([]string{"A"}, nil, nil, pprDamping, 0, pprTol); got != nil {
		t.Fatalf("no seeds should be nil, got %+v", got)
	}
	if got := PersonalizedPageRank([]string{"A"}, nil, map[string]float64{"A": 0}, pprDamping, 0, pprTol); got != nil {
		t.Fatalf("zero seeds should be nil, got %+v", got)
	}
	if got := PersonalizedPageRank(nil, nil, map[string]float64{"X": 1}, pprDamping, 0, pprTol); got == nil {
		// 种子实体不在 nodes：防御性并入后仍可排序（X 为唯一节点）
		t.Fatal("seed outside nodes should still rank via defensive join")
	}
}

// 7) relativeScores：max 归一 0~1；空集 nil。
func TestRelativeScores(t *testing.T) {
	rs := relativeScores(map[string]float64{"A": 0.5, "B": 0.25, "C": 0.25})
	assertClose(t, "A", rs["A"], 1)
	assertClose(t, "B", rs["B"], 0.5)
	if relativeScores(nil) != nil {
		t.Fatal("nil ppr should give nil relative")
	}
	if relativeScores(map[string]float64{"A": 0}) != nil {
		t.Fatal("all-zero ppr should give nil relative")
	}
}
