# 贡献指南（CONTRIBUTING）

感谢关注 OntoAgents——面向专业个人的工作学习助手（以本体为知识底座，单机自部署）。

> **English speakers welcome.** Issues, PRs and comments in English are absolutely fine — replies may be bilingual. Deep documents under `docs/` (handbook / research / learning) are Chinese-first **by design** (a deliberate trade-off to keep maintenance cost on the product); README, this guide and the code are the English-adjacent surface.
>
> 参与本项目请遵守[行为准则](CODE_OF_CONDUCT.md)；**安全漏洞请勿开公开 issue**，走 [SECURITY.md](SECURITY.md) 的私密上报通道。

## 贡献流程（人人相同，内外零差别）

> 本仓是**单一权威仓**：main 在这里，维护者与外部贡献者走同一套流程，无特殊通道。

| 改动类型 | 流程 |
| --- | --- |
| 恢复既定行为的 bug 修复、文案、等价重构、测试或开发配置整理 | 沿已有需求或记录变更，无需新增 REQ |
| 新增产品能力或改变既定行为 | 核对 main 与已有需求 → 未覆盖时按 `docs/18_REQ编号注册表.md` 领号 → 实现、验证与回写 → 准备 PR |

本地修复不以提交或推送为前提。代理操作的权限见 [AGENTS.md](AGENTS.md)；推送、创建/合并 PR 等远程写操作需明确授权，不能从本贡献流程推定授权。

带 `good first issue` / `help wanted` 标签的 issue 可直接领取（评论认领后再动手，避免撞车）。

## 开发环境

- Go 1.25+、Node 20+
- 一条命令启动：`./run-dev.sh`（Windows：`.\run-dev.ps1`；backend :8080 + web :5173 dev server；本体双平面服务按需拉起）
- **首次配置开发环境时启用 pre-commit 门禁**（环境配置需用户授权，普通任务不自动修改；未安装钩子也须手动执行文档检查）：

  ```bash
  git config core.hooksPath hooks   # 之后每次 commit 自动跑 doccheck
  ```

- 部署/环境变量/安全边界：`docs/handbook/01_整体设计/16_部署与运行.md`；仓库结构速览见 [README 目录结构](README.zh-CN.md#-目录结构)——`docs/` 根级编号档=需求与进度区，子区 `docs/handbook/` 现状手册（页面「平台知识」主树）、`docs/research/` 过程调研、`docs/learning/` 学习资产。

## 提交前必查

1. **Go 改动**：先验证受影响包，按调用面与风险扩到相关模块；无需为纯文档修改运行全部服务测试；
2. **前端改动**：`cd web && npm run build`（脚本已包含 TypeScript 检查）；官网改动在 `website/` 运行自己的构建；
3. **文档门禁**：`cd backend && go test -count=1 ./internal/doccheck` 通过（已安装的 pre-commit 钩子会执行）——校验需求引用、版本、文档结构与冲突残留；
4. **迁移**：数据库变更须有授权，新迁移放对应服务目录（backend 为 `backend/internal/store/migrations/`，数字前缀.sql）；按现有迁移机制核对编号与幂等要求；
5. **并发面改动**：对受影响包执行 `go test -race` 并记录结果（如 backend 的 `./internal/chat/`、`./internal/workflow/`）；
6. **前端动线改动**：按 [docs/20_回归冒烟清单.md](docs/20_回归冒烟清单.md) 对应模块节做 headless 冒烟；截图留本地 `smoke/`（已 gitignore，勿 `git add -f` 入库）；
7. **commit 邮箱归属**：`git config user.email` 应为 GitHub 账号**已验证邮箱**或 `<用户名>@users.noreply.github.com`——GitHub 按 commit 作者邮箱归属贡献者，未绑定邮箱的提交无法计入你的贡献者名单（历史提交在你绑定该邮箱后自动回溯归属，无需改写历史）。

## 需求编号约定（并发安全领号，人人相同）

- **领取规则**（完整版见 [118 号方案](docs/research/08_调研预研/118_开源协作_单一权威仓与并发领号机制_20261103.md) §3.2）：①`git fetch origin` 核对 main 最新 `docs/18`；②序号段 = 当前最大 seq+1；③后缀 = 本机 4 位随机（如 `openssl rand -hex 2` → `REQ-606-a3f9`）——**完整串即唯一 ID，并发撞序号段无需顺延**（后缀保证唯一）；④正式账本行（18/19/迭代表）写完整串，行文可缩写；
- 里程碑号随批同后缀（`M<seq>-<rand4>`，与批主 REQ 同串）；存量 REQ-1~604 纯序号永久有效；
- 开放需求机读队列见 `docs/19_REQ状态索引.md`；
- 产品交付详录落归属需求档迭代表 + `docs/20` 验证记录；开发治理变更记治理档，纯笔误无需批量回写；内外贡献者规则相同；
- 产品行为、能力或决策变化时更新对应 handbook 现状档，模块导读与子模块能力表按归属选择；独立方案落 `docs/research/`。细则见 [交付参考](.agents/skills/onto-dev/references/delivery.md)。

## AI 代理开发工作流（skill）

本仓开发 Skill：[onto-dev](.agents/skills/onto-dev/SKILL.md)。仅在处理 REQ 立项、交付回写或文档归区时加载入口，再按当前任务选择参考文件；普通代码阅读或文案小修无需加载。支持 `.agents/skills/` 的工具可发现它；其他工具直接读取同一文件，避免复制出多份规则。

项目权限与完成标准统一见 [AGENTS.md](AGENTS.md)。完成包括实现、运行适用检查、修复失败并重新验证；受环境限制的检查须明确标为未验证，不得以跳过冒充通过。

## PR 口径

- commit message 概述变更要点（feat/fix/docs/eval + 模块前缀）；
- 只 add 实际修改的文件路径（禁 `git add .`）；
- PR 描述按 [.github/PULL_REQUEST_TEMPLATE.md](.github/PULL_REQUEST_TEMPLATE.md) 自检清单填写；
- CI（`.github/workflows/ci.yml`）跑三平面 Go 测试 + web tsc/build——绿后合并。
