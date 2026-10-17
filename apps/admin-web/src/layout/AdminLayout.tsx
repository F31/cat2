import { useEffect, useMemo, useState } from 'react'
import { Layout, Menu, Button, Space } from 'antd'
import type { MenuProps } from 'antd'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { clearToken } from '../auth'
import { adminApi, type AdminMenu } from '../api/admin'

const { Header, Sider, Content } = Layout

type Group = { key: string; label: string; order: number; items: AdminMenu[] }

// 拉取失败时的静态兜底（与后端 AdminSection 分组一致）。
const FALLBACK: AdminMenu[] = [
  { key: 'overview', label: '经营概览', path: '/dashboard', module: 'ops', group: 'overview', group_label: '概览', group_order: 1, order: 1 },
  { key: 'content.list', label: '内容列表', path: '/contents', module: 'content', group: 'content', group_label: '内容管理', group_order: 2, order: 1 },
  { key: 'content.reviews', label: '审核队列', path: '/reviews', module: 'trust', group: 'content', group_label: '内容管理', group_order: 2, order: 2 },
  { key: 'content.certs', label: '认证审核', path: '/verify-certs', module: 'verify', group: 'content', group_label: '内容管理', group_order: 2, order: 3 },
  { key: 'content.badges', label: '认证标识', path: '/badges', module: 'verify', group: 'content', group_label: '内容管理', group_order: 2, order: 4 },
  { key: 'content.dict', label: '主体类型与选项', path: '/dict', module: 'dict', group: 'content', group_label: '内容管理', group_order: 2, order: 5 },
  { key: 'content.templates', label: '画像字段模板', path: '/profile-templates', module: 'profile', group: 'content', group_label: '内容管理', group_order: 2, order: 6 },
  { key: 'trade.orders', label: '订单管理', path: '/orders', module: 'trade', group: 'trade', group_label: '交易与会员', group_order: 3, order: 1 },
  { key: 'trade.members', label: '会员管理', path: '/members', module: 'membership', group: 'trade', group_label: '交易与会员', group_order: 3, order: 2 },
  { key: 'trade.exports', label: '导出中心', path: '/exports', module: 'ops', group: 'trade', group_label: '交易与会员', group_order: 3, order: 3 },
  { key: 'collab.cubes', label: '猫立方管理', path: '/cubes', module: 'cube', group: 'collab', group_label: '圈子与立方', group_order: 4, order: 1 },
  { key: 'collab.circles', label: '圈子管理', path: '/circles', module: 'circle', group: 'collab', group_label: '圈子与立方', group_order: 4, order: 2 },
  { key: 'operation.channel', label: '运营商', path: '/channel-operators', module: 'channel', group: 'operation', group_label: '运营管理', group_order: 5, order: 1 },
  { key: 'operation.settlements', label: '渠道结算', path: '/channel-settlements', module: 'channel', group: 'operation', group_label: '运营管理', group_order: 5, order: 2 },
  { key: 'operation.governance', label: '代操作', path: '/governance', module: 'ops', group: 'operation', group_label: '运营管理', group_order: 5, order: 3 },
  { key: 'operation.ai', label: 'AI 提示词与用量', path: '/ai-prompts', module: 'ai', group: 'operation', group_label: '运营管理', group_order: 5, order: 4 },
  { key: 'authority.users', label: '用户管理', path: '/users', module: 'identity', group: 'authority', group_label: '权限管理', group_order: 6, order: 1 },
  { key: 'authority.roles', label: '角色管理', path: '/roles', module: 'ops', group: 'authority', group_label: '权限管理', group_order: 6, order: 2 },
  { key: 'system.settings', label: '系统设置', path: '/system-settings', module: 'ops', group: 'system', group_label: '系统管理', group_order: 7, order: 1 },
  { key: 'system.audits', label: '日志审计', path: '/audits', module: 'ops', group: 'system', group_label: '系统管理', group_order: 7, order: 2 },
  { key: 'system.flags', label: '特性开关', path: '/feature-flags', module: 'ops', group: 'system', group_label: '系统管理', group_order: 7, order: 3 },
  { key: 'system.dead', label: '死信队列', path: '/dead-letters', module: 'ops', group: 'system', group_label: '系统管理', group_order: 7, order: 4 },
  { key: 'system.notice', label: '通知模板', path: '/templates', module: 'notice', group: 'system', group_label: '系统管理', group_order: 7, order: 5 },
  { key: 'system.business', label: '业务配置', path: '/settings', module: 'ops', group: 'system', group_label: '系统管理', group_order: 7, order: 6 },
  { key: 'system.miniapp', label: '小程序设置', path: '/miniapp-settings', module: 'ops', group: 'system', group_label: '系统管理', group_order: 7, order: 7 },
]

function buildGroups(items: AdminMenu[]): Group[] {
  const m = new Map<string, Group>()
  for (const it of items) {
    const key = it.group || 'system'
    const g = m.get(key) ?? { key, label: it.group_label || '系统管理', order: it.group_order ?? 99, items: [] }
    g.items.push(it)
    m.set(key, g)
  }
  return [...m.values()]
    .sort((a, b) => a.order - b.order)
    .map((g) => ({ ...g, items: [...g.items].sort((a, b) => (a.order ?? 99) - (b.order ?? 99)) }))
}

function matchItem(groups: Group[], pathname: string): { group: Group; item?: AdminMenu } {
  for (const g of groups) {
    const item = g.items.find((i) => pathname === i.path || pathname.startsWith(`${i.path}/`))
    if (item) return { group: g, item }
  }
  return { group: groups[0] }
}

// 后台壳：顶部一级菜单 + 左侧二级菜单（内容区居中）。
export default function AdminLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const [menus, setMenus] = useState<AdminMenu[]>(FALLBACK)

  useEffect(() => {
    void (async () => {
      try {
        const res = await adminApi.menus()
        if (res.items?.length) setMenus(res.items)
      } catch {
        /* 回退静态菜单 */
      }
    })()
  }, [])

  const groups = useMemo(() => buildGroups(menus), [menus])
  const active = useMemo(() => matchItem(groups, location.pathname), [groups, location.pathname])
  const topItems: MenuProps['items'] = groups.map((g) => ({ key: g.key, label: g.label }))
  const sideItems: MenuProps['items'] = active.group.items.map((i) => ({ key: i.path, label: i.label }))

  const logout = () => {
    clearToken()
    navigate('/login', { replace: true })
  }

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ background: '#fff', display: 'flex', alignItems: 'center', paddingInline: 16, gap: 16 }}>
        <div style={{ color: '#111', fontWeight: 600, whiteSpace: 'nowrap' }}>猫平方管理后台</div>
        <Menu
          mode="horizontal"
          selectedKeys={[active.group.key]}
          items={topItems}
          onClick={(e) => {
            const g = groups.find((x) => x.key === e.key)
            if (g?.items[0]) navigate(g.items[0].path)
          }}
          style={{ flex: 1, minWidth: 0, borderBottom: 'none' }}
        />
        <Space>
          <Button onClick={logout}>退出登录</Button>
        </Space>
      </Header>
      <Layout>
        <Sider theme="dark" width={208}>
          <Menu
            theme="dark"
            mode="inline"
            selectedKeys={[active.item?.path ?? active.group.items[0]?.path ?? '']}
            items={sideItems}
            onClick={(e) => navigate(e.key)}
          />
        </Sider>
        <Content style={{ margin: 16, overflow: 'auto' }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
