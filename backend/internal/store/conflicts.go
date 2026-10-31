package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// ---------------------------------------------------------------------------
// REQ-553/M360 方案分歧结构化识别与人工裁决（结构化协作批 P0；107 号 §四）——
// conflict_review 审计存储：识别=平台确定性规则档（submit_review 工具体内评估 / verify.failed
// 事件点评估），裁决=必经人工（resolve 两态：suspended→定向恢复续跑 / pending→回注消息新运行），
// 执行=复用既有挂起-恢复管线。挂起载体仍是 conversation.interrupt_state 单槽，本表存全量审计。
// ---------------------------------------------------------------------------

// ConflictParty 分歧一方（成员/角色/立场摘要）。
type ConflictParty struct {
	AgentID   string `json:"agent_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
	Position  string `json:"position,omitempty"`
}

// ConflictResolution 裁决结果（resolve 两态统一落库；decision_source=manual 审计对齐 approval.granted）。
type ConflictResolution struct {
	Adopt          string `json:"adopt"`                    // a | b | third
	ThirdText      string `json:"third_text,omitempty"`     // 第三案全文（adopt=third 必填）
	Rationale      string `json:"rationale,omitempty"`      // 裁决理由（留痕）
	DecisionSource string `json:"decision_source"`          // manual（v1 裁决必经人工）
	DecidedAt      string `json:"decided_at"`
}

// ConflictReview 分歧裁决审计行。
type ConflictReview struct {
	ID             string             `json:"id"`
	ConversationID string             `json:"conversation_id"`
	RunID          string             `json:"run_id,omitempty"`
	Kind           string             `json:"kind"` // review | event
	RuleID         string             `json:"rule_id,omitempty"`
	Parties        []ConflictParty    `json:"parties"`
	Verdicts       []map[string]any   `json:"verdicts"`
	State          string             `json:"state"` // suspended | pending | resolved | dismissed
	Resolution     *ConflictResolution `json:"resolution,omitempty"`
	CheckpointRef  string             `json:"checkpoint_ref,omitempty"`
	Summary        string             `json:"summary,omitempty"` // 方案对比摘要（裁决卡主文本）
	CreatedAt      string             `json:"created_at"`
	DecidedAt      string             `json:"decided_at,omitempty"`
}

const conflictReviewCols = `id,conversation_id,run_id,kind,rule_id,parties_json,verdicts_json,state,resolution_json,checkpoint_ref,summary,created_at,decided_at`

func scanConflictReview(row interface{ Scan(...any) error }) (*ConflictReview, error) {
	var c ConflictReview
	var parties, verdicts, resolution, decided sql.NullString
	if err := row.Scan(&c.ID, &c.ConversationID, &c.RunID, &c.Kind, &c.RuleID, &parties, &verdicts, &c.State, &resolution, &c.CheckpointRef, &c.Summary, &c.CreatedAt, &decided); err != nil {
		return nil, err
	}
	if parties.Valid && parties.String != "" {
		_ = json.Unmarshal([]byte(parties.String), &c.Parties)
	}
	if verdicts.Valid && verdicts.String != "" {
		_ = json.Unmarshal([]byte(verdicts.String), &c.Verdicts)
	}
	if resolution.Valid && resolution.String != "" {
		var r ConflictResolution
		if json.Unmarshal([]byte(resolution.String), &r) == nil {
			c.Resolution = &r
		}
	}
	if decided.Valid {
		c.DecidedAt = decided.String
	}
	return &c, nil
}

// CreateConflictReview 落分歧审计行（识别时点）。
func (s *Store) CreateConflictReview(c *ConflictReview) error {
	if c.ID == "" {
		c.ID = NewID()
	}
	if c.State == "" {
		c.State = "suspended"
	}
	parties, _ := json.Marshal(c.Parties)
	verdicts, _ := json.Marshal(c.Verdicts)
	_, err := s.DB.Exec(`INSERT INTO conflict_review (id,conversation_id,run_id,kind,rule_id,parties_json,verdicts_json,state,resolution_json,checkpoint_ref,summary,created_at,decided_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.ConversationID, c.RunID, c.Kind, c.RuleID, string(parties), string(verdicts), c.State, "", c.CheckpointRef, c.Summary, now(), nil)
	return err
}

// GetConflictReview 单行。
func (s *Store) GetConflictReview(id string) (*ConflictReview, error) {
	row := s.DB.QueryRow(`SELECT `+conflictReviewCols+` FROM conflict_review WHERE id = ?`, id)
	c, err := scanConflictReview(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

// ListConflictReviews 会话分歧清单（时间降序——审计/回放/裁决卡数据源）。
func (s *Store) ListConflictReviews(convID string) ([]*ConflictReview, error) {
	rows, err := s.DB.Query(`SELECT `+conflictReviewCols+` FROM conflict_review WHERE conversation_id = ? ORDER BY created_at DESC, id DESC`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ConflictReview{}
	for rows.Next() {
		c, err := scanConflictReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ResolveConflictReview 裁决落库（suspended|pending → resolved；state 守卫——终态不可再裁，
// 审计链单向沿 DecideCompanionCandidate 先例）。resolution 由调用方组装（含 DecisionSource/DecidedAt）。
func (s *Store) ResolveConflictReview(id, resolutionJSON string) (*ConflictReview, error) {
	res, err := s.DB.Exec(`UPDATE conflict_review SET state='resolved', resolution_json=?, decided_at=? WHERE id=? AND state IN ('suspended','pending')`,
		resolutionJSON, now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c, gerr := s.GetConflictReview(id)
		if gerr != nil {
			return nil, gerr
		}
		if c == nil {
			return nil, nil
		}
		return c, ErrConflictAlreadyResolved
	}
	return s.GetConflictReview(id)
}

// ErrConflictAlreadyResolved 分歧已裁决（终态守卫）。
var ErrConflictAlreadyResolved = errors.New("分歧已裁决（仅 suspended|pending 可裁决）")

// GetProjectConflictPolicy 项目级冲突规则档原文（''=默认档；v1 直读列——项目 CRUD 面零触碰，
// 编辑 UI/整行暴露随后续轮）。
func (s *Store) GetProjectConflictPolicy(projectID string) string {
	if projectID == "" {
		return ""
	}
	var v sql.NullString
	_ = s.DB.QueryRow(`SELECT conflict_policy FROM project WHERE id = ?`, projectID).Scan(&v)
	return v.String
}
