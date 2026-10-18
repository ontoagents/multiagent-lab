// Package ontochat OntoChat 多轮引导会话存储（REQ-103 模式 A）。
// 会话状态机：cq（列能力问题）→ domain（逐轮补全领域信息）→ draft（生成 spec 草稿）
// → refine（校验错误回喂修正）→ done（已入库）。消息与累积上下文整体存 JSON 列，
// 无新服务、无新依赖——与 REQ-103 "复用 /api/ontology-llm/generate" 口径一致。
package ontochat

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
)

var ErrNotFound = repo.ErrNotFound

// Message 单条对话消息（全量留痕，前端直接渲染）。
type Message struct {
	Role    string `json:"role"` // user | assistant | system
	Content string `json:"content"`
	TS      string `json:"ts"`
}

// Context 累积上下文（每轮追加，draft 阶段整体喂给生成器）。
type Context struct {
	Description  string           `json:"description"`             // 领域描述（cq 阶段录入）
	CQs          []string         `json:"cqs,omitempty"`           // 能力问题列表
	Hints        []string         `json:"hints,omitempty"`         // 逐轮补全的领域信息
	DraftSpec    *json.RawMessage `json:"draft_spec,omitempty"`    // 最近一次草稿（refine 阶段回喂）
	StoryStep    int              `json:"story_step,omitempty"`    // REQ-275：访谈当前步（0 起）
	StoryAnswers []string         `json:"story_answers,omitempty"` // REQ-275：访谈逐问回答
	DraftStory   string           `json:"draft_story,omitempty"`   // REQ-275：汇总生成的用户故事（精修轮对象）
	// TestReport 覆盖测试报告（REQ-342/OC-2 test 站补位：会话内 draft 后即可测试，无需先入库；
	// 快筛段=口语化逐条判定。入库后的回归验证走资产质量卡双轨报告 artifact）
	TestReport *CoverageSummary `json:"test_report,omitempty"`
}

// Session 会话聚合。
type Session struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Stage      string    `json:"stage"`
	Round      int       `json:"round"`
	Messages   []Message `json:"messages"`
	Context    Context   `json:"context"`
	OntologyID string    `json:"ontology_id,omitempty"`
	// 归属审计两列（REQ-354/P2·G4，迁移 046）：page=页面流程（缺省）；agent_chat=智能体对话
	// （ontochat_start 经 tool.Caller 注入 agent_id）——「哪个智能体产了哪些本体」可审计。
	Source    string    `json:"source"`
	AgentID   string    `json:"agent_id,omitempty"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

// Store SQLite 存储（复用 repo.Store 的连接）。
type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// Create 新建会话（stage=cq；source=page 页面流程缺省）。
func (s *Store) Create(id, title string) (*Session, error) {
	return s.CreateSourced(id, title, "page", "")
}

// CreateSourced 新建会话并落归属审计两列（REQ-354/P2·G4；source 白名单外值归 page）。
func (s *Store) CreateSourced(id, title, source, agentID string) (*Session, error) {
	if source != "page" && source != "agent_chat" {
		source = "page"
	}
	ts := now()
	_, err := s.db.Exec(
		`INSERT INTO ontochat_session (id, title, stage, round, messages, context_json, source, agent_id, created_at, updated_at)
		 VALUES (?, ?, 'cq', 0, '[]', '{}', ?, ?, ?, ?)`, id, title, source, agentID, ts, ts)
	if err != nil {
		return nil, err
	}
	return s.Get(id)
}

// Get 读取会话；不存在 → repo.ErrNotFound。
func (s *Store) Get(id string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, title, stage, round, messages, context_json, ontology_id, source, agent_id, created_at, updated_at
		 FROM ontochat_session WHERE id = ?`, id)
	var sess Session
	var rawMsgs, rawCtx, ontoID sql.NullString
	if err := row.Scan(&sess.ID, &sess.Title, &sess.Stage, &sess.Round, &rawMsgs, &rawCtx, &ontoID,
		&sess.Source, &sess.AgentID, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(rawMsgs.String), &sess.Messages); err != nil {
		sess.Messages = nil
	}
	if err := json.Unmarshal([]byte(rawCtx.String), &sess.Context); err != nil {
		sess.Context = Context{}
	}
	sess.OntologyID = ontoID.String
	return &sess, nil
}

// List 会话列表（按更新时间倒序；不含消息体，列表页轻量）。
func (s *Store) List() ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, title, stage, round, ontology_id, source, agent_id, created_at, updated_at
		 FROM ontochat_session ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var sess Session
		var ontoID sql.NullString
		if err := rows.Scan(&sess.ID, &sess.Title, &sess.Stage, &sess.Round, &ontoID,
			&sess.Source, &sess.AgentID, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		sess.OntologyID = ontoID.String
		out = append(out, sess)
	}
	return out, rows.Err()
}

