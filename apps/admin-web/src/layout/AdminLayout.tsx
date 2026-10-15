import { Layout, Menu, Button, Space } from 'antd'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { clearToken } from '../auth'

const { Header, Sider, Content } = Layout

// 后台壳：侧栏菜单 + 顶栏 + 内容区。菜单项对应各模块基础页面。
const MENU = [
  { key: '/dashboard', label: '概览' },
  { key: '/users', label: '用户' },
  { key: '/contents', label: '内容' },
  { key: '/reviews', label: '审核队列' },
  { key: '/orders', label: '订单' },
  { key: '/members', label: '会员' },
  { key: '/templates', label: '通知模板' },
]

export default function AdminLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const selected = MENU.find((m) => location.pathname.startsWith(m.key))?.key ?? '/dashboard'

  const logout = () => {
    clearToken()
    navigate('/login', { replace: true })
  }

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider theme="dark">
        <div style={{ color: '#fff', padding: 16, fontWeight: 600 }}>猫平方管理后台</div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selected]}
          items={MENU}
          onClick={(e) => navigate(e.key)}
        />
      </Sider>
      <Layout>
        <Header style={{ background: '#fff', display: 'flex', justifyContent: 'flex-end', alignItems: 'center' }}>
          <Space>
            <Button onClick={logout}>退出登录</Button>
          </Space>
        </Header>
        <Content style={{ margin: 16 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
