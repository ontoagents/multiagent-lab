// REQ-206/M41 S2 真机面单测（chat 侧）：外部后端运行编排的 ACP 接线——
// 会话延续（AttemptResume/BuildPrompt 历史去重）、MCP 引用解析（预检过滤/防线拒绝/插件端点）、
// 权限转审批（policy 映射 / approval.request 挂起 / ResolveExtApproval 决议 / 超时拒绝）。
package chat

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/connector"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/inference"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// fakeACPBackend 捕获 RunRequest 的桩后端：onRun 非空时由 onRun 自主驱动（含权限回调），
// 否则 Run 默认触发一次权限回调。
type fakeACPBackend struct {
	onRun func(req *inference.RunRequest)
}

func (f *fakeACPBackend) Name() string { return "fake-acp" }
func (f *fakeACPBackend) Probe(context.Context) inference.ProbeResult {
	return inference.ProbeResult{Available: true}
}
func (f *fakeACPBackend) Capabilities() inference.Capabilities {
	return inference.Capabilities{Chat: true, Stream: true, SkillsMode: "instruction", MCPMode: "tools", Resume: true}
}
func (f *fakeACPBackend) Run(ctx context.Context, req *inference.RunRequest, emit func(inference.Event)) error {
	if f.onRun != nil {
		f.onRun(req)
		return nil
	}
	if req.Permission != nil {
		dec := req.Permission(inference.PermissionRequest{
			ToolCallID: "tc1", Title: "write_file", Kind: "edit",
			Options: []inference.PermissionOption{
				{OptionID: "allow1", Name: "Allow once", Kind: "allow_once"},
				{OptionID: "reject1", Name: "Reject", Kind: "reject_once"},
			},
		})
		emit(inference.Event{Type: "debug.cli", Data: map[string]any{"permOutcome": dec.Outcome, "permOption": dec.OptionID}})
	}
	return nil
}

// extTestService 测试用 Service（真 store + 夹具 Assembler + 注入后端的注册表）。
func extTestService(t *testing.T, onRun func(req *inference.RunRequest)) (*Service, *store.Store) {
	t.Helper()
	asm, st, _ := newAssemblerFixture(t)
	s := NewService(st, asm, nil)
	s.Inference = inference.NewRegistry()
	s.Inference.Register(&fakeACPBackend{onRun: onRun})
	return s, st
}

