import { useEffect, useState } from 'react'
import { api } from '../../../../api/client'
import type { QualityReport } from '../../../../api/client'

// REQ-502/M310：编辑视图常驻诊断条数据源——save=false 快评（不落 artifact，REQ-331 机器产报告
// 人做决策口径）；tick 变化（元素操作保存后/外层刷新）即重跑。失败静默（诊断条缺省不阻断编辑）。
export function useQualityDigest(ontologyId: string, tick: number) {
  const [digest, setDigest] = useState<QualityReport | null>(null)
  useEffect(() => {
    if (!ontologyId) return
    let alive = true
    api
      .qualityRun(ontologyId, false, false, false)
      .then((r) => {
        if (alive) setDigest(r.report)
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [ontologyId, tick])
  return digest
}
