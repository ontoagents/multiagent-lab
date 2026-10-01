// REQ-240③/M66：能力雷达五维图（SVG 自绘零新依赖——完备/一致/可维护=qualitygate 三维分，
// 规模覆盖/连接密度=stats 派生；与 REQ-234 装载快照同数据源 qualitygate）。
// 纯函数派生：radarDimsOf(score?, stats) → 五维 0~100；score 缺失时仅 stats 两维以 50 中线降级呈现。

const SIZE = 200
const RADIUS = 74
const LEVELS = 4

/** 五维雷达值派生（qualitygate score + spec stats；两 stats 维归一化口径见下） */
export function radarDimsOf(
  score: { completeness: number; consistency: number; maintainability: number } | null | undefined,
  stats: { concepts: number; relations: number; instances: number } | { n_concepts: number; n_relations: number; n_instances: number },
): [string, number][] {
  const concepts = Math.max(0, (stats as any).concepts ?? (stats as any).n_concepts ?? 0)
  const relations = Math.max(0, (stats as any).relations ?? (stats as any).n_relations ?? 0)
  const instances = Math.max(0, (stats as any).instances ?? (stats as any).n_instances ?? 0)
  // 规模覆盖：实例对概念的覆盖度（每概念 1 实例=满格，封顶 100）
  const coverage = concepts > 0 ? Math.min(100, (instances / concepts) * 100) : 0
  // 连接密度：平均每概念关系数（每概念 2 条关系=满格，封顶 100）
  const density = concepts > 0 ? Math.min(100, (relations / concepts) * 50) : 0
  return [
    ['完备', score ? clamp(score.completeness) : 0],
    ['一致', score ? clamp(score.consistency) : 0],
    ['可维护', score ? clamp(score.maintainability) : 0],
    ['规模覆盖', clamp(coverage)],
    ['连接密度', clamp(density)],
  ]
}

const clamp = (v: number) => Math.max(0, Math.min(100, Number.isFinite(v) ? v : 0))

export default function QualityRadar({
  dims,
  size = SIZE,
  compact = false,
}: {
  dims: [string, number][]
  size?: number
  /** compact：资产卡微型形态（无刻度环数字/细线） */
  compact?: boolean
}) {
  const n = dims.length
  const scale = size / SIZE
  const cx = size / 2
  const cy = size / 2 + (compact ? 0 : 2)
  const r = RADIUS * scale
  // 顶点坐标（从正上方起，顺时针）
  const pt = (i: number, ratio: number): [number, number] => {
    const ang = -Math.PI / 2 + (i * 2 * Math.PI) / n
    return [cx + r * ratio * Math.cos(ang), cy + r * ratio * Math.sin(ang)]
  }
  const ringPath = (ratio: number) => dims.map((_, i) => pt(i, ratio)).map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)},${y.toFixed(1)}`).join(' ') + ' Z'
  const dataPath = dims.map(([, v], i) => pt(i, v / 100)).map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)},${y.toFixed(1)}`).join(' ') + ' Z'
  const score = dims.reduce((s, [, v]) => s + v, 0) / (n || 1)

  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} role="img" aria-label={`能力雷达：${dims.map(([k, v]) => `${k} ${Math.round(v)}`).join(' / ')}`} data-testid="quality-radar">
      {/* 网格环 */}
      {Array.from({ length: LEVELS }, (_, i) => (i + 1) / LEVELS).map((ratio) => (
        <path key={ratio} d={ringPath(ratio)} fill="none" stroke="var(--c-border, #d9dcec)" strokeWidth={compact ? 0.5 : 0.8} />
      ))}
      {/* 轴线 + 标签 */}
      {dims.map(([label], i) => {
        const [x, y] = pt(i, 1)
        const [lx, ly] = pt(i, 1.22)
        return (
          <g key={label}>
            <line x1={cx} y1={cy} x2={x} y2={y} stroke="var(--c-border, #d9dcec)" strokeWidth={compact ? 0.5 : 0.8} />
            <text
              x={lx}
              y={ly}
              textAnchor={Math.abs(lx - cx) < 6 ? 'middle' : lx > cx ? 'start' : 'end'}
              dominantBaseline="middle"
              fontSize={compact ? 8.5 : 10.5}
              fill="var(--c-ink-3, #9aa1bd)"
            >
              {label}
            </text>
            {!compact && (
              <text x={lx} y={ly + 12} textAnchor={Math.abs(lx - cx) < 6 ? 'middle' : lx > cx ? 'start' : 'end'} dominantBaseline="middle" fontSize={9.5} fill="var(--c-ink-2, #5a6280)">
                {Math.round(dims[i][1])}
              </text>
            )}
          </g>
        )
      })}
      {/* 数据多边形（五维均值着色：≥90 绿 / ≥70 琥珀 / 其余红） */}
      <path d={dataPath} fill={score >= 90 ? 'rgba(22,163,74,0.18)' : score >= 70 ? 'rgba(217,119,6,0.18)' : 'rgba(220,38,38,0.15)'} stroke={score >= 90 ? '#16a34a' : score >= 70 ? '#d97706' : '#dc2626'} strokeWidth={compact ? 1 : 1.5} />
      {dims.map(([k, v], i) => {
        const [x, y] = pt(i, v / 100)
        return <circle key={k} cx={x} cy={y} r={compact ? 1.5 : 2.2} fill={score >= 90 ? '#16a34a' : score >= 70 ? '#d97706' : '#dc2626'} />
      })}
    </svg>
  )
}
