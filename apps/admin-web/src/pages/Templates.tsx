import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Modal, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type NoticeTemplate } from '../api/admin'

// 通知（p4 §8.1 notice）：模板清单 + 定向公告发布（必填原因，服务端写审计）。
export default function Templates() {
  const [rows, setRows] = useState<NoticeTemplate[]>([])
  const [loading, setLoading] = useState(false)
  const [broadcasting, setBroadcasting] = useState(false)
  const [userIds, setUserIds] = useState('')
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.noticeTemplates()
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

  const broadcast = async () => {
    const ids = userIds
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    if (ids.length === 0 || !title.trim() || !content.trim() || !reason.trim()) {
      message.warning('用户、标题、内容与原因均必填')
      return
    }
    try {
      const r = await adminApi.broadcastNotice(ids, title.trim(), content.trim(), reason.trim())
      message.success(`已发送 ${r.sent} 条`)
      setBroadcasting(false)
      setUserIds('')
      setTitle('')
      setContent('')
      setReason('')
    } catch (e) {
      message.error(e instanceof Error ? e.message : '发送失败')
    }
  }

  const columns: ColumnsType<NoticeTemplate> = [
    { title: '模板码', dataIndex: 'code' },
    { title: '模块', dataIndex: 'module' },
    { title: '标题模板', dataIndex: 'title_tpl' },
    {
      title: '渠道',
      dataIndex: 'channels',
      render: (chs: string[]) => (chs ?? []).map((c) => <Tag key={c}>{c}</Tag>),
    },
    { title: '合并', dataIndex: 'merge_rule', render: (v?: string) => (v ? <Tag color="blue">同日合并</Tag> : '-') },
    {
      title: '可关闭',
      dataIndex: 'closeable',
      render: (v: boolean) => (v ? <Tag>可关闭</Tag> : <Tag color="red">不可关闭</Tag>),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void load()}>刷新</Button>
        <Button type="primary" onClick={() => setBroadcasting(true)}>
          定向公告
        </Button>
      </Space>
      <Table rowKey="code" loading={loading} columns={columns} dataSource={rows} size="small" />
      <Modal
        title="定向公告发布"
        open={broadcasting}
        onOk={() => void broadcast()}
        onCancel={() => setBroadcasting(false)}
        okText="发送"
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <Input placeholder="接收用户ID（逗号分隔）" value={userIds} onChange={(e) => setUserIds(e.target.value)} />
          <Input placeholder="标题" value={title} onChange={(e) => setTitle(e.target.value)} />
          <Input.TextArea rows={3} placeholder="内容" value={content} onChange={(e) => setContent(e.target.value)} />
          <Input.TextArea rows={2} placeholder="发布原因（必填，写入审计）" value={reason} onChange={(e) => setReason(e.target.value)} />
        </Space>
      </Modal>
    </>
  )
}
