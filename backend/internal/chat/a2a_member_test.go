package chat

// REQ-416/M212 单测：外部 A2A 成员委派工具——协作面成功路径（send+轮询→artifacts 回传）、
// REQ-319 同语义降级回喂（远端失败→member_failed JSON+run.warning，不炸 run）、
// channel=a2a 委派单事件、卡片合成描述（技能透出）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/a2a"
)

// stubA2AEndpoint A2A 成员桩：可选失败模式（""=completed artifacts / "fail"=failed 终态 / "down"=端点不可达）。
func stubA2AEndpoint(t *testing.T, mode string) string {
	t.Helper()
	if mode == "down" {
		return "http://127.0.0.1:1"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		state := "working" // message/send 即回态
		final := "completed"
		if mode == "fail" {
			final = "failed"
		}
		switch req.Method {
		case "message/send":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
				"id": "t-1", "contextId": "c-1", "kind": "task",
				"status": map[string]any{"state": state},
			}})
		case "tasks/get":
			result := map[string]any{
				"id": "t-1", "contextId": "c-1", "kind": "task",
				"status": map[string]any{"state": final},
			}
			if final == "completed" {
				result["artifacts"] = []map[string]any{{"taskId": "t-1",
					"parts": []map[string]any{{"kind": "text", "text": "外部审查结论：代码可用"}}}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "方法未支持"}})
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func newA2ATool(t *testing.T, mode string) einotool.InvokableTool {
	t.Helper()
	bt, err := newA2AMemberTool(a2aMemberDeps{
		Name: "外部审查员", Endpoint: stubA2AEndpoint(t, mode),
		CardJSON: `{"name":"外部审查员","description":"代码审查","skills":[{"id":"s","name":"代码审查","description":""}]}`,
		PollSec:  0.002,
	})
	if err != nil {
		t.Fatal(err)
	}
	it, ok := bt.(einotool.InvokableTool)
	if !ok {
		t.Fatal("a2a 成员工具应为 InvokableTool")
	}
	return it
}

func TestA2AMemberToolSuccess(t *testing.T) {
	ctx, evts := withSink(context.Background())
	dt := newA2ATool(t, "")
	out, err := dt.InvokableRun(ctx, `{"request":"审查 PR #12"}`)
	if err != nil {
		t.Fatalf("外部成员成功应直回结果: %v", err)
	}
	if !strings.Contains(out, "外部审查结论") {
		t.Fatalf("artifacts 应回传: %s", out)
	}
	d := findEvent(evts, "subagent.delegate")
	if d == nil {
		t.Fatal("缺委派单事件")
	}
	if d.data["channel"] != "a2a" || d.data["member"] != "外部审查员" || d.data["task"] != "审查 PR #12" {
		t.Fatalf("channel=a2a 委派单不符: %v", d.data)
	}
	if _, ok := d.data["endpoint"].(string); !ok {
		t.Fatal("委派单应含 endpoint")
	}
	if findEvent(evts, "run.warning") != nil {
		t.Fatal("成功路径不应有 run.warning")
	}
}

func TestA2AMemberToolDegrade(t *testing.T) {
	for _, mode := range []string{"fail", "down"} {
		ctx, evts := withSink(context.Background())
		dt := newA2ATool(t, mode)
		out, err := dt.InvokableRun(ctx, `{"request":"任意任务"}`)
		if err != nil {
			t.Fatalf("%s: 失败应降级回喂而非报错: %v", mode, err)
		}
		if !strings.Contains(out, `"member_failed"`) || !strings.Contains(out, "外部审查员") || !strings.Contains(out, `"a2a"`) {
			t.Fatalf("%s: 降级 JSON 缺关键字段: %s", mode, out)
		}
		w := findEvent(evts, "run.warning")
		if w == nil {
			t.Fatalf("%s: 缺 run.warning", mode)
		}
		mf, _ := w.data["member_failed"].(map[string]any)
		if mf == nil || mf["channel"] != "a2a" {
			t.Fatalf("%s: run.warning 缺 a2a 标注: %v", mode, w.data)
		}
	}
}

func TestA2AMemberDescriptionSynthesis(t *testing.T) {
	desc := a2aMemberDescription(a2aMemberDeps{
		Name: "ext-1", Endpoint: "http://x/api/a2a/agents/9",
		CardJSON: `{"name":"外部编码者","description":"写代码的","skills":[{"id":"s1","name":"编码"},{"id":"s2","name":"测试"}]}`,
	})
	if !strings.Contains(desc, "外部编码者") || !strings.Contains(desc, "编码、测试") || !strings.Contains(desc, "A2A") {
		t.Fatalf("描述应合成卡片名/技能: %s", desc)
	}
	// 无卡片降级：仍可用（通用描述 + 端点）
	desc2 := a2aMemberDescription(a2aMemberDeps{Name: "ext-2", Endpoint: "http://y/a2a"})
	if !strings.Contains(desc2, "ext-2") || !strings.Contains(desc2, "http://y/a2a") {
		t.Fatalf("无卡片描述应含名称与端点: %s", desc2)
	}
}

// send 即回终态（同步型远端）跳过轮询 + 终态判定口径。
func TestA2ASendDoneShortcut(t *testing.T) {
	if done, failed := a2aSendDone(nil); !done || !failed {
		t.Fatal("nil 任务按失败兜底")
	}
	mk := func(state string) *a2a.RemoteTask { return &a2a.RemoteTask{State: state} }
	if done, failed := a2aSendDone(mk("completed")); !done || failed {
		t.Fatal("completed 应为成功终态")
	}
	if done, failed := a2aSendDone(mk("input-required")); !done || !failed {
		t.Fatal("input-required 无交互通道按失败")
	}
	if done, _ := a2aSendDone(mk("working")); done {
		t.Fatal("working 非终态")
	}
}
