// REQ-214/M46 连接器 API 单测：CRUD/凭据脱敏（明文与密文均不回传）/删除保护（引用 409、内置 400）/test 状态回写。
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/inference"
	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func newConnectorAPIFixture(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	return &Server{Store: st, Box: box}
}

func doConnectorReq(s *Server, method, target string, body any) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, rd)
	// 提取路径参数（/api/connectors/{id}[/test]）
	if rest, ok := strings.CutPrefix(target, "/api/connectors/"); ok {
		id := rest
		if i := strings.Index(id, "/"); i >= 0 {
			id = id[:i]
		}
		req.SetPathValue("id", id)
	}
	switch method {
	case "GET":
		s.listConnectors(w, req)
	case "POST":
		if strings.HasSuffix(target, "/test") {
			s.testConnector(w, req)
		} else if strings.HasSuffix(target, "/preview") {
			s.previewConnector(w, req)
		} else {
			s.createConnector(w, req)
		}
	case "PUT":
		s.updateConnector(w, req)
	case "DELETE":
		s.deleteConnector(w, req)
	}
	return w
}

func TestConnectorAPIEncryptedAndMasked(t *testing.T) {
	s := newConnectorAPIFixture(t)
	// 创建（SSH 连接器带凭据）
	w := doConnectorReq(s, "POST", "/api/connectors", map[string]any{
		"kind": "ssh", "name": "ops-ssh",
		"config":  map[string]any{"host": "10.0.0.5", "user": "ops"},
		"credentials": map[string]any{"password": "SUPER-SECRET"},
	})
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		ID             string `json:"id"`
		HasCredentials bool   `json:"has_credentials"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if !out.HasCredentials {
		t.Fatal("has_credentials 应为 true")
	}
	// 脱敏断言：响应体不含明文，也不含密文（BASE64 形态）
	if strings.Contains(w.Body.String(), "SUPER-SECRET") {
		t.Fatal("响应泄漏明文凭据")
	}
	// 库内为密文
	c, _ := s.Store.GetConnectorByName("ops-ssh")
	if len(c.CredentialsEncrypted) == 0 || bytes.Contains(c.CredentialsEncrypted, []byte("SUPER-SECRET")) {
		t.Fatal("库内应为密文而非明文")
	}
	// 列表脱敏（明文与凭据字段均不回传；has_credentials 布尔除外）
	w = doConnectorReq(s, "GET", "/api/connectors", nil)
	if strings.Contains(w.Body.String(), "SUPER-SECRET") || strings.Contains(w.Body.String(), `"credentials"`) {
		t.Fatal("列表泄漏凭据（明文或字段）")
	}
	// 更新不带凭据 = 保留
	w = doConnectorReq(s, "PUT", "/api/connectors/"+out.ID, map[string]any{
		"kind": "ssh", "name": "ops-ssh", "description": "改描述",
		"config": map[string]any{"host": "10.0.0.6", "user": "ops"},
	})
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	c2, _ := s.Store.GetConnector(out.ID)
	if len(c2.CredentialsEncrypted) == 0 {
		t.Fatal("更新未带凭据应保留原凭据")
	}
	// 更新携带新凭据 = 覆盖
	w = doConnectorReq(s, "PUT", "/api/connectors/"+out.ID, map[string]any{
		"kind": "ssh", "name": "ops-ssh",
		"config":      map[string]any{"host": "10.0.0.6", "user": "ops"},
		"credentials": map[string]any{"password": "NEW-SECRET"},
	})
	c3, _ := s.Store.GetConnector(out.ID)
	if bytes.Contains(c3.CredentialsEncrypted, []byte("CIPHER")) || len(c3.CredentialsEncrypted) == 0 {
		t.Fatal("新凭据应重新加密")
	}
	plain, err := s.Box.Decrypt(c3.CredentialsEncrypted)
	if err != nil || !strings.Contains(plain, "NEW-SECRET") {
		t.Fatalf("新凭据应可解密: %v %s", err, plain)
	}
}

func TestConnectorDeleteProtection(t *testing.T) {
	s := newConnectorAPIFixture(t)
	builtin, err := s.Store.CreateConnector(&store.Connector{Kind: store.ConnectorKindMCP, Name: "builtin-fixture", Config: map[string]any{"url": "http://x/mcp"}, IsBuiltin: true})
	if err != nil {
		t.Fatal(err)
	}
	// 内置不可删
	w := doConnectorReq(s, "DELETE", "/api/connectors/"+builtin.ID, nil)
	if w.Code != 400 {
		t.Fatalf("内置删除应 400, got %d", w.Code)
	}
	// 被 agent 引用 → 409
	c, _ := s.Store.CreateConnector(&store.Connector{Kind: store.ConnectorKindMCP, Name: "refd", Config: map[string]any{"url": "http://x/mcp"}})
	if _, err := s.Store.CreateAgent(&store.Agent{Name: "user-agent", MaxIteration: 5, Connectors: []string{c.ID}}); err != nil {
		t.Fatal(err)
	}
	w = doConnectorReq(s, "DELETE", "/api/connectors/"+c.ID, nil)
	if w.Code != 409 {
		t.Fatalf("引用删除应 409, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "user-agent") {
		t.Fatal("409 应附引用者列表")
	}
	// 无引用可删
	if _, err := s.Store.CreateAgent(&store.Agent{Name: "lonely", MaxIteration: 5}); err != nil {
		t.Fatal(err)
	}
	c2, _ := s.Store.CreateConnector(&store.Connector{Kind: store.ConnectorKindMCP, Name: "free", Config: map[string]any{"url": "http://x/mcp"}})
	w = doConnectorReq(s, "DELETE", "/api/connectors/"+c2.ID, nil)
	if w.Code != 200 {
		t.Fatalf("无引用删除应 200, got %d %s", w.Code, w.Body.String())
	}
}

// kind 校验白名单 + mcp 必填 config.url。
func TestConnectorValidation(t *testing.T) {
	s := newConnectorAPIFixture(t)
	w := doConnectorReq(s, "POST", "/api/connectors", map[string]any{"kind": "grpc", "name": "x"})
	if w.Code != 400 {
		t.Fatal("未知 kind 应 400")
	}
	w = doConnectorReq(s, "POST", "/api/connectors", map[string]any{"kind": "mcp", "name": "x"})
	if w.Code != 400 {
		t.Fatal("mcp 缺 url 应 400")
	}
	w = doConnectorReq(s, "POST", "/api/connectors", map[string]any{"kind": "ssh", "name": "x"})
	if w.Code != 400 {
		t.Fatal("ssh 缺 host 应 400")
	}
}

// test 端点：mcp 不可达 → status=error 回写；列表透出。
func TestConnectorTestWritesStatus(t *testing.T) {
	s := newConnectorAPIFixture(t)
	c, _ := s.Store.CreateConnector(&store.Connector{Kind: store.ConnectorKindMCP, Name: "dead",
		Config: map[string]any{"url": "http://127.0.0.1:1/mcp"}})
	w := doConnectorReq(s, "POST", "/api/connectors/"+c.ID+"/test", nil)
	if w.Code != 200 {
		t.Fatalf("test: %d", w.Code)
	}
	var out struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.OK || out.Status != "error" {
		t.Fatalf("不可达应 error: %+v", out)
	}
	got, _ := s.Store.GetConnector(c.ID)
	if got.Status != "error" {
		t.Fatalf("status 应回写 error, got %s", got.Status)
	}
}

// ---- REQ-214 P2 批次单测 ----

// stubMCPServer 最小 MCP 面（initialize/tools_list），并记录收到的请求头（认证头注入断言用）。
func stubMCPServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var gotAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		body, _ := io.ReadAll(r.Body)
		var msg struct {
			ID    any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &msg)
		w.Header().Set("Content-Type", "application/json")
		var result any
		switch msg.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "stub", "version": "0"}}
		case "notifications/initialized":
			w.WriteHeader(202)
			return
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "echo"}}}
		default:
			result = map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
	}))
	t.Cleanup(srv.Close)
	return srv, &gotAuth
}

// preview 预检：不落库、返回工具清单。
func TestConnectorPreviewDoesNotPersist(t *testing.T) {
	s := newConnectorAPIFixture(t)
	srv, _ := stubMCPServer(t)
	before, _ := s.Store.ListConnectors()
	n0 := len(before)
	w := doConnectorReq(s, "POST", "/api/connectors/preview", map[string]any{
		"kind": "mcp", "name": "preview-x", "config": map[string]any{"url": srv.URL},
	})
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		OK    bool     `json:"ok"`
		Tools []string `json:"tools"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if !out.OK || len(out.Tools) != 1 || !strings.HasSuffix(out.Tools[0], "__echo") {
		t.Fatalf("preview 结果: %+v", out)
	}
	after, _ := s.Store.ListConnectors()
	if len(after) != n0 {
		t.Fatalf("preview 不应落库: %d -> %d", n0, len(after))
	}
}

