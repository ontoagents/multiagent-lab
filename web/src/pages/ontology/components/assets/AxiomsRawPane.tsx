import { useEffect, useState } from 'react'
import { Alert, Typography } from 'antd'
import { api } from '../../../../api/client'

// ---------------------------------------------------------------------------
// 透传公理（REQ-318/M120，77 号 §6 设计）：导入时无法投影到 spec_json 的 OWL 公理
// （类表达式/属性公理等）原样存档（artifact axioms-raw），导出 TTL 时拼接还原——
// 「三要素编辑无损 + 高级公理透传」。只读呈现；编辑走三要素，高级公理修改请外部工具
// 改后重导入（诚实口径）。引用对象被改名/删除时导出原样保留可能悬空（一期边界）。
// ---------------------------------------------------------------------------

export default function AxiomsRawPane({ ontologyId }: { ontologyId: string }) {
  const [fragments, setFragments] = useState<string[] | null>(null)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    api
      .getArtifactContent(ontologyId, 'axioms-raw')
      .then((raw) => {
        if (!alive) return
        try {
          const envelope = JSON.parse(raw) as { fragments?: string[] }
          setFragments(envelope.fragments ?? [])
        } catch {
          setFragments([])
        }
      })
      .catch((e: any) => {
        if (alive) setErr(e.message)
      })
    return () => {
      alive = false
    }
  }, [ontologyId])

  if (err) {
    return <Alert type="warning" showIcon title="透传公理读取失败" description={err} />
  }
  if (fragments === null) {
    return <Typography.Text type="secondary" style={{ fontSize: 12 }}>加载中…</Typography.Text>
  }
  if (fragments.length === 0) {
    return (
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        本体导入时无未投影公理（全部构造已映射进 spec_json 建模面），或本体系平台内生成（无透传存档）。
      </Typography.Text>
    )
  }
  return (
    <>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 8 }}
        title={`透传公理（${fragments.length} 条，只读）`}
        description="导入时无法投影到 spec_json 建模面的 OWL 公理原样存档；导出 TTL 时自动拼接还原。三要素编辑不受影响；若在编辑中改名/删除公理引用的对象，导出时相关公理将原样保留（可能悬空）。高级公理修改请在外部工具完成后重新导入。"
      />
      <pre
        style={{
          maxHeight: 420, overflow: 'auto', fontSize: 12, margin: 0, padding: 10,
          background: 'var(--c-bg-soft)', borderRadius: 6, fontFamily: 'var(--mono)',
        }}
        data-testid="axioms-raw-list"
      >
        {fragments.join('\n')}
      </pre>
    </>
  )
}
