import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Modal, Space, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminCircle } from '../api/admin'
import DataTable from '../components/DataTable'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '进行中', color: 'green' },
  1: { text: '已结束', color: 'default' },
  2: { text: '已解散', color: 'red' },
}

// 圈子管理（p4 §8.1）：列表 + 平台解散（必须给原因，服务端写审计）。
export default function Circles() {
  const [rows, setRows] = useState<AdminCircle[]>([])
  const [loading, setLoading] = useState(false)
  const [q, setQ] = useState('')
  const [target, setTarget] = useState<AdminCircle | null>(null)
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.circles(q)
      setRows(res.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [q])

  useEffect(() => {
    void load()
  }, [load])

  const confirm = async () => {
    if (!target) return
    if (!reason.trim()) {
      message.warning('必须填写原因')
      return
    }
    try {
      await adminApi.adminDissolveCircle(target.id, reason)
      message.success('已解散')
      setTarget(null)
      setReason('')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败')
    }
  }

  const columns: ColumnsType<AdminCircle> = [
    { title: '圈子ID', dataIndex: 'id' },
    { title: '名称', dataIndex: 'name' },
    { title: '类型', dataIndex: 'type', render: (v: number) => (v === 1 ? '常设' : '临时') },
    { title: '圈主', dataIndex: 'owner_user_id' },
    { title: '成员数', dataIndex: 'member_count' },
    { title: '区域', dataIndex: 'region_code', render: (v?: string) => v || '—' },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      render: (v: number) => (v ? new Date(v * 1000).toLocaleString() : '—'),
    },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: number) => <Tag color={STATUS[v]?.color}>{STATUS[v]?.text ?? v}</Tag>,
    },
    {
      title: '操作',
      render: (_, r) => (
        <Button size="small" danger disabled={r.status === 2} onClick={() => setTarget(r)}>
          解散
        </Button>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Input.Search placeholder="名称 / ID" style={{ width: 240 }} allowClear onSearch={setQ} />
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      <DataTable rowKey="id" loading={loading} columns={columns} dataSource={rows} />
      <Modal
        title={`解散圈子 ${target?.id ?? ''}`}
        open={!!target}
        onOk={() => void confirm()}
        onCancel={() => setTarget(null)}
        okText="确认"
        okButtonProps={{ danger: true }}
      >
        <Input.TextArea rows={3} placeholder="原因（必填，写入审计）" value={reason} onChange={(e) => setReason(e.target.value)} />
      </Modal>
    </>
  )
}
