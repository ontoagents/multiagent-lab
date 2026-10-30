# 改名分析报告：从 AgentLab / multiagent-lab 到新名（OntoAgents 可用性评估）

- 日期：2026-10-30
- 范围：项目改名决策——OntoAgents 撞名排查、候选名对比、改名改动面盘点、分层改名策略
- 输入：四份子调研（①OntoAgents 撞名排查〔已经独立复核员逐条验证〕②候选名提案 ③界面可见命名盘点 ④内部命名盘点）+ 权威全仓 grep 计数
- 本报告新增工作：本机复跑 grep 交叉核对（方法与结果见 §4.8），抽查两份盘点 20+ 处行级引用，新发现 1 处两份盘点均未列的命中文件

---

## 1. 结论摘要

**OntoAgents 能用，但有更优候选；改名策略建议「界面必改、部署链分批、数据面不动」三层推进，不为改名而全量翻挖。** 精确复数名 OntoAgents 目前零活跃实体撞名：`ontoagents` GitHub 组织经本机 `git remote -v` 与 GitHub API 双重确认是本项目自有公开镜像（非撞名），npm/PyPI/RubyGems/pkg.go.dev 四处包管理器实跑全空，无同名公司/产品/商标记录，法律与在用产品冲突风险低；但风险定级为「中」——单数 OntoAgent 已有两个活跃学术系统（RPI LEIA Lab 认知架构 + jdm4pku 需求访谈智能体）、复数 OntoAgents 在现行论文正文持续出现（arXiv 5 篇）、双 .com 域名均已失守，组合词占用面在 2025-26 持续扩大，长期代价是学术/开源检索噪音与品牌稀释，而非即时冲突。

要点：

- **可用性**：OntoAgents ✅ 可用（个人自部署项目无法律/产品障碍），风险方向是「噪音增长」不是「撞名实害」；若沿用，行动项=尽快注册复数 .ai/.io 等域名与关联 handle、对外叙事明确与单数 OntoAgent（RPI 认知架构）的区隔。
- **更优候选**：**Ontofolio（中文名「本典」，repo 名可用 ontfolio 或 onto-folio）**——本轮全部查名中唯一两轮初筛均零同名的候选；Onto+folio 直接命名「可查询、可审计、可出题的知识资产册」，与 Q-23 定位（产品面=个人工作学习助手、知识是结构化资产）对位比引擎层的复数 Agents 更准，避免把叙事重心拉回刚放弃的「多智能体平台」对标。次选 Ontarium、OntoDesk；OntoPilot 为避雷区（四处严重同名）。
- **策略**：界面可见名一轮必改（改动面小、零兼容风险）；部署链（安装包名/helm/compose/仓库 slug/Pages 路径）可选分批，改则必须与 CI 硬编码、用户命令同步；已落数据标识（KG RDF 命名空间 eino-lab、ADK schema 注册键、localStorage 键、设置导出格式串）建议明确不动或仅做兼容读，数据迁移成本大于品牌收益。
- **全量改名的工作量**：约 2~4 人日机械改 + 全量回归；真正 heavy/moderate 难点只有两处（GitHub 仓库与 Pages 路径改名、Windows 数据根改名迁移），其余约 30 文件为 trivial 字面量或 moderate 脚本回归；编译面可用 gofmt 批量重写 import 解决，无 Go module 之外的路径级牵动。

---

## 2. OntoAgents 撞名排查结果

排查时间 2026-10-30，已经独立复核员逐条验证（GitHub API、arXiv API、RDAP、DNS、包管理器、WebFetch 实开页面）。状态标注沿用子调研自报口径：verified=已实核，unconfirmed=未能核实。

### 2.1 精确命中（hits）——3 条，全部 verified

| # | 实体 | 状态 | 要点 | 对本项目的含义 |
|---|---|---|---|---|
| 1 | ontoagents（GitHub 组织） | verified | 本仓 public remote 即 `git@github.com:ontoagents/multiagent-lab.git`（本机 `git remote -v` 实核）；GitHub API 实查 created_at=2026-09-21，组织页 1 个公开仓库 | **本项目自有公开镜像，不是撞名**；反证复数 handle 已由本项目自己持有 |
| 2 | OnTo-Agents / "OntoAgents Information FoodChain"（Stanford，2001） | verified | daml.org 页面实抓：DARPA DAML 语义网计划，PI=Gio Wiederhold，分包 Karlsruhe、技术联系人 Stefan Decker；项目早已终止，无活跃同名实体 | 25 年前学术先例，RDF 文献高被引——语义网社区检索会先见 Stanford，构成搜索噪音底座 |
| 3 | "OntoAgents" 复数在现行论文正文的使用（arXiv 5 篇，2021~2026） | verified | arXiv API 实跑 `all:"OntoAgents"` totalResults=5，含 RPI McShane《Knowledge Engineering in the Long Game of AI》与 HARMONIC 系列及 jdm4pku 2026-05 论文 | 无以此为名的独立实体/产品，但属精确复数字串的最新占用证据，检索噪音在增长 |

