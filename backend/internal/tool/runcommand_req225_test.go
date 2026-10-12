// REQ-225②/M53 + REQ-232/M59 单测（tool 层）：run_command 执行治理（workdir/cmdGuard 双保险/
// 退出码/超时/截断/世界闸）+ agent 级 hook 链（fetchGuard 启停/自定义 deny 规则 glob+参数子串）。
package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
)

func TestRunCommandToolGateAndExec(t *testing.T) {
	// 世界外不可实例化（双保险）
	t.Setenv("SANDBOX_EXEC_WORLD", "")
	if _, err := NewRunCommandTool(RunCommandDeps{}); err == nil {
		t.Fatal("世界外实例化应被拒")
	}
	t.Setenv("SANDBOX_EXEC_WORLD", "1")
	wd := t.TempDir()
	bt, err := NewRunCommandTool(RunCommandDeps{WorkDir: wd, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	it, ok := bt.(einotool.InvokableTool)
	if !ok {
		t.Fatalf("应为 Invokable 形态，实际 %T", bt)
	}
	ctx := context.Background()

	// 正常执行 + workdir 生效
	args, _ := json.Marshal(map[string]string{"command": "pwd"})
	out, err := it.InvokableRun(ctx, string(args))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, wd) || !strings.Contains(out, "\"exit_code\":0") {
		t.Fatalf("pwd 应在 workdir 执行: %s", out)
	}

	// cmdGuard 拒绝（工具内双保险）
	args, _ = json.Marshal(map[string]string{"command": "rm -rf /"})
	out, _ = it.InvokableRun(ctx, string(args))
	if !strings.Contains(out, "denied") || !strings.Contains(out, "cmdGuard") {
		t.Fatalf("危险命令应被 cmdGuard 拒绝: %s", out)
	}

	// 非零退出码透传
	args, _ = json.Marshal(map[string]string{"command": "exit 3"})
	out, _ = it.InvokableRun(ctx, string(args))
	if !strings.Contains(out, "\"exit_code\":3") {
		t.Fatalf("非零退出码应透传: %s", out)
	}

	// 超时
	bt2, _ := NewRunCommandTool(RunCommandDeps{WorkDir: wd, Timeout: 300 * time.Millisecond})
	it2 := bt2.(einotool.InvokableTool)
	args, _ = json.Marshal(map[string]string{"command": "sleep 3"})
	out, _ = it2.InvokableRun(ctx, string(args))
	if !strings.Contains(out, "exit_code\":124") && !strings.Contains(out, "超时") {
		t.Fatalf("超时应标记 124 或超时注记: %s", out)
	}

	// 输出截断
	bt3, _ := NewRunCommandTool(RunCommandDeps{WorkDir: wd, Timeout: 5 * time.Second, MaxOutput: 10})
	it3 := bt3.(einotool.InvokableTool)
	args, _ = json.Marshal(map[string]string{"command": "yes 0123456789 | head -50"})
	out, _ = it3.InvokableRun(ctx, string(args))
	if !strings.Contains(out, "输出截断") {
		t.Fatalf("超长输出应截断: %s", out)
	}
}

func TestBuildHookChainAgentConfig(t *testing.T) {
	ctx := context.Background()

	// 默认链：fetchGuard+cmdGuard 都在
	c := BuildHookChain(nil)
	if len(c.Pre) != 2 {
		t.Fatalf("默认链应含 fetchGuard+cmdGuard，实得 %d", len(c.Pre))
	}

	// fetchGuard 停用（REQ-232①）：链中无 fetchGuard
	c = BuildHookChain(&AgentHookConfig{FetchGuardDisabled: true})
	for _, p := range c.Pre {
		if p.Guard == "fetchGuard" {
			t.Fatal("fetchGuardDisabled=true 时链中不应有 fetchGuard")
		}
	}

	// 自定义 deny 规则（REQ-232②）：glob 命中+参数子串命中才拒；不命中放行
	c = BuildHookChain(&AgentHookConfig{DenyRules: []AgentHookDenyRule{
		{Name: "no-web", ToolPattern: "web_*", ArgsPattern: "secret", Reason: "禁访外网敏感参数"},
	}})
	// web_fetch + 参数命中 → 拒
	if err := c.Pre[len(c.Pre)-1].Fn("web_fetch", `{"url":"https://a?x=secret"}`); err == nil {
		t.Fatal("glob+参数命中应拒绝")
	}
	// 工具名不命中 → 放行
	if err := c.Pre[len(c.Pre)-1].Fn("http_fetch", `{"url":"https://a?x=secret"}`); err != nil {
		t.Fatalf("工具名不命中应放行: %v", err)
	}
	// 工具名命中但参数不含子串 → 放行
	if err := c.Pre[len(c.Pre)-1].Fn("web_search", `{"q":"普通问题"}`); err != nil {
		t.Fatalf("参数子串不命中应放行: %v", err)
	}

	// cmdGuard pre-hook：run_command 危险命令拒绝；其余工具直通
	c = BuildHookChain(nil)
	var cmdGuard PreHook
	for _, p := range c.Pre {
		if p.Guard == "cmdGuard" {
			cmdGuard = p
		}
	}
	if cmdGuard.Fn == nil {
		t.Fatal("默认链应含 cmdGuard")
	}
	if err := cmdGuard.Fn("run_command", `{"command":"shutdown now"}`); err == nil {
		t.Fatal("run_command 危险命令应被 cmdGuard 拒绝")
	}
	if err := cmdGuard.Fn("http_fetch", `{"url":"http://x"}`); err != nil {
		t.Fatalf("非 run_command 工具应直通: %v", err)
	}
	_ = ctx
}
