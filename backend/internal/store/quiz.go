package store

// 出题·批改·掌握度（REQ-524/M331——Q-23 衍生③ 转正交付，docs/03 §2.71）：
// quiz_item 出题记录 + mastery_level 概念级掌握度（0.7 旧+0.3 新加权）。
// 定位=学习动线最小闭环承载；衍生② 跨会话记忆后续以此表为数据源（docs/19 池行触发条件）。

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// QuizItem 一道出题（本体概念子图——概念快照+参考答案+依据）。
type QuizItem struct {
	ID           string   `json:"id"`
	OntologyID   string   `json:"ontology_id"`
	ConceptNames []string `json:"concept_names"`
	Question     string   `json:"question"`
	Answer       string   `json:"answer"`     // 参考答案+依据（批改对照，前端作答后才展示）
	Difficulty   string   `json:"difficulty"` // basic | applied
	// CoverageVerdict REQ-545/M352 出题预检裁决（''=未预检 | answerable | not_answerable——
	// check_answerable 确定性预检随出题落库，QuizPane 可答/存疑徽标数据源）
	CoverageVerdict string `json:"coverage_verdict"`
	CreatedAt       string `json:"created_at"`
}

// MasteryLevel 概念级掌握度（asked/correct 累计+score 最近加权 0~1）。
type MasteryLevel struct {
	ConceptName  string  `json:"concept_name"`
	AskedCount   int     `json:"asked_count"`
	CorrectCount int     `json:"correct_count"`
	Score        float64 `json:"score"`
	UpdatedAt    string  `json:"updated_at"`
}

// InsertQuizItem 保存一道出题。
func (s *Store) InsertQuizItem(q *QuizItem) (*QuizItem, error) {
	if q.ID == "" {
		q.ID = NewID()
	}
	cn, err := json.Marshal(q.ConceptNames)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`INSERT INTO quiz_item (id,ontology_id,concept_names,question,answer,difficulty,coverage_verdict,created_at) VALUES (?,?,?,?,?,?,?,?)`,
		q.ID, q.OntologyID, string(cn), q.Question, q.Answer, q.Difficulty, q.CoverageVerdict, now()); err != nil {
		return nil, err
	}
	return q, nil
}

// UpdateQuizCoverage REQ-545/M352：预检裁决回写（出题后异步预检落库）。
func (s *Store) UpdateQuizCoverage(id, verdict string) error {
	_, err := s.DB.Exec(`UPDATE quiz_item SET coverage_verdict=? WHERE id=?`, verdict, id)
	return err
}

// GetQuizItem 按 id 取出题（作答后批改对照用）。
func (s *Store) GetQuizItem(id string) (*QuizItem, error) {
	row := s.DB.QueryRow(`SELECT id,ontology_id,concept_names,question,answer,difficulty,COALESCE(coverage_verdict, '') AS coverage_verdict, created_at FROM quiz_item WHERE id=?`, id)
	q := &QuizItem{}
	var cn string
	if err := row.Scan(&q.ID, &q.OntologyID, &cn, &q.Question, &q.Answer, &q.Difficulty, &q.CoverageVerdict, &q.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &HTTPError{Status: 404, Msg: "出题不存在"}
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(cn), &q.ConceptNames)
	return q, nil
}

// ListQuizItems 本体最近出题记录（学习动线历史）。
func (s *Store) ListQuizItems(ontologyID string, limit int) ([]*QuizItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.Query(`SELECT id,ontology_id,concept_names,question,answer,difficulty,COALESCE(coverage_verdict, '') AS coverage_verdict, created_at FROM quiz_item WHERE ontology_id=? ORDER BY created_at DESC LIMIT ?`, ontologyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*QuizItem{}
	for rows.Next() {
		q := &QuizItem{}
		var cn string
		if err := rows.Scan(&q.ID, &q.OntologyID, &cn, &q.Question, &q.Answer, &q.Difficulty, &q.CoverageVerdict, &q.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(cn), &q.ConceptNames)
		out = append(out, q)
	}
	return out, rows.Err()
}

// UpsertMastery 批改后更新概念掌握度：asked/correct 累计+score 0.7 旧+0.3 新加权（初值=本次分）。
func (s *Store) UpsertMastery(ontologyID, conceptName string, correct bool, score float64) error {
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	var id string
	var asked, correctN int
	var oldScore float64
	err = tx.QueryRow(`SELECT id,asked_count,correct_count,score FROM mastery_level WHERE ontology_id=? AND concept_name=?`, ontologyID, conceptName).Scan(&id, &asked, &correctN, &oldScore)
	if errors.Is(err, sql.ErrNoRows) {
		newScore := score
		cc := 0
		if correct {
			cc = 1
		}
		if _, err := tx.Exec(`INSERT INTO mastery_level (id,ontology_id,concept_name,asked_count,correct_count,score,updated_at) VALUES (?,?,?,?,?,?,?)`,
			NewID(), ontologyID, conceptName, 1, cc, newScore, now()); err != nil {
			tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	cc := correctN
	if correct {
		cc = correctN + 1
	}
	newScore := 0.7*oldScore + 0.3*score
	if _, err := tx.Exec(`UPDATE mastery_level SET asked_count=?,correct_count=?,score=?,updated_at=? WHERE id=?`,
		asked+1, cc, newScore, now(), id); err != nil {
		tx.Rollback()
		return fmt.Errorf("update mastery: %w", err)
	}
	return tx.Commit()
}

// ListMastery 本体全部概念掌握度（score 降序——薄弱概念排前）。
func (s *Store) ListMastery(ontologyID string) ([]*MasteryLevel, error) {
	rows, err := s.DB.Query(`SELECT concept_name,asked_count,correct_count,score,updated_at FROM mastery_level WHERE ontology_id=? ORDER BY score ASC`, ontologyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*MasteryLevel{}
	for rows.Next() {
		m := &MasteryLevel{}
		if err := rows.Scan(&m.ConceptName, &m.AskedCount, &m.CorrectCount, &m.Score, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
