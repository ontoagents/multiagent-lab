import { useEffect, useRef, useState } from 'react'
import { Button } from 'antd'
import { ApartmentOutlined } from '@ant-design/icons'
import { companionApi } from '../../../../api/companion'
import { useUI } from '../../../../store/ui'

// ---------------------------------------------------------------------------
// REQ-297/M101：「挖掘关系」入口（沉淀总览·图级操作归位 §十九）——异步 job 触发+内联进度
// 轮询（治 REQ-286 同步接口超时/黑盒）。点击 → 202 job_id → 1.5s 轮询 mine-jobs/{id} →
// 阶段进度（读取实体/挖掘批次 N/M/完成 K 条）→ 完成自动刷新成长图与候选。
// 404（进程重启丢内存态）→ 提示重新触发。本体级单飞（后端 409）。
// ---------------------------------------------------------------------------

export default function MineRelationsButton({ ontologyId, onDone }: { ontologyId: string; onDone?: () => void }) {
  const { showToast } = useUI()
  const [jobId, setJobId] = useState<string | null>(null)
  const [stage, setStage] = useState('')
  const [starting, setStarting] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current)
  }, [])

  const poll = (id: string) => {
    timer.current = setTimeout(async () => {
      try {
        const j = await companionApi.getMineJob(id)
        setStage(j.stage || j.phase)
        if (j.phase === 'running') {
          poll(id)
          return
        }
        if (j.phase === 'done') {
          showToast(`关系挖掘完成：${j.candidates} 条候选待确认`)
          onDone?.()
        } else {
          showToast(j.error || '关系挖掘失败', 'err')
        }
      } catch (e: any) {
        showToast(e?.message ?? '挖掘任务查询失败', 'err')
      }
      setJobId(null)
      setStage('')
    }, 1500)
  }

  const start = async () => {
    setStarting(true)
    try {
      const r = await companionApi.mineRelations(ontologyId)
      setJobId(r.job_id)
      setStage('排队中…')
      poll(r.job_id)
    } catch (e: any) {
      showToast(e?.message ?? '触发挖掘失败', 'err')
      setStarting(false)
    } finally {
      setStarting(false)
    }
  }

  const running = jobId !== null
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
      <Button
        size="small"
        icon={<ApartmentOutlined />}
        loading={starting || running}
        disabled={running}
        onClick={start}
      >
        挖掘关系
      </Button>
      {running && stage && (
        <span style={{ fontSize: 12, color: 'var(--c-ink-3)' }}>{stage}</span>
      )}
    </span>
  )
}
