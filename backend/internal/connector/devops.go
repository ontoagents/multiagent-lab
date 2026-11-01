package connector

// REQ-554/M361 研发数据源接入扩展（结构化协作批 P0；107 号 §五）——kind=devops 托管插件工具组：
// 五类数据语义分类 × 三交付形态中的「平台托管插件」形态；读类先行（107 §5.5 git 面只读边界，
// 写类审批面另行立项）。凭据（gh token）服务端解密后经子进程 env 注入，不进 LLM 上下文不进工具参数
// （plugin.go 凭据口径）。config.datasource ∈ {git, issues, ci, testreport} 细分工具组；
// git 每次调用一条只读子命令（参数白名单构造，不经模型透传）；test_report_parse 纯 Go 解析零子进程。

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// DevopsDatasources 数据源细分（config.datasource 白名单）。
var DevopsDatasources = []string{"git", "issues", "ci", "testreport"}

// DevopsToolNames 各数据源工具清单（probe 探测回写 tools_json——授权前知情）。
func DevopsToolNames(datasource string) []string {
	switch datasource {
	case "git":
		return []string{"git_status", "git_log", "git_diff", "git_show", "git_blame"}
	case "issues":
		return []string{"gh_issue_list", "gh_issue_view"}
	case "ci":
		return []string{"gh_run_list", "gh_run_view"}
	case "testreport":
		return []string{"test_report_parse"}
	default:
		return nil
	}
}

// addDevopsTools devops 连接器工具集（按 config.datasource 注册对应组；只读语义全组不变）。
func (p *PluginService) addDevopsTools(srv *mcpserver.MCPServer, c *store.Connector) error {
	ds := cfgStr(c, "datasource")
	creds, err := p.resolveCredentials(c)
	if err != nil {
		return err
	}
	desc := "经研发数据源连接器「" + c.Name + "」只读访问（凭据服务端绑定，无须提供）"
	switch ds {
	case "git":
		return addGitTools(srv, c, desc)
	case "issues":
		return addGhTools(srv, c, creds, desc, "issues")
	case "ci":
		return addGhTools(srv, c, creds, desc, "ci")
	case "testreport":
		return addTestReportTool(srv, desc)
	default:
		return fmt.Errorf("config.datasource %q 非法（须 git|issues|ci|testreport）", ds)
	}
}

// gitRunner git 只读子命令运行器（cwd=repo_dir；每次调用一条构造好的命令——模型只传参数值，
// 不传命令面，注入面收敛）。
type gitRunner struct {
	Bin string
	Dir string
}

func (g *gitRunner) run(ctx context.Context, args ...string) (string, error) {
	if g.Dir == "" {
		return "", fmt.Errorf("config.repo_dir 必填（git 仓库工作目录）")
	}
	cmd := exec.CommandContext(ctx, g.Bin, args...)
	cmd.Dir = g.Dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func addGitTools(srv *mcpserver.MCPServer, c *store.Connector, desc string) error {
	runner := &gitRunner{Bin: cfgStr(c, "git_bin"), Dir: cfgStr(c, "repo_dir")}
	if runner.Bin == "" {
		runner.Bin = "git"
	}
	type gitTool struct {
		name, brief string
		props       []mcp.ToolOption
		build       func(a map[string]any) []string
		limit       int
	}
	tools := []gitTool{
		{"git_status", desc + "：工作区状态（git status --porcelain）", nil,
			func(map[string]any) []string { return []string{"status", "--porcelain"} }, 0},
		{"git_log", desc + "：提交历史（git log，默认 ≤20 条单行）",
			[]mcp.ToolOption{mcp.WithNumber("limit", mcp.Description("条数（1~50，默认 20）"))},
			func(a map[string]any) []string {
				n := clampInt(num(a, "limit"), 1, 50, 20)
				return []string{"log", "--oneline", "-n", fmt.Sprint(n)}
			}, 0},
		{"git_diff", desc + "：变更明细（git diff；可选 commit 参数=git show）",
			[]mcp.ToolOption{mcp.WithString("commit", mcp.Description("提交 SHA（省略=工作区 diff）"))},
			func(a map[string]any) []string {
				if sha := str(a, "commit"); sha != "" && isSafeSha(sha) {
					return []string{"show", "--stat", "--patch", sha}
				}
				return []string{"diff", "--stat"}
			}, 0},
		{"git_show", desc + "：单提交全文（git show <sha>）",
			[]mcp.ToolOption{mcp.WithString("sha", mcp.Required(), mcp.Description("提交 SHA"))},
			func(a map[string]any) []string {
				sha := str(a, "sha")
				if !isSafeSha(sha) {
					return []string{"show", "--stat", "HEAD"}
				}
				return []string{"show", "--stat", "--patch", sha}
			}, 0},
		{"git_blame", desc + "：行级追溯（git blame -L <start>,<end> <file>）",
			[]mcp.ToolOption{
				mcp.WithString("file", mcp.Required(), mcp.Description("文件相对路径（工作区内）")),
				mcp.WithNumber("start", mcp.Description("起始行（默认 1）")),
				mcp.WithNumber("end", mcp.Description("结束行（默认 start+40）")),
			},
			func(a map[string]any) []string {
				file := str(a, "file")
				if file == "" || strings.Contains(file, "..") || strings.HasPrefix(file, "/") || strings.HasPrefix(file, "\\") {
					return []string{"log", "--oneline", "-n", "1"}
				}
				start := int(num(a, "start"))
				if start < 1 {
					start = 1
				}
				end := int(num(a, "end"))
				if end < start {
					end = start + 40
				}
				return []string{"blame", "-L", fmt.Sprintf("%d,%d", start, end), "--", file}
			}, 0},
	}
	for _, tl := range tools {
		opts := append([]mcp.ToolOption{mcp.WithDescription(tl.brief)}, tl.props...)
		t := mcp.NewTool(tl.name, opts...)
		srv.AddTool(t, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			out, err := runner.run(ctx, tl.build(req.GetArguments())...)
			return mcp.NewToolResultText(toolText(out, err)), nil
		})
	}
	return nil
}

