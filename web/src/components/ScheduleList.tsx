import { useEffect, useState } from 'react'
import { Button, Table, Tag, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { api } from '../api/client'
import type { Conversation } from '../api/types'

// REQ-412/M208：定时任务列表面——conversation_schedule 全量清单（会话/间隔/进度/max_runs）
// + 摘除操作。周期粒度=interval_minutes（1~10080min≤7 天，REQ-412 放宽）；cron 表达式不引入。

interface ScheduleRow {
  conversation_id: string
  interval_minutes: number
  max_runs: number
  done: number
  created_at: string
  updated_at: string
}

function fmtInterval(m: number): string {
  if (m >= 1440 && m % 1440 === 0) return `${m / 1440} 天`
  if (m >= 60 && m % 60 === 0) return `${m / 60} 小时`
  return `${m} 分钟`
}

export default function ScheduleList() {
  const [rows, setRows] = useState<ScheduleRow[]>([])
  const [convs, setConvs] = useState<Record<string, Conversation>>({})
  const [loading, setLoading] = useState(false)

  const load = () => {
    setLoading(true)
    Promise.all([api.listSchedules(), api.listConversations()])
      .then(([ss, cs]) => {
        setRows(ss)
        const m: Record<string, Conversation> = {}
        for (const c of cs) m[c.id] = c
        setConvs(m)
      })
      .catch(() => setRows([]))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  const remove = async (convId: string) => {
    await api.deleteSchedule(convId)
    load()
  }

  return (
    <div data-testid="schedule-list">
      <Button size="small" icon={<ReloadOutlined />} onClick={load} style={{ marginBottom: 8 }}>
        刷新
      </Button>
      <Table<ScheduleRow>
        rowKey="conversation_id"
        size="small"
        loading={loading}
        pagination={false}
        dataSource={rows}
        columns={[
          {
            title: '会话',
            dataIndex: 'conversation_id',
            ellipsis: true,
            render: (v: string) => convs[v]?.title || v.slice(0, 8),
          },
          { title: '间隔', dataIndex: 'interval_minutes', width: 90, render: (v: number) => <Tag style={{ margin: 0 }}>每 {fmtInterval(v)}</Tag> },
          {
            title: '进度',
            width: 90,
            render: (_: unknown, r: ScheduleRow) => `${r.done}/${r.max_runs}`,
          },
          {
            title: '操作',
            width: 80,
            render: (_: unknown, r: ScheduleRow) => (
              <Button size="small" type="link" danger style={{ padding: 0 }} onClick={() => remove(r.conversation_id)}>
                摘除
              </Button>
            ),
          },
        ]}
      />
      {rows.length === 0 && !loading && (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          暂无定时任务——会话设置（头部 ⋮）可配置定时续跑。
        </Typography.Text>
      )}
    </div>
  )
}
