import { useState } from 'react'
import { Space } from 'antd'
import { Collapse, Typography } from 'antd'
import { QuestionCircleOutlined } from '@ant-design/icons'

// REQ-505/M313：可折叠「为什么」提示条——建模决策点内联方法论（边建边教），不打断操作流。
// 100 号 R6：＋子类→上下位追问 / 关系命名→查询主体判据 / CQ 输入框→验收测试用例 / 空态引导卡附微型示例。

export default function WhyHint({ children, testid }: { children: React.ReactNode; testid?: string }) {
  const [open, setOpen] = useState(false)
  return (
    <Collapse
      size="small"
      ghost
      activeKey={open ? ['why'] : []}
      onChange={() => setOpen((o) => !o)}
      items={[
        {
          key: 'why',
          label: (
            <Space size={4}>
              <QuestionCircleOutlined style={{ color: 'var(--c-brand)' }} />
              <Typography.Text type="secondary" style={{ fontSize: 11.5 }}>
                为什么这样建模？（方法论决策点）
              </Typography.Text>
            </Space>
          ),
          children: (
            <Typography.Text type="secondary" style={{ fontSize: 12, lineHeight: 1.7, display: 'block' }} data-testid={testid}>
              {children}
            </Typography.Text>
          ),
        },
      ]}
    />
  )
}
