// REQ-225②/M53 + REQ-232/M59 单测（chat 装配层）：run_command 三前置闸（执行世界/绑定目录/
// 审批档）+ agent 级 hook 链装配（fetchGuard 启停/deny 规则进入包装链）。
package chat

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestAssemblerRunCommandGate(t *testing.T) {
	t.Setenv("SANDBOX_EXEC_WORLD", "1")
	asm, st, _ := newAssemblerFixture(t)
	// 绑定目录夹具平台自适应（REQ-321）：POSIX 形态（/tmp/rc-root）仅在 unix 语义成立——
	// Windows 的 NormalizeDir 会 Clean 成 \tmp\... 致三种绝对形态判定全落空；
	// Windows 用本机临时绝对目录承载同语义
	workDir := "/tmp/rc-root"
	if runtime.GOOS == "windows" {
		workDir = filepath.Join(t.TempDir(), "rc-root")
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ag, err := st.CreateAgent(&store.Agent{Name: "rc-agent", ToolApproval: "all", WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	tb := asm.assembleOnly(context.Background(), t, ag)
	if _, ok := tb.SourceOf["run_command"]; !ok {
		t.Fatalf("三前置全满足应装配 run_command（warnings=%s）", strings.Join(tb.Warnings, "\n"))
	}

	// 绑定目录缺失 → 不装配 + warning
	ag2, _ := st.CreateAgent(&store.Agent{Name: "rc-noworkdir", ToolApproval: "all"})
	tb2 := asm.assembleOnly(context.Background(), t, ag2)
	if _, ok := tb2.SourceOf["run_command"]; ok {
		t.Fatal("无 work_dir 不应装配 run_command")
	}
	if !strings.Contains(strings.Join(tb2.Warnings, "\n"), "绑定目录缺失") {
		t.Fatalf("缺目录应如实 warning: %s", strings.Join(tb2.Warnings, "\n"))
	}

	// 审批档缺失 → 不装配 + warning
	ag3, _ := st.CreateAgent(&store.Agent{Name: "rc-noappr", WorkDir: workDir})
	tb3 := asm.assembleOnly(context.Background(), t, ag3)
	if _, ok := tb3.SourceOf["run_command"]; ok {
		t.Fatal("审批档缺失不应装配 run_command")
	}
	if !strings.Contains(strings.Join(tb3.Warnings, "\n"), "审批档未启用") {
		t.Fatalf("缺审批应如实 warning: %s", strings.Join(tb3.Warnings, "\n"))
	}

	// 世界外（平台主进程）恒不装配——宿主机执行世界禁入
	t.Setenv("SANDBOX_EXEC_WORLD", "")
	tb4 := asm.assembleOnly(context.Background(), t, ag)
	if _, ok := tb4.SourceOf["run_command"]; ok {
		t.Fatal("世界外不应装配 run_command")
	}
}

func TestAssemblerAgentHookChain(t *testing.T) {
	asm, st, _ := newAssemblerFixture(t)
	// fetchGuard 停用 + deny 规则两条（一条无 pattern 应被忽略）
	ag, err := st.CreateAgent(&store.Agent{
		Name: "hooks-agent",
		Hooks: store.AgentHooks{
			FetchGuardDisabled: true,
			DenyRules: []store.AgentHookDenyRule{
				{Name: "no-web", ToolPattern: "web_*", Reason: "禁外网"},
				{Name: "broken", ToolPattern: ""},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	tb := asm.assembleOnly(context.Background(), t, ag)
	// 找一个包了链的工具（http_fetch 常备装配）反查行为不可行（包装不透链）——
	// 以 SourceOf 装配成功 + 无 panic 为闸；链行为断言在 tool 包单测覆盖。
	if _, ok := tb.SourceOf["http_fetch"]; !ok {
		t.Fatalf("http_fetch 应装配（warnings=%s）", strings.Join(tb.Warnings, "\n"))
	}
}
