package inference

// REQ-426/M222 单测：CLI 成员执行底座——CLIMemberRun 收集口径（stream-json result 行
// final 标记→最终文本去重；纯文本型全量拼接）+ PathProber 候选名 LookPath。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubCLIBin 临时 stub 可执行：claude-code stream-json 形态（assistant 文本 + result 行）。
func stubCLIBin(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return p
}

func TestCLIMemberRunPrefersResultLine(t *testing.T) {
	stubCLIBin(t, "stubcc", `
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"中间思考文本"}]}}'
echo '{"type":"result","subtype":"success","result":"最终答案 42"}'
`)
	ca := &cliAdapter{name: "stub", bin: []string{"stubcc"}, versionArgs: []string{"--version"},
		buildArgs: func(req *RunRequest) []string { return []string{req.Prompt} }, parseLine: parseClaudeCodeLine}
	out, calls, err := CLIMemberRun(context.Background(), ca, &RunRequest{Prompt: "任务"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "最终答案 42" {
		t.Fatalf("stream-json 型应取 result 行为最终文本（不与 assistant 文本重复拼接）: %q", out)
	}
	if calls != 0 {
		t.Fatalf("无 tool_use 行工具计数应为 0: %d", calls)
	}
}

func TestCLIMemberRunPlainTextAccumulates(t *testing.T) {
	stubCLIBin(t, "stubpt", `
echo "第一行输出"
echo "第二行输出"
`)
	ca := &cliAdapter{name: "stub", bin: []string{"stubpt"}, versionArgs: []string{"--version"},
		buildArgs: func(req *RunRequest) []string { return []string{req.Prompt} }, parseLine: parsePlainLine(nil)}
	out, _, err := CLIMemberRun(context.Background(), ca, &RunRequest{Prompt: "任务"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "第一行输出") || !strings.Contains(out, "第二行输出") {
		t.Fatalf("纯文本型应拼接全部输出: %q", out)
	}
}

func TestCLIMemberRunErrorPropagates(t *testing.T) {
	stubCLIBin(t, "stubfail", `
echo "boom" >&2
exit 3
`)
	ca := &cliAdapter{name: "stub", bin: []string{"stubfail"}, versionArgs: []string{"--version"},
		buildArgs: func(req *RunRequest) []string { return []string{req.Prompt} }, parseLine: parsePlainLine(nil)}
	_, _, err := CLIMemberRun(context.Background(), ca, &RunRequest{Prompt: "任务"})
	if err == nil || !strings.Contains(err.Error(), "退出码 3") {
		t.Fatalf("非零退出码应透传错误: %v", err)
	}
}

func TestCLILookPathCandidates(t *testing.T) {
	stubCLIBin(t, "stubreal", "exit 0")
	ca := &cliAdapter{name: "stub", bin: []string{"no-such-bin-xyz", "stubreal"}}
	if p, err := ca.LookPath(); err != nil || !strings.HasSuffix(p, "stubreal") {
		t.Fatalf("候选名应依次尝试: p=%q err=%v", p, err)
	}
	ca2 := &cliAdapter{name: "stub", bin: []string{"no-such-bin-xyz", "no-such-bin-abc"}}
	if _, err := ca2.LookPath(); err == nil {
		t.Fatal("全部候选缺失应报错")
	}
}
