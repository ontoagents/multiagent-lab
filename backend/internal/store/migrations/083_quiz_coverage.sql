-- REQ-545/M352 证据槽/缺槽结构化缺口（决策结构化借鉴批②；105 号 P1-3）——
-- quiz_item.coverage_verdict：出题预检裁决（''=未预检 | answerable | not_answerable；
-- check_answerable 确定性预检随出题落库，QuizPane 徽标与 CqQuickScreen 缺口清单数据源）。
-- 迁移编号注：REQ-545 行内原引 079 已被 REQ-548 工作区批占用（074/075 让位先例）——
-- 本件开工日 ls migrations/ max+1 动态领取=083。
ALTER TABLE quiz_item ADD COLUMN coverage_verdict TEXT NOT NULL DEFAULT '';
