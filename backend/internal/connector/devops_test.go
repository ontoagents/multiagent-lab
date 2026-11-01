package connector

// REQ-554/M361 研发数据源 devops 插件单测：工具清单映射/测试报告解析（go test -json + junit XML）/
// SHA 白名单/git 连接器端到端（真 git init 临时仓——只读五工具注册+调用出料）/testreport 组端到端。

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	"github.com/ontoagents/ontoagents/backend/internal/store"
	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

func TestDevopsToolNames_Req554(t *testing.T) {
	if got := DevopsToolNames("git"); len(got) != 5 || got[0] != "git_status" {
		t.Fatalf("git 工具清单失真: %v", got)
	}
	if got := DevopsToolNames("issues"); len(got) != 2 || got[0] != "gh_issue_list" {
		t.Fatalf("issues 工具清单失真: %v", got)
	}
	if got := DevopsToolNames("ci"); len(got) != 2 || got[0] != "gh_run_list" {
		t.Fatalf("ci 工具清单失真: %v", got)
	}
	if got := DevopsToolNames("testreport"); len(got) != 1 || got[0] != "test_report_parse" {
		t.Fatalf("testreport 工具清单失真: %v", got)
	}
	if got := DevopsToolNames("bogus"); got != nil {
		t.Fatalf("非法 datasource 应空: %v", got)
	}
}

func TestParseTestReport_Req554(t *testing.T) {
	// go test -json：2 过 1 败 1 跳（失败输出收同 test 名下 output 行）
	goJSON := strings.Join([]string{
		`{"Action":"run","Package":"pkg/a","Test":"TestOk"}`,
		`{"Action":"output","Package":"pkg/a","Test":"TestOk","Output":"--- PASS: ok\n"}`,
		`{"Action":"pass","Package":"pkg/a","Test":"TestOk"}`,
		`{"Action":"run","Package":"pkg/a","Test":"TestBad"}`,
		`{"Action":"output","Package":"pkg/a","Test":"TestBad","Output":"expected 1 got 2\n"}`,
		`{"Action":"fail","Package":"pkg/a","Test":"TestBad"}`,
		`{"Action":"run","Package":"pkg/a","Test":"TestSkip"}`,
		`{"Action":"skip","Package":"pkg/a","Test":"TestSkip"}`,
		`{"Action":"pass","Package":"pkg/a"}`,
	}, "\n")
	rep, err := ParseTestReport(goJSON)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Format != "go-test-json" || rep.Total != 3 || rep.Passed != 1 || rep.Failed != 1 || rep.Skipped != 1 {
		t.Fatalf("go json 计数失真: %+v", rep)
	}
	if len(rep.Failures) != 1 || rep.Failures[0].Name != "pkg/a/TestBad" || !strings.Contains(rep.Failures[0].Output, "expected 1 got 2") {
		t.Fatalf("失败条目失真: %+v", rep.Failures)
	}
	// junit XML（testsuites 包裹）
	junit := `<testsuites><testsuite tests="2" failures="1" skipped="0">
<testcase classname="pkg.B" name="TestX"></testcase>
<testcase classname="pkg.B" name="TestY"><failure message="boom"/></testcase>
</testsuite></testsuites>`
	rep2, err := ParseTestReport(junit)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Format != "junit-xml" || rep2.Total != 2 || rep2.Passed != 1 || rep2.Failed != 1 {
		t.Fatalf("junit 计数失真: %+v", rep2)
	}
	if len(rep2.Failures) != 1 || rep2.Failures[0].Name != "pkg.B/TestY" {
		t.Fatalf("junit 失败条目失真: %+v", rep2.Failures)
	}
	// 单 testsuite 根 + 未知格式 + 空
	if _, err := ParseTestReport(`<testsuite tests="1"><testcase name="a"/></testsuite>`); err != nil {
		t.Fatalf("单 suite 根应可解析: %v", err)
	}
	if _, err := ParseTestReport("random text"); err == nil {
		t.Fatal("未知格式应报错")
	}
	if _, err := ParseTestReport("   "); err == nil {
		t.Fatal("空内容应报错")
	}
}

