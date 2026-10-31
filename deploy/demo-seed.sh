#!/usr/bin/env bash
# REQ-556/M363 一键演示 seed（结构化协作批 P0；107 号 §7.3）——bash+curl 零 Python（D-O15）。
# 幂等灌装演示资产：「演示·」命名前缀查重跳过（两跑零重复）；重置见 demo-reset.sh。
# 灌装内容：演示智能体×1 + 演示项目×1 + 种子任务会话×2（简报为全新文案——107 号 §7.3 硬性要求
# 不复用 REQ-417 模板内历史叙事字样）。零新后端面（全走既有 REST）；**不自动触发 LLM 轮**——
# seed 只灌配置与内容，模型 Key 由演示者自行决定（本地零 Key 预设或自有 Key）。
# 用法：BASE=http://127.0.0.1:8080 PLATFORM_TOKEN=xxx ./demo-seed.sh
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"
AUTH=()
if [ -n "${PLATFORM_TOKEN:-}" ]; then
  AUTH=(-H "X-Platform-Token: ${PLATFORM_TOKEN}")
fi

api() { curl -sS "${AUTH[@]}" -H 'Content-Type: application/json' "$@"; }

# 幂等查重：同名实体存在即跳过
find_by_name() { # $1=list-url  $2=name  $3=id-key
  api "$1" | python3 - "$2" "$3" <<'PY' 2>/dev/null || echo ""
import json, sys
name, key = sys.argv[1], sys.argv[2]
data = json.load(sys.stdin)
items = data if isinstance(data, list) else data.get("agents") or data.get("projects") or data.get("conversations") or []
for it in items:
    if it.get("name") == name or it.get("title") == name:
        print(it.get(key, ""))
        break
PY
}

echo "== 演示 seed（${BASE}）=="

# ① 演示智能体（规划者角色示例；instruction 全新文案）
AGENT_ID=$(find_by_name "${BASE}/api/agents" "演示·研发协调者" "id")
if [ -z "$AGENT_ID" ]; then
  AGENT_ID=$(api -X POST "${BASE}/api/agents" -d '{
    "name": "演示·研发协调者",
    "description": "演示环境的示例智能体：面向小型研发任务的多步骤规划与执行（演示文案·全新撰写）",
    "instruction": "你是小型研发任务的协调者：先拆解任务为 2~4 个步骤，逐步执行并汇报；遇到需要用户决策的分叉点，明确列出选项等待选择。语言简洁，结论先行。"
  }' | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))")
  echo "✓ 演示·研发协调者 = $AGENT_ID"
else
  echo "· 演示·研发协调者 已存在，跳过"
fi

# ② 演示项目
PROJECT_ID=$(find_by_name "${BASE}/api/projects" "演示·示例项目" "id")
if [ -z "$PROJECT_ID" ]; then
  PROJECT_ID=$(api -X POST "${BASE}/api/projects" -d '{
    "name": "演示·示例项目",
    "description": "演示环境示例项目（全新文案）：承载演示智能体与任务会话",
    "agent_ids": ["'"$AGENT_ID"'"],
    "coordinator": "'"$AGENT_ID"'"
  }' | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))")
  echo "✓ 演示·示例项目 = $PROJECT_ID"
else
  echo "· 演示·示例项目 已存在，跳过"
fi

# ③ 种子任务会话×2（简报为全新文案——各一句话任务）
if [ -n "$PROJECT_ID" ]; then
  for TASK in "演示·任务A：小工具选型对比" "演示·任务B：文档结构调整"; do
    CONV_ID=$(find_by_name "${BASE}/api/conversations?project_id=${PROJECT_ID}" "$TASK" "id")
    if [ -z "$CONV_ID" ]; then
      CONV_ID=$(api -X POST "${BASE}/api/conversations" -d '{
        "agent_id": "'"$AGENT_ID"'", "project_id": "'"$PROJECT_ID"'",
        "title": "'"$TASK"'", "scope": "project"
      }' | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))")
      echo "✓ $TASK = $CONV_ID"
    else
      echo "· $TASK 已存在，跳过"
    fi
  done
fi

echo "== seed 完成（幂等——重复执行零重复实体）=="
echo "说明：seed 只灌配置与内容，不触发 LLM 轮；模型 Key 请在「设置-模型连接」自行配置。"
