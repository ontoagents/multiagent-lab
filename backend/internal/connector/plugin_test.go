// REQ-214/M46 连接器插件服务端到端单测（stub kubectl）：
// 经标准 MCP 客户端管线（tool.FetchMCPTools）访问 /connectors/{id}/mcp——工具前缀改名/调用/
// 凭据服务端注入（临时 kubeconfig）/非零 exit code 透传；凭据不进工具参数与快照。
package connector

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

// writeStubExe 平台自适应落 stub 可执行（REQ-321 Windows 沿例）：正文脚本原样落盘（unix 直接
// 可执行）；Windows 的 exec 不可直接执行无扩展名脚本——外补同名 .cmd 垫片（bash 解释执行同一
// 正文）补上 PATH 入口，返回 exec 可见路径。垫片先前置 Git usr/bin 到子进程 PATH：正文里的
// 外部命令在无 Git PATH 环境下也要可解析。
func writeStubExe(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		return p
	}
	shim := filepath.Join(dir, name+".cmd")
	if err := os.WriteFile(shim, []byte(stubShimBody(stubBashPath(t), p)), 0o644); err != nil {
		t.Fatal(err)
	}
	return shim
}

// stubBashPath Windows 下 stub 解释器定位：PATH 里的 bash 常为 WSL 启动器（System32\bash.exe，
// 无发行版时 execvpe(/bin/bash) 报错，gate 环境实证）——拒收后由 git.exe 反推 Git for
// Windows 安装根，再常见安装位置兜底。
func stubBashPath(t *testing.T) string {
	if p, err := exec.LookPath("bash"); err == nil && !stubIsWSLBash(p) {
		return p
	}
	if gp, err := exec.LookPath("git"); err == nil {
		root := filepath.Dir(filepath.Dir(gp)) // …\Git\cmd\git.exe → …\Git
		for _, rel := range []string{`bin\bash.exe`, `usr\bin\bash.exe`} {
			if c := filepath.Join(root, rel); c != gp {
				if _, err := os.Stat(c); err == nil {
					return c
				}
			}
		}
	}
	for _, c := range []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files\Git\usr\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Fatal("stub 解释器 bash 不可达（PATH 仅 WSL 启动器且未找到 Git for Windows）")
	return ""
}

