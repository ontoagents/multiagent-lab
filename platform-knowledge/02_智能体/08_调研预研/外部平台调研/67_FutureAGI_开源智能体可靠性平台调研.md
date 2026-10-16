---
module: 智能体
topic: Future AGI（开源 AI Agent 可靠性全栈平台）调研
desc: 调研开源项目 future-agi/future-agi 的功能全景与实现原理——六支柱（Simulate/Evaluate/Protect/Monitor/Optimize + Agent Command Center 网关）、四层运行时（Client SDK → traceAI+Go 网关 → Django 平台 → PG/ClickHouse/Redis/Temporal）、Go 网关插件化流水线与路由/缓存/预算/护栏、自研 OTel 采集器 fi-collector 的 typed-Map 落库与背压、100 个评测类与 82 条系统评测目录、persona 驱动的模拟引擎与托管沙箱、平台内置智能体 Falcon AI 与 260 工具注册表、MCP 服务化（92 工具）；并逐条梳理对本项目智能体模块可借鉴的产品设计与技术实现（含不可照搬的边界）。
synced: 2026-10-10
---

# Future AGI 调研：开源 AI Agent 可靠性全栈平台的功能与实现原理

> **调研对象**：GitHub `future-agi/future-agi`（分支 `main`，**Apache-2.0 核心 + `ee/` 目录独立企业许可**）。
> **一手来源**：仓库 `README.md` / `INSTALLATION.md` / `TESTING.md` / `LICENSE-EE`、`agentcc-gateway/README.md`、`fi-collector/README.md`、`docs/error-feed-observability.md`、`docs/simulation-run-analytics.md`，以及**逐目录源码阅读**（`futureagi/` Django 平台、`agentcc-gateway/internal/` Go 网关、`fi-collector/pkg/` 采集器、`ai_tools/`、`ee/`）。凡源码未出现者一律标注"源码未见"。
> **与邻档分工**：本文聚焦"**围绕 Agent 的评测/可观测/护栏/网关这一整条可靠性链路**"，不重复 DeepSeek Harness 运行时（31/41 号）、Eino 框架（44 号）、LangSmith Fleet 产品形态（48 号）、OpenAI Dots 常驻智能体（49 号）。**§6 借鉴章节直接面向本项目智能体模块（五层视角 / 装配链 / run_event / 调用轨迹面板 / 34 号平台助手口径）**，其余章节保持通用。

---

## 1. 一句话结论

Future AGI 不是"再造一个 Agent 运行时"，而是**把 Agent 上线前后围绕它的可靠性链路（模拟→评测→防护→监控→优化）收成一体化平台 + 一条反馈回路**，并且**用 Go 写网关与采集器、用 Django 写平台、用 OTel 做统一接入**——它的价值主张是"**不需要再拼 Langfuse + Braintrust + Helicone + Guardrails AI + 自研模拟器**"，落点是"**Agent 如何被验、被管、被改**"，而不是"Agent 怎么被搭出来"。

这与 48 号 Fleet、49 号 Dots 的战略判断同向：**生产期的可观测与治理，才是这一代平台的竞争面**。

---

## 2. 项目定位与基本盘

| 项 | 事实 |
| --- | --- |
| 仓库 | `future-agi/future-agi`（组织下另有 `traceAI`、`agent-learning-kit`、`futureagi-sdk`、`agent-command-center-sdk` 等独立 SDK 仓） |
| 许可 | **Apache-2.0 核心 + 企业许可**：`LICENSE-EE` 明确"任何名为 `ee/` 的目录（如 `futureagi/ee/`、`frontend/ee/`）属 Enterprise Code，未经付费订阅不得用于生产"。**README 宣称"Apache 2.0 — no open core gotchas"与许可证实际并存，属事实性夸张**，引用时须以 `LICENSE-EE` 为准 |
| 规模/活跃 | 创建 2026-04-23，最近推送 2026-10-10，star ≈ 2.1k，仓库体积 ≈ 200 MB，569 commits 量级（发布节奏密集） |
| 主语言 | Python（Django 平台）、Go（网关 + 采集器）、JavaScript/React（前端） |
| 运行时 | Python 3.11+（Django 5.1 + Channels）· Go 1.25+（网关）/ 1.24+（采集器）· React 18 + Vite · Node 22.18+ |
| 数据面 | **PostgreSQL**（元数据）· **ClickHouse**（span + 时序）· **Redis**（状态/实时）· **Temporal**（作业编排） |
| 部署 | Standalone（单 app 容器 + PG + CH，2 vCPU/4 GB）/ Distributed（每服务一容器 + PeerDB + Kafka，4+ vCPU/12–16 GB）/ Helm（≈4 CPU/8 GiB）；支持 air-gapped（`global.airgap=true`） |
| 遥测 | 自托管默认**开启**部署遥测（上报 owner/admin 邮箱与用量计数；不含 trace/prompt/数据集/密钥），`--no-telemetry` 或 `FUTURE_AGI_TELEMETRY_DISABLED=true` 关闭 |

---

## 3. 功能全景：六支柱 + 一条回路

官方叙事是 `simulate → evaluate → protect → monitor → optimize`，工程上落到**六支柱**（README 原话 "Six pillars. Each one replaces a tool you probably have."）：

| 支柱 | 能力 | 关键数字（README） |
| --- | --- | --- |
| **Simulate** | 面向真实 persona/对抗输入/边界场景的**多轮**对话模拟；**文本与语音**（LiveKit / VAPI / Retell / Pipecat） | 单 run 支持千级会话 |
| **Evaluate** | 一次 `evaluate()` 调起数十项指标：groundedness / hallucination / tool-use correctness / PII / tone / 自定义 rubric | 官方称 **50+ metrics**（源码实际 **100 个评测类** + 82 条系统目录，见 §5.4） |
| **Protect** | 内置 scanner + 厂商适配：PII / jailbreak / injection … | **18 内置 scanner + 15 家 vendor adapter**（Lakera / Presidio / Llama Guard …） |
| **Monitor** | OpenTelemetry 原生 tracing，覆盖 50+ 框架（LangChain / LlamaIndex / CrewAI / DSPy …），span 图 / 延迟 / token 成本 | 50+ instrumentor |
| **Agent Command Center** | OpenAI 兼容网关：100+ provider、15 路由策略、语义缓存、虚拟密钥、MCP、A2A | **~29k req/s（t3.xlarge）、P99 ≤ 21 ms（开护栏）** |
| **Optimize** | 六种提示词优化算法：GEPA / PromptWizard / ProTeGi / Bayesian / Meta-Prompt / Random；生产 trace 回流为训练数据 | **位于 `ee/`（企业许可）** |

