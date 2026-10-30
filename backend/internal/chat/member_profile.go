package chat

// REQ-429/M225：外部成员能力画像合成——A2A agent card + CLI 探测与注册表能力矩阵统一为
// 成员画像（技能/在环性/成本档），注入协调者委派决策（描述即路由语义——00 档既定原则，
// 路由裁量由协调者 LLM 按画像作出）并经连接器面呈现。成本档如实标注：外部成员成本由
// 对端自管（CLI 自身配置 / A2A 对端计量），平台无计量口径。

import (
	"encoding/json"
	"strings"

	"github.com/ontoagents/ontoagents/backend/internal/a2a"
	"github.com/ontoagents/ontoagents/backend/internal/inference"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// ExternalMemberProfile 外部成员能力画像（kind=cli|a2a；其他 kind 返回 nil）。
// cli：探测载荷（backend/version）+ 注册表能力矩阵；a2a：agent card（skills/transport/streaming/properties）。
func ExternalMemberProfile(c *store.Connector, reg *inference.Registry) map[string]any {
	if c == nil {
		return nil
	}
	switch c.Kind {
	case store.ConnectorKindCLI:
		backend, _ := c.Config["backend"].(string)
		p := map[string]any{
			"channel": "cli",
			"cost":    "由对端 CLI 自管（平台无计量）",
			"inloop":  "stream-json 行级转译（流式在环）",
		}
		var probe struct {
			Type    string `json:"type"`
			Version string `json:"version"`
		}
		if json.Unmarshal([]byte(c.CardJSON), &probe) == nil && probe.Type == "cli-probe" {
			if probe.Version != "" {
				p["version"] = probe.Version
			}
		}
		if reg != nil {
			if b := reg.Get(backend); b != nil {
				caps := b.Capabilities()
				p["backend"] = backend
				p["matrix"] = map[string]any{
					"chat": caps.Chat, "stream": caps.Stream,
					"skills_mode": caps.SkillsMode, "mcp_mode": caps.MCPMode,
				}
			}
		}
		return p
	case store.ConnectorKindA2A:
		p := map[string]any{
			"channel": "a2a",
			"cost":    "由对端自管（平台无计量）",
		}
		var card a2a.AgentCard
		if json.Unmarshal([]byte(c.CardJSON), &card) == nil {
			if card.ProtocolVersion != "" {
				p["protocol"] = card.ProtocolVersion
			}
			p["transport"] = cardTransport(&card)
			p["streaming"] = card.Capabilities.Streaming
			if card.Capabilities.Streaming {
				p["inloop"] = "对端支持流式（当前同步 send+tasks/get 轮询至终态）"
			} else {
				p["inloop"] = "同步 send+tasks/get 轮询至终态（对端无流式）"
			}
			if len(card.Skills) > 0 {
				names := make([]string, 0, len(card.Skills))
				for _, sk := range card.Skills {
					if strings.TrimSpace(sk.Name) != "" {
						names = append(names, sk.Name)
					}
				}
				p["skills"] = names
			}
			if len(card.Properties) > 0 {
				p["properties"] = card.Properties
			}
		}
		return p
	}
	return nil
}

// profileMatrixSummary 能力矩阵摘要（CLI 成员工具描述注入；路由信号）。
func profileMatrixSummary(caps inference.Capabilities) string {
	parts := []string{"对话✓"}
	if caps.Stream {
		parts = append(parts, "流式✓")
	} else {
		parts = append(parts, "流式✗")
	}
	parts = append(parts, "技能:"+caps.SkillsMode, "MCP:"+caps.MCPMode)
	return strings.Join(parts, "/")
}

// cardTransport 卡片传输形态（缺省 JSONRPC；api 层同名助手口径一致）。
func cardTransport(card *a2a.AgentCard) string {
	if card.PreferredTransport == "" {
		return "JSONRPC"
	}
	return card.PreferredTransport
}