// test 端点：工具清单 + tested_at 落库。
func TestConnectorTestPersistsTools(t *testing.T) {
	s := newConnectorAPIFixture(t)
	srv, _ := stubMCPServer(t)
	c, _ := s.Store.CreateConnector(&store.Connector{Kind: store.ConnectorKindMCP, Name: "with-tools",
		Config: map[string]any{"url": srv.URL}})
	w := doConnectorReq(s, "POST", "/api/connectors/"+c.ID+"/test", nil)
	var out struct {
		OK    bool `json:"ok"`
		Tools []string `json:"tools"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if !out.OK || len(out.Tools) == 0 {
		t.Fatalf("test 应带工具清单: %+v", out)
	}
	got, _ := s.Store.GetConnector(c.ID)
	if len(got.Tools) == 0 || got.TestedAt == "" {
		t.Fatalf("tools/tested_at 应回写: %+v", got)
	}
}

// 认证头：credentials.headers 加密落库 → preview 探测时注入 Authorization。
func TestConnectorAuthHeadersInjected(t *testing.T) {
	s := newConnectorAPIFixture(t)
	srv, auths := stubMCPServer(t)
	w := doConnectorReq(s, "POST", "/api/connectors/preview", map[string]any{
		"kind": "mcp", "name": "auth-x",
		"config":      map[string]any{"url": srv.URL},
		"credentials": map[string]any{"headers": map[string]any{"Authorization": "Bearer sk-test-123"}},
	})
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	found := false
	for _, a := range *auths {
		if a == "Bearer sk-test-123" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Authorization 头未注入: %v", *auths)
	}
}

// ---- REQ-416/M212：a2a 连接器（agent card 真握手注册）----

// stubA2AAgent 最小 A2A 智能体面（card + message/send + tasks/get）。
func stubA2AAgent(t *testing.T, fail bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail && !strings.HasSuffix(r.URL.Path, "agent-card.json") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.HasSuffix(r.URL.Path, "agent-card.json") {
			if fail {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "外部编码者", "description": "远端写码", "url": "TBD",
				"preferredTransport": "JSONRPC",
				"skills":             []map[string]any{{"id": "s1", "name": "编码"}},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"t-1","contextId":"c-1","kind":"task","status":{"state":"completed"},"artifacts":[{"taskId":"t-1","parts":[{"kind":"text","text":"ok"}]}]}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 注册闸门：卡片可达 = 201+卡片落库（tools=技能名）；不可达 = 400 注册失败。
func TestConnectorA2AHandshakeRegistration(t *testing.T) {
	s := newConnectorAPIFixture(t)
	srv := stubA2AAgent(t, false)
	w := doConnectorReq(s, "POST", "/api/connectors", map[string]any{
		"kind": "a2a", "name": "外部编码者", "config": map[string]any{"url": srv.URL},
	})
	if w.Code != 201 {
		t.Fatalf("a2a 注册应成功: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		ID    string   `json:"id"`
		Card  map[string]any `json:"card"`
		Tools []string `json:"tools"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Card == nil || out.Card["name"] != "外部编码者" {
		t.Fatalf("卡片应随出参透出: %v", out.Card)
	}
	if len(out.Tools) != 1 || out.Tools[0] != "编码" {
		t.Fatalf("tools 应为技能名: %v", out.Tools)
	}
	c, _ := s.Store.GetConnector(out.ID)
	if !strings.Contains(c.CardJSON, "preferredTransport") {
		t.Fatal("卡片原文应落库")
	}
	// 不可达 → 注册失败 400
	w = doConnectorReq(s, "POST", "/api/connectors", map[string]any{
		"kind": "a2a", "name": "死端点", "config": map[string]any{"url": stubA2AAgent(t, true).URL},
	})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "A2A 注册失败") {
		t.Fatalf("卡片不可达应 400: %d %s", w.Code, w.Body.String())
	}
	// 缺 url → 400
	w = doConnectorReq(s, "POST", "/api/connectors", map[string]any{"kind": "a2a", "name": "x"})
	if w.Code != 400 {
		t.Fatal("缺 url 应 400")
	}
	// test 端点：重新握手回写
	w = doConnectorReq(s, "POST", "/api/connectors/"+out.ID+"/test", nil)
	var tr struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tr)
	if !tr.OK {
		t.Fatalf("test 应握手成功: %s", w.Body.String())
	}
}