**产品 IA（前端 `frontend/src/sections/`，可直接看出模块切分）**：`overview / projects / agents / data(datasets) / evals / prompts / gateway / knowledge-base / annotations / dashboards / error(feed) / alerts / simulate / scenarios / persona / agent-playground / falcon-ai / develop / tasks / journey / sync / model / keys / settings`。

---

## 4. 实现原理 I：分层运行时与数据流

官方架构图口述为**四带**（client layer → edge → platform → data layer），每层都是**开放接口**（OTLP / OpenAI 兼容 HTTP / Postgres·ClickHouse SQL），可逐层替换：

```
客户端 SDK（traceAI / agent-learning-kit / futureagi-sdk / agentcc）
        │  OTLP(HTTP/gRPC)        │  OpenAI 兼容 HTTP        │  MCP / A2A
        ▼                          ▼                          ▼
Edge:  fi-collector（Go 采集器）   agentcc-gateway（Go 网关）   mcp_server（Django）
        │                          │
        ▼                          ▼
Platform（Django）: tracer（OTLP ingest·span 图）· agentic_eval（评测）· simulate（模拟）
                    · model_hub（模型/数据集/embedding）· accounts/usage/integrations
        │
        ▼
Data:  PostgreSQL · ClickHouse · Redis · Temporal
```

**分带职责（源码级）**：

- **Edge（Go）**：`fi-collector`（span 写入唯一权威路径，见 §5.3）与 `agentcc-gateway`（模型调用唯一入口，见 §4.1）。
- **Platform（Django）**：`tracer`（采集兼容与 span 图）、`agentic_eval`（评测引擎）、`simulate`（模拟域）、`model_hub`（模型注册/embedding/数据集）、`accounts`（认证/组织/计量/连接器）、`mcp_server`（把平台能力 MCP 化，见 §5.6）。
- **内置智能体**：`ee/falcon_ai`（产品内 AI 助手，见 §5.7）与 `ee/agenthub`（一组"专职 agent"：错误定位 / 评测编排 / 提示词生成·改进·优化 / 解释 / 集群 RCA）。

---

## 4.1 Go 网关 Agent Command Center（单二进制，40+ 内部包）

`agentcc-gateway/internal/` 有 **40 余个子包**（a2a / alerting / anthropicfmt / async / audit / auth / batch / budget / cache / cluster / config / contracts / edge / files / genaifmt / guardrails / mcp / metrics / middleware / modeldb / models / netguard / otel / payloads / pipeline / plugins / privacy / providers / rbac / realtime / redisstate / responses / rotation / routing / scheduled / secrets / server / streaming / tenant / tokenizer / translation / video）。核心机制：

**(a) 插件化请求流水线（最值得借鉴的骨架）**
- `pipeline.Engine` 按 `Plugin.Priority()` 升序：**pre 顺序执行 → 调 provider → post（顺序写态 + 并行只读）**；任一 pre 返回 `ShortCircuit`/`Error` 即**跳过 provider 并立刻跑 post**（保证记账/审计不丢）。流式请求的 post 由流处理器在流结束后带最终 usage 调用。
- **插件优先级是显式表**：`ipacl 10 → auth 20 → rbac 30 → cache 35 → quota 35 → budget 40 → guardrails 50 → toolpolicy 60 → validation 70 → ratelimit 80 → cost 500 → credits 510 → logging 900 → audit 900 → alerting 997 → prometheus 998 → otel 999`。
- HTTP 中间件链：`Recovery → CORS → RequestID(ULID) → LicenseAuth → KeyAuth → Timeout → router`；`Timeout` 支持请求级 `x-agentcc-request-timeout`。

**(b) 路由策略族（对应"模型路由薄"的补课清单）**
- 可直接用四种：`round-robin`（默认）/ `weighted` / `least-latency`（EWMA α=0.3）/ `cost-optimized`；另有 `adaptive`（学习期 round-robin → 满 **100** 请求后按 latency/success 归一化 + EMA 平滑 0.3、`minWeight 0.05`、30s 更新）、`fastest`（RaceExecutor 并发打多家，首个成功即胜，`cancelDelay 50ms`）、`conditional`（`$eq/$in/$regex/$and/$or…` 规则树）、`complexity`（**8 信号加权**：token_count .15 / message_count .10 / system_prompt_length .10 / tool_count .15 / multimodal .15 / keyword_heuristics .15 / structured_output .10 / max_tokens .10，映射 tier）、`model-fallback`、`mirror`（影子流量按 SampleRate 采样、fire-and-forget）、`provider-lock`（头 `x-agentcc-provider-lock`）、`access-groups`（模型组，`*`/前缀通配）。
- 稳定性参数默认值：`failover`（`maxAttempts=3`，码 `[429,500,502,503,504]`）、`circuit-breaker`（`failureThreshold=5 / successThreshold=2 / cooldown=30s`）、`retry`（`maxRetries=2 / initialDelay=500ms / maxDelay=10s / multiplier=2.0`，full-jitter）。
- **注**：README 的"~9.9 ns 加权路由"与"P99 ≤ 21 ms"**在源码中未见**对应常量（`weighted.go` 只说明热路径用无锁 `math/rand/v2` + 单目标快速路径），引用时应标明为厂商标称。

**(c) 缓存（两层）**
- L1 **exact**：后端 `memory`(默认)/`disk`/`s3`/`azure-blob`/`gcs`/`redis`，**TTL 默认 5 min**（`x-agentcc-cache-ttl` > per-org > 默认）；key = `FNV-128a(namespace|model|messages) + 追加非零采样参数/tools/tool_choice/response_format`；内存版 LRU 上限 10000。
- L2 **semantic**：后端 `memory`(默认)/`qdrant`/`weaviate`/`pinecone`；**相似度阈值默认 0.85、维度 256、条目上限 50000**；**向量化不依赖外部 embedding**——用 3/4-gram FNV-1a 哈希桶 + L2 归一 + 余弦。
- 命中态写入 `rc.Metadata["cache_status"]`（`hit_exact/hit_semantic/hit_conditional/miss/skip`），可作 span 属性。

**(d) 预算与配额**：**五级** org/team/user/key/tag 独立计数器；周期 daily/weekly/monthly/total，`warnThreshold=0.8`，`Hard=true`（**超限即拒，非降级**）；Redis 侧以**微美元整数** HINCRBY + Lua 原子检查记账。

