import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Modal, Select, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminContent } from '../api/admin'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '审核中', color: 'orange' },
  1: { text: '展示中', color: 'green' },
  2: { text: '已下架', color: 'default' },
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : '操作失败'
}

// 内容管理：列表 + 下架（下架原因）。
export default function Contents() {
  const [rows, setRows] = useState<AdminContent[]>([])
  const [loading, setLoading] = useState(false)
  const [kind, setKind] = useState<string | undefined>()
  const [status, setStatus] = useState<number | undefined>()
  const [target, setTarget] = useState<AdminContent | null>(null)
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.contents({ kind, status })
      setRows(res.items ?? [])
    } catch (e) {
      message.error(errText(e))
    } finally {
      setLoading(false)
    }
  }, [kind, status])

  useEffect(() => {
    void load()
  }, [load])

  const confirmTakedown = async () => {
    if (!target) return
    try {
      await adminApi.takedownContent(target.id, reason)
      message.success('已下架')
      setTarget(null)
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const columns: ColumnsType<AdminContent> = [
    { title: 'ID', dataIndex: 'id' },
    { title: '类型', dataIndex: 'kind' },
    { title: '标题', dataIndex: 'title' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: number) => <Tag color={STATUS[s]?.color}>{STATUS[s]?.text ?? s}</Tag>,
    },
    { title: '驳回/下架原因', dataIndex: 'reject_reason' },
    {
      title: '操作',
      render: (_, r) =>
        r.status === 1 ? (
          <Button
            size="small"
            danger
            onClick={() => {
              setTarget(r)
              setReason('')
            }}
          >
            下架
          </Button>
        ) : null,
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Select
          allowClear
          placeholder="类型"
          style={{ width: 160 }}
          value={kind}
          onChange={setKind}
          options={[
            { value: 'project', label: '项目' },
            { value: 'product', label: '产品' },
            { value: 'meeting', label: '会议' },
            { value: 'event', label: '活动' },
          ]}
        />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 160 }}
          value={status}
          onChange={setStatus}
          options={[
            { value: 0, label: '审核中' },
            { value: 1, label: '展示中' },
            { value: 2, label: '已下架' },
          ]}
        />
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      <Table rowKey="id" loading={loading} columns={columns} dataSource={rows} size="small" />
      <Modal
        title="下架内容"
        open={!!target}
        onOk={() => void confirmTakedown()}
        onCancel={() => setTarget(null)}
        okText="确认下架"
        okButtonProps={{ danger: true }}
      >
        <Input.TextArea
          rows={3}
          placeholder="下架原因"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </Modal>
    </>
  )
}
