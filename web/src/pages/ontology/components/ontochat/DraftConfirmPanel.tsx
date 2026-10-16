import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Card, Checkbox, Collapse, Input, Select, Space, Tag, Typography } from 'antd'
import { ThunderboltOutlined } from '@ant-design/icons'
import ReactDiffViewer from 'react-diff-viewer-continued'
import QualityReportCard, { qualityFindingsText } from '../QualityReportCard'
import SpecGraph from '../SpecGraph'
import { api } from '../../../../api/client'
import type { OntoChatDraftVersion, QualityReport, Spec } from '../../../../api/types'

// ---------------------------------------------------------------------------
// REQ-338（80 号建议项 X=U2+A3）：draft 阶段视图——
//   ① 入库前增量确认：概念/关系/数据属性分组勾选（复用 cqDraft/clusterDraft checkbox 交互
//      模式），裁剪后的子集作为入库形态（spec 覆盖参数，后端以 source=confirm 落版本链留痕）；
//   ② draft 双栏：左列质量报告卡（REQ-312）+ 右列差异预览（SpecGraph 图预览 + 草稿版本链
//      快照回看与文本对照 react-diff-viewer-continued，沿 VersionDiff TextDiff 同库）；
//   ③ 版本链取数 GET /api/ontochat/sessions/{id}/drafts（draft/refine 全量快照，材料引用随行）。
// ---------------------------------------------------------------------------

/** spec 快照文本归一（稳定序列化后对照才有意义；沿 VersionDiff prettySpec 口径）。 */
function formatSpec(spec: unknown): string {
  try {
    return JSON.stringify(spec, null, 2)
  } catch {
    return String(spec)
  }
}

const DIFF_STYLES = {
  contentText: { fontSize: 12, lineHeight: '18px' },
} as const

export interface DraftPicks {
  concepts: boolean[]
  relations: boolean[]
  dataprops: boolean[]
}

function allTrue(n: number): boolean[] {
  return Array.from({ length: n }, () => true)
}

