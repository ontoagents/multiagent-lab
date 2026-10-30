// REQ-360/M161 能力开关 handler 面单测：①normalizeOnchatMode 非法值 400 人话报错（create/update 双路）；
// ②内置行白名单放行 onchat_mode（迁移默认 guided 可经配置面关闭/切换，与普通智能体统一）。
package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestNormalizeOnchatModeRejected(t *testing.T) {
	s := newUnifyToolsFixture(t)
	body, _ := json.Marshal(map[string]any{"name": "x", "onchat_mode": "auto"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", bytes.NewReader(body))
	s.createAgent(w, req)
	if w.Code != 400 {
		t.Fatalf("非法 onchat_mode 创建应 400，got %d %s", w.Code, w.Body.String())
	}

	a, err := s.Store.CreateAgent(&store.Agent{Name: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]any{"name": a.Name, "onchat_mode": "bogus"})
	w = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", "/api/agents/"+a.ID, bytes.NewReader(body))
	req.SetPathValue("id", a.ID)
	s.updateAgent(w, req)
	if w.Code != 400 {
		t.Fatalf("非法 onchat_mode 更新应 400，got %d %s", w.Code, w.Body.String())
	}
}

func TestUpdateAgentOnchatModePaths(t *testing.T) {
	s := newUnifyToolsFixture(t)
	// 普通行：合法值落库
	a, err := s.Store.CreateAgent(&store.Agent{Name: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"name": a.Name, "onchat_mode": "quick"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/agents/"+a.ID, bytes.NewReader(body))
	req.SetPathValue("id", a.ID)
	s.updateAgent(w, req)
	if w.Code != 200 {
		t.Fatalf("普通行合法更新应 200：%d %s", w.Code, w.Body.String())
	}
	got, _ := s.Store.GetAgent(a.ID)
	if got.OnchatMode != "quick" {
		t.Fatalf("普通行 onchat_mode 应落 quick，got %q", got.OnchatMode)
	}

	// 内置行：白名单放行（关闭/切换均可；迁移默认 guided）
	body, _ = json.Marshal(map[string]any{"onchat_mode": ""})
	w = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", "/api/agents/builtin-assistant", bytes.NewReader(body))
	req.SetPathValue("id", "builtin-assistant")
	s.updateAgent(w, req)
	if w.Code != 200 {
		t.Fatalf("内置行 onchat_mode 更新应 200：%d %s", w.Code, w.Body.String())
	}
	b, _ := s.Store.GetAgent("builtin-assistant")
	if b.OnchatMode != "" {
		t.Fatalf("内置行应可关闭，got %q", b.OnchatMode)
	}
}
