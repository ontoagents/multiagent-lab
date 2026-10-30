package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---- 知识库（M6，方案 §5.2/§6.9）----

// KnowledgeBase 知识库。
type KnowledgeBase struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Mode        string  `json:"mode"` // rag | graphrag | wiki（REQ-241 起第三类型；KB-11 起为展示页签默认与存量兼容口径，路由按能力开关 + wiki 臂按 mode）
	TopK        int     `json:"top_k"`
	MinScore    float64 `json:"min_score"`
	// KB-11（M35/D2）：检索能力开关（解除 mode 库级二选一互斥；tab 对照展示保留）。
	// 归一规则：两者皆 false 时按 mode 派生（无能力的库无意义）。
	KBVector bool `json:"kb_vector"`
	KBGraph  bool `json:"kb_graph"`
	// KG 抽取治理配置（M16/REQ-129①）：库级抽取模型连接与提示词覆写（空 = 默认）
	KGConnID string `json:"kg_conn_id,omitempty"`
	KGPrompt string `json:"kg_prompt,omitempty"`
	// M36/KB-6：本体约束抽取挂载（I3 裁定：ontology_id 挂抽取配置侧，本体只读词表注入 prompt；
	// 空 = 自由抽取）与库级语料预算（0 = 默认 200 chunks，解除原 60 硬截断）。
	KGOntologyID string `json:"kg_ontology_id,omitempty"`
	KGMaxChunks  int    `json:"kg_max_chunks,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	// KB-20/M292：库级 embedding 维度登记（0=未登记）——首库索引时登记，写入/检索双端校验
	// 维度漂移（换 embedding 模型即拒绝写/检索报错+重建指引；KB-O3 库级维度锁定工程侧落地）。
	EmbedDim int `json:"embed_dim"`
	// Official 平台官方库标注（REQ-451：api 层按 chat.PlatformKBName 判定注入，非持久化列——
	// 前端「官方·平台知识」徽标单源；EnsurePlatformKB 按名称幂等，名称即稳定标识）。
	Official bool `json:"official,omitempty"`
}

// KnowledgeDoc 知识文档（索引状态机：pending→indexing→success|failed）。
type KnowledgeDoc struct {
	ID         string `json:"id"`
	KBID       string `json:"kb_id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	ChunkCount int    `json:"chunk_count"`
	// Dim 向量维度（REQ-439/REQ-54：12 §3.3 既有口径补实现；wiki 库 chunksOnly 零 embedding，0=如实零维）
	Dim   int    `json:"dim"`
	Error string `json:"error"`
	Source     string `json:"source"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	// REQ-241：wiki 摄取输入指纹（chunk 拼接 sha256；重建时与现值一致 = 未变更跳过 summary 生成）
	WikiHash string `json:"-"`
	// Graphrag M14 D-KB4：graphrag 模式文档索引后的 KG 抽取结果。
	// REQ-440/KB-8③：经 graphrag_json 持久化（原「仅当次接口回显」——异步化后必须落库，
	// KG 徽标/失败原因刷新不丢）；nil = rag 模式或尚无抽取结果。
	Graphrag *GraphragInfo `json:"graphrag,omitempty"`
	// GraphragJSON GraphragInfo 的落库承载（内部字段，非 API 面；空 = 无）
	GraphragJSON string `json:"-"`
}

// GraphragInfo graphrag 文档与 KG 抽取的联动结果（M14 ②⑥ 建制；D-O15 起自研抽取，degraded 不阻断语义保留）。
type GraphragInfo struct {
	OK            bool     `json:"ok"`
	Method        string   `json:"method,omitempty"` // llm | lightweight（D-O15 自研抽取方式）
	Chunks        int      `json:"chunks,omitempty"`
	Entities      int      `json:"entities,omitempty"`
	Relationships int      `json:"relationships,omitempty"`
	Degraded      bool     `json:"degraded,omitempty"`
	Error         string   `json:"error,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

// KnowledgeChunk 知识片段。
type KnowledgeChunk struct {
	ID      string `json:"id"`
	KBID    string `json:"kb_id"`
	DocID   string `json:"doc_id"`
	Seq     int    `json:"seq"`
	Content string `json:"content"`
	// KB-10②：父块内容冗余（子块检索、父块召回上下文；空=自身即独立块）
	ParentContent string `json:"-"`
	// Vector 仅 SQLite fallback 路径有值（float32 小端 BLOB）
	Vector       []byte `json:"-"`
	VectorRef    string `json:"vector_ref,omitempty"` // Qdrant point id
	StoreBackend string `json:"store_backend"`
	CreatedAt    string `json:"created_at"`
}

