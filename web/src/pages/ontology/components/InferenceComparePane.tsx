import { useEffect, useMemo, useRef, useState } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import { EditorView } from '@codemirror/view'
import { sparql } from 'codemirror-lang-sparql'
import { Alert, Button, Select, Space, Table, Tag, Typography } from 'antd'
import { CaretRightOutlined, SwapOutlined } from '@ant-design/icons'
import { api } from '../../../api/client'
import type { RuntimeProfile } from '../../../api/types'

// ---------------------------------------------------------------------------
// 推理对照双栏工作台（REQ-94② 收编 / REQ-379②·M175，2026-10-29）：
// 同一 SPARQL 对「当前方案 × 对照方案」并行执行并 diff 结果——推理新增行高亮 +
// 解释文案模板（新增实例来自 subClassOf/subPropertyOf 等规则物化，OWL_FB）。
// 对照口径=两个 running 方案（建议同本体集合、一开一关推理）；fuseki 推理方案
// 可一键克隆「无推理基线」替代此前「建两套方案人工对照」。
// 诚实边界（REQ-500）：JDK 25 环境 fuseki 推理侧实例级规则应用非确定性（rdf:type
// 绑定模式可能间歇空结果）——对照结论异常时先以 open 模式/聚合计数复核。
// ---------------------------------------------------------------------------

const DEFAULT_QUERY = `PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
SELECT ?s ?o WHERE { ?s rdf:type ?o } LIMIT 50`

type SelectResult = { kind: 'select'; vars: string[]; rows: Record<string, string>[]; ms: number }
type AskResult = { kind: 'ask'; value: boolean; ms: number }
type GraphResult = { kind: 'graph'; text: string; ms: number }
type CompareResult = SelectResult | AskResult | GraphResult

function parseResult(raw: string, json: any, t0: number): CompareResult {
  const ms = Math.round(performance.now() - t0)
  if (typeof json?.boolean === 'boolean') return { kind: 'ask', value: json.boolean, ms }
  if (json?.results?.bindings) {
    const vars: string[] = json.head?.vars ?? Object.keys(json.results.bindings[0] ?? {})
    const rows = json.results.bindings.map((b: Record<string, any>, i: number) => {
      const row: Record<string, string> = { __key: String(i) }
      for (const v of vars) row[v] = b[v]?.value ?? ''
      return row
    })
    return { kind: 'select', vars, rows, ms }
  }
  return { kind: 'graph', text: raw, ms }
}

/** select 结果行键（变量名序无关的值指纹） */
function rowKey(row: Record<string, string>, vars: string[]): string {
  return JSON.stringify([...vars].sort().map((v) => row[v] ?? ''))
}

