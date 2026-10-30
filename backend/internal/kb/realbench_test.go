//go:build kbeval

// REQ-533/M340（P24/P25 基线轮）：真实库检索链路耗时基线——手工落档承载（非 CI 断言）：
// 对 dev 库（KB_EVAL_DB 或 DB_PATH 默认 backend/data/platform.db）chunk 量级实测
// 词法臂（FTS5 BM25）单查询耗时，给 16 号 §9「sqlite 路径规模参考」第一手数字。
//
//	跑法：cd backend && go test -tags kbeval ./internal/kb/ -run TestKBRealBench -v
package kb_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestKBRealBench(t *testing.T) {
	dbPath := filepath.Join("..", "data", "platform.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Skipf("dev 库不可达（%v）——基线随真实环境轮补测", err)
	}
	defer st.DB.Close()
	var chunks int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM knowledge_chunk`).Scan(&chunks); err != nil || chunks == 0 {
		t.Skipf("dev 库无 chunk（%d）——基线随真实语料轮补测", chunks)
	}
	queries := []string{"检索质量评测基准", "部署与运行指南", "需求编号分配权威", "伴生本体沉淀"}
	var total time.Duration
	for _, q := range queries {
		start := time.Now()
		hits, _ := st.SearchChunksFTS("", store.FTSMatchQuery(q), 5)
		elapsed := time.Since(start)
		total += elapsed
		t.Logf("[基线] chunks=%d q=%q hits=%d 耗时=%s", chunks, truncateQuery(q), len(hits), elapsed)
	}
	t.Logf("======== REQ-533 基线：%d queries / %d chunks / 平均 %s ========", len(queries), chunks, total/time.Duration(len(queries)))
}

func truncateQuery(q string) string {
	r := []rune(q)
	if len(r) > 12 {
		return string(r[:12])
	}
	return q
}
