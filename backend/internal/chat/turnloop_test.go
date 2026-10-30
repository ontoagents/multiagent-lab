package chat

// REQ-385/M181 C3 LongRun 单测：TurnLoop 引擎接入面（runTurnLoop）——
//   - TestTurnLoopBasicRun：单轮直答——产物即刷落库/事件消费/无中断（与旧路径行为等价）；
//   - TestTurnLoopSteerPreempt：运行中 steer 抢占——Push(WithPreempt) 安全点打断当前轮、
//     下一轮以全量历史（含插话）接续（steer.applied 消费点可观测）；
//   - TestTurnLoopInterruptResume：审批挂起 → GenResume 定向恢复（ResumeParams 同构
//     Runner.ResumeWithParams，checkpoint 键=挂起锚点）。
// 桩模型直驱（沿 interrupt_test.go stub 形态）；Service 手工装配（Store+Assembler.CheckPoints）。

import (
	"context"
	"errors"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/ontoagents/ontoagents/backend/internal/store"
	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

// tlService 手工装配的最小 Service（TurnLoop 引擎直测；桩模型只实现 Generate→关流式）。
func tlService(t *testing.T) *Service {
	t.Helper()
	turnLoopStreamingDefault = false
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	asm := &Assembler{CheckPoints: NewMemCheckPointStore()}
	s := NewService(st, asm, nil)
	return s
}

// tlConv 测试会话（scope=agent 直聊形态；buildRunContext 走 ListMessages）。
func tlConv(t *testing.T, s *Service) (*store.Conversation, *store.Agent) {
	t.Helper()
	ag, err := s.Store.CreateAgent(&store.Agent{Name: "tl-agent"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := s.Store.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID, Title: "tl"})
	if err != nil {
		t.Fatal(err)
	}
	return conv, ag
}

// tlAgent 用桩模型构造 ChatModelAgent（工具可选）。
func tlAgent(t *testing.T, m model.BaseChatModel, tools ...einotool.BaseTool) adk.Agent {
	t.Helper()
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name: "tl-root", Model: m,
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

// TestTurnLoopBasicRun 单轮直答：助手产物轮界即刷落库（flushTurnAssistant）+运行收束无中断。
func TestTurnLoopBasicRun(t *testing.T) {
	s := tlService(t)
	conv, ag := tlConv(t, s)
	m := tlFinalModel{answer: "直答完成"}
	rt := &BuildResult{Agent: tlAgent(t, m), AgentName: "tl-root"}

	runCtx := context.Background()
	start := time.Now()
	rc := newRunConsumer(s, runCtx, conv, "tlr_basic", rt, func(*Event) {}, start, false)
	// 用户消息先落库（runOnce 语义；GenInput 首轮历史从 DB 重建仅在 steer 轮发生）
	if _, err := s.Store.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: "问题"}); err != nil {
		t.Fatal(err)
	}
	exit := s.runTurnLoop(runCtx, rc, conv, ag, rt, "tlr_basic", checkPointIDOf("tlr_basic"),
		[]*schema.Message{schema.UserMessage("问题")}, nil, &obsHandler{}, true)
	if exit != nil && exit.ExitReason != nil {
		t.Fatalf("期望干净退出，实得 %v", exit.ExitReason)
	}
	if rc.interrupted || rc.runErr != "" {
		t.Fatalf("不应中断/报错: interrupted=%v err=%q", rc.interrupted, rc.runErr)
	}
	msgs, _ := s.Store.ListMessages(conv.ID)
	var assistant string
	for _, msg := range msgs {
		if msg.Role == "assistant" {
			assistant += msg.Content
		}
	}
	if !strings.Contains(assistant, "直答完成") {
		t.Fatalf("助手产物应已轮界落库，实得 %q", assistant)
	}
}

