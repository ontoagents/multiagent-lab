package chat

// REQ-385/M181 C3 LongRun——TurnLoop push-based 原语采用（2026-10-29 专项重构轮）：
// Eino adk.TurnLoop 作为运行循环引擎接入 runOnce/Resume 主链（55 号开放 5 评估拍板「采用
// 价值成立，实施留专项重构轮」的落地）：
//   - 长任务有界增量：轮间 checkpoint（item bookkeeping）+ 每轮助手输出即刷即持久化
//     （flushTurnAssistant），中断/抢占不重放已做步；
//   - 抢占式循环：主会话运行中介入 steer——SteerRun 经 TurnLoop.Push(WithPreempt(AnySafePoint))
//     在安全点（AfterChatModel/AfterToolCalls）打断当前轮、下一轮以全量历史（含插话）接续；
//     steer.pushed/run.preempted/steer.applied 三事件使 steer 生效时点可观测；
//   - checkpoint 衔接：Store=既有 CheckPoints、CheckpointID=checkPointIDOf(runID) 由 TurnLoop
//     自动注入；ask_human/approval 挂起经 ExitState.InterruptError 语义与既有 handleInterrupted
//     兼容；Resume 收敛为 GenResume（checkpoint 含 runner 状态时框架回调，ResumeParams 定向恢复
//     与既有 Runner.ResumeWithParams 同构）；
//   - delegate 汇流：agent_as_tool 子调用异步化走 sub-agent 配置开关（delegate_bg 底盘，见
//     assembler.go delegateAsyncTool）。
// 门禁：env CHAT_TURNLOOP=0 回滚旧路径（默认开——真机冒烟+单测双绿后默认启用）。
// 诚实边界：①steer 轮以全量历史重建上下文（召回链不重跑——召回面向原始提问，插话为引导语义）；
// ②抢占在安全点生效（token 级中断不支持，Eino 框架语义）；③工作流 Runner（sequential/parallel/
// loop）保持旧路径（TurnLoop 收益在长任务智能体轮，工作流编排自有循环语义，不叠加）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// runItem TurnLoop 条目（gob 编码入 checkpoint——字段保持可 gob 基本类型）。
type runItem struct {
	Content string // steer 插话文本（kind=steer）；start 条目为原始输入（历史已含，仅作标记）
	Kind    string // start | steer
}

// turnLoopEnabled TurnLoop 引擎开关（默认开；CHAT_TURNLOOP=0 回滚 adk.Runner 直跑路径）。
func turnLoopEnabled() bool { return os.Getenv("CHAT_TURNLOOP") != "0" }

// turnLoopStreamingDefault TurnLoop 轮内流式开关（生产恒开——SSE 增量语义与旧 Runner 路径一致；
// 测试桩模型只实现 Generate 时由测试置 false。包级变量=测试注入点，生产不改）。
var turnLoopStreamingDefault = true

// loopHandle 运行中 TurnLoop 句柄（SteerRun 抢占介入入口）。
type loopHandle struct {
	loop  *adk.TurnLoop[runItem, *schema.Message]
	runID string
	// emit 该运行的实时事件通道（REQ-516/M323 P26①：steer.pushed 此前经 no-op emit 只落库
	// 不进实时 SSE——「三事件生效时点可观测」承诺在 steer.pushed 一环断链）
	emit EmitFn
}

// Steer 主会话运行中介入（REQ-385/M181 steer 深化——轮界接续→运行中抢占）：插话落消息流，
// TurnLoop.Push(WithPreempt(AnySafePoint)) 在安全点打断当前轮、下一轮以全量历史（含插话）接续。
// 返回 (false, nil)=无进行中 TurnLoop 运行（调用方 409，走常规新消息）。
func (s *Service) Steer(convID, message string) (bool, error) {
	if message == "" {
		return false, fmt.Errorf("插话内容为空")
	}
	s.mu.Lock()
	h := s.loops[convID]
	s.mu.Unlock()
	if h == nil {
		return false, nil
	}
	conv, err := s.Store.GetConversation(convID)
	if err != nil {
		return true, fmt.Errorf("会话不存在: %w", err)
	}
	// 插话先落消息流（下一轮 buildRunContext 全量历史可见——与 delegate Steer 同口径）
	if _, err := s.Store.InsertMessage(&store.Message{ConversationID: convID, Role: "user", Content: message}); err != nil {
		return true, fmt.Errorf("save steer message: %w", err)
	}
	ok, _ := h.loop.Push(runItem{Content: message, Kind: "steer"}, adk.WithPreempt[runItem, *schema.Message](adk.AnySafePoint))
	// REQ-516/M323（P26①/P26③）：steer.pushed 走该运行的 rc.emit 实时可达（原 no-op 只落库）；
	// late=true 标注运行已收束后到达的插话（Push 返回 false——内容已落消息流，重新发送即可）
	s.emitAndRecord(context.Background(), conv, h.runID, newEvent("steer.pushed", h.runID, map[string]any{
		"preempt": ok, "late": !ok, "note": runeforTitle(message, 100),
	}), h.emit)
	return true, nil
}

