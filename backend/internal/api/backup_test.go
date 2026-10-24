package api

// REQ-411/M207：备份 zip 单测——三库快照在位 + companion-graph-snapshots 拷贝。
import (
	"archive/zip"
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
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
