import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Modal, Space, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminCube } from '../api/admin'
import DataTable from '../components/DataTable'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '正常', color: 'green' },
  1: { text: '已封禁', color: 'red' },
  2: { text: '已解散', color: 'default' },
}
const PLAN: Record<number, string> = { 0: '未订阅', 1: '基础版', 2: '专业版', 3: '旗舰版' }

// 猫立方管理（p4 §8.1）：列表 + 平台封禁/解散（必须给原因，服务端写审计）。
export default function Cubes() {
  const [rows, setRows] = useState<AdminCube[]>([])
  const [loading, setLoading] = useState(false)
  const [q, setQ] = useState('')
  const [action, setAction] = useState<{ cube: AdminCube; kind: 'ban' | 'dissolve' } | null>(null)
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.cubes(q)
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
    if (!action) return
    if (!reason.trim()) {
      message.warning('必须填写原因')
      return
    }
    try {
      await adminApi.adminCubeStatus(action.cube.id, action.kind, reason)
      message.success(action.kind === 'ban' ? '已封禁' : '已解散')
      setAction(null)
      setReason('')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败')
    }
  }

  const columns: ColumnsType<AdminCube> = [
    { title: '立方ID', dataIndex: 'id' },
    { title: '名称', dataIndex: 'name' },
    { title: '猫王', dataIndex: 'leader_id' },
    { title: '猫蜜', dataIndex: 'honey_id', render: (v?: string) => v || '—' },
    { title: '订阅档位', dataIndex: 'plan_level', render: (v: number) => PLAN[v] ?? v },
    { title: '成员数', dataIndex: 'member_count' },
    { title: '区域', dataIndex: 'region_code', render: (v?: string) => v || '—' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: number) => <Tag color={STATUS[v]?.color}>{STATUS[v]?.text ?? v}</Tag>,
    },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" danger disabled={r.status !== 0} onClick={() => setAction({ cube: r, kind: 'ban' })}>
            封禁
          </Button>
          <Button size="small" danger disabled={r.status === 2} onClick={() => setAction({ cube: r, kind: 'dissolve' })}>
            解散
          </Button>
        </Space>
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
        title={action?.kind === 'ban' ? `封禁立方 ${action?.cube.id}` : `解散立方 ${action?.cube.id}`}
        open={!!action}
        onOk={() => void confirm()}
        onCancel={() => setAction(null)}
        okText="确认"
        okButtonProps={{ danger: true }}
      >
        <Input.TextArea rows={3} placeholder="原因（必填，写入审计）" value={reason} onChange={(e) => setReason(e.target.value)} />
      </Modal>
    </>
  )
}
