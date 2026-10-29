#!/usr/bin/env bash
# REQ-386/M182：T1 归因基准执行方案——同模型 einoADK vs dsh-acp A/B（57 号 T1「零代码」落码形态）。
# 前置：①设置页配两个智能体（同一模型连接；A=eino-adk 默认后端，B=deepseek-harness-acp 外部后端）
#      ②backend 运行中（默认 :8080）；③模型凭证可用；④dsh 凭据就绪（B 路经 backend 环境继承
#        DEEPSEEK_API_KEY——dsh 凭据服务或环境变量一次性配置，MISSING_CREDENTIAL 时 run.error 透出）
# 用法：AGENT_A=<id> AGENT_B=<id> bash tools/agenteval/t1-ab.sh
#  2026-10-29 真机修正：eval_test.go 有 //go:build agenteval tag（真机手动跑不进 CI），原脚本
#  缺 -tags agenteval 且 -run TestEval 名不匹配（实际 TestAgentEval）、cd 到仓库根导致 internal
#  包不可达——三处顺修后真机可跑。
set -euo pipefail
: "${AGENT_A:?需设置 A 智能体 id（eino-adk 后端）}"
: "${AGENT_B:?需设置 B 智能体 id（deepseek-harness-acp 后端）}"
cd "$(dirname "$0")/../../backend"
# 变量名对齐 eval_test.go 契约：A 路=AGENTEVAL_AGENT（单数）、B 路=AGENTEVAL_AGENT_B
export AGENTEVAL_AGENT="$AGENT_A" AGENTEVAL_AGENT_B="$AGENT_B"
echo "=== T1 归因基准：A=$AGENT_A（eino-adk） vs B=$AGENT_B（deepseek-harness-acp） ==="
go test -tags agenteval -timeout 20m ./internal/agenteval/ -run TestAgentEval -v -count=1 2>&1 | tee /tmp/t1-ab-report.log
echo "=== 报告已存 /tmp/t1-ab-report.log —— 据数据拍板：补外部 harness 治理面（51 号 W5）或收缩外部 CLI 线 ==="
