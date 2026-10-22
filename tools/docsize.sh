#!/usr/bin/env bash
# 文档体量度量（REQ-375 P1，G-7 滚动压缩治理的落地工具）
# 交付回写时顺手跑：单档超阈值即触发指针化/滚动压缩（纪律 #8）；阈值可用 DOC_SIZE_THRESHOLD 覆盖。
cd "$(dirname "$0")/.." || exit 1
THRESH="${DOC_SIZE_THRESHOLD:-150000}"
printf '%-42s %10s %8s %8s  %s\n' 'FILE' 'BYTES' 'LINES' 'MAXLINE' 'NOTE'
for f in AGENTS.md docs/*.md; do
  [ -f "$f" ] || continue
  bytes=$(wc -c < "$f")
  lines=$(wc -l < "$f")
  maxline=$(awk '{ if (length($0)>m) m=length($0) } END { print m+0 }' "$f")
  note=''
  [ "$bytes" -gt "$THRESH" ] && note="超阈值 ${THRESH}——检查行内长交付注可否指针化（纪律 #8）"
  printf '%-42s %10d %8d %8d  %s\n' "$f" "$bytes" "$lines" "$maxline" "$note"
done