**(e) 护栏（Protect 的落地处）**：`stage: pre|post`；内置 scanner 11 个（`pii-detection / content-moderation / keyword-blocklist / input-validation / prompt-injection / secret-detection / topic-restriction / language-detection / system-prompt-protection / hallucination-detection / data-leakage-prevention`）+ 按配置注册 6 个（`webhook / expression / futureagi / toolperm / mcpsec / external`）+ **15 家 vendor adapter**；动作语义 `block(短路 403) / warn / log`，请求级策略头 `X-Guardrail-Policy: log-only|disabled|strict`；`DefaultTimeout=30s` 且超时/panic 走 **fail-open**（可配）；**流式护栏**每 **100 字符** 检查一次，`FailureAction` 默认 `stop`。

**(f) 多供应商与协议转换**：`providers/`（api_format `openai/anthropic/gemini/bedrock/cohere/azure`）+ `translation/`（非 OpenAI 请求 ↔ canonical OpenAI，含 `RequestToCanonical/ResponseFromCanonical/StreamEventsFromCanonical/ErrorFromCanonical`）+ `anthropicfmt/`、`genaifmt/`、`responses/`（OpenAI Responses API）、`realtime/`（`/v1/realtime` WebSocket 双向代理）、`streaming/`（SSE + `[DONE]` 哨兵）、`batch/`、`files/`、`video/`、`async/`、`scheduled/`。

**(g) 网关自身既是 MCP / A2A 的 server 又是 client**：`/mcp`（JSON-RPC + SSE，工具按 `serverID+sep+name` 命名空间化，客户端 ping 30s、连续 3 次失败标不健康并指数重连）、`/a2a`（`message/send`、`tasks/get`、`tasks/cancel`，`GET /.well-known/agent.json` 出 agent card）；`a2a/<agent-name>` 可注册为**可路由模型**，与普通模型走同一流水线。

**(h) 安全治理**：密钥 **SHA-256 存哈希**（前缀展示）；`tenant/` 以 `key_org_id` 隔离且 org 配置变更回调失效各缓存；`netguard/` 做 **SSRF 出口防护**（禁 `169.254.0.0/16`、`0.0.0.0/8`、`100.100.100.200/32`(阿里)、`fd00:ec2::254/128`(AWS)、`168.63.129.16/32`(Azure) 等，loopback 恒拒，**DNS 解析后按实际 IP 校验防 rebinding**）；`secrets/` 支持 `vault:// / aws-sm:// / gcp-sm:// / azure-kv://`；`rotation/` 密钥轮换（旧 key drain 默认 30s）；`audit/`（缓冲 4096 + 批量 drain，sink stdout/file/webhook）；`cluster/`（心跳 10s/TTL 30s）；`redisstate/` 带熔断（3 次失败/15s cooldown）；admin 端点 `Authorization: Bearer <token>` + 常量时间比较。

---

## 5. 实现原理 II：五个关键子系统

### 5.1 可观测：traceAI（SDK 侧）+ tracer（平台侧）

- **SDK 侧 traceAI**：零配置 OpenTelemetry 埋点，覆盖 Python / TS / Java / C#，50+ 框架 instrumentor（独立仓、独立许可证）。
- **平台侧 `tracer`**：`otel_compat_urls.py` 保留 Langfuse SDK 默认路径 `{base}/api/public/otel/v1/traces` 的兼容映射；PG 的 `ObservationSpan` 模型**已标记 DEPRECATED**（"superseded by CH v2 `spans`"）。
- **span 模型**：`observation_type` 枚举 tool/chain/llm/retriever/embedding/agent/reranker/guardrail/evaluator/conversation；父子关系用 `parent_span_id` 字符串（根=`NULL`）；`Trace` 带 `input/output/error`、`external_id`、`tags`、`error_analysis_status`。
- **语义归一（最值得抄的一件小事）**：`utils/semantic_conventions.py::AttributeRegistry` 维护 **155 条属性别名优先表**，把不同埋点方言折叠到规范键。例：`INPUT_VALUE=[fi.llm.input, input.value]`、`OUTPUT_VALUE=[fi.llm.output, output.value]`、`MODEL_NAME=[llm.model_name, gen_ai.request.model, gen_ai.response.model]`、`INPUT_TOKENS=[llm.token_count.prompt, gen_ai.usage.input_tokens, llm.usage.prompt_tokens]`、`COST_TOTAL=[gen_ai.cost.total, llm.cost.total]`；`normalize_span_kind` 把 `chat/generate_content/text_completion→llm`、`embeddings→embedding`、`execute_tool→tool`、`invoke→chain`。
- **错误归因（Error Feed）**：`tracer/models/trace_error_analysis.py` + `queries/error_analysis.py` + `tasks/error_analysis.py`；路由 `/feed/issues/...`（list/stats/detail/trends/root-cause/deep-analysis/create-linear-issue）与 `/internal/error-feed-v2/...`（grouping/investigation/severity 的 claims/attempts/outbox）。**调查与分组由独立 worker `omega-error-feed-worker` 执行**，并把自身 trace 导出到平台内部的 Observe 项目（`OMEGA_OBSERVABILITY_ENABLED`、默认项目名 `error-feed-investigation` / `error-feed-grouping`）——**注意其文档明确"必须关闭内部项目的 Error Feed 扫描以避免反馈回路"**。
- **告警**：`UserAlertMonitor`（`metric_type` 含 error 数/错误率/无错会话率/provider 错误率/span·llm 时延/token 用量/日·月花费/评测指标；`threshold_type` static|percentage_change；`alert_frequency` 默认 60 min，`auto_threshold_time_window` 默认一周）。

### 5.2 采集器 fi-collector（自研 Go OTel 写入路径）

形态：**极简自研 OTLP receiver**（gRPC `:4317`、HTTP `:4318`；HTTP 接受 `POST /v1/traces` 与 `/tracer/v1/traces`，仅 `application/x-protobuf` 与 `application/json`，body 上限 **16 MiB**）。

