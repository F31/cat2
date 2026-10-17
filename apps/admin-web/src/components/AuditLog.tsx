import { useCallback, useEffect, useState } from 'react'
import { Button, Descriptions, Drawer, Input, InputNumber, Select, Space, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AuditEntry } from '../api/admin'
import DataTable from './DataTable'

const RESULT: Record<number, { text: string; color: string }> = {
  0: { text: '成功', color: 'green' },
  1: { text: '拒绝', color: 'red' },
  2: { text: '失败', color: 'orange' },
}
const CHANNEL: Record<string, string> = { admin: '后台', mp: '小程序', jobs: '任务', open: '开放' }

/** 审计轨迹共享组件（五维筛选 + target_ref 反查 + detail 快照）。 */
export default function AuditLog() {
  const [rows, setRows] = useState<AuditEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [filters, setFilters] = useState<{ actor_id?: string; action?: string; result?: string; target_ref?: string; limit: number }>({
    limit: 50,
  })
  const [detail, setDetail] = useState<AuditEntry | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.audits(filters)
      setRows(res.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [filters])

  useEffect(() => {
    void load()
  }, [load])

  const open = async (id: number) => {
    try {
      setDetail(await adminApi.auditDetail(id))
    } catch (e) {
      message.error(e instanceof Error ? e.message : '详情加载失败')
    }
  }

  const columns: ColumnsType<AuditEntry> = [
    { title: '时间', dataIndex: 'created_at', render: (v: number) => new Date(v * 1000).toLocaleString() },
    { title: '操作者', dataIndex: 'actor_id', render: (v: number, r) => `${CHANNEL[r.channel ?? ''] ?? r.channel}#${v}` },
    { title: '动作', dataIndex: 'action' },
    { title: '对象', dataIndex: 'target_ref', render: (v?: string) => (v ? <code>{v}</code> : '—') },
    {
      title: '结果',
      dataIndex: 'result',
      render: (v: number) => {
        const r = RESULT[v] ?? { text: String(v), color: 'default' }
        return <Tag color={r.color}>{r.text}</Tag>
      },
    },
    { title: '请求', dataIndex: 'request_id', render: (v?: string) => v || '—' },
    { title: '归档', dataIndex: 'archived', render: (v?: boolean) => (v ? <Tag>已归档</Tag> : '—') },
    {
      title: '操作',
      render: (_, r) => (
        <Button size="small" onClick={() => void open(r.id)}>
          详情
        </Button>
      ),
    },
  ]

  return (
    <>
      <Space wrap style={{ marginBottom: 16 }}>
        <Input placeholder="操作者 ID" style={{ width: 140 }} onChange={(e) => setFilters((f) => ({ ...f, actor_id: e.target.value || undefined }))} />
        <Input
          placeholder="动作（如 trade:refund:execute）"
          style={{ width: 240 }}
          onChange={(e) => setFilters((f) => ({ ...f, action: e.target.value || undefined }))}
        />
        <Select
          placeholder="结果"
          style={{ width: 120 }}
          allowClear
          options={[
            { value: '0', label: '成功' },
            { value: '1', label: '拒绝' },
            { value: '2', label: '失败' },
          ]}
          onChange={(v) => setFilters((f) => ({ ...f, result: v || undefined }))}
        />
        <Input placeholder="对象反查（如 order:123）" style={{ width: 200 }} onChange={(e) => setFilters((f) => ({ ...f, target_ref: e.target.value || undefined }))} />
        <InputNumber min={1} max={200} value={filters.limit} onChange={(v) => setFilters((f) => ({ ...f, limit: Number(v) || 50 }))} />
        <Button type="primary" onClick={() => void load()}>
          查询
        </Button>
      </Space>
      <DataTable rowKey="id" loading={loading} columns={columns} dataSource={rows} />
      <Drawer title={`审计详情 #${detail?.id ?? ''}`} width={560} open={!!detail} onClose={() => setDetail(null)}>
        {detail ? (
          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label="动作">{detail.action}</Descriptions.Item>
            <Descriptions.Item label="结果">{RESULT[detail.result]?.text ?? detail.result}</Descriptions.Item>
            <Descriptions.Item label="对象">{detail.target_ref || '—'}</Descriptions.Item>
            <Descriptions.Item label="IP / 请求">{`${detail.ip || '—'} / ${detail.request_id || '—'}`}</Descriptions.Item>
            <Descriptions.Item label="哈希">{detail.hash}</Descriptions.Item>
            <Descriptions.Item label="前序哈希">{detail.prev_hash || '—'}</Descriptions.Item>
            <Descriptions.Item label="快照（已脱敏）">
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{JSON.stringify(detail.detail ?? {}, null, 2)}</pre>
            </Descriptions.Item>
          </Descriptions>
        ) : null}
      </Drawer>
    </>
  )
}
