# 安全策略（Security Policy）

> English summary: please do **not** open public issues for security vulnerabilities — use GitHub's **Security → Report a vulnerability** (private advisory) or contact the maintainer privately. Expect an acknowledgement within 72 hours.

## 支持范围

- `main` 分支最新提交（源码运行）
- 最新发布的 Windows / macOS 桌面安装包（GitHub Releases）

更早版本不单独维护，建议升级。

## 如何报告漏洞

**请勿以公开 issue 形式报告安全漏洞。**

优先使用 GitHub 私密通道：仓库页 **Security → Report a vulnerability**（私有 Advisory）。报告请包含：

1. 影响面：主平台 backend（:8080）/ 本体构建平面 ontology-service（:8091）/ 运行平面 runtime-manager（:8090）/ 桌面端（Tauri 壳）；
2. 复现步骤与最小示例；
3. 涉及组件与版本（源码提交号或安装包版本）。

**预期响应**：72 小时内确认收到 → 给出评估结论与修复计划 → 修复随下一个版本发布 → 在 Advisory 中致谢报告者（可按要求匿名）。

## 设计边界（供评估参考）

- **自托管产品**：模型 API Key 经 AES-256-GCM 加密存于本地 SQLite；数据/知识不出本机——除非你自行配置外部模型或向量服务。
- **端口暴露**：三个服务端口默认面向本机场景设计；若暴露到局域网/公网，请自行套反向代理并加鉴权（见 `platform-knowledge/01_整体设计/16_部署与运行.md`）。
- **不在威胁模型内**：以已获得本机执行权为前提的攻击（本地 shell、用户显式安装的恶意技能包/引擎）；此类场景建议改为公开 issue 讨论加固方案。
- 密钥导出功能按设计**不导出** API Key（仅标注 `has_key`，导入后需重录）。