- **认证**：`X-Api-Key`+`X-Secret-Key`，或 `Authorization: Basic base64(api_key:secret_key)`（Langfuse 兼容）；直查 PG `accounts_orgapikey`（`enabled=true AND deleted=false`），`KeyType ∈ system/user/mcp`，无 workspace 时取默认 workspace 且**多默认即 fail-closed**；结果缓存 `cache_ttl 5m` / `warm_ttl 1h`。
- **typed-Map split（核心取舍）**：`pkg/adapter.Split()` 把 OTel 属性分派到 `attrs_string / attrs_number / attrs_bool` 三个 Map 列，长内容（`llm.prompt/completion/messages`、`input.value`、`output.value`、`retrieval.documents`、`embedding.embeddings`）强制进 `attributes_extra` **typed JSON overflow** 以约束 Map 基数；整数安全范围 `±9007199254740992`，越界/NaN 降级 overflow；`DeriveHotKeys()` 把 `model/provider/gen_ai_system/gen_ai_operation/tokens/cost` **提升为一级列**（别名表与 Django 侧 `semantic_conventions` 顺序一致）。CH 落库列含 `trace_id/span_id/parent_span_id/latency_ms/prompt_tokens/completion_tokens/total_tokens/cost/attrs_*/attributes_extra/end_user_id/trace_session_id`。
- **背压模型**：OTLP receiver 默认队列 → **memory_limiter 硬上限（超限回 429，SDK 退避）** → batch（**代码/配置默认 5000 行 / 5s**，README 写 10K/5s，**文档与实现存在差异**）→ retry（`max_retries 5`，`initial_backoff 100ms` → `max_backoff 10s`）→ **磁盘持久队列**（重启存活）→ CH `async_insert`。关闭时一次 drain，"in-flight 丢失有界于最后 5s"，语义 at-least-once。**没有静默丢弃、没有 OOM 崩溃**。
- **成本计算**：`pkg/pricing` 链式——先嵌入的 litellm 价格表快照（`//go:embed`，MIT，可 `FI_PRICING_JSON` 外挂刷新；tier 边界 `128_000` prompt tokens）→ 再按 org 的 `CustomAIModel` 自定义价（per-1K，缓存 24h，`errTTL 45s`）；两者都不命中时 cost=0。
- **属性目录**：`attributecatalog`（按 span 生成确定性的 top-K 键值目录，gap 原因 `max_keys/max_array_members/max_encoded_bytes/...`）与 `observedcatalog`（**动态属性发现**，wire format `futureagi.observed-attributes` v1，`MaxKeysPerSpan 128 / MaxArrayMembersPerSpan 256`，`disabled/kafka/direct` 三模式，**默认 disabled**）——即"随着新模型/新工具冒出，属性目录自动生长"。
- **为何自研**（官方三条，值得作为技术选型判据）：① 官方 contrib `clickhouseexporter` 硬编码自家 schema，不认识 typed Map / 投影 / overflow，会逼你重写全部看板查询；② Django 直写 CH 会把 Python 留在热路径（目标 1B spans/day 不可行）；③ 自研 Go exporter 的 OTel Collector framework 带 ≈300 MB 传递依赖——**注意 README 与 `server.go` 措辞不一致（后者是极简自研 receiver，未用 collector framework）**。

### 5.3 评测引擎（Evaluate）

- **架构**：抽象基类 `BaseEvaluator(ABC)`（`name/display_name/metric_ids/required_args/examples/is_failure/_evaluate/run/guard/run_batch`）；模块级注册表 `evaluations/engine/registry.py`（`_REGISTRY[eval_type_id] → class`）；执行器 `engine/runner.py::run_eval`（查类→建实例→备参→执行→格式化）。
- **规模**：`agentic_eval/core_evals/fi_evals/__all__` 导出 **100 个评测类**；系统目录 `evaluations/catalog/system_evals.yaml` **82 条**（三类：`agent`（多轮 LLM-as-judge，带工具）/ `llm`（单次 `CustomPromptEvaluator`，用户自定义 rubric）/ `code`（确定性 Python 代码））。
- **指标族（节选）**：事实性/groundedness（`GroundedEvaluator`、`AnswerSimilarity`、多种相似度：Jaccard / NormalisedLevenshtein / JaroWinkler / SorensenDice / Cosine / Phonetic）；检索排序（`RecallAtK/PrecisionAtK/NdcgAtK/Mrr/HitRate/MeanAveragePrecision`）；参考文本（`Bleu/Rouge/Meteor/Gleu/Chrf/Squad/F1/WER/CER/MER/TER/WordInfoLost/WordInfoPreserved/CodeBleu`）；RAG 上下文（`NonLlmContextPrecision/Recall`、`SemanticListContains`、`EmbeddingSimilarity`）；轨迹（`ToolCallAccuracy/TrajectoryMatch/StepCount/ApiCall`）；格式结构（`JsonSchema/IsJson/IsXml/IsSql/IsUrl/Regex/Contains*/Length*/SyntaxValidation`）；统计一致性（`CohenKappa/FleissKappa/MatthewsCorrelation/Pearson/Spearman/R2/Rmse/LogLoss/BalancedAccuracy/FBeta`）；多模态（`ClipScore/FidScore/Ssim/Psnr/ImageProperties`）；安全（`IsRefusal`、`protect_*` 系列走 Turing 模型）；文本多样性（`DistinctN/TypeTokenRatio/RepetitionRate`）。
- **LLM-as-judge 实现**：`CustomPromptEvaluator._evaluate` 组装 messages（system + 消息链 + 用户 prompt + 多模态块）后二选一——Turing 托管模型走 `ee.turing.client`，否则 `call_llm(..., response_format=...)`；解析固定键 `result` / `explanation`，指标 id 固定 `custom_eval_score`；**底层 `LLM` 走 gateway-first（带 `X-Org-Id` 头）→ 回退 litellm → 托管模型走 EE managed_ai**；成本写 ClickHouse。
- **阈值分层解析**（很值得抄的一份优先级设计）：`resolve_pass_threshold` 优先级 **run_config > 顶层平铺 > version > template > 硬兜底 0.5**。
- **运行时覆盖白名单**：`_RUNTIME_ALLOWED_KEYS`——只有白名单内的键且值非 None 才覆盖；**`model` 不在白名单**（防"用便宜模型悄悄改评测结论"）。
- **rubric 渲染**：用 **Jinja2 `SandboxedEnvironment`**（`undefined=PreserveUndefined`，防 SSTI），支持 `{{spans.0.kind}}` 这类点号/数字索引变量；`rule_prompt + criteria` 是**纯文本而非 JSON DSL**。
- **并发/重试**：`run_batch(max_parallel_evals=5)` → `ThreadPoolExecutor` 且用 `wrap_for_thread` **传播 OTel 上下文**；LLM 调用 `@retry(stop_max_attempt_number=3, wait_fixed=2000)`。
- **输出与空值语义**：`output_type ∈ Pass/Fail | score | numeric | reason | choices`；`failure: bool|None`（None=未判定）；**无 metrics 时 `value=None`**（不补 0）；`choices` 用 `choice_scores` 映射（大小写不敏感、未知标签跳过）；`clamp_unit_score` 对 None/非数值原样返回。
- **PPT 数据语义纪律**：`failed step` 记 None 而非 False；`dspy_module/` 只是"签名即 prompt 模板"的判分脚手架，**未发现 teleprompter/编译式优化**。

