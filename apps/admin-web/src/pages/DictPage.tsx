import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Input, Select, Space, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type DictOption, type DictSubjectType } from '../api/admin'
import DataTable from '../components/DataTable'

const DIMENSIONS = [
  { value: 'industry', label: '行业' },
  { value: 'sub_industry', label: '子行业' },
  { value: 'region', label: '区域' },
  { value: 'nature', label: '性质' },
  { value: 'need', label: '需求' },
  { value: 'offer', label: '供给' },
  { value: 'domain', label: '领域' },
]

// 主体类型与字典选项（p4 §8.1 dict）：后台只读核对；变更走种子/版本发布（保持口径一致）。
export default function DictPage() {
  const [types, setTypes] = useState<DictSubjectType[]>([])
  const [opts, setOpts] = useState<DictOption[]>([])
  const [dimension, setDimension] = useState('industry')
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [t, o] = await Promise.all([adminApi.dictSubjectTypes(), adminApi.dictOptions(dimension)])
      setTypes(t ?? [])
      setOpts(o ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [dimension])

  useEffect(() => {
    void load()
  }, [load])

  const filteredOpts = useMemo(() => {
    const kw = keyword.trim().toLowerCase()
    if (!kw) return opts
    return opts.filter((o) =>
      [o.code, o.label, o.parent_code ?? ''].some((v) => String(v).toLowerCase().includes(kw)),
    )
  }, [opts, keyword])

  const levelOf = (code: string) => {
    if (dimension === 'industry') {
      return code.length === 1 ? '门类' : code.length === 2 ? '大类' : code.length === 3 ? '中类' : '小类'
    }
    if (dimension === 'region') {
      return code.endsWith('0000') ? '省级' : code.endsWith('00') ? '市级' : '区县级'
    }
    return '—'
  }

  const typeCols: ColumnsType<DictSubjectType> = [
    { title: '编码', dataIndex: 'code' },
    { title: '名称', dataIndex: 'name' },
    { title: '可发布', dataIndex: 'can_publish', render: (v: boolean) => (v ? <Tag color="green">是</Tag> : '—') },
    { title: '首页展示', dataIndex: 'home_visible', render: (v: boolean) => (v ? <Tag color="blue">是</Tag> : '—') },
    { title: '排序', dataIndex: 'sort' },
    { title: '启用', dataIndex: 'enabled', render: (v: boolean) => (v ? <Tag>启用</Tag> : <Tag color="red">停用</Tag>) },
  ]

  const optCols: ColumnsType<DictOption> = [
    { title: '编码', dataIndex: 'code' },
    { title: '名称', dataIndex: 'label' },
    { title: '层级', dataIndex: 'code', width: 90, render: (v: string) => levelOf(v) },
    { title: '父级', dataIndex: 'parent_code', render: (v?: string) => v || '—' },
    { title: '排序', dataIndex: 'sort', width: 80 },
    { title: '启用', dataIndex: 'enabled', width: 80, render: (v: boolean) => (v ? <Tag>启用</Tag> : <Tag color="red">停用</Tag>) },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Select style={{ width: 180 }} value={dimension} onChange={setDimension} options={DIMENSIONS} />
        <Input
          allowClear
          style={{ width: 260 }}
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder="搜索编码 / 名称 / 父级"
        />
        <Button onClick={() => void load()} loading={loading}>
          刷新
        </Button>
        <span style={{ color: '#888' }}>字典变更随版本发布（种子导入），后台只读避免口径漂移。</span>
      </Space>
      <h3>主体类型</h3>
      <DataTable rowKey="code" loading={loading} columns={typeCols} dataSource={types} pagination={false} />
      <h3 style={{ marginTop: 24 }}>
        选项（{DIMENSIONS.find((d) => d.value === dimension)?.label}，共 {filteredOpts.length} 条）
      </h3>
      <DataTable
        rowKey="code"
        loading={loading}
        columns={optCols}
        dataSource={filteredOpts}
        scroll={{ x: 760 }}
        pagination={{ pageSize: 50, showSizeChanger: true, showTotal: (n) => `共 ${n} 条` }}
      />
    </>
  )
}
