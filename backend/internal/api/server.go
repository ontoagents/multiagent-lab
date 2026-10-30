// Package api 提供主平台 REST API 与 SSE。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/a2a"
	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/companion"
	comedata "github.com/ontoagents/ontoagents/backend/internal/companion/evaldata"
	"github.com/ontoagents/ontoagents/backend/internal/kb"
	"github.com/ontoagents/ontoagents/backend/internal/kg"
	"github.com/ontoagents/ontoagents/backend/internal/ontobuild"
	"github.com/ontoagents/ontoagents/backend/internal/ontology"
	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
	"github.com/ontoagents/ontoagents/backend/internal/tool"
)

// Server 聚合依赖并持有路由。
type Server struct {
	Store *store.Store
	Box   *secrets.Box
	Chat  *chat.Service
	// Delegates REQ-208/M42 余项：可继续子 Agent 后台委派管理器（nil=未启用）
	Delegates     *chat.DelegateManager
	A2A           *a2a.Service // REQ-369：A2A 协议端点服务
	Tools         *tool.Registry
	KB            *kb.Service
	Ontology      *ontology.Service  // M8：本体对接（反代/facade 探测）
	OntoBuild     *ontobuild.Service // O13：由知识库构建本体（KB→spec 编排）
	FilesRoot     string             // M11：项目文件根目录（上传/下载落盘）
	DBPath        string             // REQ-113：SQLite 文件路径（数据与安全概览展示 DB 体积）
	DocsRoot      string             // REQ-140：内部方案文档根目录（只读查看）
	KnowledgeRoot string             // REQ-161：platform-knowledge/ 平台知识根目录（只读查看，2026-09-25 扩展）
	Companion     *companion.Service // REQ-170/M28：伴生本体旁路管线（Run/Resume 收尾触发，低侵入）
	sched         *scheduler         // REQ-204/M39 C5：对话级定时续跑（REQ-224 起调度状态落库，重启 LoadSchedules 续装）
	RuntimeEnv    *RuntimeEnv        // REQ-191/M31：运行环境统一配置（DB 覆盖 env，动态沙箱后端解析）
	Mux           *http.ServeMux
	mcpMu         sync.Mutex   // REQ-131/M18：/mcp 工具表缓存锁
	mcpHTTP       http.Handler // REQ-131/M18：Streamable HTTP handler（mcp_serve 变更后重建）
}

