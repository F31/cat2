import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Col, Input, InputNumber, message, Row, Space, Table, Tag } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi } from '../api/admin'

interface OverdueAff {
  id: string
  user_id: string
  org_id: string
  role?: string
}

// 代操作（p4 §4.4）：任务重跑、超期归属代处理、会员调整/转移、立方与圈处置、定向公告。
// 全部要求填写原因，服务端写审计（detail 含 before/after 快照）。双人复核项待 RBAC（#13）。
export default function Governance() {
  const [jobs, setJobs] = useState<string[]>([])
  const [overdue, setOverdue] = useState<OverdueAff[]>([])
  const [busy, setBusy] = useState(false)

  // 表单状态
  const [noticeUsers, setNoticeUsers] = useState('')
  const [noticeTitle, setNoticeTitle] = useState('')
  const [noticeContent, setNoticeContent] = useState('')
  const [memberUser, setMemberUser] = useState('')
  const [memberLevel, setMemberLevel] = useState(3)
  const [transferFrom, setTransferFrom] = useState('')
  const [transferTo, setTransferTo] = useState('')
  const [cubeId, setCubeId] = useState('')
  const [circleId, setCircleId] = useState('')
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    try {
      const [j, o] = await Promise.all([adminApi.jobs(), adminApi.overdueAffiliations(7)])
      setJobs(j.items ?? [])
      setOverdue(o.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const wrap = async (fn: () => Promise<unknown>, ok: string) => {
    setBusy(true)
    try {
      await fn()
      message.success(ok)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败')
    } finally {
      setBusy(false)
    }
  }

  const overdueColumns: ColumnsType<OverdueAff> = [
    { title: '归属ID', dataIndex: 'id' },
    { title: '用户', dataIndex: 'user_id' },
    { title: '组织', dataIndex: 'org_id' },
    { title: '职务', dataIndex: 'role', render: (v?: string) => v || '—' },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button
            size="small"
            disabled={busy}
            onClick={() =>
              void wrap(() => adminApi.resolveAffiliation(r.id, true, reason || '组织管理员超期未处理'), '已代处理核验')
            }
          >
            通过
          </Button>
          <Button
            size="small"
            danger
            disabled={busy}
            onClick={() => void wrap(() => adminApi.resolveAffiliation(r.id, false, reason || '材料不足'), '已驳回')}
          >
            驳回
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <Row gutter={16}>
      <Col span={12}>
        <Card title="任务重跑（死信重放/定时任务）">
          <Space wrap>
            {jobs.map((j) => (
              <Button key={j} size="small" disabled={busy} onClick={() => void wrap(() => adminApi.runJob(j), `已触发 ${j}`)}>
                {j}
              </Button>
            ))}
            {jobs.length === 0 ? <Tag>暂无任务</Tag> : null}
          </Space>
        </Card>
        <Card title="超期未核验归属（>7 天，平台代处理）" style={{ marginTop: 16 }}>
          <Input placeholder="代处理原因（可先填）" value={reason} onChange={(e) => setReason(e.target.value)} style={{ marginBottom: 8 }} />
          <Table rowKey="id" columns={overdueColumns} dataSource={overdue} size="small" pagination={false} />
        </Card>
        <Card title="定向公告" style={{ marginTop: 16 }}>
          <Space direction="vertical" style={{ width: '100%' }}>
            <Input placeholder="接收用户 ID（逗号分隔）" value={noticeUsers} onChange={(e) => setNoticeUsers(e.target.value)} />
            <Input placeholder="标题" value={noticeTitle} onChange={(e) => setNoticeTitle(e.target.value)} />
            <Input.TextArea rows={2} placeholder="内容" value={noticeContent} onChange={(e) => setNoticeContent(e.target.value)} />
            <Button
              type="primary"
              disabled={busy}
              onClick={() =>
                void wrap(
                  () =>
                    adminApi.broadcastNotice(
                      noticeUsers.split(',').map((s) => s.trim()).filter(Boolean),
                      noticeTitle,
                      noticeContent,
                      reason || '定向公告',
                    ),
                  '已发送',
                )
              }
            >
              发送（审计）
            </Button>
          </Space>
        </Card>
      </Col>
      <Col span={12}>
        <Card title="会员人工调整（前后值审计；双人复核待 RBAC）">
          <Space wrap>
            <Input placeholder="用户 ID" value={memberUser} onChange={(e) => setMemberUser(e.target.value)} />
            <InputNumber min={1} max={3} value={memberLevel} onChange={(v) => setMemberLevel(Number(v) || 1)} addonBefore="等级" />
            <Button
              type="primary"
              disabled={busy}
              onClick={() => void wrap(() => adminApi.adjustMember(memberUser, memberLevel, reason || '商务补偿'), '已调整')}
            >
              调整
            </Button>
          </Space>
        </Card>
        <Card title="转移企业版主账号" style={{ marginTop: 16 }}>
          <Space wrap>
            <Input placeholder="原主账号 ID" value={transferFrom} onChange={(e) => setTransferFrom(e.target.value)} />
            <Input placeholder="目标账号 ID" value={transferTo} onChange={(e) => setTransferTo(e.target.value)} />
            <Button
              disabled={busy}
              onClick={() => void wrap(() => adminApi.transferMaster(transferFrom, transferTo, reason || '主体变更'), '已转移')}
            >
              转移（审计）
            </Button>
          </Space>
        </Card>
        <Card title="猫立方 / 圈子处置" style={{ marginTop: 16 }}>
          <Space direction="vertical" style={{ width: '100%' }}>
            <Space wrap>
              <Input placeholder="立方 ID" value={cubeId} onChange={(e) => setCubeId(e.target.value)} />
              <Button disabled={busy} onClick={() => void wrap(() => adminApi.adminCubeStatus(cubeId, 'ban', reason || '违规'), '已封禁')}>
                封禁
              </Button>
              <Button danger disabled={busy} onClick={() => void wrap(() => adminApi.adminCubeStatus(cubeId, 'dissolve', reason || '违规解散'), '已解散')}>
                解散
              </Button>
            </Space>
            <Space wrap>
              <Input placeholder="圈子 ID" value={circleId} onChange={(e) => setCircleId(e.target.value)} />
              <Button danger disabled={busy} onClick={() => void wrap(() => adminApi.adminDissolveCircle(circleId, reason || '违规解散'), '已解散')}>
                解散圈子
              </Button>
            </Space>
          </Space>
        </Card>
      </Col>
    </Row>
  )
}
