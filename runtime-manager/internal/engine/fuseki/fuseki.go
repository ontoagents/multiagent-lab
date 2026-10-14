// Package fuseki Apache Jena Fuseki 适配器（O6，方案 04 §4.2：SPARQL + RDFS/OWL 推理，可配置开启）。
// 启动流程：按 profile 建数据目录 → fuseki-server --conf 装载（推理级别经 config.ttl 的
// tdb:unionDefaultGraph + ja:model/infModel 表达）→ 健康检查 /$/ping。
// 推理实现说明（P1 通用版）：为控制复杂度，推理开关经 JVM 级 --generalAssemblerProfile
// 不可行（需 per-dataset assembler 配置），故采用方案 A：每个 profile 生成一份 config.ttl，
// dataset 用 infModel 包装（reasoning=true 时 rdfsLevel=OWL_RDFS_MEM_RDFS_INF，false 时纯 TDB）。
// Java 二进制要求：JDK 11+（apache-jena-fuseki 6.x 需 17+，见 FUSEKI_JAVA 提示）。
package fuseki

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/engine"
	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/procgroup"
)

type Runtime struct {
	Binary  string // fuseki-server 启动脚本路径（可空——经 candidatePaths 动态解析，REQ-307）
	JavaBin string // java 可执行（FUSEKI_JAVA，默认 PATH 上的 java）
	DataDir string // 数据根目录（每 profile 一个子目录）
	LogDir  string // 引擎日志目录
}

// PinnedVersion 一键安装 pin 的官方发行版本（REQ-307）。
const PinnedVersion = "6.2.0"

// ReleaseURLs 发行包下载候选 URL（依次尝试；教育网/云厂商镜像实测快官方源两个数量级，
// 首选北大/华为云；downloads 仅保留当前版本、archive 为全历史归档兜底；
// tar.gz 含 fuseki-server 脚本与 fuseki-server.jar）。
func ReleaseURLs() []string {
	name := "apache-jena-fuseki-" + PinnedVersion + ".tar.gz"
	return []string{
		"https://mirrors.pku.edu.cn/apache/jena/binaries/" + name,
		"https://mirrors.huaweicloud.com/apache/jena/binaries/" + name,
		"https://mirrors.tuna.tsinghua.edu.cn/apache/jena/binaries/" + name,
		"https://mirrors.aliyun.com/apache/jena/binaries/" + name,
		"https://downloads.apache.org/jena/binaries/" + name,
		"https://archive.apache.org/dist/jena/binaries/" + name,
	}
}

// ExtractServerFiles 从已下载的 apache-jena-fuseki tar.gz 提取运行必需的两件（fuseki-server
// 启动脚本 + fuseki-server.jar）到 destDir。只按 base 名匹配这两件（天然免疫包内目录前缀差异）；
// 拒绝 ".." 路径条目（防解包穿越）。java 为运行时依赖而非安装依赖，安装不要求 JDK。
func ExtractServerFiles(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip 解压失败（非 tar.gz 发行包？）: %w", err)
	}
	defer gz.Close()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	want := map[string]bool{"fuseki-server": false, "fuseki-server.jar": false}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar 读取失败: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(hdr.Name)
		if _, isTarget := want[name]; !isTarget {
			continue // 非目标文件
		}
		if want[name] {
			continue // 已提取（重名取首见）
		}
		if strings.Contains(hdr.Name, "..") {
			return fmt.Errorf("发行包含可疑路径条目，已中止: %s", hdr.Name)
		}
		dst := filepath.Join(destDir, name)
		out, err := os.OpenFile(dst+".download", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(dst + ".download")
			return fmt.Errorf("提取 %s 失败: %w", name, err)
		}
		if err := os.Rename(dst+".download", dst); err != nil {
			return err
		}
		want[name] = true
	}
	missing := []string{}
	for k, ok := range want {
		if !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("发行包缺少运行文件: %s（pin %s 的包形态可能已变化）", strings.Join(missing, ","), PinnedVersion)
	}
	return nil
}

// DownloadFunc 下载函数形态（manager.downloadTo 同签名，解耦下载机制便于单测）。
type DownloadFunc func(ctx context.Context, url, target string, minBytes int64) error

