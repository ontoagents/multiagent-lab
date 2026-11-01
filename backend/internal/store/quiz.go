package store

// 出题·批改·掌握度（REQ-524/M331——Q-23 衍生③ 转正交付，docs/03 §2.71；REQ-572/M380 闭环化）：
// quiz_item 出题记录（REQ-572 扩题型 question_type/options）+ mastery_level 概念级掌握度
// （0.7 旧+0.3 新加权）+ quiz_attempt 批改落档（REQ-572 可审计——每次提交一条）。
// 定位=学习动线闭环承载；衍生② 跨会话记忆后续以此表为数据源（docs/19 池行触发条件）。

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
	// QuestionType REQ-572/M380 题型（short_answer 简答=主观 LLM 批改 | choice 单选 | judge 判断
	// =客观确定性判定——submit 按题型分支，客观题不走 LLM）
	QuestionType string `json:"question_type"`
	// Options 客观题选项（JSON 数组，作答前可见；正确答案存 Answer 列作答后才揭示）
	Options   []string `json:"options"`
	CreatedAt string   `json:"created_at"`
}

// QuizAttempt 一次作答批改记录（REQ-572/M380 批改结果落档可审计——题目/作答/裁决/概念快照，
// 掌握度影响可回溯；沿本体审计叙事）。
type QuizAttempt struct {
	ID           string   `json:"id"`
	OntologyID   string   `json:"ontology_id"`
	QuizItemID   string   `json:"quiz_item_id"`
	QuestionType string   `json:"question_type"`
	Answer       string   `json:"answer"`
	Score        float64  `json:"score"`
	Verdict      string   `json:"verdict"`
	Feedback     string   `json:"feedback"`
	ConceptNames []string `json:"concept_names"`
	CreatedAt    string   `json:"created_at"`
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
	if q.QuestionType == "" {
		q.QuestionType = "short_answer"
	}
	cn, err := json.Marshal(q.ConceptNames)
	if err != nil {
		return nil, err
	}
	op, err := json.Marshal(q.Options)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`INSERT INTO quiz_item (id,ontology_id,concept_names,question,answer,difficulty,coverage_verdict,question_type,options,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		q.ID, q.OntologyID, string(cn), q.Question, q.Answer, q.Difficulty, q.CoverageVerdict, q.QuestionType, string(op), now()); err != nil {
		return nil, err
	}
	return q, nil
}

// UpdateQuizCoverage REQ-545/M352：预检裁决回写（出题后异步预检落库）。
func (s *Store) UpdateQuizCoverage(id, verdict string) error {
	_, err := s.DB.Exec(`UPDATE quiz_item SET coverage_verdict=? WHERE id=?`, verdict, id)
	return err
}

// quizItemCols SELECT 列单源（GetQuizItem/ListQuizItems 同构；REQ-545 爬坑「补列漏一处」防复发）。
const quizItemCols = `id,ontology_id,concept_names,question,answer,difficulty,COALESCE(coverage_verdict, '') AS coverage_verdict,COALESCE(question_type, 'short_answer') AS question_type,COALESCE(options, '[]') AS options,created_at`

func scanQuizItem(scanner interface{ Scan(...any) error }) (*QuizItem, error) {
	q := &QuizItem{}
	var cn, op string
	if err := scanner.Scan(&q.ID, &q.OntologyID, &cn, &q.Question, &q.Answer, &q.Difficulty, &q.CoverageVerdict, &q.QuestionType, &op, &q.CreatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(cn), &q.ConceptNames)
	_ = json.Unmarshal([]byte(op), &q.Options)
	return q, nil
}

// GetQuizItem 按 id 取出题（作答后批改对照用）。
func (s *Store) GetQuizItem(id string) (*QuizItem, error) {
	row := s.DB.QueryRow(`SELECT `+quizItemCols+` FROM quiz_item WHERE id=?`, id)
	q, err := scanQuizItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &HTTPError{Status: 404, Msg: "出题不存在"}
		}
		return nil, err
	}
	return q, nil
}

// ListQuizItems 本体最近出题记录（学习动线历史+REQ-572 出题禁重复数据源）。
func (s *Store) ListQuizItems(ontologyID string, limit int) ([]*QuizItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.Query(`SELECT `+quizItemCols+` FROM quiz_item WHERE ontology_id=? ORDER BY created_at DESC LIMIT ?`, ontologyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*QuizItem{}
	for rows.Next() {
		q, err := scanQuizItem(rows)
		if err != nil {
			return nil, err
		}
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

// InsertQuizAttempt REQ-572/M380：批改结果落档（每次提交一条——主观 LLM 批改与客观确定性判定同管线）。
func (s *Store) InsertQuizAttempt(a *QuizAttempt) (*QuizAttempt, error) {
	if a.ID == "" {
		a.ID = NewID()
	}
	if a.QuestionType == "" {
		a.QuestionType = "short_answer"
	}
	cn, err := json.Marshal(a.ConceptNames)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`INSERT INTO quiz_attempt (id,ontology_id,quiz_item_id,question_type,answer,score,verdict,feedback,concept_names,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.OntologyID, a.QuizItemID, a.QuestionType, a.Answer, a.Score, a.Verdict, a.Feedback, string(cn), now()); err != nil {
		return nil, err
	}
	return a, nil
}

// ListQuizAttempts 本体最近批改记录（可审计面——时间倒序；题目摘要由 handler 侧联 quiz_item）。
func (s *Store) ListQuizAttempts(ontologyID string, limit int) ([]*QuizAttempt, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.Query(`SELECT id,ontology_id,quiz_item_id,question_type,answer,score,verdict,feedback,concept_names,created_at FROM quiz_attempt WHERE ontology_id=? ORDER BY created_at DESC LIMIT ?`, ontologyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*QuizAttempt{}
	for rows.Next() {
		a := &QuizAttempt{}
		var cn string
		if err := rows.Scan(&a.ID, &a.OntologyID, &a.QuizItemID, &a.QuestionType, &a.Answer, &a.Score, &a.Verdict, &a.Feedback, &cn, &a.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(cn), &a.ConceptNames)
		out = append(out, a)
	}
	return out, rows.Err()
}
