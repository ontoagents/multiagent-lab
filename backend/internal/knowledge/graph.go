package knowledge

// REQ-372 P2/M171 后续：知识体系 L2 图谱层——摄取（卡 frontmatter → 图谱投影）+前置闭包
// （学习路径自动编排）+断链检测。卡为事实源单源（58 号 §图谱化预留：kind/domain/prereq/sources
// 天然映射为节点属性与边，L1 落卡即按此模型写，L2 摄取零改卡）；KG 为投影（每次摄取全量重建，
// 迁移 057 knowledge_node/knowledge_edge）。
//
// 知识元本体（教学示例）：节点=KnowledgeCard{kind: concept|decision|practice, domain}；
// 边=prereq（学习前置）。语义检索/GraphRAG 问答归 L3（P3 待 L2 验证拍板），本层不涉。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// CardMeta 卡 frontmatter 投影字段（行式 YAML 子集，与校验器 parseFrontmatter 同口径）。
type CardMeta struct {
	ID      string   `json:"id"` // 仓库相对路径
	Kind    string   `json:"kind"`
	Domain  string   `json:"domain"`
	Topic   string   `json:"topic"`
	Desc    string   `json:"desc"`
	Prereq  []string `json:"prereq"`  // 原文（相对卡目录路径或 ../ 形态）
	Sources []string `json:"sources"`
}

// ParseCardText 卡文本 → frontmatter 投影（生产形态；校验器测试版同规则）。
func ParseCardText(text string) (*CardMeta, error) {
	if !strings.HasPrefix(text, "---\n") {
		return nil, fmt.Errorf("缺 frontmatter（须以 --- 开头）")
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, fmt.Errorf("frontmatter 未闭合")
	}
	m := &CardMeta{}
	curKey := ""
	for _, line := range strings.Split(rest[:end], "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(line, "  - ") || strings.HasPrefix(line, "- ") {
			item := strings.TrimPrefix(strings.TrimPrefix(line, "  - "), "- ")
			item = strings.Trim(item, `[]"`)
			if curKey == "prereq" && item != "" {
				m.Prereq = append(m.Prereq, item)
			}
			if curKey == "sources" && item != "" {
				m.Sources = append(m.Sources, item)
			}
			continue
		}
		kv := strings.SplitN(trimmed, ":", 2)
		if len(kv) != 2 {
			continue
		}
		curKey = strings.TrimSpace(kv[0])
		v := strings.Trim(strings.TrimSpace(kv[1]), `[]"`)
		switch curKey {
		case "kind":
			m.Kind = v
		case "domain":
			m.Domain = v
		case "topic":
			m.Topic = v
		case "desc":
			m.Desc = v
		}
	}
	return m, nil
}

// IngestResult 摄取结果。
type IngestResult struct {
	Nodes  int      `json:"nodes"`
	Edges  int      `json:"edges"`
	Broken []string `json:"broken,omitempty"` // prereq 断链（目标卡不存在）
}

// IngestKnowledgeGraph 扫描知识体系目录 → 全量重建图谱投影。
// root=知识体系目录（如 platform-knowledge/09_知识体系）；repoRel=root 的仓库相对前缀（节点 id 用）。
func IngestKnowledgeGraph(root, repoRel string, st *store.Store) (*IngestResult, error) {
	res := &IngestResult{}
	var nodes []store.KnowledgeNode
	edgeSrc := []struct{ src, raw string }{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil // 不可读文件跳过（不阻断摄取）
		}
		meta, perr := ParseCardText(string(b))
		if perr != nil {
			return nil // 非 frontmatter 文档（导读/主档）跳过——主档的 prereq 语义由校验器承载
		}
		rel, rerr := filepath.Rel(repoRootOf(root), path)
		if rerr != nil {
			rel = path
		}
		id := filepath.ToSlash(filepath.Join(repoRel, rel))
		meta.ID = id
		if meta.Topic == "" {
			meta.Topic = strings.TrimSuffix(filepath.Base(path), ".md")
		}
		nodes = append(nodes, store.KnowledgeNode{ID: id, Kind: meta.Kind, Domain: meta.Domain, Topic: meta.Topic, Desc: meta.Desc, Sources: meta.Sources})
		for _, pr := range meta.Prereq {
			edgeSrc = append(edgeSrc, struct{ src, raw string }{id, pr})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	byID := map[string]bool{}
	for _, n := range nodes {
		byID[n.ID] = true
	}
	var edges []store.KnowledgeEdge
	for _, e := range edgeSrc {
		// prereq 原文：相对卡所在目录（cards/xxx.md 形态或 ../ 主档形态）——解析到节点 id
		target := resolveCardPath(e.src, e.raw, byID)
		if target == "" {
			res.Broken = append(res.Broken, e.src+" → "+e.raw)
			continue
		}
		edges = append(edges, store.KnowledgeEdge{Src: e.src, Dst: target, Kind: "prereq"})
	}
	sort.Strings(res.Broken)
	if err := st.ReplaceKnowledgeGraph(nodes, edges); err != nil {
		return nil, err
	}
	res.Nodes, res.Edges = len(nodes), len(edges)
	return res, nil
}

// repoRootOf root 的仓库根上溯（root 形如 <repo>/platform-knowledge/09_知识体系——
// 取 platform-knowledge 前一段作仓库相对基准；兼容直接给仓库相对路径的调用方）。
func repoRootOf(root string) string {
	parts := strings.Split(filepath.ToSlash(root), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "platform-knowledge" && i > 0 {
			return strings.Join(parts[:i], "/")
		}
	}
	return root
}

// resolveCardPath prereq 原文 → 节点 id（相对卡所在目录解析；命中已注册节点即返回）。
func resolveCardPath(srcID, raw string, byID map[string]bool) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// 已是知识体系内相对 id 形态
	if byID[raw] {
		return raw
	}
	dir := filepath.ToSlash(filepath.Dir(srcID))
	cand := filepath.ToSlash(filepath.Join(dir, raw))
	if byID[cand] {
		return cand
	}
	// 形如 "04_TBox与ABox分层.md"（同目录省略路径）已由 Join 覆盖；再尝试仅文件名匹配
	base := filepath.ToSlash(filepath.Join(dir, "cards", raw))
	if byID[base] {
		return base
	}
	return ""
}

// LearningPath 前置闭包拓扑序（学习路径自动编排：目标卡的全部 prereq 传递闭包，依赖先行）。
func LearningPath(nodes []store.KnowledgeNode, edges []store.KnowledgeEdge, target string) ([]string, error) {
	idSet := map[string]bool{}
	for _, n := range nodes {
		idSet[n.ID] = true
	}
	prereqs := map[string][]string{}
	for _, e := range edges {
		if e.Kind != "prereq" {
			continue
		}
		prereqs[e.Src] = append(prereqs[e.Src], e.Dst)
	}
	visited := map[string]bool{}
	var order []string
	var visit func(id string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if !idSet[id] {
			return fmt.Errorf("断链引用 %q（不在图谱中）", id)
		}
		visited[id] = true
		// 先闭包前置（深度优先，字典序稳定）
		ps := append([]string{}, prereqs[id]...)
		sort.Strings(ps)
		for _, p := range ps {
			if err := visit(p); err != nil {
				return err
			}
		}
		order = append(order, id)
		return nil
	}
	if !idSet[target] {
		return nil, fmt.Errorf("目标卡 %q 不在图谱中", target)
	}
	if err := visit(target); err != nil {
		return nil, err
	}
	return order, nil
}
