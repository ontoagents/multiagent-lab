# 贡献指南（CONTRIBUTING）

感谢关注 AgentLab——面向专业个人的工作学习助手（以本体为知识底座，单机自部署）。

## 开发环境

- Go 1.25+、Node 20+
- 一条命令启动：`./run-dev.sh`（backend :8080 + web :5173 dev server；本体双平面服务按需拉起）
- 文档：`platform-knowledge/01_整体设计/16_部署与运行.md`（部署/环境变量/安全边界）

## 提交前必查

1. **测试**：改动面所属模块的 Go 测试全绿（`cd backend && go test ./internal/...`；ontology-service / runtime-manager / pkg/migrate 各自 `go test ./...`）；
2. **前端**：`cd web && npx tsc --noEmit && npm run build` 绿；
3. **文档门禁**：`cd backend && go test ./internal/doccheck` 绿（pre-commit 钩子自动跑）——校验需求编号引用、文档头部版本与迭代表一致、AGENTS 窗口预算等；
4. **迁移**：新 SQLite 迁移放 `backend/internal/store/migrations/`（数字前缀.sql），CREATE TABLE 建议 `IF NOT EXISTS`（doccheck 幂等提示）；backend 迁移机制走 `pkg/migrate`；
5. **并发面改动**（TurnLoop/delegate/runner）：附 `go test -race ./internal/chat/` 结果一行。

## 需求/编号约定

- 新功能先查 `docs/18_REQ编号注册表.md` 领号（REQ 编号唯一分配权威），开放需求机读队列见 `docs/19_REQ状态索引.md`；
- 需求详录落归属档（01 智能体/03 本体/11 知识库）迭代表 + `docs/20` 冒烟清单两处（G-7 单源口径）；
- 平台知识文档（`platform-knowledge/`）发生语义级变更时同步对应模块导读页（源指针 frontmatter）。

## PR 口径

- commit message 概述变更要点（feat/fix/docs/eval + 模块前缀）；
- 只 add 实际修改的文件路径（禁 `git add .`）；
- CI（`.github/workflows/ci.yml`）跑三平面 Go 测试 + web tsc/build——绿后合并。
