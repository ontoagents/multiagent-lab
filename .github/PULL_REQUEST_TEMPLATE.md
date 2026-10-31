<!-- 修复 issue 请在此注明：Fixes #xx -->

## 变更说明

<!-- 做了什么、为什么（What & why） -->

## 变更类型

- [ ] bug 修复
- [ ] 新功能
- [ ] 文档
- [ ] 重构 / 性能
- [ ] 其他

## 自检清单（对应 [CONTRIBUTING.md](../CONTRIBUTING.md)「提交前必查」）

- [ ] 改动面所属模块 Go 测试全绿（backend / ontology-service / runtime-manager / pkg/）
- [ ] 前端改动：`cd web && npx tsc --noEmit && npm run build` 绿（无前端改动可跳过）
- [ ] `cd backend && go test ./internal/doccheck` 绿；钩子已启用：`git config core.hooksPath hooks`
- [ ] 新 SQLite 迁移位于 `backend/internal/store/migrations/`（数字前缀，`IF NOT EXISTS`）
- [ ] 并发面改动（TurnLoop / delegate / runner）：附 `go test -race ./internal/chat/` 结果一行
- [ ] 前端动线改动：按 `docs/20_回归冒烟清单.md` 对应模块节冒烟；截图留本地 `smoke/`（勿 `git add -f` 入库）
- [ ] commit 只含实际修改的文件路径（禁 `git add .`）

> 外部贡献者无需领 REQ 编号、无需回写 `docs/`——合入时由维护者代管（见 CONTRIBUTING「轻量通道」）。
