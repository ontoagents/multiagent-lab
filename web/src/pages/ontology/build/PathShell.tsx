import type { ReactNode } from 'react'
import { Button, Space, Steps, Tag, Typography } from 'antd'
import { ArrowLeftOutlined } from '@ant-design/icons'
import { PATHS, STATE_TAG, type BuildPath } from './paths'
import { deepLinkOnto } from '../shared/ontoRoute'

// ---------------------------------------------------------------------------
// REQ-598/M409 三共享壳（111 号 §3.3）：PathShell（路径页顶条，替代 BuildLayout
// 页签行职责）/ FlowSteps（向导型路径共用步骤条——步数可有差异、语言必须一致）/
// ConvergeCard（生成型路径完成时刻统一三选——进入编辑校验/前往资产详情/再建一个，
// 落地「差异只在生成段、收敛在资产段」的交互面统一）。
// 视觉静音 C7：灰字顶条、状态 Tag 异常态留色。
// ---------------------------------------------------------------------------

/** 路径页顶条：← 构建入口（回 hub，不丢进行中态——job 与草稿均在持久层）+ 路径名
 *  + 诚实状态徽标 + 右端该路径主操作。正文 work-main 标准结构。 */
export default function PathShell({
  path,
  onBack,
  actions,
  children,
}: {
  path: Exclude<BuildPath, 'home'>
  onBack: () => void
  actions?: ReactNode
  children: ReactNode
}) {
  const meta = PATHS.find((p) => p.key === path)
  const tag = STATE_TAG[path]
  return (
    <div className="work-main">
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12, flexWrap: 'wrap' }} data-testid={`path-shell-${path}`}>
        <Button size="small" type="text" icon={<ArrowLeftOutlined />} onClick={onBack} data-testid="path-back-hub">
          构建入口
        </Button>
        <Typography.Title level={5} style={{ margin: 0, flex: 'none' }}>
          {meta?.label ?? path}
        </Typography.Title>
        <Tag color={tag.color} style={{ margin: 0, fontSize: 11, lineHeight: '18px', padding: '0 8px' }} data-testid={`path-state-${path}`}>
          {tag.text}
        </Tag>
        {meta?.when && (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {meta.when}
          </Typography.Text>
        )}
        {actions && <Space size={8} style={{ marginLeft: 'auto' }}>{actions}</Space>}
      </div>
      <div className="build-path-body">{children}</div>
    </div>
  )
}

/** 向导型路径共用步骤条（视觉对齐 custom 的 Steps 语言）。各路径步数不变（不做假统一）。 */
export function FlowSteps({
  items,
  current,
  onChange,
}: {
  items: { key: string; title: string; description?: string }[]
  current: number
  onChange?: (index: number) => void
}) {
  return (
    <Steps
      size="small"
      current={current}
      onChange={onChange}
      items={items.map((s, i) => ({
        key: s.key,
        title: s.title,
        description: s.description,
        status: i === current ? 'process' : 'wait',
        className: i === current ? 'onto-step-selected' : undefined,
      }))}
      style={{ marginBottom: 14 }}
    />
  )
}

/** 收敛卡统一件（111 号 §3.3-ConvergeCard）：生成型路径完成时刻统一三选——
 *  ①进入编辑校验（深链 custom ② 编辑步）②前往资产详情 ③再建一个（重置本路径）。
 *  REQ-600：datasource/kb/ontochat 等各路径完成件全量替换为本件（custom S4 编辑完成态保留 DoneCTA）。 */
export function ConvergeCard({
  ontologyId,
  detail,
  onRebuild,
}: {
  ontologyId: string
  detail?: string
  onRebuild?: () => void
}) {
  return (
    <Space direction="vertical" size={6} style={{ display: 'flex' }} data-testid="converge-card">
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        {detail ?? '构建产物已入库'}，下一步：
      </Typography.Text>
      <Space wrap>
        <Button
          size="small"
          type="primary"
          onClick={() => deepLinkOnto({ col: 'build', path: 'custom', params: { onto: ontologyId, step: '1' } })}
        >
          进入编辑校验
        </Button>
        <Button size="small" onClick={() => deepLinkOnto({ col: 'assets', assetId: ontologyId }, [['eino.onto.asset.focus', ontologyId]])}>
          前往资产详情
        </Button>
        {onRebuild && <Button size="small" onClick={onRebuild}>再建一个</Button>}
      </Space>
    </Space>
  )
}
