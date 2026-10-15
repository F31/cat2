import { useCallback, useEffect, useState } from 'react'
import { Button, Form, Input, InputNumber, Modal, Space, Switch, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type FeatureFlag } from '../api/admin'

// 特性开关（灰度发布）：白名单/比例放量；客户端按用户评估（GET /mp/v1/flags）。
export default function FeatureFlags() {
  const [rows, setRows] = useState<FeatureFlag[]>([])
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<FeatureFlag | null>(null)
  const [form] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.featureFlags()
      setRows(res.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const open = (f?: FeatureFlag) => {
    const cur = f ?? { key: '', enabled: false, ratio: 0, users: [] }
    setEditing(cur)
    form.setFieldsValue({ key: cur.key, ratio: cur.ratio, users: (cur.users ?? []).join(',') })
  }

  const save = async () => {
    const v = await form.validateFields()
    const users = String(v.users ?? '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
      .map(Number)
    try {
      await adminApi.setFeatureFlag(v.key, editing?.enabled ?? false, Number(v.ratio) || 0, users)
      message.success('已保存')
      setEditing(null)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  const toggle = async (f: FeatureFlag, enabled: boolean) => {
    try {
      await adminApi.setFeatureFlag(f.key, enabled, f.ratio ?? 0, f.users ?? [])
      message.success(enabled ? '已开启' : '已关闭')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败')
    }
  }

  const columns: ColumnsType<FeatureFlag> = [
    { title: '开关', dataIndex: 'key' },
    {
      title: '状态',
      dataIndex: 'enabled',
      render: (v: boolean, r) => <Switch checked={v} onChange={(x) => void toggle(r, x)} />,
    },
    { title: '比例放量', dataIndex: 'ratio', render: (v?: number) => (v ? `${v}%` : '—') },
    {
      title: '白名单',
      dataIndex: 'users',
      render: (us?: number[]) => (us?.length ? <Tag>{us.length} 人</Tag> : '—'),
    },
    {
      title: '操作',
      render: (_, r) => (
        <Button size="small" onClick={() => open(r)}>
          编辑
        </Button>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void load()}>刷新</Button>
        <Button type="primary" onClick={() => open()}>
          新建开关
        </Button>
      </Space>
      <Table rowKey="key" loading={loading} columns={columns} dataSource={rows} size="small" />
      <Modal title="特性开关" open={!!editing} onOk={() => void save()} onCancel={() => setEditing(null)} okText="保存">
        <Form form={form} layout="vertical">
          <Form.Item name="key" label="开关键名" rules={[{ required: true }]}>
            <Input placeholder="如 feed.new_ranking" disabled={!!editing && !!editing.key} />
          </Form.Item>
          <Form.Item name="ratio" label="比例放量（0-100，按用户稳定哈希）">
            <InputNumber min={0} max={100} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="users" label="白名单用户 ID（逗号分隔，命中即开）">
            <Input placeholder="1001,1002" />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
