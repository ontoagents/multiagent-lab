// REQ-358/M158 B（EvoOntology 增量借鉴·93 号 §五 P1③）：伴生图语义检索工具——自然语言→
// 语义匹配→实体+邻域（browse/resolve 式），伴生图消费从「每轮注入」升级「按需查询」。
// 落点与 REQ-354 工具面协调（同注册表、默认关不进任何默认工具集——92 号 §5.2；Agent 显式
// 勾选生效）。检索实现= companion.Service.SemanticSearch（独立函数，旁路包）——tool→companion
// 环约束（companion→chat→tool）经装配层闭包注入破环（REQ-355 R2 先例）。
// ontology_id 缺省解析发起智能体的伴生绑定（tool.Caller 身份上下文，REQ-354/P2 同源）。
package tool

import (
	"context"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// CompanionSearchHit 语义检索命中实体投影（companion.SemanticSearchHit 的工具面投影，
// 结构逐字段同构——环约束下的镜像类型，装配适配器逐字段转换）。
type CompanionSearchHit struct {
	Label      string   `json:"label"`
	Kind       string   `json:"kind,omitempty"`
	Definition string   `json:"definition,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Score      float64  `json:"score"`
	Neighbors  []string `json:"neighbors,omitempty"`
}

type companionSearchIn struct {
	Query      string `json:"query" jsonschema:"自然语言查询（如「之前聊过的部署策略有哪些」）；语义匹配伴生图内实体，同义改述可命中"`
	OntologyID string `json:"ontology_id,omitempty" jsonschema:"伴生本体 id（缺省=当前智能体绑定的伴生本体；跨会话共享图按需查询时显式传入）"`
	TopK       int    `json:"top_k,omitempty" jsonschema:"返回命中实体数上限（缺省 5，最大 10）"`
}

type companionSearchOut struct {
	Results    []CompanionSearchHit `json:"results"`
	Count      int                  `json:"count"`
	OntologyID string               `json:"ontology_id"`
	Hint       string               `json:"hint,omitempty"`
}

// CompanionSearchDeps 伴生语义检索工具依赖（api.NewServer 装配闭包注入——环约束破环面）。
type CompanionSearchDeps struct {
	Store *store.Store
	// SemanticSearch 伴生图语义检索（*companion.Service.SemanticSearch 适配；ontologyID 为空
	// 或未绑定时返回 companion.ErrCompanionNotBound 语义错误）
	SemanticSearch func(ctx context.Context, ontologyID, query string, topK int) ([]CompanionSearchHit, error)
}

func companionSearchFn(deps CompanionSearchDeps) func(ctx context.Context, in companionSearchIn) (companionSearchOut, error) {
	return func(ctx context.Context, in companionSearchIn) (companionSearchOut, error) {
		ontID := in.OntologyID
		if ontID == "" {
			// 缺省解析发起智能体的伴生绑定（REQ-354/P2·tool.Caller 身份上下文）
			agentID := ""
			if c, ok := CallerFrom(ctx); ok {
				agentID = c.AgentID
			}
			if agentID == "" {
				return companionSearchOut{}, fmt.Errorf("无法解析发起智能体（缺省 ontology_id 仅对智能体工具调用可用），请显式传 ontology_id")
			}
			a, err := deps.Store.GetAgent(agentID)
			if err != nil || a.CompanionOntologyID == "" {
				return companionSearchOut{Hint: "该智能体未绑定伴生本体（侧板-伴生本体开启后可查询）。如实转述，不要编造。"}, nil
			}
			ontID = a.CompanionOntologyID
		}
		hits, err := deps.SemanticSearch(ctx, ontID, in.Query, in.TopK)
		if err != nil {
			return companionSearchOut{OntologyID: ontID, Hint: "检索失败：" + err.Error() + "。如实转述，不要编造结果。"}, nil
		}
		out := companionSearchOut{Results: hits, Count: len(hits), OntologyID: ontID}
		if len(hits) == 0 {
			out.Hint = "伴生图中没有语义匹配的实体——对话中沉淀的知识尚未覆盖该话题；如实告知，不要编造。"
		} else {
			out.Hint = "把命中实体按定义与邻域转述给用户；实体间关系（Neighbors）已含方向。"
		}
		return out, nil
	}
}

// NewCompanionSearchTool REQ-358 B：伴生图按需语义检索（只读消费面）。
func NewCompanionSearchTool(deps CompanionSearchDeps) (einotool.BaseTool, error) {
	return utils.InferTool("companion_search",
		"在伴生本体图（对话中沉淀的跨会话动态知识）中语义检索实体与邻域。用于用户询问「之前聊过的/我们讨论过的」类话题时按需查询，而不是凭对话记忆猜测。",
		companionSearchFn(deps))
}

// RegisterCompanionSearchTool 注册（REQ-358 B；仅注册不进默认工具集——92 号 §5.2，默认关原则）。
func RegisterCompanionSearchTool(r *Registry, deps CompanionSearchDeps) error {
	return r.Register(&Entry{
		ID:          "companion_search",
		Description: "在伴生本体图（对话中沉淀的跨会话动态知识，全部会话共享）中语义检索：自然语言→匹配实体+定义+关系邻域。用户问「之前聊过的 X」「我们讨论过的 Y」时按需查询",
		Source:      SourceBuiltin,
		New: func(ctx context.Context) (einotool.BaseTool, error) {
			return NewCompanionSearchTool(deps)
		},
	})
}
