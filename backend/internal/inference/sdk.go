// S3/REQ-206（M41）：SDK 事件审计通道——dsh --profile sdk 的 JSON-RPC 2.0 over stdio 客户端
// 与 sdkAdapter 装配。定位（31 号 §4 P2）：事件富度通道，非生命周期通道——
// `session.event` 不过滤地投递运行时内每个会话的全量 session-log 事件（含 ACP 不暴露的
// 治理面 sandbox/approval 与请求面 request/header/context），全量转译落平台 run_event
// 供审计与回放；会话生命周期（resume/取消/权限/MCP 挂载）由 ACP 主通道承载，本通道
// 无 resume 方法、无 cancel 方法（放弃轮次=关闭运行时）、server→client 请求未实现
// （无权限回调）——官方 Known Limitations，协议包 types.d.ts 与 README 实证（0.1.5-rc.3）。
//
// 协议帧形态以本机 @deepseek-ai/dsh-sdk-protocol 0.1.5-rc.3 为准（31 号 §12 S0 实证同源）：
//   - initialize{cwd,provider,model[,reasoningEffort,maxTokens]} → {serverInfo:{name,version}}——
//     provider/model 在该构建**必填**（缺省恒抛 no adapter registered），路由族默认双值
//     "deepseek-official"（服务端字段缺省同值；模型选择走 dsh 侧自身配置）；
//   - session/prompt{sessionId,contentBlocks:[{type:"text",text}]} → {messageId}
//     **持久入队回执，非结果**——过程经 session.event 通知流出，turn/end 即本轮结算；
//   - shutdown{} → {}（应答后 dispose）；
//   - session.event 封套 {sessionId, event:{type,seq,time,data[,ignorable]}}，data 按 type 分派：
//     tool/call{callId,name,arguments 原始 JSON 串}/tool/result{message.source.callId,content}/
//     assistant/message{message.content 文本块}/turn/end{reason}/其余全量审计转发。
//
// 一次 Run = 一个进程一个会话一轮：spawn → initialize → session/prompt（sessionId 平台命名
// plt-<conv>-<milli>，逐 Run 新会话；平台历史由提示装配承载，dsh 侧会话即审计工件）→
// 等待 turn/end（session.status idle 为兜底结算信号）→ 静默窗口收尾帧 → shutdown → 关闭阶梯。
package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// sdkAuditEvent 平台侧审计事件类型：承载 dsh session-log 原始事件的转发（审计/回放消费，
// 与 SSE 透传同源——emitAndRecord 全量落 run_event）。
const sdkAuditEvent = "external.audit"

// sdkAuditCap 单条审计事件载荷上限（字节）：request/header 携带全量工具 schema 可达数十 KB，
// 超限截断保头（审计保真与 run_event 体积的折中；截断以 marker 诚实标注）。
const sdkAuditCap = 16 * 1024

// sdkIdleWatchdog 通道空闲看门狗：超过该时长无任何入站帧判通道失联（正常模型轮步间
// 间隔为秒级；真实长思考亦在分钟内出帧）。
const sdkIdleWatchdog = 5 * time.Minute

// sdkProviderRoute / sdkModelRoute initialize 必填路由双值（0.1.5-rc.3 实证：缺省即抛错；
// 服务端字段缺省同为 "deepseek-official"）。
const (
	sdkProviderRoute = "deepseek-official"
	sdkModelRoute    = "deepseek-official"
)

type sdkAdapter struct {
	bin         string
	versionArgs []string
	profileArgs []string
}

func newSDKAdapter() Backend {
	return &sdkAdapter{
		bin:         "dsh",
		versionArgs: []string{"--version"},
		profileArgs: []string{"--profile", "sdk"}, // S0：发行模板自动引导
	}
}

func (a *sdkAdapter) Name() string { return "deepseek-harness-sdk" }

// Capabilities（31 号 §5 P2 行）：事件审计富度通道——工具事件/治理面/请求面全量可见，
// 但无生命周期（Resume ❌ 无方法/取消=关运行时/MCP 无挂载面/无权限回调）。
func (a *sdkAdapter) Capabilities() Capabilities {
	return Capabilities{
		Chat: true, Stream: true,
		SkillsMode: "instruction", MCPMode: "instruction",
		AgentAsTool: false, Workflow: false,
		Resume: false,
	}
}

