-- REQ-232/M59（51 号 W4 拍板；随 REQ-225/M53 领取轮同批交付）：hooks 可配置化——
-- agent 级 hook 配置（fetchGuard 启停 + 自定义 deny 规则），JSON 单列；空 = 全默认（fetchGuard 启用、无自定义规则）。
ALTER TABLE agent ADD COLUMN hooks_json TEXT NOT NULL DEFAULT '';
