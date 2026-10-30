package a2a

// REQ-387/M183：A2A 任务落库单测——写穿/DB 回读/孤儿对账。
import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func persistFixture(t *testing.T) (*Service, *store.Store, *store.Agent) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ag, err := st.CreateAgent(&store.Agent{Name: "a2a-agent", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	return New(st), st, ag
}

func TestA2ATaskPersistenceAndRestartReadback(t *testing.T) {
	svc, st, ag := persistFixture(t)
	// 模拟 message/send：直接构造任务+写穿（走生产 persistTask 路径需 handler——此处验证等价链）
	task := &Task{ID: "t-1", ContextID: "conv-1", AgentID: ag.ID, State: "working", Timestamp: "now"}
	svc.mu.Lock()
	svc.tasks[task.ID] = task
	svc.mu.Unlock()
	svc.persistTask(task)

	// 新 Service 实例（模拟重启）：tasks/get 回读
	svc2 := New(st)
	rec := httptest.NewRecorder()
	svc2.handleTasksGet(rec, rpcRequest{ID: 1, Params: json.RawMessage(`{"id":"t-1"}`)})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"working"`) {
		t.Fatalf("重启后应回读 DB: %d %s", rec.Code, rec.Body.String())
	}
	// 终态写穿后再查（completed 带 artifact）
	task.State, task.ArtifactText = "completed", "结果文本"
	svc.persistTask(task)
	row, _ := st.GetA2ATask("t-1")
	if row == nil || row.State != "completed" {
		t.Fatalf("终态应落库: %+v", row)
	}
	var pl store.A2APayload
	_ = json.Unmarshal([]byte(row.Payload), &pl)
	if pl.ArtifactText != "结果文本" {
		t.Fatalf("artifact 应落库: %s", row.Payload)
	}
}
