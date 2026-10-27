package api

// REQ-451 单测：①/api/kb 列表 official 标注（平台知识库单源判定）②/api/tools group 透出
// ③doc_read 分段入参经 handler 面的 DTO 形态（listTools 含新工具与分组）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/chat"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

func TestListKBOfficialFlag(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateKnowledgeBase(&store.KnowledgeBase{Name: "我的私有库", Mode: "rag"}); err != nil {
		t.Fatal(err)
	}
	if _, err := chat.EnsurePlatformKB(t.Context(), st); err != nil {
		t.Fatal(err)
	}

	s := &Server{Store: st}
	rec := httptest.NewRecorder()
	s.listKB(rec, httptest.NewRequest(http.MethodGet, "/api/kb", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("listKB 应 200: %d", rec.Code)
	}
	var kbs []store.KnowledgeBase
	if err := json.Unmarshal(rec.Body.Bytes(), &kbs); err != nil {
		t.Fatal(err)
	}
	official := map[string]bool{}
	for _, k := range kbs {
		official[k.Name] = k.Official
	}
	if !official[chat.PlatformKBName] {
		t.Fatalf("平台知识库应标 official=true：%v", official)
	}
	if official["我的私有库"] {
		t.Fatalf("普通库不应标 official：%v", official)
	}
}

func TestListToolsGroupAndNewEntries(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	reg := tool.NewRegistry()
	if err := chat.RegisterAssistantTools(reg, chat.AssistantDeps{Store: st, DocsRoot: t.TempDir(), KnowledgeRoot: t.TempDir(), DefaultPrompt: "P"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Tools: reg}
	rec := httptest.NewRecorder()
	s.listTools(rec, httptest.NewRequest(http.MethodGet, "/api/tools", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("listTools 应 200: %d", rec.Code)
	}
	var tools []toolEntryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &tools); err != nil {
		t.Fatal(err)
	}
	byID := map[string]toolEntryDTO{}
	for _, tl := range tools {
		byID[tl.ID] = tl
	}
	for _, id := range []string{"doc_read", "doc_list", "doc_grep"} {
		tl, ok := byID[id]
		if !ok {
			t.Fatalf("工具 %s 应透出：%v", id, byID)
		}
		if tl.Group != chat.ToolGroupPlatformKnowledge {
			t.Fatalf("%s group 应为平台知识：%+v", id, tl)
		}
	}
}
