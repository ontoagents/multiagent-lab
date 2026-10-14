import { useMemo, useState } from 'react'
import { Alert, Button, Card, Input, Space, Tag, Typography } from 'antd'
import { CloudDownloadOutlined, LinkOutlined, SearchOutlined } from '@ant-design/icons'
import { api } from '../../../api/client'
import type { ImportReport } from '../../../api/types'
import { ONTOLOGY_CATALOG } from './catalogData'
import { useUI } from '../../../store/ui'

// ---------------------------------------------------------------------------
// 权威本体库（REQ-316/M118，76 号分析 P1）：内置精选目录（真机验证过的 W3C/ETSI/DCMI/
// EDM Council 权威本体）+ 来源代理拉取 → 既有导入管线（嗅探/审查报告/防线自动生效）。
// 运行与拓展零新增——入库后走既有「本体运行挂载 / Fork / 导入合并」动线（页面内指引）。
// ---------------------------------------------------------------------------

export default function CatalogPane({ onCreated }: { onCreated?: (id: string) => void }) {
  const { showToast } = useUI()
  const [kw, setKw] = useState('')
  const [busyId, setBusyId] = useState<string | null>(null)
  const [customUrl, setCustomUrl] = useState('')
  const [customBusy, setCustomBusy] = useState(false)
  const [lastReport, setLastReport] = useState<{ name: string; report: ImportReport } | null>(null)

  const list = useMemo(() => {
    const k = kw.trim().toLowerCase()
    if (!k) return ONTOLOGY_CATALOG
    return ONTOLOGY_CATALOG.filter((e) =>
      [e.name, e.domain, e.org, e.description].some((s) => s.toLowerCase().includes(k)),
    )
  }, [kw])

  const doImport = async (entryId: string, url: string) => {
    setBusyId(entryId)
    try {
      const r = await api.catalogFetch(url)
      // 走既有导入管线：格式嗅探 + 审查报告 + REQ-305 防线（与「导入文件/粘贴内容」同链路）
      const imp = await api.importOntologyContent(r.filename, r.content)
      setLastReport({ name: imp.ontology.name, report: imp.report })
      showToast(`已引入「${imp.ontology.name}」（可到「本体资产」运行、Fork 或导入合并拓展）`)
      onCreated?.(imp.ontology.id)
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusyId(null)
    }
  }

  const doCustomImport = async () => {
    if (!customUrl.trim()) {
      showToast('请填写来源 URL', 'err')
      return
    }
    setCustomBusy(true)
    try {
      const r = await api.catalogFetch(customUrl.trim())
      const imp = await api.importOntologyContent(r.filename, r.content)
      setLastReport({ name: imp.ontology.name, report: imp.report })
      showToast(`已引入「${imp.ontology.name}」`)
      setCustomUrl('')
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setCustomBusy(false)
    }
  }

  return (
    <>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 10 }}
        title="权威本体库——查询并引入业界权威本体（REQ-316）"
        description="内置目录均为 W3C / ETSI / DCMI / EDM Council 发布的权威本体（来源经真机可达性验证）；「引入到本地」经平台代理拉取源文件并走既有导入审查管线（有损项如实报告）。双形态口径：original 源文件完整语义无损（运行/查询直挂）；归一化 spec_json 为建模子集——大型 OWL 本体的投影丢弃项会较多（导入报告逐项列明），未投影公理已透传存档（详情页「透传公理」可查）。引入后即可在「本体资产」中直接挂运行方案运行，或 Fork / 导入合并进行拓展。许可以来源条目标注为准。"
      />
      <Space style={{ marginBottom: 10 }} wrap>
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="按名称 / 领域 / 组织搜索…"
          value={kw}
          onChange={(e) => setKw(e.target.value)}
          style={{ width: 320 }}
          data-testid="catalog-search"
        />
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {list.length} / {ONTOLOGY_CATALOG.length} 条（首批均经真机可达性验证）
        </Typography.Text>
      </Space>
      <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
        {list.map((e) => (
          <Card
            key={e.id}
            size="small"
            style={{ flex: '1 1 300px', maxWidth: 460 }}
            title={<span style={{ fontSize: 13 }}>{e.name}</span>}
            extra={
              <Button
                size="small"
                icon={<CloudDownloadOutlined />}
                loading={busyId === e.id}
                onClick={() => doImport(e.id, e.ttlUrl)}
                data-testid={`catalog-import-${e.id}`}
              >
                引入到本地
              </Button>
            }
          >
            <Space size={6} wrap style={{ marginBottom: 6 }}>
              <Tag color="geekblue" style={{ margin: 0 }}>{e.domain}</Tag>
              <Tag style={{ margin: 0 }}>{e.org}</Tag>
              <Tag color="gold" style={{ margin: 0 }}>{e.license}</Tag>
            </Space>
            <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 6 }}>
              {e.description}
            </Typography.Paragraph>
            <Typography.Link href={e.docUrl} target="_blank" rel="noreferrer" style={{ fontSize: 12 }}>
              <LinkOutlined /> 规范文档（来源页面）
            </Typography.Link>
          </Card>
        ))}
      </div>
      {list.length === 0 && (
        <Typography.Text type="secondary">无匹配条目——目录外来源可用下方「自定义来源 URL」引入。</Typography.Text>
      )}
      <Card size="small" style={{ marginTop: 12 }} title="自定义来源 URL（目录外的任意 TTL / RDF-XML 本体）">
        <Space direction="vertical" style={{ width: '100%' }} size={8}>
          <Input
            value={customUrl}
            onChange={(e) => setCustomUrl(e.target.value)}
            placeholder="https://…（公网可达的 .ttl / .rdf / .owl 源文件地址）"
            data-testid="catalog-custom-url"
          />
          <Space>
            <Button type="primary" ghost loading={customBusy} onClick={doCustomImport} data-testid="catalog-custom-import">
              拉取并导入
            </Button>
            <Typography.Text type="secondary" style={{ fontSize: 11 }}>
              经平台代理拉取（仅公网来源，≤8MB）后走既有导入审查管线；内网/环回地址不允许。
            </Typography.Text>
          </Space>
        </Space>
      </Card>
      {lastReport && (
        <Card size="small" style={{ marginTop: 12 }} title={`最近引入：「${lastReport.name}」导入报告`}>
          <ImportReportView report={lastReport.report} />
          <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 6 }}>
            后续动线：到「本体资产」可直接挂运行方案运行（original TTL 零转换），或 Fork 为新本体拓展、经「导入合并」并入既有本体。
          </Typography.Text>
        </Card>
      )}
    </>
  )
}

/** 导入报告轻量渲染（lossy/warning 项如实呈现——REQ-235 口径） */
function ImportReportView({ report }: { report: ImportReport }) {
  return (
    <div style={{ fontSize: 12 }}>
      <Space size={6} wrap>
        <Tag style={{ margin: 0 }}>格式 {report.format}</Tag>
        {report.lossy ? <Tag color="orange" style={{ margin: 0 }}>有损导入</Tag> : <Tag color="green" style={{ margin: 0 }}>无损</Tag>}
      </Space>
      {(report.warnings?.length ?? 0) > 0 && (
        <ul style={{ margin: '6px 0 0', paddingLeft: 18 }}>
          {report.warnings!.slice(0, 8).map((w, i) => (
            <li key={i} style={{ color: 'var(--c-ink-2)' }}>{w}</li>
          ))}
          {report.warnings!.length > 8 && <li>…共 {report.warnings!.length} 条</li>}
        </ul>
      )}
    </div>
  )
}
