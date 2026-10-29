package chat

// REQ-208/M42 余项：可继续子 Agent（continuation）后台委派管理器。
// agent_as_tool 协调者既有一次性同步委派（delegatetool.go，REQ-319 包装）之外的可继续形态：
//   - 后台委派 delegate_background：子会话（一等会话 scope=agent）后台运行，协调者不阻塞；
//   - steer 中途引导 delegate_steer：向运行中子智能体追加指令——插话落子会话并标记，
//     当前轮结束后**自动续跑一轮**消费插话（Eino Runner 无 mid-run 输入通道，诚实边界：
//     非 token 级中断注入，为「轮界接续」语义）；已完成/已停止则直接以插话续跑；
//   - 冷恢复：子会话历史持久化，重开即接续上下文；backend 重启孤儿委派标记 stopped，
//     delegate_continue 从子会话续跑；
//   - interrupt 打断 delegate_interrupt：Stop 子会话运行（stopped 留档）。
// 委派全程发 subagent.delegate 事件（phase=started|completed|failed|stopped|steered），
// 会话轨迹与 UI 卡片消费。

import (
	"errors"

	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// DelegateRunTimeout 单轮后台运行上限（沿 a2a message/send 后台执行口径）。
const DelegateRunTimeout = 10 * time.Minute

// DelegateResultDigestCap 协调者收取结果时的摘要上限（rune；防协调者上下文膨胀，
// 全文在子会话——REQ-203 结果压缩回传口径的可继续形态延伸）。
const DelegateResultDigestCap = 2000

// DelegateManager 后台委派管理器（进程内 goroutine 承载运行轮；事实源在 conversation_delegate 表）。
type DelegateManager struct {
	Store *store.Store
	// StartRun 后台执行一次子会话运行（生产装配=chat.Run+伴生收尾+最终助手消息读取，
	// 与 SSE/A2A 路径同管线；测试=桩）。返回最终助手消息全文。
	StartRun func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error)
	// StopRun 停止某会话进行中的运行（生产=chat.Service.Stop；测试=桩可空）。
	StopRun func(conversationID string) bool

	mu    sync.Mutex
	live  map[string]func() // delegateID → 打断钩子（cancel 当前轮 ctx）
}

// NewDelegateManager 构造。
func NewDelegateManager(st *store.Store) *DelegateManager {
	return &DelegateManager{Store: st, live: map[string]func(){}}
}

// emitDelegateEvent 委派事件（会话级轨迹；run_id=子会话当前轮；channel 缺省 internal——
// REQ-416 起外部 A2A 成员不产 delegate 行，后台委派恒为平台内部成员）。
func (m *DelegateManager) emitDelegateEvent(convID, runID, phase string, data map[string]any) {
	if m.Store == nil {
		return
	}
	full := map[string]any{"phase": phase, "channel": "internal"}
	for k, v := range data {
		full[k] = v
	}
	b, err := json.Marshal(full)
	if err != nil {
		return
	}
	if _, err := m.Store.InsertEvent(&store.RunEvent{ConversationID: convID, RunID: runID, Type: "subagent.delegate", Data: string(b), SchemaVersion: store.EventSchemaVersion}); err != nil {
		log.Printf("[delegate] 事件落库失败: %v", err)
	}
}

// Start 后台委派：建子会话+登记+goroutine 运行轮（首轮输入=task）。
func (m *DelegateManager) Start(parent *store.Conversation, member *store.Agent, task string) (*store.Delegate, error) {
	child, err := m.Store.CreateConversation(&store.Conversation{
		Scope: "agent", AgentID: &member.ID,
		Title: "[委派] " + runeforTitle(task, 30),
	})
	if err != nil {
		return nil, fmt.Errorf("创建子会话失败: %w", err)
	}
	d := &store.Delegate{
		ParentConversationID: parent.ID,
		ChildConversationID:  child.ID,
		MemberAgentID:        member.ID,
		MemberName:           member.Name,
		Task:                 task,
	}
	if err := m.Store.CreateDelegate(d); err != nil {
		return nil, err
	}
	m.emitDelegateEvent(parent.ID, "", "started", map[string]any{
		"delegate_id": d.ID, "member": member.Name, "task": runeforTitle(task, 200),
		"child_conversation_id": child.ID, "via": "background",
	})
	go m.runLoop(d.ID, child, member, task)
	return d, nil
}

