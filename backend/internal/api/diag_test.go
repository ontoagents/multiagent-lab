package api

// REQ-410/M206：诊断包单测——脱敏（不出 base_url/api_key）+ healthz 样本在位。
import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func TestExportDiagnostics(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	_, _ = st.CreateConnection(&store.ModelConnection{
		Name: "secret-conn", ConnType: "chat", Protocol: "openai_compat",
		BaseURL: "https://sk-secret.example.com/v1", ModelName: "m", Enabled: true,
	}, []byte("fake-encrypted-key"))
	s := &Server{Store: st}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/diagnostics/export", nil)
	s.exportDiagnostics(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"healthz"`) || !strings.Contains(body, `"recent_errors"`) {
		t.Fatalf("应含 healthz 与 recent_errors: %.200s", body)
	}
	// 脱敏：不出 baseURL 与密文
	if strings.Contains(body, "sk-secret.example.com") || strings.Contains(body, "fake-encrypted-key") {
		t.Fatal("诊断包泄漏敏感信息")
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
}
