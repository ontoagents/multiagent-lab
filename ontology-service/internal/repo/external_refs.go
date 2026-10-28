package repo

// REQ-262/M256：跨本体引用登记（external_refs 单源，D-O27 三形态收敛）——
// store 层 CRUD+存在性校验+悬空检测（qualitygate 消费）。

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ExternalRef 跨本体引用登记一条（68 号 §3.2 模型）。
type ExternalRef struct {
	RefID         string  `json:"ref_id"`
	FromOntoID    string  `json:"from_onto_id"`
	FromConcept   string  `json:"from_concept"`
	ToOntoID      string  `json:"to_onto_id"` // 空=外部词表/未导入资产（to_concept 承载外部 IRI）
	ToConcept     string  `json:"to_concept"` // 名字引用；to_onto_id 空时可为外部 IRI
	RefKind       string  `json:"ref_kind"`   // import | alignment | reuse
	Predicate     string  `json:"predicate"`  // alignment 细分（exactMatch/closeMatch/broadMatch/narrowMatch/relatedMatch）
	Confidence    float64 `json:"confidence"`
	Justification string  `json:"justification"`
	CreatedBy     string  `json:"created_by"`
	CreatedAt     string  `json:"created_at"`
	Note          string  `json:"note"`
	// 悬空检测结果（读侧填充；qualitygate 同源逻辑）
	Dangling string `json:"dangling,omitempty"` // ""|missing_ontology|missing_concept
}

var validRefKinds = map[string]bool{"import": true, "alignment": true, "reuse": true}

// OntologyLevelRef 本体级引用哨兵源概念（owl:imports 自动登记用——不挂单概念）。
const OntologyLevelRef = "(ontology)"

var validPredicates = map[string]bool{
	"exactMatch": true, "closeMatch": true, "broadMatch": true, "narrowMatch": true, "relatedMatch": true,
}