// waitExtPending 轮询等待外部审批挂起登记出现（waitExtApproval 先登记再发事件再阻塞）。
func waitExtPending(t *testing.T, s *Service, convID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		pending := s.extApprovals[convID] != nil
		s.mu.Unlock()
		if pending {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("审批挂起未登记")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRunExternalACPWiring 会话延续接线：有历史 → AttemptResume + BuildPrompt(true) 历史去重；
// 首轮 → 不延续。
func TestRunExternalACPWiring(t *testing.T) {
	var mu sync.Mutex
	var captured *inference.RunRequest
	s, st := extTestService(t, func(req *inference.RunRequest) {
		mu.Lock()
		captured = &inference.RunRequest{SessionID: req.SessionID, AttemptResume: req.AttemptResume, BuildPrompt: req.BuildPrompt, Prompt: req.Prompt}
		mu.Unlock()
	})
	ag, err := st.CreateAgent(&store.Agent{Name: "acp-agent", InferenceBackend: "fake-acp"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Title: "acp-wiring", Scope: "agent", AgentID: &ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	// 预置一轮历史（非首态）
	if _, err := st.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: "上轮问题"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "assistant", Content: "上轮回答"}); err != nil {
		t.Fatal(err)
	}
	agent := ag

	if _, err := s.runExternal(context.Background(), conv, agent, "run-1", "本轮输入", 0, func(ev *Event) {}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	cp := captured
	mu.Unlock()
	if cp == nil {
		t.Fatal("后端未被调用")
	}
	if !cp.AttemptResume {
		t.Fatalf("有历史时应尝试会话延续")
	}
	if cp.BuildPrompt == nil {
		t.Fatal("Resume 能力后端应装配 BuildPrompt 回调")
	}
	if p := cp.BuildPrompt(true); !strings.Contains(p, "## 用户输入") || strings.Contains(p, "对话历史") {
		t.Fatalf("resumed=true 应历史去重: %q", p)
	}
	if p := cp.BuildPrompt(false); !strings.Contains(p, "对话历史") {
		t.Fatalf("resumed=false 应拼全量历史: %q", p)
	}
	if cp.SessionID != conv.ID {
		t.Fatalf("SessionID 应映射平台会话 ID: %q", cp.SessionID)
	}

	// 首轮（无历史）：AttemptResume=false，BuildPrompt(false) 拼全量历史。
	conv2, err := st.CreateConversation(&store.Conversation{Title: "acp-wiring-2", Scope: "agent", AgentID: &ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	captured = nil
	mu.Unlock()
	if _, err := s.runExternal(context.Background(), conv2, agent, "run-2", "首轮输入", 0, func(ev *Event) {}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	cp = captured
	mu.Unlock()
	if cp.AttemptResume {
		t.Fatalf("首轮不应尝试会话延续")
	}
	// 首轮（conv2 无前轮）：不延续；BuildPrompt(false) 无历史段（本就无前轮）
	if cp.AttemptResume {
		t.Fatalf("首轮不应尝试会话延续")
	}
	if p := cp.BuildPrompt(false); strings.Contains(p, "对话历史") {
		t.Fatalf("无前轮的提示不应有历史段: %q", p)
	}
}

// TestRunExternalACPPolicyApproval 审批 all 档：权限回调 → approval.request 挂起 → 决议放行。
func TestRunExternalACPPolicyApproval(t *testing.T) {
	approved := make(chan string, 1)
	s, st := extTestService(t, func(req *inference.RunRequest) {
		if req.Permission == nil {
			t.Error("审批档下应有权限回调")
			return
		}
		dec := req.Permission(inference.PermissionRequest{
			ToolCallID: "tc1", Title: "write_file", Kind: "edit",
			Options: []inference.PermissionOption{{OptionID: "allow1", Name: "Allow once", Kind: "allow_once"}},
		})
		approved <- dec.Outcome + ":" + dec.OptionID
	})
	ag, err := st.CreateAgent(&store.Agent{Name: "acp-agent-appr", InferenceBackend: "fake-acp", ToolApproval: "all"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Title: "acp-appr", Scope: "agent", AgentID: &ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	agent := ag

	done := make(chan error, 1)
	go func() {
		_, err := s.runExternal(context.Background(), conv, agent, "run-appr", "触发权限", 0, func(ev *Event) {})
		done <- err
	}()
	waitExtPending(t, s, conv.ID)
	if err := s.ResolveExtApproval(conv.ID, "approve"); err != nil {
		t.Fatalf("决议失败: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-approved:
		if v != "selected:allow1" {
			t.Fatalf("approve 应映射首个 allow 选项: %q", v)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("权限回调未完成")
	}
	// 决议后挂起登记应清理；再次决议 → 无待决错误
	s.mu.Lock()
	pending := s.extApprovals[conv.ID] != nil
	s.mu.Unlock()
	if pending {
		t.Fatal("决议后挂起登记未清理")
	}
	if err := s.ResolveExtApproval(conv.ID, "deny"); err == nil {
		t.Fatal("无待决时应返回 ErrNoPendingExtApproval")
	}
}

// TestWaitExtApprovalDenyAndTimeout 拒绝决议与挂起超时两分支。
func TestWaitExtApprovalDenyAndTimeout(t *testing.T) {
	s, st := extTestService(t, nil)
	ag, err := st.CreateAgent(&store.Agent{Name: "acp-agent-deny"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Title: "acp-deny", Scope: "agent", AgentID: &ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	agent := ag
	agent.ToolApproval = "all"
	pr := inference.PermissionRequest{ToolCallID: "tc1", Title: "bash", Options: []inference.PermissionOption{
		{OptionID: "allow1", Kind: "allow_once"}, {OptionID: "reject1", Kind: "reject_once"},
	}}
	emit := func(ev *Event) {}

	// deny → cancelled
	resCh := make(chan inference.PermissionDecision, 1)
	go func() {
		resCh <- s.waitExtApproval(context.Background(), conv, agent, "r", pr, emit)
	}()
	waitExtPending(t, s, conv.ID)
	if err := s.ResolveExtApproval(conv.ID, "deny"); err != nil {
		t.Fatalf("deny 决议失败: %v", err)
	}
	if d := <-resCh; d.Outcome != "cancelled" {
		t.Fatalf("deny 应 cancelled: %+v", d)
	}

	// 超时 → cancelled（收缩预算 30ms）
	agent.ApprovalTimeoutHours = 30.0 / 3.6e6
	resCh2 := make(chan inference.PermissionDecision, 1)
	go func() {
		resCh2 <- s.waitExtApproval(context.Background(), conv, agent, "r2", pr, emit)
	}()
	select {
	case d := <-resCh2:
		if d.Outcome != "cancelled" {
			t.Fatalf("超时应 cancelled: %+v", d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("超时分支未按时返回")
	}
}

// TestExternalMCPRefs MCP 引用解析：不可达预检过滤 / 防线拒绝 / 插件端点构造（认证头解密
// 复用 mcpHeaders 装配链路径，由 REQ-214 P2 既有测试覆盖）。
func TestExternalMCPRefs(t *testing.T) {
	s, st := extTestService(t, nil)
	asm := s.Assembler
	emit := func(ev *Event) {}

	// 1) mcp 直通不可达 → 预检剔除（不注入，避免 dsh session/new 整体回滚）
	bad, err := st.CreateConnector(&store.Connector{Kind: store.ConnectorKindMCP, Name: "bad-mcp", Config: map[string]any{"url": "http://127.0.0.1:1/mcp"}})
	if err != nil {
		t.Fatal(err)
	}
	// 2) mcp 直通指向插件服务（绕过授权防线）→ 拒绝
	asm.PluginEndpoint = "http://plugin.internal"
	guarded, err := st.CreateConnector(&store.Connector{Kind: store.ConnectorKindMCP, Name: "guarded", Config: map[string]any{"url": "http://plugin.internal/connectors/x/mcp"}})
	if err != nil {
		t.Fatal(err)
	}
	refs := s.externalMCPRefs(context.Background(), &store.Conversation{ID: "c1"}, &store.Agent{Connectors: []string{bad.ID, guarded.ID}}, "r", emit)
	if len(refs) != 0 {
		t.Fatalf("不可达/防线拒绝的引用不应注入: %+v", refs)
	}

	// 3) kubernetes 连接器 + 真插件服务（stub kubectl）→ 构造 {pluginBase}/connectors/{id}/mcp 注入
	asm.PluginEndpoint = ""
	plugin := connector.NewPlugin(st, asm.Box, stubKubectlForAssembler(t))
	srv := httptest.NewServer(plugin.Handler())
	defer srv.Close()
	enc, err := asm.Box.Encrypt(`{"kubeconfig":"apiVersion: v1"}`)
	if err != nil {
		t.Fatal(err)
	}
	k8s, err := st.CreateConnector(&store.Connector{Kind: store.ConnectorKindKubernetes, Name: "k8s-ops", Config: map[string]any{"namespace": "ops"}, CredentialsEncrypted: enc})
	if err != nil {
		t.Fatal(err)
	}
	asm.PluginEndpoint = srv.URL
	refs = s.externalMCPRefs(context.Background(), &store.Conversation{ID: "c1"}, &store.Agent{Connectors: []string{k8s.ID}}, "r", emit)
	if len(refs) != 1 || refs[0].URL != srv.URL+"/connectors/"+k8s.ID+"/mcp" || refs[0].Name != "k8s-ops" {
		t.Fatalf("插件端点引用构造不符: %+v", refs)
	}
	_ = secrets.Box{} // 保持 import 对齐（secrets 经 newAssemblerFixture 间接使用）
}
