import { useCallback, useEffect, useState } from 'react'
import { Button, Select, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminOrder } from '../api/admin'
import { fenToYuan } from '@catcube/utils'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '待支付', color: 'orange' },
  1: { text: '已支付', color: 'green' },
  2: { text: '已关闭', color: 'default' },
}

// 订单列表（只读；退款/履约补偿已有 admin 接口，页面后续接入）。
export default function Orders() {
  const [rows, setRows] = useState<AdminOrder[]>([])
  const [loading, setLoading] = useState(false)
  const [status, setStatus] = useState<number | undefined>()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.orders({ status })
      setRows(res.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [status])

  useEffect(() => {
    void load()
  }, [load])

  const columns: ColumnsType<AdminOrder> = [
    { title: '订单号', dataIndex: 'id' },
    { title: '业务类型', dataIndex: 'biz_type' },
    { title: '金额（元）', dataIndex: 'amount_fen', render: (f: number) => fenToYuan(f) },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: number) => <Tag color={STATUS[s]?.color}>{STATUS[s]?.text ?? s}</Tag>,
    },
    {
      title: '支付截止',
      dataIndex: 'pay_expire_at',
      render: (t: string) => (t ? t.slice(0, 19).replace('T', ' ') : ''),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 160 }}
          value={status}
          onChange={setStatus}
          options={[
            { value: 0, label: '待支付' },
            { value: 1, label: '已支付' },
            { value: 2, label: '已关闭' },
          ]}
        />
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      <Table rowKey="id" loading={loading} columns={columns} dataSource={rows} size="small" />
    </>
  )
}