func TestIsSafeSha_Req554(t *testing.T) {
	for _, ok := range []string{"abcdef12", "ABCDEF0123456789abcdef0123456789abcdef01", "1234"} {
		if !isSafeSha(ok) {
			t.Fatalf("%q 应合法", ok)
		}
	}
	for _, bad := range []string{"", "abc", "abc;rm", "a b", "abcdefg!", "../../etc"} {
		if isSafeSha(bad) {
			t.Fatalf("%q 应拒绝", bad)
		}
	}
}

// initTempRepo 真 git 临时仓（init+config+commit——git CLI 随开发环境在 PATH）。
func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "tester")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello req554\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init commit req554")
	return dir
}

func TestPluginDevopsGitEndToEnd_Req554(t *testing.T) {
	p, st, _ := newPluginFixture(t)
	repo := initTempRepo(t)
	c, err := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindDevops, Name: "repo-main",
		Config: map[string]any{"datasource": "git", "repo_dir": repo},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	bts, err := tool.FetchMCPTools(context.Background(), c.Name, srv.URL+"/connectors/"+c.ID+"/mcp", 10e9)
	if err != nil {
		t.Fatalf("fetch tools: %v", err)
	}
	got := map[string]einotool.BaseTool{}
	for _, bt := range bts {
		ti, _ := bt.Info(context.Background())
		got[ti.Name] = bt
	}
	for _, w := range DevopsToolNames("git") {
		if _, ok := got[c.Name+"__"+w]; !ok {
			t.Fatalf("缺工具 %s，实际 %v", w, keysOf(got))
		}
	}
	// git_log 调用出料：含本次提交信息
	inv, ok := got[c.Name+"__git_log"].(einotool.InvokableTool)
	if !ok {
		t.Fatal("git_log 应为 InvokableTool")
	}
	out, err := inv.InvokableRun(context.Background(), marshalArgs(map[string]any{"limit": 5}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "init commit req554") {
		t.Fatalf("git_log 出料应含提交信息: %s", out)
	}
	// git_status 调用（干净仓=空输出不报错）
	stInv, _ := got[c.Name+"__git_status"].(einotool.InvokableTool)
	if _, err := stInv.InvokableRun(context.Background(), marshalArgs(map[string]any{})); err != nil {
		t.Fatalf("git_status: %v", err)
	}
	// 非法 datasource 注册被拒（诚实报错不静默空表）
	c2, _ := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindDevops, Name: "repo-bad",
		Config: map[string]any{"datasource": "bogus", "repo_dir": repo},
	})
	if _, herr := p.handlerOf(c2.ID); herr == nil {
		t.Fatal("非法 datasource 应注册失败")
	}
}

func TestPluginDevopsTestReportEndToEnd_Req554(t *testing.T) {
	p, st, _ := newPluginFixture(t)
	c, err := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindDevops, Name: "tr",
		Config: map[string]any{"datasource": "testreport"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()
	bts, err := tool.FetchMCPTools(context.Background(), c.Name, srv.URL+"/connectors/"+c.ID+"/mcp", 10e9)
	if err != nil {
		t.Fatalf("fetch tools: %v", err)
	}
	if len(bts) != 1 {
		t.Fatalf("testreport 组应 1 工具: %d", len(bts))
	}
	inv, ok := bts[0].(einotool.InvokableTool)
	if !ok {
		t.Fatal("test_report_parse 应为 InvokableTool")
	}
	sample := fmt.Sprintf("%s\n%s\n%s", `{"Action":"run","Package":"p","Test":"T1"}`, `{"Action":"output","Package":"p","Test":"T1","Output":"boom\n"}`, `{"Action":"fail","Package":"p","Test":"T1"}`)
	out, err := inv.InvokableRun(context.Background(), marshalArgs(map[string]any{"report": sample}))
	if err != nil {
		t.Fatal(err)
	}
	// 出料为 JSON-in-JSON（MCP text 帧转义）——按关键词断言计数与失败输出
	if !strings.Contains(out, "failed") || !strings.Contains(out, "boom") || !strings.Contains(out, "go-test-json") {
		t.Fatalf("解析出料失真: %s", out)
	}
}
