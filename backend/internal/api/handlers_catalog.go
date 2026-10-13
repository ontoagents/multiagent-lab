package api

// handlers_catalog.go 权威本体库来源代理（REQ-316/M118）：
// GET /api/ontology-catalog/fetch?url=<encoded> —— 只读代理拉取权威本体源文件（TTL/RDF-XML），
// 返回 {filename, content_type, content} 供前端直接走既有导入管线（importOntologyContent，
// REQ-235 有损报告 / REQ-305 体积与规模防线自动生效）。目录元数据在前端常量（离线可用）。
// SSRF 防护：仅 http/https + host 解析全部 IP 须为公网（禁环回/私网/link-local）。

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const catalogFetchMaxBytes = 8 << 20 // 8MB：权威本体源文件上限（schema.org 级 ~数千概念亦在 REQ-305 限内）

// catalogFilename 依 URL 末段与 content-type 推断导入文件名（扩展名决定 importer 嗅探与 original 形态标注）。
func catalogFilename(rawURL, contentType string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "source.ttl"
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	name := segs[len(segs)-1]
	ct := strings.ToLower(contentType)
	switch {
	case strings.HasSuffix(name, ".ttl"), strings.HasSuffix(name, ".rdf"), strings.HasSuffix(name, ".owl"), strings.HasSuffix(name, ".n3"), strings.HasSuffix(name, ".nt"):
		return name
	case strings.Contains(ct, "turtle"):
		if name == "" {
			return "source.ttl"
		}
		return name + ".ttl"
	case strings.Contains(ct, "rdf+xml"), strings.Contains(ct, "xml"):
		if name == "" {
			return "source.rdf"
		}
		return name + ".rdf"
	default:
		if name == "" {
			return "source.ttl"
		}
		return name + ".ttl"
	}
}

// isPublicHost host 解析的全部 IP 须为公网单播（SSRF：禁环回/私网/link-local/未指定）。
func isPublicHost(host string) bool {
	ips, err := net.LookupHost(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, s := range ips {
		ip := net.ParseIP(s)
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return false
		}
	}
	return true
}

func (s *Server) catalogFetch(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url 必填且须为 http(s) 地址"})
		return
	}
	if !isPublicHost(u.Hostname()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "目标地址不可达或不允许（仅公网来源；禁内网/环回）"})
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, raw, nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("来源拉取失败: %v（权威站点可达性随网络环境，可稍后重试或手动下载后经「导入文件」引入）", err)})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("来源返回 %s（目录条目的 URL 随上游版本可能变化）", resp.Status)})
		return
	}
	ct := resp.Header.Get("Content-Type")
	if cl := resp.ContentLength; cl > catalogFetchMaxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": fmt.Sprintf("来源文件超过上限（%d MB）", catalogFetchMaxBytes>>20)})
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, catalogFetchMaxBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("来源读取中断: %v", err)})
		return
	}
	if int64(len(body)) > catalogFetchMaxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": fmt.Sprintf("来源文件超过上限（%d MB）", catalogFetchMaxBytes>>20)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"filename":     catalogFilename(raw, ct),
		"content_type": ct,
		"content":      string(body),
	})
}
