package facade

// REQ-435/M231 运行面能力工具化——facade 工具面扩展（治「运行面零覆盖」最大缺口，96 号 O-1 延伸）。
//
// 三组 12 件（沿 REQ-366 先例：facade 无条件注册可枚举，内部智能体启用门控在平台 backend
// 装配层 agent.onto_ops——tool.OntoOperationTools 同源；写类八件入 danger 档审批）：
//   方案管理组（7）：runtime_profile_list（只读）+ create/update/delete/start/stop/reload（写类六件
//     ——方案变更是运行面最重操作）；生命周期语义与 rest.go 同源（update/delete 先停 running、
//     start/reload 失败回快照 last_error）。
//   引擎运维组（3）：engines_status（只读自检）/ engine_install（写类——StartInstall 异步任务，
//     工具内轮询 EngineStatuses 至 Installing 结束，超时回状态续查沿 cq_benchmark_run 先例）/
//     profile_logs（只读）。
//   观测组（2）：profile_trace（只读——REQ-94 翻译透视）/ route_ambiguities（只读——REQ-384）。
//
// ④caller 审计归因搭车（REQ-366 诚实边界预留位）：写类工具可选 caller 参数（协调者自报身份，
//   沿 REQ-354 G4「随调用传身份」payload 形态），写操作落 trace_log（Tool=runtime_audit 行，
//   profile_trace 可读回）——归因面=trace_log 复用；版本快照 agent 列仍预留不反转。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ontoagents/ontoagents/runtime-manager/internal/engine/oxigraph"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/store"
)

// runtimeInstallPollTimeout engine_install 工具内轮询上限（超出回状态续查）。
const runtimeInstallPollTimeout = 120 * time.Second

