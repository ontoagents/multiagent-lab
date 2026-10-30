// Package manager 运行方案编排（方案 04 §4.3）：
// 生命周期（start/stop/reload）、形态分发（从构建平面拉取）、健康检查、日志查询、降级状态维护。
package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ontoagents/ontoagents/runtime-manager/internal/engine"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/engine/fuseki"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/engine/oxigraph"
	"github.com/ontoagents/ontoagents/runtime-manager/internal/store"
)

type Manager struct {
	Store         *store.Store
	Engines       map[string]engine.Runtime // engine 名 → 适配器（oxigraph/fuseki；O6 起多引擎）
	BuildURL      string                    // 构建平面地址（拉取本体形态）
	LogDir        string
	InstallBinDir string // 一键安装落点目录（REQ-146，默认 data/bin）
	// SnapshotDir REQ-377/M173：伴生子图快照目录（默认 data/companion-graph-snapshots；
	// Stop/Reload 前快照、Start 后回放——伴生数据与引擎数据目录解耦存活）
	SnapshotDir string
	HTTP        *http.Client
	HealthTries int // 启动健康检查重试次数

	mu    sync.Mutex
	procs map[string]*engine.Process // profileID → 运行句柄（进程内态，重启 Manager 后按 stopped 处理）

	installMu  sync.Mutex
	installing bool   // 一键安装进行中（REQ-146；REQ-471 起经 installingName 归属到具体引擎）
	installErr string // 最近一次安装失败原因
	// REQ-471：安装任务与结果的引擎归属 + 下载进度（同 installMu 门控；单任务串行）
	installingName       string  // 正在安装的引擎名（安装结束后清空）
	installErrEngine     string  // installErr 归属引擎（与 installErr 同写）
	installProgress      float64 // 当前安装下载进度 0~100（下载推进回调写入）
	installProgressKnown bool    // 总量已知才置位（Content-Length 缺失时进度不透出）
}

func New(st *store.Store, buildURL, logDir, installBinDir string) *Manager {
	_ = os.MkdirAll(logDir, 0o755)
	_ = os.MkdirAll(installBinDir, 0o755)
	return &Manager{
		Store: st, Engines: map[string]engine.Runtime{}, BuildURL: strings.TrimRight(buildURL, "/"), LogDir: logDir,
		InstallBinDir: installBinDir,
		HTTP:          &http.Client{Timeout: 30 * time.Second},
		HealthTries:   20,
		procs:         map[string]*engine.Process{},
	}
}

// RegisterEngine 注册引擎适配器（main 启动时按环境探测注册）。
func (m *Manager) RegisterEngine(name string, eng engine.Runtime) {
	m.Engines[name] = eng
}

// engineFor 按 profile.engine 取适配器；未注册给出可自助的提示。
func (m *Manager) engineFor(name string) (engine.Runtime, error) {
	if eng, ok := m.Engines[name]; ok {
		return eng, nil
	}
	return nil, fmt.Errorf("引擎 %q 未注册: %s", name, engineHint(name))
}

func engineHint(name string) string {
	switch name {
	case "fuseki":
		return "可在本体运行页一键下载安装（官方发行包写入 data/bin 即时生效；运行需 JDK 17+），或从 https://jena.apache.org/download/ 下载 apache-jena-fuseki 后设置 FUSEKI_BIN 指向 fuseki-server 脚本（windows 为 fuseki-server.bat）"
	case "oxigraph":
		return "请在本体运行页一键安装（写入 data/bin 即时生效），或安装 oxigraph_server 并加入 PATH（或设置 OXIGRAPH_BIN）后重启 runtimed"
	}
	return "请检查 runtimed 启动配置（对应引擎二进制未就绪或未注册）"
}

// knownEngines 引擎状态汇总的固定顺序（未注册的也呈现，REQ-146）。
var knownEngines = []string{"oxigraph", "fuseki"}

