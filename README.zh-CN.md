<div align="center">

<img src="web/public/logo.svg" width="110" alt="AgentLab logo" />

# AgentLab —— 智能体构建平台

**构建智能体，也构建它们的本体，全部本地运行。**

[English](README.md) | [简体中文](README.zh-CN.md)

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)
![Storage](https://img.shields.io/badge/Storage-SQLite-003B57?logo=sqlite&logoColor=white)
![MCP](https://img.shields.io/badge/MCP-native-8A2BE2)
![Platform](https://img.shields.io/badge/platform-Windows_%7C_macOS_%7C_Linux-lightgrey)

</div>

单机可部署的**智能体构建平台**（Go + CloudWeGo Eino/ADK + React + SQLite），以双核心组织能力：

- **智能体工程** —— 对话式智能体工作台，执行过程完全可观测，支持多智能体协作与知识 / 技能 / 本体增强。
- **本体工程** —— 本体（知识图谱）全生命周期：构建 → 资产管理 → 运行引擎 → 消费与审计。

一切本地运行：数据存于本地 SQLite，模型 API Key 经 AES-256-GCM 加密存盘，除你自行配置的模型服务外无外部依赖。

## 目录

- [✨ 核心能力](#-核心能力)
- [🚀 快速开始](#-快速开始)
- [💻 开发模式](#-开发模式)
- [🧱 技术栈](#-技术栈)
- [🗂 目录结构](#-目录结构)
- [📚 文档索引](#-文档索引)
- [🧭 Roadmap](#-roadmap)
- [❓ 常见问题](#-常见问题)
- [🤝 参与贡献](#-参与贡献)
- [📜 许可证](#-许可证)

## ✨ 核心能力

| 领域 | 说明 |
| --- | --- |
| 对话工作台 | SSE 流式回复（Markdown 渲染）；执行过程完全可观测——工具调用 / 深度思考 / 子智能体委派 / 知识召回 / 本体查询以来源着色的事件卡呈现（可展开 JSON、调试原始事件、导出重放）；输入框内嵌配置开关与模型快捷切换 |
| 对比模式 | 2~4 窗格同问不同配置横向对照（模型 / 知识库 / 本体 / 技能 / 温度 / 提示词覆盖） |
| 多智能体协作 | 项目编排多 Agent（AgentAsTool / Transfer），委派过程时间线可见 |
| 知识增强 | 文档上传 → 切分 → 向量化 → 检索召回（Qdrant 可选，SQLite fallback）；GraphRAG 子模块覆盖图谱浏览、多跳检索、抽取治理与社区摘要全局问答 |
| 技能系统 | 技能包（提示词指令 + 工具白名单 + 资源）注入 Agent，支持注入预览与会话级启停 |
| 本体构建 | 多条构建路径（自定义、OntoChat 引导、OntoExtend、由知识库构建、CSV 灌装等）；LLM 生成必过质量门禁并自修复 |
| 本体资产与运行 | 多形态资产管理、版本 diff、源码视图、SPARQL 工作台、三维浏览 + WebVOWL 对照；Oxigraph / Fuseki 引擎方案管理、引擎一键安装；MCP facade 暴露 `onto_*` / `sparql_query` 工具 |
| 伴生本体 | 动态薄本体：对话收尾旁路抽取知识候选，人工确认后并入会话图与检索源 |
| 消费与审计 | 本体消费侧观测台（TTL / 文本双源 KG 检索）+ 决策审计全程留痕 |
| 模型管理 | 多供应商多模型连接（OpenAI 兼容 / Anthropic 双协议）、连接测试、自动发现、厂商预设、按维度 token 用量统计 |
| 开放集成 | Agent 对外 MCP 服务化——任意 MCP 客户端（Claude Desktop / Cursor 等）经 `/mcp` 端点直连对话 |
| 可插拔执行 | 进程内装配（默认）/ Docker 沙箱（每 Agent 独立容器）/ K8s Pod 后端，同一接口扩展 |
| 平台知识 | 产品设计 / 技术原理 / 模块导读随应用内置、页内即读 |

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
| 后端 | Go 1.22+ · CloudWeGo Eino/ADK · mark3labs/mcp-go · SQLite（可选 Qdrant 向量库） |
| 前端 | React 18 · TypeScript · Vite · Ant Design 6 · Ant Design X · zustand · 3d-force-graph |
| 协议 | REST + SSE（流式）· MCP（消费 + 服务化双向） |

## 🗂 目录结构

```
backend/             Go 主平台后端（:8080，同时托管前端静态资源）
ontology-service/    本体构建平面（:8091，多形态资产 / 质量门禁）
runtime-manager/     本体运行平面（:8090，引擎方案管理）
web/                 React 18 + Vite + TS 前端
apps/desktop/        桌面客户端（Tauri 壳前端，Windows / macOS）
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
| [docs/15_开源项目及论文登记簿.md](docs/15_开源项目及论文登记簿.md) | 开源项目与论文台账 |
| [docs/18_REQ编号注册表.md](docs/18_REQ编号注册表.md) | REQ 编号唯一分配权威 |
| [docs/20_回归冒烟清单.md](docs/20_回归冒烟清单.md) | 回归冒烟清单 |
| [platform-knowledge/](platform-knowledge/README.md) | 模块导读（产品设计 / 技术原理 / 调研，按模块组织） |
| [platform-knowledge/01_整体设计/17_产品_信息架构与界面设计.md](platform-knowledge/01_整体设计/17_产品_信息架构与界面设计.md) | 界面 / 信息架构口径事实源 |
| [platform-knowledge/02_智能体/27_外部MCP客户端直连验证包.md](platform-knowledge/02_智能体/27_外部MCP客户端直连验证包.md) | Claude Desktop / Cursor 直连配置 |
| [AGENTS.md](AGENTS.md) | AI 编码助手项目引导 |

Roadmap 与需求状态以 **docs/02 §12** 与 **docs/18** 为准——本 README 有意不重复维护状态细节。

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

- `docs/` 内需求与设计文档为事实源；新需求实施前必须在 [docs/18_REQ编号注册表.md](docs/18_REQ编号注册表.md) 登记编号。
- 回归验证遵循 [docs/20_回归冒烟清单.md](docs/20_回归冒烟清单.md)。
- AI 编码助手请先阅读 [AGENTS.md](AGENTS.md)。

## 📜 许可证

[Apache-2.0](LICENSE)
