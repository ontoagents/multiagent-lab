import type { ReactNode } from 'react'
import { Tag, Typography } from 'antd'
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
      <div className="work-head">
        <div className="work-head-text">
          <div className="work-head-title">
            <Typography.Title level={4} style={{ margin: 0 }}>
              本体构建
            </Typography.Title>
            <Tag color="blue" style={{ margin: 0 }}>六条构建路径</Tag>
          </div>
          <p className="work-head-desc">
            同一个本体可以由不同路径建成（学习要点各不相同）；产物统一进入「本体资产」栏管理。
          </p>
        </div>
      </div>

      {/* 顶栏路径切换器（REQ-337：六路径页面化后各页共用的常驻导航） */}
      <div className="build-path-tabs" role="tablist" aria-label="构建路径切换" data-testid="build-path-tabs">
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

      <div className="build-path-body">{children}</div>
    </div>
  )
}
