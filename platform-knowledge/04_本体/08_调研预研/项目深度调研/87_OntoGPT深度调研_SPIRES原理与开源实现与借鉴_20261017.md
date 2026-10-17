---
module: 本体
topic: OntoGPT 深度调研（SPIRES 原理与开源实现与借鉴）
desc: OntoGPT/SPIRES 深度调研——本体落地抽取（ontology-grounded extraction）方法拆解、评估实证、与本项目构建六路径对照及借鉴途径
synced: 2026-10-17
---

# 87_OntoGPT 深度调研_SPIRES 原理与开源实现与借鉴

> 方法论：idea-to-feasible-plan Skill（源资料精读 → 实现盘点 → 映射设计 → 约束化方案 → 分阶段路线）
> 源资料：[SPIRES 论文](https://arxiv.org/abs/2304.02711)（Caufield et al., Bioinformatics 40(3) 2024, btae104；arXiv v2 2023-12-22）+ [OntoGPT 官方文档](https://monarch-initiative.github.io/ontogpt/) + [PyPI 1.2.0](https://pypi.org/project/ontogpt/)（2026-09-09 发布）+ [ontogpt-experiments](https://github.com/monarch-initiative/ontogpt-experiments) grounding 对比 notebook
> 项目基线：[03 号](../../../docs/03_本体_需求文档.md) v1.50（REQ-171 六路径已交付/REQ-331 质量评估 P1/REQ-344~346 评测批）、[26 号方案](../方法论与理论/26_本体_多途径构建平台可落地方案.md)（LinkML 系列已列 P3 边界）、[86 号体检](../产品与设计分析/86_本体实现现状体检与不足清单_20261017.md)

---

## 摘要

OntoGPT 是 Monarch Initiative（LBNL）的开源 Python 包（BSD-3，monarch-initiative/ontogpt），本体定位为「**LLM 结构化抽取 + 本体 grounding**」工具链，核心是 SPIRES 方法：给一份 LinkML schema 与自由文本，递归逐类生成 prompt 让 LLM 只回填**名字**，再用 OAKlib 标注器把名字对齐到真实本体 ID，**未对齐者标 `AUTO:` 前缀而非编造**。其最强实证是一组 grounding 对比：GO 100 随机术语，SPIRES 管道 97~98 正确，**裸问 LLM 仅 3 正确**（mass hallucination）；RE 任务（BC5CDR）F=43.8 仅中游——**卖点不是抽取精度而是「ID 只可能来自真本体」**。对本项目：六路径中 KB 策略 A 与 llmcreate 均无构建期 ontology grounding（qualitygate 只检结构、LOV 只搜词表），SPIRES 的「名字-不-ID+grounding 校验环」是 REQ-79 对齐总纲缺失的构建期特例，建议并入 REQ-79 口径拆子项立项；模板三合一（schema=prompt=validator）与递归抽取作 P3/观察。运行时不引入（Python 栈，沿 D-O15 与 65/66 号先例）。

## 1. 项目全景

| 维度 | 事实 |
|---|---|
| 主体 | Monarch Initiative（LBNL 环境基因组部），作者 Caufield/Hegde/Emonet/…/Mungall；Bosch Research 资助部分工作 |
| 仓库/包 | github.com/monarch-initiative/ontogpt，PyPI `ontogpt` 1.2.0（2026-09-09），BSD-3-Clause，Python ≥3.10 |
| 论文 | SPIRES: Bioinformatics 2024;40(3):btae104（doi:10.1093/bioinformatics/btae104，arXiv:2304.02711v2） |
| 生态位 | OAKlib（标注/grounding）+ LinkML（schema 语言）+ linkml-owl→ROBOT→Elk（公理化与推理）+ KG-Hub（落库）；LinkML 生态表将其定位在「Reuse Ontologies」阶段（建议 ontology mappings 与 value sets）；伴生项目 CurateGPT（人工策展辅助）、TALISMAN（基因集富集摘要） |
| 新近演进 | ①LiteLLM 多模型接入（OpenAI/Anthropic/Groq/本地等）；②README「Why OntoGPT?」重写：grounding without hallucination / schema adherence / repeatable batch / 模板库 / Agent Skills；③`skills/` 四个 Agent Skills（跑抽取/选模板/写模板/排障）教 coding agent 调用；④OHDSI 2025 报告的社区分支用 Instructor 库替换伪 YAML 解析+SemSQL 本地词表 grounding（air-gapped OMOP 场景） |

## 2. SPIRES 方法拆解（论文 §3）

**输入**：(1) 知识 schema S（LinkML）；(2) 入口类 C；(3) 文本 T。**输出**：符合 S 的实例树（JSON/YAML/RDF/OWL）。

1. **GeneratePrompt**：`Instructions + AttributeTemplate(S,C,T) + "Text:" + T + "==="`——AttributeTemplate 按 C 的属性清单生成**伪 YAML 属性模板**（`food_item: <the food item>`；multivalued 前缀「A semicolon-separated list」；属性可带自定义 prompt 注解）。**每次只放当前类的字段**，上下文经济。
2. **CompletePrompt**：LLM 补全，期望返回伪 YAML。
3. **ParseCompletion + 递归**：LLM 输出不被信任为严格 YAML——按行启发式解析（首个 `:` 切分、大小写不敏感、空白归一）。取值按 range 分派：基元类型直收；引用类/枚举→送 Step4 grounding；**内联类→递归调用 SPIRES(S, Range(a), v)**（如 `amount: 2 tablespoons` 触发 Quantity 子抽取），直到非内联类。
4. **Grounding and Normalization**：对实例树全部**命名实体叶子**做 grounding——类上以 `annotators` 注解声明词表链（如 `sqlite:obo:mesh, sqlite:obo:chebi, gilda:`, 依序尝试），grounding 结果再过**标识符约束**校验：`id_prefixes`（前缀白名单）、`pattern`（ID 正则，如 `^MESH:[CD][0-9]{6}$`）、`values_from`（动态值集=对源本体的声明式查询，「取 O1 的 A/B/C 分支排除 D，再并入 O2 全部」）。1.2.0 行为：grounding 失败的名字**不猜 ID，标 `AUTO:` 前缀**；输出附 `validation` 段逐 ID 报告存在性与标签匹配，可替换则替换。
5. **TranslateToOWL（可选）**：linkml-owl 按 schema 内映射把实例树翻成 OWL 公理→ROBOT merge 外部本体子集→Elk 推理分类/一致性检查。

**Schema 即三合一**：LinkML 模板同时是 ①抽取目标结构 ②prompt 生成源（`prompt:`/`prompt.examples:` 注解） ③输出校验器（编译为 Pydantic 类）。模板库 `src/ontogpt/templates/` 数十份（disease/phenotype/drug/gene/gocam/metagenome/recipe…），论文 Table 1 列五例（FOODON+UO / MONDO+CHEBI+MESH / MESH / ENVO / MONDO+HPO）。

## 3. 评估实证（论文 §2.1/§4.3/§4.4 + 社区分支）

**3.1 Entity grounding 对比（核心卖点，§4.3）**——三个本体各随机 100 术语，SPIRES 管道 vs 裸 prompt 直接要 ID：

| 本体 | SPIRES+GPT-3.5 | SPIRES+GPT-4-turbo | 裸 GPT-3.5 | 裸 GPT-4-turbo |
|---|---|---|---|---|
| GO | **98/100** | 97/100 | 3/100（mass hallucination：给 100 个候选但 ID 基本错） | 拒答（自称无实时库访问） |
| EMAPA | **100/100** | 拒答 | 反复给错本体（EHDAA2 人解剖学 ID） | 拒答 |
| MONDO | 97/100 | **18/100**（把「UV-induced skin damage, susceptibility to」解析成「skin damage」类错误） | ≤1/100 | ≤1/100 |

**3.2 BC5CDR 关系抽取（诚实的中游成绩，§4.4）**：500 摘要/1066 CID 三元组，零微调——no-chunking+GPT-4：**F=43.80**（P 0.69/R 0.32）；chunking+GPT-3.5：F=41.16。位次=18 支原赛队伍平均之下（最佳 0.57，但全训练集工程）；对比领域微调 BioGPT 44.98。**结论：zero-shot RE 精度不占优，优势在零训练成本+任意 schema 适配**。评估资产全部开放：`ontogpt eval EvalCTD`+Zenodo openai_cache.db（prompt/响应缓存库可复现）。

**3.3 社区生产化分支（OHDSI 2025）**：SPIRES→OMOP 临床 grounding（Llama3-8b 本地模型，air-gapped）：实验一临床笔记→SNOMED 严格 72%/有效 85%/近匹配 96%（规则基线 20%/64%）；实验二放疗部位标签→SNOMED 85% 正确。两处工程改造值得注意：**Instructor 库（Pydantic 中心）替换伪 YAML 启发式解析**；**SemSQL 本地词表替换 BioPortal 外部服务**。

**3.4 论文自认局限（§5）**：OpenAI 大语料成本高；闭源模型训练数据不可考有偏；嵌套建议 ≤2 层；解析脆弱（启发式伪 YAML）；LLM 输出大小写/空白创造性偏差需归一。

## 4. 三策略与版本演进

| 策略 | 机制 | 状态 |
|---|---|---|
| **SPIRES** | 上述本体落地抽取（ZSL，schema 驱动） | 主力，持续演进 |
| **HALO**（HAllucinating Latent Ontologies） | few-shot：给种子示例让 LLM「幻觉」出候选领域本体再筛选 | 早期（0.2 时代），现非主推 |
| **SPINDOCTOR** | 聚类受控词+LLM 摘要生成新本体条目（表型本体增量归纳/伪富集） | 辅助功能 |

## 5. 与已有调研对照定位

| 对照 | 关系 |
|---|---|
| OntoChat（63/83 号，REQ-271~275/342） | OC=**对话式多轮**构建（人机协作产 spec）；OntoGPT=**单次批量**文档→结构（无人值守管线上）。互补：OC 交互层可借 SPIRES 的 grounding 环兜住对话中间产物 |
| OntoExtend（64/84 号，REQ-343/344） | OE=三段式 RAG 生成+评估；OntoGPT 无检索链，但其 gold 评估方法论（BC5CDR 全量+缓存复现）与 OE-3 移除法评估器同族 |
| CQ4OE（81 号，REQ-345 基准集） | CQ4OE=从 CQ 评本体生成；OntoGPT 评估=从 gold 语料评抽取。都是「gold terms+P/R/F」族——85 号基准集扩条的直接方法论参照 |
| open-ontologies（34/65/66 号，双轨已退役） | 同为「外部构建工具」：oo 曾真接入后退役（65/66 号先例）。OntoGPT 应**直接按退役后口径对照**——不接运行时，只取方法论 |
| Helix Foundry（67 号）A1「结构确定性推断+LLM 只写描述」 | 与 SPIRES「LLM 只出名字，ID 由确定性标注器出」**同构**——两个独立来源共同指向「LLM 退出标识符事务」原则 |

## 6. 结合本项目的可借鉴点清单（S3 映射）

**现状盘点（S2）**：六路径中「文本→结构」面=KB 策略 A（REQ-108，chunk→LLM+CQ 引导→spec_json）与 llmcreate（REQ-82，六路径①共用生成核，Validate+qualitygate 双源修复 ≤3 轮）；grounding 面=**缺位**——qualitygate 11 检查项全是结构级（孤立概念/悬空端点等），LOV 薄层只做词表**发现**不做实体**对齐**，REQ-79（本体对齐）长期未动且其前置仅被 343 进程内向量检索部分满足（86 号 N8）；评估面=85 号 CQ 基准集 15 条（N2 统计强度不足）。

| # | OntoGPT 机制 | 本项目对位与映射 | 状态 |
|---|---|---|---|
| J1 | **名字-不-ID+grounding 校验环+`AUTO:` 隔离** | 构建期实体 grounding 薄层：llmcreate/KB 策略 A 抽出的概念与关系标签→已有本体概念对齐（两段式：SQL 标签精确匹配→embedding 近邻，复用 ontorag retriever 元素向量基础设施，注意 86 号 N3 版本缓存宜先行）；未对齐者标 `auto:` 前缀进审查清单不静默入库；qualitygate 增「未对齐实体」检查项（warning 级） | 🆕（**并入 REQ-79 口径拆子项**——79 为运行/导入期对齐总纲，本项=构建期特例；不建议新开平行号） |
| J2 | **模板三合一**（schema→prompt 属性模板→校验器同源） | spec_json JSON Schema 已是校验器；借鉴「prompt 从 schema 属性清单自动组装」降低手工 prompt 漂移——llmcreate prompt 组装重构一小步，非引入 LinkML（26 号 P3 边界维持：与 spec_json 冲突） | ⏸（挂 REQ-346 测试资产产品化邻近面评估） |
| J3 | **递归抽取切片**（每 prompt 单类字段，内联类递归） | KB 策略 A 长文档抽取的上下文经济方案；同时启发「抽取模板按任务注册」而非单一巨型 prompt | ⏸（REQ-108 P2 增强评估项） |
| J4 | **gold+P/R/F+缓存复现评估法**（eval 子命令+openai_cache.db） | 85 号 CQ 基准集扩条的方法论对照（gold terms 口径一致）；本体评测批（REQ-344/345/346）的报告可比化（REQ-257 升级项）可参考其「评估=可复现命令+缓存」形态 | ✅ 方法论引用（86 号 N2 扩条依据之一） |
| J5 | **本地词表快照 grounding**（sqlite:obo / SemSQL） | LOV 在线搜索之外的离线对位：常用词表本地快照+本地标注——与 D-O15 轻量自研相容，但当前无明确触发诉求 | ⏸ 观察（LOV 502 优雅降级已可用） |
| J6 | **HALO/SPINDOCTOR 归纳式本体生长**（few-shot 幻觉候选+聚类+LLM 摘要成新类） | REQ-103 对话伴生本体生长与 REQ-342 OC 论文深化的对照方法论（「聚类+LLM 命名」是伴生概念聚合进本体的可行机械） | ⏸ 观察对照 |
| J7 | 运行时接入（Python 包/CLI） | Go 栈+D-O15（不接运行时依赖，参考自研轻量）+65/66 号 oo 双轨退役先例 | ⛔ 不引入；ontogpt CLI 可作**离线对照工具**（评测对照/模板设计参考，不入启动链路） |

**建议三档（待拍板，沿 79/80/85 号先例格式）**：
- **建议 A（P2，M148 预留）**：构建期实体 grounding 薄层（J1）——并入 REQ-79 拆「构建期 grounding」子项；依赖=86 号 N3 retriever 版本缓存先行（复用同一向量化基础设施）；验收=llmcreate+KB 策略 A 产物 grounding 报告+qualitygate 新检查项+auto: 前缀审查动线。
- **建议 B（P3，触发驱动）**：抽取模板注册表（J2+J3）——每构建任务一份「schema 节选+prompt 属性模板+校验器」三合一资产；触发条件=REQ-346 管理面落地后评估或 KB 策略 A 真机长文档抽取出现上下文瓶颈。
- **建议 C（观察）**：J4/J5/J6 登记方法论参照不立项；ontogpt 作离线对照工具（如需评测对照再议）。

## 7. 风险与边界

1. **RE 精度中游是常态不是缺陷**：SPIRES 的价值主张=zero-shot 灵活性+ID 可靠性，不是抽取精度；本项目引入其机制时验收指标应设 grounding 命中率（对位 98/3 那组数字）而非 RE F 值。
2. **伪 YAML 解析已被社区淘汰**（Instructor/结构化输出替代）：本项目 LLM 调用应直接用 JSON/结构化输出，**只借鉴「模板即 prompt」思想不借鉴其文本解析**。
3. **LinkML 边界**（26 号 §2.1 P3 先例维持）：LinkML 与 spec_json 双 schema 语言冲突，不引入；J2 用 JSON Schema 平移。
4. **生物医学模板偏重**：模板库以生命科学为主（recipe 证明通用性但单薄），通用领域需自写模板——与本项目领域无关的模板不构成直接复用面。
5. **token 成本**：大语料批抽取成本高（论文 §5.2 自认）；本项目构建场景是单本体草稿生成，量级可控。

## 8. 维护说明

- 立项/状态变化回写：18 号注册表+03 号（若拍板建议 A/B）+本档 §6 状态列。
- 登记簿：15 号 §六（OntoGPT 项目行）+§T1（SPIRES 论文行）——本轮已登记。
- 关联档：81 号（评估方法论同族）、84 号（A2 embedding 边界与 N3 缓存）、86 号（N2/N3/N8）、65/66 号（外部工具退役先例）。