// addRuntimeTools REQ-435：运行面三组 12 件工具注册（Manager 未接=不注册——构造兼容）。
func (f *Facade) addRuntimeTools(s *server.MCPServer) {
	if f.Manager == nil {
		return
	}
	oid := requiredString
	pid := func(name, desc string) mcp.ToolOption { return oid(name, desc) }

	s.AddTool(mcp.NewTool("runtime_profile_list",
		mcp.WithDescription("列出全部运行方案（id/名称/引擎/挂载本体/状态/端口）——运行面总览只读面。"),
	), f.handleProfileList())

	s.AddTool(mcp.NewTool("runtime_profile_create",
		mcp.WithDescription("创建运行方案（写操作，需审批）。engine 仅支持 oxigraph/fuseki；ontologies_json 为挂载本体 id 数组。"),
		oid("name", "方案名称"),
		mcp.WithString("engine", mcp.Description("引擎：oxigraph（默认）| fuseki")),
		mcp.WithString("ontologies_json", mcp.Description("挂载本体 id 数组 JSON，如 [\"ins-1\"]（可选，缺省空）")),
		mcp.WithNumber("port", mcp.Description("SPARQL 端口（可选，缺省自动分配）")),
		mcp.WithString("caller", mcp.Description("调用者身份（可选，如智能体名）——落运行面审计")),
	), f.handleProfileCreate())

	s.AddTool(mcp.NewTool("runtime_profile_update",
		mcp.WithDescription("更新运行方案元数据（写操作，需审批；running 方案会先停止——配置变更需重启生效）。只更新传入字段。"),
		pid("profile_id", "方案 id"),
		mcp.WithString("name", mcp.Description("新名称（可选）")),
		mcp.WithString("ontologies_json", mcp.Description("挂载本体 id 数组 JSON（可选，传入即整体覆盖）")),
		mcp.WithString("config", mcp.Description("引擎配置 JSON 串（可选，传入即覆盖）")),
		mcp.WithNumber("port", mcp.Description("SPARQL 端口（可选）")),
		mcp.WithString("caller", mcp.Description("调用者身份（可选）——落运行面审计")),
	), f.handleProfileUpdate())

	s.AddTool(mcp.NewTool("runtime_profile_delete",
		mcp.WithDescription("删除运行方案（写操作，需审批；running 方案先停止再删除）。"),
		pid("profile_id", "方案 id"),
		mcp.WithString("caller", mcp.Description("调用者身份（可选）——落运行面审计")),
	), f.handleProfileDelete())

	s.AddTool(mcp.NewTool("runtime_profile_start",
		mcp.WithDescription("启动运行方案（写操作，需审批）。失败回快照 last_error 定位。"),
		pid("profile_id", "方案 id"),
		mcp.WithString("caller", mcp.Description("调用者身份（可选）——落运行面审计")),
	), f.handleProfileStart())

	s.AddTool(mcp.NewTool("runtime_profile_stop",
		mcp.WithDescription("停止运行方案（写操作，需审批）。"),
		pid("profile_id", "方案 id"),
		mcp.WithString("caller", mcp.Description("调用者身份（可选）——落运行面审计")),
	), f.handleProfileStop())

	s.AddTool(mcp.NewTool("runtime_profile_reload",
		mcp.WithDescription("重载运行方案（写操作，需审批）——spec 版本超前于运行快照时把新版本灌入引擎（REQ-155 生命周期 drift 修复动作）。"),
		pid("profile_id", "方案 id"),
		mcp.WithString("caller", mcp.Description("调用者身份（可选）——落运行面审计")),
	), f.handleProfileReload())

	s.AddTool(mcp.NewTool("engines_status",
		mcp.WithDescription("引擎自检（只读）：各引擎可用性/版本/安装态 + docker 可用性。engine_install 后用本工具确认安装结果。"),
	), f.handleEnginesStatus())

	s.AddTool(mcp.NewTool("engine_install",
		mcp.WithDescription("一键安装引擎发行包（写操作，需审批；异步任务——工具内轮询至完成，超时回当前安装态可再次调用续查）。仅 fuseki 需要安装（oxigraph 内置）。"),
		oid("engine", "引擎名（fuseki）"),
		mcp.WithNumber("wait_seconds", mcp.Description("工具内等待上限秒（可选，缺省 120，硬上限 120）")),
		mcp.WithString("caller", mcp.Description("调用者身份（可选）——落运行面审计")),
	), f.handleEngineInstall())

	s.AddTool(mcp.NewTool("profile_logs",
		mcp.WithDescription("读取运行方案进程日志尾部（只读；含运行面审计行——写操作的 caller 归因随日志可查）。"),
		pid("profile_id", "方案 id"),
		mcp.WithNumber("tail", mcp.Description("尾部行数（可选，缺省 200）")),
	), f.handleProfileLogs())

	s.AddTool(mcp.NewTool("profile_trace",
		mcp.WithDescription("读取方案翻译透视记录（只读，时间倒序；含 SPARQL 查询透视与 runtime_audit 调用者归因行）。"),
		pid("profile_id", "方案 id"),
		mcp.WithNumber("limit", mcp.Description("条数上限（可选，缺省 50）")),
	), f.handleProfileTrace())

	s.AddTool(mcp.NewTool("route_ambiguities",
		mcp.WithDescription("多本体路由歧义观测（只读，REQ-384）：同名概念/实例跨本体歧义清单与建议（消除歧义后 sparql_query/onto_* 路由更稳）。"),
	), f.handleRouteAmbiguities())
}

// ---- handlers ----

func (f *Facade) profileSnapshots() []*store.Profile {
	ps, _ := f.Store.List()
	return ps
}

func (f *Facade) handleProfileList() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		out := make([]map[string]any, 0)
		for _, p := range f.profileSnapshots() {
			out = append(out, map[string]any{"id": p.ID, "name": p.Name, "engine": p.Engine,
				"ontologies": p.OntologyIDs, "status": p.Status, "port": p.Port})
		}
		return jsonResult(map[string]any{"profiles": out}), nil
	}
}

// auditCaller REQ-435④：写操作 caller 归因落 trace_log（runtime_audit 行；失败不影响主操作）。
func (f *Facade) auditCaller(action, profileID, caller string) {
	if strings.TrimSpace(caller) == "" {
		return
	}
	_ = f.Store.SaveTrace(&store.Trace{Tool: "runtime_audit", ProfileID: profileID,
		OntologyID: "-", Sparql: "caller=" + caller + " action=" + action, Ok: true})
}

