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

// ---- REQ-427/M223：CLIMemberRunInDocker 沙箱执行——argv 构造/绑定目录/凭据 env 注入/解析层复用 ----

// stubDockerBin 仿真 docker CLI：跳过 run --rm/-v/-w/-e 旗标后取镜像与命令并 exec（argv 落 $ARGS_FILE）。
func stubDockerBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "docker")
	script := `#!/bin/bash
shift
img=""
flags=()
while [ $# -gt 0 ]; do
  case "$1" in
    --rm) flags+=("--rm"); shift;;
    -v|-w) flags+=("$1" "$2"); shift 2;;
    -e) export "$2"; flags+=("$1" "$2"); shift 2;;
    *) img="$1"; shift; { printf 'IMAGE=%s\n' "$img"; printf '%s\n' "${flags[@]}"; printf '%s\n' "$@"; } > "${ARGS_FILE:-/dev/null}"; exec "$@";;
  esac
done
exit 9
`
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return p
}

func TestCLIMemberRunInDocker(t *testing.T) {
	stubCLIBin(t, "stubcc", `
echo '{"type":"result","subtype":"success","result":"沙箱内执行完成 REQ427_ENV='"$REQ427_ENV"'"}'
`)
	argsFile := filepath.Join(t.TempDir(), "argv.txt")
	t.Setenv("ARGS_FILE", argsFile)
	stubDockerBin(t)
	ca := &cliAdapter{name: "stub", bin: []string{"stubcc"}, versionArgs: []string{"--version"},
		buildArgs: func(req *RunRequest) []string { return []string{"-p", req.Prompt, "--output-format", "stream-json", "--verbose"} }, parseLine: parseClaudeCodeLine}
	out, calls, err := CLIMemberRunInDocker(context.Background(), ca,
		&RunRequest{Prompt: "任务"},
		DockerRunSpec{Image: "sandbox-img:dev", Cwd: "/tmp/ws", Env: map[string]string{"REQ427_ENV": "sk-test"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "沙箱内执行完成") || !strings.Contains(out, "sk-test") {
		t.Fatalf("沙箱执行应回传结果且 -e 凭据注入生效: %q", out)
	}
	if calls != 0 {
		t.Fatalf("工具计数: %d", calls)
	}
	argv, _ := os.ReadFile(argsFile)
	argvS := string(argv)
	for _, want := range []string{"IMAGE=sandbox-img:dev", "-v", "/tmp/ws:/workspace", "-w", "/workspace", "-e", "REQ427_ENV=sk-test", "stubcc", "-p", "任务", "--output-format", "stream-json"} {
		if !strings.Contains(argvS, want) {
			t.Fatalf("docker argv 缺 %q: %s", want, argvS)
		}
	}
}

func TestCLIMemberRunInDockerNoCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // 无 docker
	ca := &cliAdapter{name: "stub", bin: []string{"stubcc"}, buildArgs: func(*RunRequest) []string { return nil }}
	if _, _, err := CLIMemberRunInDocker(context.Background(), ca, &RunRequest{}, DockerRunSpec{Image: "x"}); err == nil || !strings.Contains(err.Error(), "docker CLI 不可达") {
		t.Fatalf("docker 缺失应报错: %v", err)
	}
}

// ---- REQ-428/M224：AuditHook 过程转译——宿主/沙箱两执行世界事件序列 ----

func TestCLIMemberRunWithAuditSequence(t *testing.T) {
	stubCLIBin(t, "stubcc", `
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"第一步说明"}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"read_file","input":{"path":"a.go"}}]}}'
echo '{"type":"result","subtype":"success","result":"最终答案"}'
`)
	ca := &cliAdapter{name: "stub", bin: []string{"stubcc"}, versionArgs: []string{"--version"},
		buildArgs: func(req *RunRequest) []string { return []string{req.Prompt} }, parseLine: parseClaudeCodeLine}
	var seq []string
	out, _, err := CLIMemberRunWithAudit(context.Background(), ca, &RunRequest{Prompt: "任务"}, func(protoEvent, summary string) {
		seq = append(seq, protoEvent+"|"+summary)
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "最终答案" {
		t.Fatalf("结果文本: %q", out)
	}
	want := []string{"text|第一步说明", "tool_use|read_file {\"path\":\"a.go\"}", "result|最终答案"}
	if len(seq) != len(want) {
		t.Fatalf("事件序列长度: %v", seq)
	}
	for i, w := range want {
		if seq[i] != w {
			t.Fatalf("事件[%d]=%q want %q", i, seq[i], w)
		}
	}
}

func TestCLIMemberRunInDockerAuditHook(t *testing.T) {
	stubCLIBin(t, "stubcc", `
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"沙箱内过程"}]}}'
`)
	stubDockerBin(t)
	ca := &cliAdapter{name: "stub", bin: []string{"stubcc"}, versionArgs: []string{"--version"},
		buildArgs: func(req *RunRequest) []string { return []string{req.Prompt} }, parseLine: parseClaudeCodeLine}
	var got []string
	_, _, err := CLIMemberRunInDocker(context.Background(), ca, &RunRequest{Prompt: "任务"},
		DockerRunSpec{Image: "img:dev", Hook: func(protoEvent, summary string) { got = append(got, protoEvent+"|"+summary) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "text|沙箱内过程" {
		t.Fatalf("沙箱 hook 转译: %v", got)
	}
}
