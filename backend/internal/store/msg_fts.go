package store

// REQ-528/M335（P18）：会话内容检索——conversations.messages 接入 FTS5（沿 chunk_fts 同族
// 模式：trigram 分词 CJK 友好+惰性建表+存量回填+写穿），「找回上次结论」单用户核心复用动线
// 补全（原仅标题子串过滤——消息正文不可搜）。短期前端过滤（消息已全量拉取）由 ChatWindow
// 承载；本文件为中期 FTS 档：跨会话按内容定位（会话 id+消息摘要），Sidebar 搜索框两档消费。

import (
	"log"
	"strings"
)

// MsgFTSReady 惰性探测 FTS5 并确保 message_fts 虚拟表+存量回填（进程内一次；写失败仅告警
// 不阻断启动——与 chunk_fts 同口径：检索面降级回标题子串，语义与历史一致）。
func (s *Store) MsgFTSReady() bool {
	if s == nil || s.DB == nil {
		return false
	}
	s.msgFtsOnce.Do(func() {
		if _, err := s.DB.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS message_fts USING fts5(content, message_id UNINDEXED, conversation_id UNINDEXED, tokenize='trigram')`); err != nil {
			log.Printf("[store] FTS5 不可用，会话内容检索退役（标题子串照常）: %v", err)
			return
		}
		if _, err := s.DB.Exec(`INSERT INTO message_fts(content, message_id, conversation_id)
			SELECT content, id, conversation_id FROM message
			WHERE id NOT IN (SELECT message_id FROM message_fts)`); err != nil {
			log.Printf("[store] message_fts 存量回填失败（内容检索覆盖不全，重启自动补）: %v", err)
		}
		s.msgFtsOK = true
	})
	return s.msgFtsOK
}

// MsgFTSHit 会话内容检索命中（会话+消息定位+摘要）。
type MsgFTSHit struct {
	ConversationID string
	MessageID      string
	Role           string
	Content        string // 全文（前端截摘要）
	Score          float64
}

// SearchMessagesFTS 跨会话消息内容检索（BM25 排序；query 须 FTSQuote 转义，<3 rune 由调用方跳过）。
// role 过滤可选（assistant=只搜回答——「上次问的结论」高频面）。
func (s *Store) SearchMessagesFTS(query, role string, limit int) ([]MsgFTSHit, error) {
	if !s.MsgFTSReady() || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	where := `WHERE fts.message_fts MATCH ?`
	args := []any{FTSQuote(query)}
	if role != "" {
		where += ` AND m.role = ?`
		args = append(args, role)
	}
	args = append(args, limit)
	rows, err := s.DB.Query(`SELECT m.conversation_id, m.id, m.role, m.content,
		bm25(message_fts) FROM message_fts fts
		JOIN message m ON m.id = fts.message_id `+where+` ORDER BY bm25(message_fts) LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MsgFTSHit{}
	for rows.Next() {
		var h MsgFTSHit
		if err := rows.Scan(&h.ConversationID, &h.MessageID, &h.Role, &h.Content, &h.Score); err != nil {
			return nil, err
		}
		h.Score = -h.Score // bm25 取负转正（越大越相关——chunk_fts 同口径）
		out = append(out, h)
	}
	return out, rows.Err()
}

// InsertMessageFTS 写穿：InsertMessage 成功后同步入 FTS（失败仅告警——辅助索引，检索面
// 回退标题子串；残留行经回填幂等补齐）。
func (s *Store) InsertMessageFTS(m *Message) {
	if m == nil || !s.MsgFTSReady() {
		return
	}
	// 幂等写（NOT IN 守卫）——首条消息的写穿与 MsgFTSReady 存量回填竞态（回填先拾取该行，
	// 显式写再插即重复，JOIN 检索出双行）；chunk_fts 回填同守卫口径。
	if _, err := s.DB.Exec(`INSERT INTO message_fts(content, message_id, conversation_id)
		SELECT ?,?,? WHERE ? NOT IN (SELECT message_id FROM message_fts)`,
		m.Content, m.ID, m.ConversationID, m.ID); err != nil {
		log.Printf("[store] message_fts 写穿失败（重启回填补齐）: %v", err)
	}
}
