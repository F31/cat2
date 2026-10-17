import { useCallback, useEffect, useMemo, useState } from 'react'
import { Layout, Menu, Button, Space, Dropdown, Modal, Descriptions, Tag, theme } from 'antd'
import type { MenuProps } from 'antd'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { clearToken } from '../auth'
import { adminApi, type AdminMenu } from '../api/admin'

const { Header, Sider, Content } = Layout

type GroupInfo = { key: string; label: string; order: number }
type Me = { user_id: string; nickname: string; super: boolean; roles: Array<{ code: string; name: string }> }

// 拉取失败时的静态兜底（首页不占菜单；仅保证基本可用）。
const FALLBACK: AdminMenu[] = [
  { key: 'content.list', label: '内容列表', path: '/contents', module: 'content', group: 'content', group_label: '内容管理', group_order: 2, order: 1 },
  { key: 'content.reviews', label: '审核队列', path: '/reviews', module: 'trust', group: 'content', group_label: '内容管理', group_order: 2, order: 2 },
  { key: 'content.certs', label: '认证审核', path: '/verify-certs', module: 'verify', group: 'content', group_label: '内容管理', group_order: 2, order: 3 },
  { key: 'trade.orders', label: '订单管理', path: '/orders', module: 'trade', group: 'trade', group_label: '交易与会员', group_order: 3, order: 1 },
  { key: 'trade.members', label: '会员管理', path: '/members', module: 'membership', group: 'trade', group_label: '交易与会员', group_order: 3, order: 2 },
  { key: 'collab.cubes', label: '猫立方管理', path: '/cubes', module: 'cube', group: 'collab', group_label: '圈子与立方', group_order: 4, order: 1 },
  { key: 'collab.circles', label: '圈子管理', path: '/circles', module: 'circle', group: 'collab', group_label: '圈子与立方', group_order: 4, order: 2 },
  { key: 'operation.channel', label: '运营商', path: '/channel-operators', module: 'channel', group: 'operation', group_label: '运营管理', group_order: 5, order: 1 },
  { key: 'operation.settlements', label: '渠道结算', path: '/channel-settlements', module: 'channel', group: 'operation', group_label: '运营管理', group_order: 5, order: 2 },
  { key: 'system.users', label: '用户管理', path: '/users', module: 'identity', group: 'system', group_label: '系统管理', group_order: 6, order: 1 },
  { key: 'system.roles', label: '角色管理', path: '/roles', module: 'ops', group: 'system', group_label: '系统管理', group_order: 6, order: 2 },
  { key: 'system.settings', label: '系统设置', path: '/system-settings', module: 'ops', group: 'system', group_label: '系统管理', group_order: 6, order: 3 },
  { key: 'system.audits', label: '日志审计', path: '/audits', module: 'ops', group: 'system', group_label: '系统管理', group_order: 6, order: 4 },
  { key: 'system.flags', label: '特性开关', path: '/feature-flags', module: 'ops', group: 'system', group_label: '系统管理', group_order: 6, order: 5 },
  { key: 'system.dead', label: '死信队列', path: '/dead-letters', module: 'ops', group: 'system', group_label: '系统管理', group_order: 6, order: 6 },
  { key: 'system.notice', label: '通知模板', path: '/templates', module: 'notice', group: 'system', group_label: '系统管理', group_order: 6, order: 7 },
]

function groupsFromItems(items: AdminMenu[]): GroupInfo[] {
  const m = new Map<string, GroupInfo>()
  for (const it of items) {
    if (!it.group) continue
    if (!m.has(it.group)) m.set(it.group, { key: it.group, label: it.group_label || it.group, order: it.group_order ?? 99 })
  }
  return [...m.values()].sort((a, b) => a.order - b.order)
}

function matchIn(items: AdminMenu[], pathname: string): AdminMenu | undefined {
  return items.find((i) => !i.path.startsWith('/m/') && (pathname === i.path || pathname.startsWith(`${i.path}/`)))
}