// EngineStatuses 引擎自检汇总（REQ-146）：oxigraph/fuseki 全量呈现（未注册=不可用 + 指引），
// 附加一键安装任务态。
func (m *Manager) EngineStatuses() []engine.EngineStatus {
	m.installMu.Lock()
	active, lastErr := m.installing, m.installErr
	activeName, errEngine := m.installingName, m.installErrEngine
	prog, progKnown := m.installProgress, m.installProgressKnown
	m.installMu.Unlock()
	out := make([]engine.EngineStatus, 0, len(knownEngines))
	for _, name := range knownEngines {
		var st engine.EngineStatus
		if eng, ok := m.Engines[name]; ok {
			if p, ok := eng.(engine.StatusProbe); ok {
				st = p.Probe()
			} else {
				st = engine.EngineStatus{Engine: name, Registered: true}
			}
		} else {
			st = engine.EngineStatus{Engine: name, Registered: false, Hint: engineHint(name)}
		}
		if name == "oxigraph" || name == "fuseki" {
			st.Installable = true
			// REQ-471：安装中/失败原因按引擎归属（activeName 空为历史兜底形态，双引擎呈现）
			st.Installing = active && (activeName == "" || activeName == name)
			if active && name == activeName && progKnown {
				p := prog
				st.InstallProgress = &p
			}
			if lastErr != "" && (errEngine == "" || errEngine == name) {
				st.LastInstallError = lastErr
			}
		}
		out = append(out, st)
	}
	return out
}

// StartInstall 异步发起一键安装（REQ-146，仅 oxigraph）：进行中返回错误；
// 结果经 EngineStatuses 的 Installing/LastInstallError/Installed 轮询呈现。
func (m *Manager) StartInstall(name string) error {
	if name != "oxigraph" && name != "fuseki" {
		return fmt.Errorf("引擎 %q 暂不支持一键安装", name)
	}
	m.installMu.Lock()
	if m.installing {
		m.installMu.Unlock()
		return fmt.Errorf("安装任务进行中，请稍候")
	}
	m.installing = true
	m.installErr = ""
	m.installingName = name
	m.installProgress, m.installProgressKnown = 0, false
	m.installMu.Unlock()
	go func() {
		// REQ-307：fuseki 发行包 ~220MB（oxigraph ~20MB），超时按引擎区分（30min/10min）
		budget := 10 * time.Minute
		if name == "fuseki" {
			budget = 30 * time.Minute
		}
		ctx, cancel := context.WithTimeout(context.Background(), budget)
		defer cancel()
		var err error
		if name == "fuseki" {
			err = m.installFuseki(ctx)
		} else {
			err = m.installOxigraph(ctx)
		}
		m.installMu.Lock()
		m.installing = false
		m.installingName = ""
		m.installProgress, m.installProgressKnown = 0, false
		if err != nil {
			m.installErr = err.Error()
			m.installErrEngine = name
		}
		m.installMu.Unlock()
	}()
	return nil
}

// setInstallProgress 更新当前安装的下载进度（REQ-471；0~100 截断，经 /api/engines 轮询透出）。
func (m *Manager) setInstallProgress(pct float64) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	m.installMu.Lock()
	m.installProgress, m.installProgressKnown = pct, true
	m.installMu.Unlock()
}

// installFuseki 一键安装（REQ-307）：官方发行包（pin fuseki.PinnedVersion，downloads→archive 双源）
// 下载后提取 fuseki-server 脚本与 fuseki-server.jar 两件到 data/bin——fuseki 适配器候选序动态解析
// 即时命中，无需重启 runtimed。java 为运行时依赖（启动时预检），安装不要求 JDK。
func (m *Manager) installFuseki(ctx context.Context) error {
	return fuseki.Install(ctx, m.InstallBinDir, fuseki.ReleaseURLs(), m.downloadToWithProgress)
}

// downloadToWithProgress downloadTo 的 fuseki.DownloadFunc 形态（REQ-471）：下载推进回调
// 直写 Manager 安装进度（fuseki.Install 内部对进度无感，恒传 nil）。
func (m *Manager) downloadToWithProgress(ctx context.Context, url, target string, minBytes int64, _ func(loaded, total int64)) error {
	return downloadTo(ctx, url, target, minBytes, m.onDownloadProgress)
}

// onDownloadProgress 下载推进 → 安装进度 0~100（REQ-471）。
func (m *Manager) onDownloadProgress(loaded, total int64) {
	if total > 0 {
		m.setInstallProgress(float64(loaded) / float64(total) * 100)
	}
}