// Install 一键安装：依次尝试 urls 下载发行包到临时文件 → 提取两件到 destDir → 清理。
func Install(ctx context.Context, destDir string, urls []string, download DownloadFunc) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("fuseki 一键安装暂不支持 windows（fuseki-server 为 shell 脚本）：请从 https://jena.apache.org/download/ 下载 zip 发行包手动配置 FUSEKI_BIN")
	}
	tmp := filepath.Join(os.TempDir(), "apache-jena-fuseki-"+PinnedVersion+".tar.gz.download")
	defer os.Remove(tmp)
	var lastErr error
	for _, url := range urls {
		if err := download(ctx, url, tmp, 50<<20); err != nil {
			lastErr = err
			continue
		}
		if err := ExtractServerFiles(tmp, destDir); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("全部下载源失败: %w", lastErr)
}

func New(binary, javaBin, dataDir, logDir string) *Runtime {
	_ = os.MkdirAll(dataDir, 0o755)
	_ = os.MkdirAll(logDir, 0o755)
	return &Runtime{Binary: binary, JavaBin: javaBin, DataDir: dataDir, LogDir: logDir}
}

// candidatePaths fuseki-server 候选序（REQ-307）：显式配置 → PATH → data/bin（一键安装落点，
// 装后即命中、无需重启 runtimed）。与 oxigraph 适配器搜索语义一致。
func (r *Runtime) candidatePaths() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(r.Binary)
	if p, err := exec.LookPath("fuseki-server"); err == nil {
		add(p)
	}
	add("data/bin/fuseki-server")
	return out
}

// resolveBinary 探测可用 fuseki-server；全落空给可自助的安装指引（含一键安装提示）。
func (r *Runtime) resolveBinary() (string, error) {
	cands := r.candidatePaths()
	if bin, ok := engine.FindExecutable(cands); ok {
		return bin, nil
	}
	return "", fmt.Errorf("未找到 fuseki-server（已探测 %s）: 可在本体运行页「一键下载安装」（官方发行包 pin %s，写入 data/bin 即时生效，运行需 JDK 17+），或从 https://jena.apache.org/download/ 下载 apache-jena-fuseki 解压后设置 FUSEKI_BIN 指向其 fuseki-server 脚本",
		strings.Join(cands, " → "), PinnedVersion)
}

// Probe 引擎自检（REQ-146/REQ-307）：支持一键安装（官方发行包自动下载解包，java 为运行时依赖）。
func (r *Runtime) Probe() engine.EngineStatus {
	st := engine.EngineStatus{Engine: "fuseki", Registered: true, Installable: true}
	cands := r.candidatePaths()
	st.Searched = cands
	if bin, ok := engine.FindExecutable(cands); ok {
		st.Installed = true
		st.Binary = bin
		if _, jok := engine.FindExecutable([]string{r.JavaBin, "java"}); !jok {
			st.Hint = "fuseki-server 已找到，但未找到 java（JDK 17+）：安装 JDK 并加入 PATH，或设置 FUSEKI_JAVA 后重启 runtimed"
		}
		return st
	}
	st.Hint = "未安装：可在本体运行页「一键下载安装」（官方发行包自动下载解包到 data/bin，即时生效）；运行需 JDK 17+（java 缺失时安装后仍会提示）；或手动下载 apache-jena-fuseki（https://jena.apache.org/download/）设置 FUSEKI_BIN"
	return st
}