// runLoop 运行轮循环：首轮=task；此后每轮开始前取走 steer 插话作为输入（轮界接续）。
// 一次插话恰好多跑一轮；循环随插话队列延长，无插话即收束。
func (m *DelegateManager) runLoop(delegateID string, child *store.Conversation, member *store.Agent, firstInput string) {
	input := firstInput
	for round := 1; ; round++ {
		if round > 1 {
			steer, err := m.Store.TakeDelegateSteer(delegateID)
			if err != nil {
				m.finish(delegateID, child, "failed", "", err.Error())
				return
			}
			input = steer
			_ = m.Store.SetDelegateRun(delegateID, "")
		}
		runID := "bgr_" + store.NewID()
		_ = m.Store.SetDelegateRun(delegateID, runID)
		ctx, cancel := context.WithTimeout(context.Background(), DelegateRunTimeout)
		m.mu.Lock()
		m.live[delegateID] = cancel
		m.mu.Unlock()
		text, err := m.StartRun(ctx, child, member, runID, input)
		m.mu.Lock()
		delete(m.live, delegateID)
		m.mu.Unlock()
		cancel()
		if err != nil {
			// REQ-520/M327：stopped 哨兵（生产 StartRun 对用户打断返回 ErrRunStopped）与
			// ctx 取消（打断/超时）同归 stopped 留档（可冷恢复）
			if ctx.Err() != nil || errors.Is(err, ErrRunStopped) {
				m.finish(delegateID, child, "stopped", truncateRunesBG(text), "已打断（子会话历史完整，可续跑恢复）")
				return
			}
			m.finish(delegateID, child, "failed", truncateRunesBG(text), err.Error())
			return
		}
		_ = m.Store.UpdateDelegateStatus(delegateID, "completed", text, "")
		m.emitDelegateEvent(child.ID, runID, "completed", map[string]any{"delegate_id": delegateID})
		m.emitDelegateEvent(m.parentOf(delegateID), runID, "completed", map[string]any{
			"delegate_id": delegateID, "member": member.Name, "result_head": truncateRunesBG(text),
		})
		// 收束判定：无新插话即结束；有插话 → 续跑轮（插话在循环头取走）
		pending, _ := m.Store.GetDelegate(delegateID)
		if pending == nil || pending.SteerNote == "" {
			return
		}
		m.emitDelegateEvent(m.parentOf(delegateID), runID, "steered", map[string]any{
			"delegate_id": delegateID, "member": member.Name, "note": "插话已受理，续跑一轮消费",
		})
	}
}

// finish 终态落库+事件（parent 侧可观测）。
func (m *DelegateManager) finish(delegateID string, child *store.Conversation, status, digest, msg string) {
	_ = m.Store.UpdateDelegateStatus(delegateID, status, digest, msg)
	d, err := m.Store.GetDelegate(delegateID)
	if err != nil {
		return
	}
	m.emitDelegateEvent(d.ChildConversationID, d.RunID, status, map[string]any{"delegate_id": delegateID})
	m.emitDelegateEvent(d.ParentConversationID, d.RunID, status, map[string]any{
		"delegate_id": delegateID, "member": d.MemberName, "result_head": digest, "error": msg,
	})
}

// parentOf 委派归属父会话（事件投递用；查不到返回空串跳过）。
func (m *DelegateManager) parentOf(delegateID string) string {
	d, err := m.Store.GetDelegate(delegateID)
	if err != nil {
		return ""
	}
	return d.ParentConversationID
}

