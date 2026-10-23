import { useCallback, useEffect, useState } from 'react'
import { Button, DatePicker, InputNumber, message, Select, Space, Table, Tag } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi } from '../api/admin'
import { getToken } from '../auth'

interface ExportTask {
  id: string
  template: string
  status: number
  rows: number
  error?: string
  created_at?: string
  expires_at?: string
}

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '待生成', color: 'default' },
  1: { text: '生成中', color: 'processing' },
  2: { text: '完成', color: 'green' },
  3: { text: '失败', color: 'red' },
}

const TEMPLATES = [
  { value: 'reconciliation', label: '对账导出（订单/支付/履约）' },
  { value: 'orders', label: '订单明细' },
  { value: 'users', label: '用户列表（脱敏）' },
  { value: 'reviews', label: '审核记录' },
  { value: 'settlements', label: '结算明细（渠道开通后启用）' },
]

// 导出中心（p4 §7.4）：申请 → 异步生成 → 下载（带水印 + 导出审计）。
export default function Exports() {
  const [rows, setRows] = useState<ExportTask[]>([])
  const [loading, setLoading] = useState(false)
  const [template, setTemplate] = useState('reconciliation')
  const [days, setDays] = useState(30)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.exports()
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

  const create = async () => {
    const to = Math.floor(Date.now() / 1000)
    const from = to - days * 86400
    try {
      await adminApi.createExport(template, from, to)
      message.success('已提交导出（稍后刷新）')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '申请失败')
    }
  }

  const download = async (id: string) => {
    try {
      const res = await adminApi.downloadExport(id)
      if (!res.url.startsWith('/api/')) {
        window.open(res.url, '_blank')
        return
      }
      const token = getToken()
      const r = await fetch(res.url, { headers: token ? { Authorization: `Bearer ${token}` } : {} })
      if (!r.ok) throw new Error('下载失败')
      const blob = await r.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `export-${id}.csv`
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      message.error(e instanceof Error ? e.message : '下载失败')
    }
  }

  const columns: ColumnsType<ExportTask> = [
    { title: 'ID', dataIndex: 'id' },
    { title: '模板', dataIndex: 'template' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: number) => {
        const s = STATUS[v] ?? { text: String(v), color: 'default' }
        return <Tag color={s.color}>{s.text}</Tag>
      },
    },
    { title: '行数', dataIndex: 'rows' },
    { title: '错误', dataIndex: 'error', render: (v?: string) => v || '—' },
    {
      title: '操作',
      render: (_, r) => (
        <Button size="small" disabled={r.status !== 2} onClick={() => void download(r.id)}>
          下载
        </Button>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Select style={{ width: 260 }} value={template} onChange={setTemplate} options={TEMPLATES} />
        <InputNumber min={1} max={365} value={days} onChange={(v) => setDays(Number(v) || 30)} addonAfter="天" />
        <Button type="primary" onClick={() => void create()}>
          申请导出
        </Button>
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      <Table rowKey="id" loading={loading} columns={columns} dataSource={rows} size="small" />
      <DatePicker style={{ display: 'none' }} />
    </>
  )
}
