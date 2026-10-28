package store

// REQ-224 H10/60 号搭车件：本体消费贡献度量。
// onto_* facade 工具（source=ontology:ontology:facade 装配标注）在 chat 层 tool.result 到达时
// 逐次落 ontology_hit（迁移 055），并沿 companion.hit 先例发 ontology.hit run_event；
// 本档承载逐次落行与两类聚合（单本体画像 / 全局概览），消费侧入质量卡与消费审计栏。

import (
	"strings"
)

// OntologyHit 一次本体工具消费命中。
type OntologyHit struct {
	ID             string `json:"id"`
	OntologyID     string `json:"ontology_id"`
	ConversationID string `json:"conversation_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	ProfileID      string `json:"profile_id,omitempty"` // REQ-482/M282：方案归因（facade 工具结果 scope 块透出）
	Tool           string `json:"tool"`
	Target         string `json:"target,omitempty"` // 概念/实例名或 SPARQL 首行（截断）
	OK             bool   `json:"ok"`
	CreatedAt      string `json:"created_at"`
}

// InsertOntologyHit 落一次命中（失败仅日志——度量旁路不阻断对话主链）。
func (s *Store) InsertOntologyHit(h *OntologyHit) error {
	if h.ID == "" {
		h.ID = NewID()
	}
	_, err := s.DB.Exec(`INSERT INTO ontology_hit (id, ontology_id, conversation_id, run_id, profile_id, tool, target, ok, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		h.ID, h.OntologyID, h.ConversationID, h.RunID, h.ProfileID, h.Tool, h.Target, boolInt(h.OK), now())
	return err
}

// OntologyRunGroup 组维度行（REQ-482/M282④）：一次回答（run）命中的本体集合与方案归因
// ——消费观测页「一次回答命中哪些本体」数据源。
type OntologyRunGroup struct {
	RunID      string   `json:"run_id"`
	ProfileID  string   `json:"profile_id,omitempty"`
	Hits       int      `json:"hits"`
	LastAt     string   `json:"last_hit_at,omitempty"`
	Ontologies []string `json:"ontologies"`
}

