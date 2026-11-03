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

- [ ] 行为级改动已领 REQ 号：`git fetch origin` 后查 `docs/18_REQ编号注册表.md` 最新号，取序号段 max+1 + 本机 4 位随机后缀（如 `openssl rand -hex 2` → `REQ-606-a3f9`）——完整串即唯一 ID，并发撞序号段无需顺延（详见 118 号方案 §3.2）；bug 修复/文档改进等非行为级改动可跳过
- [ ] 改动面所属模块 Go 测试全绿（backend / ontology-service / runtime-manager / pkg/）
- [ ] 前端改动：`cd web && npx tsc --noEmit && npm run build` 绿（无前端改动可跳过）
- [ ] `cd backend && go test ./internal/doccheck` 绿；钩子已启用：`git config core.hooksPath hooks`
- [ ] 新 SQLite 迁移位于 `backend/internal/store/migrations/`（数字前缀，`IF NOT EXISTS`）
- [ ] 并发面改动（TurnLoop / delegate / runner）：附 `go test -race ./internal/chat/` 结果一行
- [ ] 前端动线改动：按 `docs/20_回归冒烟清单.md` 对应模块节冒烟；截图留本地 `smoke/`（勿 `git add -f` 入库）
- [ ] commit 只含实际修改的文件路径（禁 `git add .`）

> 领号规则人人相同（内外零差别）：fork 同步 main 后按上方规则自领即可，正式账本行写完整串；交付回写按 [onto-dev skill](.agents/skills/onto-dev/SKILL.md) 清单（小修可最小回写：归属档迭代表一行 + `docs/20` 对应行；维护者合入时补漏）。