### 5.4 模拟引擎（Simulate）——本项目最该对标的子系统

- **域模型**：`Scenario`（`scenario_type` graph/script/dataset，`source_type` agent_definition/prompt）、`RunTest`（含 `scenarios` M2M、`dataset_row_ids`、`enable_tool_evaluation`）、`TestExecution`（`ExecutionStatus` 7 态，`trials` 默认 1）、`CallExecution`（`CallStatus` 7 态，含 `recording_url` / `cost_cents` / `provider_call_data` / `conversation_metrics_data`）、`CallTranscript`（`SpeakerRole`: user/assistant/system/tool_calls/tool_call_result/unknown，逐段 `start_time_ms/end_time_ms/confidence_score`）、`CallExecutionSnapshot`（重跑前快照，`RerunType`: eval_only / call_and_eval）。
- **Persona 体系**：结构化字段含 `gender/age_group/occupation/location/personality/communication_style/languages/accent/conversation_speed`、`interrupt_sensitivity`、`finished_speaking_sensitivity`、`punctuation/slang_usage/typos_frequency/emoji_usage/tone/verbosity`；**内置 18 个 system persona**（如 1 The Impatient Driver / 3 The Stressed Accountant / 4 The Frustrated Subscriber / 13 The Tech-Savvy Young Professional / 18 The Enterprise IT Admin），落库为 `persona_type=system, is_default=True`。
- **对话引擎**：抽象 `ChatServiceBlueprint` + provider 注册表（VAPI / FUTUREAGI），编排核心 `chat_sim.py`（`initiate_chat` → `create_session` → 逐轮 `send_message`；`MAX_CONVERSATION_TURNS` 默认 **500**；模拟者=AI 顾客充当 `user`，被测 agent=assistant）；文本风格指南 `constants/persona_prompt_guides.py`。
- **托管沙箱（hosted harness）**：把"被测 agent + 其世界"放进**一次性托管沙箱**跑 ALK（Agent Learning Kit）。`HostedHarnessJob`（`State` 11 态，**`MAX_SCENARIOS_PER_JOB=5000`**，带 `idempotency_key`/`request_digest`/`seed`/`deadline_at`）、`HostedHarnessAttempt`（`State` 9 态，带 `token_hash/fence_hash/expires_at`、`effective_parallelism/degrade_reasons`）；沙箱 provider 抽象（`daytona` / `e2b`）；**出口域白名单 `_MAX_EGRESS_DOMAINS=20`**；引擎目录 `postgres16/redis7/rabbitmq3.13`、运行时目录 `python 3.11–3.13 / node 20·22 / git·ffmpeg`；容量公式 `HARNESS_MAX_WORLD_SLOTS` 默认 **8**；沙箱日志**脱敏后** gzip 存 S3（`_redact` 覆盖私钥/Bearer/query token）。
- **凭证三层**：`ProviderCredentials`（Fernet，密文前缀 `enc::`）、`HostedHarnessSecret`（tenant 级）、`HarnessEnvironmentCredentials` + `HarnessCredentialFile`；`harness_credentials.materialize_secret_refs` 把平台文件 ref 换为 attempt 级 runner ref。
- **Temporal 编排**：队列 `tasks_s / tasks_l / tasks_xl / simulation_runner`；常量——`LAUNCH_BATCH_SIZE=50`、`LAUNCH_SUB_BATCH_SIZE=10`、`LAUNCH_SUB_BATCH_DELAY_SECONDS=5.0`、`CONTINUE_AS_NEW_THRESHOLD=500`、`DISPATCHER_CONTINUE_AS_NEW_THRESHOLD=2000`、`MAX_CALLS_PER_ORCHESTRATOR=500`、`DEFAULT_APP_LIMIT=100`、`DEFAULT_ORG_LIMIT=25`、`MAX_CALL_DURATION_SECONDS=1800`、`STALE_SLOT_THRESHOLD_SECONDS=2400`；工作流 `TestExecutionWorkflow / RerunCoordinatorWorkflow / SimulationRunnerWorkflow / HostedHarnessGatewayWorkflow`。
- **模拟→评测衔接**：guest 只回 eval 名，平台按白名单映射（`harness_evals.py`，`MOST_SELECTED_EVALS=8`）；`harness_run_evals.py` 给已完成 run 的每条 call 排队补评（窗口 `10min`、时钟偏移容忍 `60s`）；`run_regrade.py` 有类型化拒绝异常并把失败回滚。
- **分析口径（`run_dashboard_v3*.py` / `run_reliability_v3.py`）**：`GOAL_OUTCOMES=(queued,in_progress,passed,failed,error,escalated,inconclusive)`；延迟 p50/p90/p99 只在 `latency_ms>=0` 时计入；**sentiment 明确"Provider-reported only，平台不计算"**；可靠性用多次 trial 聚合出 `passed/failed/flaky/not_evaluated` 与 `flip_rate`，并用 cluster-robust 方差 + Wilson（`z=1.96`）给 95% 区间；**"缺失测量保持 null 而非 0"**（`measured` 计数为 0 时值为 None；`NOT_REPORTED` 区分"未上报"与 0）。

### 5.5 优化与内置智能体

- **Optimize（`ee/agent_opt/`，企业许可）**：`optimizers/` 六个算法（`gepa / promptwizard / protegi / bayesian_search / metaprompt / random_search` + `stepper`）+ `generators/` + `datamappers/`；把生产 trace 回流为训练数据。
- **Falcon AI（`ee/falcon_ai/`，产品内 AI 助手）**：WebSocket consumer + `modes.py`（`CORE_TOOLS` 6 个 + `COMMON_TOOLS` 20 个 + 13 个工具类别，按 mode 组合暴露）+ `builtin_skills/*.yaml`（**10 个内置技能**：analyze-costs / analyze-trace-errors / build-dataset / cluster-rca / compare-models / debug-traces / fix-with-falcon / localize-errors / optimize-prompts / run-evaluations，每个含 `slug / name / icon / description / trigger_phrases / tool_names / instructions`）+ `context_manager.py` 上下文压缩（`COMPACTION_MESSAGE_THRESHOLD=20`、`COMPACTION_TOKEN_THRESHOLD=80000`、`MAX_HISTORY_TOKENS=120000`、`KEEP_RECENT_MESSAGES=8`、单条 tool 结果 `MAX_RESULT_CHARS=2000`、单条消息 `MAX_MESSAGE_CHARS=4000`、`CHARS_PER_TOKEN=3.5`）+ `views_memory.py`（key/value 记忆 `save/list/delete`）+ `mcp_proxy.py` + `oauth_client.py`。
- **工具注册表 `ai_tools/`**：`ToolRegistry` 单例 + 模块级 `@register_tool` 装饰器，**260 个工具**（17 组：agents / annotation_queues / annotations / context / datasets / docs / evaluations / experiments / optimization / prompts / simulation / tracing / usage / users / web），**同时供 MCP Server 与产品内 AI 助手消费**（"One registry, two consumers"）。