// isSafeSha 提交 SHA 白名单（hex 4~40 位——防任意参数注入 git 命令面）。
func isSafeSha(s string) bool {
	if len(s) < 4 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// ghRunner gh CLI 只读运行器（token 经 env 注入子进程，不进工具参数——plugin 凭据口径）。
type ghRunner struct {
	Bin   string
	Token string
	Repo  string
}

func (g *ghRunner) run(ctx context.Context, args ...string) (string, error) {
	if g.Repo != "" {
		args = append([]string{"-R", g.Repo}, args...)
	}
	cmd := exec.CommandContext(ctx, g.Bin, args...)
	if g.Token != "" {
		cmd.Env = append(cmd.Environ(), "GH_TOKEN="+g.Token)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func addGhTools(srv *mcpserver.MCPServer, c *store.Connector, creds map[string]string, desc, group string) error {
	runner := &ghRunner{Bin: cfgStr(c, "gh_bin"), Token: creds["gh_token"], Repo: cfgStr(c, "repo")}
	if runner.Bin == "" {
		runner.Bin = "gh"
	}
	if group == "issues" {
		list := mcp.NewTool("gh_issue_list", mcp.WithDescription(desc+"：issue 清单（gh issue list，≤30 条）"),
			mcp.WithString("state", mcp.Description("状态过滤：open|closed|all（默认 open）")),
			mcp.WithString("label", mcp.Description("标签过滤")))
		srv.AddTool(list, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			a := req.GetArguments()
			args := []string{"issue", "list", "--limit", "30"}
			if st := str(a, "state"); st == "open" || st == "closed" || st == "all" {
				args = append(args, "--state", st)
			}
			if lb := str(a, "label"); lb != "" {
				args = append(args, "--label", lb)
			}
			out, err := runner.run(ctx, args...)
			return mcp.NewToolResultText(toolText(out, err)), nil
		})
		view := mcp.NewTool("gh_issue_view", mcp.WithDescription(desc+"：单 issue 详情与评论（gh issue view --comments）"),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("issue 编号")))
		srv.AddTool(view, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			n := int(req.GetArguments()["number"].(float64))
			out, err := runner.run(ctx, "issue", "view", fmt.Sprint(n), "--comments")
			return mcp.NewToolResultText(toolText(out, err)), nil
		})
		return nil
	}
	// ci 组
	runs := mcp.NewTool("gh_run_list", mcp.WithDescription(desc+"：CI 运行清单（gh run list，≤20 条）"),
		mcp.WithNumber("limit", mcp.Description("条数（1~20，默认 10）")))
	srv.AddTool(runs, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		n := clampInt(num(req.GetArguments(), "limit"), 1, 20, 10)
		out, err := runner.run(ctx, "run", "list", "--limit", fmt.Sprint(n))
		return mcp.NewToolResultText(toolText(out, err)), nil
	})
	view := mcp.NewTool("gh_run_view", mcp.WithDescription(desc+"：单次 CI 运行详情（gh run view <id>）"),
		mcp.WithNumber("run_id", mcp.Required(), mcp.Description("run id")))
	srv.AddTool(view, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := int64(num(req.GetArguments(), "run_id"))
		out, err := runner.run(ctx, "run", "view", fmt.Sprint(id))
		return mcp.NewToolResultText(toolText(out, err)), nil
	})
	return nil
}

// TestReport 测试报告结构化结果（test_report_parse 出料）。
type TestReport struct {
	Format   string           `json:"format"` // go-test-json | junit-xml
	Total    int              `json:"total"`
	Passed   int              `json:"passed"`
	Failed   int              `json:"failed"`
	Skipped  int              `json:"skipped"`
	Failures []TestReportFail `json:"failures,omitempty"`
}

