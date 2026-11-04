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

- [ ] 新增能力/改变既定行为：复用已有 REQ 或按 [需求登记](../.agents/skills/onto-dev/references/requirements.md) 领号（恢复既定行为的修复、文档、测试、开发配置整理可标不适用）
- [ ] Go 改动：受影响包测试通过；扩展范围与改动风险相符
- [ ] 前端改动：`cd web && npm run build` 通过；官网使用 `website/` 自己的构建（无相关改动可标不适用）
- [ ] `cd backend && go test -count=1 ./internal/doccheck` 通过
- [ ] 有数据库变更：已获授权，迁移放对应服务迁移目录，编号核对无冲突
- [ ] 并发面改动：附受影响包 `go test -race` 结果
- [ ] 前端动线改动：按 `docs/20_回归冒烟清单.md` 对应模块节冒烟；截图留本地 `smoke/`（勿 `git add -f` 入库）
- [ ] commit 只含实际修改的文件路径（禁 `git add .`）

> 仅填写适用项；列出实际执行的验证与未验证项。交付回写按 [onto-dev](../.agents/skills/onto-dev/SKILL.md) 的对应参考；远程操作权限见 [AGENTS.md](../AGENTS.md)，本模板不授予推送或合并权限。
