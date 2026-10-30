package rest

// REQ-542/M349 构建面只读闭包预览端点（103 号 §6.5 步骤二前半）：
// GET /api/ontologies/{id}/closure-preview——运行面 expandClosure 算法前移的只读预览，
// 三出料=全量依赖闭包图（nodes+edges）+解析后 schema 预览（pin/head 解析合成）+
// 循环引用专项报告。不回写不阻断（D-O27 边界扩写，03 §5 行注）：悬空引用入报告不拦截、
// 零写副作用（不建资产不改 spec 不触发门禁）；与 REQ-278 /impact（版本 diff+引用波及面）
// 为扩面衔接——本端点补全量闭包图与解析后 schema 视图。

import (
	"net/http"
)

func (s *Server) closurePreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	prev, err := s.Store.BuildClosurePreview(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, prev)
}
