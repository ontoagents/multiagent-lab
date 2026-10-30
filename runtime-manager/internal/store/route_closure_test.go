package store

// REQ-540④：闭包补全本体进 RunningByOntology 路由（修运行面缺口 3 前半）——
// 经 imports/external_refs 自动装入、不在声明集的本体，loaded_versions 快照键入路由匹配集。

import (
	"path/filepath"
	"testing"
)

func TestRunningByOntologyClosureAdded_Req540(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "rt540.db"), filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	p := &Profile{ID: "pd", Name: "闭包装载方案", Engine: "oxigraph", OntologyIDs: []string{"o4"}, Config: "{}", Port: 9202}
	if err := st.Create(p); err != nil {
		t.Fatal(err)
	}
	if err := st.SetStatus("pd", "running", "", ""); err != nil {
		t.Fatal(err)
	}
	// 启动成功后闭包补全本体 o5 入 loaded_versions 快照（声明集只有 o4）
	if err := st.SetLoadedVersions("pd", `{"o4":1,"o5":2}`); err != nil {
		t.Fatal(err)
	}
	// 修复前：o5 只在快照不在声明集 → 路由不中；修复后：可按自身 id 路由
	got, err := st.RunningByOntology("o5")
	if err != nil || got == nil || got.ID != "pd" {
		t.Fatalf("闭包补全本体应可路由: err=%v got=%+v", err, got)
	}
	// 声明集本体路由行为不变
	if got, err := st.RunningByOntology("o4"); err != nil || got == nil || got.ID != "pd" {
		t.Fatalf("声明集本体路由应保持: err=%v got=%+v", err, got)
	}
	// RouteAmbiguities 同口径：o5 单方案命中不入歧义清单
	amb, err := st.RouteAmbiguities()
	if err != nil {
		t.Fatal(err)
	}
	if len(amb) != 0 {
		t.Fatalf("单方案命中不应有歧义: %+v", amb)
	}
	// 第二个运行方案也装载 o5（快照键）→ 歧义观测面呈现
	p2 := &Profile{ID: "pe", Name: "第二方案", Engine: "oxigraph", OntologyIDs: []string{}, Config: "{}", Port: 9203}
	if err := st.Create(p2); err != nil {
		t.Fatal(err)
	}
	if err := st.SetStatus("pe", "running", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetLoadedVersions("pe", `{"o5":2}`); err != nil {
		t.Fatal(err)
	}
	amb2, err := st.RouteAmbiguities()
	if err != nil {
		t.Fatal(err)
	}
	if len(amb2) != 1 || amb2[0].OntologyID != "o5" || amb2[0].ResolvedID != "pd" {
		t.Fatalf("o5 双方案应入歧义（resolved=最早 pd）: %+v", amb2)
	}
	// 停用后歧义与路由同时收敛
	if err := st.SetStatus("pe", "stopped", "", ""); err != nil {
		t.Fatal(err)
	}
	if got, err := st.RunningByOntology("o5"); err != nil || got == nil || got.ID != "pd" {
		t.Fatalf("停用 pe 后 o5 仍路由 pd: err=%v got=%+v", err, got)
	}
}
