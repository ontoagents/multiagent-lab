import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Space, Spin, Tag, Typography } from 'antd'
import { CaretRightOutlined, ExperimentOutlined } from '@ant-design/icons'
import { api } from '../../../api/client'

// ---------------------------------------------------------------------------
// CQ 覆盖快筛就地化（REQ-500/M308，P15 定位件优先例外）：
// 入库完成态（DoneCTA 旁）默认触发 CQ 覆盖快筛——「建完即可测」完成仪式；
// 逐条三态徽标（可答/存疑/不可答）+不可答条目级缺口清单带「去补概念/加同义词」直达钮
//（复用覆盖测试站既有 cq-coverage 管线，不新造入口）。
// 成本护栏三条：①CQ 条数上限 10 ②预估成本中性文案 ③同版本命中缓存不重跑
//（localStorage eino.cqscreen.{oid}，复用 QualityCardPane lastReport 版本比对反向使用口径）。
// ---------------------------------------------------------------------------

const SCREEN_CQ_CAP = 10

type Verdict = { cq: string; verdict: string; explanation: string; missing?: string[] }
type ScreenReport = { version: string; verdicts: Verdict[]; passed: number; total: number }

function cacheKey(oid: string) { return `eino.cqscreen.${oid}` }

function loadCache(oid: string, version: string): ScreenReport | null {
  try {
    const raw = localStorage.getItem(cacheKey(oid))
    if (!raw) return null
    const rep = JSON.parse(raw) as ScreenReport
    if (rep.version === version && rep.verdicts?.length) return rep
  } catch { /* 缓存损坏忽略——重跑 */ }
  return null
}

