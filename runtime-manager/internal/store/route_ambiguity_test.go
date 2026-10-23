package store

// REQ-384/M180：多本体路由语义收口——歧义观测与告警路径单测。
import (
	"path/filepath"
	"testing"
)

func TestRouteAmbiguities(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "rt.db"), filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	mk := func(id string, ontos ...string) {
		p := &Profile{ID: id, Name: id + "-name", Engine: "oxigraph", OntologyIDs: ontos, Config: "{}", Port: 9201}
		if err := st.Create(p); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		if err := st.SetStatus(id, "running", "", ""); err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
	}
	mk("pa", "o1", "o2")
	mk("pb", "o1")
	mk("pc", "o3")

	// 歧义扫描：o1 双方案命中；o2/o3 单方案不入列
	amb, err := st.RouteAmbiguities()
	if err != nil {
		t.Fatalf("ambiguities: %v", err)
	}
	if len(amb) != 1 || amb[0].OntologyID != "o1" {
		t.Fatalf("应仅 o1 歧义: %+v", amb)
	}
	if len(amb[0].Profiles) != 2 || amb[0].Profiles[0].ID != "pa" || amb[0].Profiles[1].ID != "pb" {
		t.Fatalf("profiles 应按 created_at 序: %+v", amb[0].Profiles)
	}
	if amb[0].ResolvedID != "pa" {
		t.Fatalf("resolved 应为最早方案 pa: %s", amb[0].ResolvedID)
	}

	// RunningByOntology：多命中仍路由最早（确定序），返回 pa
	p, err := st.RunningByOntology("o1")
	if err != nil || p.ID != "pa" {
		t.Fatalf("路由应取最早 pa: %v %+v", err, p)
	}
	// 单命中正常
	p2, err := st.RunningByOntology("o3")
	if err != nil || p2.ID != "pc" {
		t.Fatalf("单命中应直取: %v %+v", err, p2)
	}
	// 无命中：沿既有契约返回 (nil, nil)（facade route 侧判 p==nil 转 ONTOLOGY_SERVICE_UNAVAILABLE）
	p3, err := st.RunningByOntology("o-none")
	if err != nil || p3 != nil {
		t.Fatalf("无命中应 (nil,nil): %v %+v", err, p3)
	}

	// 歧义消除：停一个方案后 o1 出列
	if err := st.SetStatus("pb", "stopped", "", ""); err != nil {
		t.Fatal(err)
	}
	amb2, _ := st.RouteAmbiguities()
	if len(amb2) != 0 {
		t.Fatalf("停用后歧义应清空: %+v", amb2)
	}
}
