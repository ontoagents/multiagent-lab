package api

// REQ-413②③/M209：规模护栏单测——KB 大文档导入护栏（超限 400 引导拆分）+SQLite 写冲突
// 诚实提示（writeErr SQLITE_BUSY → 503 人话+重试指引）。
// REQ-439/M235：base64 二进制通道单测（b64 解码→filename 分发抽取→入库）+doc.dim 落库回读。
import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/kb"
	"github.com/ontoagents/ontoagents/backend/internal/store"
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

// REQ-439/M235：base64 二进制通道全链——最小 DOCX 经 b64 无损到服 → filename 分发抽取 →
// wiki 库 chunksOnly（零 embedding）落 chunk 池到 success；dim=0 如实零维。
func TestKBImportBase64DOCX(t *testing.T) {
	s := newKBGuardFixture(t)
	kbsvc, err := kb.NewService(s.Store, nil, "sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	s.KB = kbsvc
	kbw, err := s.Store.CreateKnowledgeBase(&store.KnowledgeBase{Name: "b64-wiki", Mode: "wiki"})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w0, _ := zw.Create("word/document.xml")
	w0.Write([]byte(`<w:document><w:body><w:p><w:r><w:t>REQ-439 二进制通道验证：本体工程与知识库的交集治理五原则。</w:t></w:r></w:p></w:body></w:document>`))
	zw.Close()

	b, _ := json.Marshal(map[string]any{
		"title": "交集治理.docx", "filename": "交集治理.docx",
		"content_b64": base64.StdEncoding.EncodeToString(buf.Bytes()),
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/kb/"+kbw.ID+"/docs", bytes.NewReader(b))
	req.SetPathValue("id", kbw.ID)
	s.importKBDoc(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("b64 DOCX 导入应 201，得 %d %s", w.Code, w.Body.String())
	}
	var doc store.KnowledgeDoc
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	// REQ-440：端点已异步化——响应为 indexing 态，轮询至后台收敛
	if doc.Status != "indexing" {
		t.Fatalf("异步端点应返回 indexing 态，得 %s", doc.Status)
	}
	for i := 0; i < 100 && doc.Status == "indexing"; i++ {
		time.Sleep(50 * time.Millisecond)
		var err error
		got, err := s.Store.GetKnowledgeDoc(doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		doc = *got
	}
	if doc.Status != "success" || doc.ChunkCount == 0 {
		t.Fatalf("wiki 全链应收敛 success 且有 chunks，得 status=%s chunks=%d", doc.Status, doc.ChunkCount)
	}
	if doc.Dim != 0 {
		t.Fatalf("wiki 库零 embedding，dim 应如实为 0，得 %d", doc.Dim)
	}
	chunks, err := s.Store.ListKnowledgeChunksByDoc(doc.ID)
	if err != nil || len(chunks) == 0 {
		t.Fatalf("chunks 落库不符: %v %d", err, len(chunks))
	}
	if !strings.Contains(chunks[0].Content, "二进制通道验证") {
		t.Fatalf("抽取文本未进 chunk 池: %q", chunks[0].Content)
	}
}

// REQ-439：坏 base64 400 明确报错（不落到「抽取后文本为空」误导文案）。
func TestKBImportBase64Bad(t *testing.T) {
	s := newKBGuardFixture(t)
	kbw, err := s.Store.CreateKnowledgeBase(&store.KnowledgeBase{Name: "b64-bad"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"title": "x", "filename": "x.docx", "content_b64": "!!!not-base64!!!"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/kb/"+kbw.ID+"/docs", bytes.NewReader(b))
	req.SetPathValue("id", kbw.ID)
	s.importKBDoc(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("坏 b64 应 400，得 %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "content_b64 不是合法 base64") {
		t.Fatalf("报错文案不符: %s", w.Body.String())
	}
}

// REQ-439/REQ-54：doc.dim 落库回读（rag 库 embedding 维度回写——store 层往返；
// 全链 dim 值依赖真实 embedding 连接，headless 冒烟沿 20 号 2.26b）。
func TestKBDocDimPersist(t *testing.T) {
	s := newKBGuardFixture(t)
	kbw, err := s.Store.CreateKnowledgeBase(&store.KnowledgeBase{Name: "dim-kb"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := s.Store.CreateKnowledgeDoc(&store.KnowledgeDoc{KBID: kbw.ID, Title: "dim-doc"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpdateKnowledgeDocStatus(doc.ID, "success", 7, 1536, ""); err != nil {
		t.Fatal(err)
	}
	got, err := s.Store.GetKnowledgeDoc(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Dim != 1536 || got.ChunkCount != 7 || got.Status != "success" || got.UpdatedAt == "" {
		t.Fatalf("dim/chunks/status/updated_at 回读不符: dim=%d chunks=%d status=%s updated=%q", got.Dim, got.ChunkCount, got.Status, got.UpdatedAt)
	}
}