const kbCols = `id,name,description,mode,top_k,min_score,kb_vector,kb_graph,kg_conn_id,kg_prompt,kg_ontology_id,kg_max_chunks,embed_dim,created_at,updated_at`
const kdocCols = `id,kb_id,title,status,chunk_count,dim,error,source,created_at,updated_at,wiki_hash,graphrag_json`
const kchunkCols = `id,kb_id,doc_id,seq,content,parent_content,vector,vector_ref,store_backend,created_at`

func scanKB(row interface{ Scan(...any) error }) (*KnowledgeBase, error) {
	var k KnowledgeBase
	var kv, kg int
	if err := row.Scan(&k.ID, &k.Name, &k.Description, &k.Mode, &k.TopK, &k.MinScore, &kv, &kg, &k.KGConnID, &k.KGPrompt, &k.KGOntologyID, &k.KGMaxChunks, &k.EmbedDim, &k.CreatedAt, &k.UpdatedAt); err != nil {
		return nil, err
	}
	k.KBVector, k.KBGraph = kv != 0, kg != 0
	if k.Mode == "" {
		k.Mode = "rag" // 迁移前的老行（008 未跑时列不存在不会走到这；防御默认）
	}
	return &k, nil
}

func scanKDoc(row interface{ Scan(...any) error }) (*KnowledgeDoc, error) {
	var d KnowledgeDoc
	if err := row.Scan(&d.ID, &d.KBID, &d.Title, &d.Status, &d.ChunkCount, &d.Dim, &d.Error, &d.Source, &d.CreatedAt, &d.UpdatedAt, &d.WikiHash, &d.GraphragJSON); err != nil {
		return nil, err
	}
	if d.GraphragJSON != "" { // REQ-440：KG 抽取结果落库回读（老行/rag 模式为空跳过）
		var gi GraphragInfo
		if json.Unmarshal([]byte(d.GraphragJSON), &gi) == nil {
			d.Graphrag = &gi
		}
	}
	return &d, nil
}

