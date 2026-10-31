package api

import (
	"net/http"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// usageStats 使用统计（GET /api/stats/usage?group_by=model|agent|project|conversation&from=YYYY-MM-DD&to=YYYY-MM-DD）。
// from/to 可选、闭区间；契约：{"rows":[{key,label,calls,prompt_tokens,completion_tokens,total_tokens}]}。
func (s *Server) usageStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	groupBy := q.Get("group_by")
	switch groupBy {
	case "model", "agent", "project", "conversation", "member": // REQ-204/M39 C6 + REQ-557/M364 过程账本成员维度
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "group_by must be model|agent|project|conversation|member"})
		return
	}

	from, ok := parseUsageDate(q.Get("from"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from/to 必须为 YYYY-MM-DD"})
		return
	}
	to, ok := parseUsageDate(q.Get("to"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from/to 必须为 YYYY-MM-DD"})
		return
	}
	// 闭区间日期字符串（YYYY-MM-DD）字典序即时间序，可直接比较
	if from != "" && to != "" && from > to {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from 不能晚于 to"})
		return
	}

	// REQ-557/M364：group_by=member 过程账本成员维度（数据源=run.finished.usage_by_agent——
	// 与 agent 维度正交：member=run 内成员含委派子成员，agent=发起 run 的智能体）
	if groupBy == "member" {
		members, err := s.Store.MemberUsageStats(store.UsageRange{From: from, To: to})
		if err != nil {
			writeErr(w, err)
			return
		}
		if members == nil {
			members = []*store.MemberUsage{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"rows": members, "group_by": "member"})
		return
	}
	rows, err := s.Store.UsageStats(groupBy, store.UsageRange{From: from, To: to})
	if err != nil {
		writeErr(w, err)
		return
	}
	if rows == nil {
		rows = []store.UsageRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

// parseUsageDate 校验并规范化 YYYY-MM-DD 日期参数；空串表示未提供。
func parseUsageDate(v string) (string, bool) {
	if v == "" {
		return "", true
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return "", false
	}
	return t.Format("2006-01-02"), true
}


// runMetrics GET /api/stats/run/{run_id}/metrics——单 run 过程账本三出料（REQ-557/M364；107 号 §8.3）：
// 参与度（usage_by_agent 发射点归因+tool.call 计数+enter/exit 在位时长）/介入频次（审批·挂起·
// 插话·拦截·抢占计数+决议比+委派 phase 分布）/阶段耗时（纯派生）。前端不复算（单源口径）。
func (s *Server) runMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := s.Store.RunMetrics(r.PathValue("run_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if m == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "该 run 无落库事件（run_id 有误）"})
		return
	}
	writeJSON(w, http.StatusOK, m)
}
