// REQ-214 顺修回归：恢复链 checkpoint 锚点——多级审批第二次恢复不再 checkpoint not exist。
// 场景复现：Run 挂起（id=runID 派生）→ Resume 批准 → 模型再次调工具再次挂起
// （ADK 沿用恢复锚点 id 覆盖保存快照）→ handleInterrupted 须记录同一 id。
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

// Drive the public Run/Resume paths through the real assembler and SQLite
// checkpoints. Only the remote model is replaced: it asks for two sequential
// tool calls, each of which must be approved before it actually executes.
func TestRepeatedApprovalResumeAfterRestart(t *testing.T) {
	for _, turnLoop := range []string{"0", "1"} {
		t.Run("turnloop="+turnLoop, func(t *testing.T) {
			t.Setenv("CHAT_TURNLOOP", turnLoop)
			oldStreaming := turnLoopStreamingDefault
			turnLoopStreamingDefault = false
			t.Cleanup(func() { turnLoopStreamingDefault = oldStreaming })
			modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Stream   bool `json:"stream"`
					Messages []struct {
						Role string `json:"role"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				completed := 0
				for _, msg := range req.Messages {
					if msg.Role == "tool" {
						completed++
					}
				}
				message := map[string]any{"role": "assistant", "content": "both approvals completed"}
				finish := "stop"
				if completed < 2 {
					message["content"] = ""
					message["tool_calls"] = []map[string]any{{"index": 0, "id": fmt.Sprintf("approval-%d", completed+1), "type": "function",
						"function": map[string]any{"name": "current_time", "arguments": "{}"}}}
					finish = "tool_calls"
				}
				field := "message"
				if req.Stream {
					field = "delta"
					w.Header().Set("Content-Type", "text/event-stream")
				}
				b, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion", "model": "fixture",
					"choices": []map[string]any{{"index": 0, field: message, "finish_reason": finish}}})
				if req.Stream {
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", b)
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write(b)
				}
			}))
			t.Cleanup(modelServer.Close)
			dir := t.TempDir()
			box, err := secrets.LoadKeyFile(filepath.Join(dir, ".secret"))
			if err != nil {
				t.Fatal(err)
			}
			dbPath := filepath.Join(dir, "platform.db")
			st, err := store.Open(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			conn, err := st.CreateConnection(&store.ModelConnection{Name: "approval-fixture", ConnType: "chat", Protocol: "openai_compat",
				BaseURL: modelServer.URL, ModelName: "fixture", Enabled: true, IsDefault: true}, nil)
			if err != nil {
				t.Fatal(err)
			}
			ag, err := st.CreateAgent(&store.Agent{Name: "approval-chain", ModelConnID: &conn.ID, Tools: []string{"current_time"}, ToolApproval: "all", MaxIteration: 10})
			if err != nil {
				t.Fatal(err)
			}
			conv, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID, Title: "approval chain"})
			if err != nil {
				t.Fatal(err)
			}
			newService := func() *Service {
				reg := tool.NewRegistry()
				if err := tool.RegisterBuiltin(reg); err != nil {
					t.Fatal(err)
				}
				return NewService(st, &Assembler{Store: st, Box: box, Tools: reg, CheckPoints: NewStoreCheckPointStore(st)}, nil)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			s := newService()
			if res, err := s.Run(ctx, conv, ag, "approval-run-1", "run two approved tools", 0, false, nil); err != nil || res.Error != "" {
				t.Fatalf("initial run: result=%+v err=%v", res, err)
			}
			for step := 1; step <= 2; step++ {
				conv, err = st.GetConversation(conv.ID)
				if err != nil {
					t.Fatal(err)
				}
				var interrupted interruptState
				if json.Unmarshal([]byte(conv.InterruptState), &interrupted) != nil || interrupted.Kind != "approval" || interrupted.TargetID == "" {
					t.Fatalf("approval %d missing: %q", step, conv.InterruptState)
				}
				if interrupted.CheckpointID != "ckpt_approval-run-1" {
					t.Fatalf("approval %d lost original checkpoint: %q", step, interrupted.CheckpointID)
				}
				// Restart before every approval: no process-local anchor may be needed.
				if err := st.Close(); err != nil {
					t.Fatal(err)
				}
				st, err = store.Open(dbPath)
				if err != nil {
					t.Fatal(err)
				}
				s = newService()
				res, err := s.Resume(ctx, conv, ag, fmt.Sprintf("approval-run-%d", step+1), tool.ApprovalApprove, 0, false, nil)
				if err != nil || res.Error != "" {
					t.Fatalf("resume %d: result=%+v err=%v", step, res, err)
				}
			}
			conv, err = st.GetConversation(conv.ID)
			if err != nil || conv.InterruptState != "" {
				t.Fatalf("finished run remains interrupted: %+v err=%v", conv, err)
			}
			messages, err := st.ListMessages(conv.ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, msg := range messages {
				found = found || (msg.Role == "assistant" && strings.Contains(msg.Content, "both approvals completed"))
			}
			if !found {
				t.Fatal("approved execution did not persist its final answer")
			}
		})
	}
}

func anchorCPID(t *testing.T, s *Service, convID string) string {
	t.Helper()
	conv, err := s.Store.GetConversation(convID)
	if err != nil {
		t.Fatal(err)
	}
	var st interruptState
	if err := json.Unmarshal([]byte(conv.InterruptState), &st); err != nil {
		t.Fatalf("interrupt_state 解析: %v", err)
	}
	return st.CheckpointID
}

func TestInterruptCheckpointAnchor(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/anchor.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Service{Store: st}
	ag, err := st.CreateAgent(&store.Agent{Name: "anchor-agent", MaxIteration: 5})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := st.CreateConversation(&store.Conversation{Scope: "agent", AgentID: &ag.ID, Title: "anchor"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	noemit := func(*Event) {}

	// ① 新 Run 挂起：checkpoint id = runID 派生
	s.handleInterrupted(ctx, conv, "run-1", nil, noemit, "")
	if got := anchorCPID(t, s, conv.ID); got != "ckpt_run-1" {
		t.Fatalf("新 Run 应 runID 派生, got %s", got)
	}

	// ② Run 清锚点语义：再次 Run 前 cpAnchors 无该会话
	s.cpAnchors.Delete(conv.ID)
	s.handleInterrupted(ctx, conv, "run-2", nil, noemit, "")
	if got := anchorCPID(t, s, conv.ID); got != "ckpt_run-2" {
		t.Fatalf("清锚点后应 runID 派生, got %s", got)
	}

	// ③ Resume 后再次挂起：ADK 沿用锚点 id 覆盖保存——平台须记录同一 id
	s.cpAnchors.Store(conv.ID, "ckpt_run-2")
	s.handleInterrupted(ctx, conv, "run-3", nil, noemit, "")
	if got := anchorCPID(t, s, conv.ID); got != "ckpt_run-2" {
		t.Fatalf("恢复链二次挂起应沿用锚点 id, got %s", got)
	}
}
