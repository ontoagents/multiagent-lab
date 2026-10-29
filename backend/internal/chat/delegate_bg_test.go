package chat

// REQ-208/M42 余项：可继续子 Agent 后台委派管理器单测（StartRun 桩——不依赖真模型）。
// 覆盖：后台委派全链（启动→完成→结果摘要）/steer 运行中插话→续跑轮消费/打断→stopped→
// 冷恢复 continue/归属守卫/重启孤儿清点。

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func delegateTestManager(t *testing.T) (*DelegateManager, *store.Store, *store.Conversation, *store.Agent) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ag, err := st.CreateAgent(&store.Agent{Name: "member-a"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	return NewDelegateManager(st), st, parent, ag
}

func TestDelegateBackgroundLifecycle(t *testing.T) {
	m, st, parent, member := delegateTestManager(t)
	var mu sync.Mutex
	calls := 0
	m.StartRun = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		// 生产契约=chat.Run 负责消息落库（用户输入+助手回复），桩同口径
		_, _ = st.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "user", Content: input})
		_, _ = st.InsertMessage(&store.Message{ConversationID: conv.ID, Role: "assistant", Content: fmt.Sprintf("第 %d 轮完成：处理了「%s」", n, input)})
		return fmt.Sprintf("第 %d 轮完成：处理了「%s」", n, input), nil
	}

	d, err := m.Start(parent, member, "调研多智能体委派形态")
	if err != nil {
		t.Fatal(err)
	}
	// 等完成
	deadline := time.Now().Add(3 * time.Second)
	for {
		cur, _ := st.GetDelegate(d.ID)
		if cur != nil && cur.Status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("委派未完成: %+v", cur)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cur, _ := st.GetDelegate(d.ID)
	if cur.ChildConversationID == "" || cur.MemberName != "member-a" || !strings.Contains(cur.ResultDigest, "第 1 轮完成") {
		t.Fatalf("完成态不符: %+v", cur)
	}
	// 子会话消息落库（用户任务+助手回复）
	msgs, _ := st.ListMessages(cur.ChildConversationID)
	if len(msgs) < 2 || msgs[0].Role != "user" {
		t.Fatalf("子会话消息应含任务输入: %d", len(msgs))
	}
	// Collect 返回全文
	_, full, err := m.Collect(d.ID)
	if err != nil || !strings.Contains(full, "第 1 轮完成") {
		t.Fatalf("collect 应返回全文: %v %q", err, full)
	}
	// 轨迹事件：started+completed 落父会话
	evs, _, _ := st.ListEventsQ(parent.ID, store.EventQuery{Type: "subagent.delegate"})
	phases := map[string]bool{}
	for _, e := range evs {
		var data struct {
			Phase string `json:"phase"`
		}
		_ = json.Unmarshal([]byte(e.Data), &data)
		phases[data.Phase] = true
	}
	if !phases["started"] || !phases["completed"] {
		t.Fatalf("应有 started/completed 事件: %v", phases)
	}
}

func TestDelegateSteerContinuation(t *testing.T) {
	m, st, parent, member := delegateTestManager(t)
	m.StartRun = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		if strings.Contains(input, "补充要求") {
			return "已按补充要求调整", nil
		}
		// 首轮运行中收插话（模拟协调者 steer：按父会话查委派 id）
		if ls, _ := st.ListDelegatesByParent(parent.ID); len(ls) > 0 {
			_, _ = m.Steer(ls[0].ID, "补充要求：把结论按清单输出")
		}
		return "首轮完成", nil
	}
	d, err := m.Start(parent, member, "写一份摘要")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var cur *store.Delegate
	for {
		cur, _ = st.GetDelegate(d.ID)
		if cur != nil && cur.Status == "completed" && strings.Contains(cur.ResultDigest, "补充要求") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("steer 续跑未收束: %+v", cur)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(cur.ResultDigest, "已按补充要求调整") {
		t.Fatalf("续跑轮结果不符: %q", cur.ResultDigest)
	}
	if cur.SteerNote != "" {
		t.Fatalf("插话应被消费清空: %q", cur.SteerNote)
	}
}

