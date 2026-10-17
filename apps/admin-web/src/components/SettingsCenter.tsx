import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Card, Space, Tabs, message } from 'antd'
import { adminApi, type SettingItem } from '../api/admin'
import SettingsForm from './SettingsForm'

// 分组：安全类细分「登录安全 / 密码策略」；其余按配置键首段自动分组（新增键无需改前端）。
const GROUP_LABEL: Record<string, string> = {
  sms: '短信网关',
  ai: 'AI 网关',
  policy: '复核策略',
  channel: '渠道与佣金',
  membership: '会员价格',
  dsc: '匹配参数',
  audit: '审计与告警',
  msg: '消息留存',
}
const GROUP_ORDER = ['登录安全', '密码策略', '短信网关', 'AI 网关', '复核策略', '渠道与佣金', '会员价格', '匹配参数', '审计与告警', '消息留存']

function groupOf(key: string): string {
  if (key.startsWith('admin.security.password.')) return '密码策略'
  if (key.startsWith('admin.security.')) return '登录安全'
  const head = key.split('.')[0]
  return GROUP_LABEL[head] ?? head
}

/** 配置中心：完全由 SettingsSchema 驱动（§8.2）；新增配置项不写前端代码。 */
export default function SettingsCenter() {
  const [items, setItems] = useState<SettingItem[]>([])
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.settings()
      setItems(res.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const groups = useMemo(() => {
    const m = new Map<string, SettingItem[]>()
    for (const it of items) {
      const g = groupOf(it.key)
      m.set(g, [...(m.get(g) ?? []), it])
    }
    const ordered = [...m.keys()].sort((a, b) => {
      const ia = GROUP_ORDER.indexOf(a)
      const ib = GROUP_ORDER.indexOf(b)
      return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib)
    })
    return ordered.map((g) => ({ key: g, label: g, items: m.get(g) ?? [] }))
  }, [items])

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void load()} loading={loading}>
          刷新
        </Button>
      </Space>
      <Tabs
        items={groups.map((g) => ({
          key: g.key,
          label: g.label,
          children: (
            <Card>
              <SettingsForm items={g.items} onSaved={() => void load()} />
            </Card>
          ),
        }))}
      />
    </>
  )
}