### 2.2 近名占用（near misses）——12 条：11 verified + 1 unconfirmed

| # | 实体 | 状态 | 要点 |
|---|---|---|---|
| 1 | OntoAgent（单数，RPI LEIA Lab 认知架构） | verified | leia-lab.com 实抓：OntoSem 深度语义+OntoGen 文本生成，HARMONIC 人机协作机器人底座，©2025 活跃（MAKE 2026 项目）；官方组织 LEIA-Lab 在册；arXiv 2603.26730（2026-03-20）核实存在——**同领域强近名，学术检索噪音最大来源** |
| 2 | jdm4pku/OntoAgent（单数，需求访谈智能体） | verified | 新发现：README 实抓「Ontology-guided Requirements Elicitation Interview Agent」，arXiv 2605.05828（2026-05-07）核实，repo 2 star、2026-02-28 仍有 push——RPI 之外第二个活跃单数同名学术系统，且同在「本体×智能体」语义域 |
| 3 | GitHub 单数同名/近名仓库群（8 仓） | verified | in:name 检索 total_count=8：derekluo/OntoAgent（2026-08 push）、hyverysmart/OntoAgent-Evolve（2026-10-25 push）、ngocthinh09/OntoAgentQA 等 0~2 star 个人级仓库；EibrielInv/OpenOntoAgent 已 archived（2022-07 起休眠）——无产品化，但组合词占用面在扩大 |
| 4 | onto-agent（rinDBeans 开源 MCP 工具） | verified | 仓库级知识图谱/架构治理引擎（AST+Kùzu 内嵌图，10 个 MCP 工具+CLI+REST+Dashboard，Apache-2.0），「活跃放缓」、需本地 pip 安装——已发布的开源产品级近名，同「图谱×Agent」域 |
| 5 | Onto-Agent Methodology（Hadzic 2009，Springer） | verified | Springer 章节活链（303 认证跳转非 404）+检索索引确认；Cambridge CARES 2019 年报以 OntoAgent 作本体属性前缀——组合词是学界通用方法论/命名前缀，独占性弱。诚实边界：章节正文未开读（认证墙） |
| 6 | OntoAgent（单数 GitHub handle） | verified | GitHub API：created_at=2025-05-13，public_repos=1（CQGen-MAS，0 star）——单数 handle 被第三方占用，复数 ontoagents 不受影响 |
| 7 | Ontoagent, INC.（纽约，公司线索） | **unconfirmed** | b2bhint 页面连续两轮 403（Cloudflare），经营内容与活跃度无法核实——单数公司名占用线索维持未证实 |
| 8 | ontoagents.com（复数 .com 域名） | verified | RDAP 实跑：2026-01-10 注册、2027-01-10 到期，nameservers=Afternic（域名交易市场，佐证停放/囤积）；页面零内容——**复数 .com 已失守但无活跃产品** |
| 9 | ontoagent.com（单数 .com 域名） | verified | 新定案（初查存疑项）：RDAP 证实 2023-04-21 注册、Squarespace 注册商、clientDeleteProhibited/clientTransferProhibited——单数 .com 同样不可得 |
| 10 | ontoagent.ai（单数 .ai 域名） | verified | IONOS 停放页（德语「已注册」+营销内容），无实际产品——被占但无内容 |
| 11 | 复数域名可注册面 | verified | DNS 实跑 ontoagents.{ai,io,org,dev,app} 全 NXDOMAIN 未注册——**复数 .ai/.io/.org/.dev/.app 五后缀目前可注册** |
| 12 | 包管理器面 | verified | npm（ontoagents/ontoagent/onto-agents/onto-agent 四查 Not found）、PyPI（四查 404）、rubygems（两查 404）、pkg.go.dev（no matches）——**复数与单数在四大包管理器全空** |

### 2.3 排查覆盖面与裁定

- 已查：git remote、GitHub API（用户/组织/仓库检索）、arXiv API、RDAP Verisign、DNS、npm/PyPI/RubyGems/pkg.go.dev、WebSearch 多轮、WebFetch 实开 8 页。
- **裁定：中**。精确复数名零活跃实体撞名、个人自部署项目可用性成立；但单数双活跃学术系统 + 复数论文用语 + 双 .com 失守 + 组合词占用面扩大，风险方向是**长期学术/开源检索噪音与品牌稀释（增长趋势）**，非法律或在用产品冲突。
- 若沿用 OntoAgents 的行动建议：①尽快注册 ontoagents.{ai,io,…} 域名与关联 handle（当前可注册窗口）②对外叙事明确「OntoAgents（复数）=本体工程×多智能体平台」与单数认知架构 OntoAgent 的区隔 ③接受 .com 不可得，官网用子路径或非 .com 域名。

---

## 3. 候选名对比

预查重口径：均为 WebSearch 初筛（2026-10-30，各 1~2 轮查询），未做包管理器/GitHub API/域名注册局级精查（见 §6 诚实边界）。

