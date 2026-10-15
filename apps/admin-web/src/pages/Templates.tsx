import { useCallback, useEffect, useState } from 'react'
import { Button, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type NoticeTemplate } from '../api/admin'

// 通知模板（只读；编辑在 p4）。展示模块、渠道、是否合并、是否可关闭。
export default function Templates() {
  const [rows, setRows] = useState<NoticeTemplate[]>([])
  const [loading, setLoading] = useState(false)

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
      </Space>
      <Table rowKey="code" loading={loading} columns={columns} dataSource={rows} size="small" />
    </>
  )
}
