import { Button, Space, Typography } from 'antd'
import { DatabaseOutlined, RightOutlined, SafetyCertificateOutlined, CloudServerOutlined } from '@ant-design/icons'

/**
 * REQ-250/G6：构建路径完成态统一 CTA——三按钮（前往资产编辑 / 查看质量卡 / 前往本体运行）。
 * 此前各路径完成态多为纯文案无按钮（OntoChat onSaved 空函数、OntoExtend 纯文本），
 * 仅 KbBuildFlow 有跳转；本组件统一收口（58 号 E1）。
 * 跳转经 eino.onto.sidebar localStorage + onto-sidebar-change 事件（全模块通用动线先例）；
 * 质量卡直达经 eino.onto.focus 约定（AssetsPage 选中后默认分区逻辑不动，落资产栏即可）。
 */
export default function DoneCTA({ ontologyId, detail = '构建产物已入库' }: { ontologyId: string; detail?: string }) {
  const go = (key: 'assets' | 'runtime') => {
    localStorage.setItem('eino.onto.sidebar', key)
    if (ontologyId) localStorage.setItem('eino.onto.focus', ontologyId)
    window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
  }
  return (
    <Space direction="vertical" size={6} style={{ display: 'flex' }}>
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>{detail}，下一步：</Typography.Text>
      <Space wrap>
        <Button size="small" type="primary" icon={<DatabaseOutlined />} onClick={() => go('assets')}>
          前往资产编辑
        </Button>
        <Button size="small" icon={<SafetyCertificateOutlined />} onClick={() => go('assets')}>
          查看质量卡
        </Button>
        <Button size="small" icon={<CloudServerOutlined />} onClick={() => go('runtime')}>
          前往本体运行 <RightOutlined />
        </Button>
      </Space>
    </Space>
  )
}
