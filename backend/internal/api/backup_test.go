package api

// REQ-411/M207：备份 zip 单测——三库快照在位 + companion-graph-snapshots 拷贝。
import (
	"archive/zip"
	"bytes"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestExportBackup(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ag, _ := st.CreateAgent(&store.Agent{Name: "bk"})
	_, _ = st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID})
	// companion 快照文件
	compDir := filepath.Join(dir, "companion-graph-snapshots")
	os.MkdirAll(compDir, 0o755)
	os.WriteFile(filepath.Join(compDir, "ont-x.json"), []byte(`{}`), 0o644)

	s := &Server{Store: st, DBPath: filepath.Join(dir, "platform.db")}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	s.exportBackup(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("zip 解析: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"data/platform.db", "data/companion-graph-snapshots/ont-x.json"} {
		if !names[want] {
			t.Fatalf("缺 %s（got %v）", want, names)
		}
	}
}

// TestExportBackupExtended REQ-517/M324（P04）：覆盖面扩四项+失败聚合——projects/.secret/
// companion-snapshots/agent-files 子目录入包，manifest.json 记 skipped，X-Backup-Warnings 头携带跳过数。
func TestExportBackupExtended(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	write := func(rel, content string) {
		abs := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(abs), 0o755)
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("projects/p1/README.md", "项目文件")
	write("projects/p1/.git/HEAD", "git-metadata") // .git 一并入包（唯一不可从库重建的用户产出）
	write(".secret/key.bin", "secret-key")
	write("companion-snapshots/snap-1.json", `{"v":1}`)
	write("agent-files/ag-1/artifact.txt", "子目录产物") // agent-files 子目录（原平铺跳过修复点）

	s := &Server{Store: st, DBPath: filepath.Join(dir, "platform.db")}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	s.exportBackup(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("zip 解析: %v", err)
	}
	names := map[string]bool{}
	var manifest []byte
	for _, f := range zr.File {
		names[f.Name] = true
		if f.Name == "manifest.json" {
			rc, _ := f.Open()
			manifest, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	for _, want := range []string{
		"data/platform.db",
		"data/projects/p1/README.md",
		"data/projects/p1/.git/HEAD",
		"data/.secret/key.bin",
		"data/companion-snapshots/snap-1.json",
		"data/agent-files/ag-1/artifact.txt",
		"manifest.json",
	} {
		if !names[want] {
			t.Fatalf("缺 %s（got %v）", want, names)
		}
	}
	// manifest 记卫星库缺失（本测试未建 ontology/runtime.db——not_found 如实列出）
	if len(manifest) == 0 {
		t.Fatal("manifest.json 应在 zip 根")
	}
	if !strings.Contains(string(manifest), "ontology.db") || !strings.Contains(string(manifest), "not_found") {
		t.Fatalf("manifest 应如实记录卫星库 not_found: %.200s", manifest)
	}
	// X-Backup-Warnings 头=跳过数
	if w := rec.Header().Get("X-Backup-Warnings"); w == "" {
		t.Fatal("部分跳过时 X-Backup-Warnings 头应在位")
	}
}

// TestExportBackupPlatformFail REQ-517：主库快照失败=500（静默 200 缺库 zip 形态根除）。
func TestExportBackupPlatformFail(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st, DBPath: filepath.Join(dir, "platform.db")}
	// 平台库源损坏形态：DBPath 指向不存在文件 → VACUUM INTO 失败 → 500
	s.DBPath = filepath.Join(dir, "gone", "platform.db")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	s.exportBackup(rec, req)
	if rec.Code != 500 {
		t.Fatalf("主库快照失败应 500，got %d", rec.Code)
	}
}

// TestKBDocContent REQ-523/M330（P05）：内容读取端点——chunk 按 seq 拼接+doc 归属守卫
//（不属于该 KB 的 doc 404；KB 模块未接入 503）。
func TestKBDocContent(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	kb, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "溯源库", Mode: "rag"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := st.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: kb.ID, Title: "溯源文档", Status: "success"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertKnowledgeChunks([]*store.KnowledgeChunk{
		{ID: store.NewID(), KBID: kb.ID, DocID: doc.ID, Seq: 1, Content: "第二段"},
		{ID: store.NewID(), KBID: kb.ID, DocID: doc.ID, Seq: 0, Content: "第一段"},
	}); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, DBPath: filepath.Join(dir, "platform.db")}

	req := httptest.NewRequest("GET", "/x", nil)
	req.SetPathValue("id", kb.ID)
	req.SetPathValue("did", doc.ID)
	rec := httptest.NewRecorder()
	s.kbDocContent(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"doc_id":"`+doc.ID+`"`) || !strings.Contains(rec.Body.String(), "第一段") {
		t.Fatalf("应含 doc_id 与拼接内容: %.200s", rec.Body.String())
	}

	// 归属守卫：另一 KB 的 doc id → 404
	kb2, _ := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "别的库", Mode: "rag"})
	req2 := httptest.NewRequest("GET", "/x", nil)
	req2.SetPathValue("id", kb2.ID)
	req2.SetPathValue("did", doc.ID)
	rec2 := httptest.NewRecorder()
	s.kbDocContent(rec2, req2)
	if rec2.Code != 404 {
		t.Fatalf("跨 KB doc 应 404，got %d", rec2.Code)
	}
}
