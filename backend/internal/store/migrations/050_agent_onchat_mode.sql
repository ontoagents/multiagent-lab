-- REQ-360/M161 OntoChat 对话建本体能力产品化（03 §2.54 A 能力开关机制层）：
-- Agent 能力开关 onchat_mode（''=关〔缺省，存量零变化〕| guided=访谈 | quick=快速）。
-- 开关 on 时装配层自动并入 ontochat_* 四工具（与 Agent.Tools 手勾取并集）+ 注入内置访谈
-- 纪律段（instruction segment source=onchat）——引导段从「用户手工粘贴」退役为系统内置。
ALTER TABLE agent ADD COLUMN onchat_mode TEXT NOT NULL DEFAULT '';

-- C1 平台助手兜底（92 号 §5.2 原则②按 03 §2.54 决策注修订）：平台助手定位为对话建本体
-- 的显式引导面/万能渠道，默认开启（能力开关 driven 装配自动并入工具+纪律段，tools 列不动）；
-- 审计如实归平台助手 id（ontochat_start 经 tool.Caller 落 agent_id）。
UPDATE agent SET onchat_mode = 'guided' WHERE id = 'builtin-assistant' AND is_builtin = 1;
