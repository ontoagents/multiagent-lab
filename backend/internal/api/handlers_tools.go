package api

import (
	"net/http"
)

// toolEntryDTO /api/tools 输出 DTO（Entry 的 New 是函数字段，不能直接 JSON 序列化）。
type toolEntryDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Group       string `json:"group,omitempty"` // REQ-451：能力分组标签（如「平台知识」，前端工具选择徽标）
}

// GET /api/tools：工具注册表列表（前端 Agent 配置勾选，REQ-24）。
func (s *Server) listTools(w http.ResponseWriter, r *http.Request) {
	entries := s.Tools.List()
	out := make([]toolEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, toolEntryDTO{ID: e.ID, Name: e.Name, Description: e.Description, Source: string(e.Source), Group: e.Group})
	}
	writeJSON(w, http.StatusOK, out)
}
