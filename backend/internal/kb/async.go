package kb

// REQ-440/KB-8③（M236）：索引异步化——导入/重建索引落库即返回（status=indexing），
// 后台串行队列完成切分→embedding→KG 抽取，HTTP 不再等待长耗时（graphrag 大文档导入
// 同步阻塞数分钟的病灶根除）。
//   - 单 worker 串行：与同步语义等价，防 SQLite 并发写（并行化留池）；
//   - ctx 脱离请求生命周期（Background+10min 上限）；
//   - 进程内队列：重启丢队列中任务 → 文档停留 indexing 态，重试入口=「重建索引」按钮（如实呈现）；
//   - GraphragIngest 结果落库（store.SetDocGraphrag）——异步化后响应不再携带终态，
//     KG 徽标/失败原因经 graphrag_json 持久可见（顺修原「仅当次回显、刷新即丢」）。
// 同步版 Import/Reindex 保留为内部路径（chat.KBAdminSource/平台助手建库等单条小文本场景）。

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// indexQueueCap 后台索引队列容量（学习尺度远够；满则如实 failed 提示重试）。
const indexQueueCap = 64

// indexJob 后台索引任务（reindex=true 时后台先清旧向量/chunks 再重索——原同步 Reindex 清理段迁入）。
type indexJob struct {
	kbcfg   *store.KnowledgeBase
	kbID    string
	docID   string
	pieces  []Piece
	reindex bool
}

// startIndexWorker NewService 装配时启动（单消费 goroutine，进程生命周期）；
// KB-21/M293：启动即扫描 indexing 悬挂文档分派恢复（自动重入队或 failed 明示）。
func (s *Service) startIndexWorker() {
	s.jobs = make(chan indexJob, indexQueueCap)
	go func() {
		for j := range s.jobs {
			s.runIndexJob(j)
		}
	}()
	s.RecoverStalledIndexing()
}

// RecoverStalledIndexing 重启悬挂恢复（KB-21/M293）：进程重启丢队列任务，文档停留 indexing。
// 分派：①有 chunks（重建中悬挂）→ 自动重入队 reindex（原文自 chunks 拼接，幂等索引语义）；
// ②无 chunks（导入中悬挂——原文仅存于丢失的内存队列，不可恢复）→ failed 明示「请重新导入」。
// 不引入外部队列（D-O15/单机基线维持）。
func (s *Service) RecoverStalledIndexing() {
	kbs, err := s.Store.ListKnowledgeBases()
	if err != nil {
		log.Printf("[kb] warn: 悬挂索引扫描失败（列库）: %v", err)
		return
	}
	requeued, marked := 0, 0
	for _, kb := range kbs {
		docs, err := s.Store.ListKnowledgeDocs(kb.ID)
		if err != nil {
			continue
		}
		for _, d := range docs {
			if d.Status != "indexing" {
				continue
			}
			chunks, cerr := s.Store.ListKnowledgeChunksByDoc(d.ID)
			if cerr == nil && len(chunks) > 0 {
				// 重建中悬挂：原文可自 chunks 拼接——自动重入队（幂等）
				s.enqueue(indexJob{kbcfg: kb, kbID: kb.ID, docID: d.ID, reindex: true})
				requeued++
				continue
			}
			// 导入中悬挂：原文未落库不可恢复——failed 明示
			_ = s.Store.UpdateKnowledgeDocStatus(d.ID, "failed", 0, 0,
				"进程重启致索引中断，导入原文未留存——请重新导入该文档")
			marked++
		}
	}
	if requeued > 0 || marked > 0 {
		log.Printf("[kb] 启动悬挂恢复：%d 个 indexing 文档重入队（重建中悬挂，幂等重索），%d 个标 failed（导入中悬挂，原文未留存）", requeued, marked)
	}
}

// ImportAsync 异步导入：校验+切分+落库（indexing）同步完成（快，400 可同步报），索引进后台。
func (s *Service) ImportAsync(_ context.Context, kbID, title, content string) (*store.KnowledgeDoc, error) {
	kbcfg, err := s.Store.GetKnowledgeBase(kbID)
	if err != nil {
		return nil, err
	}
	pieces := SplitPieces(content)
	if len(pieces) == 0 {
		return nil, fmt.Errorf("内容为空，无法索引")
	}
	doc, err := s.Store.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: kbID, Title: title, Status: "indexing"})
	if err != nil {
		return nil, err
	}
	s.enqueue(indexJob{kbcfg: kbcfg, kbID: kbID, docID: doc.ID, pieces: pieces})
	return s.Store.GetKnowledgeDoc(doc.ID)
}

