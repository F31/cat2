import { useMemo } from 'react'
import { Button, Form, Input, InputNumber, Space, Switch, message } from 'antd'
import { adminApi, type SettingItem } from '../api/admin'

const MASK = '******'

function isBool(it: SettingItem) {
  return it.type === 'bool'
}
function isNum(it: SettingItem) {
  return it.type === 'int' || it.type === 'fen' || it.type === 'rate'
}

/** 按 SettingsSchema 自动渲染一组配置（新增配置项无需改前端代码）。 */
export default function SettingsForm({ items, onSaved }: { items: SettingItem[]; onSaved: () => void }) {
  const [form] = Form.useForm()
  const initial = useMemo(() => {
    const m: Record<string, unknown> = {}
    for (const it of items) {
      m[it.key] = isBool(it) ? it.value === 'true' : it.value
    }
    return m
  }, [items])

  const save = async () => {
    const values = form.getFieldsValue()
    try {
      let changed = 0
      for (const it of items) {
        const raw = values[it.key]
        if (it.sensitive && raw === MASK) continue // 未修改的敏感项
        const value = typeof raw === 'boolean' ? String(raw) : String(raw ?? '')
        if (value !== it.value) {
          await adminApi.setSetting(it.key, value)
          changed++
        }
      }
      message.success(changed > 0 ? `已保存 ${changed} 项（热生效）` : '无变更')
      onSaved()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  return (
    <Form form={form} layout="vertical" initialValues={initial} key={items.map((i) => i.key).join(',')}>
      {items.map((it) => (
        <Form.Item key={it.key} name={it.key} label={`${it.description ?? it.key}（${it.key}）`}>
          {isBool(it) ? (
            <Switch />
          ) : isNum(it) ? (
            <InputNumber style={{ width: 240 }} min={0} />
          ) : it.sensitive ? (
            <Input.Password style={{ maxWidth: 480 }} placeholder={it.value === MASK ? '已设置（不回显）' : it.default} />
          ) : (
            <Input style={{ maxWidth: 480 }} placeholder={it.default ?? ''} />
          )}
        </Form.Item>
      ))}
      <Space>
        <Button type="primary" onClick={() => void save()}>
          保存
        </Button>
        <Button onClick={() => form.resetFields()}>重置</Button>
      </Space>
    </Form>
  )
}