function groupForPath(items: AdminMenu[], pathname: string): GroupInfo | undefined {
  const hit = matchIn(items, pathname)
  if (!hit?.group) return undefined
  return { key: hit.group, label: hit.group_label || hit.group, order: hit.group_order ?? 99 }
}

function firstLeaf(items: AdminMenu[]): AdminMenu | undefined {
  const byParent = new Map<string, AdminMenu[]>()
  for (const it of items) {
    const p = it.parent_key || ''
    byParent.set(p, [...(byParent.get(p) ?? []), it])
  }
  const walk = (parent: string): AdminMenu | undefined => {
    for (const it of byParent.get(parent) ?? []) {
      if (it.path.startsWith('/m/')) {
        const child = walk(it.key)
        if (child) return child
      } else {
        return it
      }
    }
    return undefined
  }
  return walk('')
}

function treeOf(items: AdminMenu[]): MenuProps['items'] {
  const byParent = new Map<string, AdminMenu[]>()
  for (const it of items) {
    const p = it.parent_key || ''
    byParent.set(p, [...(byParent.get(p) ?? []), it])
  }
  const node = (it: AdminMenu): NonNullable<MenuProps['items']>[number] => {
    const children = byParent.get(it.key) ?? []
    if (children.length > 0) {
      return { key: it.key, label: it.label, children: children.map(node) }
    }
    return { key: it.path, label: it.label }
  }
  return (byParent.get('') ?? []).map(node)
}

