import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Col, Form, Input, message, Modal, Row, Select, Space, Statistic, Switch, Table, Tag } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AiPrompt, type AiUsage } from '../api/admin'

// AI 提示词与用量（p5 §7.2/§8.2）：版本管理（仅 approved 生效）+ 各能力调用/采纳率/成本。
// 提示词变更走审计；生效须评测通过（approved=true 的最高版本生效）。
const FEATURES = ['onboard', 'publish', 'search', 'review']

export default function AiPrompts() {
  const [prompts, setPrompts] = useState<AiPrompt[]>([])
  const [usage, setUsage] = useState<AiUsage[]>([])
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<AiPrompt | null>(null)
  const [form] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [p, u] = await Promise.all([adminApi.aiPrompts(), adminApi.aiUsage(7)])
      setPrompts(p.items ?? [])
      setUsage(u.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const open = (p?: AiPrompt) => {
    const cur = p ?? { feature: 'onboard', version: '', body: '', approved: false }
    setEditing(cur)
    form.setFieldsValue(cur)
  }

  const save = async () => {
    const v = await form.validateFields()
    try {
      await adminApi.aiSavePrompt({
        feature: v.feature,
        version: v.version,
        body: v.body,
        approved: !!v.approved,
      })
      message.success('已保存（approved 决定是否生效）')
      setEditing(null)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  const promptColumns: ColumnsType<AiPrompt> = [
    { title: '能力', dataIndex: 'feature', width: 100 },
    { title: '版本', dataIndex: 'version', width: 100 },
    { title: '内容', dataIndex: 'body', ellipsis: true },
    {
      title: '状态',
      dataIndex: 'approved',
      width: 100,
      render: (v: boolean) => (v ? <Tag color="green">已生效</Tag> : <Tag>未评测</Tag>),
    },
    {
      title: '操作',
      width: 90,
      render: (_, r) => (
        <Button size="small" onClick={() => open(r)}>
          编辑
        </Button>
      ),
    },
  ]

  const usageColumns: ColumnsType<AiUsage> = [
    { title: '能力', dataIndex: 'feature' },
    { title: '成功调用', dataIndex: 'calls' },
    { title: '降级', dataIndex: 'degraded' },
    { title: '采纳', dataIndex: 'adopted' },
    { title: '放弃', dataIndex: 'rejected' },
    {
      title: '采纳率',
      dataIndex: 'adopt_rate',
      render: (v: number) => `${(v * 100).toFixed(1)}%`,
    },
    { title: 'Tokens', dataIndex: 'tokens' },
    {
      title: '成本(元)',
      dataIndex: 'cost_fen',
      render: (v: number) => (v / 100).toFixed(2),
    },
  ]

  const totalCost = usage.reduce((s, u) => s + (u.cost_fen ?? 0), 0)
  const totalCalls = usage.reduce((s, u) => s + (u.calls ?? 0), 0)

  return (
    <Row gutter={16}>
      <Col span={12}>
        <Card
          title="提示词版本（仅 approved 生效）"
          extra={<Button type="primary" size="small" onClick={() => open()}>新增版本</Button>}
        >
          <Table rowKey={(r) => `${r.feature}:${r.version}`} loading={loading} columns={promptColumns} dataSource={prompts} size="small" pagination={false} />
        </Card>
      </Col>
      <Col span={12}>
        <Card title="近 7 天用量（采纳率与成本）">
          <Space size="large" style={{ marginBottom: 12 }}>
            <Statistic title="总调用" value={totalCalls} />
            <Statistic title="成本(元)" value={totalCost / 100} precision={2} />
          </Space>
          <Table rowKey="feature" columns={usageColumns} dataSource={usage} size="small" pagination={false} />
        </Card>
      </Col>

      <Modal title={editing?.version ? '编辑提示词版本' : '新增提示词版本'} open={!!editing} onOk={() => void save()} onCancel={() => setEditing(null)} destroyOnClose>
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item name="feature" label="能力" rules={[{ required: true }]}>
            <Select options={FEATURES.map((f) => ({ value: f, label: f }))} />
          </Form.Item>
          <Form.Item name="version" label="版本号" rules={[{ required: true }]}>
            <Input placeholder="如 v2" />
          </Form.Item>
          <Form.Item name="body" label="提示词内容" rules={[{ required: true }]}>
            <Input.TextArea rows={5} />
          </Form.Item>
          <Form.Item name="approved" label="评测通过并生效" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </Row>
  )
}