// NewServer 构造并注册全部路由。
func NewServer(st *store.Store, box *secrets.Box, chatSvc *chat.Service, tools *tool.Registry, kbSvc *kb.Service, onto *ontology.Service, dbPath, docsRoot, knowledgeRoot string) *Server {
	comp := companion.NewService(st, box, nil)
	chatSvc.Companion = comp // REQ-170 P2「KG 检索源并入」：伴生图检索源经接口反转注入 chat（companion→chat 包环约束）
	// REQ-358/M158 B+C（EvoOntology 借鉴旁路面）：①伴生图按需语义检索工具（默认关——92 号
	// §5.2，Agent 显式勾选生效）；②自动入图基准门控（事前评估旁挂，COMPANION_BENCHMARK_GATE=1
	// 显式开启；默认 nil=现行为零变化——任意域无冻结验证集时门控无信号，诚实边界见 evaldata）
	// ①工具注册（检索实现经闭包适配破 tool→companion 环，REQ-355 R2 先例）
	semSearch := func(ctx context.Context, ontID, query string, topK int) ([]tool.CompanionSearchHit, error) {
		hits, err := comp.SemanticSearch(ctx, ontID, query, topK)
		if err != nil {
			return nil, err
		}
		out := make([]tool.CompanionSearchHit, 0, len(hits))
		for _, h := range hits {
			out = append(out, tool.CompanionSearchHit{Label: h.Label, Kind: h.Kind, Definition: h.Definition, Confidence: h.Confidence, Score: h.Score, Neighbors: h.Neighbors})
		}
		return out, nil
	}
	if err := tool.RegisterCompanionSearchTool(tools, tool.CompanionSearchDeps{Store: st, SemanticSearch: semSearch}); err != nil {
		log.Printf("[companion] REQ-358 companion_search 注册失败（不影响启动）: %v", err)
	}
	// ②基准门控（COMPANION_BENCHMARK_GATE=1 显式开启；冻结问题向量缓存于门控闭包）
	if os.Getenv("COMPANION_BENCHMARK_GATE") == "1" {
		emb := &kb.Embedder{Store: st, Box: box}
		comp.AutoIngestGate = comedata.RecallFloorGate(comedata.GateDeps{
			LabelsOf: comp.LabelsFor,
			Embed:    emb.EmbedTexts,
			EmbedOne: emb.EmbedOne,
		})
		log.Printf("[companion] REQ-358 基准门控已开启（τ=%.2f，COMPANION_GATE_TAU 可调）", comedata.GateTau())
	}
	// REQ-216/M47：存量迁移幂等一次性——①旧 bool 绑定回填（创建空本体）→ ②旧 :9199 agent 图
	// 跨实例复制到宿主方案引擎本体伴生子图 → ③conv 图收敛兜底（companion_meta 标记防重复；
	// 构建平面/引擎不在位时日志告警、下次启动重试，不阻塞启动主链路）
	if err := comp.MigrateAgentGraphsToOntology(context.Background()); err != nil {
		log.Printf("[companion] REQ-216 迁移未完成（不影响启动）: %v", err)
	}
	chatSvc.Community = &kg.Summarizer{Store: st, Box: box, GenFuncs: ChatGenFuncs(st, box), Embed: kgEmbedFunc(st, box)} // KB-18/M290：社区检索 embedding 臂注入 // KB-5③：全局问答社区摘要源（connID 按库经接口参数传入；REQ-355 R2 生成注入）
	ontoBuildSvc := ontobuild.NewService(st, box, kbSvc)
	ontoBuildSvc.BuildPlaneURL = onto.BuildURL                 // REQ-267/M76：统一质量快评与反代同源（BUILD_SVC_URL）
	ontoBuildSvc.OnStage = buildflowTracePoster(onto.BuildURL) // REQ-340 A2：kb-extract 段留痕聚合到构建平面（fire-and-forget）
	s := &Server{Store: st, Box: box, Chat: chatSvc, Tools: tools, KB: kbSvc, Ontology: onto, OntoBuild: ontoBuildSvc, DBPath: dbPath, DocsRoot: docsRoot, KnowledgeRoot: knowledgeRoot, Companion: comp, Mux: http.NewServeMux()}
	// REQ-369/M167：A2A 端点服务（运行执行器=chat.Run+伴生收尾+最终助手消息读取——与 SSE 路径
	// 同一 runner 管线；伴生沉淀/命中追踪对 A2A 委派同样生效）
	s.A2A = a2a.New(st)
	s.A2A.RunFunc = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		res, rerr := chatSvc.Run(ctx, conv, agent, runID, input, 0, false, nil)
		if rerr != nil {
			return "", rerr
		}
		comp.OnRunComplete(conv, agent, runID)
		comp.RecordHits(conv.ID, runID)
		if res != nil {
			chatSvc.RecordOntologyCited(conv.ID, runID, res.AssistantMessageID) // REQ-530/M337：cited 归因（委派链同权）
		}
		if res.Error != "" {
			return "", errors.New(res.Error)
		}
		if res.Stopped {
			// REQ-520/M327：用户打断/抢占 → 哨兵错误——后台委派链（DelegateManager.runLoop）
			// 据此归类 stopped（原 (text,nil) 形态使打断被误标 completed、冷恢复失效）
			return "", chat.ErrRunStopped
		}
		msgs, merr := st.ListMessages(conv.ID)
		if merr == nil {
			for i := len(msgs) - 1; i >= 0; i-- {
				if msgs[i].ID == res.AssistantMessageID {
					return msgs[i].Content, nil
				}
			}
		}
		return "", nil
	}
	s.A2A.Stop = chatSvc.Stop
	s.A2A.Busy = chatSvc.IsRunning // REQ-441/M237：同 contextId 并发 message/send 单运行闸
	// REQ-443/M239：A2A 答案往返执行器——同 contextId 携答案再 send（会话挂起 ask_human）时
	// 走 chat.Resume 以答案续跑（跨协议人机交互闭环的续跑半边）
	s.A2A.ResumeFunc = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		res, rerr := chatSvc.Resume(ctx, conv, agent, runID, input, 0, false, nil)
		if rerr != nil {
			return "", rerr
		}
		comp.OnRunComplete(conv, agent, runID)
		comp.RecordHits(conv.ID, runID)
		if res.Error != "" {
			return "", errors.New(res.Error)
		}
		msgs, merr := st.ListMessages(conv.ID)
		if merr == nil {
			for i := len(msgs) - 1; i >= 0; i-- {
				if msgs[i].ID == res.AssistantMessageID {
					return msgs[i].Content, nil
				}
			}
		}
		return "", nil
	}
	// REQ-442/M238：A2A message/stream 流式执行器——chat.Run emit 的 message.delta 增量累积为
	// 对外过程快照（与 SSE 对话流同源同管线；首末快照由 a2a 层负责）
	s.A2A.RunStreamFunc = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string, onProgress func(string)) (string, error) {
		var acc strings.Builder
		res, rerr := chatSvc.Run(ctx, conv, agent, runID, input, 0, false, func(ev *chat.Event) {
			if ev == nil || ev.Type != "message.delta" || onProgress == nil {
				return
			}
			var d struct {
				Delta string `json:"delta"`
			}
			if json.Unmarshal(ev.Data, &d) == nil && d.Delta != "" {
				acc.WriteString(d.Delta)
				onProgress(acc.String())
			}
		})
		if rerr != nil {
			return "", rerr
		}
		comp.OnRunComplete(conv, agent, runID)
		comp.RecordHits(conv.ID, runID)
		if res.Error != "" {
			return "", errors.New(res.Error)
		}
		msgs, merr := st.ListMessages(conv.ID)
		if merr == nil {
			for i := len(msgs) - 1; i >= 0; i-- {
				if msgs[i].ID == res.AssistantMessageID {
					return msgs[i].Content, nil
				}
			}
		}
		return "", nil
	}
	// REQ-208/M42 余项：可继续子 Agent 后台委派管理器（运行执行器与 A2A RunFunc 同管线；
	// 装配器经 chatSvc.Assembler.Delegates 拿到同实例——delegate_* 工具族挂 agent_as_tool 模式）
	s.Delegates = chat.NewDelegateManager(st)
	s.Delegates.StartRun = func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string) (string, error) {
		text, rerr := s.A2A.RunFunc(ctx, conv, agent, runID, input)
		return text, rerr
	}
	s.Delegates.StopRun = chatSvc.Stop
	chatSvc.Assembler.Delegates = s.Delegates
	s.Delegates.LoadOrphans()
	// REQ-387/M183：A2A 任务启动对账——悬挂 working 标 failed（重启中断诚实报错重新委派）
	if n, err := st.MarkOrphanA2ATasks(); err == nil && n > 0 {
		log.Printf("[a2a] 启动对账：%d 条悬挂任务标 failed（重启中断，可重新委派）", n)
	}
	// REQ-445/M241：a2a_task 保留窗口——30 天前终态行启动清理
	if n, err := st.CleanupOldA2ATasks(time.Now().AddDate(0, 0, -30)); err == nil && n > 0 {
		log.Printf("[a2a] 保留窗口清理：%d 条 30 天前任务行删除", n)
	}
	s.sched = newScheduler(s)
	s.sched.load() // REQ-224/M52：重启按 DB 活跃行重新装配定时器
	s.routes()
	return s
}

