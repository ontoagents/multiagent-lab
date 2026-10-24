#!/usr/bin/env bash
# REQ-386/M182：T1 归因基准执行方案——同模型 einoADK vs dsh-acp A/B（57 号 T1「零代码」落码形态）。
# 前置：①设置页配两个智能体（同一模型连接；A=einoADK 默认后端，B=dsh-acp 外部后端）
#      ②backend 运行中（默认 :8080）；③模型凭证可用
# 用法：AGENT_A=<id> AGENT_B=<id> bash tools/agenteval/t1-ab.sh
set -euo pipefail
: "${AGENT_A:?需设置 A 智能体 id（einoADK 后端）}"
: "${AGENT_B:?需设置 B 智能体 id（dsh-acp 后端）}"
cd "$(dirname "$0")/../.."
export AGENTEVAL_AGENT_A="$AGENT_A" AGENTEVAL_AGENT_B="$AGENT_B"
echo "=== T1 归因基准：A=$AGENT_A（einoADK） vs B=$AGENT_B（dsh-acp） ==="
go test ./backend/internal/agenteval/ -run TestEval -v -count=1 2>&1 | tee /tmp/t1-ab-report.log
echo "=== 报告已存 /tmp/t1-ab-report.log —— 据数据拍板：补外部 harness 治理面（51 号 W5）或收缩外部 CLI 线 ==="
