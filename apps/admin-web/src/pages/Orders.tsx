import { useCallback, useEffect, useState } from 'react'
import { Button, Input, InputNumber, Modal, Select, Space, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminOrder } from '../api/admin'
import DataTable from '../components/DataTable'
import { fenToYuan } from '@catcube/utils'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '待支付', color: 'orange' },
  1: { text: '已支付', color: 'green' },
  2: { text: '已关闭', color: 'default' },
}

// 订单管理（p4 §8.1 trade）：列表 + 退款执行 / 履约补偿重试（均写审计）。
export default function Orders() {
  const [rows, setRows] = useState<AdminOrder[]>([])
  const [loading, setLoading] = useState(false)
  const [status, setStatus] = useState<number | undefined>()
  const [refunding, setRefunding] = useState<AdminOrder | null>(null)
  const [amount, setAmount] = useState(0)
  const [reason, setReason] = useState('')

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

  const openRefund = (o: AdminOrder) => {
    setRefunding(o)
    setAmount(o.amount_fen)
    setReason('')
  }

  const confirmRefund = async () => {
    if (!refunding) return
    if (amount <= 0 || !reason.trim()) {
      message.warning('退款金额与原因必填')
      return
    }
    try {
      await adminApi.refund(refunding.id, amount, reason)
      message.success('退款已执行')
      setRefunding(null)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '退款失败')
    }
  }

  const retry = async (o: AdminOrder) => {
    try {
      await adminApi.retryFulfillment(o.id)
      message.success('履约补偿已重试')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '重试失败')
    }
  }

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
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" danger disabled={r.status !== 1} onClick={() => openRefund(r)}>
            退款
          </Button>
          <Button size="small" disabled={r.status !== 1} onClick={() => void retry(r)}>
            履约补偿
          </Button>
        </Space>
      ),
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
      <DataTable rowKey="id" loading={loading} columns={columns} dataSource={rows} />
      <Modal
        title={`退款（订单 ${refunding?.id ?? ''}）`}
        open={!!refunding}
        onOk={() => void confirmRefund()}
        onCancel={() => setRefunding(null)}
        okText="确认退款"
        okButtonProps={{ danger: true }}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <InputNumber
            style={{ width: '100%' }}
            min={1}
            max={refunding?.amount_fen}
            value={amount}
            onChange={(v) => setAmount(Number(v) || 0)}
            addonBefore="金额（分）"
          />
          <Input.TextArea rows={3} placeholder="退款原因（必填，写入审计）" value={reason} onChange={(e) => setReason(e.target.value)} />
        </Space>
      </Modal>
    </>
  )
}