export default function DraftConfirmPanel({
  sessionId,
  draft,
  draftWarning,
  draftQuality,
  saveName,
  turning,
  onSaveName,
  onSave,
  onOptimize,
}: {
  sessionId: string
  draft: Spec
  draftWarning: string
  draftQuality: QualityReport | null
  saveName: string
  turning: boolean
  onSaveName: (v: string) => void
  onSave: (specOverride: Spec) => void
  onOptimize: (findingsText: string) => void
}) {
  // 增量确认勾选态（默认全选=整稿；REQ-338①）
  const [picks, setPicks] = useState<DraftPicks>(() => ({
    concepts: allTrue(draft.concepts?.length ?? 0),
    relations: allTrue(draft.relations?.length ?? 0),
    dataprops: allTrue(draft.data_properties?.length ?? 0),
  }))
  // 草稿版本链（REQ-338③）
  const [versions, setVersions] = useState<OntoChatDraftVersion[] | null>(null)
  const [viewV, setViewV] = useState<number | null>(null) // null=当前草稿
  const [diffOpen, setDiffOpen] = useState(false)

  // 草稿对象变化（新一轮生成/refine）→ 重置勾选为全选（新稿重新过审）
  useEffect(() => {
    setPicks({
      concepts: allTrue(draft.concepts?.length ?? 0),
      relations: allTrue(draft.relations?.length ?? 0),
      dataprops: allTrue(draft.data_properties?.length ?? 0),
    })
    setViewV(null)
  }, [draft])

  useEffect(() => {
    let alive = true
    api
      .ontoChatDrafts(sessionId)
      .then((vs) => alive && setVersions(vs ?? []))
      .catch(() => alive && setVersions([]))
    return () => {
      alive = false
    }
  }, [sessionId, draft])

  /** 勾选裁剪后的生效草稿（实例不裁剪——实例引用概念，裁剪致失引用由保存时后端校验拦截并如实提示）。 */
  const effective = useMemo<Spec>(() => {
    const concepts = (draft.concepts ?? []).filter((_, i) => picks.concepts[i] !== false)
    const relations = (draft.relations ?? []).filter((_, i) => picks.relations[i] !== false)
    const dataprops = (draft.data_properties ?? []).filter((_, i) => picks.dataprops[i] !== false)
    return { ...draft, concepts, relations, data_properties: dataprops }
  }, [draft, picks])

  const checkedCount =
    (draft.concepts?.length ?? 0) - picks.concepts.filter((v) => v === false).length +
    (draft.relations?.length ?? 0) - picks.relations.filter((v) => v === false).length +
    (draft.data_properties?.length ?? 0) - picks.dataprops.filter((v) => v === false).length
  const trimmed = checkedCount < (draft.concepts?.length ?? 0) + (draft.relations?.length ?? 0) + (draft.data_properties?.length ?? 0)

  const viewSpec: Spec = useMemo(() => {
    if (viewV == null) return effective
    const v = (versions ?? []).find((x) => x.version === viewV)
    return (v?.spec_json as Spec) ?? effective
  }, [viewV, versions, effective])

  const toggleAll = (key: keyof DraftPicks, checked: boolean) => {
    setPicks((p) => ({ ...p, [key]: p[key].map(() => checked) }))
  }

  const groupPane = (key: keyof DraftPicks, title: string, rows: { name: string; desc?: string }[]) =>
    rows.length === 0
      ? null
      : {
          key,
          label: `${title}（${rows.length - picks[key].filter((v) => v === false).length}/${rows.length} 勾选）`,
          children: (
            <div style={{ maxHeight: 240, overflowY: 'auto' }}>
              <div style={{ marginBottom: 6 }}>
                <Button size="small" type="link" onClick={() => toggleAll(key, true)}>全选</Button>
                <Button size="small" type="link" onClick={() => toggleAll(key, false)}>清空</Button>
              </div>
              {rows.map((r, i) => (
                <div key={i} style={{ display: 'flex', gap: 6, alignItems: 'flex-start', marginBottom: 4 }}>
                  <Checkbox
                    checked={picks[key][i] !== false}
                    onChange={(e) =>
                      setPicks((p) => ({ ...p, [key]: p[key].map((v, j) => (j === i ? e.target.checked : v)) }))
                    }
                    style={{ marginTop: 2 }}
                  />
                  <div style={{ minWidth: 0 }}>
                    <Typography.Text style={{ fontSize: 13, fontWeight: 500 }}>{r.name}</Typography.Text>
                    {r.desc && (
                      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
                        {r.desc.length > 80 ? r.desc.slice(0, 80) + '…' : r.desc}
                      </Typography.Paragraph>
                    )}
                  </div>
                </div>
              ))}
            </div>
          ),
        }

  return (
    <Card size="small" style={{ marginTop: 10 }} title="草稿确认（增量勾选后入库）" data-testid="draft-confirm-panel">
      {/* 双栏：左=质量报告卡；右=差异预览（SpecGraph+版本链对照）。窄屏 flexWrap 降级单列。 */}
      <div style={{ display: 'flex', gap: 14, alignItems: 'flex-start', flexWrap: 'wrap' }}>
        <div style={{ flex: '1 1 340px', minWidth: 300 }}>
          {draftWarning && <Alert type="warning" showIcon style={{ marginBottom: 8 }} title={draftWarning} />}
          {draftQuality ? (
            <QualityReportCard
              report={draftQuality}
              optimizing={turning}
              onOptimize={() => {
                const ftext = qualityFindingsText(draftQuality)
                if (!ftext) return
                onOptimize(ftext)
              }}
            />
          ) : (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              质量报告不可用（构建平面不可达时仅本地结构校验兜底）
            </Typography.Text>
          )}
        </div>
        <div style={{ flex: '1 1 380px', minWidth: 320 }} data-testid="draft-preview-pane">
          <Space size={6} wrap style={{ marginBottom: 6 }}>
            <Tag color="blue" style={{ margin: 0 }}>概念 {effective.concepts?.length ?? 0}</Tag>
            <Tag color="geekblue" style={{ margin: 0 }}>关系 {effective.relations?.length ?? 0}</Tag>
            {effective.data_properties?.length ? <Tag color="purple" style={{ margin: 0 }}>数据属性 {effective.data_properties.length}</Tag> : null}
            <Tag color="cyan" style={{ margin: 0 }}>实例 {effective.instances?.length ?? 0}</Tag>
            {versions && versions.length > 0 && (
              <Select
                size="small"
                style={{ minWidth: 190 }}
                value={viewV ?? -1}
                onChange={(v) => setViewV(v === -1 ? null : v)}
                options={[
                  { value: -1, label: '当前草稿（裁剪后）' },
                  ...(versions ?? []).map((x) => ({
                    value: x.version,
                    label: `v${x.version} · ${x.source === 'refine' ? '修正稿' : x.source === 'confirm' ? '确认裁剪' : '首稿'} · r${x.round}${x.materials?.length ? ` · 材料×${x.materials.length}` : ''}`,
                  })),
                ]}
                data-testid="draft-version-select"
              />
            )}
            {viewV != null && (
              <Button size="small" type="link" onClick={() => setDiffOpen((v) => !v)}>
                {diffOpen ? '收起对照' : '与当前草稿对照'}
              </Button>
            )}
          </Space>
          {viewV != null && (versions ?? []).find((x) => x.version === viewV)?.materials?.length ? (
            <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginBottom: 4 }}>
              该快照引用文件材料：{(versions ?? []).find((x) => x.version === viewV)!.materials!.join('、')}
            </Typography.Paragraph>
          ) : null}
          {diffOpen && viewV != null ? (
            <ReactDiffViewer
              oldValue={formatSpec((versions ?? []).find((x) => x.version === viewV)?.spec_json ?? {})}
              newValue={formatSpec(effective)}
              splitView
              leftTitle="历史快照"
              rightTitle="当前草稿（裁剪后）"
              styles={DIFF_STYLES}
              data-testid="draft-version-diff"
            />
          ) : (
            <SpecGraph spec={viewSpec} />
          )}
        </div>
      </div>

      {/* 入库前增量确认（REQ-338①：分组勾选，复用 cqDraft/clusterDraft checkbox 交互模式） */}
      <Collapse
        size="small"
        style={{ marginTop: 10 }}
        items={[
          groupPane('concepts', '概念', (draft.concepts ?? []).map((c) => ({ name: c.name, desc: c.definition }))),
          groupPane('relations', '关系', (draft.relations ?? []).map((r) => ({ name: `${r.name}（${r.from} → ${r.to}）`, desc: r.definition }))),
          groupPane('dataprops', '数据属性', (draft.data_properties ?? []).map((d) => ({ name: d.name, desc: d.range ? `${d.domain ?? ''} · ${d.range}` : d.domain }))),
        ].filter(Boolean) as never}
        data-testid="draft-increment-groups"
      />
      {trimmed && (
        <Typography.Text type="warning" style={{ fontSize: 11, display: 'block', marginTop: 6 }}>
          已裁剪 {(draft.concepts?.length ?? 0) + (draft.relations?.length ?? 0) + (draft.data_properties?.length ?? 0) - checkedCount} 项——裁剪概念后其关联实例/关系可能失引用，保存时后端校验会拦截并如实提示。原生成稿仍保留在版本链可回溯。
        </Typography.Text>
      )}

      <Space style={{ marginTop: 10 }} size={8}>
        <Input value={saveName} onChange={(e) => onSaveName(e.target.value)} placeholder="本体名称" style={{ width: 240 }} />
        <Button
          type="primary"
          icon={<ThunderboltOutlined />}
          loading={turning}
          onClick={() => onSave(effective)}
          data-testid="draft-save-btn"
        >
          创建并保存{trimmed ? `（裁剪后 ${checkedCount} 项）` : ''}
        </Button>
      </Space>
    </Card>
  )
}