| 名字 | 理由 | 预查重结果 | 与 OntoAgents 对比 |
|---|---|---|---|
| **Ontofolio**（中文名建议「本典」；repo 名可用 ontfolio 或 onto-folio） | Onto（本体底座）+ folio（册页/档案夹/作品集）——直接命名「知识资产册」形态，folio 自带编目/页码义，与可审计、可溯源（PROV-O 导出）呼应；「个人知识资产册」即产品面，双核心与产品面一名双得；无 Semantica 类历史包袱 | 两轮初筛零同名（仅「onto FOLIO」等巧合短语），**全部 14 次查名中唯一未见同名者**（不排除未被索引的新项目） | 同押双核心但落点在差异化定位而非引擎层；代价是 folio 对初见者需一句话解释 |
| Ontarium（中文名建议「本匣」） | -arium（馆/匣/容器，aquarium/terrarium 同构）——「一座自容纳的本体知识馆」，与单机自部署、SQLite 自容纳神合，产品人格暖 | 未见软件域同名；最重命中为 2019 年已注销的爱沙尼亚公司 Ontarium Inc OÜ（跨行业） | 撞名面更小、更暖；但词形生造、首读需愣半拍，智能体一侧无载体 |
| OntoDesk（中文不硬配，「本体工作台」可作副题） | Desk=个人知识工作台，直给「单机自部署的个人工作学习助手」产品面，好读好记 | 仅命中 2009 年筑波大学 HCI 论文（无仓库无产品）；与旧平台 oDesk 有字面回声（非同名） | 撞名实害更小、更像个人产品；但 desk 泛词可能被误读为「本体域 helpdesk」，引擎信号缺失 |
| Lyceon（中文名建议「学园」） | Lyceum（亚里士多德学园）——学习源头意象+ontology 的希腊哲学母体双关；-on 尾有「构件」感 | 命中 lyceon.ai——美国早期 AI SAT 辅导创业公司（同 AI+学习邻近域，极早期未融资） | 更独特优雅；但放弃 Onto 前缀后本体底座信号归零，且 AI 教育小公司在先 |
| Ontomate（中文名建议「本伴」） | mate=伴侣/学伴，精准对位「个人工作学习助手」的产品温度；好读好拼 | 命中 RGD（大鼠基因组数据库）OntoMate 文献挖掘工具——**活跃注册研究资源（RRID:SCR_018493）且同「本体+知识工具」域** | 产品人格强；但同域活跃同名在先，SEO 竞争比 OntoAgents 更直接 |
| OntoSage（「本师」慎配——佛教语已有专义，可不配中文名） | sage=智者/师长，正对出题·批改·掌握度闭环的「师者」人格，最贴学习动线 | 命中 suhasdevmane/OntoSage——建筑数字孪生 agentic 平台（LangGraph+RAG+SPARQL），**「本体+智能体+SPARQL」概念域与本仓几乎同构** | 「师者」人格贴；但概念同构撞名最刺眼，且 Sage 是 AI 圈高频泛词双重稀释 |
| OntoPilot（**不建议采用，避雷记录**） | 表面上最「顺」的命名——copilot 语境自带助手感、零解释成本 | 四处严重命中：两个 GitHub 同名项目（其一为本体定义驱动的知识型 LLM agent 框架，与本仓概念几乎相同）+ OntoPilot LLC（亚利桑那）+ Altilia 产品线 | 第一眼契合度略高，但撞名重灾度全面更高——「看似最顺、实际最不可用」 |

**候选名小结**：推荐序列 Ontofolio > Ontarium ≈ OntoDesk > Lyceon > Ontomate > OntoSage；OntoPilot 避雷。OntoAgents 本身处于「可用但中风险」区间——比 OntoPilot/OntoSage/Ontomate 的同域撞名干净，比 Ontofolio/Ontarium 的零同名嘈杂。

---

## 4. 改名改动面盘点

工作量分级：**trivial**=字面量替换；**moderate**=脚本/配置回归（构建、helm/compose 冒烟、兼容策略）；**heavy**=牵动外部 URL/数据/全仓 import 或需迁移设计。

### 4.1 界面可见（ui-visible）

