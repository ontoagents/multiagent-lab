package inference

// REQ-426/M222 单测：CLI 成员执行底座——CLIMemberRun 收集口径（stream-json result 行
// final 标记→最终文本去重；纯文本型全量拼接）+ PathProber 候选名 LookPath。

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeStubExe 平台自适应落 stub 可执行（REQ-321 Windows 沿例，同 runtime/docker_test 口径）：
// 正文脚本原样落盘（unix 直接可执行；Windows 下 bash exec "$@" 链内层解析也按此无扩展名文件
// 走 shebang）；Windows 的 exec.LookPath 按 PATHEXT 解析、无扩展名脚本不可直接执行——外补
// 同名 .cmd 垫片（interpreter 解释执行同一正文）补上 PATH 入口，返回 LookPath 可见路径。
// bash 档 shim 额外前置 Git usr/bin 到子进程 PATH：正文里的外部命令（tr 等）在无 Git PATH
// 环境下也要可解析（gate 环境实证）。
func writeStubExe(t *testing.T, dir, name, interpreter, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		return p
	}
	var shimBody string
	if interpreter == "bash" {
		bash := stubBashPath(t)
		shimBody = stubShimBody(bash, p)
	} else {
		interp, err := exec.LookPath(interpreter)
		if err != nil {
			t.Fatalf("stub 解释器 %q 不可达: %v", interpreter, err)
		}
		shimBody = "@\"" + interp + "\" \"" + p + "\" %*\r\n"
	}
	shim := filepath.Join(dir, name+".cmd")
	if err := os.WriteFile(shim, []byte(shimBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return shim
}

// stubBashPath Windows 下 stub 解释器定位：PATH 里的 bash 常为 WSL 启动器（System32\bash.exe，
// 无发行版时 execvpe(/bin/bash) 报错，gate 环境实证）——拒收后由 git.exe 反推 Git for
// Windows 安装根，再常见安装位置兜底。
func stubBashPath(t *testing.T) string {
	if p, err := exec.LookPath("bash"); err == nil && !stubIsWSLBash(p) {
		return p
	}
	if gp, err := exec.LookPath("git"); err == nil {
		root := filepath.Dir(filepath.Dir(gp)) // …\Git\cmd\git.exe → …\Git
		for _, rel := range []string{`bin\bash.exe`, `usr\bin\bash.exe`} {
			if c := filepath.Join(root, rel); c != gp {
				if _, err := os.Stat(c); err == nil {
					return c
				}
			}
		}
	}
	for _, c := range []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files\Git\usr\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Fatal("stub 解释器 bash 不可达（PATH 仅 WSL 启动器且未找到 Git for Windows）")
	return ""
}

func stubIsWSLBash(p string) bool {
	lp := strings.ToLower(p)
	return strings.Contains(lp, `\system32\`) || strings.Contains(lp, `\windowsapps\`)
}

// stubShimBody .cmd 垫片内容：bash 档先把同安装 usr/bin 前置进子进程 PATH（tr 等外部命令随取）。
func stubShimBody(bash, script string) string {
	var b strings.Builder
	if usrBin := stubUsrBin(bash); usrBin != "" {
		b.WriteString("@set \"PATH=" + usrBin + ";%PATH%\"\r\n")
	}
	b.WriteString("@\"" + bash + "\" \"" + script + "\" %*\r\n")
	return b.String()
}

// stubUsrBin bash 同安装的 POSIX 工具目录（…\Git\usr\bin）；定位不到返回 ""（不前置）。
func stubUsrBin(bash string) string {
	bin := filepath.Dir(bash)
	usrBin := bin
	if strings.EqualFold(filepath.Base(bin), "bin") && !strings.EqualFold(filepath.Base(filepath.Dir(bin)), "usr") {
		usrBin = filepath.Join(filepath.Dir(bin), "usr", "bin")
	}
	if _, err := os.Stat(filepath.Join(usrBin, "tr.exe")); err != nil {
		return ""
	}
	return usrBin
}

// stubCLIBin 临时 stub 可执行：claude-code stream-json 形态（assistant 文本 + result 行）。
func stubCLIBin(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := writeStubExe(t, dir, name, "bash", "#!/bin/sh\n"+body)
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
	if p, err := ca.LookPath(); err != nil || !stubPathHit(p) {
		t.Fatalf("候选名应依次尝试: p=%q err=%v", p, err)
	}
	ca2 := &cliAdapter{name: "stub", bin: []string{"no-such-bin-xyz", "no-such-bin-abc"}}
	if _, err := ca2.LookPath(); err == nil {
		t.Fatal("全部候选缺失应报错")
	}
}

// stubPathHit LookPath 命中判定（平台自适应）：unix 命中即无扩展名原名；Windows 经 PATHEXT
// 命中垫片（.cmd，REQ-321）——同一「stubreal 被 LookPath 命中」语义，仅后缀形态平台差异。
func stubPathHit(p string) bool {
	if runtime.GOOS == "windows" {
		return strings.HasSuffix(p, "stubreal.cmd")
	}
	return strings.HasSuffix(p, "stubreal")
}

// ---- REQ-427/M223：CLIMemberRunInDocker 沙箱执行——argv 构造/绑定目录/凭据 env 注入/解析层复用 ----

// stubDockerBin 仿真 docker CLI：跳过 run --rm/-v/-w/-e 旗标后取镜像与命令并 exec（argv 落 $ARGS_FILE）。
func stubDockerBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
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
	p := writeStubExe(t, dir, "docker", "bash", script)
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
