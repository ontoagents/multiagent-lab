import { useState } from 'react'
import { Button, Card, Segmented, Select, Space, Tag, Typography, Upload } from 'antd'
import { UploadOutlined } from '@ant-design/icons'
import { api } from '../../../api/client'
import { useUI } from '../../../store/ui'
import LoadErrorAlert from '../../../components/LoadErrorAlert'
import CqQuickScreen from '../components/CqQuickScreen'
import { invalidateOntos, useOntologies } from '../shared/useOntoList'
import { ConvergeCard } from './PathShell'

// ---------------------------------------------------------------------------
// REQ-600/M411：结构化表格映射升独立路径页（#/onto/build/structured，111 号 §3.3-④）
// ——自 BuildFromKBFlow 双 export 拆出（与 datasource 同为「结构化数据→本体」但来源
// 不同：CSV/JSON 表 vs SQLite 库；hub 分组相邻陈列）。功能原样：规则推导（实例骨架
// /模板层级两模式）→映射报告→创建入库→收敛卡三选（REQ-600 收敛卡全量替换）。
// ---------------------------------------------------------------------------

export default function StructuredFlow() {
  const { showToast } = useUI()
  const [file, setFile] = useState<{ filename: string; content: string } | null>(null)
  const [mode, setMode] = useState<'instance' | 'template'>('instance')
  const [hierarchyCols, setHierarchyCols] = useState<string[]>([])
  const [headerCols, setHeaderCols] = useState<string[]>([])
  const [targetId, setTargetId] = useState<string | undefined>(undefined)
  const [result, setResult] = useState<Awaited<ReturnType<typeof api.buildFromStructured>> | null>(null)
  const [loading, setLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [createdId, setCreatedId] = useState<string | null>(null)

  // REQ-467/M267：本体清单单源 hook（散拉收敛）
  const { ontos } = useOntologies()

  const infer = async () => {
    if (!file) return
    setLoading(true)
    setErr(null)
    setResult(null)
    try {
      const r = await api.buildFromStructured({
        filename: file.filename,
        content: file.content,
        target_ontology_id: targetId,
        mode,
        hierarchy_columns: mode === 'template' && hierarchyCols.length > 0 ? hierarchyCols : undefined,
      })
      setResult(r)
    } catch (e: any) {
      setErr(e?.message ?? '推导失败')
    } finally {
      setLoading(false)
    }
  }

  const createAsOntology = async () => {
    if (!result?.draft_spec) return
    setCreating(true)
    try {
      const draft = result.draft_spec as { name: string; description?: string; concepts: unknown[]; relations: unknown[]; instances: unknown[] }
      const created = await api.createOntology({ name: `${draft.name}-${new Date().toLocaleDateString()}`, description: draft.description ?? '由结构化数据映射推导（M-O14 P2⑤）' })
      await api.saveSpec(created.id, draft as any)
      invalidateOntos() // REQ-467：单源清单失效广播
      showToast(`已创建新本体「${created.name ?? created.id}」（骨架含 ${draft.concepts.length} 概念 / ${draft.instances.length} 实例）`)
      setCreatedId(created.id) // REQ-250/G6：完成态 CTA（替代纯 toast 指路）
    } catch (e: any) {
      showToast(e?.message ?? '创建失败', 'err')
    } finally {
      setCreating(false)
    }
  }

  return (
    <Card size="small">
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 8 }}>
        {mode === 'instance'
          ? '规则推导：首列→实例名（主概念），其余列→属性（类型推断）；可选对目标本体做概念命中标注。关系推导需语义判断——建议在骨架上用「AI 创建」继续加工。'
          : '模板层级（REQ-256）：低基数枚举列→概念层级链（列序=粗→细，共现推断父子），首列高基数时行→实例挂最细层概念。层级为规则推断，语义正确性请审查后入库；含状态/日期等并列维度列时建议显式指定层级列。'}
      </Typography.Paragraph>
      <div style={{ marginBottom: 8 }}>
        <span style={{ fontSize: 12, color: 'var(--c-ink-3)', marginInlineEnd: 8 }}>生成模式</span>
        <Segmented
          size="small"
          value={mode}
          onChange={(v) => {
            setMode(v as 'instance' | 'template')
            setResult(null)
          }}
          options={[
            { value: 'instance', label: '实例骨架' },
            { value: 'template', label: '模板层级（概念批量）' },
          ]}
        />
        {mode === 'template' && headerCols.length > 0 && (
          <Select
            mode="multiple"
            allowClear
            size="small"
            style={{ minWidth: 260, marginInlineStart: 10 }}
            placeholder="层级列（可空=规则推断全部枚举列）"
            value={hierarchyCols}
            onChange={setHierarchyCols}
            options={headerCols.map((h) => ({ value: h, label: h }))}
          />
        )}
      </div>
      <Space wrap style={{ marginBottom: 8 }}>
        <Upload maxCount={1} accept=".csv,.json" showUploadList={false} beforeUpload={() => false} onChange={({ fileList }) => {
          const raw = (fileList[0] as any)?.originFileObj
          if (!raw) return
          const rd = new FileReader()
          rd.onload = () => {
            const content = String(rd.result ?? '')
            setFile({ filename: raw.name, content })
            setResult(null)
            setHierarchyCols([])
            // 粗探表头列（层级列多选候选项）：CSV 首行 / JSON 首对象 keys
            let cols: string[] = []
            try {
              const t = content.trim()
              if (raw.name.toLowerCase().endsWith('.json') || t.startsWith('[') || t.startsWith('{')) {
                const arr = JSON.parse(t)
                const first = Array.isArray(arr) ? arr[0] : Object.values(arr as any).find((v: any) => Array.isArray(v))?.[0] ?? arr
                cols = first && typeof first === 'object' ? Object.keys(first) : []
              } else {
                cols = (t.split('\n')[0] ?? '').split(',').map((x) => x.trim().replace(/^"|"$/g, '')).filter(Boolean)
              }
            } catch {
              cols = []
            }
            setHeaderCols(cols)
          }
          rd.readAsText(raw)
        }}>
          <Button icon={<UploadOutlined />}>选择 CSV/JSON 文件</Button>
        </Upload>
        <Select
          allowClear
          showSearch
          optionFilterProp="label"
          style={{ width: 240 }}
          placeholder="可选：目标本体（概念命中标注）"
          value={targetId}
          onChange={setTargetId}
          options={ontos.map((o) => ({ value: o.id, label: o.name }))}
        />
        <Button type="primary" disabled={!file} loading={loading} onClick={infer}>
          推导映射与骨架
        </Button>
        {result && (
          <Button type="primary" ghost loading={creating} onClick={createAsOntology}>
            创建为新本体
          </Button>
        )}
      </Space>
      {file && (
        <div style={{ marginBottom: 8 }}>
          <Tag color="geekblue">{file.filename}</Tag>
          <Typography.Text type="secondary" style={{ fontSize: 11 }}>{file.content.length} 字符</Typography.Text>
        </div>
      )}
      {err && <LoadErrorAlert title="结构化推导失败" message={err} onRetry={() => setErr(null)} style={{ marginBottom: 8 }} />}
      {result && (
        <>
          <Space size={12} wrap style={{ marginBottom: 6 }}>
            <Tag color="processing">源 {result.source_kind.toUpperCase()}</Tag>
            {result.mode === 'template' && <Tag color="geekblue">模板层级</Tag>}
            <Tag color="purple">主概念 {result.main_concept}</Tag>
            <Tag>{(result.draft_spec as any)?.concepts?.length ?? 0} 概念</Tag>
            <Tag>{result.mapping.length} 列</Tag>
            <Tag>{result.draft_spec ? (result.draft_spec as any).instances.length : 0} 实例（采样≤200）</Tag>
            {result.quality && !result.quality.degraded && (
              <Tag color={result.quality.error_count > 0 ? 'red' : result.quality.overall >= 80 ? 'green' : 'orange'}>
                质量 {Math.round(result.quality.overall)}
              </Tag>
            )}
            {result.quality?.degraded && <Tag>质量快评不可达（本地结构校验兜底）</Tag>}
          </Space>
          <table className="onto-report-table" style={{ width: '100%', fontSize: 12, marginBottom: 6 }}>
            <thead>
              <tr><th>列</th><th>角色</th><th>推断类型</th><th>样例</th><th>目标本体命中</th></tr>
            </thead>
            <tbody>
              {result.mapping.map((m) => (
                <tr key={m.column}>
                  <td>{m.column}</td>
                  <td>
                    {m.role === 'instance-name' ? '实例名' : m.role === 'concept-level' ? <Tag color="geekblue" style={{ margin: 0 }}>层级 {m.level ?? ''}</Tag> : '属性'}
                  </td>
                  <td>{m.infer_type}</td>
                  <td>{m.sample || '—'}</td>
                  <td>{m.matched_concepts?.length ? <Tag color="green" style={{ margin: 0 }}>{m.matched_concepts.join(', ')}</Tag> : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {result.notes.map((n, i) => (
            <Typography.Paragraph key={i} type="secondary" style={{ fontSize: 11, marginBottom: 2 }}>· {n}</Typography.Paragraph>
          ))}
        </>
      )}
      {createdId && (
        <div style={{ marginTop: 10 }}>
          {/* REQ-600：收敛卡统一件（DoneCTA 退役——三选=进入编辑校验/前往资产详情/再建一个） */}
          <ConvergeCard
            ontologyId={createdId}
            detail="结构化骨架本体已创建"
            onRebuild={() => {
              setCreatedId(null)
              setResult(null)
              setFile(null)
            }}
          />
          {/* REQ-500/M308：导入应用后快筛入口前移（默认开） */}
          <CqQuickScreen ontologyId={createdId} autoStart />
        </div>
      )}
    </Card>
  )
}