// TestTurnLoopSteerPreempt 运行中抢占：当前轮在工具完成后（AfterToolCalls 安全点）被打断 →
// 下一轮以含插话的历史接续 → 收束。tl_gate 工具阻塞至插话落定（制造活跃轮+确定性安全点）。
func TestTurnLoopSteerPreempt(t *testing.T) {
	s := tlService(t)
	conv, ag := tlConv(t, s)
	m := &tlSteerModel{}
	gate := make(chan struct{})
	rt := &BuildResult{Agent: tlAgent(t, m, tlGateTool(t, gate)), AgentName: "tl-root"}

	runCtx := context.Background()
	start := time.Now()
	rc := newRunConsumer(s, runCtx, conv, "tlr_steer", rt, func(*Event) {}, start, false)
	if _, err := s.Store.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: "开始长任务"}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTurnLoop(runCtx, rc, conv, ag, rt, "tlr_steer", checkPointIDOf("tlr_steer"),
			[]*schema.Message{schema.UserMessage("开始长任务")}, nil, &obsHandler{}, true)
	}()

	// 等运行句柄登记（轮内活跃）后插话抢占，再放行阻塞工具 → AfterToolCalls 安全点生效
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
	ok, err := s.Steer(conv.ID, "换个方向：直接收尾")
	if err != nil || !ok {
		t.Fatalf("Steer 失败: ok=%v err=%v", ok, err)
	}
	close(gate) // 放行工具 → 安全点抢占
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TurnLoop 未在抢占后收束")
	}
	if rc.interrupted || rc.runErr != "" {
		t.Fatalf("抢占后应正常收束: interrupted=%v err=%q", rc.interrupted, rc.runErr)
	}
	if rc.stopped {
		t.Fatal("抢占续跑后运行不应被误标 stopped（终态保真）")
	}
	if !m.sawSteer.Load() {
		t.Fatal("下一轮模型输入应含插话内容（全量历史接续）")
	}
}

// TestTurnLoopInterruptResume 审批挂起 → GenResume 定向恢复 → 工具执行收束。
func TestTurnLoopInterruptResume(t *testing.T) {
	s := tlService(t)
	conv, ag := tlConv(t, s)
	reg := tool.NewRegistry()
	if err := tool.RegisterBuiltin(reg); err != nil {
		t.Fatal(err)
	}
	composed, err := reg.Compose(context.Background(), []string{"current_time"})
	if err != nil {
		t.Fatal(err)
	}
	wrapped, ok := tool.NewApprovalTool(composed.Tools[0], "current_time", nil)
	if !ok {
		t.Fatal("wrap approval failed")
	}
	rt := &BuildResult{Agent: tlAgent(t, tlEchoAfterToolModel{}, wrapped), AgentName: "tl-root"}
	runCtx := context.Background()
	start := time.Now()
	rc := newRunConsumer(s, runCtx, conv, "tlr_intr", rt, func(*Event) {}, start, false)
	if _, err := s.Store.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: "现在几点"}); err != nil {
		t.Fatal(err)
	}
	ckpt := checkPointIDOf("tlr_intr")
	s.runTurnLoop(runCtx, rc, conv, ag, rt, "tlr_intr", ckpt,
		[]*schema.Message{schema.UserMessage("现在几点")}, nil, &obsHandler{}, true)
	if !rc.interrupted {
		t.Fatal("审批工具应触发挂起中断")
	}
	if conv.InterruptState == "" {
		var st interruptState
		_ = st
	}
	// 读取挂起状态取 TargetID（handleInterrupted 已落库）
	var stt interruptState
	row, err := s.Store.GetConversation(conv.ID)
	if err != nil || row.InterruptState == "" {
		t.Fatalf("挂起状态应已落库: %v", err)
	}
	if err := json.Unmarshal([]byte(row.InterruptState), &stt); err != nil {
		t.Fatal(err)
	}
	// 定向恢复（GenResume 路径）
	rc2 := newRunConsumer(s, runCtx, conv, "tlr_intr2", rt, func(*Event) {}, time.Now(), false)
	exit := s.runTurnLoop(runCtx, rc2, conv, ag, rt, "tlr_intr2", stt.CheckpointID, nil,
		&adk.ResumeParams{Targets: map[string]any{stt.TargetID: tool.ApprovalApprove}}, &obsHandler{}, true)
	if exit != nil && exit.ExitReason != nil {
		t.Fatalf("恢复轮应干净退出，实得 %v", exit.ExitReason)
	}
	if rc2.interrupted || rc2.runErr != "" {
		t.Fatalf("恢复后不应再中断/报错: %v %q", rc2.interrupted, rc2.runErr)
	}
}

// ---- 桩模型与工具 ----

// tlFinalModel 直答桩：首轮即返回最终答案。
type tlFinalModel struct{ answer string }

func (m tlFinalModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage(m.answer, nil), nil
}
func (m tlFinalModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("stub model: stream not implemented")
}

// tlSteerModel 抢占桩：未见插话前持续调 gate 工具（制造活跃轮）；输入含插话即收束作答。
type tlSteerModel struct {
	sawSteer syncAtomicBool
}

