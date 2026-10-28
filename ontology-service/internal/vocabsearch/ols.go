// Package vocabsearch REQ-261/M259 外部词表 registry 通道扩展：OLS（EBI Ontology Lookup
// Service）API v2 搜索代理薄层——与 LOV（vocabsearch.go）同模式：进程内缓存+可注入 Base+卡片
// 投影。OLS 强项=OWLFed 生物医学标准本体（EFO/HPO/CHEBI/UBERON 等数百库），与 LOV（词表
// 定义聚合）互补；前端「参考素材/词表搜索」源切换消费（req-261）。
package vocabsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultOLS EBI OLS 官方 API v2 根。
const DefaultOLS = "https://www.ebi.ac.uk/ols4/api"

// SearchOntologies OLS 本体关键词搜索（/ontologies?q=；v2 响应内嵌 _embedded.offologies
// 容错缺失）。复用 Client 的缓存（key 加前缀 ols: 防与 LOV 查询撞键）。
func (c *Client) SearchOntologies(ctx context.Context, q string) ([]Card, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, fmt.Errorf("q 必填")
	}
	cacheKey := "ols:" + q
	if body, ok := c.cached(cacheKey); ok {
		return decodeOLSCards(body)
	}
	u := strings.TrimRight(c.OLSBase, "/") + "/ontologies?q=" + url.QueryEscape(q) + "&size=20"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OLS 上游不可达: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("OLS 上游 %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	c.store(cacheKey, body)
	return decodeOLSCards(body)
}

// decodeOLSCards OLS v2 响应 → 卡片列表（_embedded.ontologies[].ontologyId/iri/label/
// description/config.description；字段缺省容错——v2 各本体描述落在不同字段）。
func decodeOLSCards(body []byte) ([]Card, error) {
	var raw struct {
		Embedded struct {
			Ontologies []struct {
				OntologyID  string `json:"ontologyId"`
				IRI         string `json:"iri"`
				Label       string `json:"label"`
				Description string `json:"description"`
				Config      struct {
					Title       string `json:"title"`
					Description string `json:"description"`
				} `json:"config"`
			} `json:"ontologies"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("OLS 响应解析失败: %w", err)
	}
	out := make([]Card, 0, len(raw.Embedded.Ontologies))
	for _, r := range raw.Embedded.Ontologies {
		desc := r.Description
		if desc == "" {
			desc = r.Config.Description
		}
		title := r.Label
		if title == "" {
			title = r.Config.Title
		}
		if r.OntologyID == "" && r.IRI == "" {
			continue
		}
		out = append(out, Card{
			URI: r.IRI, Prefix: r.OntologyID, Title: title, Description: desc,
		})
	}
	return out, nil
}

var _ = time.Second // 保持 time 引用（cacheEntry.at 类型同源）
