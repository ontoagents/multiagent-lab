package api

// REQ-418/M214：协议面可视化端点——「设置-协议」总览面的数据源。
//   GET /api/a2a/cards  全部智能体 agent card 一次性预览（发现端点 /api/a2a/agents 的展开形态；
//                       卡片 JSON 与 server 端 AgentCard 同源，只读不编辑）
//   GET /api/a2a/tasks  a2a_task 列表（外部经协议进来的任务界面可见；updated_at 倒序，
//                       payload 内 artifact/error 随行——终态产物查看）
// 两者均沿 /api/* accessAuth 鉴权（与 A2A 服务面同域口径）。

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// listA2aCards GET /api/a2a/cards。
func (s *Server) listA2aCards(w http.ResponseWriter, _ *http.Request) {
	agents, err := s.Store.ListAgents()
	if err != nil {
		writeErr(w, err)
		return
	}
	type cardEntry struct {
		AgentID   string         `json:"agent_id"`
		Name      string         `json:"name"`
		IsBuiltin bool           `json:"is_builtin"`
		Card      map[string]any `json:"card"`
	}
	out := make([]cardEntry, 0, len(agents))
	for _, a := range agents {
		out = append(out, cardEntry{AgentID: a.ID, Name: a.Name, IsBuiltin: a.IsBuiltin, Card: s.A2A.AgentCard(a)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"cards": out, "count": len(out)})
}

// listA2aTasks GET /api/a2a/tasks?limit=100。
func (s *Server) listA2aTasks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := s.Store.ListA2ATasks(limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	type taskOut struct {
		store.A2ATaskRow
		ArtifactText string `json:"artifact_text,omitempty"`
		ErrorText    string `json:"error_text,omitempty"`
	}
	out := make([]taskOut, 0, len(rows))
	for _, row := range rows {
		var pl store.A2APayload
		_ = json.Unmarshal([]byte(row.Payload), &pl)
		out = append(out, taskOut{A2ATaskRow: *row, ArtifactText: pl.ArtifactText, ErrorText: pl.ErrorText})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": out, "count": len(out)})
}
