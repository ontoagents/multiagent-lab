package chat

// REQ-426/M222 单测：外部 CLI 成员委派工具——协作面成功路径（stub Run→文本回传）、
// REQ-319 同语义降级回喂（执行失败→member_failed JSON+run.warning，不炸 run）、
// channel=cli 委派单事件、描述合成（backend/版本/角色前缀）、cliProbeVersion 解析口径。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/inference"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func newCLITool(t *testing.T, run func(ctx context.Context, task string) (string, int, error)) einotool.InvokableTool {
	t.Helper()
	bt, err := newCLIMemberTool(cliMemberDeps{
		Name: "cli编码者", Backend: "claude-code", Version: "1.0.99",
		TaskPrefix: "你是编码成员", Run: run,
	})
	if err != nil {
		t.Fatal(err)
	}
	it, ok := bt.(einotool.InvokableTool)
	if !ok {
		t.Fatal("CLI 成员工具应为 InvokableTool")
	}
	return it
}

func TestCLIMemberToolSuccess(t *testing.T) {
	ctx, evts := withSink(context.Background())
	dt := newCLITool(t, func(ctx context.Context, task string) (string, int, error) {
		if !strings.Contains(task, "实现函数 foo") {
			t.Fatalf("任务应原样传给执行层: %s", task)
		}
		return "已完成：foo 已实现并通过测试", 2, nil
	})
	out, err := dt.InvokableRun(ctx, `{"request":"实现函数 foo"}`)
	if err != nil {
		t.Fatalf("外部成员成功应直回结果: %v", err)
	}
	if !strings.Contains(out, "foo 已实现") {
		t.Fatalf("结果文本应回传: %s", out)
	}
	d := findEvent(evts, "subagent.delegate")
	if d == nil {
		t.Fatal("缺委派单事件")
	}
	if d.data["channel"] != "cli" || d.data["member"] != "cli编码者" || d.data["backend"] != "claude-code" || d.data["task"] != "实现函数 foo" {
		t.Fatalf("channel=cli 委派单不符: %v", d.data)
	}
	if findEvent(evts, "run.warning") != nil {
		t.Fatal("成功路径不应有 run.warning")
	}
}

func TestCLIMemberToolDegrade(t *testing.T) {
	ctx, evts := withSink(context.Background())
	dt := newCLITool(t, func(ctx context.Context, task string) (string, int, error) {
		return "", 0, errors.New("退出码 1: claude 未登录")
	})
	out, err := dt.InvokableRun(ctx, `{"request":"任意任务"}`)
	if err != nil {
		t.Fatalf("失败应降级回喂而非报错: %v", err)
	}
	if !strings.Contains(out, `"member_failed"`) || !strings.Contains(out, "cli编码者") || !strings.Contains(out, `"cli"`) || !strings.Contains(out, "claude 未登录") {
		t.Fatalf("降级 JSON 缺关键字段: %s", out)
	}
	w := findEvent(evts, "run.warning")
	if w == nil {
		t.Fatal("缺 run.warning")
	}
	mf, _ := w.data["member_failed"].(map[string]any)
	if mf == nil || mf["channel"] != "cli" || mf["backend"] != "claude-code" {
		t.Fatalf("run.warning 缺 cli 标注: %v", w.data)
	}
}

func TestCLIMemberEmptyOutput(t *testing.T) {
	ctx, evts := withSink(context.Background())
	dt := newCLITool(t, func(ctx context.Context, task string) (string, int, error) { return "  ", 0, nil })
	out, err := dt.InvokableRun(ctx, `{"request":"任务"}`)
	if err != nil {
		t.Fatalf("空输出不应报错: %v", err)
	}
	if !strings.Contains(out, "未产生文本输出") {
		t.Fatalf("空输出应如实回喂: %s", out)
	}
	if findEvent(evts, "run.warning") != nil {
		t.Fatal("空输出是成功路径，不应有 run.warning")
	}
}

