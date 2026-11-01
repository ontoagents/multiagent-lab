// Package ontoapi REQ-561/M368 平台助手管理面 CLI（107 号 sibling=111 号方案档）——
// manifest 驱动的管理面命令表：域/子命令/method/路径/读写标记（ro 直行/rw danger 审批/deny
// 默认拒绝——沿 REQ-186 分级工具面覆盖面口径演进机制不反转：分级管控/两段式精神/删除类
// 不开放保留）。每端点 1~3 行；透传兜底=未知端点默认 deny。CLI 二进制（cmd/ontoapi）与
// platform_cli 工具（tool 包）共用本表单源。
package ontoapi

import "strings"

// Access 读写分级（REQ-186 口径三档）。
type Access int

const (
	Ro    Access = iota // 只读——直行
	Rw                  // 写操作——danger 审批（approval 挂起+审批卡）
	Deny                // 默认拒绝（DELETE 类沿「删除类不开放」+settings 密文导出类排除）
)

func (a Access) String() string {
	switch a {
	case Ro:
		return "ro"
	case Rw:
		return "rw"
	default:
		return "deny"
	}
}

// Endpoint 管理面端点命令行（每端点 1~3 行）。
type Endpoint struct {
	Name   string // 子命令名（agent_create 等——助手按需 --help 查询不占常驻上下文）
	Method string
	Path   string // 后端路径模板（:id 占位）
	Access Access
	Desc   string
}

// Manifest 管理面命令表（ro=读类直行；rw=创建/更新类审批；deny=删除类与密文导出类——
// REQ-186「删除类不开放」维持）。覆盖面=管理面 API（SSE 流式/文件上传二进制/密文导出/
// A2A·MCP 协议面不进 manifest——111 号诚实边界）。
var Manifest = []Endpoint{
	// ---- 智能体 ----
	{Name: "agent_list", Method: "GET", Path: "/api/agents", Access: Ro, Desc: "智能体清单（id/名称/描述/内置标记）"},
	{Name: "agent_get", Method: "GET", Path: "/api/agents/{id}", Access: Ro, Desc: "智能体详情"},
	{Name: "agent_create", Method: "POST", Path: "/api/agents", Access: Rw, Desc: "创建智能体（body=JSON）"},
	{Name: "agent_update", Method: "PUT", Path: "/api/agents/{id}", Access: Rw, Desc: "更新智能体（--from 自动 GET 取 If-Match 基线）"},
	// ---- 会话 ----
	{Name: "conversation_list", Method: "GET", Path: "/api/conversations", Access: Ro, Desc: "会话清单（agent_id/project_id/workspace_id 过滤）"},
	{Name: "conversation_get", Method: "GET", Path: "/api/conversations/{id}", Access: Ro, Desc: "会话详情（phase 状态机）"},
	{Name: "conversation_create", Method: "POST", Path: "/api/conversations", Access: Rw, Desc: "创建会话（scope=agent|project）"},
	{Name: "conversation_delete", Method: "DELETE", Path: "/api/conversations/{id}", Access: Deny, Desc: "删除会话（删除类不开放——REQ-186 口径；页面可做）"},
	// ---- 项目 ----
	{Name: "project_list", Method: "GET", Path: "/api/projects", Access: Ro, Desc: "项目清单"},
	{Name: "project_get", Method: "GET", Path: "/api/projects/{id}", Access: Ro, Desc: "项目详情"},
	{Name: "project_create", Method: "POST", Path: "/api/projects", Access: Rw, Desc: "创建项目"},
	{Name: "project_delete", Method: "DELETE", Path: "/api/projects/{id}", Access: Deny, Desc: "删除项目（删除类不开放）"},
	// ---- 知识库 ----
	{Name: "kb_list", Method: "GET", Path: "/api/kb", Access: Ro, Desc: "知识库清单"},
	{Name: "kb_docs", Method: "GET", Path: "/api/kb/{id}/docs", Access: Ro, Desc: "知识库文档清单"},
	{Name: "kb_create", Method: "POST", Path: "/api/kb", Access: Rw, Desc: "创建知识库"},
	{Name: "kb_delete", Method: "DELETE", Path: "/api/kb/{id}", Access: Deny, Desc: "删除知识库（删除类不开放）"},
	// ---- 本体 ----
	{Name: "ontology_list", Method: "GET", Path: "/api/ontologies", Access: Ro, Desc: "本体清单（反代构建平面）"},
	{Name: "ontology_get", Method: "GET", Path: "/api/ontologies/{id}", Access: Ro, Desc: "本体详情"},
	{Name: "ontology_create", Method: "POST", Path: "/api/ontologies", Access: Rw, Desc: "创建本体"},
	// ---- 模型连接 ----
	{Name: "model_list", Method: "GET", Path: "/api/connections", Access: Ro, Desc: "模型连接清单（has_credentials 脱敏）"},
	{Name: "model_create", Method: "POST", Path: "/api/connections", Access: Rw, Desc: "创建模型连接（Key 服务端加密落库）"},
	// ---- 连接器 ----
	{Name: "connector_list", Method: "GET", Path: "/api/connectors", Access: Ro, Desc: "连接器清单"},
	{Name: "connector_test", Method: "POST", Path: "/api/connectors/{id}/test", Access: Rw, Desc: "连接器探测测试"},
	// ---- 统计/指标 ----
	{Name: "stats_usage", Method: "GET", Path: "/api/stats/usage", Access: Ro, Desc: "使用统计（group_by=model|agent|project|conversation|member）"},
	{Name: "run_metrics", Method: "GET", Path: "/api/stats/run/{run_id}/metrics", Access: Ro, Desc: "单 run 过程账本（参与度/介入/阶段——REQ-557）"},
	// ---- 设置导出（密文类排除——111 号诚实边界；settings/export 明文不出密钥可放行沿 REQ-556 甄别口径，CLI v1 不进面） ----
	{Name: "settings_export", Method: "GET", Path: "/api/settings/export", Access: Deny, Desc: "设置导出（CLI 不承载——配置面走页面）"},
}

// Lookup 按子命令名查端点。
func Lookup(name string) (Endpoint, bool) {
	name = strings.TrimSpace(name)
	for _, e := range Manifest {
		if e.Name == name {
			return e, true
		}
	}
	return Endpoint{}, false
}

// Grade 命令分级判读（platform_cli 工具 args 判读口径——ro 直行/rw 审批/deny 拒绝）。
// 返回 (端点, 级别, 是否已知)；未知命令按 deny（透传兜底=未知端点默认拒绝）。
func Grade(command string) (Endpoint, Access, bool) {
	name := command
	if i := strings.IndexAny(name, " \t"); i >= 0 {
		name = name[:i]
	}
	e, ok := Lookup(name)
	if !ok {
		return Endpoint{}, Deny, false
	}
	return e, e.Access, true
}

// DenyReason deny 人话理由（错误归一口径——111 号验收「deny 不可达」）。
func DenyReason(name string) string {
	if e, ok := Lookup(name); ok {
		return "命令 " + e.Name + " 为受控操作（" + e.Method + " " + e.Path + "）——CLI 不开放（删除类/密文类沿 REQ-186「删除类不开放」口径）；如需执行请走对应页面或使用全权面另行评估"
	}
	return "未知命令 " + name + " ——CLI 未知端点默认拒绝（透传兜底 deny）；可用子命令见 ontoapi --help"
}
