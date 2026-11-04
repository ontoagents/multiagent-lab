package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/ontoagents/ontoagents/backend/internal/store"
	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

func TestHarnessRunCommandDangerRequiresApproval(t *testing.T) {
	t.Setenv("SANDBOX_EXEC_WORLD", "1")
	asm, _, _ := newAssemblerFixture(t)
	dir := t.TempDir()
	tb := asm.assembleOnly(context.Background(), t, &store.Agent{Name: "command", WorkDir: dir, ToolApproval: "danger"})
	for _, bt := range tb.Tools {
		info, err := bt.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name != "run_command" {
			continue
		}
		args, _ := json.Marshal(map[string]string{"command": "printf approved > approval-marker"})
		_, err = bt.(einotool.InvokableTool).InvokableRun(context.Background(), string(args))
		if _, statErr := os.Stat(filepath.Join(dir, "approval-marker")); !os.IsNotExist(statErr) {
			t.Fatalf("command executed before approval: %v", statErr)
		}
		if err == nil || !strings.Contains(err.Error(), "interrupt") {
			t.Fatalf("want approval interrupt, got %v", err)
		}
		return
	}
	t.Fatal("run_command not assembled")
}

type approvalProbe struct{ calls atomic.Int32 }

func (*approvalProbe) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "current_time", Desc: "approval probe"}, nil
}
func (p *approvalProbe) InvokableRun(context.Context, string, ...einotool.Option) (string, error) {
	p.calls.Add(1)
	return "executed", nil
}

type dualApprovalProbe struct{ *approvalProbe }

func (*dualApprovalProbe) StreamableRun(context.Context, string, ...einotool.Option) (*schema.StreamReader[string], error) {
	return nil, fmt.Errorf("unexpected streaming execution")
}

type streamApprovalProbe struct{}

func (*streamApprovalProbe) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "stream__write"}, nil
}
func (*streamApprovalProbe) StreamableRun(context.Context, string, ...einotool.Option) (*schema.StreamReader[string], error) {
	return nil, fmt.Errorf("must not run")
}

type baseApprovalProbe struct{}

func (*baseApprovalProbe) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "base__write"}, nil
}

func TestHarnessApprovalAssemblyRejectsUnsupportedTools(t *testing.T) {
	for _, mode := range []string{"all", "danger"} {
		for _, bt := range []einotool.BaseTool{&streamApprovalProbe{}, &baseApprovalProbe{}} {
			t.Run(fmt.Sprintf("%s/%T", mode, bt), func(t *testing.T) {
				tb := &toolBundle{Tools: []einotool.BaseTool{bt}}
				err := (&Assembler{}).stepApprovalWrap(context.Background(), &store.Agent{ToolApproval: mode}, assembleScope{}, tb)
				if err == nil {
					t.Fatal("unsupported approval tool silently assembled")
				}
			})
		}
	}
}

func TestHarnessHookedDualToolStillRequiresApproval(t *testing.T) {
	probe := &approvalProbe{}
	tb := &toolBundle{Tools: []einotool.BaseTool{&dualApprovalProbe{probe}}}
	asm := &Assembler{}
	ag := &store.Agent{ToolApproval: "all"}
	asm.stepHookWrap(ag, tb)
	if _, ok := tb.Tools[0].(einotool.InvokableTool); !ok {
		t.Fatalf("hooks removed invokable interface: %T", tb.Tools[0])
	}
	if _, ok := tb.Tools[0].(einotool.StreamableTool); !ok {
		t.Fatalf("hooks removed streamable interface: %T", tb.Tools[0])
	}
	if err := asm.stepApprovalWrap(context.Background(), ag, assembleScope{}, tb); err != nil {
		t.Fatal(err)
	}
	asm.stepNormalizeAndGate(context.Background(), ag, tb)
	_, err := tb.Tools[0].(einotool.InvokableTool).InvokableRun(context.Background(), "{}")
	if err == nil || !strings.Contains(err.Error(), "interrupt") || probe.calls.Load() != 0 {
		t.Fatalf("missing approval: calls=%d err=%v", probe.calls.Load(), err)
	}
}

