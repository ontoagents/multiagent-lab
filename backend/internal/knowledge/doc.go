// Package knowledge REQ-372/M170：知识卡自动校验器（提取流水线第②步的常跑形态）。
// 校验对象：platform-knowledge/09_知识体系/ 下的学习路径文档与三型知识卡——
//
//	①卡模板：frontmatter 必备字段（module/topic/desc/synced/kind/domain/prereq/sources/check）
//	  与枚举合法（kind=concept|decision|practice，domain=ontology|agent|kb|bridge）；
//	②篇幅：概念 300~800 / 决策 400~1000 / 实践 400~800（正文非空白 rune 数）；
//	③源指针存在性（硬纪律）：sources 中「路径 §…」形态的文件必须存在于仓库；
//	  REQ-nnn 必须在 18 号注册表出现；D-O* 跳过（决策号语义校验留人工抽检）；
//	④prereq 断链检测：相对路径必须能解析到实际卡文件；
//	⑤主档索引完备：学习路径主档必须索引本线全部卡。
//
// 校验器是 L2 图谱层的免费产物（断链检测/前置图数据源），纯测试零产品代码——
// 对应方案「P1 零代码」口径（平台前端/后端 0 改动）。
package knowledge