// ReindexAsync 异步重建：置 indexing 即返回；旧正文拼接/清理/重索全部进后台。
func (s *Service) ReindexAsync(_ context.Context, kbID, docID string) (*store.KnowledgeDoc, error) {
	kbcfg, err := s.Store.GetKnowledgeBase(kbID)
	if err != nil {
		return nil, err
	}
	doc, err := s.Store.GetKnowledgeDoc(docID)
	if err != nil {
		return nil, err
	}
	if doc.KBID != kbID {
		return nil, store.ErrNotFound
	}
	if err := s.Store.UpdateKnowledgeDocStatus(docID, "indexing", 0, 0, ""); err != nil {
		return nil, err
	}
	s.enqueue(indexJob{kbcfg: kbcfg, kbID: kbID, docID: docID, reindex: true})
	return s.Store.GetKnowledgeDoc(docID)
}

// enqueue 入队；队列满如实 failed（重试入口=重建索引按钮）。
func (s *Service) enqueue(j indexJob) {
	select {
	case s.jobs <- j:
	default:
		log.Printf("[kb] warn: 索引队列已满（cap=%d），doc %s 落 failed", indexQueueCap, j.docID)
		_ = s.Store.UpdateKnowledgeDocStatus(j.docID, "failed", 0, 0,
			fmt.Sprintf("索引队列已满（%d），请稍后点「重建索引」重试", indexQueueCap))
	}
}

// runIndexJob 后台执行单任务（reindex 先清旧；graphrag 联动结果落库）。
func (s *Service) runIndexJob(j indexJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	doc, err := s.Store.GetKnowledgeDoc(j.docID)
	if err != nil {
		log.Printf("[kb] warn: 后台索引取 doc %s 失败: %v", j.docID, err)
		return
	}
	pieces := j.pieces
	if j.reindex {
		pieces, err = s.clearForReindex(ctx, j.kbID, j.docID, j.kbcfg)
		if err != nil {
			_ = s.Store.UpdateKnowledgeDocStatus(j.docID, "failed", 0, 0, err.Error())
			return
		}
	}
	chunks, err := s.indexDoc(ctx, j.kbcfg, j.kbID, doc, pieces)
	if err != nil {
		_ = s.Store.UpdateKnowledgeDocStatus(j.docID, "failed", 0, 0, err.Error())
		return
	}
	if j.kbcfg.Mode == "graphrag" {
		s.persistGraphrag(j.kbID, j.docID, s.GraphragIngest(ctx, j.kbID, chunks)) // 降级不阻断语义保留
	}
}

// clearForReindex 重建前清理：旧正文（chunks 拼接，避免额外存储的既有语义）→ 删旧向量/chunks →
// wiki 指纹失效；返回重新切分结果。
func (s *Service) clearForReindex(ctx context.Context, kbID, docID string, kbcfg *store.KnowledgeBase) ([]Piece, error) {
	old, err := s.Store.ListKnowledgeChunksByDoc(docID)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	for i, c := range old {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(c.Content)
	}
	if sb.Len() == 0 {
		return nil, fmt.Errorf("文档无可重建的原文内容")
	}
	if err := s.Vector.DeleteByDoc(ctx, kbID, docID); err != nil {
		return nil, fmt.Errorf("清理旧向量: %w", err)
	}
	if err := s.Store.DeleteKnowledgeChunksByDoc(docID); err != nil {
		return nil, err
	}
	// REQ-241：内容将变，wiki 指纹失效（下次重建按变更重生成 summary）
	if kbcfg.Mode == "wiki" {
		_ = s.Store.SetDocWikiHash(kbID, docID, "")
	}
	return SplitPieces(sb.String()), nil
}

// persistGraphrag KG 抽取结果落库（失败仅告警——落库是展示件，不影响索引主流程）。
func (s *Service) persistGraphrag(kbID, docID string, info *store.GraphragInfo) {
	if info == nil {
		return
	}
	b, err := json.Marshal(info)
	if err != nil {
		return
	}
	if err := s.Store.SetDocGraphrag(kbID, docID, string(b)); err != nil {
		log.Printf("[kb] warn: 落库 graphrag 结果失败 (doc=%s): %v", docID, err)
	}
}
