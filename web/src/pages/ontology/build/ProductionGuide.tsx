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
import { useOntologies } from '../shared/useOntoList'

// ---------------------------------------------------------------------------
// REQ-334/M134 双模式叙事生产动线（79 号建议项 B）：为生产用户提供
// 「导入/构建 → 质量门禁 → 发布 → 运行消费」四步引导条——串 REQ-331 资产级质量
// 档案 + REQ-239 发布态 + 消费观测台（REQ-290）三段已交付能力；学习叙事不动
// （下方路径卡片即学习路径陈列，D-O11 五栏 IA 与默认页口径维持）。
// 双定位共存=模式/动线区分而非功能分叉：生产交付看上方动线，学习构建看路径卡片。
// 收起记忆 eino.onto.prodguide（REQ-484/M283：默认折叠——进阶语境让位新手首屏；'off' 收起，展开态不记忆即刷新回落折叠）。
// ---------------------------------------------------------------------------

const GUIDE_KEY = 'eino.onto.prodguide'

/** 跨栏跳转（沿演化按钮/gotoAuditPane 同款动线：写记忆 + onto-sidebar-change 事件）。
 *  REQ-407/M203：gotoAsset 额外写 eino.onto.asset.focus 深链——「去质量门禁/去发布」直达
 *  本体详情（AssetsPage 挂载即消费该键选中+进详情），不再裸跳资产栏。 */
function gotoPane(sidebar: 'assets' | 'runtime' | 'audit', assetId?: string) {
  localStorage.setItem(ONTO_SIDEBAR_KEY, sidebar)
  if (assetId) localStorage.setItem('eino.onto.asset.focus', assetId)
  window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
}

const STEP_ITEMS = [
  {
    key: 'build',
    title: '① 导入 / 构建',
    desc: '六条路径任一入口建成本体；自定义构建可直接上传 OWL/TTL 导入（导入对账零丢失）',
  },
  {
    key: 'quality',
    title: '② 质量门禁',
    desc: '资产详情「校验与质量」：三维评分 / 评估历史 / 修复指引——建议通过门禁再发布',
  },
  {
    key: 'publish',
    title: '③ 发布',
    desc: 'draft → published 命名快照：发布后运行向导优先装载已发布版本，编辑自动回 draft',
  },
  {
    key: 'consume',
    title: '④ 运行消费',
    desc: '启动运行方案（多引擎）→ 消费观测台看装载版本 / 质量快照 / 决策审计',
  },
]

export default function ProductionGuide({ onGoBuild, focusAssetId }: { onGoBuild: () => void; focusAssetId?: string | null }) {
  // REQ-484：默认折叠（用户显式展开后本会话保持；不再记忆展开态——首屏 novice-first）
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem(GUIDE_KEY) !== 'on')
  // REQ-407/M203：首页无选中态——惰性取最近创建本体作「去质量门禁/去发布」的深链目标
  // REQ-467/M267：本体清单单源 hook（散拉收敛）
  const { ontos: guideOntos } = useOntologies()
  const latestOntoId = guideOntos[0]?.id ?? null

  const collapse = () => {
    localStorage.setItem(GUIDE_KEY, 'off')
    setCollapsed(true)
  }
  const expand = () => {
    localStorage.setItem(GUIDE_KEY, 'on')
    setCollapsed(false)
  }

  if (collapsed) {
    return (
      <div style={{ marginTop: 14, display: 'flex' }}>
        <Button size="small" type="dashed" icon={<CompassOutlined />} data-testid="prod-guide-expand" onClick={expand}>
          构建之后呢？查看从构建到消费的四步动线
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
          从构建到消费的四步交付路径；选择路径构建看上方卡片
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
        <Button size="small" icon={<SafetyCertificateOutlined />} data-testid="prod-guide-go-quality" onClick={() => gotoPane('assets', focusAssetId ?? latestOntoId ?? undefined)}>
          去质量门禁
        </Button>
        <Button size="small" icon={<CloudUploadOutlined />} data-testid="prod-guide-go-publish" onClick={() => gotoPane('assets', focusAssetId ?? latestOntoId ?? undefined)}>
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