### 5.6 MCP 服务化（`futureagi/mcp_server/`）

- **不是反射**：工具清单来自"**已提交的 Swagger 2.0 契约 + 人工 curated catalog**"→ `tool_generation.py` 生成 manifest（含 `inputSchema / annotations / request.{method,path,parameters} / response.unwrap`）；`generated_registry.py` 用 `django.urls.resolve()` 判断某 path 在当前部署是否挂载（**未挂载则不暴露工具**）；执行由 `api_executor.DjangoAPIExecutor`（`APIRequestFactory` + `force_authenticate` + 注入 `X-Organization-Id`/`X-Workspace-Id`）完成。
- **规模与分组**：**92 个工具**，14 组（datasets 15 / observability 15 / dashboards 12 / simulation 10 / evaluations 7 / prompts 7 / agents 6 / annotations 4 / gateway 4 / context 3 / experiments 3 / optimization 3 / error_feed 2 / usage 1）。
- **传输**：默认 **Streamable HTTP 单端点 `/mcp`**（`stateless=True, json_response=True`，无会话粘性、可水平扩展）；SSE 为 legacy；stdio 走内部 API。会话心跳 30s、stale 24h。
- **OAuth**：自实现 `OAuthAuthorizationServerProvider`（client 30d / code 10m / access 3600 / refresh 30d / approve 10m），scopes 取工具组 keys；生产重定向前端同意页。
- **限流/响应限制**：Redis Lua 滑动窗口，`RATE_LIMITS` free `{200/min, 5000/day, 5 sessions}`、pro `{100/min, 10000/day, 5}`、enterprise `{500/min, 100000/day, None}`；响应 `MAX_RESPONSE_BYTES=64KiB`、`MAX_BLOB_STRING_CHARS=2000`，超限抛 `ResponseTooLargeError`；审计 `MCPUsageRecord` 对敏感键替换为 `[Filtered]`。

### 5.7 工程质量体系（比机制更值得抄的"纪律"）

- **测试三层门禁**：本地（跑改动面）→ git hooks（lint-staged 快速反馈）→ GitHub Actions（全套）。前端覆盖率**全局阈值 70%**（branches/functions/lines/statements），并有 `yarn test:api-journeys`（无头浏览器 API 旅程，`API_JOURNEY_MUTATIONS=1` 才跑变更类）。
- **类型纪律**：**mypy baseline 驱动**——新代码必须带类型、存量豁免；**CI 拒绝新增类型错误**（`make mypy`）。
- **pre-commit**：black / isort / ruff / django-upgrade / djlint（自动修）+ mypy / bandit / detect-secrets / django-check / check-yaml·json（守门）。
- **契约驱动**：`api_contracts/`（`gateway/agentcc-admin.schema.json`、`provider-url-policy.json`、`harness/*` 多份 seam 契约与 handoff 文档）——前后端与网关以 JSON schema/契约文件对齐。
- **给 AI 编码助手的技能包**：仓库自带 **`.agents/skills/` 与 `.claude/skills/`**，含 `reviewing-prs`（7 步 PR 审查流程 + 标准清单 + E2E 覆盖门禁）与 `writing-e2e-flows`（含 design-doc-mining / flow-plan-template / footguns / harness-cheatsheet / harness-gaps 参考件）——**把团队工程规范固化成 agent skill**。

---

## 6. 对本项目智能体模块的借鉴（产品设计 + 技术实现）

> 面向本项目智能体模块的五层视角（Model / Context / Harness / Loop / Graph，见 00 号导读与 53~56 号域档）与既有机制（装配链、run_event、调用轨迹面板 REQ-217、34 号平台助手、连接器 REQ-214、本体挂载 onto_*）。**结论一律分三档：A 可直接借鉴 / B 需改造后借鉴 / C 不能照搬（边界）**。

### 6.1 产品设计可借鉴（A 档为主）

1. **给智能体补一条"评测/回归"产品面（A，最该先做）**：本项目已有 **调用轨迹面板（REQ-217）+ run_event + 上下文预算档**，但缺"把一次真实运行固化成可重跑的评测用例"。可借鉴其**"生产 trace → 数据集 → 回归实验"**闭环与 `CallExecutionSnapshot` 的 `eval_only / call_and_eval` 两种重跑语义——落到本项目即：**给智能体加"评测集/回归集"概念 + 一键用历史运行回灌**。这是把"轨迹面板"从可读升级为可用的最短路径。
2. **技能 YAML 化 + `trigger_phrases` 驱动的技能发现（A）**：Falcon AI 的 skill 结构 `slug / name / icon / description / trigger_phrases / tool_names / instructions` 与本项目 **06_技能模块**的形态同源，但多两样值得抄：**`trigger_phrases`（用户措辞触发发现）** 与 **`tool_names` 白名单（技能自带工具子集）**。对本项目"技能携带工具白名单"（装配链第②步）是直接增强。
3. **mode/类别化的工具暴露（A）**：`modes.py` 用 `CORE_TOOLS + COMMON_TOOLS + 类别分组`按场景裁剪可见工具集。本项目**34 号平台助手**与"运行时工具清单预览"（53 号 W3）可借鉴：**按 mode 暴露工具子集**，既省上下文又降误用。
4. **"缺失值不补零"的数据口径（A）**：`measured` 为 0 时值为 `None`、`NOT_REPORTED` 与 0 分离。与本项目"诚实边界"文化完全一致，建议**直接采纳为 run 分析/调用轨迹统计的硬口径**（现在缺失项容易被默认 0 污染）。
5. **厂商自述数字与源码口径分离（A）**：该仓 README 的"9.9 ns / P99≤21ms / 10K spans"在源码中找不到或与默认值不符。**这正是本项目"文档与实现不符以实现为准并回修"（纪律 #6）的正面案例**——引用第三方能力时要标"厂商标称 vs 可验证"。

### 6.2 技术实现可借鉴（A/B 档）

