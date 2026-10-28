import type { ReactNode } from 'react'
import { Typography } from 'antd'
import { AppstoreOutlined } from '@ant-design/icons'
import { PATHS, STATE_TAG, type BuildPath } from './paths'

// ---------------------------------------------------------------------------
// REQ-337/M137：BuildLayout——构建栏共享布局（80 号 U4 拆分承载之一）：
//   work-head（标题不变）+ 顶栏路径切换器（六路径独立页面间的常驻切换；
//   形态沿 module-tabs 页签语言，含「路径选择」首页项与成熟度角标）。
// ---------------------------------------------------------------------------

export default function BuildLayout({
  buildPath,
  onSelect,
  children,
}: {
  buildPath: BuildPath
  onSelect: (key: BuildPath) => void
  children: ReactNode
}) {
  return (
    <div className="work-main">
      {/* REQ-485/M284：work-head 与路径页签合并一行——压缩公共壳（原 ~170px chrome 才见正文） */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 14, marginBottom: 12, flexWrap: 'wrap' }} data-testid="build-shell-head">
        <Typography.Title level={5} style={{ margin: 0, flex: 'none' }}>
          本体构建
        </Typography.Title>
        <div className="build-path-tabs" role="tablist" aria-label="构建路径切换" data-testid="build-path-tabs" style={{ marginBottom: 0, padding: '0 0 4px', borderBottom: 'none' }}>
        <button
          type="button"
          role="tab"
          aria-selected={buildPath === 'home'}
          className={`build-path-tab${buildPath === 'home' ? ' active' : ''}`}
          onClick={() => onSelect('home')}
          data-testid="build-path-tab-home"
        >
          <span className="build-path-tab-icon">
            <AppstoreOutlined />
          </span>
          <span className="build-path-tab-label">路径选择</span>
        </button>
        {PATHS.map((p) => (
          <button
            key={p.key}
            type="button"
            role="tab"
            aria-selected={buildPath === p.key}
            className={`build-path-tab${buildPath === p.key ? ' active' : ''}`}
            onClick={() => onSelect(p.key)}
            data-testid={`build-path-tab-${p.key}`}
          >
            <span className="build-path-tab-label">{p.label}</span>
            <span className={`build-path-dot state-${p.state}`} title={STATE_TAG[p.key].text} />
          </button>
        ))}
        </div>
      </div>

      <div className="build-path-body">{children}</div>
    </div>
  )
}
// 注：REQ-485 合并行后原 work-head 区块（大标题+六条构建路径 Tag+desc 段）退役——
// 「六条构建路径」语义由页签行自身承载（Tag 冗余删除）；desc 由各路径页 PathIntro 一句话承接。