// Append 追加消息并更新阶段/轮数/上下文（整体覆写 messages/context_json，量级小无需增量）。
func (s *Store) Append(id string, msg Message, stage *string, round *int, ctx *Context) error {
	sess, err := s.Get(id)
	if err != nil {
		return err
	}
	if msg.TS == "" {
		msg.TS = now()
	}
	sess.Messages = append(sess.Messages, msg)
	if len(sess.Messages) > 200 { // 留痕上限：超出截断最旧（防长会话膨胀）
		sess.Messages = sess.Messages[len(sess.Messages)-200:]
	}
	if stage != nil {
		sess.Stage = *stage
	}
	if round != nil {
		sess.Round = *round
	}
	if ctx != nil {
		sess.Context = *ctx
	}
	rawMsgs, _ := json.Marshal(sess.Messages)
	rawCtx, _ := json.Marshal(sess.Context)
	ts := now()
	_, err = s.db.Exec(
		`UPDATE ontochat_session SET messages = ?, context_json = ?, stage = ?, round = ?, updated_at = ? WHERE id = ?`,
		string(rawMsgs), string(rawCtx), sess.Stage, sess.Round, ts, id)
	return err
}

// UpdateContext 定点更新上下文（不追加消息；REQ-272 CQ 确认写入等场景）。
func (s *Store) UpdateContext(id string, mutate func(*Context)) error {
	sess, err := s.Get(id)
	if err != nil {
		return err
	}
	mutate(&sess.Context)
	rawCtx, _ := json.Marshal(sess.Context)
	_, err = s.db.Exec(`UPDATE ontochat_session SET context_json = ?, updated_at = ? WHERE id = ?`, string(rawCtx), now(), id)
	return err
}

// BindOntology 入库后回填产物 id 并置 done。
func (s *Store) BindOntology(id, ontologyID string) error {
	ts := now()
	_, err := s.db.Exec(
		`UPDATE ontochat_session SET ontology_id = ?, stage = 'done', updated_at = ? WHERE id = ?`,
		ontologyID, ts, id)
	return err
}

// Delete 删除会话。
func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM ontochat_session WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// REQ-338③（80 号 A3）：草稿版本链——draft/refine 每次生成落全量快照（044 迁移）。
// 会话恢复从 {round,draft} 布尔升为版本链：中断续作/跨设备续作/审计回放一并满足；
// 文件材料引用清单随快照入库（材料溯源）。
// ---------------------------------------------------------------------------

// DraftVersion 草稿快照。
type DraftVersion struct {
	Version   int             `json:"version"`
	Source    string          `json:"source"` // draft | refine | confirm
	Round     int             `json:"round"`
	Materials []string        `json:"materials,omitempty"` // 文件材料引用清单
	Spec      json.RawMessage `json:"spec_json"`
	CreatedAt string          `json:"created_at"`
}

// materialNames 提取 Hints 中文件材料的引用名（ImportFile 落库形态「[文件导入 name]\n…」）。
func MaterialNames(hints []string) []string {
	names := []string{}
	for _, h := range hints {
		if rest, ok := strings.CutPrefix(h, "[文件导入 "); ok {
			if idx := strings.Index(rest, "]"); idx > 0 {
				names = append(names, rest[:idx])
			}
		}
	}
	return names
}

// SaveDraftVersion 落一版快照（version=会话内递增；快照失败不阻断生成主链路，返回错误由调用方记日志）。
func (s *Store) SaveDraftVersion(sessionID, source string, round int, materials []string, specJSON []byte) (int, error) {
	var next int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version),0)+1 FROM ontochat_draft_version WHERE session_id=?`, sessionID).Scan(&next); err != nil {
		return 0, err
	}
	if materials == nil {
		materials = []string{}
	}
	mb, _ := json.Marshal(materials)
	_, err := s.db.Exec(`INSERT INTO ontochat_draft_version(session_id, version, spec_json, source, round, materials) VALUES(?,?,?,?,?,?)`,
		sessionID, next, string(specJSON), source, round, string(mb))
	return next, err
}

// ListDraftVersions 会话草稿版本链（version 升序；spec 全量随行——会话草稿数有限，不分页）。
func (s *Store) ListDraftVersions(sessionID string) ([]DraftVersion, error) {
	rows, err := s.db.Query(`SELECT version, source, round, materials, spec_json, created_at FROM ontochat_draft_version WHERE session_id=? ORDER BY version`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DraftVersion{}
	for rows.Next() {
		var v DraftVersion
		var materials, spec string
		if err := rows.Scan(&v.Version, &v.Source, &v.Round, &materials, &spec, &v.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(materials), &v.Materials)
		v.Spec = json.RawMessage(spec)
		out = append(out, v)
	}
	return out, rows.Err()
}