// emitRunEvent 服务级事件透出（SSE+落库同源；steer 入口无 run consumer 时用）。
func (s *Service) emitRunEvent(convID, runID, typ string, data map[string]any, emit EmitFn) {
	if emit == nil {
		emit = func(*Event) {}
	}
	s.emitAndRecord(context.Background(), &store.Conversation{ID: convID}, runID, newEvent(typ, runID, data), emit)
}

// runTurnLoop TurnLoop 驱动的运行循环（runOnce/Resume 共用执行面；rc 消费/收尾由调用方负责）。
// checkpointID=checkpoint 键（runOnce=checkPointIDOf(runID)；Resume=挂起锚点 st.CheckpointID——
// 框架按此键查 checkpoint，含 runner 状态时回调 GenResume 定向恢复）。
// 首轮输入=histMsgs（runOnce 装配产物，含用户消息）；steer 轮=buildRunContext 全量重建。
// steerable=登记运行句柄供 steerRun 抢占介入（对比窗格多路并行不登记——语义单路）。
func (s *Service) runTurnLoop(runCtx context.Context, rc *runConsumer, conv *store.Conversation, agent *store.Agent, rt *BuildResult, runID, checkpointID string, histMsgs []*schema.Message, resumeParams *adk.ResumeParams, obs *obsHandler, steerable bool) *adk.TurnLoopExitState[runItem, *schema.Message] {
	firstTurn := true
	var turnMu sync.Mutex
	loop := adk.NewTurnLoop[runItem, *schema.Message](adk.TurnLoopConfig[runItem, *schema.Message]{
		Store:        s.Assembler.CheckPoints,
		CheckpointID: checkpointID,
		GenInput: func(ctx context.Context, l *adk.TurnLoop[runItem, *schema.Message], items []runItem) (*adk.GenInputResult[runItem, *schema.Message], error) {
			turnMu.Lock()
			first := firstTurn
			turnMu.Unlock()
			if first {
				// 首轮：全量装配历史为输入（items 仅为启动标记，历史已含用户消息）
				turnMu.Lock()
				firstTurn = false
				turnMu.Unlock()
				return &adk.GenInputResult[runItem, *schema.Message]{
					Input:    &adk.AgentInput{Messages: histMsgs, EnableStreaming: turnLoopStreamingDefault},
					Consumed: items,
					RunOpts:  []adk.AgentRunOption{adk.WithCallbacks(obs)},
				}, nil
			}
			// steer 轮：插话消息已由 SteerRun 落库——全量历史重建（含插话与上轮部分产物）
			cr, err := s.buildRunContext(ctx, conv, agent)
			if err != nil {
				return nil, fmt.Errorf("steer 轮历史重建失败: %w", err)
			}
			return &adk.GenInputResult[runItem, *schema.Message]{
				Input:    &adk.AgentInput{Messages: cr.Messages, EnableStreaming: turnLoopStreamingDefault},
				Consumed: items,
				RunOpts:  []adk.AgentRunOption{adk.WithCallbacks(obs)},
			}, nil
		},
		GenResume: func(ctx context.Context, l *adk.TurnLoop[runItem, *schema.Message], interruptedItems, unhandledItems, newItems []runItem) (*adk.GenResumeResult[runItem, *schema.Message], error) {
			return &adk.GenResumeResult[runItem, *schema.Message]{
				ResumeParams: resumeParams, // 挂起答复定向恢复（ask_human/approval，与 Runner.ResumeWithParams 同构）
				Consumed:     append(append(append([]runItem{}, interruptedItems...), unhandledItems...), newItems...),
				RunOpts:      []adk.AgentRunOption{adk.WithCallbacks(obs)},
			}, nil
		},
		PrepareAgent: func(ctx context.Context, l *adk.TurnLoop[runItem, *schema.Message], consumed []runItem) (adk.TypedAgent[*schema.Message], error) {
			if rt.Agent == nil {
				return nil, fmt.Errorf("装配产物缺 Agent（TurnLoop 需要裸 Agent 形态）")
			}
			if len(consumed) > 0 && consumed[0].Kind == "steer" {
				s.emitRunEvent(conv.ID, runID, "steer.applied", map[string]any{
					"turn_items": len(consumed),
					// REQ-516/M323（P03）：标注召回上下文去留——注入已落库，全量重建历史可见
					"recall_in_store": true,
					"note":            "插话在轮界消费（全量历史含插话接续，召回上下文已落库可见）",
				}, rc.emit)
			}
			return rt.Agent, nil
		},
		OnAgentEvents: func(ctx context.Context, tc *adk.TurnContext[runItem, *schema.Message], events *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.Message]]) error {
			rc.consume(events) // 事件翻译管线复用（55 号拍板：runConsumer 消费 AgentEvent 不变）
			// 轮界落盘：本轮部分产物即刷即持久化（长任务有界增量——中断/抢占不重放已做步）
			s.flushTurnAssistant(rc, conv, runID)
			select {
			case <-tc.Preempted:
				// 抢占导致的本轮取消（context.Canceled）会置 rc.stopped——运行并未停止，
				// 复位防终态误标；随后 GenInput 消费插话续跑。
				rc.stopped = false
				s.emitRunEvent(conv.ID, runID, "run.preempted", map[string]any{
					"safe_point": "any",
					// REQ-516/M323（P03）：召回上下文已落库——续轮全量重建历史可见（注入去留可观测）
					"recall_in_store": true,
					"note":            "steer 抢占生效——当前轮在安全点终止，下一轮以全量历史（含落库召回上下文）接续",
				}, rc.emit)
				return nil // 续跑 steer 轮（GenInput 消费插话）
			default:
			}
			if rc.interrupted {
				return nil // 业务中断（ask_human/approval）：框架以 InterruptError 退出并落 checkpoint
			}
			// 本轮正常收束（最终答案产出）——平台运行语义=一次 Run 至此完成；
			// TurnLoop 本体是常驻循环（收束后等待新条目），显式 Stop 防悬挂。
			// 此后到达的 Push 为 late item（SteerRun 返回 false+未消费警告，插话已落消息流可重发）。
			tc.Loop.Stop()
			return nil
		},
	})

	// 登记运行句柄（steerRun 抢占入口；对比窗格多路不登记）；退出时按值注销
	var handle *loopHandle
	if steerable {
		handle = &loopHandle{loop: loop, runID: runID, emit: rc.emit}
		s.mu.Lock()
		s.loops[conv.ID] = handle
		s.mu.Unlock()
	}
	defer func() {
		if steerable {
			// REQ-521/M328（P22）：按值删除（指针相同才删）——双运行同键覆盖时，先完成者
			// 的 defer 不得摘掉后运行者仍在使用的句柄（原按键删除致 steer 409）
			s.removeLoopHandle(conv.ID, handle)
		}
	}()

	loop.Push(runItem{Kind: "start", Content: "run"}, adk.WithPreempt[runItem, *schema.Message](adk.AnySafePoint))
	loop.Run(runCtx)
	exit := loop.Wait()

	// 退出态映射（诚实可观测）：业务中断/取消由 rc.consume 事件侧已置位；此处补框架级信号
	if exit != nil {
		// REQ-516/M323（P26②）：框架级退出此前静默收成 completed——GenInput/历史重建/checkpoint
		// 装载失败时 rc 三态全空，runFinalize 走 default 发 run.finished reason=completed，
		// 用户插话无响应却显示正常完成。收口：非 CancelError（正常 Stop/用户取消）且三态全空
		// 的框架级退出 → run.error + rc.runErr（终态诚实）。
		if exit.ExitReason != nil && !rc.stopped && !rc.interrupted && rc.runErr == "" {
			var cancelErr *adk.CancelError
			if !errors.As(exit.ExitReason, &cancelErr) {
				s.emitRunEvent(conv.ID, runID, "run.error", map[string]any{
					"message": "TurnLoop 框架级退出（本轮未完成）: " + exit.ExitReason.Error(),
				}, rc.emit)
				rc.runErr = "turnloop: " + exit.ExitReason.Error()
			}
		}
		if exit.CheckpointErr != nil {
			s.emitRunEvent(conv.ID, runID, "run.warning", map[string]any{
				"message": "TurnLoop checkpoint 保存失败（恢复能力降级）: " + exit.CheckpointErr.Error(),
			}, rc.emit)
		}
		if n := len(exit.UnhandledItems); n > 0 {
			s.emitRunEvent(conv.ID, runID, "run.warning", map[string]any{
				"message": fmt.Sprintf("%d 条插话未消费（运行已收束）——内容已落消息流，重新发送即可继续", n),
			}, rc.emit)
		}
		if exit.InterruptedItems != nil && rc.interrupted {
			if b, err := json.Marshal(exit.InterruptedItems); err == nil {
				log.Printf("[turnloop] run %s interrupted items: %.120s", runID, string(b))
			}
		}
	}
	return exit
}

// removeLoopHandle REQ-521/M328（P22）：按值删除运行句柄——仅当登记句柄仍是自己时才删，
// 防并发同会话双运行的先完成者 defer 误摘后运行者的 steer 句柄。
func (s *Service) removeLoopHandle(convID string, h *loopHandle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur := s.loops[convID]; cur == nil || cur == h {
		delete(s.loops, convID)
	}
}

// flushTurnAssistant 轮界助手产物落盘（每轮一条 assistant 消息+段事件；rc.buf 复位防 finalize 重复落库）。
// 长任务有界增量的持久化侧：抢占/中断后已完成轮的产物已在库，续跑轮历史可见。
func (s *Service) flushTurnAssistant(rc *runConsumer, conv *store.Conversation, runID string) {
	if rc == nil {
		return
	}
	s.flushTextSegment(rc, conv, runID, rc.emit)
	if len(rc.buf) == 0 {
		return
	}
	asg := &store.Message{ConversationID: conv.ID, Role: "assistant", Content: string(rc.buf)}
	if _, err := s.Store.InsertMessage(asg); err != nil {
		log.Printf("[turnloop] save turn assistant message: %v", err)
	}
	rc.buf = nil
	rc.segmentMark = 0
}
