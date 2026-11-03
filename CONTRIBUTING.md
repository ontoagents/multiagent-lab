# 贡献指南（CONTRIBUTING）

感谢关注 OntoAgents——面向专业个人的工作学习助手（以本体为知识底座，单机自部署）。

> **English speakers welcome.** Issues, PRs and comments in English are absolutely fine — replies may be bilingual. Deep documents under `docs/` (handbook / research / learning) are Chinese-first **by design** (a deliberate trade-off to keep maintenance cost on the product); README, this guide and the code are the English-adjacent surface.
>
> 参与本项目请遵守[行为准则](CODE_OF_CONDUCT.md)；**安全漏洞请勿开公开 issue**，走 [SECURITY.md](SECURITY.md) 的私密上报通道。

## 贡献流程（人人相同，内外零差别）

> 本仓是**单一权威仓**：main 在这里，维护者与外部贡献者走同一套流程，无特殊通道。

| 改动类型 | 流程 |
| --- | --- |
| bug 修复、文档改进、小功能（非行为级） | 直接用 [issue 模板](.github/ISSUE_TEMPLATE/) 开 issue 或提 PR，无需领 REQ 编号 |
| 行为级改动（新功能/行为变更/跨模块） | 与维护者同一套流程：①同步 main → ②查 `docs/18_REQ编号注册表.md` 领号（规则见下节，并发安全免协调）→ ③开发+交付回写 → ④PR |

带 `good first issue` / `help wanted` 标签的 issue 可直接领取（评论认领后再动手，避免撞车）。

## 开发环境

- Go 1.25+、Node 20+
- 一条命令启动：`./run-dev.sh`（Windows：`.\run-dev.ps1`；backend :8080 + web :5173 dev server；本体双平面服务按需拉起）
- **启用 pre-commit 门禁（clone 后必须执行一次，否则钩子不生效）**：

  ```bash
  git config core.hooksPath hooks   # 之后每次 commit 自动跑 doccheck
  ```

- 部署/环境变量/安全边界：`docs/handbook/01_整体设计/16_部署与运行.md`；仓库结构速览见 [README 目录结构](README.zh-CN.md#-目录结构)——`docs/` 根级编号档=需求与进度区，子区 `docs/handbook/` 现状手册（页面「平台知识」主树）、`docs/research/` 过程调研、`docs/learning/` 学习资产。

## 提交前必查

1. **测试**：改动面所属模块的 Go 测试全绿（`cd backend && go test ./internal/...`；ontology-service / runtime-manager / pkg/ 各自 `go test ./...`）；
2. **前端**：`cd web && npx tsc --noEmit && npm run build` 绿；
3. **文档门禁**：`cd backend && go test ./internal/doccheck` 绿（pre-commit 钩子自动跑）——校验需求编号引用、文档头部版本与迭代表一致、AGENTS 窗口预算等；
4. **迁移**：新 SQLite 迁移放 `backend/internal/store/migrations/`（数字前缀.sql），CREATE TABLE 建议 `IF NOT EXISTS`（doccheck 幂等提示）；backend 迁移机制走 `pkg/migrate`；
5. **并发面改动**（TurnLoop/delegate/runner）：附 `go test -race ./internal/chat/` 结果一行；
6. **前端动线改动**：按 [docs/20_回归冒烟清单.md](docs/20_回归冒烟清单.md) 对应模块节做 headless 冒烟；截图留本地 `smoke/`（已 gitignore，勿 `git add -f` 入库）。

## 需求编号约定（并发安全领号，人人相同）

- **领取规则**（完整版见 [118 号方案](docs/research/08_调研预研/118_开源协作_单一权威仓与并发领号机制_20261103.md) §3.2）：①`git fetch origin` 核对 main 最新 `docs/18`；②序号段 = 当前最大 seq+1；③后缀 = 本机 4 位随机（如 `openssl rand -hex 2` → `REQ-606-a3f9`）——**完整串即唯一 ID，并发撞序号段无需顺延**（后缀保证唯一）；④正式账本行（18/19/迭代表）写完整串，行文可缩写；
- 里程碑号随批同后缀（`M<seq>-<rand4>`，与批主 REQ 同串）；存量 REQ-1~604 纯序号永久有效；
- 开放需求机读队列见 `docs/19_REQ状态索引.md`；
- 需求详录落归属档（01 智能体/03 本体/11 知识库）迭代表 + `docs/20` 冒烟清单两处（G-7 单源口径）——外部贡献的小修可最小回写（归属档迭代表一行+20 号对应行），维护者合入时补漏；
- 现状手册文档（`docs/handbook/`）发生语义级变更时同步对应模块导读页（AGENTS 纪律 7）；`docs/research/` 为过程调研区，方案档随行为级批次落位。

## AI 代理开发工作流（skill）

本仓随库附带项目级 skill：[`.agents/skills/onto-dev/SKILL.md`](.agents/skills/onto-dev/SKILL.md)——把「需求立项领号 → 方案档落位 → 实现纪律 → 交付回写 → 文档四区归区」整套流程蒸馏为代理可执行的操作手册，供维护者与使用 AI 代理的外部协作者对齐开发流程：

- **zcode / 其他支持 `.agents/skills/` 约定的工具**：clone 后自动发现；对话涉及立项、交付回写、文档归区时自动触发，也可显式调用（`/onto-dev`）；
- **Claude Code**：可直接读 `.agents/skills/onto-dev/SKILL.md`，或复制到 `.claude/skills/onto-dev/` 获得同款自动发现；
- **其他工具 / 不使用代理**：把 SKILL.md 当作流程 runbook 直接阅读即可——它就是本仓「一套逻辑」的最短完整版。

规则权威仍是 [AGENTS.md](AGENTS.md) 硬性纪律（skill 为其操作蒸馏；规则变更先落 AGENTS.md，skill 随轮跟进）。skill 对所有贡献者同等适用——领号、交付回写、文档归区一套流程内外零差别。

## PR 口径

- commit message 概述变更要点（feat/fix/docs/eval + 模块前缀）；
- 只 add 实际修改的文件路径（禁 `git add .`）；
- PR 描述按 [.github/PULL_REQUEST_TEMPLATE.md](.github/PULL_REQUEST_TEMPLATE.md) 自检清单填写；
- CI（`.github/workflows/ci.yml`）跑三平面 Go 测试 + web tsc/build——绿后合并。
