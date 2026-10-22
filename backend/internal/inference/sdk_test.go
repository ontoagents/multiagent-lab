// S3 单测：stub SDK runtime（Node 按协议帧形态应答）驱动 streamingProc + sdkAdapter。
// 帧形态以本机 @deepseek-ai/dsh-sdk-protocol 0.1.5-rc.3 为准：initialize{cwd,provider,model}
// →{serverInfo}、session/prompt{sessionId,contentBlocks}→{messageId} 入队回执、事件封套
// {type,seq,time,data}、shutdown→{}。覆盖：全链事件转译（tool.call/tool.result callId 回填/
// message.delta 文本块/其余 external.audit 审计）、turn/end 结算+静默窗口收尾帧+shutdown、
// session.status idle 兜底（prompt 已入队才生效）、initialize 错误、审计载荷截断、
// ctx 取消关闭、req.Env 透传、dsh 缺席降级可懂。
package inference

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stub 回显协议：initialize/prompt/shutdown 请求原文按行落 $SDK_STUB_LOG（JSONL），
// 事件序列按 STUB_MODE 分支。
const stubSDK = `#!/usr/bin/env node
// stub SDK runtime：dsh --profile sdk 形态（0.1.5-rc.3 帧直译）
const fs = require('fs')
const mode = process.env.STUB_MODE || ''
const log = process.env.SDK_STUB_LOG
let buf = ''
process.stdin.on('data', (d) => {
  buf += d
  let i
  while ((i = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, i).trim()
    buf = buf.slice(i + 1)
    if (!line) continue
    let f
    try { f = JSON.parse(line) } catch { continue }
    if (log) { try { fs.appendFileSync(log, JSON.stringify({ method: f.method, params: f.params }) + '\n') } catch {} }
    if (f.id != null && f.method === 'initialize') {
      if (mode === 'init_error') {
        send({ jsonrpc: '2.0', id: f.id, error: { message: 'no adapter registered for provider "x"' } })
      } else {
        send({ jsonrpc: '2.0', id: f.id, result: { serverInfo: { name: 'deepseek-harness-sdk-runtime', version: '0.0.1' } } })
      }
    } else if (f.id != null && f.method === 'session/prompt') {
      send({ jsonrpc: '2.0', id: f.id, result: { messageId: 'm-1' } })
      setTimeout(() => emitTurn(f), 30)
    } else if (f.id != null && f.method === 'shutdown') {
      send({ jsonrpc: '2.0', id: f.id, result: {} })
      setTimeout(() => process.exit(0), 20)
    }
  }
})
function emitTurn(promptFrame) {
  const ev = (type, data, seq) => send({ jsonrpc: '2.0', method: 'session.event', params: { sessionId: promptFrame.params.sessionId, event: { type, seq, time: Date.now(), data } } })
  ev('turn/start', { turn: 1 }, 1)
  ev('step/start', { turn: 1, step: 1 }, 2)
  ev('tool/call', { turn: 1, step: 1, callId: 'c1', name: 'read', arguments: '{"file":"a.txt"}' }, 3)
  ev('tool/result', { turn: 1, step: 1, message: { id: 'tr1', role: 'user', content: [{ type: 'tool-result', content: [{ type: 'text', text: '3 lines' }] }], source: { kind: 'tool', callId: 'c1' } } }, 4)
  ev('assistant/message', { turn: 1, step: 1, message: { id: 'a1', role: 'assistant', content: [{ type: 'text', text: 'done:' + (process.env.STUB_TAG || 'ok') }] }, stream: [] }, 5)
  if (mode === 'huge') {
    const bigTool = { name: 'x', schema: 'y'.repeat(40 * 1024) }
    ev('request/header', { header: { config: { provider: 'p', model: 'm' }, tools: [bigTool] }, reason: 'initial' }, 6)
  }
  if (mode === 'idle_only') {
    send({ jsonrpc: '2.0', method: 'session.status', params: { sessionId: promptFrame.params.sessionId, status: 'idle' } })
    return
  }
  if (mode === 'no_end') return // 永不结算（ctx 取消路径）
  ev('turn/end', { turn: 1, reason: 'completed' }, 7)
  // 尾随帧：静默窗口内到达（title/血缘审计）
  ev('session/title', { title: 't', source: 'fallback' }, 8)
  send({ jsonrpc: '2.0', method: 'subagent.finished', params: { provider: 'spawn-in-process', agentId: 'a', parentSessionId: 'p', childSessionId: 'c', status: 'ok', stopReason: 'completed' } })
  send({ jsonrpc: '2.0', method: 'session.status', params: { sessionId: promptFrame.params.sessionId, status: 'idle' } })
}
function send(o) { process.stdout.write(JSON.stringify(o) + '\n') }
process.stdin.on('end', () => process.exit(0))
`

