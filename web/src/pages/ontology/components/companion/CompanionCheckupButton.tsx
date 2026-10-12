import { useState } from 'react'
import { Button, Modal, Spin, Table, Tag, Typography } from 'antd'
import { SafetyCertificateOutlined } from '@ant-design/icons'
import { companionApi } from '../../../../api/companion'
import type { CompanionDiagnosis } from '../../../../api/companion'
import { useUI } from '../../../../store/ui'

// ---------------------------------------------------------------------------
// REQ-299/M103：「检查清理」入口（沉淀总览·与「挖掘关系」并排的本体级优化操作）——
// 只读诊断（重复平行边/自指边/孤立实体/重复实体/缺失定义/失效边）→ 报告 Modal 展示
// → 勾选执行有限自动清理（重复平行边合并印证求和、自指边删除；孤立/重复实体只报告——
// 孤立可能有意沉淀、重复实体走人工合并工具）。诊断与清理均落 onto_decision+快照刷新。
// ---------------------------------------------------------------------------

export default function CompanionCheckupButton({ ontologyId, onDone }: { ontologyId: string; onDone?: () => void }) {
  const { showToast } = useUI()
  const [checking, setChecking] = useState(false)
  const [report, setReport] = useState<CompanionDiagnosis | null>(null)
  const [cleaning, setCleaning] = useState(false)
  const [result, setResult] = useState<Record<string, number> | null>(null)

  const runCheck = async () => {
    setChecking(true)
    try {
      const d = await companionApi.diagnose(ontologyId)
      setReport(d)
    } catch (e: any) {
      showToast(e?.message ?? '诊断失败', 'err')
    } finally {
      setChecking(false)
    }
  }

  const runCleanup = async (actions: string[]) => {
    setCleaning(true)
    try {
      const r = await companionApi.cleanup(ontologyId, actions)
      setResult(r)
      onDone?.()
    } catch (e: any) {
      showToast(e?.message ?? '清理失败', 'err')
    } finally {
      setCleaning(false)
    }
  }

  const dupCount = report?.duplicate_edges?.length ?? 0
  const loopCount = report?.self_loops?.length ?? 0

  return (
    <>
      <Button size="small" icon={<SafetyCertificateOutlined />} loading={checking} onClick={runCheck}>
        检查清理
      </Button>
      <Modal
        title="伴生图检查报告"
        open={!!report}
        onCancel={() => {
          setReport(null)
          setResult(null)
        }}
        footer={null}
        width={640}
      >
        {!report ? (
          <Spin />
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              实体 {report.entities} · 活跃关系 {report.active_edges} · 失效关系（历史矛盾） {report.invalid_edges} · 诊断时间 {report.checked_at}
            </Typography.Text>
            <Table
              size="small"
              pagination={false}
              dataSource={[
                { key: 'dup', item: '重复平行边（同事实多边）', count: dupCount, detail: '可自动清理：保留印证最高一条并求和印证数' },
                { key: 'loop', item: '自指边（主体=客体）', count: loopCount, detail: '可自动清理：整边删除' },
                { key: 'orphan', item: '孤立实体（无任何关系边）', count: (report.orphan_nodes ?? []).length, detail: '只报告不清理——可能是有意沉淀；可用人工合并或删除处理' },
                { key: 'dupnode', item: '疑似重复实体（大小写/写法差异）', count: (report.duplicate_nodes ?? []).length, detail: '只报告不清理——请用内容清单「合并到…」逐组合并' },
                { key: 'nodef', item: '缺失定义的实体', count: report.missing_def_count, detail: '只报告——可在内容清单「编辑」补写定义' },
              ]}
              columns={[
                { title: '检查项', dataIndex: 'item' },
                { title: '数量', dataIndex: 'count', width: 60, render: (v: number) => <Tag color={v > 0 ? 'orange' : 'green'} style={{ margin: 0 }}>{v}</Tag> },
                { title: '说明', dataIndex: 'detail', render: (v: string) => <Typography.Text type="secondary" style={{ fontSize: 11 }}>{v}</Typography.Text> },
              ]}
            />
            {(dupCount > 0 || loopCount > 0) && (
              <div>
                <Typography.Text strong style={{ fontSize: 12 }}>自动清理（勾选执行）：</Typography.Text>
                <div style={{ marginTop: 6, display: 'flex', flexDirection: 'column', gap: 4 }}>
                  {dupCount > 0 && (
                    <Button size="small" loading={cleaning} onClick={() => runCleanup(['duplicateEdges'])}>
                      清理 {dupCount} 组重复平行边（印证数求和合并）
                    </Button>
                  )}
                  {loopCount > 0 && (
                    <Button size="small" danger loading={cleaning} onClick={() => runCleanup(['selfLoops'])}>
                      删除 {loopCount} 条自指边
                    </Button>
                  )}
                </div>
              </div>
            )}
            {result && (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                清理结果：{Object.entries(result).map(([k, v]) => `${k === 'duplicate_edges_removed' ? '重复边组合并' : '自指边删除'} ${v}`).join('；') || '无操作'}
              </Typography.Text>
            )}
          </div>
        )}
      </Modal>
    </>
  )
}
