package store

// REQ-495/M304 笔记本存储层单测：容器 CRUD/页面 CRUD/工作区绑定一对多（SET NULL）/KB 回流登记
// （引用语义幂等）/两源触碰同步在 handler 层（store 侧验页面 upsert 数据面）。

import (
	"errors"
	"path/filepath"
	"testing"
)

func newNoteTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestNotebookCRUD(t *testing.T) {
	st := newNoteTestStore(t)
	n, err := st.CreateNotebook(&Notebook{Name: "学习笔记"})
	if err != nil {
		t.Fatal(err)
	}
	if n.ID == "" || n.WorkspaceID != nil {
		t.Fatalf("创建往返失真: %+v", n)
	}
	// 工作区绑定（一对多——同工作区可挂多本）
	ws, err := st.CreateWorkspace(&Workspace{Name: "w"})
	if err != nil {
		t.Fatal(err)
	}
	wsID := ws.ID
	n2, err := st.CreateNotebook(&Notebook{Name: "工作区笔记", WorkspaceID: &wsID})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := st.ListNotebooks(ws.ID)
	if err != nil || len(bound) != 1 || bound[0].ID != n2.ID {
		t.Fatalf("工作区过滤失真: %v", bound)
	}
	// 绑定不存在工作区→400
	bad := "ws_missing"
	if _, err := st.CreateNotebook(&Notebook{Name: "x", WorkspaceID: &bad}); err == nil {
		t.Fatal("不存在工作区应 400")
	}
	// 删工作区→笔记本回落个人资产（SET NULL 页面保留）
	if err := st.DeleteWorkspace(ws.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetNotebook(n2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkspaceID != nil {
		t.Fatalf("删工作区应 SET NULL: %+v", got.WorkspaceID)
	}
	if err := st.DeleteNotebook(n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetNotebook(n.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("删后应 404")
	}
}

// REQ-495②段尾项（绑定 UI 轮）：绑定/换绑/解绑走 UpdateNotebook（PUT 全量语义——UI 直连端点）。
// 解绑空串必须归一 NULL：DSN foreign_keys(1) 下 '' 不在 workspace(id) 会 FK 违约 500（修复回归）。
func TestNotebookBindUnbindViaUpdate(t *testing.T) {
	st := newNoteTestStore(t)
	wsA, _ := st.CreateWorkspace(&Workspace{Name: "A"})
	wsB, _ := st.CreateWorkspace(&Workspace{Name: "B"})
	n, err := st.CreateNotebook(&Notebook{Name: "成果笔记"})
	if err != nil {
		t.Fatal(err)
	}
	// 绑定 A
	aID := wsA.ID
	bound, err := st.UpdateNotebook(&Notebook{ID: n.ID, Name: n.Name, WorkspaceID: &aID})
	if err != nil || bound.WorkspaceID == nil || *bound.WorkspaceID != aID {
		t.Fatalf("绑定 A 失真: %+v err=%v", bound, err)
	}
	// 换绑 B
	bID := wsB.ID
	moved, err := st.UpdateNotebook(&Notebook{ID: n.ID, Name: n.Name, WorkspaceID: &bID})
	if err != nil || moved.WorkspaceID == nil || *moved.WorkspaceID != bID {
		t.Fatalf("换绑 B 失真: %+v err=%v", moved, err)
	}
	// 解绑（空串→NULL，FK 不违约）
	empty := ""
	got, err := st.UpdateNotebook(&Notebook{ID: n.ID, Name: n.Name, WorkspaceID: &empty})
	if err != nil {
		t.Fatalf("解绑（空串）应归一 NULL 而非 FK 违约: %v", err)
	}
	if got.WorkspaceID != nil {
		t.Fatalf("解绑后应为 NULL: %+v", got.WorkspaceID)
	}
	if boundList, _ := st.ListNotebooks(wsA.ID); len(boundList) != 0 {
		t.Fatalf("解绑后 A 过滤应空: %v", boundList)
	}
}

func TestNotePageCRUDAndTouch(t *testing.T) {
	st := newNoteTestStore(t)
	n, _ := st.CreateNotebook(&Notebook{Name: "nb"})
	p1, err := st.CreateNotePage(&NotePage{NotebookID: n.ID, Title: "第一页", ContentMD: "# 一"})
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := st.CreateNotePage(&NotePage{NotebookID: n.ID, Title: "第二页"})
	if p1.Position >= p2.Position {
		t.Fatalf("position 应递增: %d %d", p1.Position, p2.Position)
	}
	pages, _ := st.ListNotePages(n.ID)
	if len(pages) != 2 {
		t.Fatalf("页面数失真: %d", len(pages))
	}
	// 更新（触碰）
	p1.ContentMD = "# 一（更新）"
	p1.SourceMtime = "2026-10-31T00:00:00Z"
	up, err := st.UpdateNotePage(p1)
	if err != nil {
		t.Fatal(err)
	}
	if up.ContentMD != "# 一（更新）" || up.SourceMtime == "" {
		t.Fatalf("更新失真: %+v", up)
	}
	if err := st.DeleteNotePage(p2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetNotePage(p2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("删后应 404")
	}
	// 页面归属校验：不存在笔记本→400
	if _, err := st.CreateNotePage(&NotePage{NotebookID: "nb_missing", Title: "x"}); err == nil {
		t.Fatal("不存在笔记本应 400")
	}
}

func TestNotebookSourceRegistration(t *testing.T) {
	st := newNoteTestStore(t)
	kb, err := st.CreateKnowledgeBase(&KnowledgeBase{Name: "回流库", Mode: "rag"})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := st.CreateNotebook(&Notebook{Name: "nb"})
	// 登记+幂等
	if _, err := st.CreateNotebookSource(kb.ID, n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateNotebookSource(kb.ID, n.ID); err != nil {
		t.Fatalf("重复登记应幂等: %v", err)
	}
	kbs, err := st.ListNotebookSourceKBs(n.ID)
	if err != nil || len(kbs) != 1 || kbs[0] != kb.ID {
		t.Fatalf("登记清单失真: %v", kbs)
	}
	// 不存在 KB→400
	if _, err := st.CreateNotebookSource("kb_missing", n.ID); err == nil {
		t.Fatal("不存在 KB 应 400")
	}
	// 解除
	if err := st.DeleteNotebookSource(kb.ID, n.ID); err != nil {
		t.Fatal(err)
	}
	if kbs, _ := st.ListNotebookSourceKBs(n.ID); len(kbs) != 0 {
		t.Fatal("解除后应空")
	}
}
