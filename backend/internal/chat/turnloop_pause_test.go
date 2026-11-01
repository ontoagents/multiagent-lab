package chat

// REQ-558/M365 pause 安全点挂起单测（107 号 §9.4 验收①②钉桩）：运行中 Pause →
// run.finished reason=paused + pause_state 落库（state_log 留痕）→ ResumePaused 无答复
// 沿 checkpoint 续跑（重启可恢复语义——checkpoint 落库链承载，同键装载）→ 新 runID 收束。

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// tlPauseModel pause 桩（沿 tlSteerModel 形态）：未见恢复指令前持续调 gate 工具（制造活跃轮）；
// 输入含纠偏指令即收束作答。
type tlPauseModel struct {
	sawResume atomic.Bool
}

func (m *tlPauseModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	for _, msg := range input {
		if msg.Role == schema.User && strings.Contains(msg.Content, "按纠偏指令收尾") {
			m.sawResume.Store(true)
			return schema.AssistantMessage("已按纠偏指令收尾", nil), nil
		}
	}
	return schema.AssistantMessage("", []schema.ToolCall{
		{ID: fmt.Sprintf("call_%d", time.Now().UnixNano()), Function: schema.FunctionCall{Name: "tl_gate", Arguments: `{}`}},
	}), nil
}

func (m *tlPauseModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("stub model: stream not implemented")
}

func TestTurnLoopPauseResume_Req558(t *testing.T) {
	s := tlService(t)
	var rt *BuildResult
	s.assembleOverride = func(ctx context.Context, agent *store.Agent, conv *store.Conversation) (*BuildResult, error) {
		return rt, nil // stub 模型直注（与首段运行同一 agent——桩行为一致；调用时 rt 已赋值）
	}
	conv, ag := tlConv(t, s)
	m := &tlPauseModel{}
	gate := make(chan struct{})
	rt = &BuildResult{Agent: tlAgent(t, m, tlGateTool(t, gate)), AgentName: "tl-root"}

	runCtx := context.Background()
	start := time.Now()
	runID := "tlr_pause"
	rc := newRunConsumer(s, runCtx, conv, runID, rt, func(*Event) {}, start, false, nil)
	if _, err := s.Store.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: "开始长任务"}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTurnLoop(runCtx, rc, conv, ag, rt, runID, checkPointIDOf(runID),
			[]*schema.Message{schema.UserMessage("开始长任务")}, nil, &obsHandler{}, true)
	}()

	// 等运行句柄登记（轮内活跃）后 pause 抢占，再放行阻塞工具 → AfterToolCalls 安全点生效
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		h := s.loops[conv.ID]
		s.mu.Unlock()
		if h != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("运行句柄未登记（TurnLoop 未启动）")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond) // 等首轮进入工具执行（阻塞中）
	ok, err := s.Pause(conv.ID)
	if err != nil || !ok {
		t.Fatalf("Pause 失败: ok=%v err=%v", ok, err)
	}
	close(gate) // 放行工具 → 安全点收束（Stop 落 checkpoint）
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TurnLoop 未在 pause 后收束")
	}
	// ① 终态=paused（非 stopped/completed 误标）
	if !rc.paused {
		t.Fatalf("终态应 paused: stopped=%v interrupted=%v err=%q", rc.stopped, rc.interrupted, rc.runErr)
	}
	// ② pause_state 落库（验收①；checkpoint_id 指向本 run 锚点）
	conv2, _ := s.Store.GetConversation(conv.ID)
	if conv2 == nil || conv2.PauseState == "" {
		t.Fatalf("pause_state 未落库: %+v", conv2)
	}
	if !strings.Contains(conv2.PauseState, checkPointIDOf(runID)) {
		t.Fatalf("pause_state 应含 checkpoint_id: %s", conv2.PauseState)
	}
	// ③ state_log 留痕（recordStateChange 单源口径）
	logs, lerr := s.Store.ListConversationStateLog(conv.ID, 20)
	if lerr != nil {
		t.Fatalf("state_log 读取失败: %v", lerr)
	}
	found := false
	for _, lg := range logs {
		if lg.Field == "pause_state" {
			found = true
		}
	}
	if !found {
		t.Fatal("state_log 应有 pause_state 留痕")
	}
	// ④ ResumePaused 无答复沿 checkpoint 续跑（新 runID；重启可恢复语义由 checkpoint 落库链承载）
	if _, err := s.Store.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: "按纠偏指令收尾"}); err != nil {
		t.Fatal(err)
	}
	newID, res, rerr := s.ResumePaused(runCtx, conv2, ag, "", 0, false, func(*Event) {})
	if rerr != nil {
		t.Fatalf("ResumePaused 失败: %v", rerr)
	}
	if newID == "" || newID == runID {
		t.Fatalf("恢复应开新 runID（拒绝复活旧 run）: %q", newID)
	}
	if res.Error != "" {
		t.Fatalf("恢复运行报错: %s", res.Error)
	}
	conv3, _ := s.Store.GetConversation(conv.ID)
	if conv3.PauseState != "" {
		t.Fatalf("unpause 后 pause_state 应清空: %q", conv3.PauseState)
	}
	time.Sleep(200 * time.Millisecond)
	msgs, _ := s.Store.ListMessages(conv.ID)
	var assistant string
	for _, msg := range msgs {
		if msg.Role == "assistant" {
			assistant += msg.Content
		}
	}
	if !strings.Contains(assistant, "已按纠偏指令收尾") && !m.sawResume.Load() {
		t.Fatalf("恢复续跑应收束作答: %q", assistant)
	}
	_ = res
}
