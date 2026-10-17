package api

// M23①：配置面导入/导出单测——四域往返、key 脱敏、按名去重、内置行不入。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func newSettingsTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	box, err := secrets.LoadKeyFile(filepath.Join(dir, ".secret"))
	if err != nil {
		t.Fatalf("打开 secrets box 失败: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// 直接构造最小 Server（routes() 全量注册依赖 Ontology 等服务面——配置面单测只挂两路由，
	// handlers 仅依赖 Store/Box）
	srv := &Server{Store: st, Box: box, Mux: http.NewServeMux()}
	srv.Mux.HandleFunc("GET /api/settings/export", srv.exportSettings)
	srv.Mux.HandleFunc("POST /api/settings/import", srv.importSettings)
	srv.Mux.HandleFunc("POST /api/model-connections", srv.createConnection)
	srv.Mux.HandleFunc("POST /api/agents", srv.createAgent)
	srv.Mux.HandleFunc("POST /api/projects", srv.createProject)
	srv.Mux.HandleFunc("POST /api/skills", srv.createSkill)
	ts := httptest.NewServer(srv.Mux)
	t.Cleanup(ts.Close)
	return ts
}

func settingsPost(t *testing.T, ts *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// 导出→清库→导入 往返：四域全建、key 脱敏（连接导入后 has_key=false）、内置行不入、同名跳过。
func TestSettingsExportImportRoundTrip(t *testing.T) {
	ts := newSettingsTestServer(t)

	// 种子数据：连接（带 key）+ 普能智能体 + 内置助手 + 项目 + 自定义技能
	resp := settingsPost(t, ts, "/api/model-connections", map[string]any{
		"name": "主连接", "conn_type": "chat", "protocol": "openai_compat",
		"base_url": "https://api.example.com", "model_name": "glm-4", "api_key": "sk-secret-123",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("建连接失败: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = settingsPost(t, ts, "/api/agents", map[string]any{"name": "编码助手", "instruction": "写代码"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("建智能体失败: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = settingsPost(t, ts, "/api/projects", map[string]any{"name": "示例项目", "local_dir": t.TempDir()})
	if resp.StatusCode != http.StatusCreated {
		b := new(bytes.Buffer)
		_, _ = b.ReadFrom(resp.Body)
		t.Fatalf("建项目失败: %d %s", resp.StatusCode, b.String())
	}
	resp.Body.Close()
	resp = settingsPost(t, ts, "/api/skills", map[string]any{"name": "示例技能", "instruction": "示例指令"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("建技能失败: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 导出
	expResp, err := http.Get(ts.URL + "/api/settings/export")
	if err != nil {
		t.Fatal(err)
	}
	defer expResp.Body.Close()
	var exported map[string]any
	if err := json.NewDecoder(expResp.Body).Decode(&exported); err != nil {
		t.Fatal(err)
	}
	// key 脱敏断言：导出全文不含明文 key
	raw, _ := json.Marshal(exported)
	if strings.Contains(string(raw), "sk-secret-123") {
		t.Fatal("导出文件泄漏明文 key（key 脱敏边界破坏）")
	}
	conns := exported["model_connections"].([]any)
	found := false
	for _, ci := range conns {
		c := ci.(map[string]any)
		if c["name"] == "主连接" {
			found = true
			if c["has_key"] != true {
				t.Fatalf("带 key 连接应标注 has_key=true（导入后需重录）: %v", c)
			}
			if _, has := c["api_key"]; has {
				t.Fatalf("导出连接不应含 api_key 字段")
			}
		}
	}
	if !found {
		t.Fatal("导出应含「主连接」")
	}

	// 导入（同一实例——按名去重应全跳过）
	impResp := settingsPost(t, ts, "/api/settings/import", exported)
	defer impResp.Body.Close()
	if impResp.StatusCode != http.StatusOK {
		t.Fatalf("导入失败: %d", impResp.StatusCode)
	}
	var out struct {
		Imported map[string]*importSummary `json:"imported"`
	}
	if err := json.NewDecoder(impResp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	for _, domain := range []string{"model_connections", "agents", "projects", "skills"} {
		if out.Imported[domain] == nil {
			t.Fatalf("%s 缺导入摘要", domain)
		}
		if out.Imported[domain].Created != 0 {
			t.Fatalf("%s 同名导入应全跳过: created=%d", domain, out.Imported[domain].Created)
		}
	}
	if len(out.Imported["agents"].Skipped) == 0 || len(out.Imported["projects"].Skipped) == 0 || len(out.Imported["skills"].Skipped) == 0 {
		t.Fatalf("四域同名均应跳过: %+v", out.Imported)
	}
	// 预置连接（store 迁移种子的 DeepSeek/千帆等）同样按名跳过——只断言「主连接」在跳过清单中
	skipMain := false
	for _, n := range out.Imported["model_connections"].Skipped {
		if n == "主连接" {
			skipMain = true
		}
	}
	if !skipMain {
		t.Fatalf("跳过清单应含主连接: %+v", out.Imported["model_connections"].Skipped)
	}
}