// reasoningConfig 生成带/不带推理的 dataset 配置（Turtle，assembler 语法）。
// reasoning=true：ja:InfModel + RDFS/OWL 规则推理（GenericRuleReasoner，OWL_FB 规则集）；
// reasoning=false：纯 TDB2 数据集（与 Oxigraph 同为无推理基线）。
// rulesFileURL 为 OWL_FB 规则文件绝对 file: URL（Start 时从 fuseki-server.jar 解出）。
//
// 数据一律装入默认图（tdbloader 不带 --graph）：TDB2 的 unionDefaultGraph 只在
// 数据集级 assembler 生效，图级 tdb:GraphTDB 会静默忽略，推理侧 InfModel 的
// baseModel 指向默认图才能看到数据。多本体共存默认图，URI 按本体 ID 命名空间隔离。
// 〔REQ-320 演进〕regular 无推理方案改 per-graph：--graph=urn:onto:{id} 逐本体独立命名图 +
// 数据集级 tdb:unionDefaultGraph（union 分支）；推理形态保持默认图（图级 union 静默忽略的
// 勘误仍在，推理 per-graph 待 Jena 数据集级 InfModel 改造）；伴生形态恒默认图（路径不动）。
func reasoningConfig(datasetName, dbDir, rulesFileURL string, reasoning, union bool) string {
	var b strings.Builder
	b.WriteString("@prefix tdb:   <http://jena.apache.org/2016/tdb#> .\n")
	b.WriteString("@prefix rdf:   <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .\n")
	b.WriteString("@prefix rdfs:  <http://www.w3.org/2000/01/rdf-schema#> .\n")
	b.WriteString("@prefix ja:    <http://jena.hpl.hp.com/2005/11/Assembler#> .\n")
	b.WriteString("@prefix fuseki: <http://jena.apache.org/fuseki#> .\n\n")
	b.WriteString("<#service> rdf:type fuseki:Service ;\n")
	fmt.Fprintf(&b, "    fuseki:name \"%s\" ;\n", datasetName)
	b.WriteString("    fuseki:endpoint [ fuseki:operation fuseki:query ; ] ;\n")
	b.WriteString("    fuseki:endpoint [ fuseki:operation fuseki:update ; ] ;\n")
	b.WriteString("    fuseki:dataset <#dataset> ;\n    .\n\n")
	if reasoning {
		// 推理模式：InfModel 包 TDB2 默认图（数据装入默认图，见函数头注释）
		b.WriteString("<#dataset> rdf:type ja:RDFDataset ;\n")
		b.WriteString("    ja:defaultGraph <#infModel> ;\n    .\n\n")
		b.WriteString("<#baseModel> rdf:type tdb:GraphTDB ;\n")
		fmt.Fprintf(&b, "    tdb:location \"%s\" ;\n", dbDir)
		b.WriteString("    .\n\n")
		b.WriteString("<#infModel> rdf:type ja:InfModel ;\n")
		b.WriteString("    ja:baseModel <#baseModel> ;\n")
		b.WriteString("    ja:reasoner [ rdf:type ja:GenericRuleReasoner ;\n")
		fmt.Fprintf(&b, "        ja:rulesFrom <%s> ;\n", rulesFileURL)
		b.WriteString("        ja:mode \"FULL\" ; ] ;\n    .\n")
	} else if union {
		// regular 无推理（REQ-320）：数据集级 union——命名图并集作默认图视图（与 Oxigraph
		// --union-default-graph 同语义；数据全在命名图，无「盖掉裸默认图」问题）
		b.WriteString("<#dataset> rdf:type tdb:DatasetTDB2 ;\n")
		fmt.Fprintf(&b, "    tdb:location \"%s\" ;\n", dbDir)
		b.WriteString("    tdb:unionDefaultGraph true ;\n")
		b.WriteString("    .\n")
	} else {
		// 无推理：纯 TDB2 数据集。数据在默认图（见函数头注释），不开 union——
		// unionDefaultGraph 使默认图=命名图并集，反而盖掉裸默认图里的数据。
		b.WriteString("<#dataset> rdf:type tdb:DatasetTDB2 ;\n")
		fmt.Fprintf(&b, "    tdb:location \"%s\" ;\n", dbDir)
		b.WriteString("    .\n")
	}
	return b.String()
}

// Start 装载并启动（无推理基线；带推理用 StartWithReasoning）。
func (r *Runtime) Start(ctx context.Context, profileID string, port int, ttls map[string]string) (*engine.Process, error) {
	// legacy 语义=CompanionMode（默认图装载、无 union）；见 StartWithGraphIsolation（REQ-320）。
	return r.startFuseki(ctx, profileID, port, ttls, false, true)
}

// StartWithReasoning 推理开关入口（O6）。legacy 语义=默认图装载、无 union（数据全在裸默认图时
// 开 union 反而使 Jena 的默认图视图变为「命名图并集」而盖掉裸默认图数据——见 reasoningConfig 注）。
func (r *Runtime) StartWithReasoning(ctx context.Context, profileID string, port int, ttls map[string]string, reasoning bool) (*engine.Process, error) {
	return r.startFuseki(ctx, profileID, port, ttls, reasoning, true)
}

