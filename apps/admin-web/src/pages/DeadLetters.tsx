import { useCallback, useEffect, useState } from 'react'
import { Button, Space, Table, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type DeadLetter } from '../api/admin'

// 死信队列（p1 §6.2）：查看投递失败事件并重放（复位待投递）。
export default function DeadLetters() {
  const [rows, setRows] = useState<DeadLetter[]>([])
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.deadLetters()
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

  const replay = async (eventId: string) => {
    try {
      await adminApi.replayDeadLetter(eventId)
      message.success('已重放')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '重放失败')
    }
  }

  const columns: ColumnsType<DeadLetter> = [
    { title: '事件 ID', dataIndex: 'event_id' },
    { title: 'Topic', dataIndex: 'topic' },
    { title: '发生时间', dataIndex: 'occurred_at', render: (t: string) => (t ? t.slice(0, 19).replace('T', ' ') : '') },
    {
      title: '操作',
      render: (_, r) => (
        <Button size="small" type="primary" onClick={() => void replay(r.event_id)}>
          重放
        </Button>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      <Table rowKey="event_id" loading={loading} columns={columns} dataSource={rows} size="small" />
    </>
  )
}
