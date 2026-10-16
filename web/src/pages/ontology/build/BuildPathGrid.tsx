import { Card, Tag, Typography } from 'antd'
import {
  ApartmentOutlined,
  BookOutlined,
  CloudDownloadOutlined,
  CommentOutlined,
  DatabaseOutlined,
  PartitionOutlined,
} from '@ant-design/icons'
import { PATHS, STATE_TAG, type BuildPath } from './paths'

// ---------------------------------------------------------------------------
// REQ-337/M137：构建首页收敛为「路径选择页」——六路径卡片降为入口（成熟度角标保留）。
// 主人追加需求「当前本体构建页面拥挤，为每种构建类型创建单独新增创建页面」：
// 原左路径导航+右工作区两栏退役，各路径升独立页面（顶栏路径切换器切换），本页只做选择。
// ---------------------------------------------------------------------------

const PATH_ICON: Record<string, React.ReactNode> = {
  custom: <ApartmentOutlined />,
  ontochat: <CommentOutlined />,
  kb: <DatabaseOutlined />,
  kg: <PartitionOutlined />,
  ontoextend: <PartitionOutlined />,
  catalog: <CloudDownloadOutlined />,
}

export default function BuildPathGrid({ onSelect }: { onSelect: (key: BuildPath) => void }) {
  return (
    <div data-testid="build-path-grid">
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 12 }}>
        选择一条构建路径进入独立创建页；同一个本体可以由不同路径建成（学习要点各不相同），产物统一进入「本体资产」栏管理。
      </Typography.Paragraph>
      <div className="build-path-grid">
        {PATHS.map((p) => (
          <Card
            key={p.key}
            size="small"
            hoverable
            className="build-path-card"
            onClick={() => onSelect(p.key)}
            data-testid={`build-path-card-${p.key}`}
          >
            <div className="build-path-card-head">
              <span className="build-path-card-icon">{PATH_ICON[p.key]}</span>
              <span className="build-path-card-title">{p.label}</span>
              <Tag color={STATE_TAG[p.key].color} style={{ margin: 0, fontSize: 10, lineHeight: '16px', padding: '0 6px' }}>
                {STATE_TAG[p.key].text}
              </Tag>
            </div>
            <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0, minHeight: 34 }}>
              {p.desc}
            </Typography.Paragraph>
          </Card>
        ))}
      </div>
      <Typography.Paragraph type="secondary" style={{ fontSize: 11, marginTop: 12, marginBottom: 0 }}>
        <BookOutlined /> 提示：路径间可随时经顶部切换器跳转；进行中的草稿（AI 草案/导入报告/对话会话）在各自页面内暂存。
      </Typography.Paragraph>
    </div>
  )
}