func scanKChunk(row interface{ Scan(...any) error }) (*KnowledgeChunk, error) {
	var c KnowledgeChunk
	if err := row.Scan(&c.ID, &c.KBID, &c.DocID, &c.Seq, &c.Content, &c.ParentContent, &c.Vector, &c.VectorRef, &c.StoreBackend, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

// ListKnowledgeBases 全部知识库（按创建时间升序）。
func (s *Store) ListKnowledgeBases() ([]*KnowledgeBase, error) {
	rows, err := s.DB.Query(`SELECT ` + kbCols + ` FROM knowledge_base ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KnowledgeBase
	for rows.Next() {
		k, err := scanKB(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// GetKnowledgeBase 按 ID 查询。
func (s *Store) GetKnowledgeBase(id string) (*KnowledgeBase, error) {
	k, err := scanKB(s.DB.QueryRow(`SELECT `+kbCols+` FROM knowledge_base WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return k, err
}

// normalizeKBCapabilities KB-11 能力归一：两者皆 false = 未声明/无意义 → 按 mode 派生（D2 存量口径同式）。
// REQ-241：wiki 库检索读页非读片段（零 embedding 依赖），两开关保持 false（路由按 mode 进 wiki 臂）。
func normalizeKBCapabilities(k *KnowledgeBase) {
	if k.Mode == "wiki" {
		k.KBVector, k.KBGraph = false, false
		return
	}
	if !k.KBVector && !k.KBGraph {
		if k.Mode == "graphrag" {
			k.KBGraph = true
		} else {
			k.KBVector = true
		}
	}
}

// normalizeKBMode mode 值域白名单（REQ-241 起三值；未知值回落 rag 兼容存量脏数据）。
func normalizeKBMode(mode string) string {
	switch mode {
	case "graphrag", "wiki":
		return mode
	default:
		return "rag"
	}
}

// CreateKnowledgeBase 新建（name 唯一冲突 ErrConflict）。
func (s *Store) CreateKnowledgeBase(k *KnowledgeBase) (*KnowledgeBase, error) {
	if strings.TrimSpace(k.Name) == "" {
		return nil, &HTTPError{Status: 400, Msg: "name is required"}
	}
	if k.ID == "" {
		k.ID = NewID()
	}
	if k.TopK <= 0 {
		k.TopK = 4
	}
	k.Mode = normalizeKBMode(k.Mode)
	normalizeKBCapabilities(k)
	_, err := s.DB.Exec(`INSERT INTO knowledge_base (`+kbCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		k.ID, k.Name, k.Description, k.Mode, k.TopK, k.MinScore, k.KBVector, k.KBGraph, k.KGConnID, k.KGPrompt, k.KGOntologyID, k.KGMaxChunks, k.EmbedDim, now(), now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return s.GetKnowledgeBase(k.ID)
}

// ListKBsByKGOntology REQ-233②：以指定本体为约束词表（kg_ontology_id）的知识库清单（引用聚合用）。
func (s *Store) ListKBsByKGOntology(ontologyID string) ([]*KnowledgeBase, error) {
	rows, err := s.DB.Query(`SELECT `+kbCols+` FROM knowledge_base WHERE kg_ontology_id=? ORDER BY name`, ontologyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KnowledgeBase
	for rows.Next() {
		k, err := scanKB(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// UpdateKnowledgeBase 全量更新。
func (s *Store) UpdateKnowledgeBase(k *KnowledgeBase) (*KnowledgeBase, error) {
	k.Mode = normalizeKBMode(k.Mode)
	normalizeKBCapabilities(k)
	res, err := s.DB.Exec(`UPDATE knowledge_base SET name=?,description=?,mode=?,top_k=?,min_score=?,kb_vector=?,kb_graph=?,kg_conn_id=?,kg_prompt=?,kg_ontology_id=?,kg_max_chunks=?,embed_dim=?,updated_at=? WHERE id=?`,
		k.Name, k.Description, k.Mode, k.TopK, k.MinScore, k.KBVector, k.KBGraph, k.KGConnID, k.KGPrompt, k.KGOntologyID, k.KGMaxChunks, k.EmbedDim, now(), k.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrConflict
		}
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetKnowledgeBase(k.ID)
}

// DeleteKnowledgeBase 级联删除（chunks/docs 由调用方先清理向量库后调用）。
func (s *Store) DeleteKnowledgeBase(id string) error {
	res, err := s.DB.Exec(`DELETE FROM knowledge_base WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.DB.Exec(`DELETE FROM knowledge_chunk WHERE kb_id = ?`, id)
	s.DeleteChunkFTSByKB(id) // KB-10①：词法索引级联清理
	s.DB.Exec(`DELETE FROM knowledge_doc WHERE kb_id = ?`, id)
	s.DeleteWikiPagesByKB(id) // REQ-241：wiki 页级联清理
	s.deleteKG(id, "")        // D-O15：KG 自存行级联清理
	return nil
}

// ---- Docs ----

// ListKnowledgeDocs 库内文档（按创建时间升序）。
func (s *Store) ListKnowledgeDocs(kbID string) ([]*KnowledgeDoc, error) {
	rows, err := s.DB.Query(`SELECT `+kdocCols+` FROM knowledge_doc WHERE kb_id = ? ORDER BY created_at, id`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KnowledgeDoc
	for rows.Next() {
		d, err := scanKDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetKnowledgeDoc 单文档。
func (s *Store) GetKnowledgeDoc(id string) (*KnowledgeDoc, error) {
	d, err := scanKDoc(s.DB.QueryRow(`SELECT `+kdocCols+` FROM knowledge_doc WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// CreateKnowledgeDoc 新建文档（初始 pending）。
func (s *Store) CreateKnowledgeDoc(d *KnowledgeDoc) (*KnowledgeDoc, error) {
	if d.ID == "" {
		d.ID = NewID()
	}
	if d.Status == "" {
		d.Status = "pending"
	}
	if d.Source == "" {
		d.Source = "paste"
	}
	_, err := s.DB.Exec(`INSERT INTO knowledge_doc (`+kdocCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.ID, d.KBID, d.Title, d.Status, d.ChunkCount, d.Dim, d.Error, d.Source, now(), now(), d.WikiHash, d.GraphragJSON)
	if err != nil {
		return nil, err
	}
	return s.GetKnowledgeDoc(d.ID)
}

// UpdateKnowledgeDocStatus 索引状态回写（indexing/success/failed + chunk_count/dim/error；
// REQ-439：dim 随 success 回写——embedding 维度库内单值可辨，换模型不一致提示沿 KB-O3 池）。
func (s *Store) UpdateKnowledgeDocStatus(id, status string, chunkCount, dim int, errMsg string) error {
	_, err := s.DB.Exec(`UPDATE knowledge_doc SET status=?,chunk_count=?,dim=?,error=?,updated_at=? WHERE id=?`,
		status, chunkCount, dim, errMsg, now(), id)
	return err
}

// SetDocGraphrag KG 抽取结果落库（REQ-440/KB-8③：异步化后结果持久可见——KG 徽标/失败原因刷新不丢）。
func (s *Store) SetDocGraphrag(kbID, docID, js string) error {
	_, err := s.DB.Exec(`UPDATE knowledge_doc SET graphrag_json=?, updated_at=? WHERE id=? AND kb_id=?`,
		js, now(), docID, kbID)
	return err
}

// DeleteKnowledgeDoc 删除文档及其 chunks（向量库清理由调用方先行）。
func (s *Store) DeleteKnowledgeDoc(kbID, docID string) error {
	res, err := s.DB.Exec(`DELETE FROM knowledge_doc WHERE id = ? AND kb_id = ?`, docID, kbID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.DB.Exec(`DELETE FROM knowledge_chunk WHERE doc_id = ?`, docID)
	s.deleteKG(kbID, docID) // D-O15：KG 自存行级联清理
	return nil
}

// ---- Chunks ----

// InsertKnowledgeChunks 批量写入 chunks（事务）。
// Qdrant 路径：vector 为 nil、vectorRef=point id、backend=qdrant；
// SQLite 路径：vector=BLOB、backend=sqlite。
func (s *Store) InsertKnowledgeChunks(chunks []*KnowledgeChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO knowledge_chunk (` + kchunkCols + `) VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, c := range chunks {
		if c.ID == "" {
			c.ID = NewID()
		}
		if c.CreatedAt == "" {
			c.CreatedAt = now().UTC().Format(time.RFC3339)
		}
		if _, err := stmt.Exec(c.ID, c.KBID, c.DocID, c.Seq, c.Content, c.ParentContent, c.Vector, c.VectorRef, c.StoreBackend, c.CreatedAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.UpsertChunkFTS(chunks) // KB-10①：词法索引同步（辅助索引，失败仅告警）
	return nil
}

// ListKnowledgeChunksByDoc 按 doc 取 chunks（重索引前清理/调试用）。
func (s *Store) ListKnowledgeChunksByDoc(docID string) ([]*KnowledgeChunk, error) {
	rows, err := s.DB.Query(`SELECT `+kchunkCols+` FROM knowledge_chunk WHERE doc_id = ? ORDER BY seq`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KnowledgeChunk
	for rows.Next() {
		c, err := scanKChunk(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListKnowledgeChunksByKB 按库取全部 chunks（O13 策略 A chunk 池：按 doc→seq 稳定排序喂 LLM）。
func (s *Store) ListKnowledgeChunksByKB(kbID string) ([]*KnowledgeChunk, error) {
	rows, err := s.DB.Query(`SELECT `+kchunkCols+` FROM knowledge_chunk WHERE kb_id = ? ORDER BY doc_id, seq`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KnowledgeChunk
	for rows.Next() {
		c, err := scanKChunk(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteKnowledgeChunksByDoc 删除文档全部 chunks。
func (s *Store) DeleteKnowledgeChunksByDoc(docID string) error {
	s.DeleteChunkFTSByDoc(docID) // KB-10①：词法索引级联清理
	_, err := s.DB.Exec(`DELETE FROM knowledge_chunk WHERE doc_id = ?`, docID)
	return err
}

// DocTitles 批量取文档标题（retrieval 事件 hits[].doc 需要）。
func (s *Store) DocTitles(ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	q := `SELECT id,title FROM knowledge_doc WHERE id IN (` + strings.TrimRight(strings.Repeat("?,", len(ids)), ",") + `)`
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		out[id] = title
	}
	return out, rows.Err()
}

// SearchChunksSQLite SQLite fallback 余弦 TopK（§6.9：无外部依赖对照实现）。
// 返回带 Content/DocID/Seq/Vector 的 chunks，由 kb 层计算相似度。
func (s *Store) SearchChunksSQLite(kbID string) ([]*KnowledgeChunk, error) {
	rows, err := s.DB.Query(`SELECT `+kchunkCols+` FROM knowledge_chunk WHERE kb_id = ? AND vector IS NOT NULL ORDER BY doc_id, seq`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KnowledgeChunk
	for rows.Next() {
		c, err := scanKChunk(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MarshalJSON 调试辅助（避免 chunk 内容转 JSON 时带二进制）。
func (c *KnowledgeChunk) MarshalChunkMeta() string {
	b, _ := json.Marshal(map[string]any{"id": c.ID, "doc_id": c.DocID, "seq": c.Seq, "backend": c.StoreBackend})
	return string(b)
}

var _ = fmt.Sprintf

// SetKnowledgeBaseEmbedDim KB-20/M292：库级 embedding 维度登记（仅 0→dim 首登记生效；
// 已登记不覆盖——解锁走整库重建路径。返回是否发生了登记）。
func (s *Store) SetKnowledgeBaseEmbedDim(kbID string, dim int) (bool, error) {
	if dim <= 0 {
		return false, nil
	}
	res, err := s.DB.Exec(`UPDATE knowledge_base SET embed_dim=?, updated_at=? WHERE id=? AND embed_dim=0`, dim, now(), kbID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// KBRebuildJob 库级重建任务（KB-8②/M288：kg/communities/wiki 三端点异步化落库——
// REQ-381 async_job 模式复用，单机基线不引入外部队列）。
type KBRebuildJob struct {
	ID         string `json:"id"`
	KBID       string `json:"kb_id"`
	Kind       string `json:"kind"` // kg | communities | wiki
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	ResultJSON string `json:"-"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// CreateKBRebuildJob 新建 running 任务。
func (s *Store) CreateKBRebuildJob(kbID, kind string) (*KBRebuildJob, error) {
	j := &KBRebuildJob{ID: NewID(), KBID: kbID, Kind: kind, Status: "running", CreatedAt: now().UTC().Format(time.RFC3339), UpdatedAt: now().UTC().Format(time.RFC3339)}
	_, err := s.DB.Exec(`INSERT INTO kb_rebuild_job(id,kb_id,kind,status,error,result_json,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		j.ID, j.KBID, j.Kind, j.Status, j.Error, j.ResultJSON, j.CreatedAt, j.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return j, nil
}

// FinishKBRebuildJob 终态落库（success/failed+结果 JSON）。
func (s *Store) FinishKBRebuildJob(id, status, errMsg, resultJSON string) error {
	_, err := s.DB.Exec(`UPDATE kb_rebuild_job SET status=?, error=?, result_json=?, updated_at=? WHERE id=?`,
		status, errMsg, resultJSON, now(), id)
	return err
}

// ListKBRebuildJobs 库内重建任务（新→旧；进度轮询面）。
func (s *Store) ListKBRebuildJobs(kbID string, limit int) ([]*KBRebuildJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.Query(`SELECT id,kb_id,kind,status,error,result_json,created_at,updated_at FROM kb_rebuild_job WHERE kb_id=? ORDER BY created_at DESC, id DESC LIMIT ?`, kbID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KBRebuildJob{}
	for rows.Next() {
		var j KBRebuildJob
		if err := rows.Scan(&j.ID, &j.KBID, &j.Kind, &j.Status, &j.Error, &j.ResultJSON, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &j)
	}
	return out, rows.Err()
}

// GetKBRebuildJob 单任务回读（结果 JSON 供终态取回）。
func (s *Store) GetKBRebuildJob(id string) (*KBRebuildJob, error) {
	var j KBRebuildJob
	err := s.DB.QueryRow(`SELECT id,kb_id,kind,status,error,result_json,created_at,updated_at FROM kb_rebuild_job WHERE id=?`, id).
		Scan(&j.ID, &j.KBID, &j.Kind, &j.Status, &j.Error, &j.ResultJSON, &j.CreatedAt, &j.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &j, err
}

// MarkStaleRebuildJobs 启动对账（KB-21 同门）：进程重启时 running 任务→failed（进度丢失如实明示）。
func (s *Store) MarkStaleRebuildJobs() (int64, error) {
	res, err := s.DB.Exec(`UPDATE kb_rebuild_job SET status='failed', error='进程重启致任务中断——请重新触发重建', updated_at=? WHERE status='running'`, now())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
