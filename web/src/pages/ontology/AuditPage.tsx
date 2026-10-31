import { useEffect, useState } from 'react'
import { Button, Space, Tabs, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { api } from '../../api/client'
import type { RuntimeProfile } from '../../api/types'
import AuditObserveTab, { hasQualityWarn } from './components/audit/AuditObserveTab'
import AuditDecisionTab from './components/audit/AuditDecisionTab'
import AuditHomeTab from './components/audit/AuditHomeTab'

// ---------------------------------------------------------------------------
// 消费与审计（第五栏，D-O15/REQ-110；REQ-290/M94 内容置换）
// 栏定位=本体消费侧观测台（D-O19），REQ-290 把边界厘清从「文案与来源徽标」推进到
// 「内容归属」：KB 抽取 KG 的图谱观测与 GraphRAG 试查退役归知识库模块（能力在
// 知识库 GraphRAG「图谱与统计」「全局问答」全量在位），本栏回填本体消费内容——
// 消费观测（运行方案装载/质量快照 + TTL 关键词快查 + 消费面导航）+ 决策审计
// （onto_decision 跨模块留痕，默认 ontology 过滤）+ PROV-O 导出。零后端变更。
// ---------------------------------------------------------------------------

/** 跨栏跳转到本栏（与 BuildPage/RuntimePage 的 onto-sidebar-change 机制一致） */
export function gotoAuditPane() {
  localStorage.setItem('eino.onto.sidebar', 'audit')
  window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
}

export default function AuditPage() {
  const [profiles, setProfiles] = useState<RuntimeProfile[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [tab, setTab] = useState('observe')

  const loadProfiles = () => {
    setLoading(true)
    api
      .listRuntimeProfiles()
      .then((ps) => setProfiles(ps))
      .catch(() => setProfiles([]))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    loadProfiles()
  }, [])

  const running = (profiles ?? []).filter((p) => p.status === 'running')
  const warnCount = running.filter(hasQualityWarn).length

  return (
    <div className="main">
      <div className="work-main sema-main">
        <div className="work-head">
          <div className="work-head-text">
            <div className="work-head-title">
              <Typography.Title level={4} style={{ margin: 0 }}>
                消费与审计
              </Typography.Title>
              {/* REQ-490：页头 Tag 灰字化 */}
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>消费侧观测台 · 决策留痕</Typography.Text>
            </div>
            <p className="work-head-desc">
              观测本体装载后的消费面（消费一览 / 关键词快查 / 运行态实渲 / SPARQL / AI 消费 / 问题验收导航），构建与治理决策全程留痕可溯源。知识库文本抽取知识图谱的展示与治理在知识库模块。
            </p>
          </div>
          <Space size={8} wrap>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={loadProfiles}>
              刷新
            </Button>
          </Space>
        </div>

        {/* 状态条：运行方案聚合 + 质量低分警示（REQ-234① 口径，只警示不阻断）；REQ-490 三件套瘦身为灰字一行 */}
        <div className="sema-status" style={{ fontSize: 12.5, color: 'var(--c-ink-2, var(--c-ink))', padding: '2px 0 10px' }}>
          {profiles === null ? (
            <Typography.Text type="secondary">运行方案加载中…</Typography.Text>
          ) : running.length === 0 ? (
            <Typography.Text type="secondary">
              暂无运行中的本体方案——到「本体运行」栏启动（或创建）运行方案后，此处展示装载版本与质量快照。
            </Typography.Text>
          ) : warnCount > 0 ? (
            <Typography.Text type="warning" style={{ fontSize: 12.5 }}>
              {running.length} 个运行方案消费中 · 质量低分警示 ×{warnCount}（快照异步快评写入，仅警示不阻断）；消费面见「消费观测」、决策留痕见「决策审计」页签。
            </Typography.Text>
          ) : (
            <Typography.Text type="secondary">
              {running.length} 个运行方案消费中；消费面见「消费观测」、决策留痕见「决策审计」页签。
            </Typography.Text>
          )}
        </div>

        <Tabs
          activeKey={tab}
          onChange={setTab}
          items={[
            { key: 'observe', label: '消费观测', children: <AuditObserveTab profiles={profiles ?? []} loading={loading} onReload={loadProfiles} /> },
            { key: 'audit', label: '决策审计', children: <AuditDecisionTab /> },
            { key: 'home', label: '学习引导', children: <AuditHomeTab /> },
          ]}
        />

      </div>
    </div>
  )
}
