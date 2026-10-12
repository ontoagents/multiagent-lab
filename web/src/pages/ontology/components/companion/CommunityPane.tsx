import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Card, Empty, Space, Spin, Tag, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { companionApi } from '../../../../api/companion'
import type { CompanionCommunity } from '../../../../api/companion'
import { useUI } from '../../../../store/ui'

// ---------------------------------------------------------------------------
// REQ-288/M93 伴生社区摘要（对齐 Graphiti community summaries；管线复用 KB GraphRAG
// REQ-130 形态）：Louvain 社区检测 + LLM 摘要（失败回退骨架）；eager ≤12 全量即时，
// 超出 pending 按需生成（KB-O4 成本口径）；成员不变重建时保留既有摘要不重烧 LLM。
// 消费面：全貌抽象问题（「这个伴生本体整体沉淀了什么」级提问）+ 检索零命中兜底注入臂。
// ---------------------------------------------------------------------------

const METHOD_META: Record<string, { color: string; text: string; tip: string }> = {
  llm: { color: 'green', text: 'LLM 摘要', tip: 'LLM 生成（缓存于社区表）' },
  skeleton: { color: 'default', text: '骨架摘要', tip: 'LLM 不可用时的成员+关系计数骨架' },
  'skeleton+tail': { color: 'default', text: '长尾聚合', tip: '小社区长尾合并' },
  pending: { color: 'orange', text: '待生成', tip: '按需生成（首次对话兜底或点「生成摘要」）' },
}

export default function CommunityPane({ ontologyId }: { ontologyId: string }) {
  const { showToast } = useUI()
  const [comms, setComms] = useState<CompanionCommunity[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [rebuilding, setRebuilding] = useState(false)
  const [summarizing, setSummarizing] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const r = await companionApi.communities(ontologyId)
      setComms(r.communities ?? [])
    } catch (e: any) {
      setComms([])
      showToast(e.message, 'err')
    } finally {
      setLoading(false)
    }
  }, [ontologyId, showToast])

  useEffect(() => {
    load()
  }, [load])

  const rebuild = async () => {
    setRebuilding(true)
    try {
      const r = await companionApi.rebuildCommunities(ontologyId)
      showToast(`社区检测完成：${r.count} 个社区（${r.detect_method}）${r.note ? `；${r.note}` : ''}`)
      await load()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setRebuilding(false)
    }
  }

  const summarize = async (label: string) => {
    setSummarizing(label)
    try {
      await companionApi.summarizeCommunity(ontologyId, label)
      await load()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setSummarizing(null)
    }
  }

  if (loading && comms === null) return <Spin style={{ display: 'block', margin: '40px auto' }} />

  return (
    <div>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        title="伴生社区摘要（REQ-288）"
        description="Louvain 社区检测把伴生图中紧密关联的实体聚成社区，LLM 为每个社区生成整体概括——回答「这个伴生本体整体沉淀了什么」级全貌问题；对话检索未命中具体实体时自动兜底引用相关社区。社区数 ≤12 重建即生成摘要；超出为按需生成（首次引用或手动生成，成本口径沿 KB-O4）。"
      />
      <Space style={{ marginBottom: 12 }}>
        <Button type="primary" loading={rebuilding} onClick={rebuild} data-testid="community-rebuild-btn">
          重建社区摘要
        </Button>
        <Button icon={<ReloadOutlined />} onClick={load}>
          刷新
        </Button>
      </Space>
      {comms !== null && comms.length === 0 && (
        <Empty description="尚未构建社区——先重建（需伴生图 ≥3 个实体；成员不变时重建不重烧 LLM）" />
      )}
      <Space direction="vertical" size={8} style={{ display: 'flex' }}>
        {(comms ?? []).map((c) => {
          const meta = METHOD_META[c.method] ?? { color: 'default', text: c.method, tip: '' }
          return (
            <Card
              key={c.label}
              size="small"
              title={
                <Space size={6} wrap>
                  <span>{c.label === '__tail__' ? '长尾社区' : `社区「${c.label}」`}</span>
                  <Tag style={{ margin: 0 }}>{c.members.length} 成员</Tag>
                  <Tag color={meta.color} style={{ margin: 0 }} title={meta.tip}>
                    {meta.text}
                  </Tag>
                </Space>
              }
              extra={
                (c.method === 'pending' || !c.summary) && (
                  <Button size="small" loading={summarizing === c.label} onClick={() => summarize(c.label)} data-testid={`community-summarize-${c.label}`}>
                    生成摘要
                  </Button>
                )
              }
            >
              <Typography.Paragraph style={{ marginBottom: 6 }} data-testid={`community-summary-${c.label}`}>
                {c.summary || '（待生成）'}
              </Typography.Paragraph>
              <Space size={4} wrap>
                {c.members.map((m) => (
                  <Tag key={m} style={{ margin: 0, fontSize: 11 }}>
                    {m}
                  </Tag>
                ))}
              </Space>
            </Card>
          )
        })}
      </Space>
    </div>
  )
}
