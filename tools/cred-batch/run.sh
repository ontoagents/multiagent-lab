#!/usr/bin/env bash
# REQ-394/M190：真机凭证批集中执行脚本（凭证可用窗口跑；结果人工回填台账）
set -uo pipefail
cd "$(dirname "$0")/../.."
pass=0; fail=0
run() { echo "=== $1 ==="; shift; if "$@"; then pass=$((pass+1)); else fail=$((fail+1)); fi; }

run "eval 组（伴生 gate/抽取）" go test -tags eval ./backend/internal/companion/...
if [ "${DSH_S3_LIVE:-}" = "1" ]; then
  run "dsh SDK 真机回归" go test ./backend/internal/inference/ -run Live -v
else
  echo "=== SKIP dsh SDK 真机（DSH_S3_LIVE 未设置） ==="
fi
if [ -n "${AGENTEVAL_AGENT_A:-}" ] && [ -n "${AGENTEVAL_AGENT_B:-}" ]; then
  run "agenteval A/B 全量（REQ-223/REQ-386 T1）" go test ./backend/internal/agenteval/ -run TestEval -v
else
  echo "=== SKIP agenteval A/B（AGENTEVAL_AGENT_A/B 未设置） ==="
fi
echo "结果：$pass 过 / $fail 败 —— 请将实测数据回填 01/02/03/18/20 台账（「待凭证」→实测值+日期）"
exit $fail
