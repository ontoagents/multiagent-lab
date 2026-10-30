// REQ-214/M46：外部连接器 REST API（设置页「连接器」分区 + AgentSidePanel 授权勾选数据源）。
// 凭据入参仅落库瞬间存在（AES-256-GCM 加密），响应永不回传明文（has_credentials 布尔替代）；
// 连接测试沿 REQ-191 先例（探测可用性并回写 status，不做重试风暴）。
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/a2a"
	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/connector"
	"github.com/ontoagents/ontoagents/backend/internal/inference"
	"github.com/ontoagents/ontoagents/backend/internal/store"
	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

// connectorOut 列表/详情出参（凭据脱敏）。
type connectorOut struct {
	ID             string         `json:"id"`
	Kind           string         `json:"kind"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Config         map[string]any `json:"config"`
	HasCredentials bool           `json:"has_credentials"`
	Status         string         `json:"status"`
	StatusDetail   string         `json:"status_detail"`
	Tools          []string       `json:"tools"`     // REQ-214 P2：工具名清单（test 落库；授权前知情）
	TestedAt       string         `json:"tested_at"` // REQ-214 P2：最近测试时间（状态时效）
	Card           map[string]any `json:"card,omitempty"` // REQ-416/M212：a2a 注册握手的 agent card（只读预览）
	Profile        map[string]any `json:"profile,omitempty"` // REQ-429/M225：成员能力画像（技能/在环性/成本档——连接器面呈现）
	IsBuiltin      bool           `json:"is_builtin"`
	Refs           []string       `json:"refs"` // 引用该连接器的 agent 名（删除保护）
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

func (s *Server) connectorOut(c *store.Connector) connectorOut {
	refs, err := s.Store.ConnectorRefs(c.ID)
	if err != nil {
		refs = []string{}
	}
	if refs == nil {
		refs = []string{}
	}
	var card map[string]any
	if c.CardJSON != "" {
		_ = json.Unmarshal([]byte(c.CardJSON), &card)
	}
	profile := chat.ExternalMemberProfile(c, s.chatRegistry())
	return connectorOut{
		ID: c.ID, Kind: c.Kind, Name: c.Name, Description: c.Description,
		Config: c.Config, HasCredentials: c.HasCredentials,
		Status: c.Status, StatusDetail: c.StatusDetail, Tools: c.Tools, TestedAt: c.TestedAt,
		Card: card,
		Profile: profile,
		IsBuiltin: c.IsBuiltin,
		Refs: refs, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// listConnectors GET /api/connectors。
func (s *Server) listConnectors(w http.ResponseWriter, r *http.Request) {
	cs, err := s.Store.ListConnectors()
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]connectorOut, 0, len(cs))
	for _, c := range cs {
		out = append(out, s.connectorOut(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": out})
}

type connectorIn struct {
	Kind        string         `json:"kind"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Config      map[string]any `json:"config"`
	Credentials map[string]any `json:"credentials"` // 明文仅此一瞬，加密落库后丢弃
}

