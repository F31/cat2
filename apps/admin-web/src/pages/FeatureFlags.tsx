import { useCallback, useEffect, useState } from 'react'
import { Button, Form, Input, InputNumber, Modal, Select, Space, Switch, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type FeatureFlag } from '../api/admin'

// 特性开关（灰度发布）：白名单/比例放量；客户端按用户评估（GET /mp/v1/flags）。
type UserRow = { id: string; nickname: string; phone_masked?: string; status: number }

export default function FeatureFlags() {
  const [rows, setRows] = useState<FeatureFlag[]>([])
  const [known, setKnown] = useState<Array<{ key: string; label: string; description?: string }>>([])
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<FeatureFlag | null>(null)
  const [selectedUsers, setSelectedUsers] = useState<string[]>([])
  const [pickerOpen, setPickerOpen] = useState(false)
  const [pickerRows, setPickerRows] = useState<UserRow[]>([])
  const [pickerLoading, setPickerLoading] = useState(false)
  const [form] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.featureFlags()
      setRows(res.items ?? [])
      setKnown(res.known ?? [])
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
    setSelectedUsers((cur.users ?? []).map(String))
    form.setFieldsValue({ key: cur.key, ratio: cur.ratio })
  }

  const loadUsers = useCallback(async (q?: string) => {
    setPickerLoading(true)
    try {
      const res = await adminApi.governanceUsers({ q, limit: 50 })
      setPickerRows((res.items ?? []) as UserRow[])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载用户失败')
    } finally {
      setPickerLoading(false)
    }
  }, [])

  const openPicker = () => {
    setPickerOpen(true)
    void loadUsers()
  }

  const save = async () => {
    const v = await form.validateFields()
    try {
      await adminApi.setFeatureFlag(v.key, editing?.enabled ?? false, Number(v.ratio) || 0, selectedUsers)
      message.success('已保存')
      setEditing(null)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  const toggle = async (f: FeatureFlag, enabled: boolean) => {
    try {
      await adminApi.setFeatureFlag(f.key, enabled, f.ratio ?? 0, (f.users ?? []).map(String))
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
      render: (us?: string[]) => (us?.length ? <Tag>{us.length} 人</Tag> : '—'),
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
          <Form.Item name="key" label="开关" rules={[{ required: true, message: '请选择开关' }]}>
            <Select
              placeholder="选择开关键"
              disabled={!!editing?.key}
              showSearch
              optionFilterProp="label"
              options={[
                ...known.map((k) => ({ value: k.key, label: `${k.label}（${k.key}）` })),
                ...(editing?.key && !known.some((k) => k.key === editing.key)
                  ? [{ value: editing.key, label: editing.key }]
                  : []),
              ]}
            />
          </Form.Item>
          <Form.Item name="ratio" label="比例放量（0-100，按用户稳定哈希）">
            <InputNumber min={0} max={100} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label="白名单用户" extra="命中即开；支持多选">
            <Space>
              <Input readOnly value={selectedUsers.length ? `已选 ${selectedUsers.length} 人` : '未选择'} style={{ width: 180 }} />
              <Button onClick={openPicker}>选择用户</Button>
              {selectedUsers.length ? <Button onClick={() => setSelectedUsers([])}>清空</Button> : null}
            </Space>
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="选择白名单用户"
        open={pickerOpen}
        onOk={() => setPickerOpen(false)}
        onCancel={() => setPickerOpen(false)}
        okText={`确定（已选 ${selectedUsers.length} 人）`}
        width={720}
      >
        <Input.Search placeholder="昵称 / 手机号" allowClear onSearch={(v) => void loadUsers(v)} style={{ marginBottom: 12 }} />
        <Table
          rowKey="id"
          size="small"
          loading={pickerLoading}
          dataSource={pickerRows}
          pagination={{ pageSize: 8 }}
          rowSelection={{
            selectedRowKeys: selectedUsers,
            onChange: (keys) => setSelectedUsers(keys.map(String)),
          }}
          columns={[
            { title: '用户ID', dataIndex: 'id' },
            { title: '昵称', dataIndex: 'nickname' },
            { title: '手机号', dataIndex: 'phone_masked', render: (v?: string) => v || '—' },
            { title: '状态', dataIndex: 'status', render: (v: number) => (v === 0 ? '正常' : v === 2 ? '封禁' : '限制') },
          ]}
        />
      </Modal>
    </>
  )
}