// Steer 中途引导：插话落子会话（界面对话流可见）+ 标记；运行中→轮界续跑消费；
// 已收束→直接以插话续跑一轮。
func (m *DelegateManager) Steer(delegateID, message string) (*store.Delegate, error) {
	d, err := m.Store.GetDelegate(delegateID)
	if err != nil {
		return nil, err
	}
	if message == "" {
		return nil, fmt.Errorf("插话内容为空")
	}
	// 插话先落子会话消息流（历史与冷恢复一致性：续跑轮经消息历史可见该插话）
	if _, err := m.Store.InsertMessage(&store.Message{ConversationID: d.ChildConversationID, Role: "user", Content: message}); err != nil {
		return nil, err
	}
	if _, err := m.Store.SetDelegateSteer(delegateID, message); err != nil {
		return nil, err
	}
	m.emitDelegateEvent(d.ParentConversationID, d.RunID, "steered", map[string]any{
		"delegate_id": delegateID, "member": d.MemberName, "note": runeforTitle(message, 100),
	})
	if d.Status != "running" {
		// 已收束：直接续跑一轮（冷恢复同路径）
		member, gerr := m.Store.GetAgent(d.MemberAgentID)
		if gerr != nil {
			return nil, gerr
		}
		child, gerr := m.Store.GetConversation(d.ChildConversationID)
		if gerr != nil {
			return nil, gerr
		}
		go m.runLoop(delegateID, child, member, "")
	}
	return m.Store.GetDelegate(delegateID)
}

// Continue 冷恢复/续跑：以「继续」为题从子会话续跑一轮（重启孤儿与人工恢复共用）。
func (m *DelegateManager) Continue(delegateID, instruction string) (*store.Delegate, error) {
	d, err := m.Store.GetDelegate(delegateID)
	if err != nil {
		return nil, err
	}
	if d.Status == "running" {
		return d, nil
	}
	if instruction == "" {
		instruction = "继续此前任务（上轮被中断，从既有进度接着做）"
	}
	return m.Steer(delegateID, instruction)
}

// Interrupt 打断：Stop 子会话当前轮（runLoop 收 ctx 取消→stopped 留档）。
func (m *DelegateManager) Interrupt(delegateID string) (*store.Delegate, error) {
	d, err := m.Store.GetDelegate(delegateID)
	if err != nil {
		return nil, err
	}
	if m.StopRun != nil {
		m.StopRun(d.ChildConversationID)
	}
	m.mu.Lock()
	cancel := m.live[delegateID]
	m.mu.Unlock()
	if cancel != nil {
		cancel() // 兜底（StartRun 桩不注册 cancels 时仍能收束）
	}
	return m.Store.GetDelegate(delegateID)
}

// Collect 收取结果：委派登记+子会话最终助手消息全文（工具侧自行截断）。
func (m *DelegateManager) Collect(delegateID string) (*store.Delegate, string, error) {
	d, err := m.Store.GetDelegate(delegateID)
	if err != nil {
		return nil, "", err
	}
	msgs, err := m.Store.ListMessages(d.ChildConversationID)
	if err != nil {
		return d, d.ResultDigest, nil
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" && msgs[i].Content != "" {
			return d, msgs[i].Content, nil
		}
	}
	return d, d.ResultDigest, nil
}

// LoadOrphans 重启清点：孤儿 running → stopped（冷恢复入口登记）。
func (m *DelegateManager) LoadOrphans() {
	n, err := m.Store.MarkOrphanDelegates()
	if err != nil {
		log.Printf("[delegate] 重启清点失败: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[delegate] 重启清点：%d 条进行中委派标记 stopped（子会话历史完整，可续跑冷恢复）", n)
	}
}

// runeforTitle 标题/事件用截断（单行化）。
func runeforTitle(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// truncateRunesBG 结果摘要截断（rune 安全）。
func truncateRunesBG(s string) string {
	r := []rune(s)
	if len(r) <= DelegateResultDigestCap {
		return s
	}
	return string(r[:DelegateResultDigestCap]) + "…[截断，全文见子会话]"
}