func (m *tlSteerModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	for _, msg := range input {
		if msg.Role == schema.User && strings.Contains(msg.Content, "换个方向") {
			m.sawSteer.Store(true)
			return schema.AssistantMessage("已按插话收尾", nil), nil
		}
	}
	return schema.AssistantMessage("", []schema.ToolCall{
		{ID: fmt.Sprintf("call_%d", time.Now().UnixNano()), Function: schema.FunctionCall{Name: "tl_gate", Arguments: `{}`}},
	}), nil
}
func (m *tlSteerModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("stub model: stream not implemented")
}

// tlEchoAfterToolModel 审批链桩：工具结果回喂后转述收束。
type tlEchoAfterToolModel struct{}

func (tlEchoAfterToolModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	for i := len(input) - 1; i >= 0; i-- {
		if input[i].Role == schema.Tool {
			return schema.AssistantMessage("工具结果："+input[i].Content, nil), nil
		}
	}
	return schema.AssistantMessage("", []schema.ToolCall{
		{ID: "call_t", Function: schema.FunctionCall{Name: "current_time", Arguments: `{}`}},
	}), nil
}
func (tlEchoAfterToolModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("stub model: stream not implemented")
}

// syncAtomicBool 极简布尔原子（测试专用，避 sync/atomic 泛型歧义）。
type syncAtomicBool struct {
	mu sync.Mutex
	v  bool
}

func (b *syncAtomicBool) Store(v bool) { b.mu.Lock(); b.v = v; b.mu.Unlock() }
func (b *syncAtomicBool) Load() bool   { b.mu.Lock(); defer b.mu.Unlock(); return b.v }

// tlGateTool 阻塞门工具（TurnLoop 抢占测试）：阻塞至 gate 关闭——制造确定性活跃轮，
// 插话落定后放行 → AfterToolCalls 安全点抢占生效。
func tlGateTool(t *testing.T, gate chan struct{}) einotool.BaseTool {
	t.Helper()
	tp, err := toolutils.InferTool("tl_gate", "阻塞门工具（TurnLoop 抢占测试）",
		func(_ context.Context, _ struct{}) (string, error) {
			<-gate
			return "gate-open", nil
		})
	if err != nil {
		t.Fatal(err)
	}
	return tp
}

// TestTurnLoopRecallPersisted REQ-516/M323（P03 落库侧）：召回注入即落库（system 角色）——
// persistRecallInjections 把注入文本写入 store，ListMessages 可见且 meta 记来源。
func TestTurnLoopRecallPersisted(t *testing.T) {
	s := tlService(t)
	conv, _ := tlConv(t, s)
	s.persistRecallInjections(context.Background(), conv, "tlr_recall", "kb",
		[]*schema.Message{schema.SystemMessage("REQ516 召回注入文本：kb 命中片段")}, func(*Event) {})
	msgs, err := s.Store.ListMessages(conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "REQ516 召回注入文本") {
			found = true
			if !strings.Contains(m.Meta, "recall_source") {
				t.Fatalf("system 消息应记 recall_source 溯源: meta=%q", m.Meta)
			}
		}
	}
	if !found {
		t.Fatalf("召回注入应落库为 system 消息（steer 续轮全量重建才不丢上下文）: %d 条", len(msgs))
	}
}

// tlRecallModel steer 抢占桩：首轮阻塞工具等抢占；续轮全量历史应含落库召回文本（system 原样保留）。
type tlRecallModel struct {
	sawSteer  syncAtomicBool
	sawRecall syncAtomicBool
}

func (m *tlRecallModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	steer, recall := false, false
	for _, msg := range input {
		if msg.Role == schema.User && strings.Contains(msg.Content, "换个方向") {
			steer = true
		}
		if msg.Role == schema.System && strings.Contains(msg.Content, "REQ516 落库召回文本") {
			recall = true
		}
	}
	if steer {
		m.sawSteer.Store(true)
		if recall {
			m.sawRecall.Store(true)
		}
		return schema.AssistantMessage("已按插话收尾", nil), nil
	}
	return schema.AssistantMessage("", []schema.ToolCall{
		{ID: fmt.Sprintf("call_%d", time.Now().UnixNano()), Function: schema.FunctionCall{Name: "tl_gate", Arguments: `{}`}},
	}), nil
}
func (m *tlRecallModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("stub model: stream not implemented")
}

