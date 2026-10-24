package api

// REQ-413②③/M209：规模护栏单测——KB 大文档导入护栏（超限 400 引导拆分）+SQLite 写冲突
// 诚实提示（writeErr SQLITE_BUSY → 503 人话+重试指引）。
import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func newKBGuardFixture(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &Server{Store: st}
}

func TestKBImportSizeGuard(t *testing.T) {
	s := newKBGuardFixture(t)
	kb, err := s.Store.CreateKnowledgeBase(&store.KnowledgeBase{Name: "guard-kb"})
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("字", MaxKBImportBytes/3+100) // UTF-8 每字 3B → 超 10MB 上限
	b, _ := json.Marshal(map[string]any{"title": "大文档", "content": huge})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/kb/"+kb.ID+"/docs", bytes.NewReader(b))
	req.SetPathValue("id", kb.ID)
	s.importKBDoc(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("超限应 400，得 %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "文档过大") || !strings.Contains(w.Body.String(), "拆分") {
		t.Fatalf("护栏文案不符: %s", w.Body.String())
	}
}

func TestWriteErrSQLiteBusy(t *testing.T) {
	w := httptest.NewRecorder()
	writeErr(w, errors.New("disk I/O error: database is locked (5) (SQLITE_BUSY)"))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("busy 应 503，得 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "数据库写入冲突") || !strings.Contains(w.Body.String(), "重试") {
		t.Fatalf("人话提示不符: %s", w.Body.String())
	}
	// 普通错误不受影响（仍 500）
	w2 := httptest.NewRecorder()
	writeErr(w2, errors.New("some other failure"))
	if w2.Code != http.StatusInternalServerError {
		t.Fatalf("普通错误应 500，得 %d", w2.Code)
	}
	if isSQLiteBusy(nil) || isSQLiteBusy(errors.New("normal")) {
		t.Fatal("isSQLiteBusy 判定不符")
	}
}
