// S2/REQ-206（M41）：ACP 主通道客户端——dsh --profile acp 的 JSON-RPC 2.0 over stdio 客户端
// 与 acpAdapter 装配。
//
// 协议帧形态以 31 号 §12 S0 实证为准（smoke/dsh-s0 采集）：
//   - initialize{protocolVersion,clientCapabilities} → agentCapabilities（含 sessionCapabilities.resume、
//     mcpCapabilities.http）；
//   - session/new{cwd,mcpServers} → {sessionId}（sessionId 客户端自由命名亦可，平台生成 plt- 前缀）；
//   - session/prompt{sessionId,content} → **turn 结束时才应答**（stopReason: end_turn/cancelled…），
//     过程事件经 session/update 通知流出（agent_message_chunk / tool_call{rawInput} →
//     tool_call_update{completed|failed}）；
//   - session/cancel{sessionId} 通知 = 中断；session/request_permission 请求 → 经 RunRequest.Permission
//     回调裁决（平台审批面承接；nil = 默认 deny），应答 {outcome:{outcome:"selected",optionId}|cancelled}；
//   - session/resume{sessionId,configOptions} → 跨进程恢复会话（M24 存量会话实证可恢复）。
//
// REQ-206/M41 S2 真机面（dsh 0.1.5-rc.3 装机后翻正）：①会话延续——同平台会话跨 Run 先
// session/resume 复用 dsh 持久化会话（历史去重：resumed 时 BuildPrompt(false→true) 不再拼平台
// 历史段），缺失回退 session/new 全量提示；②MCP 挂载——连接器实例 URL/headers 经 session/new
// mcpServers（HTTP 条目）注入，dsh 直连（MCPMode 翻正 tools）；③权限转审批——request_permission
// 经回调交平台审批面（approval.request 事件 + 决议端点），帧级 id 经 onRequest 透传
// （S2 修正：此前 id 丢失、应答从未真正发回）。
package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// acpSession 一次长驻 dsh 进程上的会话客户端（JSON-RPC 2.0 行分帧）。
type acpSession struct {
	proc  *streamingProc
	mu    sync.Mutex
	next  int64
	pend  map[int64]chan acpResp
	toolNames map[string]string // toolCallID → 工具名（tool_call 时记；update 帧不带 title，回填 tool.result.name）
	// notif 通知（无 id）回调；onRequest server→client request（带帧级 id，如 session/request_permission）
	// 回调——id 必须经此透传，应答才能定向送回（S2 真机联调修正：此前帧级 id 丢失、应答从未发出）。
	notif     func(method string, params json.RawMessage)
	onRequest func(id int64, method string, params json.RawMessage)
}

type acpResp struct {
	result json.RawMessage
	errMsg string
}

func newACPSession(bin string, args []string, cwd string, notif func(string, json.RawMessage), onRequest func(int64, string, json.RawMessage)) (*acpSession, error) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("PATH 中未找到 %q（未安装或未加入 PATH）", bin)
	}
	s := &acpSession{pend: map[int64]chan acpResp{}, toolNames: map[string]string{}, notif: notif, onRequest: onRequest}
	proc, err := startStreaming("dsh-acp", path, args, cwd, os.Environ(), func(line string) {
		s.onFrame(line)
	})
	if err != nil {
		return nil, err
	}
	s.proc = proc
	return s, nil
}

