package companion

// ---------------------------------------------------------------------------
// REQ-313 概念画像（concept profile）检索单元：类节点聚合画像=本名+定义+别名+一阶关系
// +后代（属类闭包）聚合为单节点语料，查询时聚合零新表（P1），注入向量/FTS 语料——
// Snowflake Flattened GraphRAG 借鉴（层级语义扁平化：类名间接词经后代定义可命中类节点）。
// 与 REQ-288 社区摘要互补（社区=拓扑聚类、实体零命中兜底；画像=属类层级聚合、常驻语料）。
// 画像仅增广检索语料（embed textOf/FTS content），渲染面（定义+邻域边）不变——诚实呈现口径不变。
// ---------------------------------------------------------------------------

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// genusRelNames 属类关系名（后代边判定）：与抽取提示词「上下位（属于/是一种）」及
// ScanGenusCandidates 产出（RelName=属于）同口径。
var genusRelNames = map[string]bool{"属于": true, "是一种": true}

// 画像语料治理常量（薄版口径：少而准，防上下文淹没）。
const (
	profileMaxDescendants = 8    // 后代列举上限（属类闭包 BFS）
	profileMaxRelations   = 6    // 一阶关系摘要上限
	profileDefSnippet     = 40   // 涵盖后代定义截断（rune）
	profileMaxRunes       = 1200 // 画像总长上限
)

// profileRelRow 关系行（SelectRelationEdges 投影 src/rel/dst）。
type profileRelRow struct{ Src, Rel, Dst string }

// relEdge 一阶关系邻接行（rel+对端标签）。
type relEdge struct{ rel, other string }

// profileAliasRow 别名行（SelectAliasPairs 投影 owner/alias）。
type profileAliasRow struct{ Owner, Alias string }

// buildConceptProfiles 纯函数：定义投影+关系行+别名行 → 主标签→画像语料。
// 无增广信号（无关系无别名）时与旧语料同形（label+"\n"+definition）——增量行为可预期；
// 仅主标签（defs 键）产画像，别名标签语料维持单独成文档（命中后 resolveHitAliases 回溯）。
func buildConceptProfiles(defs map[string]string, rels []profileRelRow, aliases []profileAliasRow) map[string]string {
	out := make(map[string]string, len(defs))
	if len(defs) == 0 {
		return out
	}
	outgoing := map[string][]relEdge{}
	incoming := map[string][]relEdge{}
	seenEdge := map[string]bool{}
	children := map[string][]string{} // 属类后代邻接（child 属于/是一种 parent）
	seenChild := map[string]bool{}
	for _, r := range rels {
		src, rel, dst := strings.TrimSpace(r.Src), strings.TrimSpace(r.Rel), strings.TrimSpace(r.Dst)
		if src == "" || rel == "" || dst == "" || src == dst {
			continue
		}
		key := src + "\x00" + rel + "\x00" + dst
		if !seenEdge[key] {
			seenEdge[key] = true
			outgoing[src] = append(outgoing[src], relEdge{rel, dst})
			incoming[dst] = append(incoming[dst], relEdge{rel, src})
		}
		if genusRelNames[rel] {
			ck := dst + "\x00" + src
			if !seenChild[ck] {
				seenChild[ck] = true
				children[dst] = append(children[dst], src)
			}
		}
	}
	aliasOf := map[string][]string{}
	for _, a := range aliases {
		owner, alias := strings.TrimSpace(a.Owner), strings.TrimSpace(a.Alias)
		if owner == "" || alias == "" || alias == owner {
			continue
		}
		aliasOf[owner] = append(aliasOf[owner], alias)
	}
	for label, def := range defs {
		var b strings.Builder
		b.WriteString(label)
		if def != "" {
			b.WriteString("\n")
			b.WriteString(def)
		}
		if als := aliasOf[label]; len(als) > 0 {
			fmt.Fprintf(&b, "\n又称 %s", strings.Join(als, "、"))
		}
		// 一阶关系摘要（出边优先入边补位；入向属类边=后代语义归「涵盖」不入摘要）
		var relParts []string
		seenRel := map[string]bool{}
		ordered := append(append([]relEdge{}, outgoing[label]...), incoming[label]...)
		for _, e := range ordered {
			if len(relParts) >= profileMaxRelations {
				break
			}
			key := e.rel + "\x00" + e.other
			if seenRel[key] || e.other == label {
				continue
			}
			seenRel[key] = true
			if containsEdge(outgoing[label], e) {
				relParts = append(relParts, fmt.Sprintf("「%s」→ %s", e.rel, e.other))
			} else {
				if genusRelNames[e.rel] {
					continue // 后代边（child 属于 label）→ 涵盖承载
				}
				relParts = append(relParts, fmt.Sprintf("%s →「%s」", e.other, e.rel))
			}
		}
		if len(relParts) > 0 {
			fmt.Fprintf(&b, "\n关联：%s", strings.Join(relParts, "；"))
		}
		// 后代属类闭包（BFS 防环；后代标签+截断定义——扁平化主信号）
		if desc := descendantsOf(label, children, profileMaxDescendants); len(desc) > 0 {
			parts := make([]string, 0, len(desc))
			for _, d := range desc {
				if dd := defs[d]; dd != "" {
					parts = append(parts, fmt.Sprintf("「%s」（%s）", d, truncateRunes(dd, profileDefSnippet)))
				} else {
					parts = append(parts, fmt.Sprintf("「%s」", d))
				}
			}
			fmt.Fprintf(&b, "\n涵盖：%s", strings.Join(parts, "、"))
		}
		out[label] = truncateRunes(b.String(), profileMaxRunes)
	}
	return out
}

