import { useCallback, useEffect, useState } from 'react'
import { Button, Input, InputNumber, Modal, Select, Space, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminMember } from '../api/admin'
import DataTable from '../components/DataTable'

const LEVEL: Record<number, string> = { 1: 'VIP', 2: 'SVIP', 3: '企业版' }
const STATUS: Record<number, { text: string; color: string }> = {
  1: { text: '生效中', color: 'green' },
  2: { text: '已过期', color: 'default' },
}

type Act = { kind: 'adjust' | 'transfer' | 'remove'; row: AdminMember }

// 会员管理（p4 §8.1 membership）：列表 + 人工调整 / 主账号转移 / 强制解除子账号（均要求原因并写审计）。
export default function Members() {
  const [rows, setRows] = useState<AdminMember[]>([])
  const [loading, setLoading] = useState(false)
  const [level, setLevel] = useState<number | undefined>()
  const [status, setStatus] = useState<number | undefined>()
  const [act, setAct] = useState<Act | null>(null)
  const [reason, setReason] = useState('')
  const [newLevel, setNewLevel] = useState(3)
  const [expireAt, setExpireAt] = useState('')
  const [toUser, setToUser] = useState('')
  const [subUser, setSubUser] = useState('')
  const [reviewToken, setReviewToken] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.members({ level, status })
      setRows(res.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [level, status])

  useEffect(() => {
    void load()
  }, [load])

  const open = (kind: Act['kind'], row: AdminMember) => {
    setAct({ kind, row })
    setReason('')
    setExpireAt('')
    setToUser('')
    setSubUser('')
    setReviewToken('')
    setNewLevel(row.level || 3)
  }

  const submit = async () => {
    if (!act) return
    if (!reason.trim()) {
      message.warning('必须填写原因')
      return
    }
    try {
      if (act.kind === 'adjust') {
        await adminApi.adjustMember(act.row.user_id, newLevel, reason, expireAt.trim() || undefined, reviewToken.trim() || undefined)
      } else if (act.kind === 'transfer') {
        if (!toUser.trim()) return message.warning('目标账号必填')
        await adminApi.transferMaster(act.row.user_id, toUser.trim(), reason)
      } else {
        if (!subUser.trim()) return message.warning('子账号必填')
        await adminApi.forceRemoveSub(act.row.user_id, subUser.trim(), reason)
      }
      message.success('已处理')
      setAct(null)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败')
    }
  }

  const columns: ColumnsType<AdminMember> = [
    { title: '用户ID', dataIndex: 'user_id' },
    { title: '等级', dataIndex: 'level', render: (l: number) => LEVEL[l] ?? l },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: number) => <Tag color={STATUS[s]?.color}>{STATUS[s]?.text ?? s}</Tag>,
    },
    { title: '开始时间', dataIndex: 'started_at', render: (t: string) => (t ? t.slice(0, 10) : '') },
    { title: '到期时间', dataIndex: 'expires_at', render: (t: string) => (t ? t.slice(0, 10) : '') },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" onClick={() => open('adjust', r)}>
            调整
          </Button>
          <Button size="small" disabled={r.level !== 3} onClick={() => open('transfer', r)}>
            转移主账号
          </Button>
          <Button size="small" disabled={r.level !== 3} onClick={() => open('remove', r)}>
            解除子账号
          </Button>
        </Space>
      ),
    },
  ]

  const title =
    act?.kind === 'adjust' ? '会员人工调整' : act?.kind === 'transfer' ? '转移企业版主账号' : '强制解除子账号'

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Select
          allowClear
          placeholder="等级"
          style={{ width: 160 }}
          value={level}
          onChange={setLevel}
          options={[
            { value: 1, label: 'VIP' },
            { value: 3, label: '企业版' },
          ]}
        />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 160 }}
          value={status}
          onChange={setStatus}
          options={[
            { value: 1, label: '生效中' },
            { value: 2, label: '已过期' },
          ]}
        />
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      <DataTable rowKey="user_id" loading={loading} columns={columns} dataSource={rows} />
      <Modal
        title={`${title}（用户 ${act?.row.user_id ?? ''}）`}
        open={!!act}
        onOk={() => void submit()}
        onCancel={() => setAct(null)}
        okText="确认"
        okButtonProps={{ danger: act?.kind !== 'adjust' }}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          {act?.kind === 'adjust' ? (
            <>
              <Select
                style={{ width: '100%' }}
                value={newLevel}
                onChange={setNewLevel}
                options={[
                  { value: 1, label: 'VIP' },
                  { value: 2, label: 'SVIP' },
                  { value: 3, label: '企业版' },
                ]}
              />
              <Input
                placeholder="到期时间（RFC3339，如 2027-01-01T00:00:00+08:00；留空不改）"
                value={expireAt}
                onChange={(e) => setExpireAt(e.target.value)}
              />
            </>
          ) : act?.kind === 'transfer' ? (
            <InputNumber
              style={{ width: '100%' }}
              controls={false}
              placeholder="目标企业版账号用户ID"
              value={toUser ? Number(toUser) : undefined}
              onChange={(v) => setToUser(v == null ? '' : String(v))}
            />
          ) : (
            <InputNumber
              style={{ width: '100%' }}
              controls={false}
              placeholder="待解除的子账号用户ID"
              value={subUser ? Number(subUser) : undefined}
              onChange={(v) => setSubUser(v == null ? '' : String(v))}
            />
          )}
          {act?.kind === 'adjust' ? (
            <Input.Password
              placeholder="复核人令牌 X-Review-Token（双人复核开启时必填，且不能与当前账号相同）"
              value={reviewToken}
              onChange={(e) => setReviewToken(e.target.value)}
            />
          ) : null}
          <Input.TextArea rows={3} placeholder="原因（必填，写入审计）" value={reason} onChange={(e) => setReason(e.target.value)} />
        </Space>
      </Modal>
    </>
  )
}