// stubSDKOnPath 把 stub 写入临时目录并注入 PATH，返回日志文件路径。
func stubSDKOnPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "dsh")
	if err := os.WriteFile(p, []byte(stubSDK), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	logPath := filepath.Join(dir, "reqs.jsonl")
	t.Setenv("SDK_STUB_LOG", logPath) // stub 经继承 env 读取；req.Env 透传断言另行覆盖
	return logPath
}

// runStubSDK 执行一次 Run 并回收事件与累计 message.delta 文本。
func runStubSDK(t *testing.T, req *RunRequest) ([]Event, string) {
	t.Helper()
	a := newSDKAdapter()
	var evs []Event
	var buf strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := a.Run(ctx, req, func(e Event) {
		evs = append(evs, e)
		if e.Type == "message.delta" {
			if d, ok := e.Data["delta"].(string); ok {
				buf.WriteString(d)
			}
		}
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return evs, buf.String()
}

func readStubLog(t *testing.T, path string) []map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 stub 日志: %v", err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func TestSDKAdapterRunEndToEnd(t *testing.T) {
	logPath := stubSDKOnPath(t)
	evs, text := runStubSDK(t, &RunRequest{
		Prompt:    "## 用户输入\n测试提示词",
		SessionID: "conv-s3",
		Cwd:       "/tmp",
	})
	if !strings.Contains(text, "done:ok") {
		t.Fatalf("assistant/message 文本块未转译 message.delta: %q", text)
	}
	var toolName, toolArgs, toolResult string
	audits := map[string]bool{}
	for _, e := range evs {
		switch e.Type {
		case "tool.call":
			toolName, _ = e.Data["name"].(string)
			if m, ok := e.Data["args"].(map[string]any); ok {
				b, _ := json.Marshal(m)
				toolArgs = string(b)
			}
		case "tool.result":
			toolResult, _ = e.Data["content"].(string)
		case "external.audit":
			if s, _ := e.Data["event"].(string); s != "" {
				audits[s] = true
			}
		}
	}
	if toolName != "read" || !strings.Contains(toolArgs, "a.txt") {
		t.Fatalf("tool.call 转译不符: name=%q args=%q", toolName, toolArgs)
	}
	if toolResult != "3 lines" {
		t.Fatalf("tool.result 内容不符: %q", toolResult)
	}
	// result 帧不带 name——须由 callId 回填 tool.call 记下的工具名
	for _, e := range evs {
		if e.Type == "tool.result" {
			if n, _ := e.Data["name"].(string); n != "read" {
				t.Fatalf("tool.result 工具名 callId 回填失败: %v", e.Data["name"])
			}
		}
	}
	// 审计转发：边界/请求面/尾随帧全量在案
	for _, want := range []string{"turn/start", "step/start", "turn/end", "session/title"} {
		if !audits[want] {
			t.Fatalf("审计事件缺 %s：实得 %v", want, audits)
		}
	}
	// subagent.finished 走通知级审计
	if !audits["subagent.finished"] {
		t.Fatalf("subagent.finished 血缘审计缺失: %v", audits)
	}
	// stub 收到的请求帧形态：initialize 必填双值 / prompt contentBlocks / shutdown
	reqs := readStubLog(t, logPath)
	var sawInit, sawPrompt, sawShutdown bool
	for _, r := range reqs {
		switch r["method"] {
		case "initialize":
			sawInit = true
			p, _ := r["params"].(map[string]any)
			if p["provider"] != sdkProviderRoute || p["model"] != sdkModelRoute {
				t.Fatalf("initialize 必填路由双值不符: %v", p)
			}
			if p["cwd"] != "/tmp" {
				t.Fatalf("initialize cwd 不符: %v", p)
			}
		case "session/prompt":
			sawPrompt = true
			p, _ := r["params"].(map[string]any)
			sid, _ := p["sessionId"].(string)
			if !strings.HasPrefix(sid, "plt-conv-s3-") {
				t.Fatalf("sessionId 平台命名前缀不符: %q", sid)
			}
			blocks, _ := p["contentBlocks"].([]any)
			if len(blocks) != 1 {
				t.Fatalf("contentBlocks 形态不符: %v", p)
			}
		case "shutdown":
			sawShutdown = true
		}
	}
	if !sawInit || !sawPrompt || !sawShutdown {
		t.Fatalf("stub 未收齐全链请求: init=%v prompt=%v shutdown=%v", sawInit, sawPrompt, sawShutdown)
	}
}

func TestSDKAdapterIdleFallbackAfterPrompt(t *testing.T) {
	stubSDKOnPath(t)
	t.Setenv("STUB_MODE", "idle_only")
	// 无 turn/end，仅 session.status idle（prompt 已入队）→ 兜底结算，Run 正常返回
	evs, text := runStubSDK(t, &RunRequest{Prompt: "x", SessionID: "conv-idle"})
	if !strings.Contains(text, "done:ok") {
		t.Fatalf("idle 兜底路径文本缺失: %q", text)
	}
	_ = evs
}

func TestSDKAdapterInitializeError(t *testing.T) {
	stubSDKOnPath(t)
	t.Setenv("STUB_MODE", "init_error")
	a := newSDKAdapter()
	err := a.Run(context.Background(), &RunRequest{Prompt: "x"}, func(Event) {})
	if err == nil || !strings.Contains(err.Error(), "SDK initialize 失败") {
		t.Fatalf("initialize 错误未透出: %v", err)
	}
}

func TestSDKAdapterAuditPayloadCap(t *testing.T) {
	stubSDKOnPath(t)
	t.Setenv("STUB_MODE", "huge")
	evs, _ := runStubSDK(t, &RunRequest{Prompt: "x", SessionID: "conv-huge"})
	capped := false
	for _, e := range evs {
		if e.Type != "external.audit" {
			continue
		}
		if p, _ := e.Data["payload"].(string); strings.Contains(p, "截断") {
			capped = true
			if len(p) > sdkAuditCap+200 {
				t.Fatalf("审计载荷超上限: %d 字节", len(p))
			}
		}
	}
	if !capped {
		t.Fatalf("超大 request/header 未截断")
	}
}

func TestSDKAdapterCtxCancel(t *testing.T) {
	stubSDKOnPath(t)
	t.Setenv("STUB_MODE", "no_end")
	a := newSDKAdapter()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.Run(ctx, &RunRequest{Prompt: "x", SessionID: "conv-cancel"}, func(Event) {})
	}()
	time.Sleep(400 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("取消后 Run 应返回 ctx 错误")
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("取消后 Run 未在 10s 内返回（关闭阶梯挂死）")
	}
}

