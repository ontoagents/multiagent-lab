---
module: 智能体
topic: Harness 域事实源（设计总纲·实现现状·演进跟踪）
desc: harness 技术沉淀单一事实点——层职责与代码地图、已交付能力表（REQ/M58 里程碑映射）、演进跟踪（触发驱动项）、开放问题与诚实边界；域内文档导航
req: [REQ-14, REQ-190, REQ-191, REQ-201, REQ-202, REQ-203, REQ-204, REQ-206, REQ-214, REQ-219, REQ-224, REQ-231, REQ-232, REQ-225, REQ-317, REQ-322, REQ-323, REQ-330, REQ-354, REQ-355, REQ-356, REQ-362]
docs: ["02 §6 五层总纲", "38 号 B 阶段", "51 号体检", "17 §4.3"]
decisions: [D-O13]
synced: 2026-10-19
---

# Harness：设计、实现与演进（2026-10-01 沉淀）

> **定位**：harness 域（工具装配/防护 hooks/审批 gating/验证背压/重试/沙箱运行后端/事件契约）的**单一沉淀与演进跟踪点**——把散在 38/51/02/17/31/41 各档的 harness 内容收敛为一处导航与一张现状表。技术总纲以 `docs/02 §6 五层总纲` 为准（本档引用不重复）；需求行以 `docs/01 §3.9` / `docs/18` 为准；本文维护「实现到哪了、下一步什么在等触发」。
> **文档地图**（harness 相关内容分布在哪）：
> | 档 | harness 相关职责 |
> | --- | --- |
> | [38 号](38_智能体演进路线建议_HarnessLoopGraph.md) | 分层理论与 B 阶段执行面补齐（**历史路线档**，B1~B5 已全部交付，不再更新） |
> | 51 号体检（2026-10-01，已清理） | harness 专项体检——结论归档于本档 §二.5，W 系列全部归并落地 |
> | docs/02 §6 五层总纲 | 技术方案事实源（层定义+代码承载对照；「配置聚合呈现≠层职责混淆」） |
> | docs/01 §3.9 + docs/18 | 需求行：REQ-202/206/214/219/224/231/232/225 |
> | [17 号 §4.3](../01_整体设计/17_产品_信息架构与界面设计.md) | 侧板 Harness 页签 IA 定案（五层视角并列、审批开关迁侧板落点） |
> | [31 号](31_智能体对接DeepSeek-Harness_可行性及方案分析.md) / [41 号](08_调研预研/外部平台调研/41_DeepSeek-Harness_实现方案深度调研.md) | 外部 harness（dsh）对接方案与源码级参照 |
> | docs/20 | 冒烟检查行：S2.18（执行面）/2.24（分层侧板）/2.26（审批面） |
> | [54 号 Context](54_Context_设计实现与演进.md) / [55 号 Loop](55_Loop_设计实现与演进.md) / [56 号 Graph](56_Graph_设计实现与演进.md) | **姊妹域档**（2026-10-01 同构沉淀）——五层各域事实源并列，五层总纲的域内展开 |

## 一、设计总纲：harness 层是什么

harness = 模型之外「怎么把事做安全、做扎实」的一层。五层视角（Model/Context/Harness/Loop/Graph）中，**归 harness**：工具面（装配/来源/前缀）、防护（hooks/SSRF/安全根）、审批 gating、验证背压、重试、沙箱执行后端；**不归**：Context（预算/压缩/剪枝/召回）、Loop（迭代上限/checkpoint/续跑）、Model（连接/采样/推理后端选择）。侧板配置页签跨层聚合呈现，层职责以本口径为准。

**组件与代码地图**（backend/）：

