import { Alert, Button, Card, Space } from 'antd'
import { ApiOutlined } from '@ant-design/icons'

// ---------------------------------------------------------------------------
// REQ-337/M137：KG 消费流程独立页（自 BuildPage 迁出，形态=入口引导卡）。
// D-O15 改造口径不变：原 semantica 流程入口，改跳「消费与审计」栏。
// ---------------------------------------------------------------------------

export default function KgGuidePage() {
  return (
    <Card className="work-card" size="small" data-testid="kg-guide-page">
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        title="KG 消费流程——自研 KG 图谱 / GraphRAG 试查 / 决策溯源（「消费与审计」栏承载）"
        description="知识库 chunk 语料经 REQ-98 LLM 能力代理抽取为自存 KG（SQLite 实体/关系/claim，D-O15），再消费为 GraphRAG 检索上下文或由 KG 直转本体（策略 B/C）；抽取/构建决策全程留痕可溯源。原 semantica worker 依赖已归档休眠。"
      />
      <div className="onto-sec" style={{ marginTop: 0 }}>
        <span className="onto-sec-title">与主线边界</span>
      </div>
      <ul className="onto-report-list">
        <li>本体建模与校验：归主线构建平面（本栏）</li>
        <li>KG 图谱浏览 / GraphRAG 试查 / 决策审计：归「消费与审计」第五栏</li>
        <li>KG→本体：本栏「由知识库构建」策略 B（kg-direct）/ C（hybrid）直读自存 KG</li>
      </ul>
      <Space style={{ marginTop: 12 }}>
        <Button
          type="primary"
          icon={<ApiOutlined />}
          onClick={() => {
            // 栏内切栏：sidebarKey 状态机经 localStorage + 事件同步（OntologyModule）
            localStorage.setItem('eino.onto.sidebar', 'audit')
            window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
          }}
        >
          前往消费与审计栏
        </Button>
      </Space>
    </Card>
  )
}
