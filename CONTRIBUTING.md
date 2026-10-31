# 贡献指南（CONTRIBUTING）

感谢关注 OntoAgents——面向专业个人的工作学习助手（以本体为知识底座，单机自部署）。

> **English speakers welcome.** Issues, PRs and comments in English are absolutely fine — replies may be bilingual. Deep documents under `docs/` and `platform-knowledge/` are Chinese-first **by design** (a deliberate trade-off to keep maintenance cost on the product); README, this guide and the code are the English-adjacent surface.
>
> 参与本项目请遵守[行为准则](CODE_OF_CONDUCT.md)；**安全漏洞请勿开公开 issue**，走 [SECURITY.md](SECURITY.md) 的私密上报通道。

## 贡献通道（先选对路径）

| 通道 | 适用 | 流程 |
| --- | --- | --- |
| **轻量通道（推荐入口）** | bug 修复、文档改进、小功能（一两个文件、行为清晰） | 直接用 [issue 模板](.github/ISSUE_TEMPLATE/) 开 issue 或提 PR——**无需自行领 REQ 编号、无需回写 `docs/`**；编号登记与需求档回写由维护者在合入时代管 |
| **常规通道** | 较大功能、行为变更、跨模块影响面 | 先开 issue 描述使用场景与触发条件，与维护者对齐方案后再动手；立项领号由维护者完成 |

带 `good first issue` / `help wanted` 标签的 issue 可直接领取（评论认领后再动手，避免撞车）。

## 开发环境

- Go 1.25+、Node 20+
- 一条命令启动：`./run-dev.sh`（Windows：`.\run-dev.ps1`；backend :8080 + web :5173 dev server；本体双平面服务按需拉起）
- **启用 pre-commit 门禁（clone 后必须执行一次，否则钩子不生效）**：

  ```bash
  git config core.hooksPath hooks   # 之后每次 commit 自动跑 doccheck
  ```

- 部署/环境变量/安全边界：`platform-knowledge/01_整体设计/16_部署与运行.md`；仓库结构速览见 [README 目录结构](README.zh-CN.md#-目录结构)。

## 提交前必查

1. **测试**：改动面所属模块的 Go 测试全绿（`cd backend && go test ./internal/...`；ontology-service / runtime-manager / pkg/ 各自 `go test ./...`）；
2. **前端**：`cd web && npx tsc --noEmit && npm run build` 绿；
3. **文档门禁**：`cd backend && go test ./internal/doccheck` 绿（pre-commit 钩子自动跑）——校验需求编号引用、文档头部版本与迭代表一致、AGENTS 窗口预算等；
4. **迁移**：新 SQLite 迁移放 `backend/internal/store/migrations/`（数字前缀.sql），CREATE TABLE 建议 `IF NOT EXISTS`（doccheck 幂等提示）；backend 迁移机制走 `pkg/migrate`；
5. **并发面改动**（TurnLoop/delegate/runner）：附 `go test -race ./internal/chat/` 结果一行；
6. **前端动线改动**：按 [docs/20_回归冒烟清单.md](docs/20_回归冒烟清单.md) 对应模块节做 headless 冒烟；截图留本地 `smoke/`（已 gitignore，勿 `git add -f` 入库）。

## 需求/编号约定（主要由维护者执行）

- 新功能先查 `docs/18_REQ编号注册表.md` 领号（REQ 编号唯一分配权威），开放需求机读队列见 `docs/19_REQ状态索引.md`——**常规通道由维护者立项时完成，轻量通道贡献者无需自办**；
- 需求详录落归属档（01 智能体/03 本体/11 知识库）迭代表 + `docs/20` 冒烟清单两处（G-7 单源口径）——合入时由维护者代回写；
- 平台知识文档（`platform-knowledge/`）发生语义级变更时同步对应模块导读页（源指针 frontmatter）。

## PR 口径

- commit message 概述变更要点（feat/fix/docs/eval + 模块前缀）；
- 只 add 实际修改的文件路径（禁 `git add .`）；
- PR 描述按 [.github/PULL_REQUEST_TEMPLATE.md](.github/PULL_REQUEST_TEMPLATE.md) 自检清单填写；
- CI（`.github/workflows/ci.yml`）跑三平面 Go 测试 + web tsc/build——绿后合并。
