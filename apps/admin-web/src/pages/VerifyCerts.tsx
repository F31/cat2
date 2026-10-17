import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Modal, Space, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminCert } from '../api/admin'
import DataTable from '../components/DataTable'

const CERT_TYPE: Record<number, string> = { 1: '实名', 2: '企业', 3: '组织' }
const CERT_STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '待审核', color: 'orange' },
  1: { text: '已通过', color: 'green' },
  2: { text: '已驳回', color: 'red' },
}

// 认证审核（p4 §8.1 verify）：平台队列，审核结论写审计；企业审核入口在小程序。
export default function VerifyCerts() {
  const [rows, setRows] = useState<AdminCert[]>([])
  const [loading, setLoading] = useState(false)
  const [action, setAction] = useState<{ cert: AdminCert; pass: boolean } | null>(null)
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.pendingCerts(1)
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

  const confirm = async () => {
    if (!action) return
    if (!action.pass && !reason.trim()) {
      message.warning('驳回必须填写原因')
      return
    }
    try {
      await adminApi.reviewCert(action.cert.id, action.pass, reason)
      message.success('已处理')
      setAction(null)
      setReason('')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败')
    }
  }

  const columns: ColumnsType<AdminCert> = [
    { title: '认证ID', dataIndex: 'id' },
    { title: '用户ID', dataIndex: 'user_id' },
    { title: '类型', dataIndex: 'cert_type', render: (v: number) => CERT_TYPE[v] ?? v },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: number) => <Tag color={CERT_STATUS[v]?.color}>{CERT_STATUS[v]?.text ?? v}</Tag>,
    },
    {
      title: '提交时间',
      dataIndex: 'submitted_at',
      render: (v: string) => (v ? v.slice(0, 19).replace('T', ' ') : '—'),
    },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" type="primary" onClick={() => setAction({ cert: r, pass: true })}>
            通过
          </Button>
          <Button size="small" danger onClick={() => setAction({ cert: r, pass: false })}>
            驳回
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void load()}>刷新队列</Button>
      </Space>
      <DataTable rowKey="id" loading={loading} columns={columns} dataSource={rows} />
      <Modal
        title={action?.pass ? '通过认证' : '驳回认证'}
        open={!!action}
        onOk={() => void confirm()}
        onCancel={() => setAction(null)}
        okText="确认"
        okButtonProps={{ danger: !action?.pass }}
      >
        <Input.TextArea
          rows={3}
          placeholder={action?.pass ? '备注（可选）' : '驳回原因（必填）'}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </Modal>
    </>
  )
}
