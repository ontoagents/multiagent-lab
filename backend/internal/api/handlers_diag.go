package api

// REQ-410/M206：诊断自助包——GET /api/diagnostics/export 生成 JSON 诊断文件（zip 评估留后续，
// JSON 单文件即够 issue 附带；脱敏=模型连接只出 has_key/name 不出明文与密文）。
// 内容：healthz 快照+最近 run.error/verify.failed 样本+脱敏配置+运行环境（版本/OS/启动参数白名单）。

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func (s *Server) exportDiagnostics(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"go_version":   runtime.Version(),
		"os":           runtime.GOOS + "/" + runtime.GOARCH,
	}
	// healthz 快照（复用 healthz 逻辑——四依赖状态）
	out["healthz"] = s.healthz(r.Context())
	// 脱敏配置：模型连接仅 name/conn_type/enabled/has_key（不出 baseURL 全串与 key）
	conns, err := s.Store.ListConnections()
	if err == nil {
		sanitized := []map[string]any{}
		for _, c := range conns {
			sanitized = append(sanitized, map[string]any{
				"name": c.Name, "conn_type": c.ConnType, "enabled": c.Enabled, "has_key": c.HasKey,
			})
		}
		out["model_connections"] = sanitized
	}
	// 最近错误样本（run.error/verify.failed 各最近 5 条）
	sample := []map[string]any{}
	for _, typ := range []string{"run.error", "verify.failed"} {
		evs, _, err := s.Store.ListEventsQ("", store.EventQuery{Type: typ, Limit: 5, Order: "desc"})
		if err == nil {
			for _, e := range evs {
				sample = append(sample, map[string]any{"type": e.Type, "run_id": e.RunID, "conversation_id": e.ConversationID, "created_at": e.CreatedAt, "data": json.RawMessage(e.Data)})
			}
		}
	}
	out["recent_errors"] = sample
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=diagnostics-"+time.Now().UTC().Format("20060102-150405")+".json")
	_ = json.NewEncoder(w).Encode(out)
}
