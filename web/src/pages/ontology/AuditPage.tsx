import { useEffect, useState } from 'react'
import { Button, Drawer, Space, Tabs, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { api } from '../../api/client'
import type { RuntimeProfile } from '../../api/types'
import AuditObserveTab, { hasQualityWarn } from './components/audit/AuditObserveTab'
import AuditDecisionTab from './components/audit/AuditDecisionTab'
import AuditGuideContent from './components/audit/AuditHomeTab'
import { DRAWER_SIZES, drawerSizeProps } from '../../lib/layout'

// ---------------------------------------------------------------------------
// 消费与审计（第五栏，D-O15/REQ-110；REQ-290/M94 内容置换；REQ-599/M410 两页签重排）
// 栏定位=本体消费侧观测台（D-O19）。页签从三减二：消费观测（回答 T2 装载健康/T3
// 消费复盘/T4 查询排查）+ 决策留痕（回答 T1 改完回看/T5 合规导出）；学习引导页签
// 退役——三段式内容迁页头「使用说明」抽屉（内容保留载体演进）。
// REQ-599：loadProfiles 错误态修复（错误×空态互斥——「后端不可达」与「暂无方案」
// 视觉可区分）；页头刷新钮语义收拢（一处刷新刷全部，reloadKey 下传两页签）。
// 知识库抽取 KG 的观测与治理仍在知识库模块（D-O19 边界不动）。
// ---------------------------------------------------------------------------

/** 跨栏跳转到本栏（与 BuildPage/RuntimePage 的 onto-sidebar-change 机制一致） */
export function gotoAuditPane() {
  localStorage.setItem('eino.onto.sidebar', 'audit')
  window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
}

export default function AuditPage() {
  const [profiles, setProfiles] = useState<RuntimeProfile[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [tab, setTab] = useState('observe')
  // REQ-599：页头刷新语义收拢——一处刷新刷全部（profiles+两页签数据）
  const [reloadKey, setReloadKey] = useState(0)
  const [guideOpen, setGuideOpen] = useState(false)

  const loadProfiles = () => {
    setLoading(true)
    api
      .listRuntimeProfiles()
      .then((ps) => {
        setProfiles(ps)
        setLoadErr(null)
      })
      .catch((e: any) => {
        setLoadErr(e?.message ?? '运行方案获取失败')
        setProfiles([])
      })
      .finally(() => setLoading(false))
  }

  const refreshAll = () => {
    loadProfiles()
    setReloadKey((k) => k + 1)
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
            {/* REQ-599：使用说明灰字链接（学习引导页签退役，内容迁此抽屉） */}
            <Button size="small" type="link" style={{ padding: 0 }} onClick={() => setGuideOpen(true)} data-testid="audit-guide-link">
              使用说明
            </Button>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={refreshAll} data-testid="audit-refresh-all">
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
              {running.length} 个运行方案消费中 · 质量低分警示 ×{warnCount}（快照异步快评写入，仅警示不阻断）。
            </Typography.Text>
          ) : (
            <Typography.Text type="secondary">
              {running.length} 个运行方案消费中。
            </Typography.Text>
          )}
        </div>

        {/* REQ-599：错误×空态互斥——后端不可达显 LoadErrorAlert（重试钮），不与空态混排 */}
        {loadErr && (
          <div style={{ margin: '6px 0 10px' }} data-testid="audit-load-error">
            {/* LoadErrorAlert 由各页签内嵌；此处页级提示承载方案加载失败 */}
            <Typography.Text type="danger" style={{ fontSize: 12.5 }}>
              运行方案加载失败：{loadErr}
            </Typography.Text>
            <Button size="small" type="link" onClick={loadProfiles}>
              重试
            </Button>
          </div>
        )}

        <Tabs
          activeKey={tab}
          onChange={setTab}
          items={[
            { key: 'observe', label: '消费观测', children: <AuditObserveTab profiles={profiles ?? []} loading={loading} reloadKey={reloadKey} /> },
            { key: 'audit', label: '决策留痕', children: <AuditDecisionTab reloadKey={reloadKey} /> },
          ]}
        />

        <Drawer
          open={guideOpen}
          onClose={() => setGuideOpen(false)}
          title="使用说明 · 本体消费链路"
          {...drawerSizeProps('audit-guide', DRAWER_SIZES.medium)}
        >
          <AuditGuideContent />
        </Drawer>

      </div>
    </div>
  )
}
