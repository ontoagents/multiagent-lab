package api

// REQ-370/M168 流水线端点单测：一键创建四角色+项目（幂等——同名复用不覆盖）、
// 工程师/评审 onto_ops 开、项目成员与协调者落齐、二次调用零重复。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func newPipelineServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return &Server{Store: st}
}

func TestOntologyPipelineCreateAndIdempotent(t *testing.T) {
	s := newPipelineServer(t)

	// ① 首次创建：四角色+项目
	req := httptest.NewRequest(http.MethodPost, "/api/ontology/collab-pipeline", nil)
	rec := httptest.NewRecorder()
	s.createOntologyPipeline(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ProjectID   string            `json:"project_id"`
		Coordinator string            `json:"coordinator_id"`
		Members     map[string]string `json:"members"`
		AgentsCreated []string        `json:"agents_created"`
		CollabMode  string            `json:"collab_mode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应解析: %v", err)
	}
	if out.CollabMode != "agent_as_tool" {
		t.Fatalf("协作模式应为 agent_as_tool: %v", out.CollabMode)
	}
	if len(out.AgentsCreated) != 4 {
		t.Fatalf("应新建 4 角色: %v", out.AgentsCreated)
	}
	if len(out.Members) != 4 || out.Coordinator != out.Members["coordinator"] {
		t.Fatalf("成员/协调者不符: %v", out.Members)
	}
	// 工程师与评审 onto_ops 开（spec 工具面门控前提）
	for _, role := range []string{"ontology_engineer", "quality_reviewer"} {
		a, err := s.Store.GetAgent(out.Members[role])
		if err != nil {
			t.Fatalf("%s 加载失败: %v", role, err)
		}
		if !a.OntoOps {
			t.Fatalf("%s 应开启 onto_ops（spec 工具面）", role)
		}
	}
	// 项目成员与协调者落齐
	p, err := s.Store.GetProject(out.ProjectID)
	if err != nil {
		t.Fatalf("项目加载失败: %v", err)
	}
	if len(p.AgentIDs) != 4 || p.Coordinator != out.Coordinator {
		t.Fatalf("项目成员/协调者不符: %+v", p)
	}

	// ② 二次调用：全复用零新建
	req2 := httptest.NewRequest(http.MethodPost, "/api/ontology/collab-pipeline", nil)
	rec2 := httptest.NewRecorder()
	s.createOntologyPipeline(rec2, req2)
	var out2 struct {
		ProjectID     string     `json:"project_id"`
		Members       map[string]string `json:"members"`
		AgentsCreated []string   `json:"agents_created"`
		ProjectCreated bool      `json:"project_created"`
	}
	_ = json.Unmarshal(rec2.Body.Bytes(), &out2)
	if len(out2.AgentsCreated) != 0 || out2.ProjectCreated {
		t.Fatalf("二次调用应全复用: created=%v projectCreated=%v", out2.AgentsCreated, out2.ProjectCreated)
	}
	if out2.ProjectID != out.ProjectID || out2.Members["domain_expert"] != out.Members["domain_expert"] {
		t.Fatalf("幂等复用应返回同 id: %v vs %v", out2, out)
	}

	// ③ 既有「本体工程师」（REQ-360 预设同名）复用不覆盖
	prev, _ := s.Store.GetAgent(out.Members["ontology_engineer"])
	prev.Instruction = "用户自定义指令"
	if _, err := s.Store.UpdateAgent(prev); err != nil {
		t.Fatal(err)
	}
	req3 := httptest.NewRequest(http.MethodPost, "/api/ontology/collab-pipeline", nil)
	rec3 := httptest.NewRecorder()
	s.createOntologyPipeline(rec3, req3)
	a3, _ := s.Store.GetAgent(out.Members["ontology_engineer"])
	if a3.Instruction != "用户自定义指令" {
		t.Fatal("同名复用不应覆盖既有 instruction（沿 REQ-360 幂等回落先例）")
	}
}
