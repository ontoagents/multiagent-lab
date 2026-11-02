package store

// C4 Evaluator 落档（REQ-388/M387——运行质量评估器，复用 REQ-223 Judge 契约不开新面）：
// run_evaluation 每次评估一条，评估时点对话末轮 goal/answer 快照+工具轨迹+Judge 三值结果。

import (
	"encoding/json"
	"strings"
)

// RunEvaluation 一次运行质量评估记录。
type RunEvaluation struct {
	ID             string   `json:"id"`
	RunID          string   `json:"run_id"`
	ConversationID string   `json:"conversation_id"`
	Goal           string   `json:"goal"`
	Answer         string   `json:"answer"`
	Score          int      `json:"score"` // 2=pass 1=partial 0=fail（Judge 三值）
	Verdict        string   `json:"verdict"`
	Reason         string   `json:"reason"`
	Tools          []string `json:"tools"`
	CreatedAt      string   `json:"created_at"`
}

// InsertRunEvaluation 保存一条评估。
func (s *Store) InsertRunEvaluation(e *RunEvaluation) (*RunEvaluation, error) {
	if e.ID == "" {
		e.ID = NewID()
	}
	tj, err := json.Marshal(e.Tools)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(`INSERT INTO run_evaluation (id,run_id,conversation_id,goal,answer,score,verdict,reason,tools,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.RunID, e.ConversationID, e.Goal, e.Answer, e.Score, e.Verdict, e.Reason, string(tj), now()); err != nil {
		return nil, err
	}
	return e, nil
}

// ListRunEvaluations 某 run 的评估历史（时间倒序）。
func (s *Store) ListRunEvaluations(runID string, limit int) ([]*RunEvaluation, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.Query(`SELECT id,run_id,conversation_id,goal,answer,score,verdict,reason,tools,created_at FROM run_evaluation WHERE run_id=? ORDER BY created_at DESC, id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*RunEvaluation{}
	for rows.Next() {
		e := &RunEvaluation{}
		var tj string
		if err := rows.Scan(&e.ID, &e.RunID, &e.ConversationID, &e.Goal, &e.Answer, &e.Score, &e.Verdict, &e.Reason, &tj, &e.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tj), &e.Tools)
		out = append(out, e)
	}
	return out, rows.Err()
}

// LastUserAndAssistantMessages 对话末轮评估输入（goal=最后 user、answer=最后 assistant）。
// C4 诚实口径：按对话末轮评估（run 粒度 user/assistant 关联无 run_id 列——沿表结构不加列）。
func (s *Store) LastUserAndAssistantMessages(convID string) (goal, answer string) {
	rows, err := s.DB.Query(`SELECT role,content FROM message WHERE conversation_id=? AND role IN ('user','assistant') ORDER BY created_at DESC, id DESC LIMIT 20`, convID)
	if err != nil {
		return "", ""
	}
	defer rows.Close()
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			continue
		}
		if role == "user" && goal == "" {
			goal = strings.TrimSpace(content)
		}
		if role == "assistant" && answer == "" {
			answer = strings.TrimSpace(content)
		}
		if goal != "" && answer != "" {
			break
		}
	}
	return goal, answer
}