func (f *Facade) handleProfileCreate() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		name, _ := args["name"].(string)
		if strings.TrimSpace(name) == "" {
			return mcp.NewToolResultErrorf("name 必填"), nil
		}
		engineName, _ := args["engine"].(string)
		if engineName == "" {
			engineName = "oxigraph"
		}
		if engineName != "oxigraph" && engineName != "fuseki" {
			return mcp.NewToolResultErrorf("仅支持 oxigraph / fuseki 引擎"), nil
		}
		oids := parseIDArray(args["ontologies_json"])
		port := 0
		if v, ok := args["port"].(float64); ok {
			port = int(v)
		}
		id := "rt_" + newID()
		p := &store.Profile{ID: id, Name: name, Engine: engineName, OntologyIDs: oids, Config: "{}", Port: port}
		if err := f.Store.Create(p); err != nil {
			return mcp.NewToolResultErrorf("创建失败: %v", err), nil
		}
		caller, _ := args["caller"].(string)
		f.auditCaller("runtime_profile_create", id, caller)
		out, _ := f.Store.Get(id)
		return jsonResult(map[string]any{"created": id, "profile": out}), nil
	}
}

func (f *Facade) handleProfileUpdate() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		id, _ := args["profile_id"].(string)
		p, err := f.Store.Get(id)
		if err != nil {
			return mcp.NewToolResultErrorf("方案不存在: %v", err), nil
		}
		name := p.Name
		if v, ok := args["name"].(string); ok && strings.TrimSpace(v) != "" {
			name = v
		}
		oids := p.OntologyIDs
		if raw, ok := args["ontologies_json"].(string); ok && strings.TrimSpace(raw) != "" {
			oids = parseIDArray(raw)
		}
		config := p.Config
		if v, ok := args["config"].(string); ok && strings.TrimSpace(v) != "" {
			config = v
		}
		port := p.Port
		if v, ok := args["port"].(float64); ok {
			port = int(v)
		}
		// 配置变更需重启生效：running 方案先停（与 rest.update 同语义）
		if p.Status == "running" || p.Status == "starting" {
			if err := f.Manager.Stop(id); err != nil {
				return mcp.NewToolResultErrorf("停止运行中方案失败: %v", err), nil
			}
		}
		if err := f.Store.UpdateMeta(id, name, config, oids, port); err != nil {
			return mcp.NewToolResultErrorf("更新失败: %v", err), nil
		}
		caller, _ := args["caller"].(string)
		f.auditCaller("runtime_profile_update", id, caller)
		out, _ := f.Store.Get(id)
		return jsonResult(map[string]any{"updated": id, "profile": out}), nil
	}
}

func (f *Facade) handleProfileDelete() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		id, _ := args["profile_id"].(string)
		p, err := f.Store.Get(id)
		if err != nil {
			return mcp.NewToolResultErrorf("方案不存在: %v", err), nil
		}
		if p.Status == "running" || p.Status == "starting" {
			_ = f.Manager.Stop(id)
		}
		if err := f.Store.Delete(id); err != nil {
			return mcp.NewToolResultErrorf("删除失败: %v", err), nil
		}
		caller, _ := args["caller"].(string)
		f.auditCaller("runtime_profile_delete", id, caller)
		return jsonResult(map[string]any{"deleted": id}), nil
	}
}

func (f *Facade) handleProfileStart() server.ToolHandlerFunc {
	return f.handleLifecycle("runtime_profile_start", func(ctx context.Context, id string) error {
		return f.Manager.Start(ctx, id)
	})
}

func (f *Facade) handleProfileStop() server.ToolHandlerFunc {
	return f.handleLifecycle("runtime_profile_stop", func(ctx context.Context, id string) error {
		return f.Manager.Stop(id)
	})
}

func (f *Facade) handleProfileReload() server.ToolHandlerFunc {
	return f.handleLifecycle("runtime_profile_reload", func(ctx context.Context, id string) error {
		return f.Manager.Reload(ctx, id)
	})
}

