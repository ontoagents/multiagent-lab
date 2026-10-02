import React, { useEffect, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { ConfigProvider, App as AntdApp, theme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import App from './App'
import { AntdBridge } from './lib/antd'
import { initTheme, readTheme, THEME_EVENT, type ThemeName } from './lib/theme'
import ErrorBoundary from './components/ErrorBoundary'
import './styles.css'
import './pages.css'

// REQ-240 前端优化⑥：明暗主题——AntD 侧 algorithm 切换，自研 token 侧 html[data-theme] 覆盖
function ThemedApp() {
  const [t, setT] = useState<ThemeName>(readTheme)
  useEffect(() => {
    const sync = () => setT(readTheme())
    window.addEventListener(THEME_EVENT, sync)
    return () => window.removeEventListener(THEME_EVENT, sync)
  }, [])
  const dark = t === 'dark'
  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        token: {
          colorPrimary: dark ? '#818cf8' : '#4f46e5',
          colorInfo: dark ? '#818cf8' : '#4f46e5',
          borderRadius: 8,
          fontFamily:
            "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
        },
        algorithm: dark ? theme.darkAlgorithm : theme.defaultAlgorithm,
      }}
    >
      <AntdApp>
        <AntdBridge />
        <ErrorBoundary>
          <App />
        </ErrorBoundary>
      </AntdApp>
    </ConfigProvider>
  )
}

initTheme()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ThemedApp />
  </React.StrictMode>,
)