// validateConnectorIn kind/name/config 结构校验（kind 创建后不可改——config/credentials 结构随之绑定）。
func validateConnectorIn(in *connectorIn) *store.HTTPError {
	kindOK := false
	for _, k := range store.ConnectorKinds {
		if in.Kind == k {
			kindOK = true
			break
		}
	}
	if !kindOK {
		return &store.HTTPError{Status: http.StatusBadRequest, Msg: "kind 取值须为 mcp|kubernetes|ssh|a2a|cli|stdio"}
	}
	if in.Name == "" {
		return &store.HTTPError{Status: http.StatusBadRequest, Msg: "name 必填（连接器实例名，装配前缀槽位）"}
	}
	switch in.Kind {
	case store.ConnectorKindMCP:
		if urlOf(in.Config) == "" {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "自定义 MCP 连接器须提供 config.url（Streamable HTTP MCP 端点）"}
		}
	case store.ConnectorKindA2A:
		if urlOf(in.Config) == "" {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "A2A 连接器须提供 config.url（外部智能体 A2A 端点，如 http://host:8080/api/a2a/agents/{id}）"}
		}
		if ci := strOf(in.Config, "context_injection"); ci != "" && ci != "auto" {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "context_injection 取值须为 空（关，默认）| auto（委派时注入项目上下文——内容随任务外发对端，请确认数据边界）"}
		}
	case store.ConnectorKindStdio:
		// REQ-433/M229：stdio 桥接——command 白名单校验（ParseStdioConfig 同口径）
		if _, serr := tool.ParseStdioConfig(in.Config); serr != nil {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "stdio 连接器：" + serr.Error()}
		}
	case store.ConnectorKindCLI:
		if strOf(in.Config, "backend") == "" {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "CLI 连接器须提供 config.backend（外部 CLI 后端名：claude-code/opencode/aider/deepseek-harness）"}
		}
		if strOf(in.Config, "backend") == "eino-adk" {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "CLI 连接器 backend 须为外部 CLI 型（eino-adk 为平台内管线，不经成员通道）"}
		}
		if sm := strOf(in.Config, "sandbox_mode"); sm != "" && sm != "docker" {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "CLI 连接器 sandbox_mode 取值须为 空（宿主进程，默认）| docker（沙箱世界，三前置闸门——docker 可达/绑定目录/审批档）"}
		}
		if ci := strOf(in.Config, "context_injection"); ci != "" && ci != "auto" {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "context_injection 取值须为 空（关，默认）| auto（委派时注入项目上下文——内容随任务外发对端，请确认数据边界）"}
		}
	case store.ConnectorKindKubernetes:
		if strOf(in.Config, "kubeconfig_path") == "" && credStr(in.Credentials, "kubeconfig") == "" {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "Kubernetes 连接器须提供 config.kubeconfig_path 或 credentials.kubeconfig（二选一）"}
		}
	case store.ConnectorKindSSH:
		if strOf(in.Config, "host") == "" {
			return &store.HTTPError{Status: http.StatusBadRequest, Msg: "SSH 连接器须提供 config.host"}
		}
	}
	return nil
}

func urlOf(cfg map[string]any) string { return strOf(cfg, "url") }

func strOf(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	v, _ := cfg[key].(string)
	return v
}

func credStr(creds map[string]any, key string) string { return strOf(creds, key) }

// encryptCredentials 凭据 map → AES-256-GCM 密文（空 map 返回 nil=不写凭据）。
func (s *Server) encryptCredentials(creds map[string]any) ([]byte, error) {
	if len(creds) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}
	return s.Box.Encrypt(string(b))
}

// createConnector POST /api/connectors。
func (s *Server) createConnector(w http.ResponseWriter, r *http.Request) {
	var in connectorIn
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if he := validateConnectorIn(&in); he != nil {
		writeErr(w, he)
		return
	}
	enc, err := s.encryptCredentials(in.Credentials)
	if err != nil {
		writeErr(w, err)
		return
	}
	c := &store.Connector{
		Kind: in.Kind, Name: in.Name, Description: in.Description,
		Config: in.Config, CredentialsEncrypted: enc,
	}
	if in.Credentials != nil && len(in.Credentials) > 0 {
		c.Status = "unknown" // 新凭据未验证
	}
	// REQ-416/M212：a2a 注册闸门——注册时真握手拉取 agent card，卡片不可达即注册失败
	if in.Kind == store.ConnectorKindA2A {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		card, raw, ferr := s.handshakeA2A(ctx, c)
		if ferr != nil {
			writeErr(w, &store.HTTPError{Status: http.StatusBadRequest, Msg: "A2A 注册失败：" + ferr.Error()})
			return
		}
		c.CardJSON = string(raw)
		c.Tools = skillNamesOf(card)
		c.Status = "ok"
		c.StatusDetail = "握手成功：" + card.Name
	}
	// REQ-426/M222：cli 注册闸门——注册时 PATH 探测真握手（LookPath + --version 取版本），
	// 能力探测入库（backend/版本/路径落 card JSON 供工具描述合成与前端预览），不可达注册失败如实报
	if in.Kind == store.ConnectorKindCLI {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		detail, raw, ferr := s.probeCLIBackend(ctx, c)
		if ferr != nil {
			writeErr(w, &store.HTTPError{Status: http.StatusBadRequest, Msg: "CLI 注册失败：" + ferr.Error()})
			return
		}
		c.CardJSON = string(raw)
		c.Status = "ok"
		c.StatusDetail = "探测成功：" + detail
	}
	created, err := s.Store.CreateConnector(c)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.connectorOut(created))
}

