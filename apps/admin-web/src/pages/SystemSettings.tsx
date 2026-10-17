import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Form, Input, InputNumber, Space, Switch, Tabs, message } from 'antd'
import { adminApi, type SettingItem } from '../api/admin'

// 系统设置（p4 §3.4）：安全相关配置集中于此，Tab 分组；超管/运营管理员可改，热生效。
const TABS: Array<{ key: string; label: string; prefixes: string[]; hint: string }> = [
  {
    key: 'login',
    label: '登录安全',
    prefixes: ['admin.security.'],
    hint: '登录失败达阈值触发短信二次验证；IP 白名单留空表示不限制（支持前缀，如 10.0.0.）。',
  },
  { key: 'sms', label: '短信网关', prefixes: ['sms.gateway.'], hint: '短信通道（验证码/通知）；dev=仅日志模拟。' },
  { key: 'ai', label: 'AI 网关', prefixes: ['ai.gateway.'], hint: '生成式 AI 独立网关（安全围栏，ADR-003）。' },
  {
    key: 'policy',
    label: '复核策略',
    prefixes: ['policy.'],
    hint: '强制双人复核（复核≠执行）的操作清单，逗号分隔；off=关闭（资金类不建议关闭）。',
  },
  {
    key: 'channel',
    label: '渠道与佣金',
    prefixes: ['channel.'],
    hint: '计佣范围/比例（万分比）与代理费/保证金金额；运行期动态可配、计佣写快照。',
  },
]

function typeOf(key: string) {
  if (key.endsWith('_enabled') || key.endsWith('enabled')) return 'bool'
  if (key.includes('threshold') || key.includes('len') || key.endsWith('_fen') || key.endsWith('_bp')) return 'int'
  return 'string'
}

// 密码策略并入登录安全之外单列（key 前缀同 admin.security.password）
const PASSWORD_TAB = { key: 'password', label: '密码策略', prefixes: ['admin.security.password.'] }

export default function SystemSettings() {
  const [items, setItems] = useState<SettingItem[]>([])
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.settings()
      setItems(res.items ?? [])
      const initial: Record<string, unknown> = {}
      for (const it of res.items ?? []) {
        initial[it.key] = typeOf(it.key) === 'bool' ? it.value === 'true' : it.value
      }
      form.setFieldsValue(initial)
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [form])

  useEffect(() => {
    void load()
  }, [load])

  const save = async (prefixes: string[]) => {
    const values = form.getFieldsValue()
    const targets = items.filter((it) => prefixes.some((p) => it.key.startsWith(p)))
    try {
      for (const it of targets) {
        const raw = values[it.key]
        const value = typeof raw === 'boolean' ? String(raw) : String(raw ?? '')
        if (value !== it.value) {
          await adminApi.setSetting(it.key, value)
        }
      }
      message.success('已保存（热生效）')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  const renderTab = (prefixes: string[], hint: string) => {
    const list = items.filter((it) => prefixes.some((p) => it.key.startsWith(p)))
    return (
      <Card>
        <p style={{ color: '#888', marginTop: 0 }}>{hint}</p>
        <Form form={form} layout="vertical" disabled={loading}>
          {list.map((it) => (
            <Form.Item key={it.key} name={it.key} label={`${it.description ?? it.key}（${it.key}）`}>
              {typeOf(it.key) === 'bool' ? (
                <Switch />
              ) : typeOf(it.key) === 'int' ? (
                <InputNumber style={{ width: 240 }} min={0} />
              ) : (
                <Input style={{ maxWidth: 480 }} placeholder={it.default ?? ''} />
              )}
            </Form.Item>
          ))}
          <Space>
            <Button type="primary" onClick={() => void save(prefixes)}>
              保存
            </Button>
            <Button onClick={() => void load()}>重置</Button>
          </Space>
        </Form>
      </Card>
    )
  }

  return (
    <Tabs
      items={[
        ...TABS.map((t) => ({ key: t.key, label: t.label, children: renderTab(t.prefixes, t.hint) })),
        { key: PASSWORD_TAB.key, label: PASSWORD_TAB.label, children: renderTab(PASSWORD_TAB.prefixes, '独立后台账号启用后生效（预留）。') },
      ]}
    />
  )
}