export default function InferenceComparePane({
  profile,
  profiles,
  onChanged,
}: {
  profile: RuntimeProfile
  profiles: RuntimeProfile[]
  onChanged?: () => void
}) {
  const others = useMemo(() => profiles.filter((p) => p.id !== profile.id && p.status === 'running'), [profiles, profile.id])
  const [counterpartId, setCounterpartId] = useState<string | null>(null)
  const [query, setQuery] = useState<string>(() => {
    try {
      return localStorage.getItem(`onto-compare-${profile.id}`) || DEFAULT_QUERY
    } catch {
      return DEFAULT_QUERY
    }
  })
  const [running, setRunning] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [resA, setResA] = useState<CompareResult | null>(null)
  const [resB, setResB] = useState<CompareResult | null>(null)
  const [cloning, setCloning] = useState(false)
  const queryRef = useRef(query)
  queryRef.current = query

  useEffect(() => {
    // 默认对照对象：同本体集合优先（推理对照的语义前提），其中同引擎（推理开关对）再优先
    if (!counterpartId && others.length > 0) {
      const myOnto = (profile.ontology_ids ?? []).join()
      const sameOnto = others.filter((p) => (p.ontology_ids ?? []).join() === myOnto)
      const pick = sameOnto.find((p) => p.engine === profile.engine) ?? sameOnto[0] ?? others[0]
      setCounterpartId(pick.id)
    }
  }, [others, profile, counterpartId])

  const counterpart = others.find((p) => p.id === counterpartId) ?? null

  const run = async () => {
    const q = queryRef.current
    setRunning(true)
    setErr(null)
    setResA(null)
    setResB(null)
    const t0 = performance.now()
    try {
      const [ra, rb] = await Promise.all([
        api.runSparql(profile.id, q),
        api.runSparql(counterpartId!, q),
      ])
      setResA(parseResult(ra.raw, ra.json, t0))
      setResB(parseResult(rb.raw, rb.json, t0))
    } catch (e: any) {
      setErr(e?.message ?? '对照执行失败')
    } finally {
      setRunning(false)
    }
  }

  const cloneBaseline = async () => {
    setCloning(true)
    try {
      const cfg = profile.engine === 'fuseki' ? JSON.stringify({ reasoning: false }) : undefined
      await api.createRuntimeProfile({
        name: `${profile.name}·无推理基线`,
        engine: profile.engine,
        ontology_ids: profile.ontology_ids ?? [],
        ...(cfg ? { config: cfg } : {}),
      })
      onChanged?.()
    } catch (e: any) {
      setErr(e?.message ?? '克隆基线失败')
    } finally {
      setCloning(false)
    }
  }

  // select 双侧 diff（单侧或形态不一致时退化为并排展示）
  const diff = useMemo(() => {
    if (resA?.kind !== 'select' || resB?.kind !== 'select') return null
    const kaSet = new Set(resA.rows.map((r) => rowKey(r, resA.vars)))
    const kbSet = new Set(resB.rows.map((r) => rowKey(r, resB.vars)))
    const onlyA = resA.rows.filter((r) => !kbSet.has(rowKey(r, resA.vars)))
    const onlyB = resB.rows.filter((r) => !kaSet.has(rowKey(r, resB.vars)))
    const common = resA.rows.length - onlyA.length
    return { onlyA, onlyB, common, kaSet, kbSet }
  }, [resA, resB])

  const selectColumns = (vars: string[]) =>
    vars.map((v) => ({ title: `?${v}`, dataIndex: v, key: v, ellipsis: true }))

  const sideTitle = (label: string, r: CompareResult | null) => (
    <Space size={6}>
      <span>{label}</span>
      {r?.kind === 'select' && <Tag style={{ margin: 0 }}>{r.rows.length} 行</Tag>}
      {r && <Tag color="blue" style={{ margin: 0 }}>{r.ms}ms</Tag>}
    </Space>
  )

  return (
    <div className="onto-inference-compare" data-testid="inference-compare">
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        title="推理对照（REQ-94②）：同一查询对两方案并行执行并 diff"
        description="左侧=当前方案；右侧=对照方案。推理新增行来自 subClassOf/subPropertyOf 等规则物化（Fuseki OWL_FB）；「仅对照侧」行=基线可见而本方案不可见（罕见，多为引擎对病态数据的处理差异）。建议同本体集合一开一关推理；fuseki 方案可一键克隆无推理基线。"
      />
      <Space style={{ marginBottom: 10 }} wrap>
        <span style={{ fontSize: 12 }}>对照方案</span>
        <Select
          data-testid="compare-counterpart"
          style={{ minWidth: 260 }}
          placeholder={others.length === 0 ? '暂无其他 running 方案' : '选择对照方案…'}
          value={counterpartId}
          onChange={setCounterpartId}
          options={others.map((p) => ({ value: p.id, label: `${p.name}（${p.engine ?? '—'}${p.engine === 'fuseki' && (p.config as any)?.reasoning ? '·推理开' : ''}）` }))}
          notFoundContent={
            <Button size="small" type="link" icon={<SwapOutlined />} loading={cloning} onClick={cloneBaseline}>
              从当前方案克隆「无推理基线」
            </Button>
          }
        />
        {profile.engine === 'fuseki' && (
          <Button size="small" icon={<SwapOutlined />} loading={cloning} onClick={cloneBaseline} data-testid="compare-clone-baseline">
            克隆本方案为无推理基线
          </Button>
        )}
      </Space>
      {others.length === 0 && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 10 }}
          title="需要第二个 running 方案作为对照"
          description="同一查询要并行打两个方案才能 diff。可用上方「克隆本方案为无推理基线」快速创建同本体集合、推理关闭的对照方案，启动后回到本页执行对照。"
        />
      )}
      <CodeMirror
        value={query}
        height="150px"
        basicSetup={{ lineNumbers: true, foldGutter: false, highlightActiveLine: true }}
        extensions={[sparql(), EditorView.lineWrapping]}
        onChange={(v) => {
          setQuery(v)
          try {
            localStorage.setItem(`onto-compare-${profile.id}`, v)
          } catch {
            /* 存储不可用则不持久化 */
          }
        }}
      />
      <div style={{ marginTop: 8, marginBottom: 4 }}>
        <Button
          type="primary"
          icon={<CaretRightOutlined />}
          loading={running}
          disabled={!counterpartId}
          onClick={run}
          data-testid="compare-run"
        >
          双栏并行执行
        </Button>
      </div>
      {err && <Alert type="error" showIcon closable style={{ marginTop: 8 }} title="对照执行失败" description={err} onClose={() => setErr(null)} />}

      {diff && (
        <div style={{ margin: '10px 0' }} data-testid="compare-diff-summary">
          <Alert
            type={diff.onlyA.length > 0 ? 'success' : 'info'}
            showIcon
            title={`对照结论：共同 ${diff.common} 行 · 推理新增 ${diff.onlyA.length} 行 · 仅对照侧 ${diff.onlyB.length} 行`}
            description={
              diff.onlyA.length > 0
                ? '新增行（左侧绿色高亮）来自推理物化——subClassOf/subPropertyOf/domain/range 等规则在推理侧可查而基线不可见；这正是推理对照要暴露的语义增量。'
                : '两侧结果一致——该查询未触及推理增量（可尝试类层级/实例分类类查询，如 ?x rdf:type ?c）。'
            }
          />
        </div>
      )}

      <div style={{ display: 'flex', gap: 12, alignItems: 'flex-start' }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="onto-sec"><span className="onto-sec-title">{sideTitle(`本方案 · ${profile.name}`, resA)}</span></div>
          {resA?.kind === 'select' && (
            <Table
              size="small"
              rowKey="__key"
              dataSource={resA.rows}
              pagination={{ pageSize: 10, size: 'small', showSizeChanger: false }}
              columns={selectColumns(resA.vars)}
              rowClassName={diff ? (r) => (!diff.kbSet?.has(rowKey(r as Record<string, string>, resA.vars)) ? 'onto-diff-row-only-a' : '') : undefined}
              locale={{ emptyText: '空结果集' }}
            />
          )}
          {resA?.kind === 'ask' && <Alert type={resA.value ? 'success' : 'warning'} showIcon title={`ASK：${resA.value}`} />}
          {resA?.kind === 'graph' && <pre className="onto-sparql-graph">{resA.text}</pre>}
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="onto-sec"><span className="onto-sec-title">{sideTitle(`对照侧 · ${counterpart?.name ?? '—'}`, resB)}</span></div>
          {resB?.kind === 'select' && (
            <Table
              size="small"
              rowKey="__key"
              dataSource={resB.rows}
              pagination={{ pageSize: 10, size: 'small', showSizeChanger: false }}
              columns={selectColumns(resB.vars)}
              rowClassName={diff ? (r) => (!diff.kaSet?.has(rowKey(r as Record<string, string>, resB.vars)) ? 'onto-diff-row-only-b' : '') : undefined}
              locale={{ emptyText: '空结果集' }}
            />
          )}
          {resB?.kind === 'ask' && <Alert type={resB.value ? 'success' : 'warning'} showIcon title={`ASK：${resB.value}`} />}
          {resB?.kind === 'graph' && <pre className="onto-sparql-graph">{resB.text}</pre>}
        </div>
      </div>

      {!resA && !resB && !err && (
        <Typography.Text type="secondary" style={{ display: 'block', marginTop: 10, fontSize: 12 }}>
          诚实边界（REQ-500）：JDK 25 环境下 fuseki 推理侧实例级规则应用非确定性（rdf:type 绑定模式可能间歇空结果）——对照结论异常时以 open 模式（?s ?p ?o）或聚合计数复核。
        </Typography.Text>
      )}
    </div>
  )
}