// TestTurnLoopSteerSeesPersistedRecall REQ-516/M323（P03 重建可见侧）：注入落库的 system 消息经
// steer 轮 buildRunContext 全量重建（mergeToolTurns 对 system 原样保留）进入续轮模型输入——
// 修复「抢占轮静默丢失本轮召回上下文」。
func TestTurnLoopSteerSeesPersistedRecall(t *testing.T) {
	s := tlService(t)
	conv, ag := tlConv(t, s)
	m := &tlRecallModel{}
	gate := make(chan struct{})
	rt := &BuildResult{Agent: tlAgent(t, m, tlGateTool(t, gate)), AgentName: "tl-root"}

	if _, err := s.Store.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: "开始长任务"}); err != nil {
		t.Fatal(err)
	}
	// 模拟 runOnce 已把召回注入落库（persistRecallInjections 产物——落库侧另有单测覆盖）
	if _, err := s.Store.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "system", Content: "REQ516 落库召回文本：kb 命中片段", Meta: `{"recall_source":"kb"}`}); err != nil {
		t.Fatal(err)
	}

	runCtx := context.Background()
	start := time.Now()
	rc := newRunConsumer(s, runCtx, conv, "tlr_recall_steer", rt, func(*Event) {}, start, false)
	done := make(chan struct{})
	go func() {
		defer close(done)
		// 首轮输入不含 system（runOnce 内存态）——落库召回只在 store，续轮重建才可见
		s.runTurnLoop(runCtx, rc, conv, ag, rt, "tlr_recall_steer", checkPointIDOf("tlr_recall_steer"),
			[]*schema.Message{schema.UserMessage("开始长任务")}, nil, &obsHandler{}, true)
	}()

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
	time.Sleep(300 * time.Millisecond)
	ok, err := s.Steer(conv.ID, "换个方向：直接收尾")
	if err != nil || !ok {
		t.Fatalf("Steer 失败: ok=%v err=%v", ok, err)
	}
	close(gate)
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TurnLoop 未在抢占后收束")
	}
	if !m.sawSteer.Load() {
		t.Fatal("续轮模型输入应含插话（全量历史接续）")
	}
	if !m.sawRecall.Load() {
		t.Fatal("续轮模型输入应含落库召回 system 文本（P03 修复点——抢占轮不丢检索上下文）")
	}
}

// TestAcquireRunGateAtomic REQ-521/M328（P22 并发闸）：检查+占位同锁段原子化——并发 acquire
// 同会话恰一成功其余 ErrConversationBusy（handler 层映射 409）。
func TestAcquireRunGateAtomic(t *testing.T) {
	s := tlService(t)
	conv, _ := tlConv(t, s)
	const n = 20
	okCh := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := s.acquireRun(conv.ID); err == nil {
				okCh <- i
			}
		}(i)
	}
	wg.Wait()
	close(okCh)
	wins := len(okCh)
	if wins != 1 {
		t.Fatalf("并发 acquire 应恰一成功，got %d", wins)
	}
	if err := s.acquireRun(conv.ID); !errors.Is(err, ErrConversationBusy) {
		t.Fatalf("占位期间再 acquire 应 busy: %v", err)
	}
	s.releaseRun(conv.ID)
	if err := s.acquireRun(conv.ID); err != nil {
		t.Fatalf("释放后应可再获: %v", err)
	}
	s.releaseRun(conv.ID)
}

// TestRemoveLoopHandleByValue REQ-521/M328（P22 句柄按值删除）：双运行同键覆盖时，
// 先完成者的按值删除不得摘掉后运行者的句柄（原按键删除致 steer 409）。
func TestRemoveLoopHandleByValue(t *testing.T) {
	s := tlService(t)
	conv, _ := tlConv(t, s)
	h1 := &loopHandle{runID: "r1"}
	h2 := &loopHandle{runID: "r2"}
	s.mu.Lock()
	s.loops[conv.ID] = h2 // 后运行者已覆盖登记
	s.mu.Unlock()
	s.removeLoopHandle(conv.ID, h1) // 先完成者退出（持旧句柄）——不得误删
	s.mu.Lock()
	got := s.loops[conv.ID]
	s.mu.Unlock()
	if got != h2 {
		t.Fatal("先完成者 defer 不得摘掉后运行者的句柄（按值删除修复点）")
	}
	s.removeLoopHandle(conv.ID, h2) // 后运行者退出——正确删除
	s.mu.Lock()
	got = s.loops[conv.ID]
	s.mu.Unlock()
	if got != nil {
		t.Fatal("本人退出应删除自己的句柄")
	}
}
