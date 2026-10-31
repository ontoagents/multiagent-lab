<div align="center">

<img src="web/public/logo.svg" width="110" alt="OntoAgents logo" />

# OntoAgents —— 智能体构建平台

**构建智能体，也构建它们的本体，全部本地运行。**

[English](README.md) | [简体中文](README.zh-CN.md)

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)
![Storage](https://img.shields.io/badge/Storage-SQLite-003B57?logo=sqlite&logoColor=white)
![MCP](https://img.shields.io/badge/MCP-native-8A2BE2)
![Platform](https://img.shields.io/badge/platform-Windows_%7C_macOS_%7C_Linux-lightgrey)

</div>

**面向专业个人的工作学习助手**（工程师/教师/合规/研究者/医生等知识密集型专业个人）——以本体为知识底座：知识不是一堆文档，而是**可查询、可审计、可出题的结构化资产**；自托管部署——本地客户端为默认形态、服务端自部署为并行方向，数据/密钥/知识不出个人掌控（Go + CloudWeGo Eino/ADK + React + SQLite），围绕双核心组织能力：

- **智能体工程** —— 对话式智能体工作台，执行过程完全可观测，支持多智能体协作与知识 / 技能 / 本体增强。
- **本体工程** —— 本体（知识图谱）全生命周期：构建 → 资产管理 → 运行引擎 → 消费与审计。

一切本地运行：数据存于本地 SQLite，模型 API Key 经 AES-256-GCM 加密存盘，除你自行配置的模型服务外无外部依赖。

## 目录

- [✨ 核心特性](#-核心特性)
- [🧭 探索详情](#-探索详情)
- [🚀 快速开始](#-快速开始)
- [💻 开发模式](#-开发模式前端热更新)
- [🧱 技术栈](#-技术栈)
- [🗂 目录结构](#-目录结构)
- [📚 文档索引](#-文档索引)
- [🧭 发展前景](#-发展前景)
- [🙏 致谢](#-致谢)
- [❓ 常见问题](#-常见问题)
- [🤝 参与贡献](#-参与贡献)
- [📜 许可证](#-许可证)

## ✨ 核心特性

- **执行过程全程可观测** —— 工具调用、深度思考、子智能体委派、知识召回、本体查询以事件卡逐帧呈现，可导出重放，每次回答有据可查。
- **多智能体协作** —— Agent-as-Tool / Transfer 编排、委派时间线可见；2~4 窗格对比模式同问不同配置横向对照。
- **本体即知识底座** —— 构建 → 资产 → 运行 → 消费与审计全生命周期；回答可溯源到结构化知识，而非文本碎片。
- **AI 辅助构建必过质量门禁** —— 六条构建路径（手动建模 / OntoChat 对话式 / CQ 驱动演化 / 由知识库构建 / CSV 灌装 / 导入合并）；LLM 生成内容经评分与自修复后才入库。
- **CQ 回归基准** —— 能力问题升级为可重跑的测试资产：覆盖度量化 + `check_answerable` 可回答性预检。
- **RAG + GraphRAG 双子模块** —— 向量检索 + 自研轻量 KG 后端（图谱浏览 / 多跳检索 / 社区摘要问答）；伴生本体随每轮对话成长。
- **协议级开放** —— 每个 Agent 都是 MCP Server；MCP + A2A 接入外部智能体；执行后端可插拔（进程内 / Docker / Kubernetes）。
- **单机与桌面** —— 一条命令启动、零 Python 运行时依赖；Windows / macOS 安装包；数据、密钥、本体不出个人掌控。

每条详情在下节[探索详情](#-探索详情)折叠展开；模块级导读见 [platform-knowledge/](platform-knowledge/README.md)。

## 🧭 探索详情

<details>
<summary><b>🔍 智能体工程：过程可观测的对话工作台</b></summary>

### 智能体工程：过程可观测的对话工作台

- **执行过程全程可见** —— SSE 流式回复之上，工具调用、深度思考、子智能体委派、知识召回、本体查询以来源着色的事件卡逐帧呈现；可展开 JSON、调试原始事件、导出并重放——每一次回答都有据可查。
- **多智能体协作** —— 项目编排多个 Agent（Agent-as-Tool / Transfer），委派时间线可见；2~4 窗格对比模式支持同问不同配置（模型 / 知识库 / 本体 / 技能 / 温度 / 提示词覆盖）横向对照。
- **知识·技能·本体三路增强** —— 文档上传 → 切分 → 向量化 → 检索召回，GraphRAG 子模块覆盖图谱浏览、多跳检索与社区摘要问答；技能包注入带预览与会话级启停；**伴生本体**在对话收尾自动抽取知识候选，人工确认后并入图谱与检索源——智能体越用越「懂」你的领域。
- **开放集成** —— 每个 Agent 本身就是一个 MCP Server（`/mcp` 端点），Claude Desktop / Cursor 等任意 MCP 客户端可直连对话；执行后端同接口可插拔：进程内装配（默认）/ Docker 沙箱（每 Agent 独立容器）/ Kubernetes Pod。

</details>

<details>
<summary><b>🕸 本体工程：从构建到审计的全生命周期</b></summary>

### 本体工程：从构建到审计的全生命周期

- **多条构建路径，AI 生成必过质量门禁** —— 自定义建模、OntoChat 对话式引导、CQ 驱动演化、由知识库构建、CSV 灌装等多路径并存；LLM 生成内容经质量门禁评分与自修复后才能入库。
- **论文级方法内化** —— OntoChat 论文的「CQ 抽取 → 净化 → 分析 → 测试」四步与 OntoExtend 论文的「检索 → 生成 → 集成」三段式均已按论文实现并经真机回归；**能力问题（CQ）从一次性产物升级为可重跑的回归基准**——会话级测试站、快筛 + SPARQL 精判双轨、术语 / 属性层三级覆盖量化。
- **多形态资产管理** —— 版本快照与 diff 对照、源码视图、SPARQL 工作台、三维力导向图 + WebVOWL 语义对照视图、导入合并审查向导、发布状态机；导入的有损转换如实出报告。
- **运行平面** —— Oxigraph / Fuseki 双引擎方案管理（含 OWL 推理开关）、引擎一键安装、装载对账硬闸（源 / 引擎三元组计数比对，失配即中止）、多本体 per-graph 装载隔离。
- **消费与审计** —— TTL / 文本双源 KG 检索观测台 + 决策审计全程留痕——本体不只是建模产物，更是可消费、可追责的运行资产。

</details>

<details>
<summary><b>🛠 工程化与产品形态</b></summary>

### 工程化与产品形态

- **单机可部署、一切本地** —— 一条命令启动（`run-dev.sh`），不引入 Python 运行时依赖；数据、密钥、知识、本体全部留在本机。
- **桌面客户端** —— Windows / macOS 安装包（Tauri 2 壳）：系统托盘、单实例、服务整树回收、引擎与 sidecar 首启一键安装。
- **平台知识随包内置** —— 产品设计、技术原理、模块导读、调研依据页内即读；能力边界诚实标注（如 KG 增强检索为轻量实现）。
- **质量资产随仓维护** —— 回归冒烟清单 + headless UI 冒烟套件入库，每个交付轮次可验证、可回放。

</details>

## 🚀 快速开始

### 源码运行

Linux / macOS：

```bash
./run-dev.sh          # Windows: .\run-dev.ps1
# 浏览器访问 http://localhost:8080
```

首次启动自动初始化 SQLite 数据库（`backend/data/platform.db`）并预置 DeepSeek 模型连接。

### 配置模型（必做）

打开**设置 → 模型管理**，编辑 **DeepSeek（预置）**，填入 API Key 后点「测试连接」；也可新增任意 OpenAI 兼容 / Anthropic 兼容连接（自定义 base_url + model）。Key 仅存本地，不会上传。

### 桌面安装包

壳自动拉起本地服务并打开平台窗口：

- Windows —— 见 [deploy/windows/README.md](deploy/windows/README.md)
- macOS —— 见 [deploy/macos/README.md](deploy/macos/README.md)

Docker Compose / Helm / 环境变量见[部署指南](platform-knowledge/01_整体设计/16_部署与运行.md)。

## 💻 开发模式（前端热更新）

```bash
cd backend && go run ./cmd/backend   # 终端 1（:8080）
cd web && npm run dev               # 终端 2（:5173，API 代理到 8080）
```

## 🧱 技术栈

| 层 | 选型 |
| --- | --- |
| 后端 | Go 1.25+ · CloudWeGo Eino/ADK · mark3labs/mcp-go · SQLite（可选 Qdrant 向量库） |
| 前端 | React 18 · TypeScript · Vite · Ant Design 6 · Ant Design X · zustand · 3d-force-graph |
| 协议 | REST + SSE（流式）· MCP（消费 + 服务化双向） |

## 🗂 目录结构

```
backend/             Go 主平台后端（:8080，同时托管前端静态资源）
ontology-service/    本体构建平面（:8091，多形态资产 / 质量门禁）
runtime-manager/     本体运行平面（:8090，引擎方案管理）
web/                 React 18 + Vite + TS 前端
apps/desktop/        桌面客户端（Tauri 壳前端，Windows / macOS）
pkg/                 共享 Go 模块（SQLite 迁移器 / 本体 spec / SPARQL 守护）
hooks/               Git 钩子（pre-commit doccheck 门禁；启用一次：git config core.hooksPath hooks）
wrappers/            外部智能体接入包装（如 AgentScope A2A）
data/ logs/          本地运行数据与日志（gitignore，首次启动生成）
website/             项目官网（GitHub Pages；推送 main 自动部署）
platform-knowledge/  平台知识（产品设计 / 技术原理 / 模块导读，应用内渲染）
docs/                需求与设计文档（事实源，见下方索引）
seeds/learning/      技能包与示例本体内容单源
deploy/              Docker Compose + Helm + 桌面端构建脚本
tools/ research/ smoke/   工具脚本、调研记录、冒烟测试套件
```

## 📚 文档索引

| 文档 | 内容 |
| --- | --- |
| [docs/01_智能体_需求文档_PRD.md](docs/01_智能体_需求文档_PRD.md) | 智能体需求事实源（REQ 全表 + 迭代记录） |
| [docs/02_智能体_技术方案设计.md](docs/02_智能体_技术方案设计.md) | 技术方案（架构 / API / SSE 协议 / §12 里程碑状态） |
| [docs/03_本体_需求文档.md](docs/03_本体_需求文档.md) · [docs/04_本体_方案设计.md](docs/04_本体_方案设计.md) | 本体模块需求与方案 |
| [docs/11_知识库_需求文档.md](docs/11_知识库_需求文档.md) · [docs/12_知识库_方案设计.md](docs/12_知识库_方案设计.md) | 知识库模块需求与方案 |
| [docs/15_开源项目及论文登记簿.md](docs/15_开源项目及论文登记簿.md) | 开源项目与论文台账（本项目全部外部参考的全局登记） |
| [docs/18_REQ编号注册表.md](docs/18_REQ编号注册表.md) | REQ 编号唯一分配权威 |
| [docs/20_回归冒烟清单.md](docs/20_回归冒烟清单.md) | 回归冒烟清单 |
| [platform-knowledge/](platform-knowledge/README.md) | 模块导读（产品设计 / 技术原理 / 调研，按模块组织） |
| [platform-knowledge/01_整体设计/17_产品_信息架构与界面设计.md](platform-knowledge/01_整体设计/17_产品_信息架构与界面设计.md) | 界面 / 信息架构口径事实源 |
| [platform-knowledge/02_智能体/27_外部MCP客户端直连验证包.md](platform-knowledge/02_智能体/27_外部MCP客户端直连验证包.md) | Claude Desktop / Cursor 直连配置 |
| [AGENTS.md](AGENTS.md) | AI 编码助手项目引导 |

方向性概览见下节[发展前景](#-发展前景)；需求状态细节以 **docs/02 §12** 与 **docs/18** 为准——本 README 有意不重复维护状态。

## 🧭 发展前景

项目已从教学演示演进为**可生产使用的工程化产品**：单机可部署、双平台桌面安装包、质量门禁与回归冒烟体系完备。定位已锚定**面向专业个人的工作学习助手**（Q-23，2026-10-28），后续沿双核心与助手产品面推进，方向性概览如下（具体需求行、优先级与交付状态以 docs/02 §12 与 docs/18 为准，部分方向为触发驱动）：

| 方向 | 演进内容 |
| --- | --- |
| **本体可信与零丢失** | 导入 / 运行 / 导出全链路零丢失对账：导入归一化与捕获率哨兵、未投影三元组全量余量账本、未编辑本体运行时直挂原文（已立项）；变更影响分析、双时序版本化、确定性身份键与构建产出证据分级（已立项，触发驱动） |
| **本体质量评估体系** | CQ 回归基准已落地（会话级测试站、快筛 + SPARQL 精判双轨、覆盖量化）；下一步向**资产级分层评估**演进——借鉴 CQ4OE 基准的术语 / 属性 / 公理 / 层级 / CQ 覆盖多维指标体系 |
| **知识图谱与检索增强** | KG 社区检测升级（Louvain）与多跳证据收集；「pre-filter → ranking」混合检索形态；伴生本体时间维度与智能体记忆纵深 |
| **智能体运行时** | 长任务跨会话推进与成本护栏；工作流编排（sequential / parallel / loop，触发驱动）；沙箱执行纵深（gVisor / Kata 强隔离备选、K8s 调度） |
| **交付形态与生态** | 桌面端持续打磨（CI 自动打包、自动更新等）；MCP facade 工具面扩展；权威本体库目录扩充与在线词表检索 |
| **个人知识闭环与产品面** | 助手统一入口（「项目→工作区」显示层改名随轮落地）；**笔记本**模块（REQ-495）作为自产成果文档资产层——引用回流知识库、沉淀入本体；本体驱动的出题·批改·掌握度闭环；跨会话记忆与冷启动建模体验——均为需求池触发驱动项（[docs/01 §9](docs/01_智能体_需求文档_PRD.md)） |

## 🙏 致谢

OntoAgents 站在开源社区与学术工作的肩膀上——直接依赖按各自许可证使用；方法论借鉴遵循「**借鉴思想、自研实现**」。以下清单按本项目的**架构分层**（直接使用）与**借鉴域**（重要参考）分组；完整台账（含关联需求号、评估结论与落选留档）见 [docs/15_开源项目及论文登记簿.md](docs/15_开源项目及论文登记簿.md)。

### 直接使用——按架构分层

**智能体引擎（Go 后端）**

| 项目 | 许可 | 用途 |
| --- | --- | --- |
| [CloudWeGo Eino / Eino ADK](https://github.com/cloudwego/eino) | Apache-2.0 | 智能体编排主骨架：ReAct 智能体、多智能体（Agent-as-Tool / Transfer）、流式对话 |
| [CloudWeGo Hertz](https://github.com/cloudwego/hertz) | Apache-2.0 | HTTP 框架：REST API、SSE 流式、前端静态资源托管 |
| [eino-ext](https://github.com/cloudwego/eino-ext) | Apache-2.0 | Eino 生态组件：OpenAI / Claude 模型通道、embedding、MCP 工具消费 |
| [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) | MIT | MCP 服务化基座：Agent `/mcp` 端点与本体 `onto_*` facade |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | MIT | 纯 Go SQLite 驱动（零 CGO，单机零外部依赖） |
| [santhosh-tekuri/jsonschema](https://github.com/santhosh-tekuri/jsonschema) | MIT | spec_json 结构校验 |
| [gonum](https://github.com/gonum/gonum) | BSD-3 | 规划集成：KG 社区检测（Louvain） |

**本体平面（构建 / 运行）**

| 项目 | 许可 | 用途 |
| --- | --- | --- |
| [rdflib](https://github.com/RDFLib/rdflib) | BSD-3 | Python sidecar（`tools/rdf-sidecar`）：OWL/RDF 解析、TTL 导出与回读校验 |
| [pySHACL](https://github.com/RDFLib/pySHACL) | Apache-2.0 | SHACL 约束校验（sidecar 可选组件，质量门禁业务约束层） |
| [Oxigraph](https://github.com/oxigraph/oxigraph) | MIT / Apache-2.0 | 默认 SPARQL 1.1 存储引擎（单二进制，运行平面托管拉起） |
| [Apache Jena Fuseki](https://github.com/apache/jena) | Apache-2.0 | 带 OWL 推理开关的 SPARQL 服务（运行平面可选引擎） |

**前端（React）**

| 项目 | 许可 | 用途 |
| --- | --- | --- |
| [Ant Design](https://github.com/ant-design/ant-design) · [Ant Design X](https://github.com/ant-design/x) | MIT | 前端组件库与 AI 对话组件 |
| [ReactFlow](https://github.com/xyflow/xyflow) | MIT | 本体结构视图画布（spec_json 直渲） |
| [3d-force-graph](https://github.com/vasturiano/3d-force-graph) · [react-force-graph-3d](https://github.com/vasturiano/react-force-graph-3d) | MIT | 本体三维力导向浏览 |
| [WebVOWL](https://github.com/VisualDataWeb/WebVOWL) | MIT | 本体 TBox 标准可视化对照视图 |
| [CodeMirror 6](https://github.com/codemirror/dev)（[@uiw/react-codemirror](https://github.com/uiwjs/react-codemirror)） | MIT | spec / JSON / SPARQL 编辑器（语法高亮、行内校验、文本对照） |
| [react-diff-viewer-continued](https://github.com/afzalsayed96/react-diff-viewer-continued) | MIT | 版本 diff 分栏对照 |

**桌面与交付**

| 项目 | 许可 | 用途 |
| --- | --- | --- |
| [Tauri 2](https://github.com/tauri-apps/tauri) | MIT / Apache-2.0 | Windows / macOS 桌面客户端壳（系统托盘、单实例） |

### 重要参考（借鉴思想，自研实现）

**本体方法论与工具链**（含引导使用——「引导执行」构建路径平台外使用、产物回流导入）

| 项目 / 来源 | 链接 | 借鉴情况 |
| --- | --- | --- |
| **OntoChat** | [GitHub](https://github.com/King-s-Knowledge-Graph-Lab/OntoChat) · [论文](https://arxiv.org/abs/2403.05921) | 对话式本体构建路径蓝本：用户故事 → CQ → 聚类 → 测试工作流与提示词模板；CQ 抽取 / 净化 / 分析已按论文分步实现，覆盖测试站落地为 CQ 回归基准 |
| **OntoExtend** | [GitHub](https://github.com/dersuchendee/OntoExtend) · [论文](https://arxiv.org/abs/2607.17963) | CQ 驱动本体扩展三段式（Retriever → Extender → Integrator）与检索口径（本体元素级序列化 + 句向量 top-k）；已按论文实现 CQ 驱动演化路径 |
| **CQ4OE**（OEG-UPM） | [GitHub](https://github.com/oeg-upm/cq4oe-benchmark) · [论文](https://arxiv.org/abs/2609.26029) | LLM 本体生成评估基准方法论：CQ 覆盖三级度量、多方法术语对齐 pipeline——资产级质量分层评估蓝本 |
| OWLAPY | [GitHub](https://github.com/dice-group/owlapy) | Python LLM 本体构建框架：生成-校验循环与提示词设计参考 |
| Protégé / WebProtégé（引导） | [Protégé](https://github.com/protegeproject/protege) · [WebProtégé](https://github.com/protegeproject/webprotege)（BSD / Apache-2.0） | 业界标准本体建模器（桌面 / 在线协作），导出 OWL/TTL 回流导入 |
| ROBOT（引导） | [GitHub](https://github.com/ontodev/robot)（Apache-2.0） | 命令行本体处理：模板批量生成 / merge / subset |
| OOPS!（引导） | [oops.linkeddata.es](https://oops.linkeddata.es)（Apache-2.0） | 本体建模陷阱扫描（34 类常见错误），质量门禁对照维度 |
| ELK / HermiT（引导） | [ELK](https://github.com/liveontologies/elk-reasoner) · [HermiT](https://github.com/phillord/hermit-reasoner)（LGPL） | OWL 推理器（ELK 快 / HermiT 全量），逻辑一致性校验 |

**知识系统与检索**

| 项目 / 来源 | 链接 | 借鉴情况 |
| --- | --- | --- |
| ontology-on-snowflake（Snowflake-Labs） | [GitHub](https://github.com/Snowflake-Labs/ontology-on-snowflake) | Flattened GraphRAG 概念画像预计算 → 伴生检索「概念画像检索单元」的直接依据；其基准四档数据与画像文档结构为设计输入 |
| HelixDB · Helix Foundry | [helix-db](https://github.com/HelixDB/helix-db) · [helix-foundry](https://github.com/HelixDB/helix-foundry) | 「pre-filter → ranking」混合检索形态、base+overlay+audit 三层编辑与证据分级借鉴（不引入引擎——非 RDF 生态，与 spec→TTL→SPARQL 主线不符） |
| semantica | [GitHub](https://github.com/semantica-agi/semantica) | 三层架构、GraphRAG 混合检索思路、PROV-O 决策溯源叙事——消费与审计面自研轻量实现的对照样本（不接入运行时） |
| Open Ontologies | [GitHub](https://github.com/fabio-rovai/open-ontologies) | 判词纪律、生命周期治理（plan / apply / drift）、双时序等机制深研，沉淀为调研档与自研借鉴清单（双轨集成已退役） |
| OpenBKN | [GitHub](https://github.com/openbkn-ai/bkn-foundry) | 企业级本体平台（对标 Palantir Foundry）对照参考：知识包化、本体检索、受管演进步骤化等设计层借鉴 |
| Graphiti / Zep | [GitHub](https://github.com/getzep/graphiti) | 智能体时序知识图谱记忆标杆：时间感知边、增量演化、混合检索——伴生本体二期参照 |
| LangChain / LlamaIndex | [LangChain](https://github.com/langchain-ai/langchain) · [LlamaIndex](https://github.com/run-llama/llama_index) | 跨生态对照：作外部 MCP client 消费同一 facade，验证开放集成 |

**产品与学习赛道参照**

| 项目 / 来源 | 链接 | 借鉴情况 |
| --- | --- | --- |
| **DeepTutor**（HKUDS） | [GitHub](https://github.com/HKUDS/DeepTutor) · [论文](https://arxiv.org/abs/2604.26962) | Agent-native 个人学习工作台（40k★/9 个月）——产品叙事参照与 **Q-23 定位三修正的直接输入**：场景锐利>引擎通用性；其学习闭环（出题·批改·掌握度）结构借鉴，差异化走**本体驱动**（出题覆盖度保证+概念子图掌握度 vs 纯 LLM 直出题）；冷启动建模成本=胜负手。调研详见 [91 号对标档](platform-knowledge/01_整体设计/91_DeepTutor对标分析_个人学习助手赛道_20261029.md) |
| Microsoft Ontology-Playground | [GitHub](https://github.com/microsoft/Ontology-Playground) | 同定位本体学习站：渐进式领域路径、示例本体库、成就激励等学习中心设计对照 |

### 论文与标准

<details>
<summary><b>论文与标准</b></summary>

| OntoChat: a Framework for Conversational Ontology Engineering using Language Models（ESWC 2024） | [arXiv 2403.05921](https://arxiv.org/abs/2403.05921) | 对话式本体工程四步工作流（用户故事 → CQ → 聚类 → 测试）与从业者调查 |
| Improving Ontology Requirements Engineering with OntoChat and Participatory Prompting | [arXiv 2408.15256](https://arxiv.org/abs/2408.15256) | 参与式提示协议（用户故事引导提示策略） |
| OntoChat Assistant for User Story Generation in Ontology Engineering | [ACM DL 10.1145/3810943](https://doi.org/10.1145/3810943) | 经设计验证的用户故事提示引导框架 |
| OntoExtend: A Framework for Requirement-driven and Scalable Ontology Extension with LLMs | [arXiv 2607.17963](https://arxiv.org/abs/2607.17963) | 需求驱动扩展三段式、检索口径实验与三重评估体系 |
| Ontology extension by online clustering with LLM agents（Frontiers in Big Data 2024） | [DOI 10.3389/fdata.2024.1463543](https://doi.org/10.3389/fdata.2024.1463543) | 零样本流式本体扩展路线参考 |
| CQ4OE: A Benchmark for Assessing LLM-assisted Ontology Generation | [arXiv 2609.26029](https://arxiv.org/abs/2609.26029) | CQ 对齐评估基准：多维指标体系与 CQ 覆盖三级度量 |
| MASEO: Multi-Agent System for Explainable Ontology Generation（LLMs4KG 2026） | [PDF](https://dgarijo.com/papers/maseo-camera-ready.pdf) | 四阶段多智能体生成管线（生成 → 语法 → 一致性 → 陷阱检查），构建管线编排参考 |
| LLMs for Ontology Engineering: A Systematic Literature Review（Semantic Web Journal） | [DOI 10.1177/22104968261465514](https://doi.org/10.1177/22104968261465514) | LLM 三角色（engineer / expert / evaluator）系统综述 |
| IDEA2: Expert-in-the-loop Competency Question Elicitation | [arXiv 2604.01344](https://arxiv.org/abs/2604.01344) | 半自动 CQ 引出工作流与 provenance 追踪 |
| NeOn Methodology（Suárez-Figueroa et al., Springer 2012） | —— | 本体工程生命周期场景化方法论，七阶段构建模型的源头之一 |
| Ontology Development 101（Noy & McGuinness, 2001） | [Stanford](https://protege.stanford.edu/publications/ontology_development/ontology101.pdf) | 本体建模入门经典，学习内容包方法论基线教材 |
| Gruninger & Fox: The Design and Evaluation of Ontologies（TOVE, 1995） | —— | 能力问题（CQ）方法论原始出处 |
| ReAct: Synergizing Reasoning and Acting in Language Models | [arXiv 2210.03629](https://arxiv.org/abs/2210.03629) | 智能体推理-行动交织范式（Eino ADK 智能体模式的工程范式依据） |
| From Local to Global: A Graph RAG Approach to Query-Focused Summarization（Microsoft Research） | [arXiv 2404.16130](https://arxiv.org/abs/2404.16130) | GraphRAG 理论源：社区摘要 + 查询聚焦总结 |
| LLM Wiki: a pattern for building personal knowledge bases using LLMs（A. Karpathy） | [Gist](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f) | 「知识写时编译、持续保鲜」范式——知识库与伴生本体写时积累路线的对照物 |
| Model Context Protocol（MCP）规范 | [modelcontextprotocol.io](https://modelcontextprotocol.io) | 全线集成协议：工具消费与 Agent / 本体 facade 服务化双向 |
| RDF 1.1 / OWL 2 / SHACL / SPARQL 1.1 / PROV-O（W3C） | [RDF](https://www.w3.org/TR/rdf11-concepts/) · [OWL 2](https://www.w3.org/TR/owl2-overview/) · [SHACL](https://www.w3.org/TR/shacl/) · [SPARQL 1.1](https://www.w3.org/TR/sparql11-query/) · [PROV-O](https://www.w3.org/TR/prov-o/) | 数据模型、表达力基线、约束校验、查询语言与溯源本体的标准依据 |

</details>

## ❓ 常见问题

<details>
<summary><b>对话报「运行前装配失败」</b></summary>

未配置可用模型连接——去「设置 → 模型管理」填 Key。

</details>

<details>
<summary><b>换端口</b></summary>

`./run-dev.sh 9090` 或 `.\run-dev.ps1 -Port 9090`。

</details>

<details>
<summary><b>数据库重置</b></summary>

停止服务后删除 `backend/data/` 目录（会清空所有对话与配置）。

</details>

<details>
<summary><b>知识库检索不可用</b></summary>

embedding 连接未配置或索引未就绪——知识库页有状态徽标与引导；DeepSeek 不含 embedding，需另配向量服务（如硅基流动 BGE、千帆 embeddings-v1）。

</details>

## 🤝 参与贡献

请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md)——其中定义了两条贡献通道：**bug 修复 / 文档 / 小功能走轻量通道**（无需领 REQ 编号，需求登记与文档回写由维护者合入时代管），较大功能先开 issue 对齐方案再动手。Issue 与 PR 中英文皆可。

- 报 bug 请用 [issue 模板](.github/ISSUE_TEMPLATE/)；带 `good first issue` / `help wanted` 标签的 issue 可直接认领。
- 安全漏洞：遵循 [SECURITY.md](SECURITY.md)——私密上报，勿开公开 issue。
- 参与本项目请遵守[行为准则](CODE_OF_CONDUCT.md)。
- `docs/` 内需求与设计文档为事实源；回归验证遵循 [docs/20_回归冒烟清单.md](docs/20_回归冒烟清单.md)；AI 编码助手请先阅读 [AGENTS.md](AGENTS.md)。

## 📜 许可证

[Apache-2.0](LICENSE)
