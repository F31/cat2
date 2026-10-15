import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Modal, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminUser } from '../api/admin'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '正常', color: 'green' },
  1: { text: '限制', color: 'orange' },
  2: { text: '停用', color: 'red' },
  3: { text: '注销', color: 'default' },
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : '操作失败'
}

// 用户列表与封禁/解封。
export default function Users() {
  const [rows, setRows] = useState<AdminUser[]>([])
  const [loading, setLoading] = useState(false)
  const [q, setQ] = useState('')
  const [banTarget, setBanTarget] = useState<AdminUser | null>(null)
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.users({ q: q || undefined })
      setRows(res.items ?? [])
    } catch (e) {
      message.error(errText(e))
    } finally {
      setLoading(false)
    }
  }, [q])

  useEffect(() => {
    void load()
  }, [load])

  const unban = async (u: AdminUser) => {
    try {
      await adminApi.unbanUser(u.id)
      message.success('已解封')
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const confirmBan = async () => {
    if (!banTarget) return
    try {
      await adminApi.banUser(banTarget.id, reason)
      message.success('已封禁')
      setBanTarget(null)
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const columns: ColumnsType<AdminUser> = [
    { title: 'ID', dataIndex: 'id' },
    { title: '昵称', dataIndex: 'nickname' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: number) => <Tag color={STATUS[s]?.color}>{STATUS[s]?.text ?? s}</Tag>,
    },
    { title: '手机', dataIndex: 'phone_masked' },
    {
      title: '注册时间',
      dataIndex: 'created_at',
      render: (t: string) => (t ? t.slice(0, 19).replace('T', ' ') : ''),
    },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          {r.status === 2 ? (
            <Button size="small" onClick={() => void unban(r)}>
              解封
            </Button>
          ) : (
            <Button
              size="small"
              danger
              onClick={() => {
                setBanTarget(r)
                setReason('')
              }}
            >
              封禁
            </Button>
          )}
        </Space>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Input
          placeholder="按昵称搜索"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onPressEnter={() => void load()}
          style={{ width: 240 }}
        />
        <Button onClick={() => void load()}>搜索</Button>
      </Space>
      <Table rowKey="id" loading={loading} columns={columns} dataSource={rows} size="small" />
      <Modal
        title="封禁用户"
        open={!!banTarget}
        onOk={() => void confirmBan()}
        onCancel={() => setBanTarget(null)}
        okText="确认封禁"
        okButtonProps={{ danger: true }}
      >
        <Input.TextArea
          rows={3}
          placeholder="封禁原因"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </Modal>
    </>
  )
}
