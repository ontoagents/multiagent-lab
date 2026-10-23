package api

// REQ-208/M42 余项：可继续子 Agent（continuation）REST 面——会话 UI 委派卡片数据源。
// 协调者模型走 delegate_* 工具族（chat/delegate_tools.go）；人工侧经本组端点查看/
// 插话/打断/恢复。归属校验：会话端点天然限定 parent；delegates/{id} 直查校验可见性
// （父会话或子会话本人可见——单机单用户平台从简，仅校验存在性）。

import (
	"net/http"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// listDelegates GET /api/conversations/{id}/delegates：父会话的后台委派列表（新→旧）。
func (s *Server) listDelegates(w http.ResponseWriter, r *http.Request) {
	if _, err := s.Store.GetConversation(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.Store.ListDelegatesByParent(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []*store.Delegate{}
	}
	writeJSON(w, http.StatusOK, list)
}

// getDelegate GET /api/delegates/{id}：委派详情+子会话最终回复全文（collect 视图）。
func (s *Server) getDelegate(w http.ResponseWriter, r *http.Request) {
	if s.Delegates == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "后台委派未启用"})
		return
	}
	d, result, err := s.Delegates.Collect(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"delegate": d, "result": result})
}

// steerDelegate POST /api/delegates/{id}/steer {message}：人工中途引导。
func (s *Server) steerDelegate(w http.ResponseWriter, r *http.Request) {
	if s.Delegates == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "后台委派未启用"})
		return
	}
	var in struct {
		Message string `json:"message"`
	}
	if err := decodeJSON(r, &in); err != nil || in.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message 必填"})
		return
	}
	d, err := s.Delegates.Steer(r.PathValue("id"), in.Message)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// interruptDelegate POST /api/delegates/{id}/interrupt：人工打断。
func (s *Server) interruptDelegate(w http.ResponseWriter, r *http.Request) {
	if s.Delegates == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "后台委派未启用"})
		return
	}
	d, err := s.Delegates.Interrupt(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// continueDelegate POST /api/delegates/{id}/continue {instruction?}：冷恢复/续跑。
func (s *Server) continueDelegate(w http.ResponseWriter, r *http.Request) {
	if s.Delegates == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "后台委派未启用"})
		return
	}
	var in struct {
		Instruction string `json:"instruction"`
	}
	_ = decodeJSON(r, &in) // instruction 可选
	d, err := s.Delegates.Continue(r.PathValue("id"), in.Instruction)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
