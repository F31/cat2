import { useCallback, useEffect, useState } from 'react'
import { Button, Form, Input, InputNumber, Modal, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi } from '../api/admin'

interface BadgeDef {
  code: string
  label: string
  level: number
}

// 认证标识配置（p4 §6.7）：新增标识=加配置+前端样式，不改接口；授予/撤销为代操作（审计）。
export default function Badges() {
  const [rows, setRows] = useState<BadgeDef[]>([])
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<BadgeDef | null>(null)
  const [granting, setGranting] = useState(false)
  const [form] = Form.useForm()
  const [grantForm] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.badgeDefs()
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

  const openEdit = (d?: BadgeDef) => {
    const cur = d ?? { code: '', label: '', level: 1 }
    setEditing(cur)
    form.setFieldsValue(cur)
  }

  const save = async () => {
    const v = await form.validateFields()
    try {
      await adminApi.upsertBadge(v.code, v.label, Number(v.level) || 1)
      message.success('已保存（前端按 code 渲染样式）')
      setEditing(null)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  const grant = async () => {
    const v = await grantForm.validateFields()
    try {
      await adminApi.grantBadge(v.subject_type, v.subject_id, v.code, !!v.grant)
      message.success('已提交（审计留痕）')
      setGranting(false)
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败')
    }
  }

  const columns: ColumnsType<BadgeDef> = [
    { title: '编码', dataIndex: 'code' },
    { title: '名称', dataIndex: 'label' },
    { title: '层级', dataIndex: 'level', render: (v: number) => <Tag>{v}</Tag> },
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
        <Button type="primary" onClick={() => openEdit()}>
          新增标识
        </Button>
        <Button onClick={() => setGranting(true)}>授予 / 撤销</Button>
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      <Table rowKey="code" loading={loading} columns={columns} dataSource={rows} size="small" />
      <Modal title="标识定义" open={!!editing} onOk={() => void save()} onCancel={() => setEditing(null)} okText="保存">
        <Form form={form} layout="vertical">
          <Form.Item name="code" label="编码" rules={[{ required: true }]}>
            <Input disabled={!!editing?.code} placeholder="如 partner" />
          </Form.Item>
          <Form.Item name="label" label="名称" rules={[{ required: true }]}>
            <Input placeholder="如 生态伙伴" />
          </Form.Item>
          <Form.Item name="level" label="层级（1-3，3 最高）">
            <InputNumber min={1} max={3} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
      <Modal title="授予 / 撤销标识" open={granting} onOk={() => void grant()} onCancel={() => setGranting(false)} okText="提交">
        <Form form={grantForm} layout="vertical" initialValues={{ subject_type: 'user', grant: true }}>
          <Form.Item name="subject_type" label="主体类型" rules={[{ required: true }]}>
            <Input placeholder="user / org" />
          </Form.Item>
          <Form.Item name="subject_id" label="主体 ID" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="code" label="标识编码" rules={[{ required: true }]}>
            <Input placeholder="如 official" />
          </Form.Item>
          <Form.Item name="grant" label="动作">
            <Input placeholder="true=授予 / false=撤销" />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
