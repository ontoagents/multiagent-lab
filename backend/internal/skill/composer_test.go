package skill

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func seededComposer(t *testing.T, skills ...*store.Skill) (*Composer, *store.Agent) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ag, err := st.CreateAgent(&store.Agent{Name: "host", Instruction: "你是运维助手"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range skills {
		created, err := st.CreateSkill(sk)
		if err != nil {
			t.Fatal(err)
		}
		ag.Skills = append(ag.Skills, created.ID)
	}
	if _, err := st.UpdateAgent(ag); err != nil {
		t.Fatal(err)
	}
	return &Composer{Store: st}, ag
}

// REQ-203 B6：目录注入——含名称与描述，不含正文全文（渐进披露）。
func TestComposeInstructionCatalogNotFullBody(t *testing.T) {
	c, ag := seededComposer(t,
		&store.Skill{Name: "k8s-triage", Description: "K8s 排障三步法", Instruction: "第一步……\n第二步……\n第三步……（很长的正文）", Enabled: true},
		&store.Skill{Name: "no-desc", Instruction: "首行描述\n正文第二行", Enabled: true},
	)
	out := c.ComposeInstruction(ag)
	if !strings.Contains(out, "k8s-triage") || !strings.Contains(out, "K8s 排障三步法") {
		t.Fatalf("目录应含名称与描述: %s", out)
	}
	if strings.Contains(out, "第二步") || strings.Contains(out, "正文第二行") {
		t.Fatal("正文不应常驻提示词（渐进披露）")
	}
	if !strings.Contains(out, "load_skill") || !strings.Contains(out, "首行描述") {
		t.Fatalf("目录应指路 load_skill 且无描述技能回退首行: %s", out)
	}
	// 无技能 → base 原样
	c2, ag2 := seededComposer(t)
	if got := c2.ComposeInstruction(ag2); got != "你是运维助手" {
		t.Fatalf("无技能应原样: %q", got)
	}
}

// load_skill：已挂载取全文；未知回执拒绝（不炸 run 口径）。
func TestLoadSkillTool(t *testing.T) {
	bt, err := NewLoadSkillTool([]*store.Skill{{Name: "k8s-triage", Instruction: "完整正文 A", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	it := bt.(einotool.InvokableTool)
	if res, err := it.InvokableRun(context.Background(), `{"name":"k8s-triage"}`); err != nil || !strings.Contains(res, "完整正文 A") {
		t.Fatalf("应取到全文: %v %s", err, res)
	}
	if res, err := it.InvokableRun(context.Background(), `{"name":"unknown"}`); err != nil || !strings.Contains(res, "不在本 agent 已挂载清单") {
		t.Fatalf("未知技能应回执拒绝: %v %s", err, res)
	}
}

// REQ-373：集合挂载展开——成员目录替代集合自身；停用/悬空逐层诚实跳过。
func TestCollectionExpansion(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m1, err := st.CreateSkill(&store.Skill{Name: "m1", Description: "成员一", Instruction: "正文一", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	m2, err := st.CreateSkill(&store.Skill{Name: "m2", Description: "成员二", Instruction: "正文二", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	m3, err := st.CreateSkill(&store.Skill{Name: "m3", Description: "成员三", Instruction: "正文三", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	coll, err := st.CreateSkill(&store.Skill{
		Name: "pack", Description: "集合", Kind: "collection",
		Children: []string{m1.ID, m2.ID, m3.ID, "skill_ghost_deleted"}, // 悬空子件
		Enabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := &Composer{Store: st}
	ag := &store.Agent{Name: "host", Skills: []string{coll.ID}}

	loaded := c.LoadedSkills(ag)
	if len(loaded) != 2 || loaded[0].Name != "m1" || loaded[1].Name != "m3" {
		t.Fatalf("应展开启用成员并跳过停用/悬空: %d 件", len(loaded))
	}
	for _, sk := range loaded {
		if sk.Kind == "collection" {
			t.Fatal("集合自身不应出现在展开结果")
		}
	}
	out := c.ComposeInstruction(ag)
	if !strings.Contains(out, "m1") || !strings.Contains(out, "m3") || strings.Contains(out, "pack」：") {
		t.Fatalf("目录应含成员目录行且无集合行: %s", out)
	}
	// 工具并集：成员工具并入（集合自身 tools 不存）
	if got := c.SkillTools(ag); len(got) != 0 {
		t.Fatalf("无工具成员并集应为空: %v", got)
	}

	// 集合停用=整体不展开
	coll.Enabled = false
	if _, err := st.UpdateSkill(coll); err != nil {
		t.Fatal(err)
	}
	if got := c.LoadedSkills(ag); len(got) != 0 {
		t.Fatalf("停用集合应不展开: %d", len(got))
	}

	// 环防护：异常自引数据不炸（visited 终止）
	coll.Enabled = true
	coll.Children = []string{coll.ID}
	if _, err := st.UpdateSkill(coll); err != nil {
		t.Fatal(err)
	}
	if got := c.LoadedSkills(ag); len(got) != 0 {
		t.Fatalf("自引环应安全终止: %d", len(got))
	}
}
