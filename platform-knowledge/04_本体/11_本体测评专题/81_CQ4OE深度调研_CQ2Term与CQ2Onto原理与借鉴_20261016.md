---
module: 本体
topic: CQ4OE 深度调研（CQ2Term + CQ2Onto 评估基准原理与项目借鉴）
desc: CQ 驱动本体生成评估基准 CQ4OE 的系列全景、CQ2Term/CQ2Onto 方法拆解与对构建面八条借鉴清单
synced: 2026-10-16
---

# CQ4OE 调研报告：CQ 驱动本体工程的评估基准与方法论

> **调研日期**：2026-10-16 | **调研范围**：CQ4OE 系列工作（CQ2Term + CQ2Onto）及相关研究 | **证据来源**：arXiv 原文、GitHub 仓库、OEG-UPM 官网、Semantic Web Journal 等

---

## 摘要

**CQ4OE** 是马德里理工大学本体工程组（OEG-UPM）于 2026 年 9 月发布的 LLM 辅助本体生成评估基准（[arXiv:2609.26029](https://arxiv.org/abs/2609.26029)），包含两个互补任务：**CQ2Term**（99 个 CQ 的术语级评估）和 **CQ2Onto**（118 个 CQ 的完整 OWL 本体级评估），覆盖 6 个源本体、9 个 LLM、3 种生成策略。核心发现是 **LLM 恢复显式词汇（类 F₁≈60%）远优于构建正确本体结构（属性 F₁≈32%，Closure F₁≈17%）**，且 Full CQ Coverage 仅 2.1%——几乎没有任何 LLM 能完整满足单个 CQ 的全部公理需求。该团队同期发表了 LLMs4OE 系统文献综述（Semantic Web Journal 2026）和多智能体框架 MASEO（LLMs4KG 2026 workshop），形成从"评估基准→生成流水线→领域综述"的完整研究矩阵。

对于 eino-multiagent-lab 项目，CQ4OE 的 **多维评估指标体系**（术语/属性/公理/层级/CQ 覆盖五层）和 **CQ-to-axiom provenance 追溯机制** 可直接为 REQ-331（质量评估 P1）的质量门禁设计提供方法论基础；其 **多方法术语对齐 pipeline**（Hard+Lexical+Semantic Top3 聚合）可嵌入 qualitygate 解决"标签不一致但语义等价"的匹配难题；其 **CQ 条件覆盖度量**（AtLeastOne/Mean/Full 三级）可为 REQ-247/248 的 CQ 一等公民化设计补充量化评估维度。

---

## 1. CQ4OE 系列全景：研究团队、论文矩阵与开源生态

### 1.1 研究团队与资助背景

CQ4OE 由 **Jiayi Li、Ziyuan Wang、Daniel Garijo、María Poveda-Villalón** 四人完成，均来自马德里理工大学（UPM）信息学工程学院的 **Ontology Engineering Group（OEG）**[(arXiv)](https://arxiv.org/abs/2609.26029)。OEG 是欧洲本体工程领域的头部研究组，在方法论（Methontology、NeOn、LOT）、工具链（OOPS!、Widoco、Themis）和标准参与（W3C/ETSI/ISO）方面积累深厚。

CQ4OE 隶属于 **SOEL 项目**（Supporting Ontology Engineering with Large Language Models, PID2023-152703NA-I00），由西班牙 MCIN/AEI 和欧洲区域发展基金资助[(CQ4OE 官网)](https://oeg-upm.github.io/cq4oe-benchmark/leaderboard/)。SOEL 的目标是系统性地将 LLM 集成到本体工程全流程，在 IoT、语言学和科学研究三个领域验证。

### 1.2 论文矩阵与时间线

CQ4OE 并非孤立工作，而是 OEG 团队在"LLM+本体工程"方向系列研究的最新成果。以下为该团队 2024–2026 年的核心产出矩阵：

| 时间 | 论文/资源 | 发表场所 | 核心贡献 |
|---|---|---|---|
| 2024-10 | LLMs for OE: A Landscape of Tasks and Benchmarking Challenges | ISWC 2024 HGAIS | 将 LLM+OE 工作按 LOT 方法论分类，识别任务/输入/输出映射 |
| 2026-01 | LLMs for OE: A Systematic Literature Review | Semantic Web Journal 17(4) | 分析 30 篇论文，识别 LLM 三种角色（engineer/expert/evaluator），指出 benchmark 缺失 |
| 2026-04 | IDEA2: Expert-in-the-loop CQ Elicitation | arXiv:2604.01344 | Semi-automated CQ 引出工作流，expert-in-the-loop + provenance tracking |
| 2026 | MASEO: Multi-Agent System for Explainable Ontology Generation | LLMs4KG 2026 (CEUR) | 四阶段多智能体本体生成（Generation→Syntax→Consistency→Pitfall），含 provenance |
| **2026-09** | **CQ4OE: A Benchmark for Assessing LLM-assisted Ontology Generation** | **arXiv:2609.26029** | **CQ-aligned gold standard + 双任务评估（CQ2Term/CQ2Onto）+ 多维指标体系** |

该矩阵呈现清晰的演进脉络：先通过 SLR 识别领域痛点和任务分类[(SWJ)](https://doi.org/10.1177/22104968261465514)，再开发生成流水线（MASEO），最后构建标准化评估基准（CQ4OE）——形成"综述→方法→评估"的完整闭环。

### 1.3 开源资源与可复现性

CQ4OE 的开源生态较为完善[(GitHub)](https://github.com/oeg-upm/cq4oe-benchmark)：

- **代码仓库**：`github.com/oeg-upm/cq4oe-benchmark`（Apache-2.0），含 gold standards、evaluation pipeline、run-level reports
- **数据集**：HuggingFace (`hf/8712`) + Zenodo (`20080309`)
- **在线 Leaderboard**：`w3id.org/cq4oe/leaderboard`，支持新方法提交对比
- **指标目录**：每个度量指标有持久化 W3ID 标识符和 Turtle 定义（`w3id.org/cq4oe/metrics`）
- **MASEO 代码**：`github.com/oeg-upm/maseo`，含 dataset 和 prompts

---

## 2. CQ2Term 方法拆解：从 CQ 到术语的概念化评估

### 2.1 任务定义

**CQ2Term** 评估 LLM 从自然语言 CQ 中识别和组织显式类（classes）与属性（properties）的能力——即本体工程中"概念化"阶段的自动化程度[(CQ2Term.ttl)](https://oeg-upm.github.io/cq4oe-benchmark/OntoCatalogue/KG/CQ2Term.ttl)。

- **输入**：一组自然语言 CQ（如"Which plants eat animals?"）
- **输出**：每个 CQ 所需的 classes 和 properties 集合（如 {Plant, Animal, eats}）
- **目的**：隔离评估 LLM 的词汇概念化能力，不涉及完整本体生成

### 2.2 Gold Standard 构建：E/I/R 三类术语集

CQ2Term 的 gold standard 基于精细的术语标注。对每个保留的 CQ_i，标注三类术语集[(arXiv Section 3.2)](https://arxiv.org/html/2609.26029v1)：

- **E_i（Explicit）**：CQ 文本中的直接表面匹配术语，如"Which plants eat animals?"中的 Plant、Animal、eats
- **I_i（Implicit）**：同义或等价表述，如"predators"对应的 Carnivore
- **R_i（Derived）**：CQ 未提及但形式化回答所需的推导术语，如回答"食肉植物"问题时需要的 CarnivorousPlant

CQ2Term 仅使用 **E_i（显式术语）**，因为它评估的是表面词汇识别能力。这一筛选从 118 个 CQ 中排除了 19 个推理密集型 CQ，最终留下 **99 个 CQ**。

### 2.3 术语对齐 Pipeline：多方法聚合策略

LLM 生成的标签常与 gold standard 不一致但语义等价（如 hasPart vs containsComponent），因此需要鲁棒的术语对齐。CQ4OE 设计了 **五方法聚合 pipeline**[(arXiv Section 4.1)](https://arxiv.org/html/2609.26029v1)：

| 方法 | 类型 | 说明 |
|---|---|---|
| HardMatch | 精确匹配 | 字符串完全相等 |
| SequenceMatch | 词法 | Python difflib.SequenceMatcher |
| LevenshteinDistance | 编辑距离 | textdistance 库 |
| JaroWinklerDistance | 前缀相似度 | textdistance 库 |
| SemanticCosineSimilarity | 语义 | embeddinggemma via Ollama |

**聚合公式**：HardMatch 为精确匹配覆盖；非 Hard 方法取 **Top3 最高分的均值**作为综合得分：

$$s(t_g, t_p) = \begin{cases} 1, & \text{if } s_{\text{hard}} = 1 \\ \frac{1}{3}\sum_{m \in \text{Top3}} s_m(t_g, t_p), & \text{otherwise} \end{cases}$$

**阈值设定**：类 τ_C=0.6，属性 τ_P=0.7（属性阈值更高以防止短标签's has_/is_ 前缀的虚假匹配）。匹配采用贪婪一对一策略。

这一设计的关键洞察是：**单一方法有互补性失败模式**——精确匹配保守（类 51.7%，属性 30.0%），语义匹配激进（类 73.9%，属性 63.3%），聚合后更鲁棒。

### 2.4 评估指标体系

CQ2Term 采用两类指标：

**全局指标**（Global）：Precision / Recall / F₁，基于对齐后的 TP/FP/FN 计算。

**CQ 条件覆盖**（CQ-conditioned Coverage）：检查恢复的术语是否挂载在正确的 CQ 下——一个 gold 术语只有在其对应 CQ 的预测中也被匹配才算覆盖。三个子指标：

- **AtLeastOne (A1)**：至少有一个术语被正确恢复的 CQ 比例
- **MeanCoverage (MC)**：所有 CQ 的平均覆盖率
- **FullCoverage (FC)**：所有术语全部被正确恢复的 CQ 比例

### 2.5 实验结果

9 个 LLM（DeepSeek V4-Pro/Flash/V3.2、Qwen3.6 Plus/Flash/35B-A3B/27B、Gemma 4 31B-IT/26B-A4B-IT）在 6 个域上产生 54 次运行，关键发现[(arXiv Section 5.1)](https://arxiv.org/html/2609.26029v1)：

- **全局术语 F₁**：59.1%（DeepSeek V3.2）~ 66.5%（DeepSeek V4-Pro）
- **类 vs 属性**：类恢复 67.1% vs 属性 55.2%，属性精确率仅 47.9%
- **域差异远大于模型差异**：AWO 达 90.5% F₁，Wine 仅 48.0%
- **CQ 覆盖**：A1=89.2%、MC=54.8%、**FC=23.5%**
- **关键反直觉发现**：Water 域全局 F₁=70.6% 但 FullCoverage=0%——**强全局词汇恢复不等于 CQ 级完整性**

---

## 3. CQ2Onto 方法拆解：从 CQ 到完整 OWL 本体的多维评估

### 3.1 任务定义

**CQ2Onto** 评估 LLM 生成完整 OWL 本体的能力——不仅要求词汇识别，还要求理解隐式和推导性术语，并将其组装为具有正确属性建模、公理和层次结构的形式化本体[(CQ2Onto.ttl)](https://oeg-upm.github.io/cq4oe-benchmark/OntoCatalogue/KG/CQ2Onto.ttl)。

- **输入**：一组自然语言 CQ
- **输出**：完整 OWL 本体（含类/属性层次、属性特性、domain/range 公理、TBox 约束）
- **覆盖**：118 个 CQ，6 个域

### 3.2 六维评估体系

CQ2Onto 在 CQ2Term 的基础上扩展了 4 个评估维度[(arXiv Section 4.3)](https://arxiv.org/html/2609.26029v1)：

| 维度 | 评估目标 | 说明 |
|---|---|---|
| ClassProperty | 类/属性词汇恢复 | 同 CQ2Term |
| PropertyCharacteristics | 属性特性建模 | functional/symmetric/transitive 等 OWL 特性 |
| Triple | Domain/Range 三元组 | 将 domain/range 公理视为 (s,p,o) 三元组评估图结构 |
| Axiom | TBox 公理级结构 | 递归翻译命名术语后严格匹配交集/并集/限制/基数 |
| HierarchyClosure | 推理闭包层级 | 用 HermiT 推理器比较 gold/predicted 的 SubClassOf 闭包 |
| CQCoverage | CQ 条件覆盖 | 公理级 + Closure rescue 两种形式 |

每个维度同时报告 **全局视图（Global）** 和 **对齐条件视图（AC）**——后者仅评估已对齐术语上的元素，隔离了"词汇不匹配"和"结构错误"两种失败模式。

### 3.3 三种生成策略

实验使用三种策略，均复用 MASEO 的 prompting set 以控制变量[(arXiv Section 5)](https://arxiv.org/html/2609.26029v1)：

- **Zero-shot**：一次性将全部 CQ 输入 LLM
- **Iterative**：逐个顺序输入 CQ
- **Multi-agent**：初始生成后经 RDFLib（语法）→ HermiT（一致性）→ OOPS!（pitfall）三轮迭代修复，支持回溯

共产生 162 个本体（9 模型 × 6 域 × 3 策略）。

### 3.4 实验结果与三大 LLM 局限

关键数据[(arXiv Section 5.1)](https://arxiv.org/html/2609.26029v1)：

- **类 F₁ 平均 59.7%，属性 F₁ 仅 31.8%**——从词汇到结构的性能陡降
- **Closure F₁ 仅 16.7%**——预测本体层级极浅
- **AC vs Global 差距巨大**：Triple-AC 36.4% vs Triple-G 12.4%，Axiom-AC 35.3% vs Axiom-G 15.0%——未对齐词汇是主要误差来源
- **Multi-agent 提升公理覆盖**：Axioms-Mean 从 18.3%（zero-shot）提升到 24.2%，但**不增加层级链**
- **Closure rescue 效果微弱**：平均仅提升 1.4 个百分点，Full 不变

三大 recurrent LLM 局限：

1. **层级链生成严重不足**：VGO/SWO 预测本体仅 0.6/0.7 个闭包对（gold 为 12/8），HermiT 几乎无层级可推理
2. **属性建模不稳定**：属性标签被改写/别名的频率远高于类，导致属性 F₁ 仅为类 F₁ 的约一半
3. **CQ 完整性极低**：Axiom-Full 仅约 2%——模型能恢复部分公理但几乎不能完整满足任何 CQ

---

## 4. CQ4OE 整体框架：CQ 驱动的本体工程生命周期闭环

### 4.1 方法论定位：CQ 作为需求-设计-评估的贯穿线索

在传统本体工程方法论中，CQ 的角色已得到广泛认可：

- **Methontology**：CQ 在知识规范化阶段从需求文档中提取
- **NeOn**：CQ 从场景/用户需求中推导，驱动本体设计
- **eXtreme Design (XD)**：CQ 与 Ontology Design Pattern (ODP) 结合，CQ 触发模式选择
- **LOT (Linked Open Terms)**：CQ 贯穿需求规格→概念化→编码→评估全生命周期

CQ4OE 的创新在于将 CQ 从**方法论指引**提升为**可计算评估的锚点**——通过 CQ-to-term 和 CQ-to-axiom 的显式 provenance，使得"本体是否满足需求"从定性判断变为可量化、可追溯、可复现的评估。

### 4.2 Gold Standard 构建方法论

CQ4OE 的 gold standard 构建采用 **四阶段标注流程**，全程实施 **三人审核裁决协议**[(arXiv Section 3.2)](https://arxiv.org/html/2609.26029v1)：

**Phase 1：核心术语选择**——对源本体按度排序（入度+出度），用 owl2diagram 可视化验证，保留高连接度且领域相关的核心术语 T_core。

**Phase 2：CQ 筛选与术语标注**——从 255 个已发布 CQ 中过滤掉需要外部知识、不可从源本体回答、或针对 ABox 实例级的 CQ，保留 110 个。对每个 CQ_i 标注 E_i/I_i/R_i 三类术语集。

**Phase 3：CQ 增强**——检查 T_core 中的术语是否都被至少一个 CQ 覆盖，未覆盖的通过人工编写新 CQ 补充。仅新增 8 个 CQ（3.1%），最终 118 个。

**Phase 4：Gold Standard 构建**——CQ2Term 取 E_i 构建 CQ-to-term 映射；CQ2Onto 取 E_i∪I_i∪R_i 并从源本体抽取 CQ 相关 TBox 片段，仅保留"移除后使 CQ_i 不可答"的公理（充分性原则）。

**质量**：118 个 CQ 中 85.6%（101 个）首轮即通过，修改集中在复杂本体（SWO 10 个、VGO 3 个、ODRL 2 个）。

### 4.3 CQ-to-Axiom Provenance：严格的需求追溯

CQ4OE 的 provenance 设计比 MASEO 的"来源标注"更严格。MASEO 记录"哪个 CQ 引入了哪个术语"（来源追溯），而 CQ4OE 的 CQ-to-axiom 链接采用**反事实充分性判据**——"如果移除该公理，CQ_i 将不可答"[(arXiv Section 3.2)](https://arxiv.org/html/2609.26029v1)。这使得每个 CQ 关联的公理集构成**充分集**，而非仅仅是"相关"。

这一设计对本体工程有深远意义：它使得"本体是否满足需求"的评估从模糊的整体判断精确到单个 CQ 级别的归因——可以定位是哪个 CQ 的哪些公理缺失导致需求不满足。

### 4.4 团队系列工作的协同关系

OEG 团队的系列工作形成了清晰的协同矩阵：

- **SLR**[(SWJ)](https://doi.org/10.1177/22104968261465514)：识别 30 篇 LLM+OE 论文，揭示评估碎片化问题——为 CQ4OE 提供问题定义
- **MASEO**[(CEUR)](https://dgarijo.com/papers/maseo-camera-ready.pdf)：四阶段多智能体生成流水线——为 CQ4OE 提供 multi-agent baseline 实现
- **IDEA2**[(arXiv)](https://arxiv.org/html/2604.01344)：Expert-in-the-loop CQ 引出——填补"从需求到 CQ"的前端
- **CQ4OE**[(arXiv)](https://arxiv.org/abs/2609.26029)：标准化评估基准——填补"从 CQ 到本体的评估"的后端

---

## 5. 与 OntoChat/OntoExtend 对照定位

### 5.1 定位差异

CQ4OE 与项目已有调研的 OntoChat/OntoExtend 在本体工程生命周期中占据不同位置：

| 维度 | OntoChat | CQ4OE | OntoExtend |
|---|---|---|---|
| **核心任务** | 对话式需求引出（User Story → CQ） | CQ→本体的生成与评估 | 需求驱动的本体扩展（ODP+LOV） |
| **生命周期阶段** | 前端：需求规格 | 后端：概念化+编码+评估 | 演化：维护+扩展 |
| **输入** | 用户故事/访谈/文件材料 | 一组 CQ | 目标本体 + ODP + 扩展需求 |
| **输出** | CQ 集 + 聚类 + 草稿本体 | 评估报告（多维指标+CQ 覆盖） | 扩展草稿 + merge 审查 |
| **LLM 角色** | CQ 抽取/净化/聚类辅助 | 本体生成（被评估对象） | ODP 适配/扩展草稿生成 |
| **人的角色** | 共创引导（对话式交互） | 可选（benchmark 评估无需人） | 审查+确认（human-in-the-loop） |
| **评估方法** | 用户评分（Likert 量表） | 自动化多维指标（12+ metric） | 冲突检测 + strict 门禁 |
| **开源** | 部分（Polifonia 项目内） | 完全开源（Apache-2.0） | 项目自研 |

### 5.2 互补关系

三者构成 **需求引出→生成评估→演化扩展** 的全链路：

```
[OntoChat: 需求 → CQ]  →  [CQ4OE: CQ → 本体 + 评估]  →  [OntoExtend: 本体 + 新需求 → 扩展]
```

OntoChat 解决了"如何从非结构化需求中提取高质量 CQ"（86.4% 的受访者认为这是最需计算支持的任务），但产出 CQ 后的本体生成与质量评估是空白——这正是 CQ4OE 的领域。而 OntoExtend 处理本体交付后的增量演化，形成完整闭环。

值得注意的是，**CQ4OE 论文中的 Related Work 并未引用 OntoChat**——这表明两者在学术脉络上是独立发展的，但方法论上高度互补。将 CQ4OE 的评估方法引入项目的 OntoChat 生成链路，可以显著提升生成质量的可量化程度。

---

## 6. 结合本项目的可借鉴点清单

基于 CQ4OE 的方法论与项目当前状态（REQ-171 多途径构建平台、OntoChat 闭环已通、质量门禁雏形已有），梳理以下可借鉴点：

### 6.1 借鉴点总览

| # | 借鉴什么 | 对应项目模块/REQ | 优先级 | 实施建议 |
|---|---|---|---|---|
| 1 | **CQ→术语→本体的分层评估方法**：CQ2Term 评术语、CQ2Onto 评结构，分离概念化能力和建模能力 | REQ-331 质量评估（P1 M131） | **P1** | 在 qualitygate 中增加"术语级"和"结构级"两层评估，而非单一整体评分 |
| 2 | **多方法术语对齐 pipeline**：Hard+Lexical+Semantic Top3 聚合，类/属性分别设阈值 | qualitygate 门禁 + REQ-331 | **P1** | 嵌入 importer/merge 流程，解决"LLM 生成 hasPart 但 gold 是 containsComponent"的等价匹配 |
| 3 | **CQ 条件覆盖三级度量**：AtLeastOne / MeanCoverage / FullCoverage | REQ-247/248 CQ 面板增强 | **P1** | 在 Spec 编辑器 CQ 面板中，为每个 CQ 显示覆盖状态（全覆盖/部分覆盖/未覆盖） |
| 4 | **CQ-to-axiom provenance 追溯**：每个公理标注服务哪些 CQ，移除后使 CQ 不可答则保留 | REQ-247/248 CQ 追溯增强 | **P2** | 在 spec 的 CQ 字段中增加 axiom provenance，支持"点击 CQ → 高亮相关公理" |
| 5 | **Multi-agent 生成+修复流水线**：Generation→Syntax(RDFLib)→Consistency(HermiT)→Pitfall(OOPS!) | REQ-337 构建类型独立页面 + REQ-340 管线编排 | **P2** | 作为"由 CQ 构建"路径的后端实现参考，尤其三工具串联修复模式 |
| 6 | **Gold standard 构建方法论**：四阶段标注 + 三人审核 + E/I/R 术语分类 | 评估数据集建设（为 REQ-331 提供 ground truth） | **P2** | 为项目已有的种子本体（Wine/AWO 等教学本体）标注 CQ→term/axiom 映射 |
| 7 | **Closure rescue 机制**：HermiT 推理恢复层级关系，虽效果有限但提供了"推理补偿"思路 | REQ-333 NL2SPARQL（P2 M133） | **P3** | 在 NL2SPARQL 的 SPARQL 查询验证中，考虑推理器辅助的隐含关系发现 |
| 8 | **W3ID 持久化标识符**：每个指标/任务/维度有唯一 URI，支持跨 pipeline 集成 | D-O19 消费侧观测台 | **P3** | 为质量评估指标建立持久化标识，便于在观测台中引用和展示 |

### 6.2 重点借鉴点详述

**借鉴点 1（P1）：分层评估方法 → REQ-331**

当前 OntoChat 的 REQ-274"覆盖测试快筛"是粗粒度的——仅判断 CQ 是否"大致被覆盖"。CQ4OE 展示了如何将评估分解为正交的多个维度：术语层（ClassProperty）、属性建模层（PropertyCharacteristics）、图结构层（Triple）、逻辑层（Axiom）、推理层（HierarchyClosure）。

**具体落地建议**：在 qualitygate 中实现类似的五层评估，每层独立打分。OntoChat 生成的草稿本体先过术语层（CQ→classes/properties 是否齐全），再过结构层（domain/range 是否正确），最后过逻辑层（公理是否充分）。这比当前的单一"质量报告卡"提供更精确的诊断信息。

**借鉴点 2（P1）：多方法术语对齐 → qualitygate**

CQ4OE 的术语对齐 pipeline 解决了一个实际痛点：LLM 生成的本体标签与参考标准不一致但语义等价。当前 qualitygate 的匹配逻辑如果仅用精确匹配或单一编辑距离，会产生大量假阴性。

**具体落地建议**：在 qualitygate 的匹配模块中实现 HardMatch + Levenshtein + SemanticSimilarity 三方法聚合。阈值可参考 CQ4OE 的经验（类 0.6、属性 0.7）。SemanticSimilarity 可用本地 embedding 模型（如 embeddinggemma 或项目已有的 embedding 服务）避免额外 API 调用。

**借鉴点 3（P1）：CQ 条件覆盖 → REQ-247/248**

项目已将 CQ 一等公民化（spec 可选 cq 字段），但缺少量化评估。CQ4OE 的三级覆盖度量提供了从粗到细的完整方案：

- **AtLeastOne**：快速筛查——CQ 是否有至少一个相关术语被覆盖
- **MeanCoverage**：渐进度量——CQ 的平均术语覆盖率
- **FullCoverage**：严格度量——CQ 是否完全被满足

**具体落地建议**：在 Spec 编辑器的 CQ 面板中，为每个 CQ 显示三级覆盖指示器（如 🟢全绿/🟡部分/🔴未覆盖），让用户直观看到哪些 CQ 已满足、哪些还需补充。

**借鉴点 5（P2）：Multi-agent 流水线 → REQ-337/340**

MASEO 的四阶段流水线（Generation→Syntax→Consistency→Pitfall）是一个经过验证的 LLM 本体生成后处理模式。CQ4OE 的实验表明 multi-agent 策略将公理覆盖从 18.3% 提升到 24.2%（+32% 相对提升），虽然绝对值不高但证明了结构化修复的有效性。

REQ-340（rest.go 拆分+管线编排）正在规划统一的"来源→抽取→草稿→门禁→审查→入库"管线。MASEO 的四阶段设计可直接作为管线中"草稿→门禁"段的参考架构：RDFLib 语法校验→HermiT 一致性检查→OOPS! pitfall 扫描，每步带回溯。

---

## 7. 局限与风险提示

### 7.1 CQ4OE 自身局限

1. **源本体覆盖有限**：仅 6 个本体（Wine/AWO/ODRL/SAREF4WATR/VGO/SWO），虽覆盖三个规模层级，但未涉及中文本体、工业本体或大规模上层本体（如 SUMO/CYC）[(arXiv Section 6)](https://arxiv.org/html/2609.26029v1)
2. **CQ 来源非真实需求收集**：CQ 来自已有本体的已发布 CQ，而非从真实用户故事中引出——缺少"需求模糊性"和"利益相关者冲突"等真实场景特征
3. **仅评估 TBox**：不涉及 ABox（实例数据）的评估，而实际本体应用中实例数据的质量同样重要
4. **LLM contamination 风险**：部分源本体（如 Wine）可能出现在 LLM 训练数据中，未来需引入 post-cutoff 本体做 contamination-controlled 评估

### 7.2 对本项目场景的适配挑战

1. **领域差异**：CQ4OE 的 6 个源本体以教学/示例本体为主，项目的目标场景（K8s 运维本体等）在复杂度和专业性上可能显著不同
2. **语言差异**：CQ4OE 的 CQ 均为英文，项目的 CQ 可能涉及中文——中文术语对齐需要额外的分词和语义匹配策略
3. **规模差异**：CQ4OE 最大本体 SWO 有 1971 个类但 CQ 仅 35 个，实际工业本体可能 CQ 数量远超此范围
4. **评估定位**：CQ4OE 是研究基准（benchmark），直接用于生产环境的质量门禁需要工程化适配——如评估延迟、embedding 模型部署、HermiT 推理开销等

### 7.3 风险与缓解

| 风险 | 缓解策略 |
|---|---|
| CQ4OE 指标计算依赖 HermiT 推理器，对大规模本体性能差 | 考虑用 OWLReasoner 的增量推理模式，或对大规模本体仅评估 CQ 相关子集 |
| 语义对齐依赖 embeddinggemma，可能不适合中文/领域术语 | 替换为多语言 embedding 模型（如 multilingual-e5）或领域微调模型 |
| Gold standard 构建需要大量人工标注 | 可先用 LLM 预标注 + 人工审核的半自动模式（参考 IDEA2 思路）降低标注成本 |
| CQ4OE 的 multi-agent 策略提升有限（~32%） | 结合项目的 human-in-the-loop 设计（OntoChat 对话式确认），将人工审核作为最终质量保障 |

---

## 附录：关键资源链接

| 资源 | URL |
|---|---|
| CQ4OE 论文 | https://arxiv.org/abs/2609.26029 |
| CQ4OE GitHub | https://github.com/oeg-upm/cq4oe-benchmark |
| CQ4OE Leaderboard | https://w3id.org/cq4oe/leaderboard |
| CQ4OE Catalogue | https://oeg-upm.github.io/cq4oe-benchmark/OntoCatalogue/html/catalog.html |
| CQ4OE 数据集 (Zenodo) | https://doi.org/10.5281/zenodo.20080309 |
| CQ4OE 数据集 (HuggingFace) | https://doi.org/10.57967/hf/8712 |
| MASEO 论文 | https://dgarijo.com/papers/maseo-camera-ready.pdf |
| MASEO GitHub | https://github.com/oeg-upm/maseo |
| SLR (Semantic Web Journal) | https://doi.org/10.1177/22104968261465514 |
| IDEA2 | https://arxiv.org/abs/2604.01344 |
| SOEL 项目 | https://oeg-upm.github.io/soel-site/website/ |
| OEG-UPM | https://oeg.fi.upm.es/ |