// OntologyRunGroups 最近 N 次回答的本体命中组（按 run 聚合，倒序）。
func (s *Store) OntologyRunGroups(limit int) ([]*OntologyRunGroup, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := s.DB.Query(`SELECT run_id, COALESCE(profile_id,''), COUNT(*), COALESCE(MAX(created_at),''),
		GROUP_CONCAT(DISTINCT ontology_id)
		FROM ontology_hit WHERE run_id != '' GROUP BY run_id ORDER BY MAX(created_at) DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*OntologyRunGroup
	for rows.Next() {
		var g OntologyRunGroup
		var joined string
		if err := rows.Scan(&g.RunID, &g.ProfileID, &g.Hits, &g.LastAt, &joined); err != nil {
			return nil, err
		}
		for _, o := range strings.Split(joined, ",") {
			if o = strings.TrimSpace(o); o != "" {
				g.Ontologies = append(g.Ontologies, o)
			}
		}
		out = append(out, &g)
	}
	return out, rows.Err()
}

// OntologyToolStat 工具维度计数。
type OntologyToolStat struct {
	Tool  string `json:"tool"`
	Count int    `json:"count"`
}

// OntologyTargetStat 被引用概念/实例 TopN 行。
type OntologyTargetStat struct {
	Target string `json:"target"`
	Count  int    `json:"count"`
}

// OntologyContribution 单本体消费贡献画像（质量卡/消费审计栏数据源）。
type OntologyContribution struct {
	OntologyID string               `json:"ontology_id"`
	TotalHits  int                  `json:"total_hits"`
	OKHits     int                  `json:"ok_hits"`
	FirstHitAt string               `json:"first_hit_at,omitempty"`
	LastHitAt  string               `json:"last_hit_at,omitempty"`
	ByTool     []OntologyToolStat   `json:"by_tool"`
	TopTargets []OntologyTargetStat `json:"top_targets"`
	Recent     []OntologyHit        `json:"recent"`
}

// OntologyContribution 聚合单本体：命中次数、按工具分布、被引用概念 TopN（仅成功命中计）、最近 20 条。
func (s *Store) OntologyContribution(ontologyID string, topN int) (*OntologyContribution, error) {
	if topN <= 0 || topN > 50 {
		topN = 10
	}
	c := &OntologyContribution{OntologyID: ontologyID}
	if err := s.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(ok),0), COALESCE(MIN(created_at),''), COALESCE(MAX(created_at),'')
		FROM ontology_hit WHERE ontology_id = ?`, ontologyID).
		Scan(&c.TotalHits, &c.OKHits, &c.FirstHitAt, &c.LastHitAt); err != nil {
		return nil, err
	}
	if c.TotalHits == 0 {
		return c, nil
	}
	rows, err := s.DB.Query(`SELECT tool, COUNT(*) FROM ontology_hit WHERE ontology_id = ? GROUP BY tool ORDER BY COUNT(*) DESC`, ontologyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t OntologyToolStat
		if err := rows.Scan(&t.Tool, &t.Count); err != nil {
			return nil, err
		}
		c.ByTool = append(c.ByTool, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	trows, err := s.DB.Query(`SELECT target, COUNT(*) FROM ontology_hit WHERE ontology_id = ? AND ok = 1 AND target != ''
		GROUP BY target ORDER BY COUNT(*) DESC, target LIMIT ?`, ontologyID, topN)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var t OntologyTargetStat
		if err := trows.Scan(&t.Target, &t.Count); err != nil {
			return nil, err
		}
		c.TopTargets = append(c.TopTargets, t)
	}
	if err := trows.Err(); err != nil {
		return nil, err
	}
	recent, err := s.ontologyHitRecent(`WHERE ontology_id = ?`, ontologyID, 20)
	if err != nil {
		return nil, err
	}
	c.Recent = recent
	return c, nil
}

// OntologyContributionSummaryRow 全局概览行（消费审计栏一览）。
type OntologyContributionSummaryRow struct {
	OntologyID string `json:"ontology_id"`
	TotalHits  int    `json:"total_hits"`
	OKHits     int    `json:"ok_hits"`
	LastHitAt  string `json:"last_hit_at,omitempty"`
	LastTool   string `json:"last_tool,omitempty"`
}

// OntologyContributionSummary 跨本体命中概览（倒序按命中数）。
func (s *Store) OntologyContributionSummary(limit int) ([]*OntologyContributionSummaryRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.Query(`SELECT ontology_id, COUNT(*) total, COALESCE(SUM(ok),0), COALESCE(MAX(created_at),''),
		COALESCE((SELECT tool FROM ontology_hit h2 WHERE h2.ontology_id = h1.ontology_id ORDER BY created_at DESC LIMIT 1), '')
		FROM ontology_hit h1 GROUP BY ontology_id ORDER BY total DESC, ontology_id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*OntologyContributionSummaryRow
	for rows.Next() {
		var r OntologyContributionSummaryRow
		if err := rows.Scan(&r.OntologyID, &r.TotalHits, &r.OKHits, &r.LastHitAt, &r.LastTool); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// ontologyHitRecent 最近命中明细（cond/args 由调用方拼装——两处聚合共用）。
func (s *Store) ontologyHitRecent(where, arg string, limit int) ([]OntologyHit, error) {
	rows, err := s.DB.Query(`SELECT id, ontology_id, conversation_id, run_id, profile_id, tool, target, ok, created_at
		FROM ontology_hit `+where+` ORDER BY created_at DESC LIMIT ?`, arg, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OntologyHit
	for rows.Next() {
		var h OntologyHit
		var ok int
		if err := rows.Scan(&h.ID, &h.OntologyID, &h.ConversationID, &h.RunID, &h.ProfileID, &h.Tool, &h.Target, &ok, &h.CreatedAt); err != nil {
			return nil, err
		}
		h.OK = ok == 1
		out = append(out, h)
	}
	return out, rows.Err()
}
