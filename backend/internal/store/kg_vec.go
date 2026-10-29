package store

// ---- KB-4②③/M287 实体向量缓存（kg_entity_vec，迁移 071）----
//
// 实体「名称+描述」embedding 的落库缓存：抽取期（GraphragIngest 后刷新缺向量/内容变化实体）、
// 检索期（双入口：query 向量 vs 实体向量余弦 TopN 并入种子集）。缓存键=entity_uid（KB-16
// 稳定 id），实体行删除由 ReplaceKGForDoc/MergeKGEntities/deleteKG 的孤儿清理同式回收。
// 教学规模（实体数百~千级）全库读入内存余弦（35 号 KB-4 草案「SQLite BLOB 余弦」口径）。

import (
	"database/sql"
	"fmt"
)

// KGEntityVec 实体向量缓存行（dim 逐行登记——KB-20 维度漂移检测的承载列）。
type KGEntityVec struct {
	KBID        string `json:"kb_id"`
	UID         string `json:"entity_uid"`
	Dim         int    `json:"dim"`
	Vec         []byte `json:"-"`
	Fingerprint string `json:"fingerprint"`
	UpdatedAt   string `json:"updated_at"`
}

// KGEntityVecs 读全库实体向量（检索期内存余弦用；实体多时按需分页留后续——教学规模直读）。
func (s *Store) KGEntityVecs(kbID string) ([]*KGEntityVec, error) {
	rows, err := s.DB.Query(`SELECT kb_id, entity_uid, dim, vec, fingerprint, updated_at FROM kg_entity_vec WHERE kb_id = ?`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KGEntityVec{}
	for rows.Next() {
		r := &KGEntityVec{}
		if err := rows.Scan(&r.KBID, &r.UID, &r.Dim, &r.Vec, &r.Fingerprint, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertKGEntityVecs 批量写入实体向量（PK=entity_uid 冲突覆盖——同一实体重算即刷新）。
func (s *Store) UpsertKGEntityVecs(rows []*KGEntityVec) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT INTO kg_entity_vec(kb_id, entity_uid, dim, vec, fingerprint, updated_at) VALUES (?,?,?,?,?,?)
		ON CONFLICT(entity_uid) DO UPDATE SET kb_id=excluded.kb_id, dim=excluded.dim, vec=excluded.vec,
		fingerprint=excluded.fingerprint, updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, r := range rows {
		if _, err := st.Exec(r.KBID, r.UID, r.Dim, r.Vec, r.Fingerprint, r.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteOrphanKGEntityVecs 孤儿向量清理：entity_uid 不在实体表（实体被替换/合并/删除后回收缓存）。
// deleteKG（级联后）直呼；ReplaceKGForDoc/MergeKGEntities 用事务内变体同式。
func (s *Store) DeleteOrphanKGEntityVecs(kbID string) error {
	_, err := s.DB.Exec(orphanVecSQL, kbID, kbID)
	return err
}

// deleteOrphanKGEntityVecsTx 事务内变体（与实体删除同事务提交，失败回滚整体）。
func deleteOrphanKGEntityVecsTx(tx *sql.Tx, kbID string) error {
	_, err := tx.Exec(orphanVecSQL, kbID, kbID)
	return err
}

// KGEntityNamesOfKB 全库实体 uid→展示名投影（实体向量臂把 kg_entity_vec 的 uid 行
// 映射回名称——uid 不作 name 反解（KB-16 口径），轻量单查不拉全实体行）。
func (s *Store) KGEntityNamesOfKB(kbID string) (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT uid, name FROM kg_entity WHERE kb_id = ? AND uid != ''`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var uid, name string
		if err := rows.Scan(&uid, &name); err != nil {
			return nil, err
		}
		out[uid] = name
	}
	return out, rows.Err()
}

const orphanVecSQL = `DELETE FROM kg_entity_vec WHERE kb_id = ? AND entity_uid NOT IN
	(SELECT uid FROM kg_entity WHERE kb_id = ? AND uid != '')`

// KGEntityVecFingerprint 实体向量指纹（name+description 内容变化检测——未变跳过重算 embedding）。
// fnv-1a 64 位 hex16（防意外重算的内容指纹，非安全场景）。
func KGEntityVecFingerprint(name, description string) string {
	const fnvOffset, fnvPrime = uint64(14695981039346656037), uint64(1099511628211)
	h := fnvOffset
	for _, b := range []byte(name + "\x1f" + description) {
		h ^= uint64(b)
		h *= fnvPrime
	}
	return fmt.Sprintf("%016x", h)
}