| 组件 | 代码 | 职责 |
| --- | --- | --- |
| 检索源接口族 | `internal/chat/{runner.go,kbsource.go}` | 对话召回三源接口反转（nil=未接入降级）：CompanionSource〔REQ-170〕/CommunitySource〔KB-5③〕/KBSource+KBAdminSource〔REQ-355 R1——chat 零 kb import，api.NewKBChatSource 适配 kb.Service；recallKB 经 RetrieveContext 单方法检索+渲染〕 |
| 装配链 assembleTools | `internal/chat/assembler.go` | 8 步管线：内置勾选→技能白名单→连接器（mcp 直通/k8s·ssh 经插件服务 :8093）→本体 facade→项目文件→执行面原语→hook 包装→审批包装；`{连接器实例名}__{tool}` 前缀改名；先到先得+重名告警 |
| HookChain | `internal/tool/hooks.go` | PreHook（带 guard 名，拒绝=结构化回执回喂+hook.denied 审计事件）/PostHook；fetchGuard（http_fetch 内网预检，启用）/cmdGuard（危险命令模式表，预置未启用） |
| 审批 gating | `internal/tool/approval.go` | 三档 off/danger/all（危险清单=写类内置+http_fetch+连接器/MCP 非 read-only 前缀，ontology/oo 免审）；豁免清单 approval_exempt；挂起经 checkpoint（SuspenseAt 持久化），恢复重入超时自动 deny |
| 验证背压 | `internal/chat/verify.go` | verify_on_stop：运行收尾执行验证命令，失败→run.finished reason=verify_failed+独立 verify.failed 事件 |
| 重试 | `internal/chat/retry.go` | 幂等错误白名单 N=2 指数退避（当前仅装配期 MCP 拉取） |
| 沙箱后端 | `internal/runtime/` | inprocess/docker/k8s/auto（lastGood 粘滞）+agent/run 两作用域；runtime_settings DB 覆盖 env（REQ-191）；启动对账/确定性路由/端口探测/回环绑定（REQ-236） |
| 事件契约 | `internal/chat/runner.go`（渲染层归并与时序条在 `web/src/components/TraceDrawer.tsx`——REQ-284：连续 delta 归并思考/正文段+四泳道 waterfall，治逐 token 行） | run_event 全集（tool.call/result 32KB 头尾截断/verify 独立事件/connector.degraded/hook.denied/approval.granted·denied 带 decision_source/run.started 生效策略透出 tool_approval {mode,source}） |
| 装配预览 | `internal/api/handlers_toolpreview.go` | GET /api/agents/{id}/tool-preview（四源合并确定性预览+遮蔽告警，零 MCP 拨号）+ GET /api/hooks |
| 分层侧板 | `web/src/components/AgentSidePanel.tsx` | Harness 页签：运行后端/沙箱资源/工作目录/验证命令/审批策略（三档+豁免+超时）/工具预览卡/hooks 卡（后端读取） |
| 外部 harness 接口 | `internal/inference/acp.go` | dsh 通道面（ACP 长驻 stdio/session 语义转译/权限默认 deny；外部 CLI 后端不走装配链——harness 配置对其不生效） |

## 二、实现现状（已交付能力表）