// Use a real checkpoint with an old/legacy pending approval, then rebuild the
// agent with the current policy, as Service.Resume does after configuration edits.
type pendingApprovalProbe struct{ state *tool.ApprovalState }

func (*pendingApprovalProbe) Info(context.Context) (*schema.ToolInfo, error) {
	return (&approvalProbe{}).Info(context.Background())
}
func (p *pendingApprovalProbe) InvokableRun(ctx context.Context, args string, _ ...einotool.Option) (string, error) {
	return "", compose.StatefulInterrupt(ctx, &tool.ApprovalInfo{ToolName: "current_time", Arguments: args}, p.state)
}
func approvalTestRunner(t *testing.T, bt einotool.BaseTool, cp CheckPoints) *adk.Runner {
	t.Helper()
	ctx := context.Background()
	ag, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{Name: "approver", Model: stubApprovalModel{}, ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: []einotool.BaseTool{bt}}}})
	if err != nil {
		t.Fatal(err)
	}
	return adk.NewRunner(ctx, adk.RunnerConfig{Agent: ag, EnableStreaming: false, CheckPointStore: cp})
}
func TestHarnessApprovalResumePolicy(t *testing.T) {
	cases := []struct {
		name, at, decision string
		exempt             bool
		timeout            float64
		wantCalls          int32
		source             string
	}{
		{"expired approve", time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339), tool.ApprovalApprove, false, 1, 0, "timeout"},
		{"expired exempt approve", time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339), tool.ApprovalApprove, true, 1, 0, "timeout"},
		{"new exemption cannot override deny", time.Now().UTC().Format(time.RFC3339), tool.ApprovalDeny, true, 1, 0, "manual"},
		{"new exemption retains approval audit", time.Now().UTC().Format(time.RFC3339), tool.ApprovalApprove, true, 1, 1, "manual"},
		{"unexpired approve", time.Now().UTC().Format(time.RFC3339), tool.ApprovalApprove, false, 1, 1, "manual"},
		{"legacy timestamp", "", tool.ApprovalApprove, false, 1, 1, "manual"},
		{"unparseable timestamp", "legacy", tool.ApprovalApprove, false, 1, 1, "manual"},
		{"timeout disabled", time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339), tool.ApprovalApprove, false, 0, 1, "manual"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			cp := NewMemCheckPointStore()
			seed := &pendingApprovalProbe{&tool.ApprovalState{ToolName: "current_time", Arguments: "{}", SuspendedAt: tc.at}}
			root, _ := consumeEvents(t, approvalTestRunner(t, seed, cp).Run(ctx, []*schema.Message{schema.UserMessage("invoke")}, adk.WithCheckPointID("policy")))
			if root == nil {
				t.Fatal("missing initial interrupt")
			}
			probe := &approvalProbe{}
			policy := &tool.ApprovalPolicy{TimeoutHours: tc.timeout}
			if tc.exempt {
				policy.ExemptTools = []string{"current_time"}
			}
			wrapped, ok := tool.NewApprovalTool(probe, "current_time", policy)
			if !ok {
				t.Fatal("cannot wrap")
			}
			type audit struct{ kind, source string }
			events := make(chan audit, 4)
			ctx = tool.WithEventSink(ctx, func(kind string, data map[string]any) {
				if strings.HasPrefix(kind, "approval.") {
					events <- audit{kind, fmt.Sprint(data["decision_source"])}
				}
			})
			iter, err := approvalTestRunner(t, wrapped, cp).ResumeWithParams(ctx, "policy", &adk.ResumeParams{Targets: map[string]any{root.ID: tc.decision}})
			if err != nil {
				t.Fatal(err)
			}
			next, result := consumeEvents(t, iter)
			if next != nil {
				t.Fatal("unexpected second interrupt")
			}
			if probe.calls.Load() != tc.wantCalls {
				t.Fatalf("executions=%d want %d; output=%s", probe.calls.Load(), tc.wantCalls, result)
			}
			wantKind := "approval.denied"
			if tc.wantCalls == 1 {
				wantKind = "approval.granted"
			}
			select {
			case ev := <-events:
				if ev.kind != wantKind || ev.source != tc.source {
					t.Fatalf("audit=%+v want %s/%s", ev, wantKind, tc.source)
				}
			default:
				t.Fatal("missing approval audit")
			}
			if len(events) != 0 {
				t.Fatal("duplicate approval audit")
			}
		})
	}
}

