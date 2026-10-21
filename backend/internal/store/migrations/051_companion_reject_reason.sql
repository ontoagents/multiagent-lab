-- REQ-358/M158 随批小件（EvoOntology 借鉴·93 号 §7.2 Reject 归因留档）：伴生候选拒绝归因——
-- 自动入图基准门控（事前评估旁挂）拒绝时写入门控归因（benchmark_recall/gate:<detail>），
-- 人工拒绝保持空串（存量零变化）。完整三层归因不引入（93 号 §7.2 口径）。
ALTER TABLE companion_candidate ADD COLUMN reject_reason TEXT NOT NULL DEFAULT '';
