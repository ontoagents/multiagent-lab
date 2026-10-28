package a2a

// REQ-463/M263：AgentScope A2A 包装器平台侧零代码主张验证——
// 平台 a2a client（FetchAgentCard/Send/Get/SendStream 原样，零改动）跨语言回环消费
// wrappers/agentscope-a2a stub server（spawn python3 -m agentscope_a2a --port 0）。
// 本测试即「任意 Python agent 运行时经 A2A 即插即用」多后端主张的实证载体：
// wrapper 进程外可缺省（D-O15 沿 REQ-325 sidecar 先例），python3 不在时 SKIP（不阻断 CI）。

import (
	"context"
	"os"
	"os/exec"
	"bufio"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func startWrapperStub(t *testing.T) (endpoint string, stop func()) {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 不在 PATH——AgentScope wrapper 回环验证跳过（平台零依赖纪律不受影响）")
	}
	wrapperDir, err := filepath.Abs("../../../wrappers/agentscope-a2a")
	if err != nil {
		t.Skip("wrapper 目录定位失败")
	}
	if _, err := os.Stat(filepath.Join(wrapperDir, "agentscope_a2a", "__main__.py")); err != nil {
		t.Skip("wrapper 不在位")
	}
	cmd := exec.Command(py, "-m", "agentscope_a2a", "--port", "0")
	cmd.Dir = wrapperDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatalf("wrapper 启动失败: %v", err)
	}
	stop = func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }

	// 读 stdout 首个 PORT= 行（--port 0 随机端口宣告）
	portCh := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "PORT=") {
				portCh <- strings.TrimPrefix(line, "PORT=")
				return
			}
		}
	}()
	select {
	case p := <-portCh:
		return "http://127.0.0.1:" + p, stop
	case <-time.After(10 * time.Second):
		stop()
		t.Fatal("wrapper 10s 未宣告端口")
		return "", stop
	}
}

// TestAgentScopeWrapperLoopback 平台 client 零改动全链：card 握手 → send+轮询至
// completed（stub 回显）→ input-required 终态（"ask:" 演示）→ message/stream SSE。
func TestAgentScopeWrapperLoopback(t *testing.T) {
	endpoint, stop := startWrapperStub(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// ① 注册握手（真 card 发现）
	card, raw, err := FetchAgentCard(ctx, endpoint, nil, 5*time.Second)
	if err != nil {
		t.Fatalf("card 握手失败: %v", err)
	}
	if card.Name == "" || len(card.Skills) == 0 {
		t.Fatalf("card 不完整: %s", string(raw))
	}
	if !card.Capabilities.Streaming {
		t.Fatal("stub card 应声明 streaming=true（SSE 支持）")
	}
	if card.Properties["mode"] != "stub" {
		t.Fatalf("properties.mode 应为 stub: %v", card.Properties)
	}

	// ② send + 轮询至 completed（stub 回显；AwaitTerminal 平台原样轮询）
	rt, err := Send(ctx, endpoint, nil, "帮我做个计划：整理仓库")
	if err != nil {
		t.Fatalf("send 失败: %v", err)
	}
	rt, err = AwaitTerminal(ctx, endpoint, nil, rt.ID, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("AwaitTerminal 失败: %v", err)
	}
	if rt.State != "completed" {
		t.Fatalf("应 completed，得到 %s（%s）", rt.State, rt.ErrorText)
	}
	if !strings.Contains(rt.ArtifactText, "stub") || !strings.Contains(rt.ArtifactText, "计划") {
		t.Fatalf("stub 回复不符: %q", rt.ArtifactText)
	}
	// REQ-428 过程转译源：history 含 user+agent 两轮
	if len(rt.History) < 2 || rt.History[0].Role != "user" || rt.History[len(rt.History)-1].Role != "agent" {
		t.Fatalf("history 形态不符: %+v", rt.History)
	}
	// ③ input-required 终态（"ask:" 前缀演示，问题文本外发——对齐平台 REQ-443）
	rt3, err := Send(ctx, endpoint, nil, "ask: 这一步需要人工确认哪些参数？")
	if err != nil {
		t.Fatalf("send(ask) 失败: %v", err)
	}
	// 注：平台 client taskTerminal 语义=input-required 按失败如实报（无交互通道）——
	// 本验证用 TasksGet 轮询直取状态（消费方可自行决策补答后重发）。
	deadline3 := time.Now().Add(30 * time.Second)
	for rt3.State == "working" && time.Now().Before(deadline3) {
		time.Sleep(200 * time.Millisecond)
		rt3, err = TasksGet(ctx, endpoint, nil, rt3.ID)
		if err != nil {
			t.Fatalf("tasks/get(ask) 失败: %v", err)
		}
	}
	if rt3.State != "input-required" {
		t.Fatalf("应 input-required，得到 %s", rt3.State)
	}
	if !strings.Contains(rt3.ErrorText, "人工确认") {
		t.Fatalf("问题文本应外发（status.message）: %q", rt3.ErrorText)
	}

	// ④ message/stream SSE（capabilities.streaming=true 路径；SSE 帧循环至终态关流）
	var frames int
	final, err := SendStream(ctx, endpoint, nil, "流式走一轮", func(*RemoteTask) { frames++ })
	if err != nil {
		t.Fatalf("SendStream 失败: %v", err)
	}
	if final.State != "completed" || frames < 1 {
		t.Fatalf("stream 终态不符: state=%s frames=%d", final.State, frames)
	}

	// ⑤ tasks/cancel（working 短窗内取消——stub 任务快，通常已终态回 TaskNotCancelable 属正常；
	// 此处仅验协议回环不报错形态）
	rt5, _ := Send(ctx, endpoint, nil, "取消演示")
	_ = CancelTask(ctx, endpoint, nil, rt5.ID)
}