func (a *sdkAdapter) Probe(ctx context.Context) ProbeResult {
	path, err := exec.LookPath(a.bin)
	if err != nil {
		return ProbeResult{Available: false, Reason: fmt.Sprintf("PATH 中未找到 %q（未安装或未加入 PATH）", a.bin)}
	}
	vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(vctx, path, a.versionArgs...).Output()
	if err != nil {
		return ProbeResult{Available: false, Path: path, Reason: fmt.Sprintf("执行 %s --version 失败: %v", a.bin, err)}
	}
	version := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	return ProbeResult{Available: true, Version: version, Path: path}
}

// sdkClient 一个 SDK runtime 进程上的 JSON-RPC 客户端（行分帧复用 acpSession 骨架的心智，
// 但帧分发不处理 server→client request——SDK 通道服务器从不发送）。
type sdkClient struct {
	proc *streamingProc
	mu   sync.Mutex
	next int64
	pend map[int64]chan acpResp
	// onNotif 通知回调（session.event/session.status/subagent.*）。
	onNotif func(method string, params json.RawMessage)
}

func newSDKClient(bin string, args []string, cwd string, env []string, onNotif func(string, json.RawMessage)) (*sdkClient, error) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("PATH 中未找到 %q（未安装或未加入 PATH）", bin)
	}
	c := &sdkClient{pend: map[int64]chan acpResp{}, onNotif: onNotif}
	proc, err := startStreaming("dsh-sdk", path, args, cwd, env, func(line string) {
		c.onFrame(line)
	})
	if err != nil {
		return nil, err
	}
	c.proc = proc
	return c, nil
}

// onFrame 行帧分发：response（带 id 无 method）→ 唤醒等待者；notification（仅 method）→ onNotif。
// （带 id 带 method 的 server→client request 在 SDK 通道不出现——传输层支持但服务器从不发送，
// 0.1.5-rc.3 实证；防御起见丢弃不崩溃。）
func (c *sdkClient) onFrame(line string) {
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
		return // server→client request：SDK 通道未使用，忽略
	}
	if frame.ID != nil {
		c.mu.Lock()
		ch, ok := c.pend[*frame.ID]
		if ok {
			delete(c.pend, *frame.ID)
		}
		c.mu.Unlock()
		if ok {
			msg := ""
			if frame.Error != nil {
				msg = frame.Error.Message
			}
			ch <- acpResp{result: frame.Result, errMsg: msg}
		}
		return
	}
	if frame.Method != "" && c.onNotif != nil {
		c.onNotif(frame.Method, frame.Params)
	}
}