// ---- REQ-426/M222：kind=cli 连接器——注册闸门（PATH 探测真握手）/校验/能力入库 ----

// stubProbeBackend 探测恒可用的桩后端（注册进独立 Registry 避免真机 PATH 依赖）。
type stubProbeBackend struct{}

func (stubProbeBackend) Name() string { return "stub-cli" }
func (stubProbeBackend) Probe(_ context.Context) inference.ProbeResult {
	return inference.ProbeResult{Available: true, Version: "stub 1.0", Path: "/usr/bin/stub-cli"}
}
func (stubProbeBackend) Capabilities() inference.Capabilities { return inference.Capabilities{Chat: true} }
func (stubProbeBackend) Run(_ context.Context, _ *inference.RunRequest, _ func(inference.Event)) error {
	return nil
}

func newCLIConnectorAPIFixture(t *testing.T) *Server {
	t.Helper()
	s := newConnectorAPIFixture(t)
	reg := inference.NewRegistry()
	reg.Register(stubProbeBackend{})
	s.Chat = &chat.Service{Inference: reg}
	return s
}

func TestConnectorCLIRegistrationGate(t *testing.T) {
	s := newCLIConnectorAPIFixture(t)
	// 缺 backend → 400
	w := doConnectorReq(s, "POST", "/api/connectors", map[string]any{"kind": "cli", "name": "x"})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "config.backend") {
		t.Fatalf("缺 backend 应 400: %d %s", w.Code, w.Body.String())
	}
	// backend=eino-adk → 400（平台内管线不经成员通道）
	w = doConnectorReq(s, "POST", "/api/connectors", map[string]any{"kind": "cli", "name": "x", "config": map[string]any{"backend": "eino-adk"}})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "外部 CLI 型") {
		t.Fatalf("eino-adk 应拒绝: %d %s", w.Code, w.Body.String())
	}
	// 未知 backend → 注册失败 400（不可达注册失败如实报）
	w = doConnectorReq(s, "POST", "/api/connectors", map[string]any{"kind": "cli", "name": "x", "config": map[string]any{"backend": "no-such-cli"}})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "CLI 注册失败") {
		t.Fatalf("未知 backend 应 400: %d %s", w.Code, w.Body.String())
	}
	// 桩后端 → 注册成功，能力探测入库（card=cli-probe 载荷 + status ok）
	w = doConnectorReq(s, "POST", "/api/connectors", map[string]any{
		"kind": "cli", "name": "编码成员", "description": "外部 CLI 编码智能体",
		"config": map[string]any{"backend": "stub-cli", "task_prefix": "只写代码"},
	})
	if w.Code != 201 {
		t.Fatalf("cli 注册应成功: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		ID     string         `json:"id"`
		Status string         `json:"status"`
		Detail string         `json:"status_detail"`
		Card   map[string]any `json:"card"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Status != "ok" || !strings.Contains(out.Detail, "stub 1.0") {
		t.Fatalf("探测成功应回写状态: %v %v", out.Status, out.Detail)
	}
	if out.Card == nil || out.Card["type"] != "cli-probe" || out.Card["backend"] != "stub-cli" || out.Card["version"] != "stub 1.0" {
		t.Fatalf("cli-probe 载荷应透出: %v", out.Card)
	}
	c, _ := s.Store.GetConnector(out.ID)
	if !strings.Contains(c.CardJSON, "cli-probe") {
		t.Fatal("探测原文应落库")
	}
	// test 端点：重新探测刷新状态
	w = doConnectorReq(s, "POST", "/api/connectors/"+out.ID+"/test", nil)
	var tr struct {
		OK     bool   `json:"ok"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tr)
	if !tr.OK || !strings.Contains(tr.Detail, "PATH 探测成功") {
		t.Fatalf("test 应探测成功: %s", w.Body.String())
	}
}