// StartWithGraphIsolation per-graph 装载入口（REQ-320/M122）：regular=逐本体 tdbloader
// --graph=urn:onto:{oid} 落独立命名图 + tdb2:unionDefaultGraph（消费面无 GRAPH 子句查询可见
// 全数据集）；CompanionMode=legacy 路径原样（伴生池/宿主边界不动）。
func (r *Runtime) StartWithGraphIsolation(ctx context.Context, profileID string, port int, ttls map[string]string, opts engine.GraphIsolationOpts) (*engine.Process, error) {
	return r.startFuseki(ctx, profileID, port, ttls, opts.Reasoning, opts.CompanionMode)
}

// StartWithReasoning 装载并启动。ttls: ontology_id → TTL 内容；reasoning=true 开 RDFS/OWL 推理。
func (r *Runtime) startFuseki(ctx context.Context, profileID string, port int, ttls map[string]string, reasoning, companionMode bool) (*engine.Process, error) {
	// 预检：fuseki-server 脚本（候选序动态解析，REQ-307——FUSEKI_BIN 未配置时 data/bin 一键安装命中）
	bin, err := r.resolveBinary()
	if err != nil {
		return nil, err
	}
	java := r.JavaBin
	if java == "" {
		java = "java"
	}
	if _, err := exec.LookPath(java); err != nil {
		return nil, fmt.Errorf("未找到 java（Fuseki 为 Java 进程，需 JDK 17+）: 请安装 JDK 并加入 PATH，或设置 FUSEKI_JAVA 指向 java 可执行文件")
	}

	dir := filepath.Join(r.DataDir, profileID)
	_ = os.RemoveAll(dir) // 幂等：重建数据目录（显式重载语义，REQ-87）
	dbDir := filepath.Join(dir, "tdb")
	loadDir := filepath.Join(dir, "load")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	if err := os.MkdirAll(loadDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建装载目录失败: %w", err)
	}

	// 装载：s-put 逐本体入默认图（TDB2 命令行装载，避免 text/turtle POST 到未启动服务）
	for oid, ttl := range ttls {
		f := filepath.Join(loadDir, sanitizeID(oid)+".ttl")
		if err := os.WriteFile(f, []byte(ttl), 0o644); err != nil {
			return nil, err
		}
	}
	// 生成 dataset 配置（推理开关 + union 默认图开关）；推理时从 fuseki-server.jar 解出 OWL_FB 规则文件（绝对 file: URL，不依赖 CWD）
	rulesURL := ""
	if reasoning {
		rf, err := extractRulesFile(java, filepath.Dir(bin), dir)
		if err != nil {
			return nil, err
		}
		rulesURL = "file://" + rf
	}
	cfgPath := filepath.Join(dir, "config.ttl")
	perGraph := !companionMode && !reasoning // 推理形态保持默认图（图级 union 静默忽略勘误，见 reasoningConfig 注）
	if err := os.WriteFile(cfgPath, []byte(reasoningConfig("ds", dbDir, rulesURL, reasoning, perGraph)), 0o644); err != nil {
		return nil, err
	}

	// 装载阶段：tdbloader 离线灌入（快于在线 SPARQL PUT，且不依赖服务启动）。
	// perGraph（REQ-320①）：--graph=urn:onto:{oid} 逐本体落独立命名图（tdbloader 不带图参数只能进
	// 默认图）；其余（伴生形态/推理形态）：不带 --graph 进默认图（推理侧 InfModel baseModel 指向
	// 默认图，路径原样不动——REQ-320 P1 边界）。
	// Fuseki 6.x 精简发行版无独立 tdbloader 脚本，统一走 java -cp fuseki-server.jar tdb2.tdbloader。
	fusekiJar := filepath.Join(filepath.Dir(bin), "fuseki-server.jar")
	for oid := range ttls {
		f := filepath.Join(loadDir, sanitizeID(oid)+".ttl")
		args := []string{"-cp", fusekiJar, "tdb2.tdbloader", "--loc", dbDir}
		if perGraph {
			args = append(args, "--graph="+engine.GraphIRI(oid))
		}
		args = append(args, f)
		cmd := exec.CommandContext(ctx, java, args...)
		cmd.Env = append(os.Environ(), fmt.Sprintf("JAVA_HOME=%s", javaHome(java)))
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("tdbloader 装载 %s 失败: %v: %s", oid, err, tail(out, 300))
		}
		_ = os.Remove(f)
	}

	// serve：fuseki-server --config config.ttl --port（Fuseki 6.x 无 --host 参数，默认监听 localhost）
	cmd := exec.Command(bin, "--config", cfgPath, "--port", fmt.Sprint(port))
	cmd.Env = append(os.Environ(), fmt.Sprintf("JAVA=%s", java), fmt.Sprintf("FUSEKI_HOME=%s", filepath.Dir(bin)))
	procgroup.SetPgid(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("fuseki-server 启动失败: %w", err)
	}
	logPath := filepath.Join(r.LogDir, profileID+".log")
	go drainLog(logPath, stderr)
	go drainLog(logPath, stdout)

	// Fuseki 6.x dataset 端点为 /ds/（POST SPARQL 即查询；GET /$/ping 做健康探测）
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/ds/", port)
	p := engine.NewProcess(endpoint, cmd.Process.Pid, func() error {
		_ = procgroup.Kill(cmd.Process.Pid, true)
		done := make(chan struct{})
		go func() { _, _ = cmd.Process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(8 * time.Second): // JVM 退出较慢，宽限 8s
			_ = procgroup.Kill(cmd.Process.Pid, false)
		}
		return nil
	})
	return p, nil
}

