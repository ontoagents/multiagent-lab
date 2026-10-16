// retriever.go 向量检索段（REQ-343/OE-1，64 号 §4.3 模块 B）：
// 元素 SearchText → 主平台 embedding 代理（/api/ontology-llm/embed，连接与密钥归主平台）
// → L2 归一 + 内积 top-k（TopK，ontorag.go）。检索片段只读上下文不变式：只进 prompt，不改写。
package ontorag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

// Client 检索客户端（PlatformURL=主平台地址，与 llmcreate.Creator 同一装配口径）。
type Client struct {
	PlatformURL string
	HTTP        *http.Client
}

// NewClient 构造检索客户端。
func NewClient(platformURL string) *Client {
	return &Client{PlatformURL: platformURL, HTTP: &http.Client{}}
}

// Embed 批量向量化（经主平台 embedding 代理；默认 embedding 连接在主平台侧解析）。
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	// 代理单批上限 512（handlers_ontology.go embedOntologyLLM），客户端分批兜底
	const batch = 512
	out := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += batch {
		end := start + batch
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := c.embedOnce(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

func (c *Client) embedOnce(ctx context.Context, texts []string) ([][]float64, error) {
	body, _ := json.Marshal(map[string]any{"texts": texts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.PlatformURL, "/")+"/api/ontology-llm/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用主平台 embedding 代理失败: %w", err)
	}
	defer resp.Body.Close()
	var res struct {
		Vectors [][]float64 `json:"vectors"`
		Error   string      `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("embedding 代理响应解析失败: %w", err)
	}
	if res.Error != "" {
		return nil, fmt.Errorf("embedding 代理错误: %s", res.Error)
	}
	if len(res.Vectors) != len(texts) {
		return nil, fmt.Errorf("embedding 代理返回数量不匹配（期望 %d 得到 %d）", len(texts), len(res.Vectors))
	}
	return res.Vectors, nil
}

// Index 元素集向量化（抽取+序列化+embedding 一次到位；返回元素集与同序向量，供 TopK 检索）。
func (c *Client) Index(ctx context.Context, ontologyID string, sp *pkgspec.Spec) ([]Element, [][]float64, error) {
	elements := ElementsFromSpec(ontologyID, sp)
	if len(elements) == 0 {
		return nil, nil, fmt.Errorf("目标本体无 TBox 元素（概念/关系/数据属性为空），无可检索上下文")
	}
	texts := make([]string, len(elements))
	for i, e := range elements {
		texts[i] = e.SearchText()
	}
	vecs, err := c.Embed(ctx, texts)
	if err != nil {
		return nil, nil, err
	}
	return elements, vecs, nil
}

// Retrieve 查询检索（CQ 编码 → top-k 命中）。
func (c *Client) Retrieve(ctx context.Context, query string, elements []Element, vectors [][]float64, topK int) ([]Hit, error) {
	qv, err := c.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(qv) == 0 {
		return nil, fmt.Errorf("查询向量化返回空")
	}
	return TopK(qv[0], elements, vectors, topK), nil
}
