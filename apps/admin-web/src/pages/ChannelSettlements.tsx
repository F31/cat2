import { useCallback, useEffect, useState } from 'react'
import { Button, Form, Input, InputNumber, Modal, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type ChannelSettlementRow } from '../api/admin'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '待结算', color: 'gold' },
  1: { text: '已复核', color: 'blue' },
  2: { text: '已打款', color: 'green' },
  3: { text: '已驳回', color: 'red' },
}

function yuan(fen: number) {
  return (fen / 100).toFixed(2)
}

// 渠道结算（p4 §2.4）：生成 → 复核 → 打款（复核≠执行，同一人不可既复核又打款）。
export default function ChannelSettlements() {
  const [rows, setRows] = useState<ChannelSettlementRow[]>([])
  const [loading, setLoading] = useState(false)
  const [genOpen, setGenOpen] = useState(false)
  const [paying, setPaying] = useState<ChannelSettlementRow | null>(null)
  const [genForm] = Form.useForm()
  const [payForm] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.channelSettlements()
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

  const generate = async () => {
    const v = await genForm.validateFields()
    try {
      await adminApi.channelGenerateSettlement(String(v.operator_id).trim(), (v.month as string) || undefined)
      message.success('已生成')
      setGenOpen(false)
      genForm.resetFields()
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '生成失败')
    }
  }

  const review = async (r: ChannelSettlementRow) => {
    try {
      await adminApi.channelReviewSettlement(String(r.id))
      message.success('已复核')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '复核失败')
    }
  }

  const reject = async (r: ChannelSettlementRow) => {
    try {
      await adminApi.channelRejectSettlement(String(r.id), '资料不符')
      message.success('已驳回')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '驳回失败')
    }
  }

  const pay = async () => {
    if (!paying) return
    const v = await payForm.validateFields()
    try {
      await adminApi.channelPaySettlement(String(paying.id), v.voucher_ref)
      message.success('已打款')
      setPaying(null)
      payForm.resetFields()
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '打款失败（复核≠执行）')
    }
  }

  const columns: ColumnsType<ChannelSettlementRow> = [
    { title: 'ID', dataIndex: 'id' },
    { title: '运营商', dataIndex: 'operator_id' },
    { title: '月份', dataIndex: 'month' },
    { title: '应结（元）', dataIndex: 'payable_fen', render: (v: number) => yuan(v) },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: number) => <Tag color={STATUS[v]?.color}>{STATUS[v]?.text ?? v}</Tag>,
    },
    { title: '复核人', dataIndex: 'reviewed_by', render: (v: number) => (v ? v : '—') },
    { title: '打款人', dataIndex: 'paid_by', render: (v: number) => (v ? v : '—') },
    { title: '凭证', dataIndex: 'voucher_ref', render: (v: string) => v || '—' },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          {r.status === 0 ? (
            <>
              <Button size="small" type="primary" onClick={() => void review(r)}>
                复核
              </Button>
              <Button size="small" danger onClick={() => void reject(r)}>
                驳回
              </Button>
            </>
          ) : null}
          {r.status === 1 ? (
            <Button
              size="small"
              onClick={() => {
                setPaying(r)
                payForm.resetFields()
              }}
            >
              打款
            </Button>
          ) : null}
        </Space>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void load()}>刷新</Button>
        <Button type="primary" onClick={() => setGenOpen(true)}>
          生成结算单
        </Button>
      </Space>
      <Table rowKey="id" loading={loading} columns={columns} dataSource={rows} size="small" />

      <Modal title="生成结算单" open={genOpen} onOk={() => void generate()} onCancel={() => setGenOpen(false)} okText="生成">
        <Form form={genForm} layout="vertical">
          <Form.Item name="operator_id" label="运营商 ID" rules={[{ required: true }]}>
            <Input placeholder="运营商 ID" />
          </Form.Item>
          <Form.Item name="month" label="月份（留空=当月，YYYY-MM）">
            <Input placeholder="2026-10" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title="结算打款（复核≠执行）" open={!!paying} onOk={() => void pay()} onCancel={() => setPaying(null)} okText="确认打款">
        <p style={{ color: '#888' }}>同一人不得既复核又打款；请填写对公打款凭证（media 引用）。</p>
        <Form form={payForm} layout="vertical">
          <Form.Item name="voucher_ref" label="凭证引用" rules={[{ required: true }]}>
            <Input placeholder="media://..." />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
