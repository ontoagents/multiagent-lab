# OpenBKN 专题（09）

> 建档：2026-10-16（开发者指令「在本体模块下建一个 openbkn 专题，集中管理相关知识」；**专题下只保留方案文档**，源码快照不入专题不入库）。

## 一、定位与结论

**OpenBKN**（github.com/openbkn-ai，4 仓：bkn-foundry / bkn-studio / bkn-sdk / bkn-samples）——企业级开源本体平台，对标 Palantir Foundry Ontology。

- **整体定位（D-O12，2026-09-22 开发者拍板）：仅作参考记录，不落地实施**——平台级功能重叠+许可证约束（Apache-2.0 与 OpenBKN License 混合，部分模块禁多租户/托管/白标）+ fork 包袱；策略为「借鉴思想 + 重实现」。
- 登记状态：登记簿第一类「本体构建开源方案」🔭 对照参考（docs/15_开源项目及论文登记簿.md OpenBKN 行）。

## 二、专题文档

| 档号 | 文档 | 说明 |
| --- | --- | --- |
| 82 | [82_OpenBKN构建与运行深度分析_20261016.md](82_OpenBKN构建与运行深度分析_20261016.md) | **权威档（v2）**：源码级构建/运行全链路——BKN Lang v2.0.1 定义格式与三条构建路径、运行侧四类查询（全部无实例搬运）+ Action 执行链 + Agent 消费面、Risk Type 一等公民、AGENTS.md 分层治理、借鉴分级 A（可直接抄）/B（改造借鉴）清单。2026-10-16 随 33 号 v1 并入升版。 |

> 历史沿革：32 号独立档（2026-09-29 并入 33 号）→ 33 号 v1（2026-10-16 并入 82 号退役）。82 号即本专题唯一方案文档，后续 OpenBKN 相关增量调研与借鉴跟踪均归档于此。

## 三、与本项目需求映射（源自 82 号附编）

- REQ-82 生成格式备选（BKN Lang Markdown+YAML DSL 对照）；REQ-95 diff 参照（知识网络包化 tar/CHECKSUM/Diff）；REQ-94/101 trace 铁律；REQ-90/91 对比案例（kweaver-ai 同源）；仓库 AGENTS.md 治理（分层规则系统+Agent 硬规矩）。

## 四、源码快照（不入库）

分析用代码快照保留在仓库外项目目录（专题目录不收）：`/Coze/Drive/本体学习/OpenBKN代码分析_20261016/`（bkn-foundry v0.2.1 @ 798abfa、bkn-sdk、bkn-samples v0.2.0 三 clone+原始报告底稿），供后续 diff 对比与增量调研复用；重新拉取直接 git clone 即可。
