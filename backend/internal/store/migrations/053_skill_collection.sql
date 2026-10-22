-- 053 技能集合（REQ-373/M171）：一卡多子技能。
-- kind=''=普通技能；kind='collection'=集合容器——instruction 不注入（装配期按 children
-- 展开为成员目录，REQ-203 目录+load_skill 形态保留成员各自描述的触发粒度）、tools 不存
-- （装配期取成员并集）。成员须为普通技能（写入口径拦截集合嵌套）；删除成员由 store 层
-- 同步摘除各集合 children 中的引用。
ALTER TABLE skill ADD COLUMN kind TEXT NOT NULL DEFAULT '';
ALTER TABLE skill ADD COLUMN children TEXT NOT NULL DEFAULT '[]';