| 位置 | 现值 | 工作量 |
|---|---|---|
| web/index.html:7 | `<title>AgentLab 智能体构建平台</title>`（已核：web/src 无动态 document.title 覆盖，静态唯一） | trivial |
| web/src/components/GlobalNav.tsx:116-117 | 侧边栏品牌名+副标题（全 web/src 唯一品牌渲染点） | trivial |
| web/src/pages/SettingsPage.tsx:783 | 设置导出默认文件名 `multiagent-lab-settings-<日期>.json` | trivial |
| web/public/logo.svg:2 | 图标注释「MultiAgentLab 应用图标」（README 门面 logo 与 favicon 同源） | trivial |
| website/index.html:11-12 | 静态 SEO meta+title（运行时被 locale.ts SITE_META 覆盖） | trivial |
| website/src/locale.ts:12,36-44 | localStorage 键 `agentlab.locale`（internal，改名丢已持久化语言选择）+双语 SITE_META | trivial（键名见 §5「明确不动」） |
| website/src/App.tsx:140,172 | 顶栏 brand-name+页脚品牌行 | trivial |
| website/src/pages/Home.tsx:290,324,364 | hero H1+「30 秒了解」双语标题 | trivial |
| website/src/pages/Home.tsx:298-306,332-340 | 快速开始命令块：`git clone …/multiagent-lab.git`+`cd multiagent-lab`+`helm install agentlab deploy/helm`（已核 ：306,340） | moderate（须与仓库/部署实际名同步） |
| website/src/pages/Deploy.tsx:15-16,86,147,151,272 | RELEASES_URL/DEPLOY_DOC_URL 常量+克隆命令+Releases 链接（指涉 `ontoagents/multiagent-lab`） | moderate |
| website/src/pages/Deploy.tsx:120,124 | 安装包名文案 `MultiAgentLab_<版本>_setup.exe / _macos_universal.dmg`（双语）——须与 deploy/ 产物名同轮同步 | moderate |
| website/src/pages/Ecosystem.tsx:323,381,390,420,429 | 生态图中心节点 `eino-multiagent-lab`+双语品牌文案 | trivial |
| website/src/pages/Implementation.tsx:475,536 | 正文品牌指涉双语两处 | trivial |
| website/src/components/mockups.tsx:49 | 产品预览 SVG 中的「AgentLab」字样 | trivial |
| website/src/styles.css:2 | CSS 源注释（不渲染） | trivial |
| README.md:3,5,20,184 / README.zh-CN.md:3,5,184 | 中英门面（H1/alt/首段自称/致谢），同轮同步改 | trivial |
| CONTRIBUTING.md:3 | 品牌自称 | trivial |

### 4.2 内部代码（internal-code）

| 位置 | 现值 | 工作量 |
|---|---|---|
| 9 个 go.mod module 路径（已核全部 9 处） | `github.com/xiaoyao/eino-multiagent-lab/{backend,ontology-service,runtime-manager,pkg/migrate,pkg/sparqlguard,pkg/ontology/spec}`（6 处，前缀含 eino-）+ `github.com/xiaoyao/multiagent-lab/deploy/{macos/launcher,windows/launcher,windows/setup}`（3 处，无 eino-——**两套前缀并存**）；go.work 聚合 9 模块（go.work 本身无品牌字符串） | heavy（改则全仓 import 重写，可 gofmt 批量解决） |
| import 引用面 | `github.com/xiaoyao/eino-multiagent-lab/...`：backend 217 文件、ontology-service 79 文件、runtime-manager 26 文件、pkg 3 文件（本机按扩展名复核：216/78/25 个 .go + 各 1 个 go.mod，与盘点吻合，命中面纯 Go 代码无嵌入式资产） | heavy（机械改+全量回归） |
| backend/internal/tool/askhuman.go:20-21、approval.go:22-23 | ADK schema 注册键 `eino_multiagent_lab_tool_ask_human_info/_ask_human_state/_approval_info/_approval_state`（全仓下划线变体仅此 4 处）——可能随运行状态持久化，改后旧 resume 失配 | moderate（兼容策略） |
| backend/internal/tool/mcp.go:184、stdio.go:73、api/handlers_mcp.go:43 | MCP 对外协议自述 `eino-multiagent-lab`——改即生效、无持久兼容 | trivial |
| backend/internal/a2a/a2a.go:129,133 | A2A agent card `tags:["eino-multiagent-lab","agent"]`+`provider.organization` | trivial |
| backend/internal/api/handlers_settings.go:21,74 | `settingsExportFormat="multiagent-lab-settings"`+导出文件名——若导入侧校验格式串，改名后旧导出文件失配需兼容读取 | moderate |
| backend/internal/companion/sparql.go:21-28 | KG RDF 命名空间 `http://eino-lab/ontology/thin/`、`/graph/`、`/e/`、`/msg/`（4 常量，backend 30 处含测试）——**已入库 Oxigraph/Fuseki 图 URI 与导出 TTL 引用稳定，改名需数据迁移或双读** | moderate~heavy（数据兼容） |
| backend/internal/api/handlers_kg.go:127 | KG 导出 TTL `@prefix onto: <https://eino-multiagent-lab.local/onto/>`——同上属落数据引用面 | moderate |
| backend/internal/companion/ttl.go:80 | TTL 导出注释行（纯文案） | trivial |
| backend/internal/inference/cli_member.go:41、deploy/docker-compose.yml:25 | 沙箱镜像名仅注释/示例，无代码默认值（5 个 Dockerfile grep 品牌 0 命中） | trivial（可不动） |
| apps/desktop/src-tauri/tauri.conf.json:3,5、tauri.macos.conf.json:6、Cargo.toml:2、src/main.rs:5、package.json:2 | Tauri 壳 `productName=MultiAgentLab`+`identifier=com.multiagentlab.desktop`（第三种变体 multiagentlab 无连字符）；Cargo.lock/package-lock 随构建再生 | moderate |
| apps/desktop/src-tauri/src/lib.rs:93,118-121 | 数据根默认目录 `multiagent-lab`+**曾用名一次性迁移先例**（eino-lab→multiagent-lab：旧目录存在且新目录不存在时整体改名，失败沿用旧目录不丢数据）——再迁需复制同一模式 | moderate |
| runtime-manager/internal/engine/fuseki/install_test.go:238,249 | **本次交叉核对新发现（两份盘点均未列）**：Windows 测试夹具路径 `D:\MultiAgentLab\data\engines\rt_x\tdb`——生产代码 `reasoningConfig`（fuseki.go:291）以参数收 dbDir、无品牌常量，纯测试夹具、无功能耦合 | trivial |