6. **跨来源属性归一表（A，低成本高收益）**：`AttributeRegistry` 的 **155 条别名优先表**把不同埋点方言折叠到规范键（`INPUT_VALUE=[fi.llm.input, input.value]` …）。本项目 run_event 是自定事件模型、且要接外部 harness/CLI 后端（31 号 dsh / Claude Code），**建议引入一层"别名→规范键"映射表**，为将来统一外部事件、以及调用轨迹面板的过滤/分组提供单一口径。
7. **显式插件优先级 + "post 分并行只读组 / 顺序写态组"（A）**：本项目装配链是"按序合并、先到先得"，但**优先级是隐含的**。可借鉴其**显式优先级表**（10…999）与**短路后仍跑 post 以保记账/审计不丢**的语义——尤其"**短路/失败也必须落审计与用量**"这一条，对 REQ-14 审批与 run_event 完整性有直接价值。
8. **流式输出分块护栏（A）**：其 `StreamGuardrailChecker` 每 **100 字符**跑一次 post 护栏、默认动作 `stop`。本项目 SSE 流式对话 + 本体伴生事件（REQ-281）场景下，"**流中分块校验并支持中止**"比"结束后整段校验"更有用。
9. **护栏策略头 `log-only / disabled / strict` + fail-open 可配（A）**：把"观测态 / 关闭 / 严格"做成**请求级可切**策略，并显式声明超时行为（其默认 `30s` 超时 **fail-open**）。本项目 REQ-14 目前是"保守全审"（53 号边界），可借鉴其"**策略可切 + 超时行为显式**"的设计，把"保守"从硬编码变成可配置的默认值。
10. **零依赖语义缓存（B）**：`FNV n-gram 哈希 + L2 归一 + 余弦`，**不引入外部 embedding**，阈值 0.85。本项目做"模型/工具结果缓存"时若想避免额外向量依赖，这套是现成参考；也可与 35 号"少压缩、结构化压缩""缓存经济学"结论互相印证。
11. **路由策略族作为"模型路由薄"的补课清单（A/B）**：体检 A-6 标本项目模型路由薄。其 `complexity`（**8 信号加权→tier**）、`adaptive`（学习期→按延迟/成功率归一化）、`fastest`（竞速首胜）、`cost-optimized`（最低优先级组内轮转）**可直接作为立项输入**；`provider-lock` 头与 `mirror` 影子流量（采样、fire-and-forget、30s 超时）也是低成本高价值项。
12. **沙箱出口域白名单 + 容量槽 + 降级原因（B）**：本项目沙箱包（in-proc/docker/k8s）可借鉴**出口域白名单（上限 20）**、**容量槽公式（默认 8）** 与 **`degrade_reasons` 显式降级原因**——尤其"降级必须显式记录原因"与本项目"诚实标注"一致。
13. **Temporal 编排常量（B）**：批次/子批次/延迟（50 / 10 / 5s）、continue-as-new 阈值（500 / 2000）、并发上限（app 100 / org 25）、stale 阈值（2400s）等**是可直接调参借用的工程标定**；本项目 Loop 层（55 号）的定时续跑与调度 DB 化可对照其队列分层（s/tasks_l/xl/runner）。
14. **评测阈值分层解析 + 运行时覆盖白名单（B）**：`run_config > 顶层 > version > template > 0.5` 与"`model` 不进白名单"两条，对本项目**本体运行方案/技能挂载的配置覆盖**（多来源配置的优先级与安全边界）是很干净的范式参考。
15. **rubric 用 `Jinja2 SandboxedEnvironment` 渲染（A）**：若本项目做"提示词模板变量渲染"（技能说明、本体指引、评测标准），**必须用 sandbox 环境**，这是低成本的安全防线（防 SSTI）。
16. **工具注册表"单一真源 + 双消费者"（A）**：`ai_tools` 的 `ToolRegistry` 单例被 MCP Server 与产品内 AI 助手共用（260 工具）。本项目**工具合并管线**与 **MCP 挂载**现在是两条路径，可借鉴"**一个注册表、多个消费者（对话/MCP/内置助手）**"，避免内外两套工具清单漂移。
17. **契约驱动前后端 + mypy baseline + 覆盖率门禁（A）**：本项目 AGENTS.md 已有硬性纪律体系，可补两条可落地的：**接口契约为文件**（如 `api_contracts/*.schema.json`）与**类型基线门禁**（拒新增类型错误）。这两条对"多 agent 并行改同一仓库"的场景收益最大。
18. **给 AI 编码助手发技能包（A）**：其 `.agents/skills/`（reviewing-prs / writing-e2e-flows）是"**把团队工程规范固化成 agent skill**"的现成范例——本项目已有 `docs/20_回归冒烟清单` 与 AGENTS.md 纪律，**可把"冒烟清单"直接改写成一份 agent skill**（编写/复跑冒烟的可执行流程），让协作 agent 与人都按同一清单走。

### 6.3 不能照搬的边界（C 档，必须显式标注）

19. **整栈是"重"的 Python/Django + ClickHouse + Redis + Temporal 架构（C）**：与本项目 **D-O15「不引入 Python 运行时依赖、单机零 venv」** 直接冲突。**只能借鉴设计思路；若要真接，唯一合规形态是"独立进程/容器对接"，且需按 16 号部署档评估资源**。
20. **open-core 边界必须标注（C）**：**Optimize（agent_opt）、Protect 模型（Turing）、voice、falcon_ai、agenthub、usage 全在 `ee/` 下，属 Enterprise License**。引用时**不得把"六支柱"整体当作 Apache-2.0 可用能力**——README 的"no open core gotchas"与 `LICENSE-EE` 并存，须以许可证为准。
21. **迁移期与文档-实现差异（C）**：仓库正处于 **PG → ClickHouse 25.3 迁移 + OTLP 接入迁至 fi-collector** 的过渡期（多处注释标 2026-06，`ObservationSpan` 已 DEPRECATED，Django 侧 `v1/traces` 被注释）。**引用其"当前架构"时要注意时序**，别把迁移中间态当稳态。
22. **"用模型审模型"的自证风险（C，需带边界引用）**：其内部 error-feed worker 把自身 trace 导出到平台内部 Observe 项目，并需**手工关闭内部项目的扫描以避免反馈回路**——这是"审计系统自己也要被审计"的经典陷阱，引用其评测能力时须连同"**需切断自评回路**"一起引（与 47 号 Rubric Judges"误差高度相关、级联收益仅 1.5~2.7 点"的发现同类）。

---

## 7. 资料与开源项目汇总

### 表 1 · 平台组件与关键技术（仓库内）