func TestCLIMemberDescriptionSynthesis(t *testing.T) {
	desc := cliMemberDescription(cliMemberDeps{
		Name: "cli-1", Backend: "claude-code", Version: "1.0.99", TaskPrefix: "只写测试不改实现",
	})
	if !strings.Contains(desc, "claude-code") || !strings.Contains(desc, "1.0.99") || !strings.Contains(desc, "只写测试不改实现") || !strings.Contains(desc, "CLI") {
		t.Fatalf("描述应合成 backend/版本/角色前缀: %s", desc)
	}
	// 无版本/前缀降级：仍可用（backend+名称）
	desc2 := cliMemberDescription(cliMemberDeps{Name: "cli-2", Backend: "opencode"})
	if !strings.Contains(desc2, "cli-2") || !strings.Contains(desc2, "opencode") {
		t.Fatalf("无版本描述应含名称与 backend: %s", desc2)
	}
}

func TestCLIProbeVersionParse(t *testing.T) {
	if v := cliProbeVersion(`{"type":"cli-probe","backend":"claude-code","version":"1.2.3","path":"/usr/bin/claude"}`); v != "1.2.3" {
		t.Fatalf("cli-probe 载荷应取 version: %q", v)
	}
	if v := cliProbeVersion(`{"name":"外部审查员","skills":[]}`); v != "" {
		t.Fatalf("a2a 卡片形态应返回空: %q", v)
	}
	if v := cliProbeVersion(""); v != "" {
		t.Fatalf("空 card 应返回空: %q", v)
	}
}

// ---- 装配链（REQ-426/M222）：assembleCLIMembers 直测——成功装配/二进制缺失告警跳过/
// 未知 backend/重名 -cli 后缀/注册表未接降级 ----

func newCLIFixture(t *testing.T) (*Assembler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	return &Assembler{Store: st, Box: box, Inference: inference.NewRegistry()}, st
}