func (s *Server) routes() {
	m := s.Mux
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.healthz(r.Context()))
	})

	// Agents
	// REQ-131/M18：Agent 对外 MCP 服务化——/mcp Streamable HTTP + 管理端点
	m.Handle("/mcp", s.mcpAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 懒重建：mcp_serve 变更后 invalidateMCP() 生效，工具表按最新 enabled Agent 组装
		s.mcpHandler().ServeHTTP(w, r)
	})))
	m.HandleFunc("GET /api/agents/{id}/mcp-serve", s.getAgentMcpServe)
	m.HandleFunc("POST /api/agents/{id}/mcp-serve/reset", s.resetAgentMcpToken)
	// REQ-231⑤⑥（51 号 W3）：装配预览与 hooks 清单（Harness 页签数据面）
	m.HandleFunc("GET /api/agents/{id}/tool-preview", s.agentToolPreview)
	m.HandleFunc("GET /api/hooks", s.listHooks)

	m.HandleFunc("GET /api/agents", s.listAgents)
	// M10 §6.3：沙箱配置下发（内部端点，一次性 token）
	m.HandleFunc("GET /api/internal/agents/{id}/manifest", s.getManifest)
	// M11 §6.13：项目文件（上传/列表/下载）
	m.HandleFunc("GET /api/projects/{id}/files", s.listProjectFiles)
	m.HandleFunc("POST /api/projects/{id}/files", s.uploadProjectFile)
	m.HandleFunc("GET /api/projects/{id}/files/{fid}/content", s.downloadProjectFile)
	m.HandleFunc("POST /api/projects/{id}/graph-state/sync", s.graphStateSync)   // REQ-389/M185：运行状态→伴生图投影（实验）
	m.HandleFunc("POST /api/projects/{id}/graph-state/query", s.graphStateQuery) // REQ-389/M185：SPARQL 查运行状态（实验）
	// M12 REQ-101/102：项目绑定本地目录 + 目录文件视图
	m.HandleFunc("POST /api/projects/validate-dir", s.validateProjectDir)
	// REQ-133：同机部署系统目录选择对话框（远程部署由前端降级手输）
	m.HandleFunc("POST /api/projects/pick-dir", s.pickProjectDir)
	m.HandleFunc("GET /api/projects/{id}/dir-files", s.listDirFiles)
	m.HandleFunc("GET /api/projects/{id}/dir-file", s.getDirFile)
	// M12 REQ-102 深度版：Git 视图（提交历史 / 分支 / 变更明细）
	m.HandleFunc("GET /api/projects/{id}/git-log", s.gitProjectLog)
	m.HandleFunc("GET /api/projects/{id}/git-branches", s.gitProjectBranches)
	m.HandleFunc("GET /api/projects/{id}/git-commit-files", s.gitProjectCommitFiles)
	m.HandleFunc("GET /api/projects/{id}/git-commit-patch", s.gitProjectCommitPatch)
	m.HandleFunc("GET /api/projects/{id}/git-working", s.gitProjectWorking)
	m.HandleFunc("POST /api/agents", s.createAgent)
	// REQ-370/M168：多 Agent 协作本体工程流水线一键创建（幂等——同名智能体/项目复用）
	m.HandleFunc("POST /api/ontology/collab-pipeline", s.createOntologyPipeline)
	// REQ-224 H10/60 号搭车件：本体消费贡献度量（onto_* 工具命中次数/被引用概念 TopN；
	// 精确路由压过 /api/ontology/ 反代前缀，沿 collab-pipeline 先例）
	m.HandleFunc("GET /api/ontology/{id}/contribution", s.ontologyContribution)
	m.HandleFunc("GET /api/ontology/contribution/summary", s.ontologyContributionSummary)
	m.HandleFunc("GET /api/ontology/contribution/run-groups", s.ontologyRunGroups) // REQ-482/M282④ 组维度

	m.HandleFunc("GET /api/agents/{id}", s.getAgent)
	m.HandleFunc("PUT /api/agents/{id}", s.updateAgent)
	m.HandleFunc("DELETE /api/agents/{id}", s.deleteAgent)
	// REQ-226/M54：配置治理（版本/回滚）
	m.HandleFunc("GET /api/agents/{id}/config-versions", s.listAgentConfigVersions)
	// REQ-311 B2/M115：Harness 审计跨会话聚合（审批挂起/决议、hook 拦截、verify 背压）
	m.HandleFunc("GET /api/agents/{id}/harness-audit", s.agentHarnessAudit)
	m.HandleFunc("GET /api/agents/{id}/config-versions/{version}/diff", s.agentConfigDiff)
	m.HandleFunc("POST /api/agents/{id}/config-versions/{version}/rollback", s.rollbackAgentConfig)

	// REQ-170/M28：伴生本体（候选确认流 API 先行；绑定经 REQ-216 companion_ontology_id）
	m.HandleFunc("GET /api/companion/candidates", s.listCompanionCandidates)
	m.HandleFunc("POST /api/companion/candidates/{id}/confirm", s.confirmCompanionCandidate)
	m.HandleFunc("POST /api/companion/candidates/{id}/reject", s.rejectCompanionCandidate)
	m.HandleFunc("GET /api/companion/status", s.companionStatus)
	m.HandleFunc("GET /api/companion/graph", s.companionGraph)
	m.HandleFunc("POST /api/companion/agents/{id}/bind", s.bindCompanionAgent)
	m.HandleFunc("POST /api/companion/agents/{id}/reset", s.resetCompanionAgent)
	m.HandleFunc("POST /api/companion/agents/{id}/reseed", s.reseedCompanionAgent)
	m.HandleFunc("GET /api/companion/ontologies/{id}/candidates", s.ontologyCompanionCandidates)
	m.HandleFunc("GET /api/companion/ontologies/{id}/agents", s.ontologyCompanionAgents)
	m.HandleFunc("POST /api/companion/ontologies/{id}/reset", s.resetCompanionOntology)
	m.HandleFunc("GET /api/companion/ontologies/{id}/export-ttl", s.exportCompanionTTL) // REQ-285④：伴生子图 TTL 导出
	m.HandleFunc("GET /api/companion/bound-ontologies", s.companionBoundOntologies)
	m.HandleFunc("GET /api/companion/graph-owner", s.companionGraphOwner)
	// REQ-288/M93：伴生社区摘要（Louvain+LLM 摘要复用 KG REQ-130 形态；检索零命中兜底注入臂）
	m.HandleFunc("GET /api/companion/communities", s.companionCommunities)
	m.HandleFunc("POST /api/companion/communities/rebuild", s.companionCommunitiesRebuild)
	m.HandleFunc("POST /api/companion/communities/summarize", s.companionCommunitySummarize)
	// REQ-286/M91：伴生图质量三维度（关系挖掘/实体编辑/合并/关系增删）
	m.HandleFunc("POST /api/companion/ontologies/{id}/mine-relations", s.mineCompanionRelations)
	m.HandleFunc("GET /api/companion/mine-jobs/{id}", s.getMineJob)
	m.HandleFunc("POST /api/companion/ontologies/{id}/entities/edit", s.editCompanionEntity)
	m.HandleFunc("POST /api/companion/ontologies/{id}/entities/delete", s.deleteCompanionEntity)
	m.HandleFunc("POST /api/companion/ontologies/{id}/entities/merge", s.mergeCompanionEntity)
	m.HandleFunc("POST /api/companion/ontologies/{id}/relations/add", s.addCompanionRelation)
	m.HandleFunc("POST /api/companion/ontologies/{id}/relations/delete", s.deleteCompanionRelation)
	// REQ-299/M103：伴生图检查清理（诊断只读+有限自动清理）
	m.HandleFunc("POST /api/companion/ontologies/{id}/diagnose", s.diagnoseCompanionGraph)
	m.HandleFunc("POST /api/companion/ontologies/{id}/cleanup", s.cleanupCompanionGraph)
	m.HandleFunc("GET /api/companion/prompts", s.companionPrompts) // REQ-303 C2：提示词只读可见

	// Projects
	m.HandleFunc("GET /api/projects", s.listProjects)
	m.HandleFunc("POST /api/projects", s.createProject)
	m.HandleFunc("GET /api/projects/{id}", s.getProject)
	m.HandleFunc("PUT /api/projects/{id}", s.updateProject)
	m.HandleFunc("PUT /api/projects/{id}/agents", s.setProjectAgents)
	m.HandleFunc("DELETE /api/projects/{id}", s.deleteProject)

	// Conversations
	m.HandleFunc("GET /api/conversations", s.listConversations)
	m.HandleFunc("POST /api/conversations", s.createConversation)
	m.HandleFunc("GET /api/conversations/{id}", s.getConversation)
	m.HandleFunc("PUT /api/conversations/{id}", s.updateConversation)
	m.HandleFunc("DELETE /api/conversations/{id}", s.deleteConversation)
	m.HandleFunc("GET /api/conversations/{id}/messages", s.listMessages)
	m.HandleFunc("GET /api/conversations/{id}/messages/paged", s.listMessagesPaged) // REQ-413/M209 长会话分页护栏
	m.HandleFunc("GET /api/diagnostics/export", s.exportDiagnostics)                // REQ-410/M206 诊断自助包
	m.HandleFunc("GET /api/backup/export", s.exportBackup)                          // REQ-411/M207 一键备份 zip
	m.HandleFunc("GET /api/conversations/{id}/events", s.listEvents)
	m.HandleFunc("POST /api/conversations/{id}/fork", s.forkConversation) // REQ-478③：派生会话（配置+消息拷贝）
	m.HandleFunc("POST /api/conversations/{id}/runs", s.runConversation)
	m.HandleFunc("POST /api/conversations/{id}/stop", s.stopConversation)
	m.HandleFunc("POST /api/conversations/{id}/steer", s.steerConversation) // REQ-385/M181：运行中介入（TurnLoop 抢占式 steer）
	m.HandleFunc("GET /api/conversations/{id}/run-events", s.runEvents)    // REQ-527/M334：事件回放（run_id 过滤+offset 增量轮询续看）
	m.HandleFunc("GET /api/search/messages", s.searchMessages)             // REQ-528/M335：会话内容检索（FTS5——跨会话按消息正文定位）
	m.HandleFunc("POST /api/companion/ontologies/{id}/promote", s.promoteCompanion)            // REQ-529/M336：伴生图→正式本体晋升
	m.HandleFunc("POST /api/kb/{id}/import-project-file", s.importProjectFileToKB)             // REQ-529/M336：项目产物一键入库
	m.HandleFunc("GET /api/conversations/{id}/export", s.exportConversation) // REQ-113①：对话导出 Markdown
	m.HandleFunc("GET /api/docs/read", s.docRead)
	m.HandleFunc("GET /api/assistant/config", s.assistantConfigGet) // M27/REQ-166
	m.HandleFunc("PUT /api/assistant/config", s.assistantConfigPut)
	m.HandleFunc("GET /api/assistant/proposal", s.assistantProposalGet)                   // M-O14 阶段三：L1 提案两段式（查看）
	m.HandleFunc("POST /api/assistant/proposal/{id}/apply", s.assistantProposalApply)     // 确认应用
	m.HandleFunc("POST /api/assistant/proposal/{id}/discard", s.assistantProposalDiscard) // 忽略
	m.HandleFunc("POST /api/assistant/optimize", s.assistantOptimize)                     // M27/REQ-167                                  // REQ-140：内部方案文档只读查看
	m.HandleFunc("POST /api/conversations/{id}/auto-name", s.autoNameConversation)        // REQ-136：对话自动命名
	// M11 收尾：中断恢复（ask_human 答复定向续跑）
	m.HandleFunc("POST /api/conversations/{id}/resume", s.resumeConversation)
	// REQ-224①：会话状态机读模型 + 变更留痕（状态族统一收口治理面）
	m.HandleFunc("GET /api/conversations/{id}/state", s.conversationState)
	m.HandleFunc("GET /api/conversations/{id}/state-log", s.conversationStateLog)
	// REQ-372 P2：知识体系 L2 图谱层（graph/path 查询+refresh 摄取——卡为事实源 KG 为投影）
	m.HandleFunc("GET /api/knowledge/graph", s.knowledgeGraph)
	m.HandleFunc("GET /api/knowledge/graph/path", s.knowledgeGraphPath)
	m.HandleFunc("POST /api/knowledge/graph/refresh", s.knowledgeGraphRefresh)
	// REQ-372 P3/M265：L3 服务层——llms.txt 聚合视图（请求时生成，与落库 llms.md 同源）
	m.HandleFunc("GET /api/knowledge/llms.txt", s.knowledgeLlmsTxt)
	// REQ-208/M42 余项：可继续子 Agent——人工侧委派查看/插话/打断/恢复（模型侧走 delegate_* 工具族）
	m.HandleFunc("GET /api/conversations/{id}/delegates", s.listDelegates)
	m.HandleFunc("GET /api/delegates/{id}", s.getDelegate)
	m.HandleFunc("POST /api/delegates/{id}/steer", s.steerDelegate)
	m.HandleFunc("POST /api/delegates/{id}/interrupt", s.interruptDelegate)
	m.HandleFunc("POST /api/delegates/{id}/continue", s.continueDelegate)
	// REQ-206/M41 S2：外部推理后端（deepseek-harness-acp）权限转审批人工决议
	m.HandleFunc("POST /api/conversations/{id}/ext-approval", s.extApproval)

	// Model connections
	m.HandleFunc("GET /api/model-connections", s.listConnections)
	m.HandleFunc("POST /api/model-connections", s.createConnection)
	m.HandleFunc("GET /api/model-connections/{id}", s.getConnection)
	m.HandleFunc("PUT /api/model-connections/{id}", s.updateConnection)
	m.HandleFunc("DELETE /api/model-connections/{id}", s.deleteConnection)
	m.HandleFunc("PUT /api/model-connections/{id}/default", s.setDefaultConnection)
	m.HandleFunc("POST /api/model-connections/test", s.testConnection)
	m.HandleFunc("POST /api/model-connections/{id}/list-models", s.listConnectionModels)
	// M23①（REQ-113 互补）：配置面导入/导出（四域 JSON；key 脱敏）
	m.HandleFunc("GET /api/settings/export", s.exportSettings)
	m.HandleFunc("POST /api/settings/import", s.importSettings)
	// M10/10b：沙箱生命周期（状态/启动/停止，per Agent）
	m.HandleFunc("GET /api/agents/{id}/sandbox", s.sandboxStatus)
	// REQ-218④/M49：智能体文件视图（安全根=agent.work_dir，沿项目 dir-files 先例）
	m.HandleFunc("GET /api/agents/{id}/dir-files", s.listAgentDirFiles)
	m.HandleFunc("GET /api/agents/{id}/dir-file", s.getAgentDirFile)
	// REQ-336：对话窗口附加文件上传（落智能体文件根；未设置 work_dir 时落默认 agent-files 目录）
	m.HandleFunc("POST /api/agents/{id}/files", s.uploadAgentDirFile)
	m.HandleFunc("POST /api/agents/{id}/sandbox/start", s.sandboxStart)
	m.HandleFunc("POST /api/agents/{id}/sandbox/stop", s.sandboxStop)
	// REQ-191/M31：运行环境统一配置（设置页「运行环境」分区；本体引擎执行方式仍走 /api/runtime-config 反代）
	m.HandleFunc("GET /api/runtime-env", s.runtimeEnvGet)
	m.HandleFunc("PUT /api/runtime-env", s.runtimeEnvPut)
	m.HandleFunc("POST /api/runtime-env/test", s.runtimeEnvTest)
	// REQ-214/M46：外部连接器（设置页「连接器」分区 + 侧板授权勾选数据源；凭据加密落库永不回传）
	m.HandleFunc("GET /api/connectors", s.listConnectors)
	m.HandleFunc("POST /api/connectors", s.createConnector)
	m.HandleFunc("PUT /api/connectors/{id}", s.updateConnector)
	m.HandleFunc("DELETE /api/connectors/{id}", s.deleteConnector)
	m.HandleFunc("POST /api/connectors/{id}/test", s.testConnector)
	m.HandleFunc("POST /api/connectors/preview", s.previewConnector) // REQ-214 P2：创建前预检（不落库）
	// REQ-148 供应商分组：同一供应商可多实例（分组标识与 BaseURL 解耦）+ 别名（展示层）
	m.HandleFunc("GET /api/provider-groups", s.listProviderGroups)
	m.HandleFunc("POST /api/provider-groups", s.createProviderGroup)
	m.HandleFunc("PUT /api/provider-groups/{id}", s.updateProviderGroup)
	// M13/D-O13 §6.16：推理后端探测清单
	m.HandleFunc("GET /api/inference-backends", s.listInferenceBackends)
	m.HandleFunc("POST /api/inference-backends/reprobe", s.reprobeInferenceBackends)

	// 使用统计（按 model|agent|project 聚合 run_event）
	m.HandleFunc("GET /api/stats/storage", s.storageOverview) // REQ-113②：数据量概览
	m.HandleFunc("GET /api/stats/usage", s.usageStats)
	// REQ-204/M39：进度产物人类侧导出 + 对话级定时续跑（进程内调度器，重启失效）
	m.HandleFunc("GET /api/conversations/{id}/todo.md", s.exportTodo)
	m.HandleFunc("POST /api/conversations/{id}/schedule", s.createSchedule)
	m.HandleFunc("DELETE /api/conversations/{id}/schedule", s.deleteSchedule)
	m.HandleFunc("GET /api/schedules", s.listSchedules)

	// 工具注册表（REQ-24 工具勾选）
	m.HandleFunc("GET /api/tools", s.listTools)
	// REQ-369/M167：A2A 端点暴露（发现面/Agent Card/JSON-RPC；鉴权沿全局 accessAuth）
	m.HandleFunc("GET /api/a2a/agents", s.A2A.AgentsExposed)
	m.HandleFunc("GET /api/a2a/agents/{id}/.well-known/agent-card.json", func(w http.ResponseWriter, r *http.Request) {
		s.A2A.HandleCard(w, r, r.PathValue("id"))
	})
	m.HandleFunc("POST /api/a2a/agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.A2A.HandleRPC(w, r, r.PathValue("id"))
	})
	// REQ-418/M214：协议面可视化（设置页「协议」总览数据源）
	m.HandleFunc("GET /api/a2a/cards", s.listA2aCards)
	m.HandleFunc("GET /api/a2a/tasks", s.listA2aTasks)

	// 技能库（M7，§6.12）
	// Knowledge base（M6，§450）
	m.HandleFunc("GET /api/kb", s.listKB)
	m.HandleFunc("POST /api/kb", s.createKB)
	m.HandleFunc("GET /api/kb/{id}", s.getKB)
	m.HandleFunc("PUT /api/kb/{id}", s.updateKB)
	m.HandleFunc("DELETE /api/kb/{id}", s.deleteKB)
	m.HandleFunc("GET /api/kb/{id}/docs", s.listKBDocs)
	m.HandleFunc("POST /api/kb/{id}/docs", s.importKBDoc)
	m.HandleFunc("DELETE /api/kb/{id}/docs/{did}", s.deleteKBDoc)
	m.HandleFunc("POST /api/kb/{id}/docs/{did}/reindex", s.reindexKBDoc)
	m.HandleFunc("GET /api/kb/{id}/docs/{did}/content", s.kbDocContent) // REQ-523/M330：溯源最后一跳——chunk 按 seq 拼接原文

	// 出题·批改·掌握度（REQ-524/M331——Q-23 衍生③ 转正交付；精确路由压过 /api/ontologies* 反代前缀）
	m.HandleFunc("POST /api/quiz/{id}/generate", s.quizGenerate)
	m.HandleFunc("POST /api/quiz/{id}/items/{qid}/submit", s.quizSubmit)
	m.HandleFunc("GET /api/quiz/{id}/mastery", s.quizMastery)
	m.HandleFunc("GET /api/quiz/{id}/items", s.quizHistory)
	m.HandleFunc("POST /api/kb/{id}/search-preview", s.previewKBSearch)
	m.HandleFunc("POST /api/kb/{id}/graphrag-search", s.graphragSearchKB) // M14 D-KB4：GraphRAG 子模块直查
	m.HandleFunc("GET /api/kb/{id}/wiki/pages", s.wikiPages)              // REQ-241：wiki 页面视图
	m.HandleFunc("POST /api/kb/{id}/wiki/rebuild", s.wikiRebuild)         // REQ-241：手动全量重建

	// O13 由知识库构建本体（D-O14/REQ-108，M15）：精确路由压过 /api/ontologies* 反代前缀
	m.HandleFunc("GET /api/kbs/selectable-for-ontology-build", s.selectableForOntologyBuild)
	m.HandleFunc("POST /api/ontologies/build-from-kb", s.buildFromKB)
	m.HandleFunc("POST /api/ontologies/build-from-structured", s.buildFromStructured)
	m.HandleFunc("POST /api/ontologies/kg-to-spec-json", s.kgToSpecJSON)
	// REQ-267/M76：ai-draft-async 的轮询通道反代（此前异步端点无前端调用方亦无反代路由，前端 GET 会 404）
	m.Handle("GET /api/ai-draft-jobs/{id}", s.Ontology.BuildProxy())

	// KG 自存 + 消费/审计（D-O15/REQ-110：去-semantica 化，零外部进程）
	m.HandleFunc("GET /api/kg/{kbID}", s.kgRead)
	m.HandleFunc("POST /api/kg/{kbID}/rebuild", s.kgRebuild)
	// M16 阶段一（REQ-127）：图谱浏览与统计
	m.HandleFunc("GET /api/kg/{kbID}/stats", s.kgStats)
	m.HandleFunc("GET /api/kg/{kbID}/entities", s.kgEntitySearch)
	m.HandleFunc("GET /api/kg/{kbID}/neighborhood", s.kgNeighborhood)
	// M16 阶段二（REQ-129）：抽取治理与人工反馈
	m.HandleFunc("POST /api/kg/{kbID}/review", s.kgReview)
	m.HandleFunc("POST /api/kg/{kbID}/merge", s.kgMerge)
	m.HandleFunc("GET /api/kg/{kbID}/quality", s.kgQuality)
	m.HandleFunc("GET /api/kg/{kbID}/merge-suggestions", s.kgMergeSuggestions)
	m.HandleFunc("POST /api/kg/{kbID}/merge/auto", s.kgMergeAuto)   // KB-19/M291：高置信自动合并（带撤销快照）
	m.HandleFunc("POST /api/kg/{kbID}/merge/undo", s.kgMergeUndo)   // KB-19/M291：一键撤销（快照重放）
	m.HandleFunc("PUT /api/kg/{kbID}/entity-alias", s.kgEntityAlias)                // M36/KB-7②：别名人工标注
	m.HandleFunc("POST /api/kg/{kbID}/communities/rebuild", s.kgCommunitiesRebuild) // M16/REQ-130：社区重建
	m.HandleFunc("GET /api/kg/{kbID}/communities", s.kgCommunities)
	m.HandleFunc("POST /api/kb/{id}/global-search", s.kgGlobalSearch) // REQ-130：全局问答
	m.HandleFunc("GET /api/audit/decisions", s.listDecisions)
	m.HandleFunc("POST /api/audit/decisions", s.createDecision)
	m.HandleFunc("GET /api/audit/decisions/{id}/chain", s.decisionChain)
	m.HandleFunc("GET /api/audit/prov-export", s.provExport)

	m.HandleFunc("GET /api/skills", s.listSkills)
	m.HandleFunc("POST /api/skills", s.createSkill)
	m.HandleFunc("POST /api/skills/import", s.importSkills) // REQ-371：目录式技能包批量导入（zip）
	m.HandleFunc("GET /api/skills/{id}", s.getSkill)
	m.HandleFunc("PUT /api/skills/{id}", s.updateSkill)
	m.HandleFunc("GET /api/skills/{id}/preview", s.previewSkill)
	m.HandleFunc("DELETE /api/skills/{id}", s.deleteSkill)

	// 本体对接（M8 §6.10）：模型能力代理 + 双反向代理（同源透传免跨域）
	m.HandleFunc("POST /api/ontology-llm/generate", s.generateOntologyLLM)
	m.HandleFunc("POST /api/ontology-llm/embed", s.embedOntologyLLM) // REQ-343 OE-1：本体元素向量检索代理
	if s.Ontology != nil {
		m.Handle("/api/ontologies", s.Ontology.BuildProxy()) // → 构建平面 BUILD_SVC_URL(:8091)
		m.Handle("/api/ontologies/", s.Ontology.BuildProxy())
		// REQ-216 增量②b：本体删除伴生拦截（精确路由压过反代前缀——绑定者 409 保护+删除后快照清理）
		m.HandleFunc("DELETE /api/ontologies/{id}", s.deleteOntologyGuard)
		// REQ-316/M118：权威本体库来源代理（只读拉取公网 TTL/RDF → 前端走既有导入管线）
		m.HandleFunc("GET /api/ontology-catalog/fetch", s.catalogFetch)
		// REQ-233②/M60：本体「被引用」三源聚合（运行方案挂载/KB 词表/伴生绑定，删除确认预检同源）
		m.HandleFunc("GET /api/ontologies/{id}/references", s.ontologyReferences)
		// REQ-250/52 号 E6 余项：本体治理审计——spec 保存/导入/合并/fork/CSV 灌装落 onto_decision
		// （subject_kind=ontology；精确路由压过反代前缀，转发成功后异步记事件；删除审计在 deleteOntologyGuard 内）
		m.HandleFunc("PUT /api/ontologies/{id}/spec", s.auditOntologyProxy)
		m.HandleFunc("POST /api/ontologies/import", s.auditOntologyProxy)
		m.HandleFunc("POST /api/ontologies/{id}/merge/apply", s.auditOntologyProxy)
		m.HandleFunc("POST /api/ontologies/{id}/fork", s.auditOntologyProxy)
		m.HandleFunc("POST /api/ontologies/{id}/ingest-csv", s.auditOntologyProxy)
		// REQ-292/M96 对象级 overlay（编辑三层）：人工编辑/撤回经审计代理落 onto_decision（overlay.edit/revert）
		m.HandleFunc("PUT /api/ontologies/{id}/objects/{kind}/{name}/overlay", s.auditOntologyProxy)
		m.HandleFunc("DELETE /api/ontologies/{id}/objects/{kind}/{name}/overlay", s.auditOntologyProxy)
		// REQ-103 模式 A：OntoChat 会话/turn/save 全在构建平面 /api/ontochat/*（bugfix：此前漏注册反代，
		// 同源请求命中主后端 404 文本，前端 JSON.parse 报 "Unexpected non-whitespace character after JSON"）
		m.Handle("/api/ontochat", s.Ontology.BuildProxy())
		m.Handle("/api/ontochat/", s.Ontology.BuildProxy())
		// REQ-171/156/157/M-O15：质量门禁（quality check/report）+ 工具链（toolchain 五工具）+
		// LOV 词表搜索 + 导入合并（merge preview/apply）+ 质量门禁开关（quality-config）——构建平面新端点前缀
		m.Handle("/api/ontology/", s.Ontology.BuildProxy())
		// M-O17 重构补遗（工具链配置页 404 修复）：PipelinePane 走 /api/pipelines*（REST 工具链配置，:8091）——
		// REQ-75/76 交付时页面挂本体模块内但平台反代从未覆盖该前缀
		m.Handle("/api/pipelines", s.Ontology.BuildProxy())
		m.Handle("/api/pipelines/", s.Ontology.BuildProxy())
		m.Handle("/api/runtime-profiles", s.Ontology.RuntimeProxy()) // → 运行平面 RUNTIME_MGR_URL(:8090)
		m.Handle("/api/runtime-profiles/", s.Ontology.RuntimeProxy())
		// REQ-179/M-O16：全局运行配置（执行方式）——运行平面单行配置表
		m.Handle("/api/runtime-config", s.Ontology.RuntimeProxy())
		// REQ-146：引擎自检与一键安装（install 为 202 异步任务，轮询 /api/engines 无长连接）
		m.Handle("/api/engines", s.Ontology.RuntimeProxy())
		m.Handle("/api/engines/", s.Ontology.RuntimeProxy())
		// D-O15：/api/semantica* 与 /semantica/explorer* 反代已随「去-semantica 化」移除，
		// 消费/审计改走上方自研 /api/kg、/api/audit 端点（§4.9 反转注记）
	}
}

