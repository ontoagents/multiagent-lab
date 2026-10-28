import { useEffect, useState } from 'react'
import { api } from '../../../api/client'
import type { Ontology } from '../../../api/types'

// ---------------------------------------------------------------------------
// REQ-467/M267：本体清单单源——模块级缓存+订阅广播。此前 7+ 处各自 useEffect 散拉
// （CustomBuildFlow/ProductionGuide/OntoExtendFlow/BuildFromKBFlow/RuntimePage/AssetsPage），
// 创建/删除/重命名后互不知情需手动刷新；收敛后变更方调 invalidateOntos() 一处失效、
// 全部订阅方即时收到新清单。零第三方依赖（SWR/react-query 不引入，沿 D-O5 自研边界）。
// ---------------------------------------------------------------------------
type Snapshot = { ontos: Ontology[]; loading: boolean; err: string | null }

let cache: Ontology[] | null = null
let err: string | null = null
let inflight: Promise<Ontology[]> | null = null
const subs = new Set<(s: Snapshot) => void>()

const snap = (): Snapshot => ({ ontos: cache ?? [], loading: inflight != null, err })
const notify = () => subs.forEach((f) => f(snap()))

async function fetchOntos(force: boolean): Promise<Ontology[]> {
  if (!force && cache && !err) {
    notify()
    return cache
  }
  if (inflight) return inflight
  notify()
  inflight = api
    .listOntologies()
    .then((ls) => {
      cache = ls
      err = null
      return ls
    })
    .catch((e: any) => {
      err = e?.message ?? '加载失败'
      cache = []
      throw e
    })
    .finally(() => {
      inflight = null
      notify()
    })
  return inflight
}

/** 变更后失效并立即重拉（全部订阅方即时同步） */
export function invalidateOntos() {
  cache = null
  fetchOntos(true).catch(() => {})
}

/** 订阅本体清单（缓存优先；多组件共享一次请求） */
export function useOntologies(): Snapshot & { reload: () => Promise<void> } {
  const [s, setS] = useState<Snapshot>(snap)
  useEffect(() => {
    const f = (n: Snapshot) => setS(n)
    subs.add(f)
    fetchOntos(false).catch(() => {})
    setS(snap())
    return () => {
      subs.delete(f)
    }
  }, [])
  return { ...s, reload: () => fetchOntos(true).then(() => undefined).catch(() => {}) }
}