### 4.3 构建部署（build-deploy）

| 位置 | 现值 | 工作量 |
|---|---|---|
| .github/workflows/client-release.yml:5,32,81,133 | **发布门禁硬编码仓库 slug** `if: github.repository == 'ontoagents/multiagent-lab'`——仓库名一改 3 个 job 的 if 即失配，release 静默跳过 | moderate |
| .github/workflows/client-release.yml:2-3,72,76,124,128 | 发布产物名 `MultiAgentLab_*_setup.exe / _macos_universal.dmg`（mv/path 通配） | trivial~moderate |
| Windows 安装链 9 文件 | installer.iss:9,16-21,60（exe 名+安装目录 `%LOCALAPPDATA%\Programs\MultiAgentLab`+卸载注册表键+AppPublisher）、build.ps1:46,48,162,231、sfx/installer.cmd:6,11,39、sfx/install-shortcuts.ps1:8-47、sfx/uninstall-core.ps1:9,15,26,31,63、launcher/setup winres.json（本机新点名）、launcher/netdiag.go（本机新点名）、windows/README.md、.gitignore:35——改名涉旧版升级识别与卸载残留 | moderate |
| macOS bundle 7 文件 | Info.plist:5-8（CFBundleName/Executable=MultiAgentLab；**CFBundleIdentifier=com.einolab.MultiAgentLab——bundle id 前缀是更旧的第三层品牌 einolab**）、launcher.sh:2、build.sh（19 行，本机计数最大集中点）、dmg_layout.py、shell/window.applescript、staging-assets/README.txt、macos/README.md——改名涉旧 app 覆盖升级；窗口标题无硬编码品牌 | moderate |
| helm chart 9 文件 | deploy/helm/multiagent-lab/（目录名）+Chart.yaml:2 `name: multiagent-lab`+values.yaml 镜像名/DNS `multiagent-lab-*`/ingress host `multiagent-lab.local`（本机复核：helm 目录内 agentlab 0 命中，「helm release 名 agentlab」仅存在于官网快速开始命令的用户输入侧） | moderate |
| deploy/docker-compose.yml:1,6 | compose 项目名 `name: multiagent-lab`——决定卷/网络前缀（multiagent-lab_* 共 5 卷），改名后旧卷成孤儿需数据迁移 | moderate |
| Windows 数据根 | `%LOCALAPPDATA%\multiagent-lab`（uninstall-core.ps1:31）——**破坏性变更需迁移兼容**（lib.rs 曾用名迁移模式可复制） | moderate~heavy |
| website/vite.config.ts:24,26 | Pages 部署子路径 `base '/multiagent-lab/'`（浏览器地址栏可见）+`__REPO_SLUG__` 缺省 `'ontoagents/multiagent-lab'`（CI 经 WEBSITE_REPO_SLUG 注入自动跟随触发仓） | moderate |

### 4.4 文档（docs）

| 位置 | 现值 | 工作量 |
|---|---|---|
| docs/ + platform-knowledge/ 全目录 | 品牌变体命中 **去重 116 行**（本机 union 实测；权威表 30+38+19+58=145 为两模式行数和、同行双计）——docs 集中在 20 号冒烟清单（9 行）/18 号注册表（9 行）/01 PRD（5 行）/02 方案（3 行） | 按治理纪律处理（见 §5） |

文档面**不做一次性全量改名**：按 AGENTS.md 纪律 6/8（活文档滚动、交付注指针化），品牌定义处（17_产品_信息架构、00_整体设计 定位行等）随改名交付轮优先更新，其余历史行随轮滚动消化；git 历史兜底。

### 4.5 仓库基础设施（repo-infra）

