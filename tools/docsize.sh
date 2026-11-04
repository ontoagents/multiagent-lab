#!/usr/bin/env bash
# 文档体量度量（REQ-375 P1，G-7 滚动压缩治理的落地工具）
# 修改大档时查看体量；仅提示，不扩大当前任务范围（AGENTS 纪律 10）。
# 阈值可用 DOC_SIZE_THRESHOLD 覆盖。
cd "$(dirname "$0")/.." || exit 1
THRESH="${DOC_SIZE_THRESHOLD:-150000}"
printf '%-42s %10s %8s %8s  %s\n' 'FILE' 'BYTES' 'LINES' 'MAXLINE' 'NOTE'
for f in AGENTS.md docs/*.md; do
  [ -f "$f" ] || continue
  bytes=$(wc -c < "$f")
  lines=$(wc -l < "$f")
  maxline=$(awk '{ if (length($0)>m) m=length($0) } END { print m+0 }' "$f")
  note=''
  [ "$bytes" -gt "$THRESH" ] && note="超阈值 ${THRESH}——优先压缩本次新增长注，旧档另行治理（纪律 10）"
  printf '%-42s %10d %8d %8d  %s\n' "$f" "$bytes" "$lines" "$maxline" "$note"
done