// HealthCheck SPARQL ping（Fuseki 就绪标志：/$/ping 200 或 query 端点可应答）。
func (r *Runtime) HealthCheck(ctx context.Context, endpoint string) error {
	ping := strings.Replace(endpoint, "/ds/", "/$/ping", 1)
	tr := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", ping, nil)
	if err != nil {
		return err
	}
	if resp, err := tr.Do(req); err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
	}
	// ping 未就绪则探 query 端点
	req2, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader("SELECT * WHERE {} LIMIT 1"))
	if err != nil {
		return err
	}
	req2.Header.Set("Content-Type", "application/sparql-query")
	resp, err := tr.Do(req2)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("SPARQL 端点异常: %s", resp.Status)
	}
	return nil
}

// extractRulesFile 从 fuseki-server.jar 解出 etc/owl-fb.rules 到 destDir，返回绝对路径。
// Jena 6.x 内置 urn:x-jena:* 规则 URL 处理器已移除，故显式解压后以 file: URL 引用。
// JDK 自带 jar 工具（<javaHome>/bin/jar）；找不到时回退 PATH 上的 jar。
func extractRulesFile(java, fusekiDir, destDir string) (string, error) {
	jar := filepath.Join(fusekiDir, "fuseki-server.jar")
	dst := filepath.Join(destDir, "owl-fb.rules")
	if _, err := os.Stat(dst); err == nil {
		return dst, nil // 已解出（同 profile 重启复用）
	}
	jarTool := filepath.Join(javaHome(java), "bin", "jar")
	if _, err := os.Stat(jarTool); err != nil {
		jarTool = "jar"
	}
	jarCmd := exec.Command(jarTool, "xf", jar, "etc/owl-fb.rules")
	jarCmd.Dir = destDir
	if out, err := jarCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("解压 OWL 推理规则文件失败: %v: %s", err, tail(out, 200))
	}
	src := filepath.Join(destDir, "etc", "owl-fb.rules")
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("规则文件未解出: %s 不存在", src)
	}
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// javaHome 从 java 可执行路径推导 JAVA_HOME（<home>/bin/java → <home>）；PATH 上的 java 则留空。
func javaHome(javaBin string) string {
	if abs, err := filepath.Abs(javaBin); err == nil && filepath.Base(abs) == "java" {
		return filepath.Dir(filepath.Dir(abs))
	}
	return ""
}

func sanitizeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if r == '_' || r == '-' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func tail(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		s = s[len(s)-n:]
	}
	return s
}

func drainLog(path string, r io.Reader) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = io.Copy(f, r)
}
