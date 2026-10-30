// 主平台 backend 入口：装配依赖并启动 HTTP 服务（默认 :8080）。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/api"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/chat"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/connector"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/inference"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/kb"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/kg"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/ontology"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/skill"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

func main() {
	addr := getenv("ADDR", ":8080")
	dbPath := getenv("DB_PATH", "./data/platform.db")
	keyFile := getenv("SECRET_KEY_FILE", "./data/.secret")

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	box, err := secrets.LoadKeyFile(keyFile)
	if err != nil {
		log.Fatalf("load secret key: %v", err)
	}

	// 工具注册表（M5，方案 §6.8）：内置工具启动时登记
	reg := tool.NewRegistry()
	if err := tool.RegisterBuiltin(reg); err != nil {
		log.Fatalf("register builtin tools: %v", err)
	}

	// REQ-426/M222：推理后端注册表先于装配器构造——Assembler 与 chat.Service 共用同一实例
	//（CLI 外部成员装配与外部推理执行同源，避免双注册表探测口径分叉）
	infReg := inference.NewRegistry() // M13/D-O13 §6.16：推理后端注册表（eino-adk + 外部 CLI）
	asm := &chat.Assembler{
		Store:       st,
		Box:         box,
		Tools:       reg,
		Composer:    &skill.Composer{Store: st},              // M9：技能注入
		Ontology:    ontology.NewService(),                   // M8：本体对接（facade/双反代/guide）
		FilesRoot:   getenv("FILES_ROOT", "./data/projects"), // M11：项目文件根目录
		CheckPoints: chat.NewStoreCheckPointStore(st),        // REQ-204/M39 C1：SQLite 持久化（重启后挂起中断可恢复）
		// REQ-336：智能体默认文件根（<DB 同级>/agent-files）——未设置 work_dir 的智能体文件工具
		// 安全根与对话附加文件落位回落于此（web=仓库 data/；客户端=data.location 数据目录内）
		AgentFilesRoot: filepath.Join(filepath.Dir(dbPath), "agent-files"),
		Inference:      infReg, // REQ-426/M222：CLI 外部成员装配（nil=不装配兼容旧构造路径）
	}
	// REQ-427/M223②：docker 可达探测（sandbox_mode=docker 的 CLI 成员三前置之沙箱执行世界）——
	// docker version 子进程探测（仅在装配含沙箱 CLI 成员的项目会话时触发，不做常驻缓存）
	asm.DockerProbe = func(ctx context.Context) bool {
		bin := getenv("DOCKER_BIN", "")
		if bin == "" {
			if p, err := exec.LookPath("docker"); err == nil {
				bin = p
			}
		}
		if bin == "" {
			return false
		}
		out, err := exec.CommandContext(ctx, bin, "version", "--format", "{{.Server.Version}}").Output()
		return err == nil && strings.TrimSpace(string(out)) != ""
	}
	// 知识库服务（M6，§6.9）：向量后端按 KB_VECTOR_BACKEND（qdrant|sqlite），Qdrant 走 REST（QDRANT_URL）
	kbSvc, err := kb.NewService(st, box,
		getenv("KB_VECTOR_BACKEND", "qdrant"),
		getenv("QDRANT_URL", "http://127.0.0.1:6333"))
	if err != nil {
		log.Fatalf("init knowledge base service: %v", err)
	}
	// D-O15/REQ-110：KG 自研抽取器注入（REQ-98 LLM 能力代理主路径 + 规则抽取回退，零外部进程；
	// KG_LLM_CONN_ID 可选指定模型连接，缺省走默认 chat 连接）
	// M36/KB-6③：本体约束抽取词表注入（I3 裁定：构建平面 spec 只读投影；构建平面不可达时降级自由抽取）。
	ontoSvc := ontology.NewService()
	kbSvc.SetKGExtractor((&kg.Extractor{
		Store:    st,
		Box:      box,
		ConnID:   getenv("KG_LLM_CONN_ID", ""),
		GenFuncs: api.ChatGenFuncs(st, box), // REQ-355 R2：kg→chat 生成依赖反转（装配层闭包注入）
		OntoVocab: func(ctx context.Context, ontologyID string) (*kg.OntoVocab, error) {
			v, err := ontoSvc.FetchSpecVocab(ctx, ontologyID)
			if err != nil {
				return nil, err
			}
			return &kg.OntoVocab{Name: v.Name, Concepts: v.Concepts, Relations: v.Relations}, nil
		},
	}).ExtractForDoc)
	// REQ-241：wiki 页面生成器注入（chat.GenerateStructured 转接闭包——kb 包不 import chat 防环；
	// WIKI_LLM_CONN_ID 可选指定生成连接，缺省走默认 chat 连接）
	wikiConn := getenv("WIKI_LLM_CONN_ID", "")
	kbSvc.SetWikiLLM(func(ctx context.Context, connID, prompt, schemaJSON string) (string, error) {
		if connID == "" {
			connID = wikiConn
		}
		res, err := chat.GenerateStructured(ctx, st, box, connID, prompt, schemaJSON)
		if err != nil {
			return "", err
		}
		return string(res.DraftJSON), nil
	})

	kbSrc, kbAdmin := api.NewKBChatSource(kbSvc) // REQ-355 R1：kb→chat 接口反转装配（适配器，nil=未接入）
	// REQ-430/M226：外部成员上下文供给——KB 检索源注入装配器（经适配器闭包取渲染文本；
	// nil=KB 源不参与。项目文件清单/本体概念源在 Assemble 内按会话能力组装）
	if kbSrc != nil {
		asm.KBRetriever = func(ctx context.Context, cfg *store.KnowledgeBase, query string, topK int, minScore float64) (string, error) {
			text, _, _, _, err := kbSrc.RetrieveContext(ctx, cfg, query, topK, minScore)
			return text, err
		}
	}
	svc := chat.NewService(st, asm, kbSrc)
	// REQ-192/M32：平台助手配置单源归一引导——builtin 行 instruction 空时写入当前默认基座
	//（一次性移植 assistant_config 存量微调），此后 agent 内置行即配置单源、assistant_config 退役。
	// REQ-451：基座升级同步前置——行内 instruction 精确等于历史基座时覆写为当前基座（真实微调不动）。
	if err := st.SyncBuiltinAssistantPrompt(api.AssistantDefaultPrompt, api.AssistantPromptHistory); err != nil {
		log.Printf("[backend] sync builtin assistant prompt: %v", err)
	}
	if err := st.EnsureBuiltinAssistantInstruction(api.AssistantDefaultPrompt); err != nil {
		log.Printf("[backend] ensure builtin assistant instruction: %v", err)
	}
	// REQ-213：平台助手工具面归一引导——内置行 tools 幂等并入 L0+L1 基座八工具（迁移 030 存量修正的兜底自愈）。
	if err := st.EnsureAssistantTools(); err != nil {
		log.Printf("[backend] ensure assistant tools: %v", err)
	}
	svc.Inference = infReg // M13/D-O13 §6.16：推理后端注册表（eino-adk + 外部 CLI；REQ-426 起与 Assembler 共用）
	knowledgeRoot := getenv("KNOWLEDGE_ROOT", "../platform-knowledge")
	srv := api.NewServer(st, box, svc, reg, kbSvc, asm.Ontology, dbPath, getenv("DOCS_ROOT", "../docs"), knowledgeRoot)
	// REQ-186 阶段二：平台助手 L0 只读工具面（doc_read/doc_list/doc_grep/列表×3/查配置；写类工具不开放）
	if err := chat.RegisterAssistantTools(reg, chat.AssistantDeps{Store: st, DocsRoot: getenv("DOCS_ROOT", "../docs"), KnowledgeRoot: knowledgeRoot, DefaultPrompt: api.AssistantDefaultPrompt}); err != nil {
		log.Printf("[backend] assistant tools register: %v", err)
	}
	// REQ-186 阶段一/三（M-O14 流水线）：平台知识 KB 化工具（sync/search_platform_kb）与 L1 提案工具（propose_assistant_config 两段式）
	if err := chat.RegisterAssistantL1Tools(reg, chat.AssistantDeps{Store: st, KnowledgeRoot: knowledgeRoot}, kbAdmin); err != nil {
		log.Printf("[backend] assistant L1 tools register: %v", err)
	}
	// REQ-451：平台知识库启动自动同步（后台 goroutine）——原「对话内懒触发 sync_platform_kb」
	// 存在重启/新会话陈旧窗口；mtime 幂等对账，失败仅记日志（懒触发路径仍兜底）。
	go chat.SyncPlatformKBStartup(context.Background(), st, kbAdmin, knowledgeRoot)
	// M10 §6.3 + REQ-191/M31：沙箱执行后端动态装配——运行方式（进程内嵌/docker/k8s/auto）与
	// K8s 访问认证集中为「运行环境」配置（DB runtime_settings 覆盖启动期 env，env 为初始值
	// 兜底）；设置页修改后新 Run 生效（resolver 按配置重建后端，auto 探测缓存随之重置），
	// 运行中实例不受影响。inprocess/未配置镜像 = 未启用沙箱（与原 SANDBOX_IMAGE 空语义一致）。
	// 注：Defaults.SandboxMode 存 env SANDBOX_BACKEND 原值（docker|k8s|auto），空时由
	// api.RuntimeEnv.Build 按「SANDBOX_IMAGE 非空即启用，缺省 docker」语义判定——存量零回归。
	platformURL := getenv("PLATFORM_URL_EXTERNAL", "http://host.docker.internal"+addr)
	srv.RuntimeEnv = &api.RuntimeEnv{
		Store:      st,
		TokenIssue: srv.IssueManifestToken,
		Defaults: store.RuntimeSettings{
			SandboxMode:          getenv("SANDBOX_BACKEND", ""),
			SandboxImage:         getenv("SANDBOX_IMAGE", ""),
			SandboxScope:         getenv("SANDBOX_SCOPE", ""),
			DockerBin:            getenv("DOCKER_BIN", ""),
			KubectlBin:           getenv("KUBECTL_BIN", ""),
			K8sKubeconfig:        getenv("SANDBOX_K8S_KUBECONFIG", ""),
			K8sContext:           getenv("SANDBOX_K8S_CONTEXT", ""),
			K8sNamespace:         getenv("SANDBOX_K8S_NAMESPACE", ""),
			K8sEndpointMode:      getenv("SANDBOX_K8S_ENDPOINT_MODE", ""),
			PlatformURLInCluster: getenv("PLATFORM_URL_IN_CLUSTER", ""),
			PlatformURLExternal:  platformURL,
		},
	}
	svc.Runtime = &api.DynamicRuntime{Env: srv.RuntimeEnv}
	if img := srv.RuntimeEnv.Defaults.SandboxImage; img != "" {
		log.Printf("[backend] runtime env resolver enabled: env_mode=%s image=%s（运行方式可在设置页「运行环境」分区动态切换）", getenv("SANDBOX_BACKEND", "(按镜像自动判定)"), img)
	} else {
		log.Printf("[backend] runtime env resolver enabled: sandbox not configured（可在设置页「运行环境」分区启用）")
	}

	// REQ-214/M46：外部连接器启动引导——存量 mcp_servers 挂载幂等迁移为连接器实例+引用
	//（实例名沿用原 name，装配前缀槽位不变=工具名零破坏；mcp_servers 置空即完成）。
	if n, err := st.MigrateAgentMCPToConnectors(); err != nil {
		log.Printf("[backend] migrate agent mcp_servers to connectors: %v", err)
	} else if n > 0 {
		log.Printf("[backend] REQ-214: 已迁移 %d 个 agent 的存量 MCP 挂载为连接器引用", n)
	}
	// REQ-214/M46 阶段二：平台托管自研 Go MCP 插件服务（进程内嵌、loopback 独立端口）——
	// kubernetes/ssh 连接器经此走标准 MCP 客户端管线，凭据服务端绑定不进 LLM 上下文不进工具参数。
	pluginAddr := getenv("CONNECTOR_PLUGIN_ADDR", "127.0.0.1:8093")
	asm.PluginEndpoint = "http://" + pluginAddr
	plugin := connector.NewPlugin(st, box, srv.RuntimeEnv.Defaults.KubectlBin)
	pluginSrv := &http.Server{Addr: pluginAddr, Handler: plugin.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("[connector-plugin] listening on %s（kubernetes/ssh 连接器运行平面）", pluginAddr)
		if err := pluginSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[connector-plugin] 插件服务启动失败（k8s/ssh 连接器装配将降级告警）: %v", err)
		}
	}()

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           withStatic(cors(accessAuth(srv.Mux))), // accessAuth：REQ-209 远程访问凭证（PLATFORM_TOKEN 未设置时零开销透传）
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("[backend] listening on %s (db=%s)", addr, dbPath)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("[backend] shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	_ = pluginSrv.Shutdown(ctx)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// withStatic 若存在 ../web/dist 则托管前端产物（单进程运行整个平台）；SPA fallback 到 index.html。
func withStatic(next http.Handler) http.Handler {
	dist := getenv("WEB_DIST", "../web/dist")
	abs, err := filepath.Abs(dist)
	exists := err == nil
	if exists {
		if _, err := os.Stat(abs); err != nil {
			exists = false
		}
	}
	if !exists {
		return next
	}
	fs := http.FileServer(http.Dir(abs))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// REQ-131/M18：/mcp（Agent 对外 MCP 服务化）与 /api、/healthz 同属 API 面，绕过静态托管
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/healthz") || strings.HasPrefix(r.URL.Path, "/mcp") {
			next.ServeHTTP(w, r)
			return
		}
		p := filepath.Join(abs, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			// 入口页不缓存，避免前端更新后浏览器仍引用旧产物；assets 文件名带 hash 可缓存
			if strings.HasSuffix(r.URL.Path, "index.html") {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fs.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(abs, "index.html"))
	})
}

// cors REQ-534/M341：CORS 白名单化（本机前端 dev server:5173 + 同源）——替换 ACAO=* 全放行
// （任意网页跨站可读 API 响应）。技术边界如实注记：CORS 只阻跨站「读响应」，不阻「写执行」
// （no-cors 简单请求照发）——写侧闭环靠网络收敛（compose 127.0.0.1 绑定）或 PLATFORM_TOKEN。
// CORS_ALLOW_ORIGINS 可覆盖（逗号分隔；缺省本机 dev server 与同源）。
func cors(next http.Handler) http.Handler {
	allowed := map[string]bool{
		"http://localhost:5173": true,
		"http://127.0.0.1:5173": true,
	}
	if v := os.Getenv("CORS_ALLOW_ORIGINS"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				allowed[o] = true
			}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if o, err := url.Parse(origin); err == nil && o.Host == r.Host {
				allowed[origin] = true // 同源请求（反代形态）放行
			}
		}
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Platform-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