| 能力 | REQ | 里程碑 | 状态 | 验证 |
| --- | --- | --- | --- | --- |
| 上下文工程（相邻层前置） | REQ-201 | M37 | ✅ | 真机 11/11 |
| 执行面原语+hooks 骨架+verify 背压+重试 | REQ-202/203 | M38 | ✅ | 真机 12 断言 |
| Loop 持久化 checkpoint/续跑/usage | REQ-204 | M39 | ✅（核心） | 真机 9/9 |
| 连接器统一抽象+插件服务+凭据加密 | REQ-214 | M46 | ✅ | 真机 GLM run |
| 分层侧板 Harness/Context/Loop/Graph 页签 | REQ-219 | M50 | ✅ | headless 16/16 |
| dsh 通道面 S1+S2（外部 harness） | REQ-206 | M41 | ✅（S3 可选待领取） | stub ACP 单测 |
| 事件契约（审计事件/截断/独立 verify） | REQ-224 | M52 | ✅ | 单测+真机 |
| 审批三档+豁免+挂起超时+生效透出+装配预览 | REQ-231 | M58 | ✅ | 真机 danger 全链（挂起→approve→granted 审计→文件写出；豁免免审）+headless 7/7 |
| 沙箱纵深（work_dir volume/run_command/出网） | REQ-225 | M53 | 📋 触发驱动 | — |
| hooks 可配置化（agent 级开关/自定义 deny 规则） | REQ-232 | M59 | ✅（2026-10-13 M59 领取交付：迁移 046 hooks_json+BuildHookChain agent 级装配+GET /api/hooks 生效链+hooks 卡可配化；run_command 消费绑定 REQ-225② 同轮兑现） | 单测+headless 7/7 smoke/req225/ |
| 审计族 live 透出（审批决议/守卫拦截/验证背压/连接器降级/产物/CLI 运行中进聊天流） | REQ-311 A1 | M115 | ✅ | 真机 connector.degraded live 卡实锤（坏 MCP 即时拒）+UI 16/16 smoke/req311/ |
| 对话执行过程树形化（tool_call_id 栈式调用树〔委派嵌套为子节点〕/live 展开·完成自动收起摘要行/入参单行滚动/分级按钮上浮对话框上方） | REQ-317 | M119 | ✅ | 真机 GLM 三调用链+UI 14/14 smoke/req317/ |
| 对话窗口交互重排（过程显示三档迁头部 ops 行紧邻观测——对话历史上方「看什么」控制归一处/REQ-302 会话伴生选择器迁输入区 chips 行伴生 chip〔头部零本体选择控件〕/移除最大迭代静态标签与 composer 工具条；顺修伴生切换裸 PUT 盲清 title 存量缺陷→patchConv 合并式） | REQ-322 | M124 | ✅ | UI 22/22 smoke/req322/ |
| 对话过程可观测三整理（「过程」单入口面板=显示档+采集档+关系说明〔答「是否重复」〕/伴生沉淀卡归组〔处理中展开、完成自动收起、手动优先〕/执行树叶级收放〔活跃叶子自动展开执行细节、完成自动收起〕；顺修 live 判定工具执行窗口整树反收+委派归组劈组〔subagent 事件劈 tool.call/result 两组〕→整轮一棵树） | REQ-323 | M125 | ✅ | 真机 GLM 项目委派链+UI 21/21 smoke/req323/ |
| 伴生沉淀过程呈现解耦（伴生管线过程事件出聊天流进右上角悬浮胶囊 CompanionCapsule〔与 chat-stop 对角对称；started 出现→增量摘要→终态定格淡出可点回→明细面板〕/companion.hit 保留流内〔本职 vs 旁路分界〕/REQ-323 伴生组退役留回放摘要行/live「运行完成」卡改插助手消息之后对齐回放；**前置件=收尾轮询游标修复**——order=desc 最新窗+sawDone 基线闸，治 limit:50+ASC 恒取最老 50 条的盲区〔60 存量+2 新鲜 API 实锤，修复前实测 197 条会话「对话完等很久不显示、切回来才显示」〕） | REQ-330 | M129 | ✅ | 真机 GLM 伴生链+UI 17/17 smoke/req330/ |
| 对话界面配置归位与会话文件附加（TraceDrawer 去设置化〔显示/治理区退役→过程面板+⋮会话设置；原始 JSON/时序条改轨迹局部开关〕/头部⋮会话设置面板=会话级配置单一入口〔重命名/审批/检索参数/伴生/智能体配置〕方块侧板钮退役/发送框收敛〔模型 chip 退役模型归智能体、伴生迁⋮、/ 触发快速配置、回形针附件上传入工作目录随消息发送〕/智能体默认工作目录〈DB 同级 agent-files/{id}，客户端数据目录内〉） | REQ-336 | M136 | ✅ | UI 20/20 smoke/req336/ |
| 对话式本体构建工具组（ontochat_start/turn/save/export_ttl 四件——包装构建平面 /api/ontochat/* REST：guided/quick 双模式+202 job 工具内轮询同 ONTOCHAT_LLM_TIMEOUT·超时 job_id 续查+save confirm_note 确认纪律与门禁明细透传+export_ttl 草稿 TTL 直出〔24KB 截断+计数+已入库回填指引，导出≠入库〕；Registry 注册**不进任何默认 agent 工具集**、Agent.Tools 显式勾选生效；ontochat_save 入 REQ-231① 危险清单 danger 档同审=审批双保险，其余三件只读/过程类免 danger 档；BUILD_SVC_URL 直连双进程可用+不可达诚实回执；引导段模板落 platform-knowledge/04_本体/02_构建.md） | REQ-354 | M154/M160 | ✅（2026-10-17 领取交付：真机 GLM 全链〔quick 访谈→CQ 列号确认→58.7s 草稿 98.95→save 201+quality-history 双行〕+审批链真机〔all 档 run.interrupted 挂起→resume approve 执行〕+headless 8/8 smoke/req354/；**P2 2026-10-18**：跨动线深链〔ToolLine 深链钮→OntoChat 流程页焦点续作/资产详情〕+审计两列〔tool.WithCaller 上下文→构建平面 source/agent_id〕+export_ttl 工具，真机 GLM 15/15 smoke/req354-p2/） | 单测 7+4 组+22+14 包绿+真机×2+8/8+15/15 |
| 对话时间线按序交错与执行详情完成收起（ZCode 式对话流：applyRunEvent 正文按模型步分段〔closeTailSegment 过程事件到达关段/空段弹出防幽灵气泡，思考卡按步拆分 evKey 步序号〕+RunBlock 整轮归组〔run.started…终态 done 收起一行任务小结〔工具数/委派/思考字数/耗时/tokens，自 __tool_phase 组 calls 统计〕+最终回答外显块下+重展回看；异常态 stopped/interrupted/error/verify_failed 平铺不收起；纯问答轮不包块〕+toBubbleList 共用管线主区/对比窗格/块体三面同构） | REQ-362 | M162 | ✅ | 真机 GLM+UI 23/23 smoke/req362/（分段 chat-md≥2 实锤/收起摘要/回放同构/显示档联动/审批挂起链） |
| 对话建本体能力产品化（**能力开关**：Agent.OnchatMode 字段+侧板分区〔开关/默认模式/产出清单〕+装配自动并入四工具与手勾并集+**内置纪律段注入**〔instruction segment source=onchat 沿本体指引段先例，防误触发纪律内置〕——引导段手工粘贴退役；**页面渠道流程**：OntoChat 新建入口「在智能体对话中构建」显式选项〔channel-first 会话由智能体 start 保审计归因+预填不代发〕+中途携 session_id 转对话+渠道列表兜底〔已具能力置顶/平台助手/一键新建本体工程师预设〕；**平台助手默认挂载四件**〔92 号 §5.2 原则②默认关修订：显式引导面〕+**本体工程师预设智能体**〔演进位=内置本体专业能力触发驱动〕；产出清单=构建平面 List 增 agent_id 过滤；会话级工具注入不做〔方案 2/4 独立形态退役，与 REQ-336 收敛相逆〕） | REQ-360 | M161 | 📋 已立项待领取（03 §2.54，2026-10-18 开发者三轮拍板合并立项） | — |
| 外部 harness 治理面（页签诚实降维+dsh 权限转审批） | REQ-206 余项 | M41 真机联调轮 | 📋 归 REQ-206 扩注 | — |
| MCP 工具清单装配缓存（每轮消息装配重做 MCP 连接+Initialize+tools/list 的开销与失败面治理〔runner.go:261 每轮 Assemble→assembler.go:577 facade FetchTools+Agent 级连接器〕；FetchMCPTools 进程内缓存 serverName+URL+认证头指纹键·TTL 60s 起步·失败回退直连，降级/ontology__ 改名/审批面零变化；观察项=tool annotations/2025-06-18 structured output） | REQ-356 | M156 | 📋 已立项待领取 | — |
| 执行期超时统一入口+连接器周期探测 | 51 号 W6 | — | ⏳ 需求池观察条目 | — |

## 二.5 体检结论归档（51 号，2026-10-01 清理轮吸收；原档已清理，本节为唯一留存）

| # | 体检结论（2026-10-01 时点） | 归并终局 |
| --- | --- | --- |
| P-H1 | 审批粒度两档过粗（off/all，read-only 也挂起；GLM todo_write 先写习惯放大摩擦） | ✅ REQ-231① 三档 danger（M58） |
| P-H2 | 生效策略不可见（agent 级×会话级装配期合并，对话内无显示） | ✅ REQ-231④ 生效透出（M58） |
| P-H3 | 外部 CLI 后端时 Harness 页签静默失效 | REQ-206 扩注（51 号 W5 定案：UI 标注+保存校验，随 M41 真机轮） |
| P-H4 | 运行时工具清单无确定性预览（四源遮蔽仅 debug 快照可见） | ✅ REQ-231⑤ tool-preview（M58） |
| P-H5 | hooks 说明卡静态文案 | ✅ REQ-231⑥ /api/hooks 后端读取（M58） |
| P-H6 | 审批挂起无超时与触达 | ✅ REQ-231③ 超时自动 deny（M58）；主动触达残留（55 号 Loop 开放问题 3） |
| A-H1 | 沙箱执行世界与文件原语割裂（docker/k8s 零 volume 实证） | REQ-225①（M53 触发驱动） |
| A-H2 | 治理动作无结构化审计事件 | ✅ REQ-224③④（M52：hook.denied/approval.granted·denied） |
| A-H3 | tool.result 落库无截断 | ✅ REQ-224⑤ 32KB 头尾截断（M52） |
| A-H4 | hooks 硬编码不可扩展；fetchGuard 只护 http_fetch | REQ-232（M59 触发驱动；cmdGuard 绑 REQ-225②） |
| A-H5 | 执行期超时/重试编码期固定 | ⏳ W6 需求池观察条目 |
| A-H6 | 连接器运行期健康无事件化 | ✅ REQ-224 connector.degraded（M52）；周期探测在 W6 |

（39 号全局体检的 harness 相关项：A-2 事件契约未治理、A-4 沙箱纵深——同上归并终局。）

## 三、开放问题与诚实边界

1. **mcp 直通连接器保守全审**——danger 档无法静态判定外部 MCP 工具读写性，全部视为危险（read-only 免审仅覆盖内置与 oo/ontology 前缀）。
2. **挂起超时仅在恢复重入时判定**——挂起中无后台时钟（免常驻进程），后端重启后超时视角丢失。
3. **work_dir 零 volume 绑定**——inprocess 与 docker/k8s 两执行世界文件语义割裂，待 REQ-225/M53 触发（触发条件=出现真实「跑命令改代码并验证」本地任务诉求）。
4. **hooks 不可配置**——进程内固定注册表，agent 级开关与自定义 deny 规则待 REQ-232/M59 触发。
5. **外部 CLI 后端旁路**——inference_backend 为外部 CLI 时 harness 配置全部不生效，侧板降维标注随 M41 真机联调轮（51 号 W5 定案：UI 标注+保存校验都做）。
6. **装配预览不含本体 facade 与会话形态工具**——onto_* 随对话挂载、todo_write 仅会话内，预览为能力面口径（诚实注记在端点响应）。

## 四、维护约定

- 本档为 harness 域**新增演进的默认落点**：新 REQ 立项/交付后更新「实现现状」表与「开放问题」；里程碑状态以 docs/02 §12 为权威，本表只做域内速览。
- 51 号体检档内容已由本档与 REQ-231/224 交付吸收，按调研预研收录规则待整理轮归档；38 号保持历史档不动。
- 理论补充（五层模型、外部 harness 对比）继续沉淀 36/37/38 号体系，不在本档重复。