// descendantsOf 属类后代闭包（BFS 防环，限量）。
func descendantsOf(root string, children map[string][]string, limit int) []string {
	seen := map[string]bool{root: true}
	var out []string
	queue := append([]string{}, children[root]...)
	for len(queue) > 0 && len(out) < limit {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		out = append(out, cur)
		queue = append(queue, children[cur]...)
	}
	return out
}

// containsEdge 出边命中判定（方向归属：出边渲染「rel→other」，入边渲染「other→rel」）。
func containsEdge(edges []relEdge, e relEdge) bool {
	for _, x := range edges {
		if x.rel == e.rel && x.other == e.other {
			return true
		}
	}
	return false
}

// truncateRunes rune 安全截断（总长≤n：超限截 n-1 加省略号）。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return string(r[:n-1]) + "…"
}

// parseRelRows SelectRelationEdges 结果 → 关系行（解析失败返回 nil，调用方降级）。
func parseRelRows(raw []byte) []profileRelRow {
	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return nil
	}
	out := make([]profileRelRow, 0, len(res.Results.Bindings))
	for _, b := range res.Results.Bindings {
		out = append(out, profileRelRow{Src: b["src"].Value, Rel: b["rel"].Value, Dst: b["dst"].Value})
	}
	return out
}

// parseAliasPairs SelectAliasPairs 结果 → 别名行。
func parseAliasPairs(raw []byte) []profileAliasRow {
	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return nil
	}
	out := make([]profileAliasRow, 0, len(res.Results.Bindings))
	for _, b := range res.Results.Bindings {
		out = append(out, profileAliasRow{Owner: b["label"].Value, Alias: b["alias"].Value})
	}
	return out
}

// conceptProfiles 概念画像语料（REQ-313）：查询时聚合+进程内缓存（失效随 invalidateLabelCache，
// 与 vecCache/defCache 同生命周期）。关系投影查询失败返回 nil（语料回退旧格式，不阻断召回主链路）；
// 别名投影失败仅降级别名增广（关系/后代画像保留）。
func (s *Service) conceptProfiles(ctx context.Context, ontologyID string, defs map[string]string) map[string]string {
	if len(defs) == 0 {
		return nil
	}
	s.vecMu.Lock()
	if p, ok := s.profileCache[ontologyID]; ok {
		s.vecMu.Unlock()
		return p
	}
	s.vecMu.Unlock()
	raw, err := s.graphQuery(ctx, ontologyID, SelectRelationEdges(ontologyID))
	if err != nil {
		log.Printf("[companion] 画像关系投影查询失败（语料回退旧格式）: %v", err)
		return nil
	}
	rels := parseRelRows(raw)
	var aliases []profileAliasRow
	if araw, aerr := s.graphQuery(ctx, ontologyID, SelectAliasPairs(ontologyID)); aerr == nil {
		aliases = parseAliasPairs(araw)
	}
	p := buildConceptProfiles(defs, rels, aliases)
	s.vecMu.Lock()
	if s.profileCache == nil {
		s.profileCache = map[string]map[string]string{}
	}
	s.profileCache[ontologyID] = p
	s.vecMu.Unlock()
	return p
}
