import { useEffect, useRef, useState } from 'react'
import { AutoComplete, Typography } from 'antd'
import { api } from '../../../api/client'

// REQ-504/M312 命名输入内联复用联想（先找再造；100 号 R5）：
// 命名输入接 vocabSearch（LOV）+本地已声明概念——下拉「本地/LOV」带来源徽标；
// 命中 LOV 经 onPickVocab 上抛（调用方预填同义词/记录词表来源=引入并对齐）；
// 词表不可达（502）灰字「离线模式，仅本地建议」（后端 502 降级先例）。纯前端接线。

export interface VocabPick {
  title: string
  uri: string
  prefix: string
  description?: string
}

export default function NameSuggestInput({
  value,
  onChange,
  localNames = [],
  exclude,
  placeholder,
  vocab = true,
  onPickVocab,
  testid,
  disabled,
}: {
  value?: string
  onChange?: (v: string) => void
  /** 本地已声明概念名（来源徽标「本地」） */
  localNames?: string[]
  /** 排除项（如重命名时的自身名） */
  exclude?: string
  placeholder?: string
  /** 是否启用 LOV 词表联想（false=纯本地） */
  vocab?: boolean
  /** 命中 LOV 词表项上抛（调用方预填同义词/记录来源） */
  onPickVocab?: (pick: VocabPick) => void
  testid?: string
  disabled?: boolean
}) {
  const [remote, setRemote] = useState<VocabPick[]>([])
  const [offline, setOffline] = useState(false)
  const timer = useRef<number | null>(null)
  const lastQ = useRef('')

  useEffect(() => {
    if (!vocab) return
    const q = (value ?? '').trim()
    if (q.length < 2 || q === lastQ.current) return
    if (timer.current) window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      lastQ.current = q
      api
        .vocabSearch(q)
        .then((d) => {
          const cards = ((d.results ?? d.items ?? []) as VocabPick[]).slice(0, 6)
          setRemote(cards)
          setOffline(false)
        })
        .catch(() => {
          // 词表不可达（502 上游降级）——灰字离线模式，仅本地建议（REQ-504 口径）
          setRemote([])
          setOffline(true)
        })
    }, 400)
    return () => {
      if (timer.current) window.clearTimeout(timer.current)
    }
  }, [value, vocab])

  const q = (value ?? '').trim().toLowerCase()
  const localOpts = localNames
    .filter((n) => n !== exclude && (q === '' || n.toLowerCase().includes(q)))
    .slice(0, 8)
    .map((n) => ({
      value: n,
      vocabCard: undefined as VocabPick | undefined,
      label: (
        <span>
          {n} <Typography.Text type="secondary" style={{ fontSize: 10 }}>本地已声明</Typography.Text>
        </span>
      ),
    }))
  const remoteOpts = remote
    .filter((c) => (localOpts.some((o) => o.value === c.title) ? false : true))
    .map((c) => ({
      value: c.title,
      vocabCard: c,
      label: (
        <span>
          {c.title} <TagMiniVocab prefix={c.prefix} />
          {c.description && <Typography.Text type="secondary" style={{ fontSize: 10 }}> {c.description.slice(0, 40)}</Typography.Text>}
        </span>
      ),
    }))

  return (
    <>
      <AutoComplete
        value={value}
        onChange={onChange}
        options={[...localOpts, ...remoteOpts]}
        onSelect={(v, option) => {
          onChange?.(v)
          if (option?.vocabCard) onPickVocab?.(option.vocabCard as VocabPick)
        }}
        placeholder={placeholder}
        disabled={disabled}
        data-testid={testid}
        defaultActiveFirstOption={false}
        backfill={false}
      />
      {offline && (
        <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 2 }} data-testid="name-suggest-offline">
          离线模式，仅本地建议（词表服务不可达）
        </Typography.Text>
      )}
    </>
  )
}

function TagMiniVocab({ prefix }: { prefix: string }) {
  return (
    <span
      style={{
        display: 'inline-block',
        margin: '0 0 0 4px',
        padding: '0 5px',
        fontSize: 10,
        lineHeight: '16px',
        borderRadius: 4,
        background: 'var(--c-bg-soft, #f6f7fb)',
        color: 'var(--c-ink-3)',
      }}
    >
      LOV · {prefix}
    </span>
  )
}
