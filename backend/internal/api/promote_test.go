package api

// REQ-529/M336：沉淀出料口单测——①产物一键入库（项目文件文本层→ImportAsync）②二进制拒绝
// ③归属守卫。伴生晋升 SPARQL 面由 companion 既有真链测试承载（本测不重复）。

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kbpkg "github.com/ontoagents/ontoagents/backend/internal/kb"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestImportProjectFileToKB(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	bound := filepath.Join(dir, "bound")
	proj, err := st.CreateProject(&store.Project{Name: "沉淀项目", LocalDir: bound})
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(bound, 0o755)
	os.WriteFile(filepath.Join(bound, "artifact.md"), []byte("## 调研结论\n伴生沉淀应入库检索"), 0o644)
	pfID, err := st.InsertProjectFile(&store.ProjectFile{ProjectID: proj.ID, Name: "artifact.md", Path: "artifact.md", Source: "save_file"})
	if err != nil {
		t.Fatal(err)
	}
	pf, _ := st.GetProjectFile(pfID)
	kb, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "入库目标库", Mode: "rag"})
	if err != nil {
		t.Fatal(err)
	}
	kbSvc, err := kbpkg.NewService(st, nil, "sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, KB: kbSvc, DBPath: filepath.Join(dir, "platform.db")}

	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"project_id":"`+proj.ID+`","file_id":"`+pf.ID+`"}`))
	req.SetPathValue("id", kb.ID)
	rec := httptest.NewRecorder()
	s.importProjectFileToKB(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"indexing"`) && !strings.Contains(rec.Body.String(), "doc_id") {
		t.Fatalf("应返回入库 doc: %.200s", rec.Body.String())
	}

	// 二进制拒绝
	os.WriteFile(filepath.Join(bound, "bin.dat"), []byte{0x00, 0x01, 0x02}, 0o644)
	pfbID, _ := st.InsertProjectFile(&store.ProjectFile{ProjectID: proj.ID, Name: "bin.dat", Path: "bin.dat", Source: "upload"})
	pfb, _ := st.GetProjectFile(pfbID)
	req2 := httptest.NewRequest("POST", "/x", strings.NewReader(`{"project_id":"`+proj.ID+`","file_id":"`+pfb.ID+`"}`))
	req2.SetPathValue("id", kb.ID)
	rec2 := httptest.NewRecorder()
	s.importProjectFileToKB(rec2, req2)
	if rec2.Code != 400 {
		t.Fatalf("二进制应 400 指引富格式通道，got %d", rec2.Code)
	}

	// 归属守卫：别的项目的 file_id → 404
	req3 := httptest.NewRequest("POST", "/x", strings.NewReader(`{"project_id":"other-project","file_id":"`+pf.ID+`"}`))
	req3.SetPathValue("id", kb.ID)
	rec3 := httptest.NewRecorder()
	s.importProjectFileToKB(rec3, req3)
	if rec3.Code != 404 {
		t.Fatalf("跨项目归属应 404，got %d", rec3.Code)
	}
}
