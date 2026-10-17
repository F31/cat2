import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Modal, Select, Space, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminCircle } from '../api/admin'
import DataTable from '../components/DataTable'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '进行中', color: 'green' },
  1: { text: '已结束', color: 'default' },
  2: { text: '已解散', color: 'red' },
  3: { text: '已封禁', color: 'volcano' },
  4: { text: '已归档', color: 'purple' },
}

type Action = 'ban' | 'archive' | 'unban' | 'restore'
const ACTION_LABEL: Record<Action, string> = { ban: '批量封禁', archive: '批量归档', unban: '批量解封', restore: '批量恢复' }

// 圈子管理（p4 §8.1 + PRD §6.5/§11.4）：列表 + 批量封禁/归档/解封/恢复（必填原因，逐圈审计）。
export default function Circles() {
  const [rows, setRows] = useState<AdminCircle[]>([])
  const [loading, setLoading] = useState(false)
  const [q, setQ] = useState('')
  const [status, setStatus] = useState<number | undefined>()
  const [selected, setSelected] = useState<string[]>([])
  const [action, setAction] = useState<Action | null>(null)
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.circles(q, status)
      setRows(res.items ?? [])
      setSelected([])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [q, status])

  useEffect(() => {
    void load()
  }, [load])

  const submit = async () => {
    if (!action) return
    if (!reason.trim()) {
      message.warning('必须填写原因')
      return
    }
    try {
      const r = await adminApi.batchCircleStatus(action, selected, reason)
      if (r.failed > 0) {
        const fails = (r.items ?? []).filter((i) => !i.ok).map((i) => `${i.id}:${i.error ?? '失败'}`).slice(0, 3)
        message.warning(`成功 ${r.succeeded}，失败 ${r.failed}（${fails.join('；')}）`)
      } else {
        message.success(`已处理 ${r.succeeded} 个圈子`)
      }
      setAction(null)
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
  ]

  return (
    <>
      <Space wrap style={{ marginBottom: 16 }}>
        <Input.Search placeholder="名称 / ID" style={{ width: 220 }} allowClear onSearch={setQ} />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 140 }}
          value={status}
          onChange={setStatus}
          options={Object.entries(STATUS).map(([v, s]) => ({ value: Number(v), label: s.text }))}
        />
        <Button onClick={() => void load()}>刷新</Button>
        <Button danger disabled={selected.length === 0} onClick={() => setAction('ban')}>
          批量封禁
        </Button>
        <Button disabled={selected.length === 0} onClick={() => setAction('archive')}>
          批量归档
        </Button>
        <Button disabled={selected.length === 0} onClick={() => setAction('unban')}>
          批量解封
        </Button>
        <Button disabled={selected.length === 0} onClick={() => setAction('restore')}>
          批量恢复
        </Button>
        {selected.length > 0 ? <Tag color="blue">已选 {selected.length}</Tag> : null}
      </Space>
      <DataTable
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={rows}
        rowSelection={{ selectedRowKeys: selected, onChange: (keys) => setSelected(keys as string[]) }}
      />
      <Modal
        title={`${action ? ACTION_LABEL[action] : ''}（${selected.length} 个）`}
        open={!!action}
        onOk={() => void submit()}
        onCancel={() => setAction(null)}
        okText="确认"
        okButtonProps={{ danger: action === 'ban' }}
      >
        <p style={{ color: '#888' }}>
          封禁：前台隐藏、禁止加入/发布，可解封；归档：后台隐藏、只读，可恢复；原因逐圈写入审计。
        </p>
        <Input.TextArea rows={3} placeholder="原因（必填）" value={reason} onChange={(e) => setReason(e.target.value)} />
      </Modal>
    </>
  )
}
