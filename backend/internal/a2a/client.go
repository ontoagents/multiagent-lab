// Package a2a — REQ-416/M212：A2A 委派客户端（协议面 client 侧补齐，此前仅 server 单向）。
// 外部 A2A 智能体经 Agent Card 发现（真握手）+ message/send 委派 + tasks/get 轮询至终态。
// 诚实边界（与 server 端 capabilities.streaming=false 对称）：无 message/stream 订阅（纯轮询）、
// 纯文本单模态（text parts）、无 pushNotification 配置——协作面按「外部成员」如实标注。
package a2a

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CardSkill Agent Card 技能条目（合成协作面工具描述用）。
type CardSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// AgentCard 远端 Agent Card（注册握手入库的最小面；全卡另存 CardJSON）。
type AgentCard struct {
	Name               string      `json:"name"`
	Description        string      `json:"description"`
	URL                string      `json:"url"`
	ProtocolVersion    string      `json:"protocolVersion"`
	PreferredTransport string      `json:"preferredTransport"`
	Skills             []CardSkill `json:"skills"`
	Capabilities       struct {    // REQ-429/M225：能力画像（streaming=流式在环性）
		Streaming bool `json:"streaming"`
	} `json:"capabilities"`
	Properties map[string]any `json:"properties"` // REQ-429/M225：对端自定义属性（成本档等如实透出）
}

// RemoteTask 远端任务快照（tasks/get 轮询产物）。
type RemoteTask struct {
	ID           string           `json:"id"`
	ContextID    string           `json:"contextId"`
	State        string           `json:"state"`
	ArtifactText string           // artifacts text parts 拼接
	ErrorText    string           // status.message / 终态错误
	History      []HistoryMessage // REQ-428/M224：任务消息历史（role+text parts 拼接；过程转译源）
}

// HistoryMessage 任务历史单条消息（A2A Message：role=agent|user；text parts 拼接）。
type HistoryMessage struct {
	Role string
	Text string
}

// cardURL 由连接目标推导卡地址：endpoint 或其基址 + /.well-known/agent-card.json
// （已是卡地址则原样）。
func cardURL(endpoint string) string {
	e := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if strings.HasSuffix(e, "/.well-known/agent-card.json") {
		return e
	}
	return e + "/.well-known/agent-card.json"
}

