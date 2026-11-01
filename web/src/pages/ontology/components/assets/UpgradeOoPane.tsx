import { useState } from 'react'
import { Alert, Button, Descriptions, Modal, Space, Tag, Typography } from 'antd'
import { ArrowUpOutlined } from '@ant-design/icons'
import { api } from '../../../../api/client'
import type { ImportReport, Ontology } from '../../../../api/types'
import { useUI } from '../../../../store/ui'
import { invalidateOntos } from '../../shared/useOntoList'

// ---------------------------------------------------------------------------
// REQ-258/M382 spec→oo 单向升级桥（60 号 H7；D-O22 批次四——依赖 REQ-268 数据属性已交付）：
// spec_json 本体 → 导出 Turtle（含 rdfs:subClassOf 层级公理 + owl:DatatypeProperty 数据属性
// 声明 + Restrictions 限制公理，导出器既有全量）→ POST /api/ontologies/import 走导入审查底座
// 复核入库为 oo 轨资产（原始形态保留+对账块）——审查报告（lossy/warnings/unprojected/
// reconciliation 恒等式）在 Modal 内复核，保留/放弃二择。**单向**：oo→spec_json 反向互通维持
// REQ-78 冻结（两轨分工口径），本动作只开 spec→oo 方向。
// ---------------------------------------------------------------------------

export function UpgradeOoPane({ ontology, onUpgraded }: { ontology: Ontology; onUpgraded?: (id: string) => void }) {
  const { showToast } = useUI()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [created, setCreated] = useState<Ontology | null>(null)
  const [report, setReport] = useState<ImportReport | null>(null)
  const [error, setError] = useState<string | null>(null)

  const run = async () => {
    setBusy(true)
    setError(null)
    setCreated(null)
    setReport(null)
    try {
      // ① spec → Turtle（导出器全量：subClassOf 层级公理/DatatypeProperty/Restrictions）
      const res = await fetch(api.ontologyExportUrl(ontology.id, 'turtle'))
      if (!res.ok) throw new Error(`TTL 导出失败（HTTP ${res.status}）`)
      const ttl = await res.text()
      // ② TTL → 导入审查底座（sidecar round-trip+对账块），复核后入库为 oo 轨资产
      const r = await api.importOntologyContent(`${ontology.name}-oo.ttl`, ttl, `${ontology.name}（oo 轨）`)
      setCreated(r.ontology)
      setReport(r.report)
      // 注意：此处不 invalidateOntos——列表刷新会让详情区 active 一帧 undefined、本组件
      // unmount 丢 Modal 审查态（REQ-258 冒烟爬坑）；清单失效延迟到保留/放弃时点。
    } catch (e: any) {
      setError(e?.message ?? '升级失败')
    } finally {
      setBusy(false)
    }
  }

  const discard = async () => {
    if (!created) return
    try {
      await api.deleteOntology(created.id)
      invalidateOntos()
      showToast('已放弃，oo 轨资产已删除')
    } catch (e: any) {
      showToast(e?.message ?? '删除失败——可在资产列表手动删除', 'err')
    }
    setOpen(false)
  }

  const keep = () => {
    invalidateOntos() // oo 轨资产入清单（生成阶段不刷——防 Modal 审查态被列表刷新冲掉）
    showToast(`oo 轨资产已保留：「${created?.name}」`)
    onUpgraded?.(created!.id)
    setOpen(false)
  }

  const rec = report?.reconciliation

  return (
    <>
      <Button icon={<ArrowUpOutlined />} onClick={() => setOpen(true)} data-testid="upgrade-oo-go">
        升级到 oo 轨
      </Button>
      <Modal
        open={open}
        onCancel={() => !busy && setOpen(false)}
        width={680}
        title="升级到 oo 轨（spec → OWL/TTL 单向桥）"
        footer={
          created ? (
            <Space>
              <Button danger onClick={discard} data-testid="upgrade-oo-discard">
                放弃并删除
              </Button>
              <Button type="primary" onClick={keep} data-testid="upgrade-oo-keep">
                保留 oo 轨资产
              </Button>
            </Space>
          ) : (
            <Space>
              <Button onClick={() => setOpen(false)} disabled={busy}>
                取消
              </Button>
              <Button type="primary" loading={busy} onClick={run} data-testid="upgrade-oo-run">
                生成 Turtle 并走导入审查
              </Button>
            </Space>
          )
        }
      >
        <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 10 }}>
          把当前 spec_json 本体导出为 OWL/TTL（含 rdfs:subClassOf 层级公理、owl:DatatypeProperty 数据属性声明、限制公理），再经导入审查底座复核入库为 oo 轨资产（原始形态保留+对账块）。升级为<b>单向</b>：oo→spec_json 反向互通维持冻结（两轨分工口径），oo 轨资产以导入审查报告为准。
        </Typography.Paragraph>

        {error && (
          <Alert type="error" showIcon style={{ marginBottom: 10 }} title="升级失败" description={error} />
        )}

        {busy && <Typography.Text type="secondary">导出并导入审查中（sidecar round-trip，约数秒）…</Typography.Text>}

        {created && report && (
          <Space direction="vertical" size={8} style={{ width: '100%' }} data-testid="upgrade-oo-report">
            <Space size={8} wrap>
              <span style={{ fontSize: 13, fontWeight: 600 }}>{created.name}</span>
              <Tag color={report.lossy ? 'gold' : 'green'} style={{ margin: 0 }}>{report.lossy ? '有损导入' : '无损导入'}</Tag>
              {rec?.capture_rate != null && <Tag style={{ margin: 0 }}>捕获率 {(rec.capture_rate * 100).toFixed(1)}%</Tag>}
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>新资产 id：{created.id}</Typography.Text>
            </Space>
            <Descriptions size="small" column={2} bordered>
              <Descriptions.Item label="导入形态">{report.format}</Descriptions.Item>
              <Descriptions.Item label="三元组恒等式">
                in {rec?.triples_in ?? '-'} = 词表 {rec?.vocabulary ?? '-'} + 投影 {rec?.projected ?? '-'} + 余量 {rec?.remainder ?? '-'}；未对账 <b>{rec?.unaccounted ?? '-'}</b>
              </Descriptions.Item>
            </Descriptions>
            {(report.warnings?.length ?? 0) > 0 && (
              <Alert type="warning" showIcon title={`warnings（${report.warnings.length}）`} description={<div style={{ maxHeight: 120, overflowY: 'auto', fontSize: 12 }}>{report.warnings.map((w, i) => <div key={i}>· {w}</div>)}</div>} />
            )}
            {(report.unprojected?.length ?? 0) > 0 && (
              <Alert type="warning" showIcon title={`未投影三元组（${report.unprojected!.length}）`} description={<div style={{ maxHeight: 120, overflowY: 'auto', fontSize: 12 }}>{report.unprojected!.map((w, i) => <div key={i}>· {w}</div>)}</div>} />
            )}
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              复核无误请点「保留 oo 轨资产」；有损项不可接受可「放弃并删除」（原 spec 本体不受影响）。
            </Typography.Text>
          </Space>
        )}
      </Modal>
    </>
  )
}
