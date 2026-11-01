#!/usr/bin/env bash
# REQ-556/M363 演示重置（107 号 §7.3）——按「演示·」前缀走既有 DELETE API 清理（幂等；
# 本体删除受绑定者 409 保护——409 实体如实提示不强制删）。
# 用法：BASE=http://127.0.0.1:8080 PLATFORM_TOKEN=xxx ./demo-reset.sh
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"
AUTH=()
if [ -n "${PLATFORM_TOKEN:-}" ]; then
  AUTH=(-H "X-Platform-Token: ${PLATFORM_TOKEN}")
fi

api() { curl -sS "${AUTH[@]}" "$@"; }

echo "== 演示 reset（${BASE}）=="

# 会话（title 前缀 演示·）
api "${BASE}/api/conversations" | python3 -c "
import json, sys
data = json.load(sys.stdin)
items = data if isinstance(data, list) else data.get('conversations') or []
for it in items:
    if (it.get('title') or '').startswith('演示·'):
        print(it['id'])
" 2>/dev/null | while read -r id; do
  [ -n "$id" ] && api -X DELETE "${BASE}/api/conversations/${id}" -o /dev/null -w "✓ 删除会话 ${id} (%{http_code})\n" || true
done

# 项目（name 前缀 演示·）
api "${BASE}/api/projects" | python3 -c "
import json, sys
data = json.load(sys.stdin)
items = data if isinstance(data, list) else data.get('projects') or []
for it in items:
    if (it.get('name') or '').startswith('演示·'):
        print(it['id'])
" 2>/dev/null | while read -r id; do
  [ -n "$id" ] && api -X DELETE "${BASE}/api/projects/${id}" -o /dev/null -w "✓ 删除项目 ${id} (%{http_code})\n" || true
done

# 智能体（name 前缀 演示·；内置行 API 自拒）
api "${BASE}/api/agents" | python3 -c "
import json, sys
data = json.load(sys.stdin)
items = data if isinstance(data, list) else data.get('agents') or []
for it in items:
    if (it.get('name') or '').startswith('演示·') and not it.get('is_builtin'):
        print(it['id'])
" 2>/dev/null | while read -r id; do
  [ -n "$id" ] && api -X DELETE "${BASE}/api/agents/${id}" -o /dev/null -w "✓ 删除智能体 ${id} (%{http_code})\n" || true
done

echo "== reset 完成（「演示·」前缀实体已清理）=="
echo "说明：被引用保护（409）的实体如实提示——先解除引用再重跑本脚本。"
