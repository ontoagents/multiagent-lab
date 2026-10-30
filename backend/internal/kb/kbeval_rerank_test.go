//go:build kbeval

// KB-22/M294 评估件：Rerank/查询改写引入价值重估——真机量化混合检索（FTS5+向量 RRF）
// 的排序缺口，判定 Rerank 重排的实际增益空间（评估工具代码，非产品代码——结论落 11 号）。
//
//	跑法：cd backend && go test -tags kbeval ./internal/kb/ -run TestKBEvalRerank -v
//	前置：默认 embedding 连接可用；语料/题集复用 evaldata 单源。
//
// 口径：①词法臂基线（纯 FTS5）gold 排位 MRR；②混合臂（svc.Search=向量+词法 RRF 全链）同口径；
// ③缺口判定=Rerank 价值空间=「gold 已入 topK 但排位>1」（可重排救回）vs「gold 未入 topK」
// （召回缺口，Rerank 无能为力）；查询改写价值=词法 miss 案例的可救性观察。
package kb_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/kb"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/kb/evaldata"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func TestKBEvalRerank(t *testing.T) {
	box, err := secrets.LoadKeyFile(filepath.Join("..", "..", "data", ".secret"))
	if err != nil {
		t.Skipf("密钥不可用：%v", err)
	}
	// Embedder 用运行栈平台库（默认连接数据源——沿 agenteval AGENTEVAL_DB 先例；KB_EVAL_DB 可覆盖）
	liveDB := envOr("KB_EVAL_DB", filepath.Join("..", "..", "data", "platform.db"))
	live, err := store.Open(liveDB)
	if err != nil {
		t.Skipf("运行栈库不可达（%s）：%v", liveDB, err)
	}
	defer live.DB.Close()
	st, err := store.Open(filepath.Join(t.TempDir(), "eval2.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.DB.Close()

	const kbID, docID = "kbeval-rr", "d1"
	if _, err := st.CreateKnowledgeBase(&store.KnowledgeBase{ID: kbID, Name: "重估基准库", Mode: "rag", KBVector: true}); err != nil {
		t.Fatalf("create kb: %v", err)
	}
	pieces := kb.SplitText(evaldata.Corpus)
	if len(pieces) < 2 {
		t.Fatalf("语料切分应 ≥2 chunks")
	}
	// 真机 embedding 语料 → chunk 向量随插入落库（SQLite BLOB）
	emb := &kb.Embedder{Store: live, Box: box}
	vecs, err := emb.EmbedTexts(context.Background(), pieces)
	if err != nil {
		t.Skipf("embedding 不可达（评估件不伪造向量数据）：%v", err)
	}
	chunks := make([]*store.KnowledgeChunk, 0, len(pieces))
	for i, content := range pieces {
		chunks = append(chunks, &store.KnowledgeChunk{ID: string(rune('a'+i)) + "-rr", KBID: kbID, DocID: docID, Seq: i, Content: content, Vector: kb.EncodeVector(vecs[i]), StoreBackend: "sqlite"})
	}
	if err := st.InsertKnowledgeChunks(chunks); err != nil {
		t.Fatalf("insert: %v", err)
	}

	vecStore := kb.SQLiteStore{ST: st}
	questions, err := evaldata.Questions()
	if err != nil {
		t.Fatalf("questions: %v", err)
	}
	topK := 5
	var kwMRR, hyMRR float64
	n := 0
	hyGoldTop1, hyGoldRerankable, hyGoldMissed := 0, 0, 0
	for _, q := range questions {
		if q.Type != "keyword" {
			continue // Rerank 价值面=关键词/语义检索排序；图臂/全局臂不涉 Rerank
		}
		n++
		// 词法臂基线（纯 FTS5 BM25）gold 排位（gold=内容子串）
		ftsHits, err := st.SearchChunksFTS(kbID, store.FTSMatchQuery(q.Q), topK)
		if err != nil {
			t.Fatalf("fts: %v", err)
		}
		lexHits := make([]kb.Hit, 0, len(ftsHits))
		for _, fh := range ftsHits {
			lexHits = append(lexHits, kb.Hit{ChunkID: fh.ChunkID, DocID: fh.DocID, Seq: fh.Seq, Content: fh.Content, Score: fh.Score})
		}
		kwMRR += mrrOfF(func(i int) string { return lexHits[i].Content }, len(lexHits), q.Gold)
		// 混合臂：svc.Search 全链（query embed→向量臂+词法 RRF 融合）
		vecQ, verr := emb.EmbedOne(context.Background(), q.Q)
		if verr != nil {
			t.Fatalf("embed query: %v", verr)
		}
		rawVec, verr := vecStore.Search(context.Background(), kbID, vecQ, topK, 0)
		if verr != nil {
			t.Fatalf("vector arm [%s]: %v", q.ID, verr)
		}
		hyHits := rrfFuse(rawVec, lexHits, topK)
		hyMRR += mrrOfF(func(i int) string { return hyHits[i].Content }, len(hyHits), q.Gold)
		rank := goldRankF(func(i int) string { return hyHits[i].Content }, len(hyHits), q.Gold)
		switch {
		case rank == 1:
			hyGoldTop1++
		case rank > 1:
			hyGoldRerankable++ // Rerank 可救区间
		default:
			hyGoldMissed++ // 召回缺口（Rerank 无效）
		}
		t.Logf("[对照] %-8s 词法MRR累计=%.3f 混合gold排名=%d", q.ID, kwMRR, rank)
	}
	fn := float64(n)
	t.Logf("======== KB-22 重估数据（%d 题关键词集）========", n)
	t.Logf("词法臂 MRR=%.3f  混合臂 MRR=%.3f", kwMRR/fn, hyMRR/fn)
	t.Logf("混合臂 gold 分布：top1=%d  排位>1（Rerank 可救）=%d  未入 top%d（Rerank 无效）=%d", hyGoldTop1, hyGoldRerankable, topK, hyGoldMissed)
}

// mrrOfF gold 首中位倒数排名（gold=内容子串；contentOf 抽取各 hit 判定面）。
func mrrOfF(contentOf func(i int) string, n int, golds []string) float64 {
	for i := 0; i < n; i++ {
		for _, g := range golds {
			if strings.Contains(contentOf(i), g) {
				return 1.0 / float64(i+1)
			}
		}
	}
	return 0
}

func goldRankF(contentOf func(i int) string, n int, golds []string) int {
	for i := 0; i < n; i++ {
		for _, g := range golds {
			if strings.Contains(contentOf(i), g) {
				return i + 1
			}
		}
	}
	return -1
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// rrfFuse 简化 RRF 融合（k=60 标准常数——评估口径与产品 fuseHybrid 同族，非字节复刻）。
func rrfFuse(vecHits, lexHits []kb.Hit, topK int) []kb.Hit {
	const k = 60.0
	score := map[string]float64{}
	byID := map[string]kb.Hit{}
	add := func(hits []kb.Hit) {
		for i, h := range hits {
			score[h.ChunkID] += 1.0 / (k + float64(i+1))
			if _, ok := byID[h.ChunkID]; !ok {
				byID[h.ChunkID] = h
			}
		}
	}
	add(vecHits)
	add(lexHits)
	type pair struct {
		id string
		s  float64
	}
	var ps []pair
	for id, s := range score {
		ps = append(ps, pair{id, s})
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].s > ps[j].s })
	out := make([]kb.Hit, 0, len(ps))
	for _, p := range ps {
		h := byID[p.id]
		h.Score = p.s
		out = append(out, h)
		if topK > 0 && len(out) >= topK {
			break
		}
	}
	return out
}
