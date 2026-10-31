package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// ---------------------------------------------------------------------------
// REQ-546/M353 learning proposal 暂存区（决策结构化借鉴批③；105 号 P1-5）——
// 伴生晋升「提案→暂存→人批→入库」显式化：promote 改产 pending 提案（原 REQ-529 直接
// PUT 退役），approve 裁决执行入库（重取 digest 取交集防伴生图漂移），reject 留痕
// （decided_note 即 reject route 素材，衔接 REQ-544 decisions[]）。形态沿
// companion_candidate：pending→approved|rejected 单向审计链，终态不可再裁。
// ---------------------------------------------------------------------------

// PromotionPayload 提案快照（伴生图摘要切片——发起时点冻结）。
type PromotionPayload struct {
	Entities  []string    `json:"entities"`            // 实体 label 清单（去重升序）
	Relations [][3]string `json:"relations,omitempty"` // [subjectLabel, relName, objectLabel]
}

// PromotionProposal 晋升提案（learning proposal 暂存行）。
type PromotionProposal struct {
	ID               string `json:"id"`
	SourceOntologyID string `json:"source_ontology_id"` // 伴生本体（提案来源）
	TargetOntologyID string `json:"target_ontology_id"` // 晋升目标本体
	// PayloadJSON 提案快照原文（列存）；读侧解析为 Payload 透出（写侧入参同构）。
	PayloadJSON   string           `json:"-"`
	Payload       PromotionPayload `json:"payload"`
	EntityCount   int              `json:"entity_count"`
	RelationCount int              `json:"relation_count"`
	Note          string           `json:"note,omitempty"` // 发起注记（为什么晋升——提案理由）
	Status        string           `json:"status"`         // pending | approved | rejected
	// DecidedNote 裁决注记：拒绝理由留痕（reject route 素材，衔接 REQ-544）；
	// 批准时可空。
	DecidedNote string `json:"decided_note,omitempty"`
	CreatedAt   string `json:"created_at"`
	DecidedAt   string `json:"decided_at,omitempty"`
}

// ErrPromotionAlreadyDecided 提案终态后再次裁决（pending 守卫——审计链单向）。
var ErrPromotionAlreadyDecided = errors.New("提案已裁决（仅 pending 可裁决）")

const promotionProposalCols = `id,source_ontology_id,target_ontology_id,payload_json,entity_count,relation_count,note,status,decided_note,created_at,decided_at`

func scanPromotionProposal(row interface{ Scan(...any) error }) (*PromotionProposal, error) {
	var p PromotionProposal
	var decided, decidedNote, note sql.NullString
	if err := row.Scan(&p.ID, &p.SourceOntologyID, &p.TargetOntologyID, &p.PayloadJSON, &p.EntityCount, &p.RelationCount, &note, &p.Status, &decidedNote, &p.CreatedAt, &decided); err != nil {
		return nil, err
	}
	p.Note, p.DecidedNote = note.String, decidedNote.String
	if decided.Valid {
		p.DecidedAt = decided.String
	}
	if p.PayloadJSON != "" {
		_ = json.Unmarshal([]byte(p.PayloadJSON), &p.Payload) // 快照解析失败按空提案透出（诚实不虚构）
	}
	return &p, nil
}

// CreatePromotionProposal 落 pending 提案（发起时点快照冻结）。
func (s *Store) CreatePromotionProposal(p *PromotionProposal) error {
	if p.ID == "" {
		p.ID = NewID()
	}
	if p.Status == "" {
		p.Status = "pending"
	}
	if p.PayloadJSON == "" {
		b, err := json.Marshal(&p.Payload)
		if err != nil {
			return err
		}
		p.PayloadJSON = string(b)
	}
	if p.EntityCount == 0 {
		p.EntityCount = len(p.Payload.Entities)
	}
	if p.RelationCount == 0 {
		p.RelationCount = len(p.Payload.Relations)
	}
	_, err := s.DB.Exec(`INSERT INTO promotion_proposal (id,source_ontology_id,target_ontology_id,payload_json,entity_count,relation_count,note,status,decided_note,created_at,decided_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.SourceOntologyID, p.TargetOntologyID, p.PayloadJSON, p.EntityCount, p.RelationCount, p.Note, p.Status, "", now(), nil)
	return err
}

// GetPromotionProposal 单提案。
func (s *Store) GetPromotionProposal(id string) (*PromotionProposal, error) {
	row := s.DB.QueryRow(`SELECT `+promotionProposalCols+` FROM promotion_proposal WHERE id = ?`, id)
	p, err := scanPromotionProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// ListPromotionProposals 提案列表（可按来源/目标本体/状态过滤；时间降序——待裁在顶）。
func (s *Store) ListPromotionProposals(sourceOntologyID, targetOntologyID, status string) ([]*PromotionProposal, error) {
	q := `SELECT ` + promotionProposalCols + ` FROM promotion_proposal WHERE 1=1`
	var args []any
	if sourceOntologyID != "" {
		q += ` AND source_ontology_id = ?`
		args = append(args, sourceOntologyID)
	}
	if targetOntologyID != "" {
		q += ` AND target_ontology_id = ?`
		args = append(args, targetOntologyID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC, id DESC`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PromotionProposal
	for rows.Next() {
		p, err := scanPromotionProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DecidePromotionProposal 裁决提案（approve|reject 二态；pending 守卫——终态不可再裁，
// 审计链单向沿 DecideCompanionCandidate 先例）。返回裁决后的提案（未找到返回 nil）。
func (s *Store) DecidePromotionProposal(id, status, decidedNote string) (*PromotionProposal, error) {
	if status != "approved" && status != "rejected" {
		return nil, errors.New("裁决状态仅 approved|rejected")
	}
	res, err := s.DB.Exec(`UPDATE promotion_proposal SET status=?, decided_note=?, decided_at=? WHERE id=? AND status='pending'`,
		status, decidedNote, now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// 区分「不存在」与「已裁决」——读侧判定
		p, gerr := s.GetPromotionProposal(id)
		if gerr != nil {
			return nil, gerr
		}
		if p == nil {
			return nil, nil
		}
		return p, ErrPromotionAlreadyDecided
	}
	return s.GetPromotionProposal(id)
}
