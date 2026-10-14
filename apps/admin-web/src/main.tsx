import React from 'react'
import ReactDOM from 'react-dom/client'
import { ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'

// 管理后台入口（骨架）。RBAC 路由与二次确认交互在后续 change 接入。
function App() {
  return <div>猫平方管理后台（骨架已就位，页面在 p2 起逐模块接入）</div>
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN}>
      <App />
    </ConfigProvider>
  </React.StrictMode>
)
