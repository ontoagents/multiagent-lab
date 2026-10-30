package chat

// REQ-456/M252：上下文准入链 Retriever 接口化单测——
// 外部 eino Retriever 反向插入（retrieverProvider：命中注入 System/空跳过/失败降级不阻断）/
// 注册表按名稳定序/context.stats 源名与链对齐（externalSourceNames）。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

type fakeRetriever struct {
	docs []*schema.Document
	err  error
}

func (f *fakeRetriever) Retrieve(_ context.Context, _ string, _ ...retriever.Option) ([]*schema.Document, error) {
	return f.docs, f.err
}

func TestRetrieverProvider_InjectAndEvents_Req456(t *testing.T) {
	var events []map[string]any
	emit := func(ev *Event) {
		if ev.Type == "retrieval" || ev.Type == "run.warning" {
			var d map[string]any
			_ = json.Unmarshal(ev.Data, &d)
			events = append(events, d)
		}
	}
	fr := &fakeRetriever{docs: []*schema.Document{{Content: "文档甲"}, {Content: "文档乙"}}}
	rp := retrieverProvider("ext-src", fr)
	base := []*schema.Message{schema.UserMessage("问题")}
	out := rp(context.Background(), nil, nil, "run-1", "问题", base, emit)
	if len(out) != 2 {
		t.Fatalf("应注入 1 条 System 消息，得到 %d", len(out))
	}
	last := out[len(out)-1]
	if last.Role != schema.System || !strings.Contains(last.Content, "文档甲") || !strings.Contains(last.Content, "ext-src") {
		t.Fatalf("注入消息形态不符: role=%s content=%q", last.Role, last.Content[:30])
	}
	if len(events) != 1 || events[0]["source"] != "ext-src" || events[0]["hits"].(float64) != 2 { // json 解码数值为 float64
		t.Fatalf("retrieval 事件不符: %v", events)
	}
	// 空命中：零注入零事件
	fr2 := &fakeRetriever{docs: nil}
	out2 := retrieverProvider("e", fr2)(context.Background(), nil, nil, "r", "q", base, emit)
	if len(out2) != 1 {
		t.Fatalf("空命中不应注入，得到 %d", len(out2)-1)
	}
}

func TestRetrieverProvider_FailDegrade_Req456(t *testing.T) {
	var warns []map[string]any
	emit := func(ev *Event) {
		if ev.Type == "run.warning" {
			var d map[string]any
			_ = json.Unmarshal(ev.Data, &d)
			warns = append(warns, d)
		}
	}
	fr := &fakeRetriever{err: errors.New("外部源不可达")}
	base := []*schema.Message{schema.UserMessage("q")}
	out := retrieverProvider("bad-src", fr)(context.Background(), nil, nil, "r", "q", base, emit)
	if len(out) != 1 {
		t.Fatal("失败降级应不注入不阻断")
	}
	if len(warns) != 1 || !strings.Contains(warns[0]["message"].(string), "bad-src") {
		t.Fatalf("应有 run.warning 留痕: %v", warns)
	}
}

func TestRegisterRetrieverSource_Order_Req456(t *testing.T) {
	s := &Service{}
	s.RegisterRetrieverSource("zeta", &fakeRetriever{})
	s.RegisterRetrieverSource("alpha", &fakeRetriever{})
	chain := s.recallChainWithExternal()
	if len(chain) != 4 { // kb + companion + 2 external
		t.Fatalf("链长应为 4，得到 %d", len(chain))
	}
	names := externalSourceNames(s)
	if names[0] != "alpha" || names[1] != "zeta" {
		t.Fatalf("外部源应按名稳定序: %v", names)
	}
	// 覆盖注册：同名覆盖不重复
	s.RegisterRetrieverSource("alpha", &fakeRetriever{})
	if len(s.recallChainWithExternal()) != 4 {
		t.Fatal("同名注册应覆盖不追加")
	}
}

func TestProviderRetriever_Projection_Req456(t *testing.T) {
	s := &Service{} // KB/Companion 均 nil → 两源零注入（降级纪律），投影返回空
	emptyConv := &store.Conversation{} // EnableKB=false：两源零注入（nil conv 会空指针）
	r := s.NewProviderRetriever("", emptyConv, nil, "run-p", []*schema.Message{schema.UserMessage("q")}, nil)
	docs, err := r.Retrieve(context.Background(), "q")
	if err != nil {
		t.Fatalf("投影 Retrieve 不应报错: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("nil 源下应零文档，得到 %d", len(docs))
	}
	// 未知单源名报错
	r2 := s.NewProviderRetriever("nope", emptyConv, nil, "r", nil, nil)
	if _, err := r2.Retrieve(context.Background(), "q"); err == nil {
		t.Fatal("未知源名应报错")
	}
}