func TestSDKAdapterReqEnvPassthrough(t *testing.T) {
	stubSDKOnPath(t)
	// req.Env 注入 stub 进程：STUB_TAG 经 env 带回 assistant 文本，验证透传链
	_, text := runStubSDK(t, &RunRequest{Prompt: "x", SessionID: "conv-env", Env: map[string]string{"STUB_TAG": "envok"}})
	if !strings.Contains(text, "done:envok") {
		t.Fatalf("req.Env 未透传到子进程: %q", text)
	}
}

func TestSDKAdapterUnavailableWhenNoDSH(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // 空 PATH：dsh 不可达
	a := newSDKAdapter()
	if pr := a.Probe(context.Background()); pr.Available {
		t.Fatalf("dsh 缺席时应不可用")
	}
	if err := a.Run(context.Background(), &RunRequest{Prompt: "x"}, func(Event) {}); err == nil {
		t.Fatalf("dsh 缺席时 Run 应报错（全通道降级可懂）")
	}
}

func TestSDKAdapterRegistered(t *testing.T) {
	r := NewRegistry()
	if r.Get("deepseek-harness-sdk") == nil {
		t.Fatalf("deepseek-harness-sdk 未注册进 Registry")
	}
	caps := newSDKAdapter().Capabilities()
	if caps.Resume || caps.MCPMode != "instruction" || caps.AgentAsTool {
		t.Fatalf("SDK 通道能力矩阵应与 31 号 §5 P2 行一致（无生命周期面）: %+v", caps)
	}
}
