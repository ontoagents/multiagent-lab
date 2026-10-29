package kb

import (
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// ---- KB-4③/M287：自写 Personalized PageRank 多跳证据收集（HippoRAG 语义）----
//
// 替代常数打分（seed 1.0/邻居 0.6/claim 0.8）：把「逐跳扩展+固定分数」升级为
// 「以种子（向量命中/聚焦实体）为 personalization 的全局证据分布」——图感知排序。
// 纯函数零依赖（gonum 亦无现成 PageRank，35 号调研实锤），教学规模（实体数百~千级）
// 幂迭代 O(iters×E) 足够；35 号 KB-4 草案口径「自写 PPR（gonum 图结构上迭代）」的
// 直读实现：无向传播 + dangling 质量按 personalization 重分布 + L1 收敛。

const (
	pprDamping    = 0.85 // 阻尼系数（PageRank 惯例）
	pprMaxIter    = 30   // 最大迭代轮数（教学规模收敛 <20 轮）
	pprTol        = 1e-6 // L1 收敛阈值
	pprClaimFloor = 0.5  // claim 的 PPR 相对分兜底（subject 未入图时；不再用常数 0.8 伪装图分）
)

// PersonalizedPageRank 在无向图（edges 端点 ∪ 种子）上跑带重启的 PageRank。
//   - nodes：参与排序的实体名全集（检索面已扩展出的实体集）；edges 中不在 nodes 的端点自动并入；
//   - seeds：种子权重 map（向量命中分/聚焦 1.0），非负；全 0/空返回 nil（调用方回落对照口径）；
//   - 返回 map[name]score，Σ=1（浮点容差内）；score 与命中概率成正比，排序用相对分
//     （relativeScores）而非绝对值——PPR 绝对值随图规模摊薄，与向量相似度不同尺度。
func PersonalizedPageRank(nodes []string, edges []*store.KGRelationship, seeds map[string]float64,
	damping float64, maxIter int, tol float64) map[string]float64 {
	if maxIter <= 0 {
		maxIter = pprMaxIter
	}
	if damping <= 0 || damping >= 1 {
		damping = pprDamping
	}
	// 节点集 = nodes ∪ 边端点 ∪ 种子（防御性并入，调用方漏传端点也不丢证据节点）
	idx := map[string]int{}
	names := make([]string, 0, len(nodes)+2*len(edges))
	add := func(n string) {
		if n == "" {
			return
		}
		if _, ok := idx[n]; !ok {
			idx[n] = len(names)
			names = append(names, n)
		}
	}
	for _, n := range nodes {
		add(n)
	}
	for name := range seeds { // 种子防御性并入（调用方漏列端点也不丢证据节点）
		add(name)
	}
	for _, e := range edges {
		add(e.Source)
		add(e.Target)
	}
	if len(names) == 0 {
		return nil
	}
	// personalization 分布（种子权重归一）
	p := make([]float64, len(names))
	total := 0.0
	for n, w := range seeds {
		if i, ok := idx[n]; ok && w > 0 {
			p[i] = w
			total += w
		}
	}
	if total <= 0 {
		return nil
	}
	for i := range p {
		p[i] /= total
	}
	// 无向邻接表（approved 关系双端传播）与度数
	adj := make([][]int, len(names))
	for _, e := range edges {
		s, ok1 := idx[e.Source]
		t, ok2 := idx[e.Target]
		if !ok1 || !ok2 || s == t {
			continue // 端点缺失/自环不传播（自环在 merge 侧已清理，防御性跳过）
		}
		adj[s] = append(adj[s], t)
		adj[t] = append(adj[t], s)
	}
	// 幂迭代：r' = (1-d)·p + d·(Σ_{u→v} r(u)/deg(u)) + d·danglingMass·p
	r := make([]float64, len(names))
	copy(r, p)
	next := make([]float64, len(names))
	for iter := 0; iter < maxIter; iter++ {
		dangling := 0.0
		for i, deg := 0, 0; i < len(names); i++ {
			deg = len(adj[i])
			if deg == 0 {
				dangling += r[i]
			}
		}
		for i := range next {
			next[i] = (1-damping)*p[i] + damping*dangling*p[i]
		}
		for u, nbrs := range adj {
			if len(nbrs) == 0 {
				continue
			}
			share := damping * r[u] / float64(len(nbrs))
			for _, v := range nbrs {
				next[v] += share
			}
		}
		diff := 0.0
		for i := range r {
			d := next[i] - r[i]
			if d < 0 {
				d = -d
			}
			diff += d
			r[i] = next[i]
		}
		if diff < tol {
			break
		}
	}
	out := make(map[string]float64, len(names))
	for i, n := range names {
		out[n] = r[i]
	}
	return out
}

// relativeScores PPR 绝对值转相对分（max 归一到 0~1，最大证据=1.0）：Hits.Score 语义
// 与「实体命中 1.0/0.6」同为 0~1 区间，消费面排序与展示口径不变。
// 空集/最大值非正时返回 nil。
func relativeScores(ppr map[string]float64) map[string]float64 {
	if len(ppr) == 0 {
		return nil
	}
	max := 0.0
	for _, v := range ppr {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		return nil
	}
	out := make(map[string]float64, len(ppr))
	for n, v := range ppr {
		out[n] = v / max
	}
	return out
}