// 后台壳：顶部一级菜单 + 左侧按分组动态获取的二/三级菜单树（三级默认折叠）。
export default function AdminLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const { token } = theme.useToken()
  const [allItems, setAllItems] = useState<AdminMenu[]>(FALLBACK)
  const [topGroups, setTopGroups] = useState<GroupInfo[]>(groupsFromItems(FALLBACK))
  const [groupItems, setGroupItems] = useState<AdminMenu[]>([])
  const [activeGroup, setActiveGroup] = useState('')
  const [openGroups, setOpenGroups] = useState<string[]>([])
  const [me, setMe] = useState<Me | null>(null)
  const [infoOpen, setInfoOpen] = useState(false)
  const [aboutOpen, setAboutOpen] = useState(false)

  const loadGroup = useCallback(
    async (key: string, goFirst: boolean) => {
      try {
        const res = await adminApi.menus(key)
        const items = res.items ?? []
        setGroupItems(items)
        setActiveGroup(key)
        const leaf = firstLeaf(items)
        setOpenGroups(leaf?.parent_key ? [leaf.parent_key] : [])
        if (goFirst && leaf) navigate(leaf.path)
      } catch {
        /* 保持当前菜单 */
      }
    },
    [navigate],
  )

  useEffect(() => {
    void (async () => {
      try {
        const res = await adminApi.menus()
        const items = res.items ?? []
        if (items.length) {
          setAllItems(items)
          setTopGroups(res.groups?.length ? res.groups : groupsFromItems(items))
          const g = groupForPath(items, location.pathname)
          if (g) await loadGroup(g.key, false)
        }
      } catch {
        /* 回退静态菜单 */
      }
    })()
    void (async () => {
      try {
        setMe(await adminApi.me())
      } catch {
        /* 未登录/接口不可用 */
      }
    })()
    // 仅在首次挂载时初始化；后续路由变化由下面的 effect 处理。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const activeItem = useMemo(() => matchIn(groupItems, location.pathname), [groupItems, location.pathname])

  useEffect(() => {
    if (activeItem) {
      if (activeItem.parent_key) {
        setOpenGroups((prev) => (prev.includes(activeItem.parent_key!) ? prev : [...prev, activeItem.parent_key!]))
      }
      return
    }
    const g = groupForPath(allItems, location.pathname)
    if (g && g.key !== activeGroup) void loadGroup(g.key, false)
  }, [location.pathname, activeItem, allItems, activeGroup, loadGroup])

  const topItems: MenuProps['items'] = topGroups.map((g) => ({ key: g.key, label: g.label }))
  const treeItems = useMemo(() => treeOf(groupItems), [groupItems])

  const logout = () => {
    clearToken()
    navigate('/login', { replace: true })
  }

  const roleText = me?.super ? '超级管理员' : me?.roles?.map((r) => r.name).join(' / ') || '后台用户'
  const displayName = me?.nickname || (me ? `用户#${me.user_id}` : '未登录')

  const userMenu: MenuProps['items'] = [
    { key: 'info', label: '用户信息' },
    { key: 'about', label: '关于' },
    { type: 'divider' },
    { key: 'logout', label: '退出登录' },
  ]

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header
        style={{
          background: token.colorBgContainer,
          borderBottom: `1px solid ${token.colorBorderSecondary}`,
          display: 'flex',
          alignItems: 'center',
          paddingInline: 16,
          gap: 16,
        }}
      >
        <div
          onClick={() => navigate('/dashboard')}
          style={{ color: token.colorText, fontWeight: 600, whiteSpace: 'nowrap', cursor: 'pointer' }}
        >
          猫平方管理平台
        </div>
        <Menu
          theme="dark"
          mode="horizontal"
          selectedKeys={activeItem?.group ? [activeItem.group] : []}
          items={topItems}
          onClick={(e) => void loadGroup(String(e.key), true)}
          style={{ flex: 1, minWidth: 0, background: 'transparent', borderBottom: 'none' }}
        />
        <Dropdown
          trigger={['click']}
          menu={{
            items: userMenu,
            onClick: ({ key }) => {
              if (key === 'logout') logout()
              else if (key === 'info') setInfoOpen(true)
              else if (key === 'about') setAboutOpen(true)
            },
          }}
        >
          <Button type="text" style={{ color: token.colorText, paddingInline: 8 }}>
            <Space size={6}>
              <span>{displayName}</span>
              <Tag color={me?.super ? 'red' : 'blue'} style={{ marginInlineEnd: 0 }}>
                {roleText}
              </Tag>
            </Space>
          </Button>
        </Dropdown>
      </Header>
      <Layout>
        {activeItem ? (
          <Sider theme="dark" width={220}>
            <Menu
              theme="dark"
              mode="inline"
              selectedKeys={[activeItem.path]}
              openKeys={openGroups}
              onOpenChange={(keys) => setOpenGroups(keys.map(String))}
              items={treeItems}
              onClick={(e) => {
                if (String(e.key).startsWith('/')) navigate(e.key)
              }}
            />
          </Sider>
        ) : null}
        <Content style={{ margin: 16, overflow: 'auto' }}>
          <Outlet />
        </Content>
      </Layout>

      <Modal title="用户信息" open={infoOpen} onCancel={() => setInfoOpen(false)} footer={null}>
        <Descriptions column={1} size="small" bordered>
          <Descriptions.Item label="昵称">{me?.nickname || '—'}</Descriptions.Item>
          <Descriptions.Item label="用户 ID">{me?.user_id || '—'}</Descriptions.Item>
          <Descriptions.Item label="角色">
            {me?.super ? <Tag color="red">超级管理员</Tag> : null}
            {me?.roles?.length ? me.roles.map((r) => <Tag key={r.code}>{r.name}</Tag>) : me?.super ? null : '—'}
          </Descriptions.Item>
          <Descriptions.Item label="权限说明">
            {me?.super ? '隐含全部权限' : '按角色权限点与数据范围生效'}
          </Descriptions.Item>
        </Descriptions>
      </Modal>

      <Modal title="关于" open={aboutOpen} onCancel={() => setAboutOpen(false)} footer={null}>
        <p style={{ marginTop: 0 }}>猫平方管理平台</p>
        <p>版本：v1.0.0</p>
        <p>© 2026 猫平方</p>
      </Modal>
    </Layout>
  )
}