// InstallFromFile 安装使用方自行下载的发行包（REQ-307 手动路径，开发者定案「文件太大不用预下载，
// 给出下载方式由使用方自己下载」）：仅 fuseki——上传 apache-jena-fuseki tar.gz / zip（REQ-325：
// windows 用户从 jena.apache.org 自然下到 zip），服务端只做解包落位（浏览器下载自带断点续传，
// 慢网络用户彻底绕开服务端下载瓶颈）；装后即时生效无需重启 runtimed。
func (m *Manager) InstallFromFile(name string, r *http.Request) ([]string, error) {
	if name != "fuseki" {
		return nil, fmt.Errorf("引擎 %q 暂不支持上传文件安装（当前仅 fuseki）", name)
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, fmt.Errorf("multipart 解析失败: %w", err)
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("缺少 file 字段: %w", err)
	}
	defer f.Close()
	lname := strings.ToLower(hdr.Filename)
	isZip := strings.HasSuffix(lname, ".zip")
	if !isZip && !strings.HasSuffix(lname, ".tar.gz") && !strings.HasSuffix(lname, ".tgz") {
		return nil, fmt.Errorf("请上传 apache-jena-fuseki 的 tar.gz 或 zip 发行包（收到 %s）", hdr.Filename)
	}
	ext := ".tar.gz"
	if isZip {
		ext = ".zip"
	}
	tmp, err := os.CreateTemp("", "fuseki-upload-*"+ext)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, f)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	if n < 1<<20 {
		return nil, fmt.Errorf("文件过小（%d 字节），不是有效的 apache-jena-fuseki 发行包", n)
	}
	extract := fuseki.ExtractServerFiles
	installed := []string{
		filepath.Join(m.InstallBinDir, "fuseki-server"),
		filepath.Join(m.InstallBinDir, "fuseki-server.jar"),
	}
	if isZip {
		extract = fuseki.ExtractServerFilesZip
		installed = []string{
			filepath.Join(m.InstallBinDir, "fuseki-server.bat"),
			filepath.Join(m.InstallBinDir, "fuseki-server.jar"),
		}
	}
	if err := extract(tmp.Name(), m.InstallBinDir); err != nil {
		return nil, err
	}
	return installed, nil
}

// installOxigraph 下载官方 release（GOOS/GOARCH 映射资产，pin PinnedVersion）流式落盘：
// temp + rename 原子替换，装后 oxigraph 适配器动态解析即时命中（Start 无需重启 runtimed）。
func (m *Manager) installOxigraph(ctx context.Context) error {
	asset, err := oxigraph.ReleaseAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	url := oxigraph.ReleaseBaseURL + "/" + asset
	target := filepath.Join(m.InstallBinDir, "oxigraph")
	if runtime.GOOS == "windows" {
		target += ".exe"
	}
	if err := downloadTo(ctx, url, target, 1<<20, m.onDownloadProgress); err != nil {
		return err
	}
	log.Printf("[manager] oxigraph 一键安装完成: %s (%s)", target, asset)
	return nil
}

// downloadTo 流式下载 url → target（temp+rename 原子替换，非 windows 补执行位）。
// minBytes 下限防截断/错误页落盘。独立成函数便于对下载机制做零网络依赖单测（httptest）。
// REQ-307：支持 HTTP Range 断点续传——tmp 半包保留（中断不删），重试/换源时从已收字节续传
// （206 append；服务器不支持 Range 回 200 则全量重写）——本机实测外网长连接 ~1 分钟即断，
// 无续传时大包（fuseki ~220MB）永远下不完。
// onProgress 可空——下载推进回调（loaded/total 字节；Content-Length 缺失不回调；REQ-471）。
func downloadTo(ctx context.Context, url, target string, minBytes int64, onProgress func(loaded, total int64)) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("创建安装目录失败: %w", err)
	}
	tmp := target + ".download"
	var resumeFrom int64
	if fi, err := os.Stat(tmp); err == nil && fi.Size() > 0 {
		resumeFrom = fi.Size()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if resumeFrom > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeFrom))
	}
	resp, err := installHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败（%s）: %w", url, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusPartialContent: // 206 续传
	case http.StatusOK:
		// 200 全量：无半包时是正常下载；**有半包说明该源不支持 Range——报错换源并保留半包**
		//（重写会清掉已收字节，六源候选中有支持 Range 的镜像，换源续传优于从头再来）
		if resumeFrom > 0 {
			resp.Body.Close()
			return fmt.Errorf("源不支持断点续传（200 全量响应，半包 %d 字节已保留，换源重试）", resumeFrom)
		}
	default:
		return fmt.Errorf("下载失败: %s（%s）", resp.Status, url)
	}
	body := io.Reader(resp.Body)
	if onProgress != nil && resp.ContentLength > 0 {
		// REQ-471：总量已知才报进度；续传态把已收字节计入基数（206 的 Content-Length 是剩余量）
		body = &progressReader{r: resp.Body, loaded: resumeFrom, total: resumeFrom + resp.ContentLength, onProgress: onProgress}
	}
	flag := os.O_CREATE | os.O_WRONLY
	if resumeFrom > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(tmp, flag, 0o644)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	total := resumeFrom + n
	if err != nil {
		// 半包保留供断点续传（REQ-307）；仅"内容异常"（非网络中断）时清空重来
		return fmt.Errorf("下载中断（本次 %d 字节，累计 %d）: %w", n, total, err)
	}
	if total < minBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("下载内容异常（仅 %d 字节，预期更大）: %s", total, url)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmp, 0o755); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