// CreateExternalRef 登记一条跨本体引用（存在性校验：from 本体/概念须存在；
// to_onto_id 非空时目标本体须存在——to_concept 悬空允许登记，qualitygate 提醒）。
func (s *Store) CreateExternalRef(r *ExternalRef) (*ExternalRef, error) {
	r.RefKind = strings.TrimSpace(r.RefKind)
	if !validRefKinds[r.RefKind] {
		return nil, fmt.Errorf("ref_kind 须为 import/alignment/reuse，得到 %q", r.RefKind)
	}
	r.FromConcept = strings.TrimSpace(r.FromConcept)
	r.ToConcept = strings.TrimSpace(r.ToConcept)
	if r.FromConcept == "" || r.ToConcept == "" {
		return nil, errors.New("from_concept/to_concept 必填（名字引用；外部 IRI 场景 to_onto_id 留空、to_concept 填 IRI）")
	}
	if r.FromOntoID == "" {
		return nil, errors.New("from_onto_id 必填")
	}
	if r.RefKind == "alignment" && r.Predicate != "" && !validPredicates[r.Predicate] {
		return nil, fmt.Errorf("predicate 须为 SSSOM 词表（exactMatch/closeMatch/broadMatch/narrowMatch/relatedMatch），得到 %q", r.Predicate)
	}
	if r.RefKind != "alignment" {
		r.Predicate = "" // import/reuse 词表不适用（68 号 §3.1）
	}
	if r.Confidence <= 0 {
		r.Confidence = 1.0
	}
	if r.Justification == "" {
		r.Justification = "expert_review"
	}
	// 存在性校验：源本体+源概念
	from, err := s.GetOntology(r.FromOntoID)
	if err != nil || from == nil {
		return nil, fmt.Errorf("源本体 %s 不存在", r.FromOntoID)
	}
	// 本体级引用哨兵（owl:imports 装载侧自动登记——引用对象是本体自身非单概念）；
	// 其余登记只挂已声明概念。
	if r.FromConcept != OntologyLevelRef && !s.ConceptExists(r.FromOntoID, r.FromConcept) {
		return nil, fmt.Errorf("源概念 %q 在本体 %s 不存在（登记只挂已声明概念；本体级引用用 %s 哨兵）", r.FromConcept, r.FromOntoID, OntologyLevelRef)
	}
	// 目标本体（非空时校验存在；空=外部 IRI 登记态，允许悬空）
	if r.ToOntoID != "" {
		to, gerr := s.GetOntology(r.ToOntoID)
		if gerr != nil || to == nil {
			return nil, fmt.Errorf("目标本体 %s 不存在（外部词表场景请留空 to_onto_id、to_concept 填 IRI）", r.ToOntoID)
		}
	}
	if r.RefID == "" {
		r.RefID = NewRefID()
	}
	if r.CreatedAt == "" {
		r.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err = s.db.Exec(`INSERT INTO external_refs (ref_id,from_onto_id,from_concept,to_onto_id,to_concept,ref_kind,predicate,confidence,justification,created_by,created_at,note) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.RefID, r.FromOntoID, r.FromConcept, r.ToOntoID, r.ToConcept, r.RefKind, r.Predicate, r.Confidence, r.Justification, r.CreatedBy, r.CreatedAt, r.Note)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// ListExternalRefs 本体的外引清单（默认按创建时间）。
func (s *Store) ListExternalRefs(ontoID string) ([]*ExternalRef, error) {
	rows, err := s.db.Query(`SELECT ref_id,from_onto_id,from_concept,to_onto_id,to_concept,ref_kind,predicate,confidence,justification,created_by,created_at,note FROM external_refs WHERE from_onto_id=? ORDER BY created_at,ref_id`, ontoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ExternalRef
	for rows.Next() {
		r := &ExternalRef{}
		if err := rows.Scan(&r.RefID, &r.FromOntoID, &r.FromConcept, &r.ToOntoID, &r.ToConcept, &r.RefKind, &r.Predicate, &r.Confidence, &r.Justification, &r.CreatedBy, &r.CreatedAt, &r.Note); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteExternalRef 摘除一条登记。
func (s *Store) DeleteExternalRef(ontoID, refID string) error {
	res, err := s.db.Exec(`DELETE FROM external_refs WHERE ref_id=? AND from_onto_id=?`, refID, ontoID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DetectDanglingRefs 悬空引用检测（qualitygate「失效引用提醒」与读侧标注同源）：
// to_onto_id 指向的本体已删 → missing_ontology；目标概念在目标本体不存在 → missing_concept。
// 只提醒不阻断（68 号 §3.4 imports 闭包版本风险对策）。
func (s *Store) DetectDanglingRefs(ontoID string) ([]string, error) {
	refs, err := s.ListExternalRefs(ontoID)
	if err != nil {
		return nil, err
	}
	var msgs []string
	for _, r := range refs {
		if r.ToOntoID == "" {
			continue // 外部 IRI 登记态：无目标资产可校验，天然「悬空」但按设计不提醒
		}
		if _, err := s.GetOntology(r.ToOntoID); err != nil {
			msgs = append(msgs, fmt.Sprintf("%s:%s →%s（目标本体已删除）", r.FromConcept, r.RefKind, r.ToOntoID))
			continue
		}
		if !s.ConceptExists(r.ToOntoID, r.ToConcept) {
			msgs = append(msgs, fmt.Sprintf("%s:%s →%s:%s（目标概念不存在）", r.FromConcept, r.RefKind, r.ToOntoID, r.ToConcept))
		}
	}
	return msgs, nil
}

// ConceptExists 概念名存在性（spec_json.Concepts 裸名匹配；spec 原文经 artifact 层解析，
// 解析失败视为不存在——登记端前置校验用）。
func (s *Store) ConceptExists(ontoID, concept string) bool {
	raw, _, err := s.GetArtifact(ontoID, "spec_json")
	if err != nil {
		return false
	}
	var sp struct {
		Concepts []struct {
			Name string `json:"name"`
		} `json:"concepts"`
	}
	if json.Unmarshal([]byte(raw), &sp) != nil {
		return false
	}
	for _, c := range sp.Concepts {
		if c.Name == concept {
			return true
		}
	}
	return false
}

// NewRefID 引用登记 ID。
func NewRefID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "extref-" + hex.EncodeToString(b)
}

var _ = sql.ErrNoRows // 保持 import（GetOntology 错误分支语义）