func TestConnectorCLISandboxModeValidation(t *testing.T) {
	s := newCLIConnectorAPIFixture(t)
	// sandbox_mode 非法取值 → 400
	w := doConnectorReq(s, "POST", "/api/connectors", map[string]any{
		"kind": "cli", "name": "x", "config": map[string]any{"backend": "stub-cli", "sandbox_mode": "k8s"},
	})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "sandbox_mode") {
		t.Fatalf("非法 sandbox_mode 应 400: %d %s", w.Code, w.Body.String())
	}
	// sandbox_mode=docker 合法（三前置在装配期闸门，注册不拦）
	w = doConnectorReq(s, "POST", "/api/connectors", map[string]any{
		"kind": "cli", "name": "sandboxed", "config": map[string]any{"backend": "stub-cli", "sandbox_mode": "docker", "cwd": "/tmp/ws", "sandbox_image": "img:dev"},
	})
	if w.Code != 201 {
		t.Fatalf("sandbox_mode=docker 应注册成功: %d %s", w.Code, w.Body.String())
	}
}

// REQ-429/M225：连接器出参透出成员能力画像（cli 矩阵/a2a streaming——连接器面呈现源）。
func TestConnectorProfileInOut(t *testing.T) {
	s := newCLIConnectorAPIFixture(t)
	w := doConnectorReq(s, "POST", "/api/connectors", map[string]any{
		"kind": "cli", "name": "profiled", "config": map[string]any{"backend": "stub-cli"},
	})
	if w.Code != 201 {
		t.Fatalf("注册: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Profile map[string]any `json:"profile"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Profile == nil || out.Profile["channel"] != "cli" || out.Profile["matrix"] == nil {
		t.Fatalf("cli 画像应随出参透出: %v", out.Profile)
	}
	if !strings.Contains(out.Profile["cost"].(string), "对端 CLI 自管") {
		t.Fatalf("成本档: %v", out.Profile["cost"])
	}
	// mcp 连接器无画像
	w = doConnectorReq(s, "POST", "/api/connectors", map[string]any{"kind": "mcp", "name": "plain", "config": map[string]any{"url": "http://127.0.0.1:1/mcp"}})
	var out2 struct {
		Profile map[string]any `json:"profile"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out2)
	if out2.Profile != nil {
		t.Fatalf("mcp 无画像: %v", out2.Profile)
	}
}
