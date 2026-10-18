// REQ-355 R1/M155 接口反转：kb 从对话主链路的吸附型 S3 债转为接口依赖。
// chat 定义接口与投影类型、kb 的实现经 api 装配层适配注入（api.NewKBChatSource），
// nil=未接入降级（recallKB 判空跳过，行为与 KB 未启用一致）——沿 CompanionSource/CommunitySource 范式。
// 本文件类型仅引用 chat/store 侧形态：kb 包不可用时 chat 仍可编译（可插拔判据 3：抽象反转，
// 94 号《借鉴实现耦合度分析与解耦可插拔规划》§1.2/§5 R1）。
package chat

import (
	"context"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// KBHit 知识库检索命中（chat 侧最小投影，字段与 kb.RetrievalHit 对应）。
// Spans/Sources 为事件透传载荷（B1 命中区间 / REQ-241 wiki 溯源定位），chat 不解构。
type KBHit struct {
	Doc     string
	Seq     int
	Score   float64
	Excerpt string
	Spans   any // 透传：excerpt 内 rune 偏移区间（前端句级高亮）
	Sources any // 透传：wiki 臂命中页溯源（doc 标题+chunk 序号）
}

// KBGraphragDetail graphrag 扩展检索明细（chat 侧最小投影）。
// Entities/Relationships/Claims 为 retrieval 事件透传载荷（M16② 实体/关系/claims 明细），chat 不解构。
type KBGraphragDetail struct {
	Hits          []KBHit
	Entities      any
	Relationships any
	Claims        any
}

// KBSource 对话知识库检索源接口（REQ-355 R1：chat.Service.KB 由 *kb.Service 反转为此接口）。
// 实现方在 api 装配层适配 kb.Service；nil=未接入（recallKB 判空跳过）。
type KBSource interface {
	// RetrieveContext 检索并渲染注入上下文：graphrag 模式走 KG 扩展检索、KG 无命中降级向量检索
	//（degraded=true 标注降级，M14 D-KB4 口径）；命中非空时返回渲染后的注入文本（kb.RenderContext
	// 同一实现，单源渲染）。detail 随行供 retrieval 事件透出（命中区间/wiki 溯源/graphrag 明细）。
	RetrieveContext(ctx context.Context, cfg *store.KnowledgeBase, query string, topK int, minScore float64) (text string, detail *KBGraphragDetail, mode string, degraded bool, err error)
}

// KBAdminSource 平台知识库管理面接口（REQ-355 R1：assistant_kb 工具族依赖反转——增量对账与向量检索）。
// 实现方同上（api 适配 kb.Service 的 Import/DeleteDoc/Search）。
type KBAdminSource interface {
	Import(ctx context.Context, kbID, title, content string) (*store.KnowledgeDoc, error)
	DeleteDoc(ctx context.Context, kbID, docID string) error
	// Search 向量检索（assistant search_platform_kb 工具；topK/minScore 同 kb.Search 语义）。
	Search(ctx context.Context, cfg *store.KnowledgeBase, query string, topK int, minScore float64) ([]KBHit, error)
}