| 组件 | 形态/技术栈 | 职责 | 关键数字/默认值 | 对本项目相关点 | 借鉴度 |
| --- | --- | --- | --- | --- | --- |
| `agentcc-gateway` | Go 1.25 单二进制 | OpenAI 兼容网关（路由/缓存/预算/护栏/MCP/A2A） | 插件优先级 10~999；failover 3 次；CB 5/2/30s；缓存 TTL 5m、语义阈值 0.85 | 模型路由补课（A-6）、显式优先级、短路仍审计 | ★★★ |
| `fi-collector` | Go 1.24 + clickhouse-go/v2 | OTLP 采集 → CH 落库（唯一写路径） | gRPC:4317/HTTP:4318；body 16MiB；batch 5000/5s；retry 5 | 背压模型、typed-Map、hot-key 提升 | ★★★ |
| `futureagi/tracer` | Django | span 模型/图、语义归一、Error Feed、告警 | 属性别名字典 **155 条**；span 类型 11 种 | 跨来源属性归一表、流式审计 | ★★★ |
| `futureagi/agentic_eval` | Django + DSPy + OpenAI | 评测引擎（LLM-as-judge + 启发式 + ML） | **100 评测类**；并行 5；阈值兜底 0.5；重试 3×2s | 评测面立项、阈值分层、覆盖白名单 | ★★★ |
| `futureagi/evaluations` | Django | 评测注册表 + 系统目录 | **82 条系统评测**（agent/llm/code） | 技能/评测标准 YAML 化 | ★★★ |
| `futureagi/simulate` | Django + Temporal | persona 模拟、托管沙箱、语音、分析 | 18 内置 persona；max turns 500；槽位 8；job 上限 5000 | 回归集/评测用例、沙箱出口白名单 | ★★☆ |
| `futureagi/mcp_server` | Django + 官方 mcp SDK | 平台能力 MCP 化 | **92 工具/14 组**；限流 200/min；响应 64KiB | 工具"单一注册表双消费者" | ★★☆ |
| `futureagi/ai_tools` | Django | 工具注册表（单例 + 装饰器） | **260 工具/17 组** | 工具注册表统一 | ★★★ |
| `ee/falcon_ai` | Django + Channels + WebSocket | 产品内 AI 助手 | 10 内置技能；压缩阈值 20 条/80K；保留 8 条 | 34 号平台助手对标、技能 trigger_phrases | ★★★ |
| `ee/agent_opt` | Django（企业许可） | 提示词优化 | 6 算法 + stepper | 生产 trace 回流；**EE，不可照搬** | ★☆☆ |
| `frontend` | React 18 + Vite | 平台 UI（24+ 模块） | 覆盖率门禁 70% | 模块 IA 切分参考 | ★★☆ |
| `api_contracts` / `.agents/skills` | JSON Schema / SKILL.md | 契约与 AI 编码助手技能包 | reviewing-prs（7 步）/ writing-e2e-flows | 契约驱动、把工程规范固化成 skill | ★★★ |

### 表 2 · 独立开源仓与外部生态

| 名称 | 定位 | 语言/技术栈 | 许可 | 与本项目相关块 | 借鉴点 | 风险与边界 |
| --- | --- | --- | --- | --- | --- | --- |
| `future-agi/traceAI` | 零配置 OTel 埋点（50+ 框架） | Python/TS/Java/C# | Apache/MIT | 调用轨迹埋点、跨框架接入 | instrumentor 覆盖清单可作接入目标 | 与自研 run_event 范式不同，需适配层 |
| `future-agi/agent-learning-kit` | 本地优先的测试/模拟/红队/优化 | Python/TS | 开源 | 模拟与红队能力 | `--no-jev-*` 式消融开关思想（可剥离测量） | 依赖其运行时 |
| `future-agi/futureagi-sdk` | 平台 SDK（数据集/提示词/KB/实验） | Python | 开源 | 数据集/实验 API 形状 | 面向 CI/CD 的 `evaluate-pipeline` | Python 依赖（D-O15） |
| `future-agi/agent-command-center-sdk` | 网关客户端 SDK | Python/TS | 开源 | 网关客户端形状 | LangChain/LlamaIndex/React/Vercel 适配 | 与网关强绑定 |
| LiveKit / VAPI / Retell / Pipecat | 语音 agent 平台 | 各自 SDK | 商业 | （平台未做语音，参考） | 语音评测链路（转写/延迟百分位） | 语音非本项目当前范围 |
| Lakera / Presidio / Llama Guard 等 15 家 | 护栏/内容安全 | 各厂商 | 商业/开源混合 | 护栏适配器清单 | "内置 scanner + 厂商 adapter"双层 | 引入即多依赖，仅作能力对标 |
| DSPy | LLM 程序化编排 | Python | Apache | 评测判分脚手架 | `Signature + ChainOfThought` 的判分脚手架 | 其 dspy module 并未做编译式优化 |
| litellm（价格表） | 模型价格目录 | JSON/Python | MIT | 成本计算 | 价格表快照 + 自定义价兜底 | 价格时效需刷新 |

---

## 8. 需要开发者拍板/后续跟踪的开放项

1. **是否给智能体模块立项"评测/回归集"产品面**（对应 §6.1-1、§7 表 1 的 `agentic_eval`/`evaluations`）——这是本档认为**收益最高、且与既有 REQ-217 轨迹面板天然衔接**的一条；若立项，编号须先查 `docs/18` 注册表。
2. **模型路由补课范围**（对应 §6.2-11 与体检 A-6）：先做 `complexity + least-latency + cost-optimized` 三档，还是连 `adaptive/mirror` 一起——建议分批。
3. **run_event 是否引入"属性别名归一表 + 缺失值 null 纪律"**（§6.2-6、§6.1-4）：两条都是低成本改动，建议纳入下一个 Context/可观测相关任务的顺手件（**P0 级、约 1 人日**）。
4. **护栏策略可切（log-only/strict）+ 流式分块校验**（§6.2-8、§6.2-9）：与 REQ-14 审批面精细化（53 号 W2 项）合并评估。
5. **把 `docs/20_回归冒烟清单` 改写成 agent skill**（§6.2-18）：用于协作 agent 复跑冒烟，成本极低。
6. **本档引用纪律**：凡引用其能力数字，须区分"厂商标称（README）"与"源码可验证"；凡引用 `ee/` 能力，须标注 **Enterprise License**。

---

> **收尾声明**：本档为**通用行业调研 + 面向本项目的借鉴分析**，事实以 `future-agi/future-agi@main`（2026-10-10 抓取）为准；该仓处于 PG→ClickHouse 迁移过渡期，架构细节可能随版本变化，引用前请回原始来源核对。凡本文未在源码中证实的表述（如 README 的 9.9 ns / P99 ≤ 21 ms / 10K spans 批大小），已在正文标注为"厂商标称"或"文档-实现差异"。