// TestReportFail 失败条目（名称+输出截断）。
type TestReportFail struct {
	Name   string `json:"name"`
	Output string `json:"output,omitempty"`
}

// ParseTestReport 测试报告解析（go test -json 行流 / junit XML 自动判别——107 §5.3 测试执行
// 数据源的结构化出料；非诊断 run_command 的替代，执行面仍走 run_command 沙箱）。
func ParseTestReport(raw string) (*TestReport, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("报告内容为空")
	}
	if strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, "\"Action\"") {
		return parseGoTestJSON(trimmed), nil
	}
	if strings.HasPrefix(trimmed, "<") {
		return parseJUnitXML(trimmed)
	}
	return nil, fmt.Errorf("无法识别报告格式（须 go test -json 输出或 junit XML）")
}

// goTestJSONLine go test -json 单行事件的最小投影。
type goTestJSONLine struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

func parseGoTestJSON(s string) *TestReport {
	rep := &TestReport{Format: "go-test-json"}
	seen := map[string]string{} // test name → last action（run 内多事件，以最终态计）
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var ev goTestJSONLine
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.Test == "" {
			continue // 包级事件不计
		}
		name := ev.Package + "/" + ev.Test
		seen[name] = ev.Action
		if ev.Action == "fail" {
			rep.Failures = append(rep.Failures, TestReportFail{Name: name, Output: ""})
		}
	}
	// 失败输出从"output"事件粗收（同 test 名下的 === RUN 后 output 行——截断 500 rune）
	outputs := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		var ev goTestJSONLine
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &ev) != nil || ev.Test == "" || ev.Action != "output" {
			continue
		}
		name := ev.Package + "/" + ev.Test
		if len(outputs[name]) < 500 {
			outputs[name] += ev.Output
		}
	}
	for i := range rep.Failures {
		rep.Failures[i].Output = strings.TrimSpace(outputs[rep.Failures[i].Name])
	}
	for _, act := range seen {
		rep.Total++
		switch act {
		case "pass":
			rep.Passed++
		case "fail":
			rep.Failed++
		case "skip":
			rep.Skipped++
		}
	}
	return rep
}

// junitSuites junit XML 最小投影（testsuite/tests 嵌套各取常见形态）。
type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []junitSuite `xml:"testsuite"`
}
type junitSuite struct {
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Cases    []junitCase `xml:"testcase"`
}
type junitCase struct {
	Name      string    `xml:"name,attr"`
	Classname string    `xml:"classname,attr"`
	Failure   *struct{} `xml:"failure"`
	Skipped   *struct{} `xml:"skipped"`
}

func parseJUnitXML(s string) (*TestReport, error) {
	rep := &TestReport{Format: "junit-xml"}
	var root junitSuites
	// 双根兼容：testsuites 包裹优先；解析失败或空包一层时按单 testsuite 根重试（junit 常见两形态）
	if err := xml.Unmarshal([]byte(s), &root); err != nil || len(root.Suites) == 0 {
		var single junitSuite
		if err2 := xml.Unmarshal([]byte(s), &single); err2 != nil {
			if err != nil {
				return nil, fmt.Errorf("junit XML 解析失败: %w", err)
			}
			return nil, fmt.Errorf("junit XML 解析失败: %w", err2)
		}
		root.Suites = []junitSuite{single}
	}
	for _, su := range root.Suites {
		rep.Total += len(su.Cases)
		for _, tc := range su.Cases {
			name := tc.Classname + "/" + tc.Name
			switch {
			case tc.Failure != nil:
				rep.Failed++
				rep.Failures = append(rep.Failures, TestReportFail{Name: name})
			case tc.Skipped != nil:
				rep.Skipped++
			default:
				rep.Passed++
			}
		}
	}
	return rep, nil
}

// num/str/clampInt 小工具（plugin.go str 同缝；num/clamp 本文件私有）。
func num(a map[string]any, key string) float64 {
	v, _ := a[key].(float64)
	return v
}

func clampInt(v float64, lo, hi int, def int) int {
	n := int(v)
	if n == 0 {
		return def
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// addTestReportTool test_report_parse（纯 Go 零子进程；入参=报告全文）。
func addTestReportTool(srv *mcpserver.MCPServer, desc string) error {
	t := mcp.NewTool("test_report_parse",
		mcp.WithDescription(desc+"：把测试运行输出解析为结构化 pass/fail 清单（go test -json 或 junit XML；配合 run_command 执行测试后使用）"),
		mcp.WithString("report", mcp.Required(), mcp.Description("测试报告全文（go test -json 输出行流或 junit XML）")))
	srv.AddTool(t, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rep, err := ParseTestReport(str(req.GetArguments(), "report"))
		if err != nil {
			return mcp.NewToolResultText("解析失败: " + err.Error()), nil
		}
		b, _ := json.Marshal(rep)
		return mcp.NewToolResultText(string(b)), nil
	})
	return nil
}
