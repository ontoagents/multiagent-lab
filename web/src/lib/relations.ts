// REQ-617-87d8/M425-87d8 R4（121 号 §4.4 文案单源）：全站唯一「正式关系句」。
// 落点四处：两模块空态（Sidebar EmptyGuide footer）/ 工作区侧板配置视图（WorkspaceSidePanel
// ws-anchor）/ 两处删除 Modal（RelationConsequenceBlock 首行——工作区侧板删除 + 项目侧板
// Popconfirm 共用）/ 项目侧板转发源区（ProjectSidePanel「默认值转发源（归属工作区）」）。
// 改口径只改本行——四点自动同步（单源含义）。
export const RELATION_SENTENCE =
  '工作区管「场景与上下文」——这类工作用什么知识库、什么本体方案、什么智能体；项目管「产物落地」——文件写到哪、Git 怎么管。项目可归属工作区（新会话自动继承其默认值），也可独立运行；两者不互相替代。'
