package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ---- KG 自存 + 审计决策（D-O15/REQ-110：去-semantica 化，KG 一等数据落主平台库）----
//
// 三表 = 教学口径「实体 / 关系 / claim」；claim 带 chunk_id 溯源，
// 支撑 GraphRAG 一跳扩展拼上下文与 PROV-O 导出（REQ-101 审计学习视图）。

// KGEntity KG 实体（name 库内业务键；同 KB 跨 doc 同名实体视为同一实体，重建按 doc 覆盖）。
// UID 稳定实体 id（KB-16/M286：= KGEntityUID(kb,name) 确定性派生——重建不再换 id，
// name 保留为展示名+业务键兼容层；关系/claim 经 source_uid/target_uid/subject_uid 引用）。
type KGEntity struct {
	ID          string `json:"id"`
	KBID        string `json:"kb_id"`
	DocID       string `json:"doc_id,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	// Alias 别名（M36/KB-7②：检索与搜索命中别名；分号分隔多别名；重建同 名 保留）
	Alias string `json:"alias,omitempty"`
	// UID 稳定实体 id（KB-16/M286：uid = kb_id::name；upsert 冲突键——重建/重索引不变）
	UID       string `json:"uid,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// KGRelationship KG 关系（source/target 引用实体 name；type 教学口径用大写短语，如 IS_A/具有/引发）。
// SourceUID/TargetUID 稳定实体 id 引用（KB-16/M286：写侧随实体 uid 回填，存量迁移归一）。
type KGRelationship struct {
	ID     string `json:"id"`
	KBID   string `json:"kb_id"`
	DocID  string `json:"doc_id,omitempty"`
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type,omitempty"`
	// Status 审核状态（M16/REQ-129：approved|rejected；rejected 不参与检索）
	Status     string `json:"status,omitempty"`
	SourceUID  string `json:"source_uid,omitempty"`
	TargetUID  string `json:"target_uid,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// KGClaim claim：实体的一条可溯源陈述（原文句），chunk_id 指向出处片段。
// SubjectUID 稳定实体 id 引用（KB-16/M286：写侧回填，chunk-entity 倒排同源）。
type KGClaim struct {
	ID      string `json:"id"`
	KBID    string `json:"kb_id"`
	DocID   string `json:"doc_id,omitempty"`
	ChunkID string `json:"chunk_id,omitempty"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	// Status 审核状态（M16/REQ-129：approved|rejected；rejected 不参与检索）
	Status     string `json:"status,omitempty"`
	SubjectUID string `json:"subject_uid,omitempty"`
	// KB-17/M289：结构化溯源 span（chunk 内容内 rune 起止偏移；-1=未定位——失败率可量化）
	SpanStart int `json:"span_start,omitempty"`
	SpanEnd   int `json:"span_end,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// KGEntityUID 稳定实体 id 派生（KB-16/M286 单源）：kb_id::name——确定性、可读、迁移 SQL
// 同式（kb_id 为主键生成的字母数字串，'::' 分隔无歧义；name 不再从 uid 反解）。
func KGEntityUID(kbID, name string) string {
	return kbID + "::" + name
}

// OntoDecision 审计决策留痕（derived_from 指向前置决策，构成溯源链）。
type OntoDecision struct {
	ID          string `json:"id"`
	SubjectKind string `json:"subject_kind"` // kg | ontology | kb | manual
	SubjectID   string `json:"subject_id"`
	Title       string `json:"title"`
	Rationale   string `json:"rationale,omitempty"`
	DerivedFrom string `json:"derived_from,omitempty"`
	MetaJSON    string `json:"meta_json,omitempty"`
	CreatedAt   string `json:"created_at"`
}

const kgEntityCols = `id,kb_id,doc_id,name,type,description,alias,uid,created_at`
const kgRelCols = `id,kb_id,doc_id,source,target,rel_type,status,created_at`
const kgClaimCols = `id,kb_id,doc_id,chunk_id,subject,text,status,created_at,span_start,span_end`
const decisionCols = `id,subject_kind,subject_id,title,rationale,derived_from,meta_json,created_at`

func scanKGEntity(row interface{ Scan(...any) error }) (*KGEntity, error) {
	var e KGEntity
	if err := row.Scan(&e.ID, &e.KBID, &e.DocID, &e.Name, &e.Type, &e.Description, &e.Alias, &e.UID, &e.CreatedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

func scanKGRel(row interface{ Scan(...any) error }) (*KGRelationship, error) {
	var r KGRelationship
	if err := row.Scan(&r.ID, &r.KBID, &r.DocID, &r.Source, &r.Target, &r.Type, &r.Status, &r.CreatedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

func scanKGClaim(row interface{ Scan(...any) error }) (*KGClaim, error) {
	var c KGClaim
	if err := row.Scan(&c.ID, &c.KBID, &c.DocID, &c.ChunkID, &c.Subject, &c.Text, &c.Status, &c.CreatedAt, &c.SpanStart, &c.SpanEnd); err != nil {
		return nil, err
	}
	return &c, nil
}

func scanDecision(row interface{ Scan(...any) error }) (*OntoDecision, error) {
	var d OntoDecision
	if err := row.Scan(&d.ID, &d.SubjectKind, &d.SubjectID, &d.Title, &d.Rationale, &d.DerivedFrom, &d.MetaJSON, &d.CreatedAt); err != nil {
		return nil, err
	}
	return &d, nil
}

// ReplaceKGForDoc 事务落库 KG 抽取结果（重索引/重建幂等）——KB-16/M286 稳定 id 改造：
//   - 实体走稳定 uid upsert（KGEntityUID(kb,name) 确定性派生，重建不再换 id；同 KB 跨 doc
//     同名 = 同 uid 天然归一，原「删他 doc 同名再插本 doc 版本」契约退役；人工别名 upsert 保留）；
//   - 关系/claim 保持 doc 级替换（doc 事实随重索引重建），写侧回填 source_uid/target_uid/subject_uid
//     （本批实体 uid 集内直取，批外名字回查实体表，仍缺如实留空——无外键约束现状不变）；
//   - chunk↔实体倒排（kg_chunk_entity）抽取期落库（claims 的 (chunk_id, subject) 对），
//     检索期替代「KGSubjectsByChunks claim 反查」唯一路径（两路保留互为对照）；
//   - docID 非空：清本 doc 关系/claim/倒排 + 陈旧实体（uid 不在本次产出集且归属本 doc）；
//     docID 为空（KB 级重建）：清全部关系/claim/倒排 + 陈旧实体（uid 不在本次产出集）。
func (s *Store) ReplaceKGForDoc(kbID, docID string, entities []*KGEntity, rels []*KGRelationship, claims []*KGClaim) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// M36/KB-7②：别名表在写前读取（upsert 新值空则保留旧值，人工标注不因重建丢失）
	aliasAll := map[string]string{}
	if arows, err := tx.Query(`SELECT name, alias FROM kg_entity WHERE kb_id = ? AND alias != ''`, kbID); err == nil {
		for arows.Next() {
			var n, a string
			if err := arows.Scan(&n, &a); err == nil {
				aliasAll[n] = a
			}
		}
		arows.Close()
	}
	// 稳定 uid 派生（单源 KGEntityUID）+ 别名回填
	uidSet := map[string]bool{}
	uidByName := map[string]string{}
	for _, e := range entities {
		e.UID = KGEntityUID(kbID, e.Name)
		if e.Alias == "" {
			e.Alias = aliasAll[e.Name]
		}
		uidSet[e.UID] = true
		if _, ok := uidByName[e.Name]; !ok {
			uidByName[e.Name] = e.UID
		}
	}
	relWhere, claimWhere := `kb_id = ? AND doc_id = ?`, `kb_id = ? AND doc_id = ?`
	delArgs := []any{kbID, docID}
	if docID == "" { // KB 级重建：清该库全部关系/claim/倒排
		relWhere, claimWhere = `kb_id = ?`, `kb_id = ?`
		delArgs = []any{kbID}
	}
	if _, err := tx.Exec(`DELETE FROM kg_relationship WHERE `+relWhere, delArgs...); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM kg_claim WHERE `+claimWhere, delArgs...); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM kg_chunk_entity WHERE `+claimWhere, delArgs...); err != nil {
		return err
	}
	// 陈旧实体清理（uid 稳定语义下的替换面：本次抽取不再产出的实体）
	if len(entities) > 0 {
		ph := strings.TrimRight(strings.Repeat("?,", len(entities)), ",")
		args := []any{kbID}
		staleWhere := `kb_id = ? AND uid != '' AND uid NOT IN (` + ph + `)`
		if docID != "" {
			staleWhere = `kb_id = ? AND doc_id = ? AND uid != '' AND uid NOT IN (` + ph + `)`
			args = append(args, docID)
		}
		for _, e := range entities {
			args = append(args, e.UID)
		}
		if _, err := tx.Exec(`DELETE FROM kg_entity WHERE `+staleWhere, args...); err != nil {
			return err
		}
	} else { // 空抽取：沿用替换语义清实体（doc 级=本 doc；KB 级=全部）
		if _, err := tx.Exec(`DELETE FROM kg_entity WHERE `+claimWhere, delArgs...); err != nil {
			return err
		}
	}
	// 实体 upsert（冲突键=uid；row id 与 created_at 保留原值——实体身份跨重建稳定）
	ie, err := tx.Prepare(`INSERT INTO kg_entity(id,kb_id,doc_id,name,type,description,alias,created_at,uid) VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(uid) DO UPDATE SET doc_id=excluded.doc_id, type=excluded.type, description=excluded.description,
		alias=CASE WHEN excluded.alias != '' THEN excluded.alias ELSE kg_entity.alias END`)
	if err != nil {
		return err
	}
	defer ie.Close()
	for _, e := range entities {
		if _, err := ie.Exec(e.ID, e.KBID, e.DocID, e.Name, e.Type, e.Description, e.Alias, now(), e.UID); err != nil {
			return err
		}
	}
	// 批外名字的 uid 回查（关系/claim 引用不限于本批实体的防御性归一）
	uidFor := func(name string) string {
		if u, ok := uidByName[name]; ok {
			return u
		}
		var u string
		_ = tx.QueryRow(`SELECT uid FROM kg_entity WHERE kb_id = ? AND name = ?`, kbID, name).Scan(&u)
		return u
	}
	ir, err := tx.Prepare(`INSERT INTO kg_relationship(id,kb_id,doc_id,source,target,rel_type,status,created_at,source_uid,target_uid) VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ir.Close()
	for _, r := range rels {
		if r.SourceUID == "" {
			r.SourceUID = uidFor(r.Source)
		}
		if r.TargetUID == "" {
			r.TargetUID = uidFor(r.Target)
		}
		if _, err := ir.Exec(r.ID, r.KBID, r.DocID, r.Source, r.Target, r.Type, "approved", now(), r.SourceUID, r.TargetUID); err != nil {
			return err
		}
	}
	ic, err := tx.Prepare(`INSERT INTO kg_claim(id,kb_id,doc_id,chunk_id,subject,text,status,created_at,subject_uid,span_start,span_end) VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ic.Close()
	ice, err := tx.Prepare(`INSERT OR IGNORE INTO kg_chunk_entity(kb_id,doc_id,chunk_id,entity_uid,name,created_at) VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ice.Close()
	for _, c := range claims {
		if c.SubjectUID == "" {
			c.SubjectUID = uidFor(c.Subject)
		}
		if _, err := ic.Exec(c.ID, c.KBID, c.DocID, c.ChunkID, c.Subject, c.Text, "approved", now(), c.SubjectUID, c.SpanStart, c.SpanEnd); err != nil {
			return err
		}
		// chunk↔实体倒排（抽取期落库；与 claim 反查两路对照）
		if c.ChunkID != "" && c.SubjectUID != "" {
			if _, err := ice.Exec(c.KBID, c.DocID, c.ChunkID, c.SubjectUID, c.Subject, now()); err != nil {
				return err
			}
		}
	}
	// 实体向量孤儿清理（KB-4②③/M287：kg_entity_vec 键=entity_uid，实体行回收后缓存同事务回收）
	if err := deleteOrphanKGEntityVecsTx(tx, kbID); err != nil {
		return err
	}
	return tx.Commit()
}

// KGByKB 某 KB 的全量 KG 子图（O13 策略 B/C 数据源 + 消费页图谱展示）。
func (s *Store) KGByKB(kbID string) ([]*KGEntity, []*KGRelationship, error) {
	rows, err := s.DB.Query(`SELECT `+kgEntityCols+` FROM kg_entity WHERE kb_id = ? ORDER BY name`, kbID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	entities := []*KGEntity{}
	for rows.Next() {
		e, err := scanKGEntity(rows)
		if err != nil {
			return nil, nil, err
		}
		entities = append(entities, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	rows2, err := s.DB.Query(`SELECT `+kgRelCols+` FROM kg_relationship WHERE kb_id = ? ORDER BY source, target`, kbID)
	if err != nil {
		return nil, nil, err
	}
	defer rows2.Close()
	rels := []*KGRelationship{}
	for rows2.Next() {
		r, err := scanKGRel(rows2)
		if err != nil {
			return nil, nil, err
		}
		rels = append(rels, r)
	}
	return entities, rels, rows2.Err()
}

// KGCounts KB 的 KG 规模（选择器徽标 / kg_ready 判定）。
func (s *Store) KGCounts(kbID string) (entities, relationships int, err error) {
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM kg_entity WHERE kb_id = ?`, kbID).Scan(&entities); err != nil {
		return 0, 0, err
	}
	err = s.DB.QueryRow(`SELECT COUNT(*) FROM kg_relationship WHERE kb_id = ?`, kbID).Scan(&relationships)
	return entities, relationships, err
}

// KGMethod 抽取方式不落 KG 表：记录在重建时写入的审计决策行（subject_kind='kg'）meta_json，
// 由 kg 包读取（教学口径：抽取方式本身即一条决策）。

// KGSubjectsByChunks 命中 chunk → 提及实体名（GraphRAG 一跳扩展第一步）。
func (s *Store) KGSubjectsByChunks(kbID string, chunkIDs []string) ([]string, error) {
	if len(chunkIDs) == 0 {
		return nil, nil
	}
	q := `SELECT DISTINCT subject FROM kg_claim WHERE kb_id = ? AND status = 'approved' AND chunk_id IN (` +
		strings.TrimRight(strings.Repeat("?,", len(chunkIDs)), ",") + `)`
	args := make([]any, 0, len(chunkIDs)+1)
	args = append(args, kbID)
	for _, id := range chunkIDs {
		args = append(args, id)
	}
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// KGNeighbors 一跳扩展：与 seed 实体相关的关系（source 或 target 命中）。
func (s *Store) KGNeighbors(kbID string, subjects []string) ([]*KGRelationship, error) {
	if len(subjects) == 0 {
		return nil, nil
	}
	q := `SELECT ` + kgRelCols + ` FROM kg_relationship WHERE kb_id = ? AND status = 'approved' AND (source IN (` +
		strings.TrimRight(strings.Repeat("?,", len(subjects)), ",") + `) OR target IN (` +
		strings.TrimRight(strings.Repeat("?,", len(subjects)), ",") + `)) ORDER BY source, target`
	args := []any{kbID}
	for _, n := range subjects {
		args = append(args, n)
	}
	for _, n := range subjects {
		args = append(args, n)
	}
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KGRelationship{}
	for rows.Next() {
		r, err := scanKGRel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// KGNeighborsMultiHop 多跳邻域（KB-4①/M35：SQLite 递归 CTE，12 号 v0.9 原设计兑现）。
// 语义：从 seeds 出发沿 approved 关系无向行走 hops 步（1~3），返回**遍历途中触达的边**
// （与逐跳 BFS 前沿扩展同口径，而非可达节点集上的全部边）；UNION 去重 + depth 上限保证终止。
// 类型过滤由调用方做（过滤影响遍历前沿时应走逐跳路径，见 kb.GraphragQueryDetail）。
func (s *Store) KGNeighborsMultiHop(kbID string, seeds []string, hops int) ([]*KGRelationship, error) {
	if len(seeds) == 0 {
		return nil, nil
	}
	if hops < 1 {
		hops = 1
	}
	if hops > 3 {
		hops = 3
	}
	seedSrc := `SELECT ? AS name` + strings.Repeat(` UNION ALL SELECT ?`, len(seeds)-1)
	q := `WITH RECURSIVE walk(node, depth, edge_id) AS (
		SELECT name, 0, NULL FROM (` + seedSrc + `)
		UNION
		SELECT CASE WHEN r.source = w.node THEN r.target ELSE r.source END, w.depth + 1, r.id
		FROM walk w JOIN kg_relationship r
			ON r.kb_id = ? AND r.status = 'approved' AND (r.source = w.node OR r.target = w.node)
		WHERE w.depth < ?
	)
	SELECT ` + kgRelCols + ` FROM kg_relationship
	WHERE id IN (SELECT DISTINCT edge_id FROM walk WHERE edge_id IS NOT NULL)
	ORDER BY source, target`
	args := make([]any, 0, len(seeds)+2)
	for _, n := range seeds {
		args = append(args, n)
	}
	args = append(args, kbID, hops)
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KGRelationship{}
	for rows.Next() {
		r, err := scanKGRel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// KGClaimsForSubjects 实体的 claims（一跳上下文材料；限 limit 防 prompt 失控）。
func (s *Store) KGClaimsForSubjects(kbID string, subjects []string, limit int) ([]*KGClaim, error) {
	if len(subjects) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 24
	}
	q := `SELECT ` + kgClaimCols + ` FROM kg_claim WHERE kb_id = ? AND status = 'approved' AND subject IN (` +
		strings.TrimRight(strings.Repeat("?,", len(subjects)), ",") + `) ORDER BY created_at LIMIT ?`
	args := []any{kbID}
	for _, n := range subjects {
		args = append(args, n)
	}
	args = append(args, limit)
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KGClaim{}
	for rows.Next() {
		c, err := scanKGClaim(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// KGEntitiesByNames 实体详情（desc/type 回填一跳结果）。
func (s *Store) KGEntitiesByNames(kbID string, names []string) (map[string]*KGEntity, error) {
	out := map[string]*KGEntity{}
	if len(names) == 0 {
		return out, nil
	}
	q := `SELECT ` + kgEntityCols + ` FROM kg_entity WHERE kb_id = ? AND name IN (` +
		strings.TrimRight(strings.Repeat("?,", len(names)), ",") + `)`
	args := []any{kbID}
	for _, n := range names {
		args = append(args, n)
	}
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanKGEntity(rows)
		if err != nil {
			return nil, err
		}
		out[e.Name] = e
	}
	return out, rows.Err()
}

// ---- M16 阶段一（REQ-127/128）：图谱统计、实体搜索、带溯源 claims ----

// KGClaimTrace 带 chunk 溯源的 claim（doc_id/seq 定位原文片段）。
type KGClaimTrace struct {
	KGClaim
	DocID    string `json:"trace_doc_id,omitempty"`
	ChunkSeq int    `json:"trace_seq,omitempty"`
	// ChunkContent 出处 chunk 原文（M36/B1：句级溯源高亮用；仅 KGClaimsWithChunks 填充）
	ChunkContent string `json:"trace_chunk_content,omitempty"`
}

// KGStats 图谱统计卡数据（REQ-127）。
type KGStats struct {
	Entities       int            `json:"entities"`
	Relationships  int            `json:"relationships"`
	Claims         int            `json:"claims"`
	EntityTypeDist map[string]int `json:"entity_type_dist"`
	RelTypeDist    map[string]int `json:"rel_type_dist"`
	DocsTotal      int            `json:"docs_total"`
	DocsWithKG     int            `json:"docs_with_kg"`
	DegradedDocs   int            `json:"degraded_docs"`
}

// KGStatsForKB 统计：三表计数 + 类型分布 + 文档覆盖（无 KG 行的文档即 degraded/未覆盖）。
func (s *Store) KGStatsForKB(kbID string) (*KGStats, error) {
	st := &KGStats{EntityTypeDist: map[string]int{}, RelTypeDist: map[string]int{}}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM kg_entity WHERE kb_id = ?`, kbID).Scan(&st.Entities); err != nil {
		return nil, err
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM kg_relationship WHERE kb_id = ?`, kbID).Scan(&st.Relationships); err != nil {
		return nil, err
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM kg_claim WHERE kb_id = ?`, kbID).Scan(&st.Claims); err != nil {
		return nil, err
	}
	erows, err := s.DB.Query(`SELECT COALESCE(type,''), COUNT(*) FROM kg_entity WHERE kb_id = ? GROUP BY type`, kbID)
	if err != nil {
		return nil, err
	}
	defer erows.Close()
	for erows.Next() {
		var t string
		var n int
		if err := erows.Scan(&t, &n); err != nil {
			return nil, err
		}
		st.EntityTypeDist[t] = n
	}
	erows.Err()
	rrows, err := s.DB.Query(`SELECT COALESCE(rel_type,''), COUNT(*) FROM kg_relationship WHERE kb_id = ? GROUP BY rel_type`, kbID)
	if err != nil {
		return nil, err
	}
	defer rrows.Close()
	for rrows.Next() {
		var t string
		var n int
		if err := rrows.Scan(&t, &n); err != nil {
			return nil, err
		}
		st.RelTypeDist[t] = n
	}
	rrows.Err()
	// 文档覆盖：kb_doc 全集 vs kg_entity 出现过的 doc_id
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM knowledge_doc WHERE kb_id = ?`, kbID).Scan(&st.DocsTotal); err != nil {
		return nil, err
	}
	if err := s.DB.QueryRow(`SELECT COUNT(DISTINCT doc_id) FROM kg_entity WHERE kb_id = ? AND doc_id != ''`, kbID).Scan(&st.DocsWithKG); err != nil {
		return nil, err
	}
	return st, nil
}

// KGSearchEntities 实体名模糊搜索（图谱页搜索框；LIKE 转义由调用方保证，%/_ 通配在此转义）。
// M36/KB-7②：别名一并命中（命中别名的实体照常返回）。
func (s *Store) KGSearchEntities(kbID, q string, limit int) ([]*KGEntity, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q = strings.ReplaceAll(q, "%", "\\%")
	q = strings.ReplaceAll(q, "_", "\\_")
	rows, err := s.DB.Query(
		`SELECT `+kgEntityCols+` FROM kg_entity WHERE kb_id = ? AND (name LIKE '%' || ? || '%' ESCAPE '\' OR alias LIKE '%' || ? || '%' ESCAPE '\') ORDER BY name LIMIT ?`,
		kbID, q, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KGEntity{}
	for rows.Next() {
		e, err := scanKGEntity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// KGClaimsWithChunks claims 附 chunk 溯源（doc_id/seq，图谱页点击 claim 定位原文）。
func (s *Store) KGClaimsWithChunks(kbID string, subjects []string, limit int) ([]*KGClaimTrace, error) {
	if len(subjects) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 24
	}
	q := `SELECT c.id,c.kb_id,c.doc_id,c.chunk_id,c.subject,c.text,c.status,c.created_at, COALESCE(ch.doc_id,''), COALESCE(ch.seq,0), COALESCE(ch.content,'')
		FROM kg_claim c
		LEFT JOIN knowledge_chunk ch ON ch.id = c.chunk_id
		WHERE c.kb_id = ? AND c.status = 'approved' AND c.subject IN (` +
		strings.TrimRight(strings.Repeat("?,", len(subjects)), ",") + `)
		ORDER BY c.created_at LIMIT ?`
	args := []any{kbID}
	for _, n := range subjects {
		args = append(args, n)
	}
	args = append(args, limit)
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KGClaimTrace{}
	for rows.Next() {
		var t KGClaimTrace
		var doc sql.NullString
		if err := rows.Scan(&t.ID, &t.KBID, &t.DocID, &t.ChunkID, &t.Subject, &t.Text, &t.Status, &t.CreatedAt, &doc, &t.ChunkSeq, &t.ChunkContent); err != nil {
			return nil, err
		}
		if doc.Valid {
			t.DocID = doc.String
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// deleteKG 级联删除 KG 行（docID 空 = 整库）。审计决策保留（决策史不随数据删除，教学口径）。
func (s *Store) deleteKG(kbID, docID string) {
	if docID == "" {
		s.DB.Exec(`DELETE FROM kg_entity WHERE kb_id = ?`, kbID)
		s.DB.Exec(`DELETE FROM kg_relationship WHERE kb_id = ?`, kbID)
		s.DB.Exec(`DELETE FROM kg_claim WHERE kb_id = ?`, kbID)
		s.DeleteOrphanKGEntityVecs(kbID) // KB-4②③/M287：KB 删除级联回收实体向量缓存
		return
	}
	s.DB.Exec(`DELETE FROM kg_entity WHERE kb_id = ? AND doc_id = ?`, kbID, docID)
	s.DB.Exec(`DELETE FROM kg_relationship WHERE kb_id = ? AND doc_id = ?`, kbID, docID)
	s.DB.Exec(`DELETE FROM kg_claim WHERE kb_id = ? AND doc_id = ?`, kbID, docID)
	s.DeleteOrphanKGEntityVecs(kbID) // KB-4②③/M287：doc 删除后实体行可能随之回收（uid 语义）
}

// ---- 审计决策（REQ-101 学习视图，D-O15 消费/审计页）----

// InsertDecision 记录决策（id 空自动生成）。
func (s *Store) InsertDecision(d *OntoDecision) (*OntoDecision, error) {
	if strings.TrimSpace(d.Title) == "" {
		return nil, &HTTPError{Status: 400, Msg: "title is required"}
	}
	if d.ID == "" {
		d.ID = NewID()
	}
	if d.SubjectKind == "" {
		d.SubjectKind = "manual"
	}
	if d.MetaJSON == "" {
		d.MetaJSON = "{}"
	}
	_, err := s.DB.Exec(`INSERT INTO onto_decision(`+decisionCols+`) VALUES (?,?,?,?,?,?,?,?)`,
		d.ID, d.SubjectKind, d.SubjectID, d.Title, d.Rationale, d.DerivedFrom, d.MetaJSON, now())
	if err != nil {
		return nil, err
	}
	return s.GetDecision(d.ID)
}

// GetDecision 单条。
func (s *Store) GetDecision(id string) (*OntoDecision, error) {
	d, err := scanDecision(s.DB.QueryRow(`SELECT `+decisionCols+` FROM onto_decision WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// ListDecisions 决策列表（subjectKind/subjectID 可选过滤；新→旧）。
func (s *Store) ListDecisions(subjectKind, subjectID string, limit int) ([]*OntoDecision, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT ` + decisionCols + ` FROM onto_decision`
	var args []any
	var conds []string
	if subjectKind != "" {
		conds = append(conds, `subject_kind = ?`)
		args = append(args, subjectKind)
	}
	if subjectID != "" {
		conds = append(conds, `subject_id = ?`)
		args = append(args, subjectID)
	}
	if len(conds) > 0 {
		q += ` WHERE ` + strings.Join(conds, ` AND `)
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*OntoDecision{}
	for rows.Next() {
		d, err := scanDecision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DecisionChain 溯源链：沿 derived_from 向前回溯（环防御，最多 32 跳）。
func (s *Store) DecisionChain(id string) ([]*OntoDecision, error) {
	out := []*OntoDecision{}
	seen := map[string]bool{}
	cur := id
	for i := 0; i < 32 && cur != ""; i++ {
		if seen[cur] {
			break
		}
		seen[cur] = true
		d, err := s.GetDecision(cur)
		if errors.Is(err, ErrNotFound) {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, d)
		cur = d.DerivedFrom
	}
	return out, nil
}

// ---- M16 阶段二（REQ-129）：抽取治理与人工反馈 ----

// SetKGRelStatus 关系审核状态（approved|rejected；rejected 不参与检索）。
func (s *Store) SetKGRelStatus(kbID, id, status string) error {
	if status != "approved" && status != "rejected" {
		return &HTTPError{Status: 400, Msg: "status must be approved|rejected"}
	}
	res, err := s.DB.Exec(`UPDATE kg_relationship SET status=? WHERE kb_id=? AND id=?`, status, kbID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetKGClaimStatus claim 审核状态（approved|rejected）。
func (s *Store) SetKGClaimStatus(kbID, id, status string) error {
	if status != "approved" && status != "rejected" {
		return &HTTPError{Status: 400, Msg: "status must be approved|rejected"}
	}
	res, err := s.DB.Exec(`UPDATE kg_claim SET status=? WHERE kb_id=? AND id=?`, status, kbID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MergeKGEntities 实体消歧合并（REQ-129②，仅人工触发）：把 merge 实体的关系边与 claims
// 迁移到 keep 实体（关系按 source+type+target 去重），随后删除 merge 实体行。
func (s *Store) MergeKGEntities(kbID, keep string, merge []string) (movedRels, movedClaims int, err error) {
	if keep == "" || len(merge) == 0 {
		return 0, 0, &HTTPError{Status: 400, Msg: "keep 与 merge 均必填"}
	}
	for _, m := range merge {
		if m == keep {
			return 0, 0, &HTTPError{Status: 400, Msg: "merge 列表不能包含 keep 实体"}
		}
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	ph := strings.TrimRight(strings.Repeat("?,", len(merge)), ",")
	base := append([]any{kbID}, toAny(merge)...)
	// 关系迁移：source/target 指向 merge 实体的改指 keep
	res, err := tx.Exec(`UPDATE kg_relationship SET source=? WHERE kb_id=? AND source IN (`+ph+`)`, append([]any{keep, kbID}, toAny(merge)...)...)
	if err != nil {
		return 0, 0, err
	}
	movedRels += mustCount(res)
	res, err = tx.Exec(`UPDATE kg_relationship SET target=? WHERE kb_id=? AND target IN (`+ph+`)`, append([]any{keep, kbID}, toAny(merge)...)...)
	if err != nil {
		return 0, 0, err
	}
	movedRels += mustCount(res)
	// 自环清理（merge 的两端都被改到 keep）
	if _, err = tx.Exec(`DELETE FROM kg_relationship WHERE kb_id=? AND source=target`, kbID); err != nil {
		return 0, 0, err
	}
	// claims 迁移（同 subject+text 去重由唯一业务语义粗判：仅迁移 subject 命中）
	res, err = tx.Exec(`UPDATE kg_claim SET subject=? WHERE kb_id=? AND subject IN (`+ph+`)`, append([]any{keep, kbID}, toAny(merge)...)...)
	if err != nil {
		return 0, 0, err
	}
	movedClaims += mustCount(res)
	// M36/KB-7②：被并实体的名字并入 keep 的别名（合并后旧名仍可被检索/搜索命中）
	var keepAlias string
	_ = tx.QueryRow(`SELECT alias FROM kg_entity WHERE kb_id=? AND name=?`, kbID, keep).Scan(&keepAlias)
	for _, m := range merge {
		var a string
		_ = tx.QueryRow(`SELECT alias FROM kg_entity WHERE kb_id=? AND name=?`, kbID, m).Scan(&a)
		keepAlias = joinAlias(keepAlias, m, a)
	}
	if _, err = tx.Exec(`UPDATE kg_entity SET alias=? WHERE kb_id=? AND name=?`, keepAlias, kbID, keep); err != nil {
		return 0, 0, err
	}
	// 删除 merge 实体
	if _, err = tx.Exec(`DELETE FROM kg_entity WHERE kb_id=? AND name IN (`+ph+`)`, base...); err != nil {
		return 0, 0, err
	}
	// 实体向量孤儿清理（KB-4②③/M287：被并实体缓存回收；keep 行别名变化不影响向量指纹）
	if err := deleteOrphanKGEntityVecsTx(tx, kbID); err != nil {
		return 0, 0, err
	}
	return movedRels, movedClaims, tx.Commit()
}

// joinAlias 别名归并：原名 + 原别名追加（分号分隔，去重去空，别名段数上限 8 防膨胀）。
func joinAlias(existing string, names ...string) string {
	seen := map[string]bool{}
	parts := []string{}
	for _, seg := range strings.Split(existing, ";") {
		if seg = strings.TrimSpace(seg); seg != "" && !seen[seg] {
			seen[seg] = true
			parts = append(parts, seg)
		}
	}
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" && !seen[n] {
			seen[n] = true
			parts = append(parts, n)
		}
	}
	if len(parts) > 8 {
		parts = parts[:8]
	}
	return strings.Join(parts, ";")
}

// SetKGEntityAlias 设置实体别名（KB-7② 人工入口；空串清除）。
func (s *Store) SetKGEntityAlias(kbID, name, alias string) error {
	alias = joinAlias("", strings.Split(alias, ";")...) // 归一：去空白段、段数上限
	res, err := s.DB.Exec(`UPDATE kg_entity SET alias=? WHERE kb_id=? AND name=?`, alias, kbID, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// mustCount 取 RowsAffected 的计数值（忽略 error——教学口径，删除/更新失败由上游 err 把守）。
func mustCount(res sql.Result) int {
	n, _ := res.RowsAffected()
	return int(n)
}

func toAny(ss []string) []any {
	out := make([]any, 0, len(ss))
	for _, v := range ss {
		out = append(out, v)
	}
	return out
}

// KGQuality 质量面板数据（REQ-129④）：method 分布（审计决策 meta_json）、孤儿实体数、
// 高频关系类型 TopN、rejected 计数（关系/claims）。
type KGQuality struct {
	MethodDist    map[string]int `json:"method_dist"`
	OrphanEntity  int            `json:"orphan_entity"`
	TopRelTypes   []RelTypeCount `json:"top_rel_types"`
	RejectedRels  int            `json:"rejected_rels"`
	RejectedClms  int            `json:"rejected_claims"`
	Entities      int            `json:"entities"`
	Relationships int            `json:"relationships"`
}

type RelTypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

func (s *Store) KGQuality(kbID string) (*KGQuality, error) {
	q := &KGQuality{MethodDist: map[string]int{}}
	// method 分布：最近 kg 决策行 meta_json 的 method 字段
	rows, err := s.DB.Query(`SELECT meta_json FROM onto_decision WHERE subject_kind='kg' AND subject_id=? ORDER BY created_at DESC LIMIT 50`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var meta string
		if err := rows.Scan(&meta); err != nil {
			return nil, err
		}
		var m struct {
			Method string `json:"method"`
		}
		if json.Unmarshal([]byte(meta), &m) == nil && m.Method != "" {
			q.MethodDist[m.Method]++
		}
	}
	rows.Err()
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM kg_entity WHERE kb_id=?`, kbID).Scan(&q.Entities); err != nil {
		return nil, err
	}
	// 孤儿实体：不出现在任何关系边的实体
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM kg_entity e WHERE kb_id=? AND NOT EXISTS (
		SELECT 1 FROM kg_relationship r WHERE r.kb_id=? AND (r.source=e.name OR r.target=e.name))`, kbID, kbID).Scan(&q.OrphanEntity); err != nil {
		return nil, err
	}
	trows, err := s.DB.Query(`SELECT rel_type, COUNT(*) n FROM kg_relationship WHERE kb_id=? GROUP BY rel_type ORDER BY n DESC LIMIT 5`, kbID)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var t string
		var n int
		if err := trows.Scan(&t, &n); err != nil {
			return nil, err
		}
		q.TopRelTypes = append(q.TopRelTypes, RelTypeCount{Type: t, Count: n})
	}
	trows.Err()
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM kg_relationship WHERE kb_id=? AND status='rejected'`, kbID).Scan(&q.RejectedRels); err != nil {
		return nil, err
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM kg_claim WHERE kb_id=? AND status='rejected'`, kbID).Scan(&q.RejectedClms); err != nil {
		return nil, err
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM kg_relationship WHERE kb_id=?`, kbID).Scan(&q.Relationships); err != nil {
		return nil, err
	}
	return q, nil
}

// MergeSuggestion 系统合并建议（别名消歧粗规则：名称互相包含且类型相同 → 建议合并，仅提示不自动执行）。
type MergeSuggestion struct {
	Keep   string `json:"keep"`
	Merge  string `json:"merge"`
	Reason string `json:"reason"`
}

// KGMergeSuggestions 合并建议（教学口径：包含关系的同类型实体对，短名为 keep）。
func (s *Store) KGMergeSuggestions(kbID string) ([]MergeSuggestion, error) {
	ents, _, err := s.KGByKB(kbID)
	if err != nil {
		return nil, err
	}
	out := []MergeSuggestion{}
	for i, a := range ents {
		for _, b := range ents[i+1:] {
			if a.Type != b.Type || a.Name == b.Name {
				continue
			}
			long, short := a.Name, b.Name
			if len([]rune(short)) > len([]rune(long)) {
				long, short = short, long
			}
			if len([]rune(short)) >= 2 && strings.Contains(long, short) {
				out = append(out, MergeSuggestion{Keep: short, Merge: long,
					Reason: fmt.Sprintf("%q 是 %q 的子串且类型相同（%s）", short, long, a.Type)})
			}
			if len(out) >= 20 {
				return out, nil
			}
		}
	}
	return out, nil
}

// KGEntitiesByChunks KB-16/M286 倒排路径：命中 chunk → 提及实体（uid+name，kg_chunk_entity 表）。
// 检索期主路径（替代 claim 反查唯一载体）；KGSubjectsByChunks 保留为两路对照与降级。
func (s *Store) KGEntitiesByChunks(kbID string, chunkIDs []string) ([]*KGChunkEntityRef, error) {
	if len(chunkIDs) == 0 {
		return nil, nil
	}
	q := `SELECT entity_uid, name, chunk_id FROM kg_chunk_entity WHERE kb_id = ? AND chunk_id IN (` +
		strings.TrimRight(strings.Repeat("?,", len(chunkIDs)), ",") + `) ORDER BY entity_uid`
	args := make([]any, 0, len(chunkIDs)+1)
	args = append(args, kbID)
	for _, id := range chunkIDs {
		args = append(args, id)
	}
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KGChunkEntityRef{}
	for rows.Next() {
		r := &KGChunkEntityRef{}
		if err := rows.Scan(&r.UID, &r.Name, &r.ChunkID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// KGChunkSubjectsByChunks 倒排路径的名称投影（graphrag 一跳扩展 drop-in——与
// KGSubjectsByChunks claim 反查同名同集，验收「两路结果一致」的检索侧消费形态）。
func (s *Store) KGChunkSubjectsByChunks(kbID string, chunkIDs []string) ([]string, error) {
	refs, err := s.KGEntitiesByChunks(kbID, chunkIDs)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, r := range refs {
		if r.Name != "" && !seen[r.Name] {
			seen[r.Name] = true
			out = append(out, r.Name)
		}
	}
	return out, nil
}

// KGChunkEntityRef 倒排行投影（uid+展示名；ChunkID=来源 chunk——KB-4②③/M287 融合打分
// 需要实体继承其所属 chunk 的向量命中分，只增字段向后兼容）。
type KGChunkEntityRef struct {
	UID     string `json:"uid"`
	Name    string `json:"name"`
	ChunkID string `json:"chunk_id,omitempty"`
}

// KGCommunityVec 社区摘要向量缓存（KB-18/M290 embedding 臂；label 对齐 kg_community.label）。
type KGCommunityVec struct {
	KBID      string
	Label     string
	Vec       []byte
	Dim       int
	UpdatedAt string
}

// UpsertKGCommunityVecs 批量写社区向量缓存（重建/惰性补嵌共用）。
func (s *Store) UpsertKGCommunityVecs(kbID string, items map[string][]byte, dim int) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO kg_community_vec(kb_id,label,vec,dim,updated_at) VALUES (?,?,?,?,?)
		ON CONFLICT(kb_id,label) DO UPDATE SET vec=excluded.vec, dim=excluded.dim, updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	ts := now()
	for label, vec := range items {
		if _, err := stmt.Exec(kbID, label, vec, dim, ts); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListKGCommunityVecs 库内社区向量缓存（检索期融合臂读取；空 map=缓存缺失纯词面回退）。
func (s *Store) ListKGCommunityVecs(kbID string) (map[string][]byte, int, error) {
	rows, err := s.DB.Query(`SELECT label, vec, dim FROM kg_community_vec WHERE kb_id = ?`, kbID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[string][]byte{}
	dim := 0
	for rows.Next() {
		var label string
		var vec []byte
		var d int
		if err := rows.Scan(&label, &vec, &d); err != nil {
			return nil, 0, err
		}
		out[label] = vec
		if dim == 0 {
			dim = d
		}
	}
	return out, dim, rows.Err()
}
