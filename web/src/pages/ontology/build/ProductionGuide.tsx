import { useState } from 'react'
import { Button, Space, Steps, Tooltip, Typography } from 'antd'
import {
  AuditOutlined,
  CloudServerOutlined,
  CloudUploadOutlined,
  CompassOutlined,
  DownOutlined,
  SafetyCertificateOutlined,
  UpOutlined,
} from '@ant-design/icons'
import { ONTO_SIDEBAR_KEY } from '../OntologyModule'

// ---------------------------------------------------------------------------
// REQ-334/M134 双模式叙事生产动线（79 号建议项 B）：为生产用户提供
// 「导入/构建 → 质量门禁 → 发布 → 运行消费」四步引导条——串 REQ-331 资产级质量
// 档案 + REQ-239 发布态 + 消费观测台（REQ-290）三段已交付能力；学习叙事不动
// （下方路径卡片即学习路径陈列，D-O11 五栏 IA 与默认页口径维持）。
// 双定位共存=模式/动线区分而非功能分叉：生产交付看上方动线，学习构建看路径卡片。
// 收起记忆 eino.onto.prodguide（'off' 收起，收起后首页留展开钮）。
// ---------------------------------------------------------------------------

const GUIDE_KEY = 'eino.onto.prodguide'

/** 跨栏跳转（沿演化按钮/gotoAuditPane 同款动线：写记忆 + onto-sidebar-change 事件）。 */
function gotoPane(sidebar: 'assets' | 'runtime' | 'audit') {
  localStorage.setItem(ONTO_SIDEBAR_KEY, sidebar)
  window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
}

const STEP_ITEMS = [
  {
    key: 'build',
    title: '① 导入 / 构建',
    desc: '六路径任一入口建成本体——自定义构建 S1 可直接上传 OWL/TTL 导入（导入对账零丢失，REQ-328）',
  },
  {
    key: 'quality',
    title: '② 质量门禁',
    desc: '资产详情「质量卡」：三维评分 / 评估历史 / 修复指引（REQ-331），建议通过门禁再发布',
  },
  {
    key: 'publish',
    title: '③ 发布',
    desc: 'draft → published 命名快照（REQ-239）：发布后运行向导优先装载已发布版本，编辑自动回 draft',
  },
  {
    key: 'consume',
    title: '④ 运行消费',
    desc: '启动运行方案（多引擎）→ 消费观测台看装载版本 / 质量快照 / 决策审计（REQ-290）',
  },
]

export default function ProductionGuide({ onGoBuild }: { onGoBuild: () => void }) {
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem(GUIDE_KEY) === 'off')

  const collapse = () => {
    localStorage.setItem(GUIDE_KEY, 'off')
    setCollapsed(true)
  }
  const expand = () => {
    localStorage.removeItem(GUIDE_KEY)
    setCollapsed(false)
  }

  if (collapsed) {
    return (
      <div style={{ marginBottom: 12 }}>
        <Button size="small" icon={<CompassOutlined />} data-testid="prod-guide-expand" onClick={expand}>
          生产动线
        </Button>
      </div>
    )
  }

  return (
    <div
      className="prod-guide"
      data-testid="prod-guide"
      style={{
        border: '1px solid var(--c-border, #f0f0f0)',
        borderRadius: 8,
        padding: '12px 16px 8px',
        marginBottom: 16,
        background: 'var(--c-panel, #fff)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
        <CompassOutlined style={{ color: 'var(--c-primary, #1668dc)' }} />
        <Typography.Text strong style={{ fontSize: 13 }}>
          生产动线
        </Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          从构建到消费的四步交付路径（质量门禁 → 发布 → 运行消费）；学习构建看下方路径卡片
        </Typography.Text>
        <Tooltip title="收起（可从「生产动线」按钮再展开）">
          <Button
            size="small"
            type="text"
            icon={<DownOutlined />}
            aria-label="收起生产动线"
            data-testid="prod-guide-collapse"
            onClick={collapse}
            style={{ marginLeft: 'auto' }}
          />
        </Tooltip>
      </div>
      <Steps
        size="small"
        current={-1}
        items={STEP_ITEMS.map((s) => ({ key: s.key, title: s.title, description: s.desc }))}
      />
      <Space size={8} wrap style={{ marginTop: 8 }}>
        <Button size="small" icon={<UpOutlined />} data-testid="prod-guide-go-build" onClick={onGoBuild}>
          开始构建 / 导入
        </Button>
        <Button size="small" icon={<SafetyCertificateOutlined />} data-testid="prod-guide-go-quality" onClick={() => gotoPane('assets')}>
          去质量门禁
        </Button>
        <Button size="small" icon={<CloudUploadOutlined />} data-testid="prod-guide-go-publish" onClick={() => gotoPane('assets')}>
          去发布
        </Button>
        <Button size="small" icon={<CloudServerOutlined />} data-testid="prod-guide-go-runtime" onClick={() => gotoPane('runtime')}>
          去运行
        </Button>
        <Button size="small" icon={<AuditOutlined />} data-testid="prod-guide-go-audit" onClick={() => gotoPane('audit')}>
          消费观测
        </Button>
      </Space>
    </div>
  )
}