| 位置 | 现值 | 工作量 |
|---|---|---|
| git 双 remote（本机 `git remote -v` 实核） | `origin=git@github.com:xiaoyao/multiagent-lab.git`（开发主仓）+`public=git@github.com:ontoagents/multiagent-lab.git`（发布镜像仓，REQ-327 与开发主仓解耦） | heavy |
| GitHub 仓库 slug ×2 | 本仓 multiagent-lab（牵动 Pages URL `/multiagent-lab/`、CI WEBSITE_REPO_SLUG、全部克隆指南/外链）+发布仓 ontoagents/multiagent-lab（官网 Deploy 页链接与克隆命令可见） | heavy |
| GitHub 发布组织名 `ontoagents` | **候选名 OntoAgents 与该既有组织名直接同串**（本项目自有）——沿用 OntoAgents 则组织/仓库名恰好自洽；改用其他候选名则需裁决组织改名/新建组织/仅改显示名 | 决策项 |
| 本地工作目录名 | `/home/dongkui/workspace/xiaoyao/multiagent-lab`（与 GitHub 仓同名） | trivial~moderate |
| GitHub 仓库改名机制 | GitHub 对仓库 rename 提供 Web 端自动 redirect（git 操作与旧 URL 访问），但硬编码 if 条件（client-release.yml）、克隆文档、Pages base 不会自动跟随 | moderate |

### 4.6 「只改界面名、内部名不动」方案：可行性与漏网风险

**可行性：成立且改动面很小。** web 源码 3 处（index.html 标题/GlobalNav 品牌/SettingsPage 导出文件名）+website 约 34 行字面量（22 agentlab 类+12 multiagent 类，本机逐文件对账全落在 §4.1 所列 8 个文件）+README 门面 7 处+CONTRIBUTING 1 处，一轮可收口；纯文案、零数据兼容风险。

**漏网风险（三类，改前须知）**：

1. **「界面上指涉的事实」不会跟着文案走**：官网 Deploy/快速开始页向用户展示的仓库 slug `multiagent-lab`、安装包名 `MultiAgentLab_*`、helm release 名 `agentlab`、Pages 子路径 `/multiagent-lab/` 是用户会照抄的命令与可见 URL——文案改名而 deploy/ 产物、仓库与部署路径不动，会立刻产生用户可见的 URL/文件名/命令不一致。这些行（Home.tsx:298-306,332-340、Deploy.tsx:15-16,86,120,124,147,151,272、vite.config.ts:24,26）要么保持旧名（与事实一致、与新品牌不一致），要么连动部署链（升级为部分全量改）。
2. **半内部面漏网**：桌面客户端安装名 `MultiAgentLab.exe`/安装目录/注册表键、macOS `MultiAgentLab.app`、MCP/A2A 对外协议自述 `eino-multiagent-lab`（mcp.go:184、a2a.go:129,133）——纯 UI 改后桌面端与协议层仍显旧名；对外接入方（外部 MCP 客户端）可见。
3. **内部一致性**：Go module/import 面仍是 eino-multiagent-lab——不影响用户，但新名上线后代码检索会有新旧两套词汇，文档新档须明确「品牌名 ≠ module 路径」避免混淆。

### 4.7 全量改名的工作量

- **总量**：约 2~4 人日机械改 + 全量回归（内部盘点口径）；真正 heavy/moderate 难点仅两处：①GitHub 仓库与 Pages 路径 `multiagent-lab` 改名（牵动 Pages URL、CI WEBSITE_REPO_SLUG、发布仓与外链）②Windows 数据根 `%LOCALAPPDATA%\multiagent-lab` 改名（需数据迁移兼容，lib.rs 有 eino-lab→multiagent-lab 先例模式可复制）。
- **其余约 30 文件**为 trivial 字面量或 moderate 脚本回归（安装包名/Chart 名/Compose 名改后各跑一次构建与 helm/compose 冒烟即可）。
- **编译面**：无 Go module 之外的路径级牵动；module/import 重写用 gofmt 批量解决，编译风险低。
- **真正的高危不在编译，在数据与升级链**：RDF 已落数据的 eino-lab URI 引用、客户端旧版升级识别/卸载残留、旧导出设置文件导入、resume 状态失配——这些决定「明确不动」清单（§5）。

### 4.8 本机交叉核对结果（权威 grep 计数 × 两份盘点）

**核对方法**（本机实跑，node_modules 与 gitignored 构建产物除外）：
- 逐目录复跑两类计数：窄口径 `grep -rin "agentlab"` / `grep -rin "multiagent-lab"`；宽口径 `grep -rinE "multiagent[-_ ]?lab|multiagentlab"`（含 MultiAgentLab、multiagent_lab 变体）
- union 计数：`grep -rinE "agentlab|multiagent[-_ ]?lab|multiagentlab" docs platform-knowledge | wc -l`
- 文件级分布：deploy/（agentlab 类 89 行逐文件 uniq 对账）、website/src（22 行逐文件对账）、三后端目录按扩展名分布
- 行级抽查 20+ 处两份盘点引用（§4 各表「已核」标记处），另实核 `git remote -v`、go.work、9 个 go.mod、`document.title` 0 命中、helm 目录 agentlab 0 命中

**已对账项（非出入，口径说明）**：

