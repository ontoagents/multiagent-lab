package store

// REQ-287/M92 伴生检索纵深①：BM25/FTS5 兜底臂（复用 KB chunk_fts 基建形态）。
// 伴生实体本体在 Oxigraph（named graph ont-{id}），SQLite 侧无投影——本表是检索专用文本投影
// （label+定义聚合 content，trigram 分词 BM25 排序），companion.RetrievalContext 惰性回填、
// 图写入失效后下次检索重建（与 invalidateLabelCache 同一失效序列，缓存冷启动口径）。
// 查询失败（FTS5 不可用等）由调用方静默降级其余臂（与 kb_fts ftsOK 同口径）。

// CompanionFTSDoc 一条伴生实体文本投影。
type CompanionFTSDoc struct {
	Label   string // 主标签或别名（命中后回查图；别名命中由检索侧回溯主实体）
	Content string // label + definition 聚合文本（BM25 语料）
}

// ReplaceCompanionFTS 全量替换某本体的 FTS 投影（回填原子口径：先删后插同事务）。
func (s *Store) ReplaceCompanionFTS(ontologyID string, docs []CompanionFTSDoc) error {
	if s == nil || s.DB == nil {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM companion_fts WHERE ontology_id=?`, ontologyID); err != nil {
		return err
	}
	for _, d := range docs {
		if _, err := tx.Exec(`INSERT INTO companion_fts(content,label,ontology_id) VALUES(?,?,?)`,
			d.Content, d.Label, ontologyID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CompanionFTSCount 某本体投影行数（0=待回填）。
func (s *Store) CompanionFTSCount(ontologyID string) (int, error) {
	if s == nil || s.DB == nil {
		return 0, nil
	}
	var n int
	err := s.DB.QueryRow(`SELECT count(*) FROM companion_fts WHERE ontology_id=?`, ontologyID).Scan(&n)
	return n, err
}

// CompanionFTSHit 一条 BM25 命中（Score 已取负转正，越大越相关）。
type CompanionFTSHit struct {
	Label string
	Score float64
}

// SearchCompanionFTS BM25 检索（matchQuery 由调用方经 FTSMatchQuery 生成，防注入口径同 KB）。
func (s *Store) SearchCompanionFTS(ontologyID, matchQuery string, limit int) ([]CompanionFTSHit, error) {
	if s == nil || s.DB == nil {
		return nil, nil
	}
	rows, err := s.DB.Query(`SELECT label, -bm25(companion_fts) FROM companion_fts
		WHERE companion_fts MATCH ? AND ontology_id=? ORDER BY 2 DESC LIMIT ?`,
		matchQuery, ontologyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CompanionFTSHit{}
	for rows.Next() {
		var h CompanionFTSHit
		if err := rows.Scan(&h.Label, &h.Score); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
