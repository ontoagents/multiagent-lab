package store

import "encoding/json"

// REQ-372 P2：知识体系 L2 图谱层持久化（迁移 057）——卡为事实源、KG 为投影（单源原则），
// 摄取全量重建（Replace 语义）。承载「前置图自动编排」（学习路径闭包）与断链检测数据源。

// KnowledgeNode 知识卡节点。
type KnowledgeNode struct {
	ID      string   `json:"id"` // 仓库相对路径
	Kind    string   `json:"kind"`
	Domain  string   `json:"domain"`
	Topic   string   `json:"topic"`
	Desc    string   `json:"desc"`
	Sources []string `json:"sources"`
}

// KnowledgeEdge 知识卡边（prereq；kind 预留 cites/derivesFrom）。
type KnowledgeEdge struct {
	Src  string `json:"src"`
	Dst  string `json:"dst"`
	Kind string `json:"kind"`
}

// ReplaceKnowledgeGraph 全量重建投影（删除后批量写入；消费侧短暂全量窗口可接受——单机学习尺度）。
func (s *Store) ReplaceKnowledgeGraph(nodes []KnowledgeNode, edges []KnowledgeEdge) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM knowledge_node`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM knowledge_edge`); err != nil {
		return err
	}
	for _, n := range nodes {
		srcJSON, _ := json.Marshal(n.Sources)
		if _, err := tx.Exec(`INSERT INTO knowledge_node (id, kind, domain, topic, desc, sources) VALUES (?,?,?,?,?,?)`,
			n.ID, n.Kind, n.Domain, n.Topic, n.Desc, srcJSON); err != nil {
			return err
		}
	}
	for _, e := range edges {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO knowledge_edge (src, dst, kind) VALUES (?,?,?)`, e.Src, e.Dst, e.Kind); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListKnowledgeGraph 全量投影读取。
func (s *Store) ListKnowledgeGraph() ([]KnowledgeNode, []KnowledgeEdge, error) {
	rows, err := s.DB.Query(`SELECT id, kind, domain, topic, desc, sources FROM knowledge_node ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	nodes := []KnowledgeNode{}
	for rows.Next() {
		var n KnowledgeNode
		var srcJSON string
		if err := rows.Scan(&n.ID, &n.Kind, &n.Domain, &n.Topic, &n.Desc, &srcJSON); err != nil {
			return nil, nil, err
		}
		n.Sources = []string{}
		_ = json.Unmarshal([]byte(srcJSON), &n.Sources)
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	erows, err := s.DB.Query(`SELECT src, dst, kind FROM knowledge_edge ORDER BY src, dst`)
	if err != nil {
		return nil, nil, err
	}
	defer erows.Close()
	edges := []KnowledgeEdge{}
	for erows.Next() {
		var e KnowledgeEdge
		if err := erows.Scan(&e.Src, &e.Dst, &e.Kind); err != nil {
			return nil, nil, err
		}
		edges = append(edges, e)
	}
	return nodes, edges, erows.Err()
}
