import { useCallback, useEffect, useState } from 'react'
import { Button, Form, Input, InputNumber, Modal, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type ChannelFees, type ChannelOperatorRow } from '../api/admin'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '申请中', color: 'gold' },
  1: { text: '已开通', color: 'green' },
  2: { text: '已停用', color: 'red' },
}

function yuan(fen: number) {
  return (fen / 100).toFixed(2)
}

// 渠道运营商（p4 §1/§2.1）：列表、开通/停用、代理费/保证金台账。
export default function ChannelOperators() {
  const [rows, setRows] = useState<ChannelOperatorRow[]>([])
  const [loading, setLoading] = useState(false)
  const [fees, setFees] = useState<{ op: ChannelOperatorRow; data: ChannelFees } | null>(null)
  const [form] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.channelOperators()
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

  const openFees = async (op: ChannelOperatorRow) => {
    try {
      const data = await adminApi.channelOperatorFees(op.id)
      setFees({ op, data })
      form.setFieldsValue({
        agency_fee_fen: data.agency_fee_paid_fen,
        agency_fee_year: data.agency_fee_year,
        guarantee_fen: data.guarantee_fen,
      })
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载台账失败')
    }
  }

  const saveFees = async () => {
    if (!fees) return
    const v = await form.validateFields()
    try {
      await adminApi.channelRecordFees(fees.op.id, {
        agency_fee_fen: Number(v.agency_fee_fen) || 0,
        agency_fee_year: Number(v.agency_fee_year) || 0,
        guarantee_fen: Number(v.guarantee_fen) || 0,
      })
      message.success('已登记')
      void openFees(fees.op)
    } catch (e) {
      message.error(e instanceof Error ? e.message : '登记失败')
    }
  }

  const refund = async () => {
    if (!fees) return
    try {
      await adminApi.channelRefundGuarantee(fees.op.id, '退网')
      message.success('保证金已退款（台账）')
      void openFees(fees.op)
    } catch (e) {
      message.error(e instanceof Error ? e.message : '退款失败')
    }
  }

  const act = async (op: ChannelOperatorRow, action: 'open' | 'suspend') => {
    try {
      if (action === 'open') await adminApi.channelOpenOperator(op.id)
      else await adminApi.channelSuspendOperator(op.id)
      message.success('已更新')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败')
    }
  }

  const columns: ColumnsType<ChannelOperatorRow> = [
    { title: 'ID', dataIndex: 'id' },
    { title: '名称', dataIndex: 'name' },
    { title: '区域', render: (_, r) => `${r.region_name || ''}（${r.region_code}）` },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: number) => <Tag color={STATUS[v]?.color}>{STATUS[v]?.text ?? v}</Tag>,
    },
    { title: '推荐运营商', dataIndex: 'referrer_operator_id', render: (v: string) => (v && v !== '0' ? v : '—') },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          {r.status === 0 ? (
            <Button size="small" type="primary" onClick={() => void act(r, 'open')}>
              开通
            </Button>
          ) : null}
          {r.status === 1 ? (
            <Button size="small" danger onClick={() => void act(r, 'suspend')}>
              停用
            </Button>
          ) : null}
          <Button size="small" onClick={() => void openFees(r)}>
            费用台账
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
      <Table rowKey="id" loading={loading} columns={columns} dataSource={rows} size="small" />

      <Modal
        title={`费用台账 · ${fees?.op.name ?? ''}`}
        open={!!fees}
        onCancel={() => setFees(null)}
        footer={[
          <Button key="refund" danger onClick={() => void refund()} disabled={(fees?.data.guarantee_fen ?? 0) === 0}>
            保证金退款
          </Button>,
          <Button key="ok" type="primary" onClick={() => void saveFees()}>
            登记
          </Button>,
        ]}
      >
        <p style={{ color: '#888' }}>
          已收代理费 ¥{fees ? yuan(fees.data.agency_fee_paid_fen) : '0.00'}；保证金 ¥
          {fees ? yuan(fees.data.guarantee_fen) : '0.00'}（可退预收台账）
        </p>
        <Form form={form} layout="vertical">
          <Form.Item name="agency_fee_fen" label="代理费（分）">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="agency_fee_year" label="代理费年度（如 1=首年）">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="guarantee_fen" label="保证金（分，新增登记会入台账）">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
        <div style={{ marginTop: 8, color: '#888' }}>
          台账记录：
          {(fees?.data.guarantees ?? []).map((g) => (
            <div key={g.id}>
              #{g.id} ¥{yuan(g.amount_fen)} {g.refunded_at ? `（已退：${g.refund_reason || '—'}）` : '（在账）'}
            </div>
          ))}
        </div>
      </Modal>
    </>
  )
}