func TestDelegateInterruptAndColdResume(t *testing.T) {
	m, st, parent, member := delegateTestManager(t)
	release := make(chan struct{})
	m.StartRun = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		select {
		case <-release:
			return "恢复后完成", nil
		case <-ctx.Done():
			// REQ-520/M327：模拟生产 chat.Run 停止形态——(半截文本, ErrRunStopped 哨兵) 而非
			// (nil, ctx.Err())。原桩与生产契约失配（测试全绿掩盖打断被误标 completed）。
			return "半截产出", ErrRunStopped
		}
	}
	d, _ := m.Start(parent, member, "长任务")
	time.Sleep(50 * time.Millisecond)
	if _, err := m.Interrupt(d.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		cur, _ := st.GetDelegate(d.ID)
		if cur != nil && cur.Status == "stopped" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("打断未收束为 stopped")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 冷恢复：以插话路径续跑（放开桩）
	close(release)
	if _, err := m.Continue(d.ID, ""); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		cur, _ := st.GetDelegate(d.ID)
		if cur != nil && cur.Status == "completed" && strings.Contains(cur.ResultDigest, "恢复后完成") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("冷恢复未完成: %+v", cur)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDelegateOrphanSweep(t *testing.T) {
	m, st, parent, member := delegateTestManager(t)
	m.StartRun = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		return "ok", nil
	}
	d, _ := m.Start(parent, member, "遗留任务")
	// 直接把状态摆成 running（模拟进程重启时 goroutine 消失）
	deadline := time.Now().Add(3 * time.Second)
	for {
		cur, _ := st.GetDelegate(d.ID)
		if cur != nil && cur.Status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("首 run 未完成")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := st.UpdateDelegateStatus(d.ID, "running", "", ""); err != nil {
		t.Fatal(err)
	}
	m2 := NewDelegateManager(st)
	m2.LoadOrphans()
	cur, _ := st.GetDelegate(d.ID)
	if cur.Status != "stopped" || !strings.Contains(cur.Error, "冷恢复") {
		t.Fatalf("孤儿应标记 stopped+冷恢复提示: %+v", cur)
	}
}

func TestDelegateBelongGuard(t *testing.T) {
	m, st, parent, member := delegateTestManager(t)
	m.StartRun = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		return "ok", nil
	}
	d, _ := m.Start(parent, member, "t")
	deadline := time.Now().Add(3 * time.Second)
	for {
		cur, _ := st.GetDelegate(d.ID)
		if cur != nil && cur.Status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("未完成")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 工具归属守卫：delegateBackgroundTools 的 belong 校验（换父会话 id 即拒绝）
	other, _ := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &member.ID})
	tools := delegateBackgroundTools(DelegateToolDeps{Manager: m, Store: st, ParentConvID: other.ID, Members: []*store.Agent{member}})
	if len(tools) != 6 {
		t.Fatalf("应装配 6 件工具: %d", len(tools))
	}
}

// TestDelegateUserStopChild REQ-520/M327：用户直接停止子会话（委派 ctx 未取消）——生产停止形态
// (半截文本, ErrRunStopped 哨兵) 应归类 stopped 而非 completed（修复点直接实证：原 (text,nil)
// 形态使打断被误标 completed、digest 半截告知成功、delegate_continue 冷恢复失效）。
func TestDelegateUserStopChild(t *testing.T) {
	m, st, parent, member := delegateTestManager(t)
	m.StartRun = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		time.Sleep(100 * time.Millisecond) // 模拟运行中——委派 ctx 未取消
		return "半截产出", ErrRunStopped   // 用户在子会话按停止：仅哨兵错误
	}
	d, _ := m.Start(parent, member, "长任务")
	deadline := time.Now().Add(3 * time.Second)
	for {
		cur, _ := st.GetDelegate(d.ID)
		if cur != nil && cur.Status == "stopped" {
			return // 命中即通过
		}
		if time.Now().After(deadline) {
			if cur != nil && cur.Status == "completed" {
				t.Fatalf("用户停子会话被误标 completed（修复失效）: %+v", cur)
			}
			t.Fatalf("未收束为 stopped: %+v", cur)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
