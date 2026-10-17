import { useCallback, useEffect, useState } from 'react'
import { Button, Input, Modal, Space, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type ReviewTask } from '../api/admin'
import DataTable from './DataTable'

const SOURCE: Record<number, string> = { 1: '机器命中', 2: '举报', 3: '认证材料', 4: '产品入库', 5: '高风险抽检' }
const errText = (e: unknown) => (e instanceof Error ? e.message : '操作失败')

/** 审核队列共享组件（按 SLA 到期排序；通过无需原因，驳回必填原因）。 */
export default function ReviewQueue() {
  const [rows, setRows] = useState<ReviewTask[]>([])
  const [loading, setLoading] = useState(false)
  const [action, setAction] = useState<{ task: ReviewTask; confirmed: boolean } | null>(null)
  const [reason, setReason] = useState('')
  const [sug, setSug] = useState<{ id: string; text: string } | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.reviewTasks()
      setRows(res.items ?? [])
    } catch (e) {
      message.error(errText(e))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const open = (task: ReviewTask, confirmed: boolean) => {
    setAction({ task, confirmed })
    setReason('')
  }

  const confirm = async () => {
    if (!action) return
    if (!action.confirmed && !reason.trim()) {
      message.warning('驳回必须填写原因')
      return
    }
    try {
      await adminApi.resolveReview(action.task.id, action.confirmed, reason)
      message.success('已处理')
      setAction(null)
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const askAI = async (task: ReviewTask) => {
    try {
      const r = await adminApi.reviewSuggestion(task.id)
      setSug({
        id: task.id,
        text: r.available
          ? `分类：${r.category ?? '—'}｜风险：${r.risk ?? 0}/3｜摘要：${r.summary ?? ''}`
          : 'AI 未启用或暂不可用（不影响审核）',
      })
    } catch (e) {
      message.error(errText(e))
    }
  }

  const columns: ColumnsType<ReviewTask> = [
    { title: 'ID', dataIndex: 'id' },
    { title: '来源', dataIndex: 'source', render: (s: number) => SOURCE[s] ?? s },
    { title: '对象类型', dataIndex: 'subject_type' },
    { title: '对象ID', dataIndex: 'subject_id' },
    { title: '风险等级', dataIndex: 'risk_level' },
    { title: 'SLA', dataIndex: 'sla_overdue', render: (v: boolean) => (v ? <Tag color="red">超时</Tag> : <Tag>正常</Tag>) },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" type="primary" onClick={() => open(r, true)}>
            通过
          </Button>
          <Button size="small" danger onClick={() => open(r, false)}>
            驳回
          </Button>
          <Button size="small" onClick={() => void askAI(r)}>
            AI 建议
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void load()}>刷新队列</Button>
      </Space>
      <DataTable rowKey="id" loading={loading} columns={columns} dataSource={rows} />
      {sug ? (
        <Modal title={`AI 建议（任务 ${sug.id}）`} open onCancel={() => setSug(null)} footer={null}>
          {sug.text}
        </Modal>
      ) : null}
      <Modal
        title={action?.confirmed ? '通过审核' : '驳回审核'}
        open={!!action}
        onOk={() => void confirm()}
        onCancel={() => setAction(null)}
        okText="确认"
        okButtonProps={{ danger: !action?.confirmed }}
      >
        <Input.TextArea
          rows={3}
          placeholder={action?.confirmed ? '备注（可选）' : '驳回原因（必填）'}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </Modal>
    </>
  )
}
