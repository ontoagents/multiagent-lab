// Package kb 知识库（M6，方案 §6.9）：检索后端抽象 + 切分 + Embedding + 索引/检索编排。
// 后端选择：KB_VECTOR_BACKEND=qdrant|sqlite（默认 qdrant；QDRANT_URL 未配或启动探测不可达
// 自动落 SQLite fallback——KB-O1 口径，REQ-593 补实现：此前仅文档记载、探测从未接线，
// 无 Qdrant 机器上索引/删除/检索全链硬错）。
// eino-ext components/retriever/qdrant 已核对（v0.0.0-20260916）：API 为 string query + 内部 Embedder + gRPC client，
// 与本平台流程（平台管 Embedding 连接、Search 直收向量）不匹配，故按 §344 采用 Qdrant 直连 REST（仍属开源路径）。
package kb

import (
	"context"
	"fmt"
	"log"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// Chunk 待索引片段（向量已由 Service 层生成）。
type Chunk struct {
	ID      string // chunk id（SQLite 主键；Qdrant 路径转 UUID 作 point id）
	DocID   string
	Seq     int
	Content string
	Vector  []float32
}

// Hit 检索命中。
type Hit struct {
	ChunkID string
	DocID   string
	Seq     int
	Content string
	// KB-10②：所属父块内容（子块检索、父块召回上下文；空=自身即独立块）
	ParentContent string
	Score         float64
	Strategy      string // KB-10①：vector|lexical|hybrid（空 = 未走融合口径的历史路径）
}

// VectorStore 检索后端抽象（方案 §6.9）。
type VectorStore interface {
	// EnsureCollection 确保 kb 对应的 collection 存在（dim=向量维度）。
	EnsureCollection(ctx context.Context, kbID string, dim int) error
	// Upsert 写入/覆盖向量（Qdrant：point id=chunk id UUID；SQLite：不适用，由 Service 直写 BLOB）。
	Upsert(ctx context.Context, kbID string, chunks []Chunk) error
	// Search 余弦 TopK + 阈值过滤。
	Search(ctx context.Context, kbID string, vec []float32, topK int, minScore float64) ([]Hit, error)
	// DeleteByDoc 按 doc 删除全部向量。
	DeleteByDoc(ctx context.Context, kbID, docID string) error
}

// collectionName 约定（§336）：kb_{id}。
func collectionName(kbID string) string { return "kb_" + kbID }

// NewVectorStore 按配置选择后端（st 供 SQLite fallback 检索使用）。
// qdrant 分支启动探测一次（KB-O1「缺失/不可达可退 sqlite」的实现补齐，REQ-593）：
// URL 缺失或 ping 不通 → 人话告警 + SQLite fallback（backendName/Healthz/索引写入随类型开关自然切换）；
// 显式 KB_VECTOR_BACKEND=sqlite 零探测直取。运行期 Qdrant 中途掉线不做热切换（健康检查如实在案，
// 重启即回退——文档口径=启动探测+健康检查）。
func NewVectorStore(backend, qdrantURL string, st *store.Store) (VectorStore, error) {
	switch backend {
	case "sqlite":
		return SQLiteStore{ST: st}, nil
	case "qdrant", "":
		if qdrantURL == "" {
			log.Printf("[kb] QDRANT_URL 未配置：知识库向量检索改用 SQLite 内置后端（零外部依赖；装好 Qdrant 并配置 QDRANT_URL 后重启即切回）")
			return SQLiteStore{ST: st}, nil
		}
		qs := NewQdrantStore(qdrantURL)
		if !qs.ping(context.Background()) {
			log.Printf("[kb] Qdrant 不可达（%s 连接失败）：知识库向量检索自动改用 SQLite 内置后端（余弦全扫、零外部依赖），healthz knowledge_backend=sqlite-ok 如实可查——装好并启动 Qdrant 后重启即切回；qdrant 时代已入库 chunk 本地无向量，重建索引后恢复向量命中", qdrantURL)
			return SQLiteStore{ST: st}, nil
		}
		return qs, nil
	default:
		return nil, fmt.Errorf("未知 KB_VECTOR_BACKEND %q（支持 qdrant|sqlite）", backend)
	}
}
