-- REQ-558/M365 运行生命周期 pause/redo 与产物预览（结构化协作批 P1；107 号 §九）——
-- conversation.pause_state：pause 安全点挂起持久化（{checkpoint_id,run_id,paused_at} JSON）——
-- 独立列不复用 interrupt_state（Resume 强校验 TargetID，混用会逼「无答复恢复」伪装成审批答复）；
-- 跨进程重启可恢复（checkpoint 落库链——TurnLoop cleanup 保存 turnloop checkpoint）。
-- 迁移编号注：107 号 §9.3 撰写时点目录最新 078——本件开工日 ls migrations/ max+1 动态领取=086。
ALTER TABLE conversation ADD COLUMN pause_state TEXT NOT NULL DEFAULT '';
