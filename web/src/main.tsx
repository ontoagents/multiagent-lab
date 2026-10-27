import React, { useEffect, useState } from 'react'
import ReactDOM from 'react-dom/client'
import { ConfigProvider, App as AntdApp, theme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import App from './App'
import { AntdBridge } from './lib/antd'
import { initTheme, readTheme, THEME_EVENT, type ThemeName } from './lib/theme'
import ErrorBoundary from './components/ErrorBoundary'
import { installNativePickDir } from './api/native'
import './styles.css'
import './pages.css'

// REQ-133 桌面端原生目录选择：Tauri 壳内改写 api.pickProjectDir 走壳进程原生对话框
// （浏览器/web 部署无 __TAURI__ 全局，补丁自识别、行为不变）。须在任意页面渲染前安装。
installNativePickDir()

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
