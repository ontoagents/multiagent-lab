import React from 'react'
import { Button, Card, Collapse, Result, Typography } from 'antd'
import { ReloadOutlined, ClearOutlined } from '@ant-design/icons'

// ---------------------------------------------------------------------------
// 全局渲染错误边界（REQ-252）：React 渲染树内未捕获异常原本会卸载整棵 root=整站白屏
// 且无任何信息（开发者报障「本体界面打开失败，白屏」排查结论=当日多轮并行交付的
// vite 重建窗口/长开标签页 HMR 瞬态，代码无缺陷；本边界保证此后任何渲染错误可读可恢复）。
// 仅捕获渲染期错误——事件回调/异步任务的异常仍走 console，不在此拦。
// ---------------------------------------------------------------------------

interface BoundaryState {
  error: Error | null
  info: string | null
}

/** 清除本地缓存并刷新——本地状态（布局宽度/侧板记忆/打卡/主题等）损坏或陈旧时的一键自愈出口 */
function resetLocalAndReload() {
  try {
    localStorage.clear()
    sessionStorage.clear()
  } catch {
    /* 隐私模式等场景下清不掉就只刷新 */
  }
  window.location.reload()
}

export default class ErrorBoundary extends React.Component<{ children: React.ReactNode }, BoundaryState> {
  state: BoundaryState = { error: null, info: null }

  static getDerivedStateFromError(error: Error): Partial<BoundaryState> {
    return { error }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    // eslint-disable-next-line no-console
    console.error('[ErrorBoundary]', error, info.componentStack)
    this.setState({ info: info.componentStack ?? null })
  }

  render() {
    const { error, info } = this.state
    if (!error) return this.props.children
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh', padding: 24 }}>
        <Card style={{ maxWidth: 720, width: '100%' }}>
          <Result
            status="error"
            title="页面渲染出错"
            subTitle="界面遇到未捕获的渲染异常。多数情况下「清除本地缓存并刷新」可恢复（清除布局宽度/侧板视图/打卡等本地记忆）；若反复出现，请将下方错误信息反馈给开发者。"
            extra={
              <>
                <Button type="primary" icon={<ReloadOutlined />} onClick={() => window.location.reload()}>
                  刷新页面
                </Button>
                <Button icon={<ClearOutlined />} onClick={resetLocalAndReload}>
                  清除本地缓存并刷新
                </Button>
              </>
            }
          />
          <Typography.Paragraph type="secondary" code style={{ fontSize: 12, wordBreak: 'break-all' }}>
            {error.message}
          </Typography.Paragraph>
          {info && (
            <Collapse
              size="small"
              items={[
                {
                  key: 'stack',
                  label: '组件栈（排查用）',
                  children: (
                    <pre style={{ fontSize: 11, maxHeight: 240, overflow: 'auto', margin: 0, whiteSpace: 'pre-wrap' }}>{info}</pre>
                  ),
                },
              ]}
            />
          )}
        </Card>
      </div>
    )
  }
}
