package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// Skill 技能包（方案 §6.12）：配置级能力包（提示词 + 工具白名单），不是代码执行环境。
// REQ-373：Kind='collection' 为技能集合容器——Instruction 不注入（装配期按 Children 展开
// 成员目录）、Tools 不存（装配期取成员并集）；成员须为普通技能（集合不嵌套）。
type Skill struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Instruction string          `json:"instruction"`
	Tools       []string        `json:"tools"`
	Resources   []SkillResource `json:"resources"`
	Kind        string          `json:"kind"` // ''=普通技能 | 'collection'=集合
	Children    []string        `json:"children"`
	Builtin     bool            `json:"builtin"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// SkillResource 技能附加资源（P2 激活注入，本表先存）。
type SkillResource struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

const skillCols = `id,name,description,instruction,tools,resources,kind,children,builtin,enabled,created_at,updated_at`

func scanSkill(row interface{ Scan(...any) error }) (*Skill, error) {
	var s Skill
	var tools, resources, children string
	var builtin, enabled int
	if err := row.Scan(&s.ID, &s.Name, &s.Description, &s.Instruction, &tools, &resources, &s.Kind, &children, &builtin, &enabled, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.Builtin = builtin == 1
	s.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(tools), &s.Tools)
	_ = json.Unmarshal([]byte(resources), &s.Resources)
	_ = json.Unmarshal([]byte(children), &s.Children)
	if s.Tools == nil {
		s.Tools = []string{}
	}
	if s.Resources == nil {
		s.Resources = []SkillResource{}
	}
	if s.Children == nil {
		s.Children = []string{}
	}
	return &s, nil
}

func marshalSkillArrays(s *Skill) (tools, resources, children string) {
	tools = "[]"
	resources = "[]"
	children = "[]"
	if b, err := json.Marshal(s.Tools); err == nil {
		tools = string(b)
	}
	if b, err := json.Marshal(s.Resources); err == nil {
		resources = string(b)
	}
	if b, err := json.Marshal(s.Children); err == nil {
		children = string(b)
	}
	return
}

// ListSkills 返回技能列表。
func (s *Store) ListSkills() ([]*Skill, error) {
	rows, err := s.DB.Query(`SELECT ` + skillCols + ` FROM skill ORDER BY builtin DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Skill{}
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

// GetSkill 按 id 取技能。
func (s *Store) GetSkill(id string) (*Skill, error) {
	row := s.DB.QueryRow(`SELECT `+skillCols+` FROM skill WHERE id = ?`, id)
	sk, err := scanSkill(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return sk, err
}

// CreateSkill 新建技能（内置技能不可由此创建）。集合（Kind=collection）免 instruction 必填。
func (s *Store) CreateSkill(sk *Skill) (*Skill, error) {
	if strings.TrimSpace(sk.ID) == "" {
		sk.ID = NewID()
	}
	if strings.TrimSpace(sk.Name) == "" {
		return nil, &HTTPError{Status: 400, Msg: "name is required"}
	}
	if sk.Kind != "collection" && strings.TrimSpace(sk.Instruction) == "" {
		return nil, &HTTPError{Status: 400, Msg: "instruction is required"}
	}
	if sk.Kind == "collection" {
		sk.Instruction = "" // 集合无正文（成员正文经 load_skill 懒加载）
	}
	tools, resources, children := marshalSkillArrays(sk)
	_, err := s.DB.Exec(`INSERT INTO skill (id,name,description,instruction,tools,resources,kind,children,builtin,enabled,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,0,?,?,?)`,
		sk.ID, sk.Name, sk.Description, sk.Instruction, tools, resources, sk.Kind, children, boolInt(sk.Enabled), now(), now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return s.GetSkill(sk.ID)
}

// UpdateSkill 更新技能；name 唯一冲突返回 ErrConflict。kind 创建后不可变（集合/普通互转不做）。
func (s *Store) UpdateSkill(sk *Skill) (*Skill, error) {
	tools, resources, children := marshalSkillArrays(sk)
	res, err := s.DB.Exec(`UPDATE skill SET name=?,description=?,instruction=?,tools=?,resources=?,children=?,enabled=?,updated_at=? WHERE id=?`,
		sk.Name, sk.Description, sk.Instruction, tools, resources, children, boolInt(sk.Enabled), now(), sk.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrConflict
		}
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetSkill(sk.ID)
}

// DeleteSkill 删除技能（builtin 由 handler 层拦截）。删除成员时同步摘除各集合 children
// 中的引用（防悬挂——装配期对悬挂子件本就跳过，此处保数据整洁）。
func (s *Store) DeleteSkill(id string) error {
	res, err := s.DB.Exec(`DELETE FROM skill WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	rows, err := s.DB.Query(`SELECT id,children FROM skill WHERE kind='collection' AND children LIKE '%'||?||'%'`, id)
	if err != nil {
		return err
	}
	var hits []string
	for rows.Next() {
		var cid, children string
		if err := rows.Scan(&cid, &children); err != nil {
			rows.Close()
			return err
		}
		var ids []string
		_ = json.Unmarshal([]byte(children), &ids)
		for _, x := range ids {
			if x == id {
				hits = append(hits, cid)
				break
			}
		}
	}
	rows.Close()
	for _, cid := range hits {
		if err := s.RemoveFromCollection(cid, id); err != nil {
			return err
		}
	}
	return nil
}

// RemoveFromCollection 从集合 children 中摘除一个子件（保持其余顺序）。
func (s *Store) RemoveFromCollection(collectionID, childID string) error {
	sk, err := s.GetSkill(collectionID)
	if err != nil {
		return err
	}
	out := sk.Children[:0:0]
	removed := false
	for _, x := range sk.Children {
		if x == childID {
			removed = true
			continue
		}
		out = append(out, x)
	}
	if !removed {
		return nil
	}
	sk.Children = out
	_, _, children := marshalSkillArrays(sk)
	_, err = s.DB.Exec(`UPDATE skill SET children=?,updated_at=? WHERE id=?`, children, now(), collectionID)
	return err
}

// MaxCollectionChildren 集合成员数上限（与导入单包 500 技能口径对齐）。
const MaxCollectionChildren = 500

// ValidateCollectionChildren 集合成员校验：去重保序、每件存在且为普通技能（集合不嵌套）、
// 不含自身、总数不超上限。返回规范化后的 children。
func (s *Store) ValidateCollectionChildren(selfID string, ids []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == selfID || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) > MaxCollectionChildren {
		return nil, &HTTPError{Status: 400, Msg: fmt.Sprintf("集合成员最多 %d 件（当前 %d 件）", MaxCollectionChildren, len(out))}
	}
	for _, id := range out {
		sk, err := s.GetSkill(id)
		if err != nil {
			return nil, &HTTPError{Status: 400, Msg: "集合成员不存在或已被删除"}
		}
		if sk.Kind == "collection" {
			return nil, &HTTPError{Status: 400, Msg: fmt.Sprintf("「%s」是技能集合，不支持嵌套（集合成员须为普通技能）", sk.Name)}
		}
	}
	return out, nil
}