// Unknown metadata must not turn an all/danger policy into silent execution.
type invalidApprovalProbe struct {
	info *schema.ToolInfo
	err  error
}

func (p invalidApprovalProbe) Info(context.Context) (*schema.ToolInfo, error) { return p.info, p.err }
func TestHarnessApprovalAssemblyRejectsUnknownMetadata(t *testing.T) {
	for _, mode := range []string{"all", "danger"} {
		for _, bt := range []invalidApprovalProbe{{nil, fmt.Errorf("metadata unavailable")}, {nil, nil}, {&schema.ToolInfo{}, nil}} {
			if err := (&Assembler{}).stepApprovalWrap(context.Background(), &store.Agent{ToolApproval: mode}, assembleScope{}, &toolBundle{Tools: []einotool.BaseTool{bt}}); err == nil {
				t.Errorf("%s accepted unknown metadata: %+v", mode, bt)
			}
		}
	}
}

func TestHarnessApprovalFreshExemption(t *testing.T) {
	probe := &approvalProbe{}
	wrapped, _ := tool.NewApprovalTool(probe, "current_time", &tool.ApprovalPolicy{ExemptTools: []string{"current_time"}})
	out, err := wrapped.(einotool.InvokableTool).InvokableRun(context.Background(), "{}")
	if err != nil || out != "executed" || probe.calls.Load() != 1 {
		t.Fatalf("fresh exemption blocked: output=%s calls=%d error=%v", out, probe.calls.Load(), err)
	}
}

func TestHarnessHookedDualToolApprovalResume(t *testing.T) {
	for _, deny := range []bool{false, true} {
		t.Run(fmt.Sprintf("guardDenied=%t", deny), func(t *testing.T) {
			ctx := context.Background()
			probe := &approvalProbe{}
			var pre, post atomic.Int32
			hooks := &tool.HookChain{Pre: []tool.PreHook{{Guard: "probe", Fn: func(name, args string) error {
				pre.Add(1)
				if deny {
					return fmt.Errorf("blocked")
				}
				return nil
			}}}, Post: []tool.PostHook{func(name, result string) { post.Add(1) }}}
			hooked := hooks.Wrap(&dualApprovalProbe{probe})
			wrapped, ok := tool.NewApprovalTool(hooked, "current_time", nil)
			if !ok {
				t.Fatal("cannot wrap dual tool")
			}
			cp := NewMemCheckPointStore()
			runner := approvalTestRunner(t, wrapped, cp)
			root, _ := consumeEvents(t, runner.Run(ctx, []*schema.Message{schema.UserMessage("invoke")}, adk.WithCheckPointID("dual")))
			if root == nil || pre.Load() != 0 || probe.calls.Load() != 0 {
				t.Fatal("execution before approval")
			}
			iter, err := runner.ResumeWithParams(ctx, "dual", &adk.ResumeParams{Targets: map[string]any{root.ID: tool.ApprovalApprove}})
			if err != nil {
				t.Fatal(err)
			}
			next, result := consumeEvents(t, iter)
			if next != nil {
				t.Fatal("unexpected second interrupt")
			}
			want := int32(1)
			if deny {
				want = 0
				if !strings.Contains(result, "blocked") {
					t.Fatalf("missing guard feedback: %s", result)
				}
			}
			if pre.Load() != 1 || post.Load() != want || probe.calls.Load() != want {
				t.Fatalf("guard/calls/post=%d/%d/%d", pre.Load(), probe.calls.Load(), post.Load())
			}
		})
	}
}