// stubClaudeBin 临时 stub claude（--version + stream-json 输出）；PATH 前插。
func stubClaudeBin(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'claude-stub 9.9.9'; exit 0; fi\n" +
		"echo '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"stub\"}]}}'\n" +
		"echo '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"tool_use\",\"name\":\"read_file\",\"input\":{\"path\":\"a.go\"}}]}}'\n" +
		"echo '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"stub \u7ed3\u679c REQ427_TEST_ENV='\"$REQ427_TEST_ENV\"'\"}'" + "\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestAssembleCLIMembersEndToEnd(t *testing.T) {
	stubClaudeBin(t)
	asm, st := newCLIFixture(t)
	c, err := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindCLI, Name: "cli编码者",
		Config:   map[string]any{"backend": "claude-code", "task_prefix": "只写代码"},
		CardJSON: `{"type":"cli-probe","backend":"claude-code","version":"9.9.9","path":"/tmp/stub/claude"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ag := &store.Agent{Name: "协调者", Connectors: []string{c.ID}}
	src := map[string]string{}
	tools, entries, warns := asm.assembleCLIMembers(context.Background(), ag, src, "")
	if len(warns) != 0 {
		t.Fatalf("成功路径不应有告警: %v", warns)
	}
	if len(tools) != 1 || len(entries) != 1 {
		t.Fatalf("应装配 1 件成员工具: %d/%d", len(tools), len(entries))
	}
	ti, _ := tools[0].Info(context.Background())
	if ti.Name != "cli编码者" {
		t.Fatalf("工具名=连接器实例名: %s", ti.Name)
	}
	if !strings.Contains(ti.Desc, "claude-code") || !strings.Contains(ti.Desc, "9.9.9") || !strings.Contains(ti.Desc, "只写代码") {
		t.Fatalf("描述应合成 backend/版本/前缀: %s", ti.Desc)
	}
	if src["cli编码者"] != "cli:"+c.ID {
		t.Fatalf("来源标注 cli:{id}: %s", src["cli编码者"])
	}
	if !strings.Contains(fmt.Sprint(entries[0]), "CLI 外部智能体（claude-code）") {
		t.Fatalf("快照条目应标注 CLI 外部智能体: %v", entries[0])
	}
}

// failProberBackend LookPath 恒失败的外部 CLI 桩（不依赖真机安装态，测试确定性）。
type failProberBackend struct{}

func (failProberBackend) Name() string { return "fail-probe" }
func (failProberBackend) Probe(_ context.Context) inference.ProbeResult {
	return inference.ProbeResult{Available: false, Reason: `PATH 中未找到 "fail-probe"`}
}
func (failProberBackend) Capabilities() inference.Capabilities {
	return inference.Capabilities{Chat: true}
}
func (failProberBackend) Run(_ context.Context, _ *inference.RunRequest, _ func(inference.Event)) error {
	return nil
}
func (failProberBackend) LookPath() (string, error) {
	return "", errors.New(`PATH 中未找到 "fail-probe"`)
}

func TestAssembleCLIMembersDegrades(t *testing.T) {
	asm, st := newCLIFixture(t)
	asm.Inference.Register(failProberBackend{}) // 覆盖为 LookPath 恒失败桩（真机可能装有同名 CLI）
	// 二进制不可达 → 告警跳过
	c1, _ := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindCLI, Name: "死成员", Config: map[string]any{"backend": "fail-probe"},
	})
	// 未知 backend → 告警跳过
	c2, _ := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindCLI, Name: "坏配置", Config: map[string]any{"backend": "no-such"},
	})
	ag := &store.Agent{Name: "a", Connectors: []string{c1.ID, c2.ID}}
	tools, _, warns := asm.assembleCLIMembers(context.Background(), ag, map[string]string{}, "")
	if len(tools) != 0 {
		t.Fatal("两件均不应装配")
	}
	joined := strings.Join(warns, "\n")
	if !strings.Contains(joined, "二进制不可达") || !strings.Contains(joined, "不是已注册外部后端") {
		t.Fatalf("告警应如实区分两种降级: %s", joined)
	}
}

func TestAssembleCLIMembersRenameAndNilRegistry(t *testing.T) {
	stubClaudeBin(t)
	asm, st := newCLIFixture(t)
	c, _ := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindCLI, Name: "撞名成员", Config: map[string]any{"backend": "claude-code"},
	})
	// 重名：内部成员已占用同名工具名 → -cli 后缀 + 告警
	src := map[string]string{"撞名成员": "agent:x"}
	tools, _, warns := asm.assembleCLIMembers(context.Background(), &store.Agent{Name: "a", Connectors: []string{c.ID}}, src, "")
	if len(tools) != 1 {
		t.Fatal("重名应改名装配而非跳过")
	}
	ti, _ := tools[0].Info(context.Background())
	if ti.Name != "撞名成员-cli" {
		t.Fatalf("应加 -cli 后缀: %s", ti.Name)
	}
	if !strings.Contains(strings.Join(warns, "\n"), "-cli") {
		t.Fatalf("应告警改名: %v", warns)
	}
	// 注册表未接（旧构造路径）→ 如实告警不炸
	asmNil := &Assembler{Store: st}
	tools2, _, warns2 := asmNil.assembleCLIMembers(context.Background(), &store.Agent{Name: "a", Connectors: []string{c.ID}}, map[string]string{}, "")
	if len(tools2) != 0 || !strings.Contains(strings.Join(warns2, "\n"), "未接推理后端注册表") {
		t.Fatalf("nil 注册表应告警: %v", warns2)
	}
}

// ---- REQ-427/M223：外部成员治理拉齐——审批包装/沙箱三前置/凭据 env 注入 ----

// namedStubTool 命名桩工具（approveExternalMembers 输入构造用；delegatetool_test.go 已有 stubTool）。
type namedStubTool struct{ name string }

func (s namedStubTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: s.name}, nil
}
func (s namedStubTool) InvokableRun(context.Context, string, ...einotool.Option) (string, error) {
	return "", nil
}

func TestApproveExternalMembersWrap(t *testing.T) {
	asm, _ := newCLIFixture(t)
	ctx := context.Background()
	// src 表：cli:/a2a: 前缀=外部成员；agent:=内部成员（维持豁免）
	src := map[string]string{"cli成员": "cli:c1", "a2a成员": "a2a:c2", "内部成员": "agent:a1"}
	mk := func() []einotool.BaseTool {
		return []einotool.BaseTool{namedStubTool{"cli成员"}, namedStubTool{"a2a成员"}, namedStubTool{"内部成员"}}
	}
	// 关档：不包装
	tools := mk()
	if warns := asm.approveExternalMembers(ctx, &store.Agent{}, "", src, tools); len(warns) != 0 {
		t.Fatalf("审批关档不应有告警: %v", warns)
	}
	if _, ok := tools[0].(namedStubTool); !ok {
		t.Fatal("关档不应包装")
	}
	// danger 档：外部成员均包装，内部成员不包装
	tools = mk()
	warns := asm.approveExternalMembers(ctx, &store.Agent{ToolApproval: "danger"}, "danger", src, tools)
	if len(warns) != 1 || !strings.Contains(warns[0], "2 件") {
		t.Fatalf("danger 档应告警 2 件外部成员: %v", warns)
	}
	if _, ok := tools[0].(namedStubTool); ok {
		t.Fatal("cli 成员应被审批包装")
	}
	if _, ok := tools[1].(namedStubTool); ok {
		t.Fatal("a2a 成员应被审批包装")
	}
	if _, ok := tools[2].(namedStubTool); !ok {
		t.Fatal("内部成员应维持豁免")
	}
	// 豁免清单透传（豁免在运行期放行——包装恒发生，名单内工具调用直接执行）
	tools = mk()
	warns = asm.approveExternalMembers(ctx, &store.Agent{ToolApproval: "all", ApprovalExempt: []string{"cli成员"}}, "all", src, tools)
	if len(warns) != 1 || !strings.Contains(warns[0], "豁免 1 项") {
		t.Fatalf("豁免清单应随策略告警透出: %v", warns)
	}
}

func TestAssembleCLIMembersSandboxGates(t *testing.T) {
	stubClaudeBin(t)
	asm, st := newCLIFixture(t)
	mk := func(name string, cfg map[string]any) *store.Connector {
		c, err := st.CreateConnector(&store.Connector{Kind: store.ConnectorKindCLI, Name: name, Config: cfg,
			CardJSON: `{"type":"cli-probe","backend":"claude-code","version":"9.9.9"}`})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	// docker 可达但缺绑定目录/审批/镜像 → 分别告警跳过
	c1 := mk("沙箱1", map[string]any{"backend": "claude-code", "sandbox_mode": "docker"})
	c2 := mk("沙箱2", map[string]any{"backend": "claude-code", "sandbox_mode": "docker", "cwd": "/tmp/ws"})
	c3 := mk("沙箱3", map[string]any{"backend": "claude-code", "sandbox_mode": "docker", "cwd": "/tmp/ws", "sandbox_image": "img:dev"})
	asm.DockerProbe = func(context.Context) bool { return true }
	ag := &store.Agent{Name: "a", Connectors: []string{c1.ID}}
	tools, _, warns := asm.assembleCLIMembers(context.Background(), ag, map[string]string{}, "")
	if len(tools) != 0 || !strings.Contains(strings.Join(warns, "\n"), "绑定目录缺失") {
		t.Fatalf("缺 cwd 应告警: %v", warns)
	}
	ag.Connectors = []string{c2.ID}
	tools, _, warns = asm.assembleCLIMembers(context.Background(), ag, map[string]string{}, "")
	if len(tools) != 0 || !strings.Contains(strings.Join(warns, "\n"), "审批档未启用") {
		t.Fatalf("缺审批应告警: %v", warns)
	}
	ag.Connectors = []string{c3.ID}
	// 全前置满足（danger 档即可）→ 装配 + 描述含沙箱世界
	tools, _, warns = asm.assembleCLIMembers(context.Background(), ag, map[string]string{}, "danger")
	if len(warns) != 0 || len(tools) != 1 {
		t.Fatalf("全前置应装配: %v", warns)
	}
	ti, _ := tools[0].Info(context.Background())
	if !strings.Contains(ti.Desc, "docker 沙箱世界（镜像 img:dev）") {
		t.Fatalf("描述应含沙箱世界: %s", ti.Desc)
	}
	// docker 不可达 → 未装配
	asm.DockerProbe = func(context.Context) bool { return false }
	tools, _, warns = asm.assembleCLIMembers(context.Background(), ag, map[string]string{}, "all")
	if len(tools) != 0 || !strings.Contains(strings.Join(warns, "\n"), "docker 不可达") {
		t.Fatalf("docker 不可达应告警: %v", warns)
	}
}

func TestAssembleCLIMembersCredsEnvInject(t *testing.T) {
	stubClaudeBin(t)
	asm, st := newCLIFixture(t)
	enc, _ := asm.Box.Encrypt(`{"env":{"REQ427_TEST_ENV":"sk-secret"}}`)
	c, err := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindCLI, Name: "env成员",
		Config: map[string]any{"backend": "claude-code"}, CredentialsEncrypted: enc,
	})
	if err != nil {
		t.Fatal(err)
	}
	ag := &store.Agent{Name: "a", Connectors: []string{c.ID}}
	tools, _, warns := asm.assembleCLIMembers(context.Background(), ag, map[string]string{}, "")
	if len(warns) != 0 || len(tools) != 1 {
		t.Fatalf("装配失败: %v", warns)
	}
	ti, _ := tools[0].Info(context.Background())
	if strings.Contains(ti.Desc, "sk-secret") {
		t.Fatal("凭据不得进工具描述（LLM 上下文）")
	}
	it, ok := tools[0].(einotool.InvokableTool)
	if !ok {
		t.Fatal("应为 InvokableTool")
	}
	// stub claude 把 REQ427_TEST_ENV 值打进 result → 断言 env 注入子进程
	ctx := context.Background()
	out, err := it.InvokableRun(ctx, `{"request":"dump env"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sk-secret") {
		t.Fatalf("credentials.env 应注入子进程环境: %s", out)
	}
}

