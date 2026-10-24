package api

// REQ-400/M196：配置保存乐观锁（If-Match）——三实体 PUT 端点共用。
// 语义：客户端携带 If-Match: <上次读到的 updated_at>；与库内现值不一致=并发修改 →
// 409（治 REQ-219/SandboxPanel/REQ-264 三起「后保存覆盖前保存」清零事故的根因面）。
// 头缺省=不校验（存量客户端/脚本兼容——前端主保存链路随本批透传后即受保护）。
// 本体 Spec overlay base_version 为同族先例（docs/20 2.37）。

import (
	"net/http"
	"strings"
)

// checkIfMatch 校验 If-Match 与现值；返回 false 时已写 409 响应。
// 头缺省（无 If-Match）=不校验放行（存量兼容；前端主链路透传后生效）。
func checkIfMatch(w http.ResponseWriter, r *http.Request, currentUpdatedAt string) bool {
	got := strings.TrimSpace(r.Header.Get("If-Match"))
	if got == "" {
		return true
	}
	got = strings.Trim(got, `"`)
	if got == "" {
		return true
	}
	if got != currentUpdatedAt {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "配置已被其他窗口修改（If-Match 不一致）——请刷新后基于最新配置重新编辑保存（REQ-400 乐观锁）",
			"current": currentUpdatedAt,
		})
		return false
	}
	return true
}