var installHTTP = &http.Client{Timeout: 10 * time.Minute}

// progressReader 下载进度计数（REQ-471）：每个读取块回调 loaded/total（total<=0 不回调）。
type progressReader struct {
	r          io.Reader
	loaded     int64
	total      int64
	onProgress func(loaded, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 && p.total > 0 && p.onProgress != nil {
		p.loaded += int64(n)
		p.onProgress(p.loaded, p.total)
	}
	return n, err
}

// FetchTTL 从构建平面拉取本体 TTL 形态（original turtle 直接回原文；自建经 spec→sidecar 导出）。
func (m *Manager) FetchTTL(ontologyID string) (string, error) {
	ttl, _, err := m.FetchTTLWithCount(ontologyID)
	return ttl, err
}

// FetchTTLWithCount 同 FetchTTL，另回响应头 X-Onto-Triples（REQ-293①：构建平面 sidecar 精确计数；
// 头缺失（original turtle / 旧构建平面）返回 -1=未知，调用方跳过计数硬闸并如实标注 gate=skipped）。
func (m *Manager) FetchTTLWithCount(ontologyID string) (string, int, error) {
	resp, err := m.HTTP.Get(fmt.Sprintf("%s/api/ontologies/%s/export?format=turtle", m.BuildURL, ontologyID))
	if err != nil {
		return "", -1, fmt.Errorf("构建平面不可达: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return "", -1, fmt.Errorf("拉取本体 %s 失败: %s %s", ontologyID, resp.Status, string(b))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return "", -1, err
	}
	n := -1
	if h := resp.Header.Get("X-Onto-Triples"); h != "" {
		if v, perr := strconv.Atoi(h); perr == nil {
			n = v
		}
	}
	return string(b), n, nil
}

// countTriples 引擎全图三元组计数（REQ-293① 计数硬闸：装载后 SELECT COUNT 对账）。
// 两引擎（oxigraph /query、fuseki /ds/）均接受 POST SPARQL 到 proc.Endpoint。
func countTriples(ctx context.Context, endpoint string) (int, error) {
	return runCount(ctx, endpoint, "SELECT (COUNT(*) AS ?c) WHERE { ?s ?p ?o }")
}

// countGraphTriples 命名图三元组计数（REQ-320②：per-graph 装载后逐本体逐图对账，不再数
// 整个引擎默认图——多本体同方案下无 GRAPH 查询=各图并集，单体计数恒失配的口径修正）。
func countGraphTriples(ctx context.Context, endpoint, graph string) (int, error) {
	return runCount(ctx, endpoint, fmt.Sprintf("SELECT (COUNT(*) AS ?c) WHERE { GRAPH <%s> { ?s ?p ?o } }", graph))
}

func runCount(ctx context.Context, endpoint, query string) (int, error) {
	tr := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(query))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/sparql-query")
	req.Header.Set("Accept", "application/sparql-results+json")
	resp, err := tr.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("计数查询失败: %s", resp.Status)
	}
	var out struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return 0, err
	}
	if len(out.Results.Bindings) == 0 {
		return 0, fmt.Errorf("计数查询无结果")
	}
	return strconv.Atoi(out.Results.Bindings[0]["c"].Value)
}