func stubIsWSLBash(p string) bool {
	lp := strings.ToLower(p)
	return strings.Contains(lp, `\system32\`) || strings.Contains(lp, `\windowsapps\`)
}

// stubShimBody .cmd 垫片内容：bash 档先把同安装 usr/bin 前置进子进程 PATH（外部命令随取）。
func stubShimBody(bash, script string) string {
	var b strings.Builder
	if usrBin := stubUsrBin(bash); usrBin != "" {
		b.WriteString("@set \"PATH=" + usrBin + ";%PATH%\"\r\n")
	}
	b.WriteString("@\"" + bash + "\" \"" + script + "\" %*\r\n")
	return b.String()
}

// stubUsrBin bash 同安装的 POSIX 工具目录（…\Git\usr\bin）；定位不到返回 ""（不前置）。
func stubUsrBin(bash string) string {
	bin := filepath.Dir(bash)
	usrBin := bin
	if strings.EqualFold(filepath.Base(bin), "bin") && !strings.EqualFold(filepath.Base(filepath.Dir(bin)), "usr") {
		usrBin = filepath.Join(filepath.Dir(bin), "usr", "bin")
	}
	if _, err := os.Stat(filepath.Join(usrBin, "tr.exe")); err != nil {
		return ""
	}
	return usrBin
}

// stubKubectl 记录参数并按资源分流：pods → JSON 输出；secret-resource → exit 7（非零透传用例）。
func stubKubectl(t *testing.T) (bin, logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "calls.log")
	script := "#!/bin/bash\necho \"$@\" >> \"" + logPath + "\"\n" +
		"case \"$*\" in\n" +
		"  *\"get pods\"*) echo '{\"items\":[{\"metadata\":{\"name\":\"pod-a\"}}]}' ;;\n" +
		"  *\"get secret-resource\"*) echo 'rbac denied' >&2; exit 7 ;;\n" +
		"  *\" version \"*) echo '{\"serverVersion\":{\"gitVersion\":\"v1.31.0\"}}' ;;\n" +
		"  *) echo '(stub ok)' ;;\n" +
		"esac\n"
	bin = writeStubExe(t, dir, "kubectl", script)
	return bin, logPath
}

func newPluginFixture(t *testing.T) (*PluginService, *store.Store, *secrets.Box) {
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
	return NewPlugin(st, box, ""), st, box
}

// 凭据态 kubeconfig（加密落库 → 插件服务解密 → 临时文件注入 CLI；内容含哨兵串）。
const fakeKubeconfig = "apiVersion: v1\nkind: Config\n# SENTINEL-KUBECONFIG"

func encryptKubeconfig(t *testing.T, box *secrets.Box) []byte {
	t.Helper()
	b, err := box.Encrypt(`{"kubeconfig":` + strconv.Quote(fakeKubeconfig) + `}`)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPluginKubernetesMCPEndToEnd(t *testing.T) {
	p, st, box := newPluginFixture(t)
	bin, logPath := stubKubectl(t)
	p.KubectlBin = bin
	enc := encryptKubeconfig(t, box)
	c, err := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindKubernetes, Name: "k8s-ops",
		Config:               map[string]any{"context": "ctx-a", "namespace": "ns-a"},
		CredentialsEncrypted: enc,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	// 1) 标准 MCP 客户端管线：列工具 + {实例名}__{tool} 前缀
	bts, err := tool.FetchMCPTools(context.Background(), c.Name, srv.URL+"/connectors/"+c.ID+"/mcp", 10e9)
	if err != nil {
		t.Fatalf("fetch tools: %v", err)
	}
	want := []string{"k8s-ops__kubectl_get", "k8s-ops__kubectl_describe", "k8s-ops__kubectl_logs", "k8s-ops__kubectl_apply"}
	got := map[string]einotool.BaseTool{}
	for _, bt := range bts {
		ti, _ := bt.Info(context.Background())
		got[ti.Name] = bt
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Fatalf("缺工具 %s，实际 %v", w, keysOf(got))
		}
	}

	// 2) 调用 kubectl_get：参数/凭据服务端注入（--kubeconfig 临时文件含哨兵内容）
	inv, ok := got["k8s-ops__kubectl_get"].(einotool.InvokableTool)
	if !ok {
		t.Fatal("kubectl_get 应为 InvokableTool")
	}
	out, err := inv.InvokableRun(context.Background(), marshalArgs(map[string]any{"resource": "pods"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "pod-a") {
		t.Fatalf("输出应含 stub 数据: %s", out)
	}
	calls, _ := os.ReadFile(logPath)
	// -n ns-a（连接器缺省命名空间）与 --context ctx-a 透传
	if !strings.Contains(string(calls), "-n ns-a") || !strings.Contains(string(calls), "--context ctx-a") {
		t.Fatalf("调用参数缺失: %s", calls)
	}
	// --kubeconfig 指向的临时文件内容 = 凭据 kubeconfig
	kc := kubeconfigArgOf(string(calls))
	if kc == "" {
		t.Fatal("调用应带 --kubeconfig")
	}
	content, err := os.ReadFile(kc)
	if err != nil || !strings.Contains(string(content), "SENTINEL-KUBECONFIG") {
		t.Fatalf("临时 kubeconfig 应含凭据内容: %v %s", err, content)
	}

	// 3) 非零 exit code 透传
	out, err = inv.InvokableRun(context.Background(), marshalArgs(map[string]any{"resource": "secret-resource"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[exit code 7]") || !strings.Contains(out, "rbac denied") {
		t.Fatalf("exit code/stderr 应透传: %s", out)
	}
	_ = bin
}

func marshalArgs(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// 工具入参仅业务语义：凭证/凭据字段不在 inputSchema。
func TestPluginToolSchemaNoCredentials(t *testing.T) {
	p, st, box := newPluginFixture(t)
	enc := encryptKubeconfig(t, box)
	c, _ := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindKubernetes, Name: "k8s-x",
		Config:               map[string]any{"kubeconfig_path": "/tmp/kc.yaml"},
		CredentialsEncrypted: enc,
	})
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()
	bts, err := tool.FetchMCPTools(context.Background(), c.Name, srv.URL+"/connectors/"+c.ID+"/mcp", 10e9)
	if err != nil {
		t.Fatal(err)
	}
	for _, bt := range bts {
		ti, _ := bt.Info(context.Background())
		if strings.Contains(strings.ToLower(ti.Desc), "kubeconfig") && strings.Contains(ti.Desc, "SENTINEL") {
			t.Fatalf("工具描述泄漏凭据: %s", ti.Desc)
		}
		// inputSchema 序列化不含凭据字段（kubectl_get 属性白名单）
		if strings.HasSuffix(ti.Name, "kubectl_get") {
			raw, _ := json.Marshal(ti)
			for _, forbidden := range []string{"kubeconfig", "password", "private_key"} {
				if strings.Contains(string(raw), `"`+forbidden+`"`) {
					t.Fatalf("inputSchema 不应含 %s: %s", forbidden, raw)
				}
			}
		}
	}
}

// mcp kind 不由插件服务承载（走直通），未知 kind 502。
func TestPluginRejectsMCPKind(t *testing.T) {
	p, st, _ := newPluginFixture(t)
	c, _ := st.CreateConnector(&store.Connector{Kind: store.ConnectorKindMCP, Name: "plain", Config: map[string]any{"url": "http://x/mcp"}})
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()
	if _, err := tool.FetchMCPTools(context.Background(), c.Name, srv.URL+"/connectors/"+c.ID+"/mcp", 5e9); err == nil {
		t.Fatal("mcp kind 应被插件服务拒绝")
	}
}

func keysOf(m map[string]einotool.BaseTool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func kubeconfigArgOf(calls string) string {
	for _, line := range strings.Split(calls, "\n") {
		f := strings.Fields(line)
		for i, a := range f {
			if a == "--kubeconfig" && i+1 < len(f) {
				return f[i+1]
			}
		}
	}
	return ""
}

// exitError 文案（toolText 合流行为）。
func TestToolTextExitCode(t *testing.T) {
	if s := toolText("out", &exitError{Code: 7, Stderr: "boom"}); !strings.Contains(s, "[exit code 7] boom") {
		t.Fatalf("toolText: %s", s)
	}
	if s := toolText("", nil); s != "(命令成功，无输出)" {
		t.Fatalf("空输出提示: %s", s)
	}
}

// REQ-214 P2⑦：只读模式——tools/list 不含 kubectl_apply（工具级白名单最轻形态）。
func TestPluginKubernetesReadOnly(t *testing.T) {
	p, st, box := newPluginFixture(t)
	bin, _ := stubKubectl(t)
	p.KubectlBin = bin
	enc := encryptKubeconfig(t, box)
	c, err := st.CreateConnector(&store.Connector{
		Kind: store.ConnectorKindKubernetes, Name: "k8s-ro",
		Config:               map[string]any{"read_only": true},
		CredentialsEncrypted: enc,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()
	bts, err := tool.FetchMCPTools(context.Background(), c.Name, srv.URL+"/connectors/"+c.ID+"/mcp", 10e9)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, bt := range bts {
		if ti, _ := bt.Info(context.Background()); ti != nil {
			names[ti.Name] = true
		}
	}
	if names["k8s-ro__kubectl_apply"] {
		t.Fatal("只读模式不应注册 kubectl_apply")
	}
	if !names["k8s-ro__kubectl_get"] || !names["k8s-ro__kubectl_logs"] {
		t.Fatalf("只读模式应保留读工具: %v", names)
	}
	if got := KubernetesToolNames(true); len(got) != 3 {
		t.Fatalf("只读静态清单应 3 项: %v", got)
	}
	if got := KubernetesToolNames(false); len(got) != 4 {
		t.Fatalf("完整静态清单应 4 项: %v", got)
	}
}
