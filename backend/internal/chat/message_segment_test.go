package chat

// REQ-406①/M202：message.segment 分段持久化单测——纯逻辑（flushTextSegment 落点推进/空段跳过/事件落库）。
import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func TestFlushTextSegment(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ag, _ := st.CreateAgent(&store.Agent{Name: "seg-agent"})
	conv, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: st}
	rc := &runConsumer{s: s, ctx: t.Context(), conv: conv, runID: "run-seg", toolAgg: map[int]*pendingToolCall{}, toolCallAt: map[string]time.Time{}, toolCallArg: map[string]string{}, toolCallUsage: map[string]*usageSnapshot{}, toolUsageAgg: map[string]int{}}
	var events []string
	emit := func(ev *Event) { events = append(events, ev.Type) }

	// 空缓冲：无事件
	s.flushTextSegment(rc, conv, "run-seg", emit)
	if len(events) != 0 {
		t.Fatal("空缓冲不应发事件")
	}
	// 首段：落库+落点推进
	rc.buf = append(rc.buf, []byte("第一段正文。")...)
	s.flushTextSegment(rc, conv, "run-seg", emit)
	if len(events) != 1 || events[0] != "message.segment" {
		t.Fatalf("应发 message.segment: %v", events)
	}
	if rc.segmentMark != len(rc.buf) {
		t.Fatal("落点应推进到 buf 尾")
	}
	// 无新增：跳过
	s.flushTextSegment(rc, conv, "run-seg", emit)
	if len(events) != 1 {
		t.Fatal("无增量不应发事件")
	}
	// 第二段（工具边界）+ 纯空白段跳过
	rc.buf = append(rc.buf, []byte("\n\n第二段（工具后）。")...)
	s.flushTextSegment(rc, conv, "run-seg", emit)
	if len(events) != 2 {
		t.Fatalf("第二段应发: %v", events)
	}
	rc.buf = append(rc.buf, []byte("   ")...) // 纯空白
	s.flushTextSegment(rc, conv, "run-seg", emit)
	if len(events) != 2 {
		t.Fatal("纯空白段应跳过")
	}

	// 事件落库断言：两段 text 与 seq
	evs, _, err := st.ListEventsQ(conv.ID, store.EventQuery{Type: "message.segment"})
	if err != nil || len(evs) != 2 {
		t.Fatalf("segment 事件应落库 2 条: %v %d", err, len(evs))
	}
	var first struct {
		Text string `json:"text"`
		Seq  int    `json:"seq"`
	}
	_ = json.Unmarshal([]byte(evs[0].Data), &first)
	if first.Text != "第一段正文。" || first.Seq != 0 {
		t.Fatalf("首段载荷不符: %+v", first)
	}
	var second struct {
		Text string `json:"text"`
		Seq  int    `json:"seq"`
	}
	_ = json.Unmarshal([]byte(evs[1].Data), &second)
	if !strings.Contains(second.Text, "第二段") || second.Seq != 1 {
		t.Fatalf("次段载荷不符: %+v", second)
	}
}