// errConnectorBuiltin 内置实例不可删除/改核心字段。
var errConnectorBuiltin = &store.HTTPError{Status: http.StatusBadRequest, Msg: "内置连接器不可删除（可编辑描述）"}

// updateConnector PUT /api/connectors/{id}。
// 凭据口径沿模型 API Key 先例：credentials 缺省/空 = 保留原值；内置行 name/kind/config/凭据锁死（仅描述可改）。
func (s *Server) updateConnector(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.GetConnector(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var in connectorIn
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if c.IsBuiltin {
		c.Description = in.Description
		updated, err := s.Store.UpdateConnector(c)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s.connectorOut(updated))
		return
	}
	kind := c.Kind
	name := c.Name
	if in.Name != "" {
		name = in.Name
	}
	config := c.Config
	if in.Config != nil {
		config = in.Config
	}
	// 凭据：未携带（nil 或空对象）= 保留原值；携带 = 重新加密覆盖
	if len(in.Credentials) > 0 {
		enc, err := s.encryptCredentials(in.Credentials)
		if err != nil {
			writeErr(w, err)
			return
		}
		c.CredentialsEncrypted = enc
		c.Status = "unknown"
	}
	c.Kind = kind
	c.Name = name
	c.Description = in.Description
	c.Config = config
	updated, err := s.Store.UpdateConnector(c)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.connectorOut(updated))
}

