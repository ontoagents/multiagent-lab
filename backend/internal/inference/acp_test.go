// S1+S2 单测：stub ACP agent（Node 按协议帧形态应答）驱动 streamingProc + acpAdapter。
// 覆盖：initialize/session/new/session/prompt 全链、事件转译（message.chunk/tool_call/tool_call_update）、
// 权限请求应答（默认 deny / 回调 selected·optionId，帧级 id 定向送回）、会话延续（session/resume
// 优先 + 失败回退 session/new + BuildPrompt(resumed) 历史去重）、MCP 引用透传、
// ctx 取消关闭阶梯（stdin EOF→SIGTERM→SIGKILL）幂等。
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

// stub 回显协议：首个 agent_message_chunk 携带会话建立摘要（mode=new|resumed|resume_failed、
// mcp=引用数、hist=提示是否含历史段），权限裁决后追加 perm=<outcome>:<optionId>——测试据此断言。
const stubACP = `#!/usr/bin/env node
// stub ACP agent：按 31 号 §12 S0 帧形态 + @agentclientprotocol schema 应答（dsh --profile acp 形态）
let buf = ''
let mode = 'new'
let mcpCount = 0
let permReply = ''
process.stdin.on('data', (d) => {
  buf += d
  let i
  while ((i = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, i).trim()
    buf = buf.slice(i + 1)
    if (!line) continue
    let f
    try { f = JSON.parse(line) } catch { continue }
    if (f.id != null && f.method === 'initialize') {
      send({ jsonrpc: '2.0', id: f.id, result: { protocolVersion: 1, agentCapabilities: { sessionCapabilities: { load: false, resume: true, close: true, list: true }, mcpCapabilities: { http: true } } } })
    } else if (f.id != null && f.method === 'session/new') {
      // dsh 真机形态：session/new 不收客户端命名，服务端分配 UUID
      mode = 'new'
      mcpCount = (f.params.mcpServers || []).length
      send({ jsonrpc: '2.0', id: f.id, result: { sessionId: 'stub-' + Math.random().toString(36).slice(2, 10) } })
    } else if (f.id != null && f.method === 'session/resume') {
      if (process.env.RESUME_FAIL === '1') {
        send({ jsonrpc: '2.0', id: f.id, error: { message: 'session is not resumable: ' + f.params.sessionId } })
        return
      }
      mode = 'resumed'
      mcpCount = (f.params.mcpServers || []).length
      send({ jsonrpc: '2.0', id: f.id, result: { sessionId: f.params.sessionId } })
    } else if (f.id != null && f.method === 'session/prompt') {
      // 真实 ACP：字段名 prompt（ContentBlock[]）；应答在 turn 末才到（stopReason 结算）
      const text = (f.params.prompt && f.params.prompt[0] && f.params.prompt[0].text) || ''
      const hist = text.includes('对话历史') ? '1' : '0'
      send({ jsonrpc: '2.0', id: 9001, method: 'session/request_permission', params: { sessionId: 's1', toolCall: { toolCallId: 'tc1', title: 'write_file', kind: 'edit' }, options: [ { optionId: 'allow1', name: 'Allow once', kind: 'allow_once' }, { optionId: 'reject1', name: 'Reject', kind: 'reject_once' } ] } })
      setTimeout(() => {
        send({ jsonrpc: '2.0', method: 'session/update', params: { sessionId: 's1', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'mode=' + mode + ';mcp=' + mcpCount + ';hist=' + hist + ';' } } } })
        send({ jsonrpc: '2.0', method: 'session/update', params: { sessionId: 's1', update: { sessionUpdate: 'tool_call', toolCallId: 't1', title: 'grep', rawInput: { pattern: 'x' } } } })
        send({ jsonrpc: '2.0', method: 'session/update', params: { sessionId: 's1', update: { sessionUpdate: 'tool_call_update', toolCallId: 't1', title: 'grep', status: 'completed', output: { content: [{ type: 'text', text: 'matched 3 lines' }] } } } })
        send({ jsonrpc: '2.0', method: 'session/update', params: { sessionId: 's1', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'perm=' + permReply + '。' } } } })
        send({ jsonrpc: '2.0', id: f.id, result: { stopReason: 'end_turn' } })
      }, 30)
    } else if (f.id === 9001 && f.result) {
      // 客户端对权限请求的应答（帧级 id 定向送回）
      permReply = 'none'
      const o = f.result.outcome || {}
      if (o.outcome === 'selected') permReply = 'selected:' + o.optionId
      else if (o.outcome === 'cancelled') permReply = 'cancelled'
    }
  }
})
function send(o) { process.stdout.write(JSON.stringify(o) + '\n') }
process.stdin.on('end', () => process.exit(0))
`

// stubOnPath 把 stub 脚本写入临时目录并返回注入 PATH 的环境前缀。
func stubOnPath(t *testing.T) (binPath string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "dsh")
	if err := os.WriteFile(p, []byte(stubACP), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return p
}