// onFrame 行帧分发：JSON-RPC response（带 id）→ 唤醒等待者；notification/request → notif 回调。
// （session/request_permission 以 server→client request 形态到达，也走 notif，由适配器应答。）
func (s *acpSession) onFrame(line string) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return
	}
	var frame struct {
		ID     *int64          `json:"id"`
		Method string          `json:"method"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
		Params json.RawMessage `json:"params"`
	}
	if json.Unmarshal([]byte(line), &frame) != nil {
		return
	}
	if frame.ID != nil && frame.Method != "" {
		// server→client request（如 session/request_permission）：帧级 id 交 onRequest 由适配器应答
		if s.onRequest != nil {
			s.onRequest(*frame.ID, frame.Method, frame.Params)
		}
		return
	}
	if frame.ID != nil {
		s.mu.Lock()
		ch, ok := s.pend[*frame.ID]
		if ok {
			delete(s.pend, *frame.ID)
		}
		s.mu.Unlock()
		if ok {
			msg := ""
			if frame.Error != nil {
				msg = frame.Error.Message
			}
			ch <- acpResp{result: frame.Result, errMsg: msg}
		}
		return
	}
	if frame.Method != "" && s.notif != nil {
		s.notif(frame.Method, frame.Params)
	}
}

// call 发送 JSON-RPC 请求并等待应答（timeout 兜底防挂死）。
func (s *acpSession) call(ctx context.Context, method string, params any, timeout time.Duration) (json.RawMessage, error) {
	s.mu.Lock()
	s.next++
	id := s.next
	ch := make(chan acpResp, 1)
	s.pend[id] = ch
	s.mu.Unlock()
	req, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if err := s.proc.send(string(req)); err != nil {
		return nil, err
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case r := <-ch:
		if r.errMsg != "" {
			return nil, fmt.Errorf("%s 应答错误: %s", method, r.errMsg)
		}
		return r.result, nil
	case <-t.C:
		s.mu.Lock()
		delete(s.pend, id)
		s.mu.Unlock()
		return nil, fmt.Errorf("%s 超时（%s）", method, timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// notify 发送通知（无 id，不应答）。
func (s *acpSession) notify(method string, params any) error {
	req, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return err
	}
	return s.proc.send(string(req))
}

// respond 应答 server→client 的 request（如 session/request_permission）。
func (s *acpSession) respond(id int64, result any) error {
	req, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	if err != nil {
		return err
	}
	return s.proc.send(string(req))
}

func (s *acpSession) close() { s.proc.close() }

// acpContentBlock ACP 内容块：text 直形（stub/简单文本）或 content 嵌套形（S2 真机 tool_call_update）。
type acpContentBlock struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Content *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (b acpContentBlock) plainText() string {
	if b.Content != nil && b.Content.Text != "" {
		return b.Content.Text
	}
	return b.Text
}

// acpContent content 字段多态：agent_message_chunk 为单对象，tool_call_update 为数组（S2 真机实证）。
type acpContent []acpContentBlock

func (c *acpContent) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		return json.Unmarshal(b, (*[]acpContentBlock)(c))
	}
	var one acpContentBlock
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*c = acpContent{one}
	return nil
}

// acpUpdate session/update 通知载荷（content 多态见 acpContent；output 为 S0 stub 兼容形态）。
type acpUpdate struct {
	SessionID string `json:"sessionId"`
	Update    struct {
		SessionUpdate string          `json:"sessionUpdate"` // agent_message_chunk | tool_call | tool_call_update
		Content       acpContent      `json:"content"`
		ToolCallID    string          `json:"toolCallId"`
		Title         string          `json:"title"`
		RawInput      json.RawMessage `json:"rawInput"`
		Status        string          `json:"status"`
		Output        *struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	} `json:"update"`
}

// onUpdate session/update 事件转译：agent_message_chunk→message.delta、tool_call（含 rawInput）→
// tool.call、tool_call_update（completed/failed+content/output 文本）→tool.result。
func (s *acpSession) onUpdate(params json.RawMessage, emit func(Event)) {
	var u acpUpdate
	if json.Unmarshal(params, &u) != nil {
		return
	}
	switch u.Update.SessionUpdate {
	case "agent_message_chunk":
		for _, c := range u.Update.Content {
			if t := c.plainText(); t != "" {
				emit(Event{Type: "message.delta", Data: map[string]any{"delta": t}})
			}
		}
	case "tool_call":
		data := map[string]any{"name": u.Update.Title, "source": "inference"}
		if u.Update.ToolCallID != "" && u.Update.Title != "" {
			s.mu.Lock()
			s.toolNames[u.Update.ToolCallID] = u.Update.Title
			s.mu.Unlock()
		}
		if len(u.Update.RawInput) > 0 {
			var input map[string]any
			if json.Unmarshal(u.Update.RawInput, &input) == nil {
				data["args"] = input
			}
		}
		emit(Event{Type: "tool.call", Data: data})
	case "tool_call_update":
		text := ""
		for _, c := range u.Update.Content {
			text += c.plainText()
		}
		if u.Update.Output != nil {
			for _, c := range u.Update.Output.Content {
				if c.Text != "" {
					text += c.Text
				}
			}
		}
		// update 帧不带 title（S2 真机实证）——按 toolCallId 回填 tool.call 记下的工具名
		name := u.Update.Title
		if name == "" && u.Update.ToolCallID != "" {
			s.mu.Lock()
			name = s.toolNames[u.Update.ToolCallID]
			s.mu.Unlock()
		}
		data := map[string]any{"name": name, "source": "inference"}
		if u.Update.Status == "failed" {
			data["error"] = true
		}
		if text != "" {
			data["content"] = text
		}
		emit(Event{Type: "tool.result", Data: data})
	}
}

// ---- acpAdapter：dsh --profile acp 长驻进程 + 会话映射 + 事件转译 ----

type acpAdapter struct {
	bin         []string
	versionArgs []string
	profileArgs []string
}

func newACPAdapter() Backend {
	return &acpAdapter{
		bin:         []string{"dsh"},
		versionArgs: []string{"--version"},
		profileArgs: []string{"--profile", "acp"}, // S0：发行模板自动引导，无需手工建 profile
	}
}

func (a *acpAdapter) Name() string { return "deepseek-harness-acp" }

// Capabilities（REQ-206/M41 S2 真机面翻正）：Resume=true（跨 Run session/resume 延续已实现）、
// MCPMode=tools（会话级 HTTP MCP 挂载：platform 把连接器实例 URL/headers 注入 session/new，
// dsh 作为受信控制器直连——凭据 header 仅装配栈内存传递）。技能仍 instruction（dsh 有自己的
// 技能体系，平台技能继续说明注入）；AgentAsTool/Workflow 不支持（单 Agent 通道）。
func (a *acpAdapter) Capabilities() Capabilities {
	return Capabilities{
		Chat: true, Stream: true,
		SkillsMode: "instruction", MCPMode: "tools",
		AgentAsTool: false, Workflow: false,
		Resume: true,
	}
}

// Probe 与 cliAdapter 同口径：PATH + --version。
func (a *acpAdapter) Probe(ctx context.Context) ProbeResult {
	path, err := exec.LookPath(a.bin[0])
	if err != nil {
		return ProbeResult{Available: false, Reason: fmt.Sprintf("PATH 中未找到 %q（未安装或未加入 PATH）", a.bin[0])}
	}
	vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(vctx, path, a.versionArgs...).Output()
	if err != nil {
		return ProbeResult{Available: false, Path: path, Reason: fmt.Sprintf("执行 %s --version 失败: %v", a.bin[0], err)}
	}
	version := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	return ProbeResult{Available: true, Version: version, Path: path}
}

// acpSessionMap 平台会话 → dsh 会话 UUID 的落盘映射（REQ-206/M41 S2 会话延续的数据面）。
// S2 真机定案：dsh 0.1.5-rc.3 不收客户端命名——session/new 应答服务端 UUID（S0 设计输入
// 「sessionId 客户端命名可用」在该构建不成立），跨 Run/跨进程 resume 必须持久化映射。
// 文件按 key 原子重写（量级=会话数，TTL 不设；孤儿项无害——会话删除后不再被 resume）。
type acpSessionMap struct {
	mu   sync.Mutex
	path string
	m    map[string]string
}

var acpSessions = &acpSessionMap{path: sessionMapPath()}

func sessionMapPath() string {
	if p := strings.TrimSpace(os.Getenv("ACP_SESSION_MAP")); p != "" {
		return p
	}
	return "data/acp_sessions.json"
}

func (m *acpSessionMap) load() {
	if m.m != nil {
		return
	}
	m.m = map[string]string{}
	if b, err := os.ReadFile(m.path); err == nil {
		_ = json.Unmarshal(b, &m.m)
	}
}

func (m *acpSessionMap) get(key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.load()
	return m.m[key]
}

func (m *acpSessionMap) set(key, val string) {
	m.mu.Lock()
	m.load()
	m.m[key] = val
	b, err := json.Marshal(m.m)
	m.mu.Unlock()
	if err == nil {
		_ = os.MkdirAll(filepath.Dir(m.path), 0o755)
		_ = os.WriteFile(m.path, b, 0o600)
	}
}

// Run 一次推理 = 一个 ACP turn：长驻进程 + 会话建立（REQ-206/M41 S2 会话延续：有前轮先
// session/resume 复用 dsh 持久化会话〔映射落盘 data/acp_sessions.json〕，缺失/失败回退
// session/new 并如实告警）→ BuildPrompt(resumed) 装配提示（resumed=true 时历史去重）→
// session/prompt（ACP 字段名 prompt）→ 事件转译 → 应答携带 stopReason 即 turn 结束。
// ctx 取消 → session/cancel + 关闭进程。
func (a *acpAdapter) Run(ctx context.Context, req *RunRequest, emit func(Event)) error {
	var sessRef *acpSession
	var permFn func(req PermissionRequest) PermissionDecision = func(PermissionRequest) PermissionDecision {
		return PermissionDecision{Outcome: "cancelled"}
	}
	if req.Permission != nil {
		permFn = req.Permission
	}
	// onRequest：session/request_permission（带帧级 id）→ 权限回调 → 应答定向送回（optionId 形态
	// 以 @agentclientprotocol schema 为准：{outcome:{outcome:"selected",optionId}} | cancelled）。
	onReq := func(id int64, method string, params json.RawMessage) {
		if method != "session/request_permission" || sessRef == nil {
			return
		}
		var pr struct {
			ToolCall struct {
				ToolCallID string `json:"toolCallId"`
				Title      string `json:"title"`
				Kind       string `json:"kind"`
			} `json:"toolCall"`
			Options []PermissionOption `json:"options"`
		}
		_ = json.Unmarshal(params, &pr)
		// dsh 权限帧常不带 title（S2 真机实证）——用 tool.call 记下的 toolCallID→名称映射回填
		if pr.ToolCall.Title == "" && pr.ToolCall.ToolCallID != "" {
			sessRef.mu.Lock()
			pr.ToolCall.Title = sessRef.toolNames[pr.ToolCall.ToolCallID]
			sessRef.mu.Unlock()
		}
		dec := permFn(PermissionRequest{ToolCallID: pr.ToolCall.ToolCallID, Title: pr.ToolCall.Title, Kind: pr.ToolCall.Kind, Options: pr.Options})
		var outcome any
		if dec.Outcome == "selected" && dec.OptionID != "" {
			outcome = map[string]any{"outcome": "selected", "optionId": dec.OptionID}
		} else {
			outcome = map[string]any{"outcome": "cancelled"}
		}
		_ = sessRef.respond(id, map[string]any{"outcome": outcome})
	}
	notifFn := func(method string, params json.RawMessage) {
		if sessRef != nil {
			sessRef.onUpdate(params, emit)
		}
	}
	sess, err := newACPSession(a.bin[0], a.profileArgs, req.Cwd, notifFn, onReq)
	if err != nil {
		return err
	}
	sessRef = sess
	defer sess.close()

	// initialize（协议握手；agentCapabilities 校验从宽——S0：版本恒 0.0.1 不校验须自校验）
	if _, err := sess.call(ctx, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientCapabilities": map[string]any{"fs": map[string]any{"readTextFile": false, "writeTextFile": false}},
	}, 15*time.Second); err != nil {
		return fmt.Errorf("ACP initialize 失败: %w", err)
	}
	cwd := orDot(req.Cwd)
	// ACP McpServer 为判别联合：HTTP 条目必须带 type:"http"，headers 为 [{name,value}] 数组且
	// 字段必填（S2 真机实证：缺 type 或 headers 任一，条目被静默丢弃不挂载）。
	mcpRefs := make([]map[string]any, 0, len(req.MCPServers))
	for _, m := range req.MCPServers {
		hs := make([]map[string]string, 0, len(m.Headers))
		for k, v := range m.Headers {
			hs = append(hs, map[string]string{"name": k, "value": v})
		}
		mcpRefs = append(mcpRefs, map[string]any{"type": "http", "name": m.Name, "url": m.URL, "headers": hs})
	}
	// 会话建立：延续优先（AttemptResume 且映射在案），失败回退新建并如实告警（历史去重随之
	// 失效，BuildPrompt(false) 重新装配全量提示）。dsh 不收客户端命名：new/resume 均以服务端
	// 应答的 sessionId 为准并回写映射。
	var sessionID string
	resumed := false
	if req.AttemptResume && req.SessionID != "" {
		if saved := acpSessions.get(req.SessionID); saved != "" {
			sessionID = saved
			if _, err := sess.call(ctx, "session/resume", map[string]any{"cwd": cwd, "sessionId": sessionID, "mcpServers": mcpRefs}, 15*time.Second); err != nil {
				emit(Event{Type: "run.warning", Data: map[string]any{"message": fmt.Sprintf("dsh 会话恢复失败（持久化会话缺失或引擎不可达：%v），已改新建会话，本轮提示携带全量平台历史", err)}})
				sessionID = ""
			} else {
				resumed = true
			}
		}
	}
	if !resumed {
		var out struct {
			SessionID string `json:"sessionId"`
		}
		result, cerr := sess.call(ctx, "session/new", map[string]any{"cwd": cwd, "mcpServers": mcpRefs}, 15*time.Second)
		if cerr != nil {
			return fmt.Errorf("ACP session/new 失败: %w", cerr)
		}
		_ = json.Unmarshal(result, &out)
		sessionID = out.SessionID
		if sessionID == "" {
			return fmt.Errorf("ACP session/new 未返回 sessionId")
		}
		if req.SessionID != "" {
			acpSessions.set(req.SessionID, sessionID)
		}
	}
	// 提示装配在会话定案之后（resumed 决定历史段去重与否）。
	prompt := req.Prompt
	if req.BuildPrompt != nil {
		prompt = req.BuildPrompt(resumed)
	}
	// session/prompt：应答在 turn 结束时到达（stopReason）。
	result, err := sess.call(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": prompt}},
	}, 30*time.Minute)
	if ctx.Err() != nil {
		_ = sess.notify("session/cancel", map[string]any{"sessionId": sessionID})
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	var out struct {
		StopReason string `json:"stopReason"`
	}
	_ = json.Unmarshal(result, &out)
	if req.Debug >= 2 {
		mode := "new"
		if resumed {
			mode = "resumed"
		}
		emit(Event{Type: "debug.cli", Data: map[string]any{"stopReason": out.StopReason, "sessionId": sessionID, "acpSession": mode}})
	}
	return nil
}

// orDot 会话工作目录回落：dsh（ACP ResumeSessionRequest/NewSessionRequest）要求**绝对路径**，
// 空值回落进程当前工作目录（S2 真机修正：原回落 "." 被 dsh 以 "cwd must be an absolute path" 拒绝）。
func orDot(s string) string {
	if strings.TrimSpace(s) == "" {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
		return "."
	}
	return s
}