| 项 | 结论 |
|---|---|
| 权威表 multiagent-lab类 口径 | =宽口径正则，逐目录复跑全部精确复现（backend 390、deploy 145、runtime-manager 57、.github 10、docs 38、platform-knowledge 58、website/src 14）✅ |
| 内部盘点 docs「116 处命中」 vs 权威 145 行 | 116 = 去重 union 行数（本机实测 docs+platform-knowledge union 恰为 116）；145 = 两模式行数和（同一行同时含 AgentLab 类与 multiagent 类时双计）——**两种口径都对，非计数错误** ✅ |
| deploy/** agentlab类=89 | 逐文件分布 19+8+7+7+7+6+5+4×3+3×4+2+1×4=89，与权威计数精确吻合 ✅ |
| .github/** 6+10 行 | 全部落在 client-release.yml（6 行 MultiAgentLab 产物名 + 4 行 slug if + 2 行重叠），website-deploy.yml/ci.yml 无字面品牌命中（WEBSITE_REPO_SLUG 走注入），与盘点一致 ✅ |
| 后端三目录命中面 | 纯 .go + go.mod（backend 216+1、ontology-service 78+1、runtime-manager 25+1），无嵌入式文档/yaml 资产，与盘点「import 行为主」口径吻合 ✅ |
| runtime-manager agentlab类=2 行 | 即 install_test.go:238,249（见下「出入」第 1 条） |

**出入清单（盘点漏网/覆盖缺口，如实列出）**：

1. **runtime-manager/internal/engine/fuseki/install_test.go:238,249**——`D:\MultiAgentLab\data\engines\rt_x\tdb` 测试夹具两行，**两份盘点均未列**。内部盘点「全仓 Go 代码未发现独立 AgentLab 字样」对独立 AgentLab 严格成立（此为 MultiAgentLab 变体），但该文件在全量改名改动面上。生产代码 `reasoningConfig`（fuseki.go:291）以参数收 dbDir、无品牌常量——**无功能耦合，trivial**。
2. **deploy 下 5 个文件盘点未点名**（「9 文件/7 文件聚合」未展开到）：deploy/windows/launcher/winres.json、deploy/windows/setup/winres.json（各 1 行）、deploy/windows/launcher/netdiag.go（1 行）、deploy/windows/README.md（7 行）、deploy/macos/README.md（7 行）。
3. **权威 grep 计数表自身覆盖缺口**（两份盘点已覆盖这些位置，属计数表不全而非盘点漏）：无 `web/*.*` 行（web/index.html:7、web/public/logo.svg:2、web/package.json:2 有命中）；无 `apps/**` 行（src-tauri 下 8 文件有命中，内部盘点已列）；无 `pkg/**` 行（3 个 go.mod 各 1 行，内部盘点已列）；无根文件行（README.md 4 行、README.zh-CN.md 3 行、CONTRIBUTING.md 1 行、run-dev.sh/ps1 各 1 行注释，盘点已列）。
4. **构建产物噪音说明**：web/dist/**（gitignored）含内联 platform-knowledge 文档资产、大量品牌命中——非源面、构建再生，改名轮无需触碰；backend/internal/data/（未跟踪 platform.db）0 命中，未污染 backend=390 的计数。根 internal/、logs/ 为空目录（0 文件 0 命中）；hooks/、wrappers/、website/public/、AGENTS.md、go.work、eval-sample.txt 均 0 命中，与盘点一致。

---

## 5. 建议的分层改名策略

### 第一层：必改（P0——一轮收口，纯文案、零兼容风险）

- web：index.html 标题、GlobalNav 品牌名+副标题、SettingsPage 导出文件名前缀、logo.svg 注释
- website：App/Home/Ecosystem/Implementation/Deploy 的品牌文案与 SITE_META、mockups、styles.css 注释、双包名（package.json）
- README.md / README.zh-CN.md / CONTRIBUTING.md 门面
- 文档品牌定义处：17_产品_信息架构、00_整体设计 定位行（随轮）
- ⚠️ 唯一注意点：快速开始/Deploy 页的命令块与安装包名文案（§4.6 风险 1）——若部署链不动，这些行暂保持旧名并加「历史名」注，或与第二层同轮做

### 第二层：可选（P1——分批，每批带兼容策略与冒烟）

| 批 | 内容 | 前置/兼容 |
|---|---|---|
| B1 桌面安装链 | MultiAgentLab.exe/.app/bundle id/Tauri identifier/安装目录/注册表/数据根→新名 | 复制 lib.rs 曾用名迁移模式（旧目录存在且新目录不存在时整体改名，失败沿用旧目录）；产物名与官网 Deploy 文案、client-release.yml 产物行同轮 |
| B2 仓库与发布面 | 开发主仓+发布仓 slug、Pages base、WEBSITE_REPO_SLUG 缺省值、client-release.yml 4 处 if、官网全部克隆命令/链接 | GitHub rename 自动 redirect 兜底旧链接；CI if 必须同轮改否则 release 静默跳过；Pages 自定义域评估 |
| B3 helm/compose | chart 目录名+values 镜像名/DNS/host、compose 项目名 | 改后各跑一次 helm/compose 冒烟；compose 旧卷孤儿可接受或手工 `docker volume` 迁移 |
| B4 协议自述与格式串 | MCP 自述、A2A card、settingsExportFormat、导出文件名 | 格式串保留旧值兼容读取（读旧写新）；MCP/A2A 改即生效无持久兼容 |
| B5 Go module 路径 | 9 个 go.mod + 597 处 import | gofmt 批量重写；独立一批、全量测试回归后落；建议与 B2 绑定决策（module 路径含仓库名语义） |
| B6 文档滚动消化 | docs/platform-knowledge 余量 116 行中非定义处 | 沿纪律 6/8 随轮消化，不一次性翻挖 |

### 第三层：明确不动（P2/永不——数据与持久化标识，迁移成本 > 品牌收益）

- **KG RDF 命名空间** `http://eino-lab/...` 4 常量与 `eino-multiagent-lab.local` @prefix：已入库 Oxigraph/Fuseki 图 URI 与导出 TTL 引用稳定——改则需数据迁移或双读；建议不动（URI 是标识符不是品牌），最多在导出注释说明历史
- **ADK schema 注册键** `eino_multiagent_lab_tool_*` 4 键：改后旧 resume 状态失配；不动
- **localStorage 键** `agentlab.locale`：改=丢用户语言偏好；不动（或加一次性迁移读）
- **已归档产物/历史 tag/旧版本安装包**：保持原名，GitHub 历史不改
- **`.com` 域名**：双双失守且复数 .com 在 Afternic 停放（囤积），不追；用 .ai/.io 或子路径

### 推荐执行序

若采纳 Ontofolio：P0 一轮（界面+门面）→ 占位 ontfolio/onto-folio 域名与 GitHub handle/org（决策项：现有 ontoagents 组织是保留作发布镜像还是改名）→ B1/B2 择机 → B3~B6 随轮。若坚持 OntoAgents：P0 同上 → 立即注册 ontoagents.{ai,io,org,dev,app}（当前 NXDOMAIN 可注册窗口）→ 部署链同上但组织名自洽无需改。

---

## 6. 诚实边界

1. **本报告的网络查证未重跑**：第 2、3 节直接采信四份子调研的自报核验结果（其声称已经独立复核员逐条验证）；本报告新增的只有本机 grep/文件抽查交叉核对（§4.8）。子调研自报的未能核验项如实转记：crates.io API 403、Semantic Scholar 429（两轮）、DBLP 反爬墙、HuggingFace API 连接失败、Docker Hub API 返回非 JSON、USPTO TESS 未直查（以 WebSearch 代理）、Springer 章节正文在认证墙后、ontoagents.com 的 AWS IP 解析未复核（以 Afternic NS+空页替代佐证）、Ontoagent INC 经营内容 403（unconfirmed）。
2. **商标数据库未查**：USPTO/WIPO/EUIPO 未直查，「未见注册记录」仅是 WebSearch 代理结论，≠商标不存在；采纳前如需商业级确认应做正式商标检索。
3. **域名可注册性判定是浅层的**：ontoagents.{ai,io,org,dev,app} 的「可注册」仅基于 DNS NXDOMAIN，未查注册局 RDAP/保留字/溢价价格；.com 判定有 RDAP 实跑支撑。结论有时效性（2026-10-30），注册窗口随时可能关闭。
4. **候选名仅 WebSearch 初筛**：7 个候选名各 1~2 轮查询，未做 GitHub API/npm/PyPI/pkg.go.dev/域名精查——OntoAgents 本体做了四包管理器实跑而候选名没有，**Ontofolio「零同名」的置信度低于 OntoAgents 的排查深度**；采纳任一候选名前应按 OntoAgents 同等深度补一轮排查（含 ontfolio/onto-folio 的域名与 handle）。
5. **改名动线中的未决决策**（本报告不代拍板）：现有 ontoagents GitHub 组织的去留（沿用 OntoAgents 则自洽、改用他名则需裁决）；中文正式名（「本典」「本匣」等均为建议）；Go module 路径是否随品牌改（两套前缀 eino-/无 eino- 并存的统一问题）。
6. **盘点复核的覆盖度**：两份盘点的行级引用抽查 20+ 处全部吻合，但未逐条复核全部约 50 处引用；deploy 聚合项（「9 文件/7 文件聚合」）中 5 个文件未点名展开（§4.8 出入第 2 条）；权威 grep 计数表的覆盖缺口已列（§4.8 出入第 3 条）。grep 计数为行数非「处数」（同行多命中计 1）。
7. **OntoAgent 单数学术噪音不可通过改名根除**：RPI LEIA Lab 认知架构活跃且同域，无论最终用 OntoAgents、Ontofolio 还是其他名，「本体×智能体」语义域的学术检索噪音长期存在，只能靠品牌叙事与 SEO 占位对冲。
