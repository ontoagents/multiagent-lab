<div align="center">

<img src="web/public/logo.svg" width="110" alt="AgentLab logo" />

# AgentLab

**Build agents. Build their ontologies. All locally.**

[English](README.md) | [简体中文](README.zh-CN.md)

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)
![Storage](https://img.shields.io/badge/Storage-SQLite-003B57?logo=sqlite&logoColor=white)
![MCP](https://img.shields.io/badge/MCP-native-8A2BE2)
![Platform](https://img.shields.io/badge/platform-Windows_%7C_macOS_%7C_Linux-lightgrey)

</div>

**AgentLab** is a single-machine deployable **agent-building platform** built with Go (CloudWeGo Eino/ADK), React and SQLite. It is organized around two cores:

- **Agent engineering** — a conversational agent workbench with fully observable execution, multi-agent collaboration, and knowledge / skill / ontology augmentation.
- **Ontology engineering** — a full ontology (knowledge-graph) lifecycle: build → asset management → runtime engines → consumption & audit.

Everything runs locally: data lives in a local SQLite database, model API keys are AES-256-GCM encrypted on disk, and the only external dependency is the LLM provider(s) you configure.

## Contents

- [✨ Features](#-features)
- [🚀 Quick Start](#-quick-start)
- [💻 Development Mode](#-development-mode)
- [🧱 Tech Stack](#-tech-stack)
- [🗂 Repository Layout](#-repository-layout)
- [📚 Documentation](#-documentation)
- [🧭 Roadmap](#-roadmap)
- [❓ FAQ](#-faq)
- [🤝 Contributing](#-contributing)
- [📜 License](#-license)

## ✨ Features

| Area | Highlights |
| --- | --- |
| Agent workbench | SSE streaming replies with Markdown rendering; the whole execution process is observable — tool calls, deep thinking, sub-agent delegation, knowledge recall and ontology queries appear as color-coded event cards (expandable JSON, raw-event debugging, export & replay); inline configuration chips and quick model switching |
| Comparison mode | Ask the same question in 2–4 panes with different configurations (model / knowledge base / ontology / skill / temperature / prompt override) side by side |
| Multi-agent collaboration | Projects orchestrate multiple agents (Agent-as-Tool / Transfer) with a visible delegation timeline |
| Knowledge augmentation | Document upload → chunking → embedding → retrieval (Qdrant optional, SQLite fallback); a GraphRAG sub-module adds graph browsing, multi-hop retrieval, extraction governance and community-summary QA |
| Skills | Skill packs (prompt instructions + tool whitelist + resources) injected into agents, with injection preview and per-session toggles |
| Ontology build | Multiple build paths (manual, OntoChat guided authoring, OntoExtend, build-from-knowledge-base, CSV import, …); LLM-generated content must pass a quality gate with self-repair |
| Ontology assets & runtime | Multi-form asset management with version diff, source view, SPARQL workbench, 3D graph + WebVOWL views; Oxigraph / Fuseki engine profiles with one-click engine install; MCP facade exposes `onto_*` / `sparql_query` tools |
| Companion ontology | A dynamic thin ontology: after each conversation, a side-channel extracts knowledge candidates; confirmed entries merge into the session graph and retrieval sources |
| Consumption & audit | A consumption-side observatory (KG retrieval over TTL / text sources) with a full decision audit trail |
| Model management | Multi-provider, multi-model connections (OpenAI-compatible / Anthropic protocols), connection testing, auto-discovery, vendor presets, per-dimension token usage stats |
| Open integration | Agents are exposed as MCP servers — any MCP client (Claude Desktop, Cursor, …) can connect via the `/mcp` endpoint |
| Pluggable execution | In-process assembly (default) / Docker sandbox (one container per agent) / Kubernetes Pod backend behind the same interface |
| Platform knowledge | Product design / technical principles / module guides shipped with the app and rendered in-place |

## 🚀 Quick Start

### Run from source

Linux / macOS:

```bash
./run-dev.sh          # Windows: .\run-dev.ps1
# open http://localhost:8080
```

First launch automatically initializes the SQLite database (`backend/data/platform.db`) and creates a preset DeepSeek model connection.

### Configure a model (required)

Open **Settings → Model Management**, edit **DeepSeek (preset)**, fill in your API key and click **Test Connection** — or add any OpenAI-compatible / Anthropic-compatible connection (custom base_url + model). Keys are stored locally only, never uploaded.

### Desktop installers

The shell starts the local services and opens the platform window:

- Windows — see [deploy/windows/README.md](deploy/windows/README.md)
- macOS — see [deploy/macos/README.md](deploy/macos/README.md)

For Docker Compose, Helm and environment variables, see the [deployment guide](platform-knowledge/01_整体设计/16_部署与运行.md).

## 💻 Development Mode (frontend hot reload)

```bash
cd backend && go run ./cmd/backend   # terminal 1 (:8080)
cd web && npm run dev               # terminal 2 (:5173, proxies API to :8080)
```

## 🧱 Tech Stack

| Layer | Choices |
| --- | --- |
| Backend | Go 1.22+ · CloudWeGo Eino/ADK · mark3labs/mcp-go · SQLite (optional Qdrant vector store) |
| Frontend | React 18 · TypeScript · Vite · Ant Design 6 · Ant Design X · zustand · 3d-force-graph |
| Protocols | REST + SSE (streaming) · MCP (both consuming and serving) |

## 🗂 Repository Layout

```
backend/             Go main platform backend (:8080, also hosts the web UI)
ontology-service/    Ontology build plane (:8091, multi-form assets / quality gate)
runtime-manager/     Ontology runtime plane (:8090, engine profile management)
web/                 React 18 + Vite + TS frontend
apps/desktop/        Desktop client (Tauri shell frontend, Windows / macOS)
website/             Project website (GitHub Pages; auto-deploys on push to main)
platform-knowledge/  In-app platform knowledge (product design / principles / module guides)
docs/                Requirements & design documents (source of truth, see index below)
seeds/learning/      Single source for skill packs & sample ontology content
deploy/              Docker Compose + Helm + desktop build scripts
tools/ research/ smoke/   Utilities, research notes, smoke-test suites
```

## 📚 Documentation

| Document | Content |
| --- | --- |
| [docs/01_智能体_需求文档_PRD.md](docs/01_智能体_需求文档_PRD.md) | Agent requirements (REQ registry + iteration log) |
| [docs/02_智能体_技术方案设计.md](docs/02_智能体_技术方案设计.md) | Technical design (architecture / API / SSE protocol / §12 milestone status) |
| [docs/03_本体_需求文档.md](docs/03_本体_需求文档.md) · [docs/04_本体_方案设计.md](docs/04_本体_方案设计.md) | Ontology module requirements & design |
| [docs/11_知识库_需求文档.md](docs/11_知识库_需求文档.md) · [docs/12_知识库_方案设计.md](docs/12_知识库_方案设计.md) | Knowledge-base module requirements & design |
| [docs/15_开源项目及论文登记簿.md](docs/15_开源项目及论文登记簿.md) | Open-source projects & papers registry |
| [docs/18_REQ编号注册表.md](docs/18_REQ编号注册表.md) | REQ number allocation authority |
| [docs/20_回归冒烟清单.md](docs/20_回归冒烟清单.md) | Regression smoke-test checklist |
| [platform-knowledge/](platform-knowledge/README.md) | Module guides (product design / principles / research, per module) |
| [platform-knowledge/01_整体设计/17_产品_信息架构与界面设计.md](platform-knowledge/01_整体设计/17_产品_信息架构与界面设计.md) | UI / information-architecture source of truth |
| [platform-knowledge/02_智能体/27_外部MCP客户端直连验证包.md](platform-knowledge/02_智能体/27_外部MCP客户端直连验证包.md) | Connecting Claude Desktop / Cursor |
| [AGENTS.md](AGENTS.md) | Guide for AI coding assistants |

Roadmap and requirement status live in **docs/02 §12** and **docs/18** — they are intentionally not duplicated here.

## ❓ FAQ

<details>
<summary><b>Chat fails with a pre-assembly error</b></summary>

No usable model connection is configured — add your API key in Settings → Model Management.

</details>

<details>
<summary><b>Change the port</b></summary>

`./run-dev.sh 9090` or `.\run-dev.ps1 -Port 9090`.

</details>

<details>
<summary><b>Reset the database</b></summary>

Stop the service and delete `backend/data/` (this wipes all conversations and configuration).

</details>

<details>
<summary><b>Knowledge retrieval unavailable</b></summary>

An embedding connection is missing or the index is not ready; the knowledge page shows a status badge and onboarding. DeepSeek has no embedding API, so configure a separate vector service (e.g. SiliconFlow BGE, Qianfan embeddings-v1).

</details>

## 🤝 Contributing

- Requirements and design documents in `docs/` are the source of truth; new requirements must be registered in [docs/18_REQ编号注册表.md](docs/18_REQ编号注册表.md) before implementation.
- Regression checks follow [docs/20_回归冒烟清单.md](docs/20_回归冒烟清单.md).
- AI coding assistants should read [AGENTS.md](AGENTS.md) first.

## 📜 License

[Apache-2.0](LICENSE)