// FetchVersion 拉取本体当前版本号（构建平面 meta；REQ-155 阶段二 drift 检测用）。
func (m *Manager) FetchVersion(ontologyID string) (int, error) {
	resp, err := m.HTTP.Get(fmt.Sprintf("%s/api/ontologies/%s", m.BuildURL, ontologyID))
	if err != nil {
		return 0, fmt.Errorf("构建平面不可达: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var meta struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(b, &meta) != nil {
		return 0, nil // 解析失败不阻断启动（版本快照尽力而为）
	}
	return meta.Version, nil
}

// FetchMeta 构建平面本体元数据（REQ-239/M65）：GET /api/ontologies/{id} 的
// version/status/version_name 一次取回（status 为空=旧构建平面无发布态，返回 draft 兜底）。
func (m *Manager) FetchMeta(ontologyID string) (version int, status, versionName string, err error) {
	resp, gerr := m.HTTP.Get(fmt.Sprintf("%s/api/ontologies/%s", m.BuildURL, ontologyID))
	if gerr != nil {
		return 0, "", "", fmt.Errorf("构建平面不可达: %w", gerr)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var meta struct {
		Version     int    `json:"version"`
		Status      string `json:"status"`
		VersionName string `json:"version_name"`
	}
	if jerr := json.Unmarshal(b, &meta); jerr != nil {
		return 0, "", "", nil // 解析失败不阻断启动（元数据尽力而为）
	}
	if meta.Status == "" {
		meta.Status = "draft"
	}
	return meta.Version, meta.Status, meta.VersionName, nil
}

// FetchQuality 构建平面快评（REQ-234①/M61）：POST quality/check save=false 内存评分
// 零副作用；返回精简摘要。失败返回 nil（快照尽力而为，不阻断启动）。
func (m *Manager) FetchQuality(ontologyID string) map[string]any {
	body := fmt.Sprintf(`{"ontology_id":%q,"save":false}`, ontologyID)
	req, err := http.NewRequest(http.MethodPost, m.BuildURL+"/api/ontology/quality/check", strings.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req.WithContext(ctx))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var out struct {
		Report struct {
			ErrorCount   int `json:"error_count"`
			WarningCount int `json:"warning_count"`
			Score        struct {
				Overall float64 `json:"overall"`
			} `json:"score"`
		} `json:"report"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil {
		return nil
	}
	return map[string]any{
		"overall":       out.Report.Score.Overall,
		"error_count":   out.Report.ErrorCount,
		"warning_count": out.Report.WarningCount,
	}
}

// Start 启动方案：starting → 拉形态 → 引擎装载 → 健康检查 → running（失败进 error）。
func (m *Manager) Start(ctx context.Context, id string) error {
	p, err := m.Store.Get(id)
	if err != nil {
		return err
	}
	if p.Status == "running" {
		return fmt.Errorf("方案已在运行")
	}
	_ = m.Store.SetStatus(id, "starting", "", "")
	if len(p.OntologyIDs) == 0 {
		_ = m.Store.SetStatus(id, "error", "未配置本体集合", "")
		return fmt.Errorf("方案未配置本体集合")
	}
	// REQ-480/M280：组装载闭包扩展——沿仓内 external_refs（import/reuse）+owl:imports 登记
	// 递归补全挂载清单（visited set 收敛=互引环并集装载）；悬空引用阻断启动（装载链门禁）；
	// 远端 IRI 不拉取维持（D-O27）。载体=方案 OntologyIDs，闭包为装载期展开不回写声明集。
	cloRep, cerr := m.expandClosure(p.OntologyIDs)
	if cerr != nil {
		_ = m.Store.SetStatus(id, "error", cerr.Error(), "")
		return cerr
	}
	effective := cloRep.Effective
	ttls := map[string]string{}
	loadedVersions := map[string]int{}
	loadedStatus := map[string]map[string]any{} // REQ-239/M65 → REQ-293③ v2：{oid:{status,version_name,generation,triple_count,source_triple_count,gate,loaded_at}}
	sourceTriples := map[string]int{}           // REQ-293①：构建平面精确计数（-1=未知，跳过硬闸）
	for _, oid := range effective {
		ttl, srcCount, err := m.FetchTTLWithCount(oid)
		if err != nil {
			_ = m.Store.SetStatus(id, "error", err.Error(), "")
			return err
		}
		ttls[oid] = ttl
		sourceTriples[oid] = srcCount
		if v, st, vn, merr := m.FetchMeta(oid); merr == nil {
			loadedVersions[oid] = v // 尽力而为：版本快照失败不阻断启动
			loadedStatus[oid] = map[string]any{"status": st, "version_name": vn}
		}
	}
	port := p.Port
	if port == 0 {
		port = nextPort(m.Store)
		_ = m.Store.SetPort(id, port)
	}
	eng, err := m.engineFor(p.Engine)
	if err != nil {
		_ = m.Store.SetStatus(id, "error", err.Error(), "")
		return err
	}
	// REQ-320②：对账口径与装载方式联动——伴生方案/推理 fuseki 走默认图计数（legacy 语义），
	// regular per-graph 方案逐本体按独立命名图计数（不再数并集）。
	companionMode := companionProfileName(p.Name)
	reasoningGate := p.Engine == "fuseki" && profileReasoning(p.Config) // REQ-329：推理形态等值闸改下限闸（见 gate 处注释）
	perGraphGate := !companionMode && !reasoningGate
	// REQ-481/M281：装载期跨本体推理物化——oxigraph regular 方案对组装载全集并置物化
	//（owlrl OWL 2 RL 数据级闭包），推断三元组装独立 inferred 图（raw/inferred 双图隔离
	// provenance 可分；per-graph 计数硬闸只对 raw 图，等值口径不受影响）；fuseki InfModel
	// 查询期形态维持；物化失败/依赖缺失降级跳过（留装载日志，不阻断）。
	var extraGraphs map[string]string
	var matLines []string
	if !companionMode && p.Engine == "oxigraph" {
		gr, inferredTTL, _, line := m.materializeInferred(ttls)
		matLines = append(matLines, line)
		if gr != "" {
			extraGraphs = map[string]string{gr: inferredTTL}
		}
	}
	proc, err := m.startEngine(ctx, eng, p, port, ttls, extraGraphs)
	if err != nil {
		_ = m.Store.SetStatus(id, "error", err.Error(), "")
		return err
	}
	m.mu.Lock()
	m.procs[id] = proc
	m.mu.Unlock()
	_ = m.Store.SetStatus(id, "starting", "", fmt.Sprint(proc.PID))
	// REQ-480：装载报告呈现——闭包补全/互引环/远端跳过 + 装载前并置一致性检查（强告警不阻断）；
	// 引擎日志同文件追加（startEngine 成功后写入，防引擎截断）。
	m.appendProcLog(id, cloRep.logLines())
	if len(matLines) > 0 {
		m.appendProcLog(id, matLines)
	}
	if line := m.preloadReasonCheck(ttls); line != "" {
		m.appendProcLog(id, []string{line})
	}

	// 健康检查直至就绪
	var lastErr error
	for i := 0; i < m.HealthTries; i++ {
		if ctx.Err() != nil {
			break
		}
		lastErr = eng.HealthCheck(ctx, proc.Endpoint)
		if lastErr == nil {
			// REQ-293① 计数硬闸（REQ-320② 口径修正）：源三元组数（构建平面 X-Onto-Triples）
			// ≠ 引擎侧计数 → 中止启动，旧 loaded 快照保持不动（此前成功装载的 generation
			// 语义上仍「活跃」，Foundry 原子发布口径）。regular 方案按命名图逐本体对账。
			// REQ-480：闭包补全本体同闸同权（有效装载清单逐本体对账沿用）。
			for _, oid := range effective {
				src := sourceTriples[oid]
				if src < 0 {
					if _, ok := loadedStatus[oid]; !ok {
						loadedStatus[oid] = map[string]any{}
					}
					loadedStatus[oid]["gate"] = "skipped" // 计数未知（original turtle/REQ-318 拼接还原）——诚实标注跳过
					continue
				}
				var got int
				var cerr error
				graphLabel := "默认图"
				if perGraphGate {
					graphLabel = engine.GraphIRI(oid)
					got, cerr = countGraphTriples(ctx, proc.Endpoint, graphLabel)
				} else {
					got, cerr = countTriples(ctx, proc.Endpoint)
				}
				if cerr != nil {
					// 引擎不支持计数（罕见）——不阻断，标注 skipped
					if _, ok := loadedStatus[oid]; !ok {
						loadedStatus[oid] = map[string]any{}
					}
					loadedStatus[oid]["gate"] = "skipped"
					break
				}
				// REQ-329：推理形态（fuseki InfModel OWL_FB）在引擎侧物化推断三元组，默认图计数
				// 恒 ≥ 源计数（真机实锤 源1342/引擎3595），等值对账结构性失配。改下限闸：got < src
				// 仍失配中止（截断/部分装载的硬闸语义保留）；≥ 判 pass 并留痕推断增量。
				mismatch := got != src
				if mismatch && reasoningGate {
					mismatch = got < src
				}
				if mismatch {
					proc.Stop()
					m.mu.Lock()
					delete(m.procs, id)
					m.mu.Unlock()
					msg := fmt.Sprintf("装载对账失败（REQ-293①/REQ-320② 计数硬闸）：本体 %s 图 %s 源 %d 三元组 ≠ 引擎 %d——启动中止，旧装载快照保持", oid, graphLabel, src, got)
					_ = m.Store.SetStatus(id, "error", msg, "")
					return fmt.Errorf("%s", msg)
				}
				if _, ok := loadedStatus[oid]; !ok {
					loadedStatus[oid] = map[string]any{}
				}
				loadedStatus[oid]["gate"] = "pass"
				if reasoningGate && got != src {
					loadedStatus[oid]["engine_triples"] = got // 推断物化后引擎侧实际计数，如实留痕
				}
			}
			// REQ-293③ 装载态升级 v2：行数一致性 + 装载 generation 号 + 装载时间
			cycleTags := cloRep.cycleTags() // REQ-480：互引环逐本体标注（报告呈现环）
			for _, oid := range effective {
				if _, ok := loadedStatus[oid]; !ok {
					loadedStatus[oid] = map[string]any{}
				}
				loadedStatus[oid]["generation"] = fmt.Sprintf("g%d-%d", loadedVersions[oid], time.Now().Unix())
				loadedStatus[oid]["loaded_at"] = time.Now().UTC().Format(time.RFC3339)
				if src, ok := sourceTriples[oid]; ok && src >= 0 {
					loadedStatus[oid]["source_triple_count"] = src
					loadedStatus[oid]["triple_count"] = src
				}
				if tag, ok := cycleTags[oid]; ok {
					loadedStatus[oid]["import_cycle"] = tag
				}
			}
			if b, jerr := json.Marshal(loadedVersions); jerr == nil {
				_ = m.Store.SetLoadedVersions(id, string(b)) // REQ-155 阶段二：加载版本快照（drift 检测）
			}
			if b, jerr := json.Marshal(loadedStatus); jerr == nil {
				_ = m.Store.SetLoadedStatus(id, string(b)) // REQ-239/M65：装载发布状态快照（draft 装载警示数据源）
			}
			// REQ-234①/M61：装载质量快照（低分警示不阻断——异步快评，失败静默跳过）
			go func(ids []string, pid string) {
				q := map[string]any{}
				for _, oid := range ids {
					if sum := m.FetchQuality(oid); sum != nil {
						q[oid] = sum
					}
				}
				if len(q) == 0 {
					return
				}
				if b, jerr := json.Marshal(q); jerr == nil {
					_ = m.Store.SetLoadedQuality(pid, string(b))
				}
			}(effective, id)
			// REQ-377/M173：伴生子图回放恢复（快照存在才动作；失败仅日志不阻断启动）
			m.restoreCompanionGraphs(id, p.OntologyIDs, proc.Endpoint)
			_ = m.Store.SetStatus(id, "running", "", "")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	proc.Stop()
	m.mu.Lock()
	delete(m.procs, id)
	m.mu.Unlock()
	_ = m.Store.SetStatus(id, "error", "健康检查失败: "+errStr(lastErr), "")
	return fmt.Errorf("方案启动后健康检查失败: %v", lastErr)
}

// Stop 停止方案。REQ-377/M173：停止前自动快照伴生子图（数据/代码分离——本体图可重建，
// 伴生沉淀不可重建；快照失败仅日志不阻断停止）。
func (m *Manager) Stop(id string) error {
	if _, err := m.Store.Get(id); err != nil {
		return err
	}
	m.mu.Lock()
	proc := m.procs[id]
	delete(m.procs, id)
	m.mu.Unlock()
	if proc != nil {
		if p, gerr := m.Store.Get(id); gerr == nil {
			m.snapshotCompanionGraphs(id, p.OntologyIDs, proc.Endpoint)
		}
		_ = proc.Stop()
	}
	return m.Store.SetStatus(id, "stopped", "", "")
}

// Reload 显式重载（REQ-87）：重建数据目录并重新拉取最新版本形态。
func (m *Manager) Reload(ctx context.Context, id string) error {
	if err := m.Stop(id); err != nil {
		return err
	}
	return m.Start(ctx, id)
}

// Logs 返回最近 tail 行。
func (m *Manager) Logs(id string, tail int) ([]string, error) {
	if _, err := m.Store.Get(id); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(m.LogDir, id+".log"))
	if err != nil {
		return []string{}, nil // 无日志（从未启动）
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if tail > 0 && len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return lines, nil
}

// ProcEndpoint 返回 running 方案的 SPARQL 端点（facade 路由用）。
func (m *Manager) ProcEndpoint(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.procs[id]; p != nil {
		return p.Endpoint, nil
	}
	return "", fmt.Errorf("方案 %s 未在运行", id)
}

// startEngine 按引擎能力分发启动（实现 ReasoningRuntime 的引擎透传推理开关，O6）。
func (m *Manager) startEngine(ctx context.Context, eng engine.Runtime, p *store.Profile, port int, ttls map[string]string, extraGraphs map[string]string) (*engine.Process, error) {
	// REQ-179/M-O16：执行方式为系统级全局配置——docker=容器执行（oxigraph 官方镜像）；
	// native=内置二进制子进程；k8s=接口预留。
	// REQ-236④/M63（D-O20 v0.73 变更，2026-10-01 开发者拍板）：默认执行方式改 docker，
	// **未安装 docker 时启动期降级进程内 native**（迁移 006 翻转存量默认值；此处为配置 docker
	// 但运行环境探测不可用的运行期兜底——配置值不写回，用户显式配置优先）。
	// REQ-320：per-graph 装载语义随方案透传（伴生形态保持 legacy 路径，regular 走独立命名图）。
	opts := engine.GraphIsolationOpts{
		CompanionMode: companionProfileName(p.Name),
		Reasoning:     profileReasoning(p.Config),
		ExtraGraphs:   extraGraphs, // REQ-481/M281：inferred 推断图（oxigraph per-graph 装载；fuseki 忽略）
	}
	switch m.Store.GetConfig().ExecutionMethod {
	case "docker":
		// REQ-379③/M175：fuseki docker 形态评估结论（详设「优先 docker 化，不可行则 per-profile
		// 降级 native 兜底」）——fuseki 无官方 Docker 镜像，社区镜像（secoresearch/fuseki）不含
		// 平台灌入链依赖的宿主 tdbloader 离线装载工具链，容器化适配未真机验证；本分支落
		// **降级 native 兜底**（治「默认 docker 配置 × fuseki=必失败」组合矛盾），docker 化
		// 随镜像适配专项轮（含 per-graph 装载容器内等价物评估）。
		if p.Engine == "fuseki" {
			log.Printf("[manager] 方案 %s：fuseki 引擎 docker 形态未验证（社区镜像缺平台灌入链工具链），per-profile 降级 native（REQ-379③）", p.ID)
			break
		}
		// docker 不可用的运行期降级对所有引擎生效（REQ-325 顺修：原实现降级 break 仅包住
		// oxigraph 分支，fuseki 等其他引擎在「默认 docker + 未装 docker」的 fresh 环境恒报错
		// ——D-O20 v0.73「未安装 docker 时启动期降级进程内 native」本意即全引擎兜底）
		if !oxigraph.DockerAvailable() {
			log.Printf("[manager] 方案 %s：执行方式配置为 docker 但 docker 不可用，降级进程内 native（REQ-236④）", p.ID)
			break
		}
		if ox, ok := eng.(*oxigraph.Runtime); ok {
			return ox.DockerStartOpts(ctx, p.ID, port, ttls, opts)
		}
		return nil, fmt.Errorf("执行方式 docker 下引擎 %s 不支持（REQ-379③：fuseki 已降级 native 兜底，此处为未知引擎防御）", p.Engine)
	case "k8s":
		return nil, fmt.Errorf("执行方式 k8s 为接口预留（复用 M10 10d K8sBackend 模式，随集群环境落地）——当前请在系统配置切换 docker 容器或内置二进制")
	}
	if gi, ok := eng.(engine.GraphIsolationRuntime); ok {
		return gi.StartWithGraphIsolation(ctx, p.ID, port, ttls, opts)
	}
	if rr, ok := eng.(engine.ReasoningRuntime); ok {
		return rr.StartWithReasoning(ctx, p.ID, port, ttls, profileReasoning(p.Config))
	}
	return eng.Start(ctx, p.ID, port, ttls)
}

// companionProfileName 伴生宿主方案判定（REQ-309 池「伴生·共享引擎」与存量「伴生·{本体名}」
// 自动命名契约）：此类方案保持 legacy 默认图装载与对账口径、不开 union——伴生读写走自描述
// GRAPH 块/GRAPH ont-{id}，池「子图互不可见」验收不可破（REQ-320 P1 边界）。
func companionProfileName(name string) bool { return strings.HasPrefix(name, "伴生·") }

// profileReasoning 从 profile config JSON 读 reasoning 开关（O6：fuseki 推理对照基座）。
func profileReasoning(cfgJSON string) bool {
	var cfg struct {
		Reasoning bool `json:"reasoning"`
	}
	if cfgJSON != "" {
		_ = json.Unmarshal([]byte(cfgJSON), &cfg)
	}
	return cfg.Reasoning
}

// nextPort 端口分配（REQ-236③/M63）：在 max+1 基线上逐个 bind 试探（127.0.0.1 回环探测，
// 占用即跳过），治「手工指定/残留进程占用同端口只能靠引擎 bind 失败进 error」——分配即确认可用。
func nextPort(st *store.Store) int {
	list, _ := st.List()
	max := 9200
	taken := map[int]bool{}
	for _, p := range list {
		if p.Port > max {
			max = p.Port
		}
		if p.Port > 0 {
			taken[p.Port] = true
		}
	}
	for cand := max + 1; cand < max+64; cand++ {
		if taken[cand] {
			continue
		}
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(cand)))
		if err != nil {
			continue // 占用（引擎残留/其他服务），跳过
		}
		_ = ln.Close()
		return cand
	}
	return max + 1 // 兜底（探测全部失败交由引擎启动报错，不静默吞）
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
