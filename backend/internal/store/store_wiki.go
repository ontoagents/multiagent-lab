// REQ-241 知识库 LLM Wiki 类型（M67）：wiki_page 页面存储。
// 重建全量替换（沿 kg_community 语义 = 缓存失效）；sources_json=溯源 chunk_id 数组。
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// WikiPage LLM 摄取生成的 wiki 页面（五类：summary/entity/concept/topic/synthesis/index）。
type WikiPage struct {
	ID          string   `json:"id"`
	KBID        string   `json:"kb_id"`
	PageType    string   `json:"page_type"` // summary | entity | concept | topic | synthesis | index
	Title       string   `json:"title"`
	ContentMD   string   `json:"content_md"`
	Sources     []string `json:"sources"` // 溯源 chunk_id（页 → chunk 池下钻原文）
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

const wikiPageCols = `id,kb_id,page_type,title,content_md,sources_json,created_at,updated_at`

func scanWikiPage(row interface{ Scan(...any) error }) (*WikiPage, error) {
	var p WikiPage
	var src string
	if err := row.Scan(&p.ID, &p.KBID, &p.PageType, &p.Title, &p.ContentMD, &src, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(src), &p.Sources) // 损坏容错：空数组
	if p.Sources == nil {
		p.Sources = []string{}
	}
	return &p, nil
}

// SetDocWikiHash 回写文档摄取指纹（wiki 重建跳过判定）。
func (s *Store) SetDocWikiHash(kbID, docID, hash string) error {
	_, err := s.DB.Exec(`UPDATE knowledge_doc SET wiki_hash=?,updated_at=? WHERE id=? AND kb_id=?`,
		hash, now(), docID, kbID)
	return err
}

// ReplaceWikiPages 全量替换某库的页面（重建语义；沿 kg_community）。
func (s *Store) ReplaceWikiPages(kbID string, pages []*WikiPage) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM wiki_page WHERE kb_id = ?`, kbID); err != nil {
		return err
	}
	for _, p := range pages {
		p.KBID = kbID // 服务端权威回填（防调用方漏填静默失联）
		if p.ID == "" {
			p.ID = NewID()
		}
		src, err := json.Marshal(p.Sources)
		if err != nil {
			src = []byte("[]")
		}
		if _, err := tx.Exec(`INSERT INTO wiki_page (`+wikiPageCols+`) VALUES (?,?,?,?,?,?,?,?)`,
			p.ID, p.KBID, p.PageType, p.Title, p.ContentMD, string(src), now(), now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListWikiPages 某库全部页面（index 页在前，其余按类型+标题稳定排序）。
func (s *Store) ListWikiPages(kbID string) ([]*WikiPage, error) {
	rows, err := s.DB.Query(`SELECT `+wikiPageCols+` FROM wiki_page WHERE kb_id = ?
		ORDER BY CASE page_type WHEN 'index' THEN 0 WHEN 'summary' THEN 1 WHEN 'entity' THEN 2 WHEN 'concept' THEN 3 WHEN 'topic' THEN 4 ELSE 5 END, title, id`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*WikiPage{}
	for rows.Next() {
		p, err := scanWikiPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteWikiPagesByKB 删库级联。
func (s *Store) DeleteWikiPagesByKB(kbID string) error {
	_, err := s.DB.Exec(`DELETE FROM wiki_page WHERE kb_id = ?`, kbID)
	return err
}

// DeleteWikiSummaryPage 删除文档时移除其 summary 页（页标题 = 文档标题；聚合页留待重建收敛）。
func (s *Store) DeleteWikiSummaryPage(kbID, title string) error {
	_, err := s.DB.Exec(`DELETE FROM wiki_page WHERE kb_id = ? AND page_type='summary' AND title = ?`, kbID, title)
	return err
}

// GetWikiPage 单页（404 用 ErrNotFound；预览/下钻用，列表接口已带全文，当前无调用方——预留）。
func (s *Store) GetWikiPage(kbID, id string) (*WikiPage, error) {
	p, err := scanWikiPage(s.DB.QueryRow(`SELECT `+wikiPageCols+` FROM wiki_page WHERE kb_id = ? AND id = ?`, kbID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}
