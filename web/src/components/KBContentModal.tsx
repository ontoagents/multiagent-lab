import { useEffect, useRef, useState } from 'react'
import { Drawer, Spin, Tag, Typography } from 'antd'
import { api } from '../api/client'
import { DRAWER_SIZES } from '../lib/layout'

/**
 * KB 文档原文查看（REQ-523/M330 溯源最后一跳）：命中卡/试运行命中点「doc 名」→
 * chunk 按 seq 拼接原文（GET /api/kb/{id}/docs/{did}/content）弹层呈现，#seq 锚点定位。
 * 壳沿 DocViewerModal 范式（默认关闭、点开才挂载、可手动关闭）；与文件系统 DocViewer
 * 不同源——原文仅在 KnowledgeChunk，走内容端点聚合（不照搬 path 读文件白名单）。
 */
export default function KBContentModal({
  target,
  onClose,
}: {
  /** null=关闭；非空={kbId, docId, doc 名, seq 定位锚点} */
  target: { kbId: string; docId: string; doc: string; seq: number } | null
  onClose: () => void
}) {
  const [chunks, setChunks] = useState<{ seq: number; content: string }[]>([])
  const [docTitle, setDocTitle] = useState('')
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const seqRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!target) return
    setLoading(true)
    setErr(null)
    setChunks([])
    api
      .kbDocContent(target.kbId, target.docId)
      .then((r) => {
        setDocTitle(r.doc || target.doc)
        setChunks(r.chunks)
      })
      .catch((e) => setErr(e.message))
      .finally(() => setLoading(false))
  }, [target])

  // #seq 锚点定位（命中片段所在 chunk 滚动到视口）
  useEffect(() => {
    if (!target || loading || err) return
    const t = setTimeout(() => {
      seqRef.current?.scrollIntoView({ block: 'center' })
    }, 80)
    return () => clearTimeout(t)
  }, [target, loading, err])

  return (
    <Drawer
      open={!!target}
      onClose={onClose}
      title={
        <Typography.Text strong style={{ fontSize: 14 }}>
          原文 · {docTitle || target?.doc || ''}
        </Typography.Text>
      }
      width={DRAWER_SIZES.medium}
    >
      {loading && <Spin style={{ display: 'block', margin: '40px auto' }} />}
      {err && <Typography.Text type="danger">加载失败：{err}</Typography.Text>}
      {!loading && !err && chunks.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {chunks.map((c) => (
            <div
              key={c.seq}
              ref={target && c.seq === target.seq ? seqRef : undefined}
              style={{
                padding: '8px 10px',
                borderRadius: 6,
                border: target && c.seq === target.seq ? '1px solid var(--c-brand, #4b6bfb)' : '1px solid var(--c-border, #d9dcec)',
                background: target && c.seq === target.seq ? 'var(--c-bg-soft, #f6f7fb)' : undefined,
              }}
            >
              <Tag style={{ margin: '0 0 4px', fontSize: 11 }}>#{c.seq}</Tag>
              <div className="chat-md" style={{ fontSize: 13, lineHeight: 1.75, whiteSpace: 'pre-wrap' }}>
                {c.content}
              </div>
            </div>
          ))}
        </div>
      )}
    </Drawer>
  )
}