// call 发送请求并等待应答（timeout 兜底防挂死）。
func (c *sdkClient) call(ctx context.Context, method string, params any, timeout time.Duration) (json.RawMessage, error) {
	c.mu.Lock()
	c.next++
	id := c.next
	ch := make(chan acpResp, 1)
	c.pend[id] = ch
	c.mu.Unlock()
	req, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if err := c.proc.send(string(req)); err != nil {
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
		c.mu.Lock()
		delete(c.pend, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("%s 超时（%s）", method, timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// sdkSessionEvent session.event 封套（协议包 SessionEventNotification + SessionEvent envelope）。
type sdkSessionEvent struct {
	SessionID string `json:"sessionId"`
	Event     struct {
		Type string          `json:"type"`
		Seq  int64           `json:"seq"`
		Data json.RawMessage `json:"data"`
	} `json:"event"`
}

// sdkContentBlock dsh-llm ContentBlock（text 直取；tool-call/reasoning 等其余形态按原文透传）。
type sdkContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// sdkToolResultMessage tool/result 的 message 字段（ToolResultMessage：role=user，
// content=[ToolResultBlock{content:ContentBlock[]}]，callId 关联在 source.callId）。
type sdkToolResultMessage struct {
	Content []struct {
		Type    string            `json:"type"`
		Content []sdkContentBlock `json:"content"`
		IsError bool              `json:"isError"`
	} `json:"content"`
	Source struct {
		Kind   string `json:"kind"`
		CallID string `json:"callId"`
	} `json:"source"`
}

// sdkEventTranslator session.event → 平台事件转译器（一次 Run 一个实例）。
// 语义映射：tool/call→tool.call、tool/result→tool.result（callId 回填工具名）、
// assistant/message 文本块→message.delta；其余（turn/step 边界、request/header、
// request/context、session/title、sandbox/approval 治理面等）全量 external.audit 审计转发。
type sdkEventTranslator struct {
	mu        sync.Mutex
	callNames map[string]string // callId → 工具名（tool.result 回填；result 帧不带 name）
}

func (t *sdkEventTranslator) onSessionEvent(params json.RawMessage, emit func(Event)) (finished bool) {
	var n sdkSessionEvent
	if json.Unmarshal(params, &n) != nil || n.Event.Type == "" {
		return false
	}
	switch n.Event.Type {
	case "tool/call":
		var d struct {
			CallID    string `json:"callId"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"` // 模型产出的原始 JSON 串（未解析）
		}
		if json.Unmarshal(n.Event.Data, &d) != nil {
			return false
		}
		if d.CallID != "" && d.Name != "" {
			t.mu.Lock()
			t.callNames[d.CallID] = d.Name
			t.mu.Unlock()
		}
		data := map[string]any{"name": d.Name, "source": "inference"}
		if strings.TrimSpace(d.Arguments) != "" {
			var args map[string]any
			if json.Unmarshal([]byte(d.Arguments), &args) == nil {
				data["args"] = args
			} else {
				data["args_raw"] = d.Arguments
			}
		}
		emit(Event{Type: "tool.call", Data: data})
	case "tool/result":
		var d struct {
			Message sdkToolResultMessage `json:"message"`
			Error   *struct {
				Name string `json:"name"`
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(n.Event.Data, &d) != nil {
			return false
		}
		var text strings.Builder
		isErr := d.Error != nil
		for _, blk := range d.Message.Content {
			if blk.IsError {
				isErr = true
			}
			for _, c := range blk.Content {
				if c.Type == "text" && c.Text != "" {
					text.WriteString(c.Text)
				}
			}
		}
		name := ""
		if id := d.Message.Source.CallID; id != "" {
			t.mu.Lock()
			name = t.callNames[id]
			t.mu.Unlock()
		}
		data := map[string]any{"name": name, "source": "inference"}
		if isErr {
			data["error"] = true
		}
		if text.Len() > 0 {
			data["content"] = text.String()
		}
		emit(Event{Type: "tool.result", Data: data})
	case "assistant/message":
		var d struct {
			Message struct {
				Content []sdkContentBlock `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(n.Event.Data, &d) != nil {
			return false
		}
		// 只取文本块（reasoning/tool-call 不作正文重放；推理内容沿平台 reasoning.delta 语义不在
		// 外部通道映射，审计事件携带原始帧）
		var text strings.Builder
		for _, c := range d.Message.Content {
			if c.Type == "text" {
				text.WriteString(c.Text)
			}
		}
		if text.Len() > 0 {
			emit(Event{Type: "message.delta", Data: map[string]any{"delta": text.String()}})
		}
	case "turn/end":
		t.audit(n, emit)
		return true // 本轮结算信号（主Run流程据此收尾）
	default:
		t.audit(n, emit)
	}
	return false
}

// audit 其余事件全量转发为 external.audit 审计事件（payload 截断保头）。
func (t *sdkEventTranslator) audit(n sdkSessionEvent, emit func(Event)) {
	payload := string(n.Event.Data)
	if len(payload) > sdkAuditCap {
		payload = payload[:sdkAuditCap] + "…（截断，原始长度 " + fmt.Sprint(len(n.Event.Data)) + " 字节）"
	}
	emit(Event{Type: sdkAuditEvent, Data: map[string]any{
		"backend":    "deepseek-harness-sdk",
		"event":      n.Event.Type,
		"seq":        n.Event.Seq,
		"session_id": n.SessionID,
		"payload":    payload,
	}})
}

// Run 一次推理 = 一个 SDK runtime 进程上的一轮 turn（形态详见包注）。
func (a *sdkAdapter) Run(ctx context.Context, req *RunRequest, emit func(Event)) error {
	var (
		turnDone   = make(chan struct{}) // turn/end 结算信号（close 即完成）
		doneOnce   sync.Once
		promptSent atomic.Bool // idle 兜底结算仅在 prompt 已入队后生效（防通道早期噪声帧提前收尾）
		serverInfo map[string]any
		messageID  string
		sessionID  = fmt.Sprintf("plt-%s-%d", orEmptySession(req.SessionID), time.Now().UnixMilli())
	)
	tr := &sdkEventTranslator{callNames: map[string]string{}}
	markDone := func() { doneOnce.Do(func() { close(turnDone) }) }
	onNotif := func(method string, params json.RawMessage) {
		switch method {
		case "session.event":
			if tr.onSessionEvent(params, emit) {
				markDone()
			}
		case "session.status":
			// 整个 agent 的 running/idle 转换：idle 兜底结算（turn/end 缺失的防御路径，
			// 正常路径 turn/end 先到）
			var st struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(params, &st) == nil && st.Status == "idle" && promptSent.Load() {
				markDone()
			}
		default:
			// subagent.started/finished 等其余通知：会话级血缘审计
			payload := string(params)
			if len(payload) > sdkAuditCap {
				payload = payload[:sdkAuditCap] + "…（截断）"
			}
			emit(Event{Type: sdkAuditEvent, Data: map[string]any{
				"backend": a.Name(), "event": method, "payload": payload,
			}})
		}
	}
	env := os.Environ()
	for k, v := range req.Env {
		env = append(env, k+"="+v)
	}
	cl, err := newSDKClient(a.bin, a.profileArgs, req.Cwd, env, onNotif)
	if err != nil {
		return err
	}
	defer cl.proc.close()

	// initialize：provider/model 必填（0.1.5-rc.3 实证缺省抛 no adapter registered），
	// 双值取路由族缺省 deepseek-official（模型选择由 dsh 侧自身配置承载）。
	initResult, err := cl.call(ctx, "initialize", map[string]any{
		"cwd":      orDot(req.Cwd),
		"provider": sdkProviderRoute,
		"model":    sdkModelRoute,
	}, 30*time.Second)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("SDK initialize 失败: %w", err)
	}
	var ir struct {
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	_ = json.Unmarshal(initResult, &ir)
	serverInfo = map[string]any{"name": ir.ServerInfo.Name, "version": ir.ServerInfo.Version}
	if ir.ServerInfo.Name != "" && ir.ServerInfo.Name != "deepseek-harness-sdk-runtime" {
		// serverInfo.name 为协议稳定值（types.d.ts 实证）——异常值诚实拒绝，不做静默适配
		return fmt.Errorf("SDK initialize 返回异常 serverInfo.name %q（期望 deepseek-harness-sdk-runtime）", ir.ServerInfo.Name)
	}

	// session/prompt：持久入队回执（messageId），过程事件经 session.event 流出。
	promptResult, err := cl.call(ctx, "session/prompt", map[string]any{
		"sessionId":     sessionID,
		"contentBlocks": []map[string]any{{"type": "text", "text": req.Prompt}},
	}, 30*time.Second)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("SDK session/prompt 失败: %w", err)
	}
	var pr struct {
		MessageID string `json:"messageId"`
	}
	_ = json.Unmarshal(promptResult, &pr)
	messageID = pr.MessageID
	promptSent.Store(true)
	if req.Debug >= 2 {
		emit(Event{Type: "debug.cli", Data: map[string]any{
			"phase": "sdk", "sessionId": sessionID, "messageId": messageID, "serverInfo": serverInfo,
		}})
	}

	// 等待 turn 结算（turn/end 或 session.status idle），带空闲看门狗与进程退出监控。
	watchdog := time.NewTimer(sdkIdleWatchdog)
	defer watchdog.Stop()
	exitCh := cl.proc.done
	select {
	case <-turnDone:
	case <-watchdog.C:
		return fmt.Errorf("SDK 通道空闲超时（%s 无入站事件），放弃本轮", sdkIdleWatchdog)
	case <-exitCh:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("SDK runtime 进程在 turn 结算前退出（%v）", cl.proc.waitErr)
	case <-ctx.Done():
		return ctx.Err() // 取消=关闭运行时（SDK 通道无 cancel 方法，官方口径）
	}

	// 收尾帧静默窗口：turn/end 后仍可能有 session/title、subagent.finished 等尾随事件，
	// 静默 2s 或进程退出即止，随后 shutdown（幂等，失败不致命——关闭阶梯兜底）。
	quiet := time.NewTimer(2 * time.Second)
	defer quiet.Stop()
loop:
	for {
		select {
		case <-quiet.C:
			break loop
		case <-exitCh:
			break loop
		case <-ctx.Done():
			break loop
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = cl.call(shutdownCtx, "shutdown", map[string]any{}, 3*time.Second)
	if req.Debug >= 2 {
		emit(Event{Type: "debug.cli", Data: map[string]any{"phase": "sdk-done", "sessionId": sessionID}})
	}
	return nil
}

// orEmptySession 会话标识空值回落（RunRequest.SessionID 由 chat 层恒注入 conv.ID；
// 空值防御回落 anon，仅影响 dsh 侧会话命名）。
func orEmptySession(s string) string {
	if strings.TrimSpace(s) == "" {
		return "anon"
	}
	return s
}
