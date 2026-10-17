import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type FieldDef, type ProfileTemplate } from '../api/admin'

// 画像字段模板（p4 §8.1 profile）：主体类型 × 模板字段 + 字段池；字段池由代码侧登记随版本发布。
export default function ProfileTemplates() {
  const [items, setItems] = useState<ProfileTemplate[]>([])
  const [pool, setPool] = useState<FieldDef[]>([])
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.profileTemplates()
      setItems(res.items ?? [])
      setPool(res.field_pool ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const cols: ColumnsType<FieldDef> = [
    { title: '字段', dataIndex: 'field' },
    { title: '名称', dataIndex: 'label' },
    { title: '类型', dataIndex: 'value_type', render: (v: string) => <Tag>{v}</Tag> },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void load()} loading={loading}>
          刷新
        </Button>
        <span style={{ color: '#888' }}>模板与字段池随版本发布；此处用于核对渲染顺序与必填项。</span>
      </Space>
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        {items.map((t) => (
          <Card key={t.code} size="small" title={`${t.name}（${t.code}）`}>
            <Table
              rowKey="field"
              size="small"
              pagination={false}
              dataSource={t.fields ?? []}
              columns={[
                { title: '顺序', dataIndex: 'sort', width: 80 },
                { title: '字段', dataIndex: 'field' },
                { title: '名称', dataIndex: 'label' },
                { title: '类型', dataIndex: 'value_type', render: (v: string) => <Tag>{v}</Tag> },
                {
                  title: '必填',
                  dataIndex: 'required',
                  width: 90,
                  render: (v: boolean) => (v ? <Tag color="red">必填</Tag> : '—'),
                },
              ]}
            />
          </Card>
        ))}
        <Card size="small" title="字段池（dct_field_def）">
          <Table rowKey="field" size="small" pagination={false} dataSource={pool} columns={cols} />
        </Card>
      </Space>
    </>
  )
}
