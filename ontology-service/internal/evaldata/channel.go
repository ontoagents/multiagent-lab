// REQ-257（60 号 H6）分渠道质量画像：构建渠道派生 + 分组统计（纯派生 v1，零迁移）。
// 渠道口径（沿资产列表来源分组 D-O21 规则扩展）：
//   seed   = 种子内置（seed_*/内置三份语义 ID）
//   fork   = forked_from 非空（派生副本）
//   import = 存在原始形态 artifact（owl/turtle/jsonld——外部资产经导入保真度管线进来的）
//   custom = 其余（自定义手工/AI 创建/OntoChat/KB 构建同形态，v1 不细分——细分需生成时落
//            source_path 标记，诚实遗留为观察项，见 60 号 H6 注）
package evaldata

import "strings"

// DeriveChannel 按本体元数据派生构建渠道。
func DeriveChannel(ontologyID, forkedFrom string, hasOriginal bool) string {
	if isSeedID(ontologyID) {
		return "seed"
	}
	if strings.TrimSpace(forkedFrom) != "" {
		return "fork"
	}
	if hasOriginal {
		return "import"
	}
	return "custom"
}

func isSeedID(id string) bool {
	if strings.HasPrefix(id, "onto_seed") {
		return true
	}
	switch id {
	case "onto_k8s_ops", "onto_med_common", "onto_gene_core":
		return true
	}
	return false
}

// ChannelStat 单渠道质量分布（分组报告行）。
type ChannelStat struct {
	Channel  string  `json:"channel"`
	Count    int     `json:"count"`
	AvgScore float64 `json:"avg_score"`
	MinScore float64 `json:"min_score"`
	MaxScore float64 `json:"max_score"`
	StructOK int     `json:"struct_ok"` // 结构校验通过数
}