// REQ-428/M224：CLI 成员过程转译端到端——装配链真实调用成员工具，stream-json 的
// text/tool_use/result 逐条转 external.audit run_event（data 形态断言）。
func TestAssembleCLIMembersProcessAudit(t *testing.T) {
	stubClaudeBin(t)
	asm, st := newCLIFixture(t)
	c, err := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindCLI, Name: "过程成员",
		Config: map[string]any{"backend": "claude-code"},
		CardJSON: `{"type":"cli-probe","backend":"claude-code","version":"9.9.9"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	ag := &store.Agent{Name: "a", Connectors: []string{c.ID}}
	tools, _, warns := asm.assembleCLIMembers(context.Background(), ag, map[string]string{}, "")
	if len(warns) != 0 || len(tools) != 1 {
		t.Fatalf("装配失败: %v", warns)
	}
	it, ok := tools[0].(einotool.InvokableTool)
	if !ok {
		t.Fatal("应为 InvokableTool")
	}
	ctx, evts := withSink(context.Background())
	if _, err := it.InvokableRun(ctx, `{"request":"过程任务"}`); err != nil {
		t.Fatal(err)
	}
	var audits []capturedEvent
	for _, e := range *evts {
		if e.typ == "external.audit" {
			audits = append(audits, e)
		}
	}
	if len(audits) != 3 {
		t.Fatalf("应转译 3 条过程事件（text/tool_use/result）: %d", len(audits))
	}
	if audits[0].data["proto_event"] != "text" || audits[1].data["proto_event"] != "tool_use" || audits[2].data["proto_event"] != "result" {
		t.Fatalf("proto_event 序列不符: %v/%v/%v", audits[0].data["proto_event"], audits[1].data["proto_event"], audits[2].data["proto_event"])
	}
	if s, _ := audits[1].data["summary"].(string); !strings.Contains(s, "read_file") {
		t.Fatalf("tool_use 摘要应含工具名: %s", s)
	}
	if audits[0].data["source"] != "cli_member" || audits[0].data["channel"] != "cli" || audits[0].data["member"] != "过程成员" {
		t.Fatalf("external.audit data 形态不符: %v", audits[0].data)
	}
}

// ---- REQ-429/M225：成员能力画像合成——描述注入/画像卡/成本档口径 ----

func TestExternalMemberProfile(t *testing.T) {
	asm, st := newCLIFixture(t)
	// cli：探测载荷 + 注册表能力矩阵
	c, _ := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindCLI, Name: "cli画像", Config: map[string]any{"backend": "claude-code"},
		CardJSON: `{"type":"cli-probe","backend":"claude-code","version":"9.9.9"}`,
	})
	p := ExternalMemberProfile(c, asm.Inference)
	if p["channel"] != "cli" || p["version"] != "9.9.9" || p["matrix"] == nil {
		t.Fatalf("cli 画像形态: %v", p)
	}
	if !strings.Contains(p["cost"].(string), "对端 CLI 自管") {
		t.Fatalf("成本档应如实标注: %v", p["cost"])
	}
	// a2a：card（capabilities.streaming+properties）
	c2, _ := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindA2A, Name: "a2a画像",
		CardJSON: `{"name":"对端","protocolVersion":"0.3.0","preferredTransport":"JSONRPC","capabilities":{"streaming":true},"properties":{"billing":"per-task"},"skills":[{"id":"s","name":"审查"}]}`,
	})
	p2 := ExternalMemberProfile(c2, nil)
	if p2["streaming"] != true || p2["transport"] != "JSONRPC" || p2["protocol"] != "0.3.0" {
		t.Fatalf("a2a 画像形态: %v", p2)
	}
	if !strings.Contains(p2["inloop"].(string), "对端支持流式") {
		t.Fatalf("在环性应如实标注: %v", p2["inloop"])
	}
	if p2["properties"].(map[string]any)["billing"] != "per-task" {
		t.Fatalf("对端 properties 应透出: %v", p2["properties"])
	}
	// 非 External kind → nil
	c3, _ := st.CreateConnector(&store.Connector{Kind: store.ConnectorKindMCP, Name: "m", Config: map[string]any{"url": "http://x"}})
	if ExternalMemberProfile(c3, nil) != nil {
		t.Fatal("mcp 连接器无成员画像")
	}
}

func TestProfileMatrixSummaryAndDescription(t *testing.T) {
	s := profileMatrixSummary(inference.Capabilities{Chat: true, Stream: true, SkillsMode: "instruction", MCPMode: "instruction"})
	if !strings.Contains(s, "对话✓") || !strings.Contains(s, "流式✓") || !strings.Contains(s, "技能:instruction") {
		t.Fatalf("矩阵摘要: %s", s)
	}
	// CLI 描述注入画像（路由信号）
	desc := cliMemberDescription(cliMemberDeps{Name: "m", Backend: "claude-code", Matrix: s})
	if !strings.Contains(desc, "能力画像：") || !strings.Contains(desc, "MCP:instruction") {
		t.Fatalf("描述应含能力画像: %s", desc)
	}
}