// handleLifecycle 启动/停止/重载三件共用骨架（失败回快照 last_error——与 rest 同语义）。
func (f *Facade) handleLifecycle(action string, op func(ctx context.Context, id string) error) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		id, _ := args["profile_id"].(string)
		if id == "" {
			return mcp.NewToolResultErrorf("profile_id 必填"), nil
		}
		if err := op(ctx, id); err != nil {
			p, _ := f.Store.Get(id)
			return mcp.NewToolResultErrorf("%s 失败: %v（快照：%s）", action, err, profileBrief(p)), nil
		}
		caller, _ := args["caller"].(string)
		f.auditCaller(action, id, caller)
		out, _ := f.Store.Get(id)
		return jsonResult(map[string]any{"action": action, "profile": out}), nil
	}
}

func profileBrief(p *store.Profile) string {
	if p == nil {
		return "（方案不存在）"
	}
	return fmt.Sprintf("id=%s name=%s status=%s last_error=%s", p.ID, p.Name, p.Status, p.LastError)
}

func (f *Facade) handleEnginesStatus() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(map[string]any{"engines": f.Manager.EngineStatuses(), "docker_available": oxigraph.DockerAvailable()}), nil
	}
}

func (f *Facade) handleEngineInstall() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		name, _ := args["engine"].(string)
		if err := f.Manager.StartInstall(name); err != nil {
			return mcp.NewToolResultErrorf("安装任务启动失败: %v", err), nil
		}
		wait := runtimeInstallPollTimeout
		if v, ok := args["wait_seconds"].(float64); ok && v > 0 && v < 120 {
			wait = time.Duration(v) * time.Second
		}
		caller, _ := args["caller"].(string)
		f.auditCaller("engine_install:"+name, "-", caller)
		deadline := time.Now().Add(wait)
		for {
			time.Sleep(2 * time.Second)
			for _, st := range f.Manager.EngineStatuses() {
				if st.Engine != name {
					continue
				}
				if !st.Installing {
					if st.LastInstallError != "" {
						return mcp.NewToolResultErrorf("ENGINE_INSTALL_FAILED: %s 安装失败：%s", name, st.LastInstallError), nil
					}
					return jsonResult(map[string]any{"engine": name, "installed": st.Installed, "status": st}), nil
				}
				if time.Now().After(deadline) {
					return jsonResult(map[string]any{"engine": name, "status": "installing",
						"hint": fmt.Sprintf("安装仍在进行（已等 %.0f 秒）——稍后用 engines_status 续查", wait.Seconds())}), nil
				}
			}
		}
	}
}

func (f *Facade) handleProfileLogs() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		id, _ := args["profile_id"].(string)
		tail := 200
		if v, ok := args["tail"].(float64); ok && v > 0 {
			tail = int(v)
		}
		lines, err := f.Manager.Logs(id, tail)
		if err != nil {
			return mcp.NewToolResultErrorf("日志读取失败: %v", err), nil
		}
		return jsonResult(map[string]any{"profile_id": id, "lines": lines}), nil
	}
}

func (f *Facade) handleProfileTrace() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		id, _ := args["profile_id"].(string)
		limit := 50
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		traces, err := f.Store.ListTraces(id, limit)
		if err != nil {
			return mcp.NewToolResultErrorf("透视读取失败: %v", err), nil
		}
		return jsonResult(map[string]any{"profile_id": id, "traces": traces}), nil
	}
}

func (f *Facade) handleRouteAmbiguities() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ambs, err := f.Store.RouteAmbiguities()
		if err != nil {
			return mcp.NewToolResultErrorf("歧义观测读取失败: %v", err), nil
		}
		return jsonResult(map[string]any{"ambiguities": ambs, "owner_cache_ttl": OwnerCacheTTL.String()}), nil
	}
}

// parseIDArray ontologies_json → []string（JSON 数组串或原生数组两形态；坏格式返回空）。
func parseIDArray(raw any) []string {
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		var out []string
		_ = json.Unmarshal([]byte(v), &out)
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// newID 与 rest.newID 同形态（进程内唯一即可）。
func newID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}