// ---- JSON 工具 ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[api] encode response: %v", err)
	}
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return &store.HTTPError{Status: http.StatusBadRequest, Msg: "invalid JSON body: " + err.Error()}
	}
	return nil
}

func writeErr(w http.ResponseWriter, err error) {
	var he *store.HTTPError
	if errors.As(err, &he) {
		writeJSON(w, he.Status, map[string]string{"error": he.Msg})
		return
	}
	// REQ-521/M328 并发运行闸：同会话已有进行中运行 → 409 人话提示（与 A2A REQ-441 Busy 同构）
	if errors.Is(err, chat.ErrConversationBusy) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "该会话已有进行中的运行——请等待完成或先停止，再发送新消息/恢复"})
		return
	}
	var mnc *chat.ModelNotConfiguredError
	if errors.As(err, &mnc) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": mnc.Error()})
		return
	}
	// KB-20/M292：embedding 维度漂移 → 400 人话（含重建指引——原 500 对「换模型后检索失败」不可解释）
	var dme kb.DimMismatchError
	if errors.As(err, &dme) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": dme.Error()})
		return
	}
	// REQ-413③/M209：SQLite 写冲突诚实提示——busy_timeout 5s 兜底后仍锁（长写事务如
	// 备份 VACUUM/批量导入）→ 503 人话+重试指引（原样 500 "database is locked" 对用户不可解释）
	if isSQLiteBusy(err) {
		log.Printf("[api] sqlite busy: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "数据库写入冲突（另一操作正在写库）——请稍后重试；若反复出现，请避开备份/批量导入窗口（REQ-413）",
		})
		return
	}
	log.Printf("[api] internal error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

// isSQLiteBusy SQLITE_BUSY/locked 判定（错误串匹配——驱动侧 SQLITE_BUSY 文案两种形态）。
func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "SQLITE_BUSY")
}

// healthz 汇总后端依赖状态（§6.10/§8：模型默认连接、本体 facade、知识库向量后端可达性）。
func (s *Server) healthz(ctx context.Context) map[string]any {
	out := map[string]any{"status": "ok"}

	if def, err := s.Store.GetDefaultConnection("chat"); err != nil {
		out["model_default_conn"] = "error"
	} else if def == nil {
		out["model_default_conn"] = "not_configured"
	} else {
		out["model_default_conn"] = "ok"
	}
	if defE, err := s.Store.GetDefaultConnection("embedding"); err != nil {
		out["embedding_default_conn"] = "error"
	} else if defE == nil {
		out["embedding_default_conn"] = "not_configured"
	} else {
		out["embedding_default_conn"] = "ok"
	}

	if s.KB == nil {
		out["knowledge_backend"] = "disabled"
	} else {
		out["knowledge_backend"] = s.KB.Healthz(ctx)
	}

	switch {
	case s.Ontology == nil:
		out["ontology_facade"] = "disabled"
	case s.Ontology.Reachable(ctx):
		out["ontology_facade"] = "ok"
	default:
		out["ontology_facade"] = "unreachable"
	}
	return out
}
