package tool

// REQ-561/M368 platform_cli 工具单测：help 直行/deny 人话+hook.denied 事件/ro exec（真二进制
// 测试内 go build）/rw StatefulInterrupt 中断信号。事件汇沿 WithEventSink ctx 注入（生产路径同源）。
import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
)

// buildOntoapi 测试内构建 ontoapi（真二进制 e2e——go build 到临时目录）。
func buildOntoapi(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go 不可达（跳过 CLI e2e）")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "ontoapi-test")
	if os.PathSeparator == '\\' {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "github.com/ontoagents/ontoagents/backend/cmd/ontoapi")
	cmd.Dir = "../.." // 仓库 backend 根（module 内）
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("构建 ontoapi 失败: %v\n%s", err, out)
	}
	return bin
}

func withBin(t *testing.T, bin string) einotool.InvokableTool {
	t.Helper()
	old := PlatformCLIBin
	PlatformCLIBin = bin
	t.Cleanup(func() { PlatformCLIBin = old })
	bt, err := NewPlatformCLITool()
	if err != nil {
		t.Fatal(err)
	}
	inv, ok := bt.(einotool.InvokableTool)
	if !ok {
		t.Fatalf("platform_cli 应为 InvokableTool，实际 %T", bt)
	}
	return inv
}

func mkArgs(cmd string, args ...string) string {
	parts := []string{`{"command":"` + cmd + `"`}
	if len(args) > 0 {
		parts = append(parts, `,"args":[`)
		for i, a := range args {
			if i > 0 {
				parts = append(parts, ",")
			}
			parts = append(parts, `"`+strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(a)+`"`)
		}
		parts = append(parts, "]")
	}
	return strings.Join(append(parts, "}"), "")
}

func TestPlatformCLITool_Req561(t *testing.T) {
	bin := buildOntoapi(t)
	inv := withBin(t, bin)

	// ① help 直行（可发现性）
	out, err := inv.InvokableRun(context.Background(), mkArgs("help"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "agent_create") {
		t.Fatalf("help 应列子命令: %s", out)
	}
	// ② deny：工具侧人话拒绝（双闸之一）+ hook.denied 事件
	denyEvt := false
	ctx := WithEventSink(context.Background(), func(t2 string, data map[string]any) {
		if t2 == "hook.denied" {
			denyEvt = true
		}
	})
	out, err = inv.InvokableRun(ctx, mkArgs("conversation_delete", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "删除类") {
		t.Fatalf("deny 应人话: %s", out)
	}
	if !denyEvt {
		t.Fatal("deny 应发 hook.denied 事件")
	}
	// ③ ro：exec 真二进制（无后端——错误人话透传形态；可达路径由 :8094 冒烟承载）
	out, err = inv.InvokableRun(context.Background(), mkArgs("agent_list"))
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("ro 命令应有输出（含后端不可达人话）")
	}
	// ④ rw：StatefulInterrupt 挂起（中断错误信号——沿 connector_approval_test 判定口径）
	_, ierr := inv.InvokableRun(context.Background(), mkArgs("agent_create", `{"name":"x"}`))
	if ierr == nil || !strings.Contains(strings.ToLower(ierr.Error()), "interrupt") {
		t.Fatalf("rw 命令应 StatefulInterrupt 挂起审批: %v", ierr)
	}
	// ⑤ 未知命令 deny 人话
	out, err = inv.InvokableRun(context.Background(), mkArgs("hack_system"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "未知命令") {
		t.Fatalf("未知命令应人话拒绝: %s", out)
	}
	// ⑥ 二进制缺失诚实降级
	PlatformCLIBin = filepath.Join(t.TempDir(), "no-such-ontoapi")
	if _, err := NewPlatformCLITool(); err == nil {
		t.Fatal("二进制缺失应报错（诚实降级不装配）")
	}
}
