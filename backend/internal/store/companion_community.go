package store

// REQ-288/M93 伴生社区摘要存储（对齐 kg_community 形制；表结构见迁移 045）。

import (
	"encoding/json"
	"sort"
)

// CompanionCommunity 伴生图单社区（label=成员字典序最小实体名或 __tail__）。
type CompanionCommunity struct {
	OntologyID string   `json:"ontology_id"`
	Label      string   `json:"label"`
	Summary    string   `json:"summary"`
	Method     string   `json:"method"` // llm | skeleton | skeleton+tail | pending
	Members    []string `json:"members"`
	CreatedAt  string   `json:"created_at,omitempty"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
}

// ListCompanionCommunities 某本体的社区清单（成员数降序、label 升序——沿 kg 排序口径）。
func (s *Store) ListCompanionCommunities(ontologyID string) ([]*CompanionCommunity, error) {
	if s == nil || s.DB == nil {
		return nil, nil
	}
	rows, err := s.DB.Query(`SELECT ontology_id,label,summary,method,members_json,created_at,updated_at
		FROM companion_community WHERE ontology_id=?`, ontologyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*CompanionCommunity{}
	for rows.Next() {
		c := &CompanionCommunity{}
		var members string
		if err := rows.Scan(&c.OntologyID, &c.Label, &c.Summary, &c.Method, &members, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(members), &c.Members)
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Members) != len(out[j].Members) {
			return len(out[i].Members) > len(out[j].Members)
		}
		return out[i].Label < out[j].Label
	})
	return out, rows.Err()
}

// ReplaceCompanionCommunities 全量替换（重建语义）；成员清单未变的社区保留既有
// summary/method（成员不变不重烧 LLM——成本口径沿 KB-O4）。返回保留复用的社区数。
func (s *Store) ReplaceCompanionCommunities(ontologyID string, next []*CompanionCommunity) (int, error) {
	if s == nil || s.DB == nil {
		return 0, nil
	}
	prev, err := s.ListCompanionCommunities(ontologyID)
	if err != nil {
		return 0, err
	}
	key := func(members []string) string {
		cp := append([]string{}, members...)
		sort.Strings(cp)
		b, _ := json.Marshal(cp)
		return string(b)
	}
	prevByKey := map[string]*CompanionCommunity{}
	for _, p := range prev {
		prevByKey[p.Label+"|"+key(p.Members)] = p
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM companion_community WHERE ontology_id=?`, ontologyID); err != nil {
		return 0, err
	}
	reused := 0
	for _, c := range next {
		members, _ := json.Marshal(c.Members)
		if p, ok := prevByKey[c.Label+"|"+key(c.Members)]; ok && p.Summary != "" {
			c.Summary, c.Method = p.Summary, p.Method
			reused++
		}
		if _, err := tx.Exec(`INSERT INTO companion_community(ontology_id,label,summary,method,members_json)
			VALUES(?,?,?,?,?)`, ontologyID, c.Label, c.Summary, c.Method, string(members)); err != nil {
			return reused, err
		}
	}
	return reused, tx.Commit()
}

// UpdateCompanionCommunitySummary 按需摘要回填（pending → llm/skeleton；首触发成本口径）。
func (s *Store) UpdateCompanionCommunitySummary(ontologyID, label, summary, method string) error {
	if s == nil || s.DB == nil {
		return nil
	}
	_, err := s.DB.Exec(`UPDATE companion_community SET summary=?, method=?, updated_at=datetime('now')
		WHERE ontology_id=? AND label=?`, summary, method, ontologyID, label)
	return err
}