// runStub 执行一次 Run 并回收事件流与累计 message.delta 文本。
func runStub(t *testing.T, req *RunRequest) ([]Event, string) {
	t.Helper()
	a := newACPAdapter()
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

func TestACPAdapterRunEndToEnd(t *testing.T) {
	stubOnPath(t)
	if pr := newACPAdapter().Probe(context.Background()); !pr.Available {
		t.Fatalf("probe: %+v", pr)
	}
	// 无 Permission 回调 = 默认 deny（cancelled 应答送回）；AttemptResume=false 走 session/new；
	// BuildPrompt(false) 携带历史段、MCP 引用 2 条透传。
	var permCalled int
	evs, text := runStub(t, &RunRequest{
		Prompt:        "## 对话历史\n用户：上轮\n\n## 用户输入\n测试提示词",
		SessionID:     "conv-abc/1 x",
		AttemptResume: false,
		MCPServers: []MCPServerRef{
			{Name: "a", URL: "http://127.0.0.1:1/mcp"},
			{Name: "b", URL: "http://127.0.0.1:2/mcp", Headers: map[string]string{"X-Auth": "k"}},
		},
		Permission: func(PermissionRequest) PermissionDecision {
			permCalled++
			return PermissionDecision{Outcome: "cancelled"}
		},
	})
	if !strings.HasPrefix(text, "mode=new;mcp=2;hist=1;") {
		t.Fatalf("会话建立摘要不符: %q", text)
	}
	if !strings.Contains(text, "perm=cancelled") {
		t.Fatalf("权限默认 deny 应答未被 stub 收到: %q", text)
	}
	if permCalled != 1 {
		t.Fatalf("权限回调应恰被调用 1 次，实得 %d", permCalled)
	}
	var toolName, toolResult string
	for _, e := range evs {
		switch e.Type {
		case "tool.call":
			toolName, _ = e.Data["name"].(string)
		case "tool.result":
			toolResult, _ = e.Data["content"].(string)
		}
	}
	if toolName != "grep" || toolResult != "matched 3 lines" {
		t.Fatalf("tool events: name=%q result=%q", toolName, toolResult)
	}
}

func TestACPAdapterPermissionSelected(t *testing.T) {
	stubOnPath(t)
	// 回调裁决 selected → stub 应收到 {outcome:{outcome:"selected",optionId:"allow1"}}
	_, text := runStub(t, &RunRequest{
		Prompt: "x",
		Permission: func(pr PermissionRequest) PermissionDecision {
			if pr.Title != "write_file" || pr.ToolCallID != "tc1" || len(pr.Options) != 2 {
				t.Fatalf("权限载荷不符: %+v", pr)
			}
			return PermissionDecision{Outcome: "selected", OptionID: "allow1"}
		},
	})
	if !strings.Contains(text, "perm=selected:allow1") {
		t.Fatalf("selected+optionId 应答未被 stub 收到: %q", text)
	}
}

func TestACPAdapterResumeFlow(t *testing.T) {
	stubOnPath(t)
	// 非首轮回退场景：resume 失败 → 告警 + 回退 session/new + BuildPrompt(false) 全量历史。
	t.Setenv("RESUME_FAIL", "1")
	evs, text := runStub(t, &RunRequest{
		Prompt:        "ignored",
		SessionID:     "conv-r1",
		AttemptResume: true,
		BuildPrompt: func(resumed bool) string {
			if resumed {
				t.Fatalf("resume 失败时不得以 resumed=true 装配")
			}
			return "## 对话历史\n用户：上轮\n\n## 用户输入\nhi"
		},
	})
	warned := false
	for _, e := range evs {
		if e.Type == "run.warning" {
			if s, _ := e.Data["message"].(string); strings.Contains(s, "会话恢复失败") {
				warned = true
			}
		}
	}
	if !warned {
		t.Fatalf("resume 失败回退告警未透出")
	}
	if !strings.HasPrefix(text, "mode=new;") || !strings.Contains(text, "hist=1") {
		t.Fatalf("回退后应走 session/new 且全量历史: %q", text)
	}

	// 延续场景：resume 成功 → session/resume + BuildPrompt(true) 历史去重。
	t.Setenv("RESUME_FAIL", "")
	_, text = runStub(t, &RunRequest{
		Prompt:        "ignored",
		SessionID:     "conv-r1",
		AttemptResume: true,
		BuildPrompt: func(resumed bool) string {
			if !resumed {
				return "## 对话历史\n用户：上轮\n\n## 用户输入\nhi"
			}
			return "## 用户输入\nhi（增量）"
		},
	})
	if !strings.HasPrefix(text, "mode=resumed;") || !strings.Contains(text, "hist=0") {
		t.Fatalf("延续应走 session/resume 且历史去重: %q", text)
	}
}

func TestStreamingProcCloseLadderIdempotent(t *testing.T) {
	stubOnPath(t)
	sess, err := newACPSession("dsh", []string{"--profile", "acp"}, "", func(string, json.RawMessage) {}, func(int64, string, json.RawMessage) {})
	if err != nil {
		t.Fatal(err)
	}
	sess.close()
	sess.close() // 幂等：重复关闭不 panic 不挂死
	select {
	case <-sess.proc.done:
	case <-time.After(8 * time.Second):
		t.Fatalf("关闭阶梯未在 8s 内退出进程")
	}
}

func TestACPAdapterUnavailableWhenNoDSH(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // 空 PATH：dsh 不可达
	a := newACPAdapter()
	pr := a.Probe(context.Background())
	if pr.Available {
		t.Fatalf("dsh 缺席时应不可用")
	}
	if err := a.Run(context.Background(), &RunRequest{Prompt: "x"}, func(Event) {}); err == nil {
		t.Fatalf("dsh 缺席时 Run 应报错（全通道降级可懂）")
	}
}
