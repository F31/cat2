import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type SettingItem } from '../api/admin'

// 业务配置（配置三层第三层）：后台改，热生效；敏感项不回显明文。
export default function Settings() {
  const [rows, setRows] = useState<SettingItem[]>([])
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<Record<string, string>>({})

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.settings()
      setRows(res.items ?? [])
      setEditing(Object.fromEntries((res.items ?? []).map((r) => [r.key, r.value ?? ''])))
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const save = async (key: string) => {
    try {
      await adminApi.setSetting(key, editing[key] ?? '')
      message.success('已保存（热生效）')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  const columns: ColumnsType<SettingItem> = [
    { title: '配置键', dataIndex: 'key' },
    { title: '类型', dataIndex: 'type', render: (v: string) => <Tag>{v}</Tag> },
    { title: '说明', dataIndex: 'description' },
    { title: '默认值', dataIndex: 'default', render: (v?: string) => (v ? <code>{v}</code> : '—') },
    {
      title: '当前值',
      dataIndex: 'value',
      render: (_: string, r) =>
        r.sensitive ? <Tag>已脱敏</Tag> : (
          <Input
            size="small"
            value={editing[r.key] ?? ''}
            onChange={(e) => setEditing((s) => ({ ...s, [r.key]: e.target.value }))}
          />
        ),
    },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" type="primary" onClick={() => void save(r.key)}>
            保存
          </Button>
          <Button size="small" onClick={() => setEditing((s) => ({ ...s, [r.key]: r.default ?? '' }))}>
            恢复默认
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      <Table rowKey="key" loading={loading} columns={columns} dataSource={rows} size="small" />
    </>
  )
}
