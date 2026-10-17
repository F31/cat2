import { useCallback, useEffect, useState } from 'react'
import { Button, Form, Input, Modal, Select, Space, Switch, Table, Tag, message } from 'antd'
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
  const [editing, setEditing] = useState<NoticeTemplate | null>(null)
  const [form] = Form.useForm()

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

  const openEdit = (t: NoticeTemplate) => {
    setEditing(t)
    form.setFieldsValue({
      title_tpl: t.title_tpl,
      content_tpl: t.content_tpl,
      channels: t.channels ?? ['inapp'],
      closeable: t.closeable,
    })
  }

  const saveTemplate = async () => {
    if (!editing) return
    const v = await form.validateFields()
    try {
      await adminApi.updateNoticeTemplate(editing.code, {
        title_tpl: String(v.title_tpl).trim(),
        content_tpl: String(v.content_tpl).trim(),
        channels: v.channels ?? [],
        closeable: !!v.closeable,
      })
      message.success('模板已更新')
      setEditing(null)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败')
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
    {
      title: '操作',
      render: (_, r) => (
        <Button size="small" onClick={() => openEdit(r)}>
          编辑
        </Button>
      ),
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
      <Modal title={`编辑模板 · ${editing?.code ?? ''}`} open={!!editing} onOk={() => void saveTemplate()} onCancel={() => setEditing(null)} okText="保存">
        <Form form={form} layout="vertical">
          <Form.Item name="title_tpl" label="标题模板" rules={[{ required: true, message: '请输入标题模板' }]}>
            <Input maxLength={120} />
          </Form.Item>
          <Form.Item name="content_tpl" label="内容模板" rules={[{ required: true, message: '请输入内容模板' }]}>
            <Input.TextArea rows={4} maxLength={500} />
          </Form.Item>
          <Form.Item name="channels" label="渠道" rules={[{ required: true, message: '至少选择一个渠道' }]}>
            <Select
              mode="multiple"
              options={[
                { value: 'inapp', label: '站内信' },
                { value: 'subscribe', label: '订阅消息' },
                { value: 'sms', label: '短信' },
              ]}
            />
          </Form.Item>
          <Form.Item name="closeable" label="允许用户关闭" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

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
