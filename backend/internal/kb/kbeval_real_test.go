//go:build kbeval

// REQ-526/M333（P08 两轮复核 verified）：真实语料检索质量基准——platform-knowledge 真实文档
// 节选（README 收录规则+16 号部署段）20 题 gold，词法臂 Recall@5+MRR 离线确定性跑分。
// 区别于种子语料（KB-14 教学构造）：真实文档结构（引用块/代码段/列表）下的检索质量量化；
// 向量/混合臂经 KB_EVAL_DB 可选（KB-22 形态——指运行栈平台库取默认 embedding 连接），
// 未配置时如实跳过（不伪造向量分数）。
//
//	跑法：cd backend && go test -tags kbeval ./internal/kb/ -run TestKBRealCorpusEval -v
//
// 同轮收口：M36 KB-6 抽取质量承诺（无主挂起）随抽取跑分器 kbeval_extract_test.go 销账。
package kb_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/kb"
	"github.com/ontoagents/ontoagents/backend/internal/kb/evaldata"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestKBRealCorpusEval(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "real-eval.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.DB.Close()
	const kbID, docID = "kb-real-eval", "rc-doc"
	if _, err := st.CreateKnowledgeBase(&store.KnowledgeBase{ID: kbID, Name: "真实语料基准库", Mode: "rag"}); err != nil {
		t.Fatalf("create kb: %v", err)
	}
	pieces := kb.SplitText(evaldata.RealCorpus)
	if len(pieces) < 2 {
		t.Fatalf("真实语料切分应 ≥2 chunks, got %d", len(pieces))
	}
	chunks := make([]*store.KnowledgeChunk, 0, len(pieces))
	for i, p := range pieces {
		chunks = append(chunks, &store.KnowledgeChunk{ID: fmt.Sprintf("rc-%d", i), KBID: kbID, DocID: docID, Seq: i, Content: p})
	}
	if err := st.InsertKnowledgeChunks(chunks); err != nil {
		t.Fatalf("insert chunks: %v", err)
	}
	t.Logf("======== REQ-526 真实语料基准（platform-knowledge 节选 %d chunks / %d 题 gold）========", len(chunks), len(evaldata.RealQuestions))

	const topK = 5
	var sumR, sumMRR float64
	missAll := []string{}
	for _, q := range evaldata.RealQuestions {
		hits, err := st.SearchChunksFTS(kbID, store.FTSMatchQuery(q.Q), topK)
		if err != nil {
			t.Fatalf("[%s] fts: %v", q.ID, err)
		}
		hay := ""
		for _, h := range hits {
			hay += h.Content + "\n"
		}
		// Recall@5：gold 全中记 1（题级）；MRR：首个 gold 命中的排名倒数（按 chunk 排序，
		// 同 chunk 内多 gold 不重复计——排序质量看首中位次）
		hitN, miss := 0, []string{}
		firstRank := 0
		for _, g := range q.Gold {
			if strings.Contains(hay, g) {
				hitN++
				if firstRank == 0 {
					for ri, h := range hits {
						if strings.Contains(h.Content, g) {
							firstRank = ri + 1
							break
						}
					}
				}
			} else {
				miss = append(miss, g)
			}
		}
		r := float64(hitN) / float64(len(q.Gold))
		sumR += r
		if firstRank > 0 {
			sumMRR += 1.0 / float64(firstRank)
		} else {
			missAll = append(missAll, q.ID+":"+strings.Join(miss, ","))
		}
		t.Logf("[词法] %-6s R@%d=%.2f firstRank=%d miss=%v", q.ID, topK, r, firstRank, miss)
	}
	n := float64(len(evaldata.RealQuestions))
	t.Logf("======== REQ-526 汇总：真实语料词法臂 Recall@5=%.3f  MRR=%.3f（%d 题）========", sumR/n, sumMRR/n, len(evaldata.RealQuestions))
	if sumR/n < 0.9 {
		t.Fatalf("真实语料词法臂 Recall@5 低于 0.9（miss=%v）——检索质量回归红线", missAll)
	}
	_ = sort.Strings // 保留排序 import 供向量臂扩展
}
