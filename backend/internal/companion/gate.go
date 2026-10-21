// gate.go REQ-358/M158 C（EvoOntology 增量借鉴·93 号 §五 P2④/§7.1）伴生线门控类型与
// 只读辅助——门控决策函数本体在 companion/evaldata（旁路包，冻结基准集所在）；本文件只
// 承载 Service 挂点所需的类型与导出窄面，伴生主链路零逻辑变更（D-O26/93 号 §7.4）。
package companion

import "context"

// GateCandidateInfo 门控征询的候选快照（最小面；自动化裁决不触人工字段）。
type GateCandidateInfo struct {
	ID          string
	Kind        string // concept | relation | event
	Name        string
	Definition  string
	Confidence  float64
	OntologyID  string // 宿主伴生本体（门控据此取图信号）
}

// GateFunc 基准门控决策：true=放行入图；false=拒绝（reason 落 reject_reason，93 号 §7.2）。
// 旁挂语义：阈值通过后追加征询；实现须只读（不得改图）。
type GateFunc func(ctx context.Context, cand GateCandidateInfo) (allow bool, reason string)

// LabelsFor 实体标签清单（门控/评测等旁路面只读取数口；读侧兜底拉起，宿主不可达返回空）。
func (s *Service) LabelsFor(ctx context.Context, ontologyID string) ([]string, error) {
	return s.queryLabels(ctx, ontologyID)
}
