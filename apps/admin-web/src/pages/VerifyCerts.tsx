import { useCallback, useEffect, useState } from 'react'
import { Button, Descriptions, Input, Modal, Space, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminCert, type RealnameDetail } from '../api/admin'
import DataTable from '../components/DataTable'
import MediaThumb from '../components/MediaThumb'

const CERT_TYPE: Record<number, string> = { 1: '实名', 2: '个人企业身份', 3: '组织', 4: '企业', 5: '机构' }
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
  const [realname, setRealname] = useState<RealnameDetail | null>(null)
  const [realnameReason, setRealnameReason] = useState('')
  const [realnameLoading, setRealnameLoading] = useState(false)

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

  const openRealname = async (cert: AdminCert) => {
    setRealnameLoading(true)
    try {
      const detail = await adminApi.realnameDetail(cert.id)
      setRealname(detail)
      setRealnameReason('')
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载实名详情失败')
    } finally {
      setRealnameLoading(false)
    }
  }

  const reviewRealname = async (pass: boolean) => {
    if (!realname) return
    if (!pass && !realnameReason.trim()) {
      message.warning('驳回必须填写原因')
      return
    }
    try {
      await adminApi.reviewCert(realname.cert_id, pass, realnameReason)
      message.success('已处理')
      setRealname(null)
      setRealnameReason('')
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
      title: '申请信息',
      render: (_, r) => (
        <div style={{ fontSize: 12 }}>
          {r.extra?.org_code ? <div>代码：{r.extra.org_code}</div> : null}
          {r.extra?.legal_name ? <div>负责人：{r.extra.legal_name}</div> : null}
          {!r.extra?.org_code && !r.extra?.legal_name ? '—' : null}
        </div>
      ),
    },
    {
      title: '资质材料',
      render: (_, r) => (
        <Space wrap>
          {(r.material_file_ids ?? []).length === 0
            ? '—'
            : (r.material_file_ids ?? []).map((id) => <MediaThumb key={id} id={id} />)}
        </Space>
      ),
    },
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
          {r.cert_type === 1 ? (
            <Button size="small" type="primary" loading={realnameLoading} onClick={() => void openRealname(r)}>
              查看实名
            </Button>
          ) : (
            <>
              <Button size="small" type="primary" onClick={() => setAction({ cert: r, pass: true })}>
                通过
              </Button>
              <Button size="small" danger onClick={() => setAction({ cert: r, pass: false })}>
                驳回
              </Button>
            </>
          )}
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

      <Modal
        title="实名审核详情"
        open={!!realname}
        onCancel={() => setRealname(null)}
        footer={
          <Space>
            <Button danger onClick={() => void reviewRealname(false)}>
              驳回
            </Button>
            <Button type="primary" onClick={() => void reviewRealname(true)}>
              通过
            </Button>
          </Space>
        }
        width={680}
      >
        {realname ? (
          <>
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="用户 ID">{realname.user_id}</Descriptions.Item>
              <Descriptions.Item label="姓名">{realname.real_name || '—'}</Descriptions.Item>
              <Descriptions.Item label="身份证号">{realname.id_card_no || '—'}</Descriptions.Item>
              <Descriptions.Item label="提交时间">
                {realname.submitted_at ? realname.submitted_at.slice(0, 19).replace('T', ' ') : '—'}
              </Descriptions.Item>
            </Descriptions>
            <div style={{ margin: '12px 0' }}>
              <Space wrap>
                {(realname.material_file_ids ?? []).map((id) => (
                  <MediaThumb key={id} id={id} />
                ))}
                {(realname.material_file_ids ?? []).length === 0 ? <Tag>无材料</Tag> : null}
              </Space>
            </div>
            <p style={{ color: '#888', fontSize: 12 }}>查看实名详情已写入审计；请仅用于本次审核。</p>
            <Input.TextArea
              rows={3}
              placeholder="驳回原因（驳回必填）"
              value={realnameReason}
              onChange={(e) => setRealnameReason(e.target.value)}
            />
          </>
        ) : null}
      </Modal>
    </>
  )
}
