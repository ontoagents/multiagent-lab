// REQ-360/M161 对话建本体能力开关单测：装配层自动并入四工具（与手勾并集去重/来源标注 onchat）、
// 未开能力不装配（不静默改配置）、内置纪律段注入与 REQ-311 透镜切分、迁移 050 平台助手默认开。
package chat

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

func newOnchatFixture(t *testing.T) *Assembler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	if err := tool.RegisterBuiltin(reg); err != nil {
		t.Fatal(err)
	}
	return &Assembler{Store: st, Box: box, Tools: reg}
}

func toolNames(tb *toolBundle) map[string]string {
	out := map[string]string{}
	for _, bt := range tb.Tools {
		if ti, err := bt.Info(context.Background()); err == nil && ti != nil {
			out[ti.Name] = tb.SourceOf[ti.Name]
		}
	}
	return out
}

// 能力开关 on：四工具自动并入，来源 onchat；off：不装配（不静默改配置）。
func TestAssembleOnchatCapabilitySwitch(t *testing.T) {
	asm := newOnchatFixture(t)

	on := &store.Agent{Name: "builder", OnchatMode: "guided"}
	tb, err := asm.assembleTools(context.Background(), on, assembleScope{})
	if err != nil {
		t.Fatal(err)
	}
	names := toolNames(tb)
	for _, id := range tool.OnchatToolIDs {
		if names[id] != "onchat" {
			t.Fatalf("开关 on 应装配 %s（来源 onchat），got %q", id, names[id])
		}
	}

	off := &store.Agent{Name: "plain"}
	tb2, err := asm.assembleTools(context.Background(), off, assembleScope{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range tool.OnchatToolIDs {
		if src, ok := toolNames(tb2)[id]; ok {
			t.Fatalf("开关 off 不应装配 %s，got 来源 %q（不静默改配置）", id, src)
		}
	}
}

// 与手勾取并集：已手勾的工具保持原来源（builtin），其余三件补齐为 onchat，不重复注册。
func TestAssembleOnchatUnionWithHandPicked(t *testing.T) {
	asm := newOnchatFixture(t)
	ag := &store.Agent{Name: "hand", Tools: []string{"ontochat_start"}, OnchatMode: "quick"}
	tb, err := asm.assembleTools(context.Background(), ag, assembleScope{})
	if err != nil {
		t.Fatal(err)
	}
	names := toolNames(tb)
	if names["ontochat_start"] != "builtin" {
		t.Fatalf("手勾工具来源应保持 builtin，got %q", names["ontochat_start"])
	}
	for _, id := range []string{"ontochat_turn", "ontochat_save", "ontochat_export_ttl"} {
		if names[id] != "onchat" {
			t.Fatalf("并集补齐 %s 应为 onchat，got %q", id, names[id])
		}
	}
	count := 0
	for name := range names {
		if name == "ontochat_start" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("ontochat_start 应去重为一份，got %d", count)
	}
}

// 纪律段：开关注入（含模式缺省文案）/关闭为空；REQ-311 透镜按段头切分标注 source=onchat。
func TestOnchatDisciplineAndSegments(t *testing.T) {
	if s := onchatDiscipline(&store.Agent{Name: "x"}); s != "" {
		t.Fatalf("开关 off 纪律段应为空，got %q", s)
	}
	g := onchatDiscipline(&store.Agent{Name: "x", OnchatMode: "guided"})
	if !strings.Contains(g, "# 对话建本体纪律（OntoChat）") || !strings.Contains(g, "mode 缺省 guided") {
		t.Fatalf("纪律段应含段头与 guided 缺省，got %q", g)
	}
	q := onchatDiscipline(&store.Agent{Name: "x", OnchatMode: "quick"})
	if !strings.Contains(q, "quick=快速模式") {
		t.Fatalf("quick 模式文案缺失，got %q", q)
	}

	inst := "你是测试助手。" + g
	segs := instructionSegments(inst)
	if len(segs) != 2 {
		t.Fatalf("应切分为 base+onchat 两段，got %d 段", len(segs))
	}
	if segs[0]["source"] != "base" || segs[1]["source"] != "onchat" {
		t.Fatalf("切分来源不符：%v / %v", segs[0]["source"], segs[1]["source"])
	}
	if !strings.Contains(segs[1]["text"].(string), "一次只问一个问题") {
		t.Fatalf("onchat 段文本不符：%v", segs[1]["text"])
	}
}

// 迁移 050：平台助手内置行默认 onchat_mode=guided（C1 显式引导面，03 §2.54 决策注）；
// 普通行缺省空=存量零变化。
func TestMigrationOnchatDefaults(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b, err := st.GetAgent("builtin-assistant")
	if err != nil {
		t.Fatal(err)
	}
	if b.OnchatMode != "guided" {
		t.Fatalf("平台助手应默认 guided，got %q", b.OnchatMode)
	}
	a, err := st.CreateAgent(&store.Agent{Name: "normal"})
	if err != nil {
		t.Fatal(err)
	}
	if a.OnchatMode != "" {
		t.Fatalf("普通智能体缺省应为空，got %q", a.OnchatMode)
	}
	got, err := st.UpdateAgent(&store.Agent{ID: a.ID, Name: a.Name, OnchatMode: "quick", MaxIteration: a.MaxIteration, RuntimeBackend: a.RuntimeBackend})
	if err != nil {
		t.Fatal(err)
	}
	if got.OnchatMode != "quick" {
		t.Fatalf("UpdateAgent 应落 onchat_mode=quick，got %q", got.OnchatMode)
	}
}