// FetchAgentCard 注册握手：拉取并解析 Agent Card（不可达/非卡片结构即失败——注册闸门）。
func FetchAgentCard(ctx context.Context, endpoint string, headers map[string]string, timeout time.Duration) (*AgentCard, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cardURL(endpoint), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("卡地址构造失败: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("agent card 不可达: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, nil, fmt.Errorf("agent card 拉取失败（HTTP %d）: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("agent card 读取失败: %w", err)
	}
	var card AgentCard
	if err := json.Unmarshal(raw, &card); err != nil {
		return nil, nil, fmt.Errorf("agent card 解析失败（非卡片结构）: %w", err)
	}
	if strings.TrimSpace(card.Name) == "" {
		return nil, nil, fmt.Errorf("agent card 缺 name 字段（非 A2A 卡片）")
	}
	return &card, raw, nil
}

// rpcDo JSON-RPC 2.0 调用（与 server 端同形态；error 响应转 Go error）。
func rpcDo(ctx context.Context, endpoint string, headers map[string]string, method string, params any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("A2A 端点不可达: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("A2A 响应读取失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("A2A 端点 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("A2A 响应非 JSON-RPC 形态: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("A2A 错误 %d: %s", out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}

// taskStateOf wire task → RemoteTask（artifacts text parts 拼接；status.message 承载错误）。
func taskStateOf(result json.RawMessage) (*RemoteTask, error) {
	var t struct {
		ID        string `json:"id"`
		ContextID string `json:"contextId"`
		Status    struct {
			State   string `json:"state"`
			Message *struct {
				Parts []struct {
					Kind string `json:"kind"`
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"message"`
		} `json:"status"`
		Artifacts []struct {
			Parts []struct {
				Kind string `json:"kind"`
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"artifacts"`
		History []struct {
			Role  string `json:"role"`
			Parts []struct {
				Kind string `json:"kind"`
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"history"`
	}
	if err := json.Unmarshal(result, &t); err != nil {
		return nil, fmt.Errorf("task 解析失败: %w", err)
	}
	rt := &RemoteTask{ID: t.ID, ContextID: t.ContextID, State: t.Status.State}
	var b strings.Builder
	for _, a := range t.Artifacts {
		for _, p := range a.Parts {
			if p.Kind == "text" || p.Kind == "" {
				b.WriteString(p.Text)
			}
		}
	}
	rt.ArtifactText = b.String()
	if t.Status.Message != nil {
		var mb strings.Builder
		for _, p := range t.Status.Message.Parts {
			if p.Kind == "text" || p.Kind == "" {
				mb.WriteString(p.Text)
			}
		}
		rt.ErrorText = strings.TrimSpace(mb.String())
	}
	// REQ-428/M224：任务消息历史转译源（role+text parts 拼接，顺序保留供过程回放）
	for _, h := range t.History {
		var hb strings.Builder
		for _, p := range h.Parts {
			if p.Kind == "text" || p.Kind == "" {
				hb.WriteString(p.Text)
			}
		}
		rt.History = append(rt.History, HistoryMessage{Role: h.Role, Text: hb.String()})
	}
	return rt, nil
}

// SendStream REQ-432/M228①：message/stream SSE 订阅委派（对端 capabilities.streaming=true 时
// 替代纯轮询）——每条 SSE data 为 JSON-RPC 响应（result=任务快照），逐快照合并回调 onEvent
// （过程转译源），连接关闭即终态返回。对端不支持/中途失败由调用方回落 send+轮询。
func SendStream(ctx context.Context, endpoint string, headers map[string]string, task string, onEvent func(*RemoteTask)) (*RemoteTask, error) {
	params := map[string]any{
		"message": map[string]any{
			"role":      "user",
			"kind":      "message",
			"messageId": fmt.Sprintf("m-%d", time.Now().UnixNano()),
			"parts":     []map[string]any{{"kind": "text", "text": task}},
		},
	}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": time.Now().UnixNano(), "method": "message/stream", "params": params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return nil, fmt.Errorf("对端不支持 message/stream（HTTP %d %s）", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var final *RemoteTask
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var env struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(payload), &env) != nil {
			continue
		}
		if env.Error != nil {
			return final, fmt.Errorf("message/stream 错误: %s", env.Error.Message)
		}
		rt, err := taskStateOf(env.Result)
		if err != nil || rt == nil {
			continue
		}
		if onEvent != nil {
			onEvent(rt)
		}
		final = rt
		if done, _ := taskTerminal(rt.State); done {
			break
		}
	}
	if final == nil {
		return nil, fmt.Errorf("message/stream 无任务快照返回")
	}
	return final, nil
}

// Send message/send 委派（立即回任务——通常 working 态；终态经 AwaitTerminal 轮询）。
func Send(ctx context.Context, endpoint string, headers map[string]string, task string) (*RemoteTask, error) {
	params := map[string]any{
		"message": map[string]any{
			"role":      "user",
			"kind":      "message",
			"messageId": fmt.Sprintf("m-%d", time.Now().UnixNano()),
			"parts":     []map[string]any{{"kind": "text", "text": task}},
		},
	}
	result, err := rpcDo(ctx, endpoint, headers, "message/send", params)
	if err != nil {
		return nil, err
	}
	return taskStateOf(result)
}

// TasksGet 单次任务快照。
func TasksGet(ctx context.Context, endpoint string, headers map[string]string, taskID string) (*RemoteTask, error) {
	result, err := rpcDo(ctx, endpoint, headers, "tasks/get", map[string]any{"id": taskID})
	if err != nil {
		return nil, err
	}
	return taskStateOf(result)
}

// CancelTask REQ-441/M237：远端任务取消（best-effort——委派超时/打断时止住远端白跑；
// 远端已终态回 TaskNotCancelable 属正常响应不放大）。
func CancelTask(ctx context.Context, endpoint string, headers map[string]string, taskID string) error {
	_, err := rpcDo(ctx, endpoint, headers, "tasks/cancel", map[string]any{"id": taskID})
	return err
}

// taskTerminal 终态判定（A2A 状态机：submitted/working 非终态；input-required 无交互通道按失败如实报）。
func taskTerminal(state string) (done bool, failed bool) {
	switch state {
	case "completed":
		return true, false
	case "failed", "canceled", "rejected", "input-required":
		return true, true
	}
	return false, false
}

// AwaitTerminal 轮询至终态（interval 上钳 30s 下钳 200ms；ctx 取消/超时即停）。
func AwaitTerminal(ctx context.Context, endpoint string, headers map[string]string, taskID string, interval time.Duration) (*RemoteTask, error) {
	if interval < 200*time.Millisecond {
		interval = 200 * time.Millisecond
	}
	if interval > 30*time.Second {
		interval = 30 * time.Second
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var last *RemoteTask
	for {
		rt, err := TasksGet(ctx, endpoint, headers, taskID)
		if err != nil {
			return nil, err
		}
		last = rt
		done, failed := taskTerminal(rt.State)
		if done {
			if failed {
				detail := rt.ErrorText
				if detail == "" {
					detail = "远端任务终态 " + rt.State
				}
				return rt, fmt.Errorf("远端任务 %s（%s）", rt.State, detail)
			}
			return rt, nil
		}
		select {
		case <-ctx.Done():
			return last, fmt.Errorf("等待远端任务终态中断: %w", ctx.Err())
		case <-tick.C:
		}
	}
}
