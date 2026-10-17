import { useEffect, useState } from 'react'
import { Layout, Menu, Button, Space } from 'antd'
import type { MenuProps } from 'antd'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { clearToken } from '../auth'
import { adminApi } from '../api/admin'

const { Header, Sider, Content } = Layout

// 后台壳：菜单由服务端 AdminSection 声明（p1 §9.9）；拉取失败回退静态菜单。
const FALLBACK: MenuProps['items'] = [
  { key: '/dashboard', label: '概览' },
  { key: '/users', label: '用户' },
  { key: '/contents', label: '内容' },
  { key: '/dict', label: '主体类型与选项' },
  { key: '/profile-templates', label: '画像字段模板' },
  { key: '/reviews', label: '审核队列' },
  { key: '/verify-certs', label: '认证审核' },
  { key: '/orders', label: '订单' },
  { key: '/members', label: '会员' },
  { key: '/cubes', label: '猫立方管理' },
  { key: '/circles', label: '圈子管理' },
  { key: '/templates', label: '通知模板' },
  { key: '/dead-letters', label: '死信队列' },
  { key: '/feature-flags', label: '特性开关' },
  { key: '/settings', label: '业务配置' },
  { key: '/audits', label: '审计轨迹' },
  { key: '/exports', label: '导出中心' },
  { key: '/governance', label: '代操作' },
  { key: '/ai-prompts', label: 'AI 提示词与用量' },
  { key: '/badges', label: '认证标识' },
  { key: '/roles', label: '角色与权限' },
  { key: '/system-settings', label: '系统设置' },
  { key: '/miniapp-settings', label: '小程序设置' },
  { key: '/channel-operators', label: '运营商' },
  { key: '/channel-settlements', label: '渠道结算' },
]

export default function AdminLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const [items, setItems] = useState<MenuProps['items']>(FALLBACK)

  useEffect(() => {
    void (async () => {
      try {
        const res = await adminApi.menus()
        if (res.items?.length) {
          setItems([
            { key: '/dashboard', label: '概览' },
            ...res.items.map((m) => ({ key: m.path, label: m.label })),
          ])
        }
      } catch {
        /* 回退静态菜单 */
      }
    })()
  }, [])

  const menuKeys = (items ?? []).map((i) => String((i as { key: string }).key))
  const selected = menuKeys.find((k) => location.pathname.startsWith(k)) ?? '/dashboard'

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
          items={items}
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
