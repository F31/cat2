import { useCallback, useEffect, useState } from 'react'
import { Button, Select, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminMember } from '../api/admin'

const LEVEL: Record<number, string> = { 1: 'VIP', 2: 'SVIP', 3: '企业版' }
const STATUS: Record<number, { text: string; color: string }> = {
  1: { text: '生效中', color: 'green' },
  2: { text: '已过期', color: 'default' },
}

// 会员列表（只读）。
export default function Members() {
  const [rows, setRows] = useState<AdminMember[]>([])
  const [loading, setLoading] = useState(false)
  const [level, setLevel] = useState<number | undefined>()
  const [status, setStatus] = useState<number | undefined>()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.members({ level, status })
      setRows(res.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [level, status])

  useEffect(() => {
    void load()
  }, [load])

  const columns: ColumnsType<AdminMember> = [
    { title: '用户ID', dataIndex: 'user_id' },
    { title: '等级', dataIndex: 'level', render: (l: number) => LEVEL[l] ?? l },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: number) => <Tag color={STATUS[s]?.color}>{STATUS[s]?.text ?? s}</Tag>,
    },
    {
      title: '开始时间',
      dataIndex: 'started_at',
      render: (t: string) => (t ? t.slice(0, 10) : ''),
    },
    {
      title: '到期时间',
      dataIndex: 'expires_at',
      render: (t: string) => (t ? t.slice(0, 10) : ''),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Select
          allowClear
          placeholder="等级"
          style={{ width: 160 }}
          value={level}
          onChange={setLevel}
          options={[
            { value: 1, label: 'VIP' },
            { value: 3, label: '企业版' },
          ]}
        />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 160 }}
          value={status}
          onChange={setStatus}
          options={[
            { value: 1, label: '生效中' },
            { value: 2, label: '已过期' },
          ]}
        />
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      <Table rowKey="user_id" loading={loading} columns={columns} dataSource={rows} size="small" />
    </>
  )
}
