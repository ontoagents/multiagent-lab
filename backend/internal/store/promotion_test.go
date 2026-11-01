package store

// REQ-546/M353 learning proposal 暂存区存储层单测：提案 CRUD（快照冻结）/清单过滤/
// pending 守卫（终态不可再裁——审计链单向）。

import (
	"errors"
	"path/filepath"
	"testing"
)

func newPromotionTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestPromotionProposalCRUD_Req546(t *testing.T) {
	st := newPromotionTestStore(t)
	p := &PromotionProposal{
		SourceOntologyID: "onto_comp",
		TargetOntologyID: "onto_main",
		Note:             "对话沉淀的概念子图",
		Payload: PromotionPayload{
			Entities:   []string{"交换机", "路由器"},
			Relations:  [][3]string{{"交换机", "级联", "路由器"}},
		},
	}
	if err := st.CreatePromotionProposal(p); err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Fatal("创建后应回填 id")
	}
	got, err := st.GetPromotionProposal(p.ID)
	if err != nil || got == nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.Status != "pending" || got.EntityCount != 2 || got.RelationCount != 1 {
		t.Fatalf("pending 提案往返失真: %+v", got)
	}
	if len(got.Payload.Entities) != 2 || got.Payload.Entities[0] != "交换机" {
		t.Fatalf("快照实体失真: %+v", got.Payload.Entities)
	}
	if len(got.Payload.Relations) != 1 || got.Payload.Relations[0][1] != "级联" {
		t.Fatalf("快照关系失真: %+v", got.Payload.Relations)
	}
	if got.CreatedAt == "" {
		t.Fatal("created_at 应落库")
	}

	// 清单过滤：按状态/来源/目标
	all, err := st.ListPromotionProposals("", "", "")
	if err != nil || len(all) != 1 {
		t.Fatalf("全量清单失真: %v %v", all, err)
	}
	bySrc, _ := st.ListPromotionProposals("onto_comp", "", "")
	if len(bySrc) != 1 {
		t.Fatal("来源过滤失真")
	}
	if none, _ := st.ListPromotionProposals("onto_other", "", ""); len(none) != 0 {
		t.Fatal("不匹配来源应空")
	}
	// 未决清单命中 / 已决清单空
	if pend, _ := st.ListPromotionProposals("", "", "pending"); len(pend) != 1 {
		t.Fatal("pending 过滤失真")
	}
	if appr, _ := st.ListPromotionProposals("", "", "approved"); len(appr) != 0 {
		t.Fatal("approved 过滤失真")
	}

	// 裁决：approve（带注记）
	dp, err := st.DecidePromotionProposal(p.ID, "approved", "核对无误")
	if err != nil {
		t.Fatalf("裁决失败: %v", err)
	}
	if dp.Status != "approved" || dp.DecidedAt == "" {
		t.Fatalf("裁决往返失真: %+v", dp)
	}
	// 终态守卫：再裁返回 ErrPromotionAlreadyDecided
	if _, err := st.DecidePromotionProposal(p.ID, "rejected", "改口"); !errors.Is(err, ErrPromotionAlreadyDecided) {
		t.Fatalf("终态再裁应 ErrPromotionAlreadyDecided, got %v", err)
	}
	// 非法状态拒绝
	if _, err := st.DecidePromotionProposal(p.ID, "confirmed", ""); err == nil {
		t.Fatal("非法裁决状态应报错")
	}

	// reject 留痕（独立提案）
	p2 := &PromotionProposal{SourceOntologyID: "onto_comp", TargetOntologyID: "onto_main",
		Payload: PromotionPayload{Entities: []string{"孤立品"}}}
	if err := st.CreatePromotionProposal(p2); err != nil {
		t.Fatal(err)
	}
	rp, err := st.DecidePromotionProposal(p2.ID, "rejected", "与目标本体定位不符")
	if err != nil || rp.Status != "rejected" || rp.DecidedNote != "与目标本体定位不符" {
		t.Fatalf("拒绝留痕失真: %+v %v", rp, err)
	}
	// 不存在 id → (nil, nil)（handler 侧 404 判定）
	if np, err := st.DecidePromotionProposal("nope", "approved", ""); np != nil || err != nil {
		t.Fatalf("不存在 id 应 (nil,nil): %+v %v", np, err)
	}
	if gp, _ := st.GetPromotionProposal("nope"); gp != nil {
		t.Fatal("不存在 id 读取应 nil")
	}
}