export default function CqQuickScreen({ ontologyId, autoStart = false }: { ontologyId: string; autoStart?: boolean }) {
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [report, setReport] = useState<ScreenReport | null>(null)
  const [noCq, setNoCq] = useState(false)
  const startedRef = useRef(false)

  const go = (section?: string) => {
    localStorage.setItem('eino.onto.sidebar', 'assets')
    if (ontologyId) localStorage.setItem('eino.onto.asset.focus', ontologyId)
    if (section) localStorage.setItem('eino.onto.asset.section', section)
    window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
  }

  // 快筛执行：CQ 上限截取→job 提交→轮询→缓存落位
  const run = async (version: string) => {
    setBusy(true)
    setErr(null)
    setNoCq(false)
    try {
      // 取 spec.CQ（经质量卡同源：cqSparql 端点返回 items 即 CQ 集合，缺省全量时端点内取 spec.CQ；
      // 上限护栏在提交侧截取——cqs 显式传入 ≤10 条）
      let cqs: string[] = []
      try {
        const r = await api.cqSparql(ontologyId)
        cqs = (r.items ?? []).map((it: { cq: string }) => it.cq).filter(Boolean)
      } catch { /* 端点失败走端点内缺省（spec.CQ） */ }
      if (cqs.length > SCREEN_CQ_CAP) cqs = cqs.slice(0, SCREEN_CQ_CAP)
      const j = await api.coverageCQTest(ontologyId, cqs.length > 0 ? cqs : undefined)
      const timer = setInterval(async () => {
        try {
          const job = await api.getOntoChatJob(j.job_id)
          if (job.progress) setProgress(job.progress)
          if (job.status === 'done' || job.status === 'error' || job.status === 'cancelled') {
            clearInterval(timer)
            setBusy(false)
            setProgress('')
            if (job.status !== 'done') {
              setErr(job.error ?? '快筛未完成')
              return
            }
            const result = job.result as { verdicts: Verdict[]; passed: number; total: number }
            const rep: ScreenReport = { version, verdicts: result.verdicts ?? [], passed: result.passed ?? 0, total: result.total ?? 0 }
            setReport(rep)
            try { localStorage.setItem(cacheKey(ontologyId), JSON.stringify(rep)) } catch { /* 存储满忽略 */ }
          }
        } catch { /* 单次轮询失败下次重试 */ }
      }, 1500)
    } catch (e: any) {
      setBusy(false)
      const msg = e?.message ?? '提交失败'
      if (msg.includes('spec.CQ 为空')) setNoCq(true)
      else setErr(msg)
    }
  }

  // 同版本缓存命中即渲染（护栏③）；autoStart 时未命中自动触发（学习动线默认开）
  useEffect(() => {
    if (!ontologyId || startedRef.current) return
    startedRef.current = true
    ;(async () => {
      let version = ''
      try { version = String((await api.getOntology(ontologyId)).version ?? '') } catch { /* 版本未知=缓存不命中 */ }
      const cached = loadCache(ontologyId, version)
      if (cached) {
        setReport(cached)
        return
      }
      if (autoStart) void run(version)
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ontologyId])

  const verdictTag = (v: string) => {
    if (v === 'Yes') return <Tag color="green" style={{ margin: 0 }}>可答</Tag>
    if (v === 'No') return <Tag color="red" style={{ margin: 0 }}>不可答</Tag>
    return <Tag color="orange" style={{ margin: 0 }}>存疑</Tag>
  }

  return (
    <div className="onto-cq-quickscreen" data-testid="cq-quickscreen" style={{ marginTop: 10 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <Typography.Text strong style={{ fontSize: 13 }}>
          <ExperimentOutlined /> CQ 覆盖快筛（建完即可测）
        </Typography.Text>
        <Button
          size="small"
          icon={<CaretRightOutlined />}
          loading={busy}
          disabled={busy}
          onClick={() => {
            let version = ''
            void api.getOntology(ontologyId).then((o: { version?: number }) => { version = String(o.version ?? '') })
            void run(version)
          }}
          data-testid="cq-screen-run"
        >
          {report ? '重跑快筛' : '运行快筛'}
        </Button>
        {report && (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            可答 {report.passed}/{report.total}（base_version {report.version || '—'}——同版本命中缓存不重跑）
          </Typography.Text>
        )}
      </div>
      <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 2 }}>
        快筛按能力问题逐条判「本体能否结构化回应」（约每条 1 次 LLM 调用；最多取前 {SCREEN_CQ_CAP} 条）；完整双轨测试见「资产 → 质量卡 → 覆盖测试站」。
      </Typography.Text>

      {busy && (
        <div style={{ marginTop: 6 }}>
          <Spin size="small" /> <Typography.Text type="secondary" style={{ fontSize: 12 }}>{progress || '提交中…'}</Typography.Text>
        </div>
      )}
      {err && <Alert type="warning" showIcon style={{ marginTop: 6 }} title={err} closable onClose={() => setErr(null)} />}
      {noCq && (
        <Alert
          type="info"
          showIcon
          style={{ marginTop: 6 }}
          title="本体的能力问题（CQ）为空——先在构建会话抽取 CQ 并确认，再回来快筛"
          action={<Button size="small" onClick={() => go('decl')}>去构建会话</Button>}
        />
      )}
      {report && report.verdicts.length > 0 && (
        <div style={{ marginTop: 6 }} data-testid="cq-screen-results">
          {report.verdicts.map((v, i) => (
            <div key={i} style={{ padding: '4px 0', borderBottom: '1px dashed var(--c-line)' }}>
              <Space size={6} align="start" style={{ width: '100%' }}>
                {verdictTag(v.verdict)}
                <Typography.Text style={{ fontSize: 12, flex: 1 }}>{v.cq}</Typography.Text>
              </Space>
              {v.explanation && (
                <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginLeft: 52 }}>{v.explanation}</Typography.Text>
              )}
              {/* REQ-545/M352：存疑/不可答态渲染结构化缺口清单 + go() 直达钮（原仅 No 态有钮——存疑态同权） */}
              {v.verdict !== 'Yes' && (
                <Space size={4} style={{ marginLeft: 52, marginTop: 2 }} data-testid={`cq-screen-gaps-${i}`}>
                  {(v.missing?.length ?? 0) > 0 && (
                    <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                      缺口：{v.missing!.slice(0, 5).join('、')}{v.missing!.length > 5 ? '…' : ''}
                    </Typography.Text>
                  )}
                  <Button size="small" type="link" style={{ padding: 0 }} onClick={() => go('hierarchy')}>去补概念</Button>
                  <Button size="small" type="link" style={{ padding: 0 }} onClick={() => go('decl')}>加同义词</Button>
                </Space>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