// deleteConnector DELETE /api/connectors/{id}：内置拦截；被 agent 引用时 409 附引用者。
func (s *Server) deleteConnector(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.GetConnector(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if c.IsBuiltin {
		writeErr(w, errConnectorBuiltin)
		return
	}
	refs, err := s.Store.ConnectorRefs(c.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(refs) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "连接器仍被智能体引用，请先在智能体侧板取消授权",
			"refs":  refs,
		})
		return
	}
	if err := s.Store.DeleteConnector(c.ID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// probeConnector 按 kind 探测连接（test 与 preview 共用）。
// mcp = 真 MCP 握手（initialize + list_tools，取回工具名清单）；kubernetes = kubectl 集群探针；
// ssh = 拨号 + 认证；a2a = agent card 真握手（REQ-416/M212，回读卡片与技能名清单）；
// cli = PATH 探测 + --version（REQ-426/M222，能力入库刷新 card）。
// 探测仅验证可达性，不做重试风暴（REQ-191 口径）。
func (s *Server) probeConnector(ctx context.Context, c *store.Connector) (ok bool, detail string, tools []string, card map[string]any) {
	switch c.Kind {
	case store.ConnectorKindMCP:
		headers := s.mcpHeaders(c)
		bts, ferr := tool.FetchMCPToolsWithHeaders(ctx, c.Name, urlOf(c.Config), headers, 12*time.Second)
		ok = ferr == nil
		if ok {
			detail = "可达，暴露 " + strconv.Itoa(len(bts)) + " 个工具"
			names := make([]string, 0, len(bts))
			for _, bt := range bts {
				if ti, ierr := bt.Info(ctx); ierr == nil && ti != nil && ti.Name != "" {
					names = append(names, ti.Name)
				}
			}
			tools = names
		} else {
			detail = "不可达：" + ferr.Error()
		}
		return ok, detail, tools, nil
	case store.ConnectorKindA2A:
		hd, raw, ferr := s.handshakeA2A(ctx, c)
		if ferr != nil {
			return false, "不可达：" + ferr.Error(), nil, nil
		}
		names := skillNamesOf(hd)
		_ = json.Unmarshal(raw, &card)
		return true, fmt.Sprintf("握手成功：%s（%d 项技能，transport %s）", hd.Name, len(names), transportOf(hd)), names, card
	case store.ConnectorKindKubernetes:
		ok, detail = s.testKubernetesConnector(ctx, c)
		if ok {
			ro, _ := c.Config["read_only"].(bool)
			tools = connector.KubernetesToolNames(ro)
		}
		return ok, detail, tools, nil
	case store.ConnectorKindSSH:
		ok, detail = s.testSSHConnector(ctx, c)
		if ok {
			tools = connector.SSHToolNames()
		}
		return ok, detail, tools, nil
	case store.ConnectorKindStdio:
		// REQ-433/M229：stdio 探测 = spawn+initialize+tools/list 真握手（工具清单入库）
		cfg, serr := tool.ParseStdioConfig(c.Config)
		if serr != nil {
			return false, "配置不符：" + serr.Error(), nil, nil
		}
		names, defs, ferr := tool.ProbeStdioMCP(ctx, cfg, 30*time.Second)
		if ferr != nil {
			return false, "spawn/握手失败：" + ferr.Error(), nil, nil
		}
		card = map[string]any{"type": "stdio-probe", "command": strOf(c.Config, "command"), "tools": names, "count": len(names)}
		tools = defsToNames(defs)
		return true, fmt.Sprintf("握手成功：暴露 %d 个工具", len(names)), tools, card
	case store.ConnectorKindCLI:
		// REQ-426/M222：cli 探测 = backend 合法性 + PATH + --version（真握手，能力入库刷新 card）
		detail, raw, ferr := s.probeCLIBackend(ctx, c)
		if ferr != nil {
			return false, "不可达：" + ferr.Error(), nil, nil
		}
		_ = json.Unmarshal(raw, &card)
		return true, "PATH 探测成功：" + detail, nil, card
	default:
		return false, "未知连接器类型 " + c.Kind, nil, nil
	}
}

// handshakeA2A a2a 连接器 agent card 真握手（凭据头沿 credentials.headers 先例注入）。
func (s *Server) handshakeA2A(ctx context.Context, c *store.Connector) (*a2a.AgentCard, []byte, error) {
	return a2a.FetchAgentCard(ctx, urlOf(c.Config), s.mcpHeaders(c), 12*time.Second)
}

// chatRegistry 成员画像合成的注册表访问（Chat 未接时 nil——画像降级为 card 原文）。
func (s *Server) chatRegistry() *inference.Registry {
	if s.Chat == nil {
		return nil
	}
	return s.Chat.Inference
}

// probeCLIBackend cli 连接器真握手（REQ-426/M222）：backend 须为注册表内外部 CLI 型，
// 再走适配器 Probe（PATH 探测 + --version）。返回（可读摘要, 探测 JSON 原文, 错误）——
// JSON 落 card 字段（cli-probe 载荷：backend/available/version/path），工具描述合成与前端预览共用。
func (s *Server) probeCLIBackend(ctx context.Context, c *store.Connector) (string, []byte, error) {
	backend := strOf(c.Config, "backend")
	b := s.Chat.Inference.Get(backend)
	if b == nil {
		return "", nil, fmt.Errorf("backend %q 不是已注册的外部 CLI 后端（可选：claude-code/opencode/aider/deepseek-harness）", backend)
	}
	pr := b.Probe(ctx)
	payload, _ := json.Marshal(map[string]any{
		"type": "cli-probe", "backend": backend,
		"available": pr.Available, "version": pr.Version, "path": pr.Path, "reason": pr.Reason,
	})
	if !pr.Available {
		return "", payload, fmt.Errorf("%s", pr.Reason)
	}
	return pr.Version + "（" + pr.Path + "）", payload, nil
}

// skillNamesOf 卡片技能名清单（tools 列承载——授权前知情沿 REQ-214 P2 口径）。
func skillNamesOf(card *a2a.AgentCard) []string {
	names := make([]string, 0, len(card.Skills))
	for _, sk := range card.Skills {
		if strings.TrimSpace(sk.Name) != "" {
			names = append(names, sk.Name)
		}
	}
	return names
}

func transportOf(card *a2a.AgentCard) string {
	if card.PreferredTransport == "" {
		return "JSONRPC"
	}
	return card.PreferredTransport
}

// mcpHeaders mcp 直通连接器的认证头（credentials.headers 凭据态，REQ-214 P2③；明文仅调用栈内存）。
func (s *Server) mcpHeaders(c *store.Connector) map[string]string {
	creds, err := s.resolveCredentials(c)
	if err != nil {
		log.Printf("[connectors] %s 认证头解密失败（按无头直通）: %v", c.Name, err)
		return nil
	}
	raw, ok := creds["headers"].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for k, v := range raw {
		if sv, ok := v.(string); ok && sv != "" {
			out[k] = sv
		}
	}
	return out
}

// testConnector POST /api/connectors/{id}/test：探测连接并回写 status + 工具清单 + tested_at。
func (s *Server) testConnector(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.GetConnector(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	ok, detail, tools, card := s.probeConnector(ctx, c)
	c.Status = map[bool]string{true: "ok", false: "error"}[ok]
	c.StatusDetail = detail
	c.TestedAt = nowRFC3339()
	if tools != nil {
		c.Tools = tools
	}
	if card != nil {
		b, _ := json.Marshal(card)
		c.CardJSON = string(b)
	}
	if _, err := s.Store.UpdateConnector(c); err != nil {
		log.Printf("[connectors] test 回写状态失败: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "detail": detail, "status": c.Status, "tools": c.Tools})
}

// previewConnector POST /api/connectors/preview：不落库按表单探测（REQ-214 P2④ 创建前先测通）。
// 凭据明文仅此一瞬（与 create 同暴露面），探测结果不回写任何存储。
func (s *Server) previewConnector(w http.ResponseWriter, r *http.Request) {
	var in connectorIn
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if he := validateConnectorIn(&in); he != nil {
		writeErr(w, he)
		return
	}
	enc, err := s.encryptCredentials(in.Credentials)
	if err != nil {
		writeErr(w, err)
		return
	}
	c := &store.Connector{
		Kind: in.Kind, Name: orDefault(in.Name, "preview"),
		Description: in.Description, Config: in.Config, CredentialsEncrypted: enc,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	ok, detail, tools, _ := s.probeConnector(ctx, c)
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "detail": detail, "tools": tools})
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// nowRFC3339 当前时间（API 层回写 tested_at 用；UTC RFC3339 与 store 时间戳形态对齐）。
func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// resolveCredentials 解密凭据 JSON（无凭据返回空 map）。明文仅在调用栈内存存在。
func (s *Server) resolveCredentials(c *store.Connector) (map[string]any, error) {
	out := map[string]any{}
	if len(c.CredentialsEncrypted) == 0 {
		return out, nil
	}
	plain, err := s.Box.Decrypt(c.CredentialsEncrypted)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(plain), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// testKubernetesConnector kubectl 探针（CLI + 认证 + 服务端可达）。
func (s *Server) testKubernetesConnector(ctx context.Context, c *store.Connector) (bool, string) {
	creds, err := s.resolveCredentials(c)
	if err != nil {
		return false, "凭据解密失败：" + err.Error()
	}
	kcRaw, _ := creds["kubeconfig"].(string)
	runner := &connector.KubectlRunner{
		Bin:           s.kubectlBin(),
		Context:       strOf(c.Config, "context"),
		Namespace:     strOf(c.Config, "namespace"),
		Kubeconfig:    strOf(c.Config, "kubeconfig_path"),
		KubeconfigRAW: kcRaw,
	}
	detail, err := runner.Probe(ctx)
	if err != nil {
		return false, "不可达：" + err.Error()
	}
	return true, detail
}

// testSSHConnector 拨号 + 认证 + echo 探针。
func (s *Server) testSSHConnector(ctx context.Context, c *store.Connector) (bool, string) {
	creds, err := s.resolveCredentials(c)
	if err != nil {
		return false, "凭据解密失败：" + err.Error()
	}
	pw, _ := creds["password"].(string)
	pk, _ := creds["private_key"].(string)
	pp, _ := creds["passphrase"].(string)
	port := strOf(c.Config, "port")
	runner := &connector.SSHRunner{
		Host: strOf(c.Config, "host"), User: strOf(c.Config, "user"), Port: port,
		Password: pw, PrivateKey: pk, Passphrase: pp,
	}
	detail, err := runner.Probe(ctx)
	if err != nil {
		return false, "不可达：" + err.Error()
	}
	return true, detail
}

// kubectlBin kubectl 路径：运行环境配置优先，env 兜底（REQ-191 同口径）。
func (s *Server) kubectlBin() string {
	if s.RuntimeEnv != nil && s.RuntimeEnv.Defaults.KubectlBin != "" {
		return s.RuntimeEnv.Defaults.KubectlBin
	}
	return os.Getenv("KUBECTL_BIN")
}

// defsToNames stdio 工具定义 → 名称清单（REQ-433 tools 列承载）。
func defsToNames(defs []tool.StdioToolDef) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}
