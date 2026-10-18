import { useCallback, useEffect, useState } from 'react'
import { Button, Collapse, Descriptions, Drawer, Empty, Input, Modal, Space, Table, Tabs, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminCube } from '../api/admin'
import DataTable from '../components/DataTable'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '正常', color: 'green' },
  1: { text: '已封禁', color: 'red' },
  2: { text: '已解散', color: 'default' },
}
const PLAN: Record<number, string> = { 0: '未订阅', 1: '基础版', 2: '专业版', 3: '旗舰版' }
const ROLE: Record<number, { text: string; color: string }> = {
  1: { text: '猫王', color: 'gold' },
  2: { text: '猫蜜', color: 'purple' },
  3: { text: '成员', color: 'default' },
}
const fmt = (v?: number | string) => {
  if (!v) return '—'
  if (typeof v === 'string') return v.slice(0, 19).replace('T', ' ')
  return new Date(v * 1000).toLocaleString()
}

// 广场 → 猫立方管理：列表/封禁/解散 + 详情（成员/猫王猫蜜调整/项目/群聊合规查看）。
export default function Cubes() {
  const [rows, setRows] = useState<AdminCube[]>([])
  const [loading, setLoading] = useState(false)
  const [q, setQ] = useState('')
  const [action, setAction] = useState<{ cube: AdminCube; kind: 'ban' | 'dissolve' } | null>(null)
  const [reason, setReason] = useState('')
  const [detail, setDetail] = useState<Awaited<ReturnType<typeof adminApi.cubeDetail>> | null>(null)
  const [chats, setChats] = useState<Awaited<ReturnType<typeof adminApi.cubeChats>> | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [removeTarget, setRemoveTarget] = useState<{ uid: string; name: string } | null>(null)
  const [removeReason, setRemoveReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.cubes(q)
      setRows(res.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [q])

  useEffect(() => {
    void load()
  }, [load])

  const confirm = async () => {
    if (!action) return
    if (!reason.trim()) {
      message.warning('必须填写原因')
      return
    }
    try {
      await adminApi.adminCubeStatus(action.cube.id, action.kind, reason)
      message.success(action.kind === 'ban' ? '已封禁' : '已解散')
      setAction(null)
      setReason('')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败')
    }
  }

  const openDetail = async (id: string) => {
    setDetailLoading(true)
    try {
      const [d, c] = await Promise.all([adminApi.cubeDetail(id), adminApi.cubeChats(id)])
      setDetail(d)
      setChats(c)
    } catch (e) {
      message.error(e instanceof Error ? e.message : '详情加载失败')
    } finally {
      setDetailLoading(false)
    }
  }

  const refreshDetail = async () => {
    if (!detail) return
    await openDetail(detail.cube.id)
    void load()
  }

  const setLeader = (uid: string, name: string) => {
    if (!detail) return
    Modal.confirm({
      title: `将 ${name || uid} 设为猫王？`,
      content: '原猫王将转为普通成员；变更立即生效并写审计。',
      onOk: async () => {
        await adminApi.cubeSetLeader(detail.cube.id, uid)
        message.success('已调整猫王')
        await refreshDetail()
      },
    })
  }

  const setHoney = (uid: string, name: string) => {
    if (!detail) return
    Modal.confirm({
      title: `将 ${name || uid} 设为猫蜜？`,
      content: '原猫蜜将转为普通成员；变更立即生效并写审计。',
      onOk: async () => {
        await adminApi.cubeSetHoney(detail.cube.id, uid)
        message.success('已调整猫蜜')
        await refreshDetail()
      },
    })
  }

  const submitRemove = async () => {
    if (!detail || !removeTarget) return
    if (!removeReason.trim()) {
      message.warning('必须填写原因')
      return
    }
    try {
      await adminApi.cubeRemoveMember(detail.cube.id, removeTarget.uid, removeReason.trim())
      message.success('已移除成员')
      setRemoveTarget(null)
      setRemoveReason('')
      await refreshDetail()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '移除失败')
    }
  }

  const columns: ColumnsType<AdminCube> = [
    { title: '立方ID', dataIndex: 'id' },
    { title: '名称', dataIndex: 'name' },
    { title: '猫王', dataIndex: 'leader_id' },
    { title: '猫蜜', dataIndex: 'honey_id', render: (v?: string) => v || '—' },
    { title: '订阅档位', dataIndex: 'plan_level', render: (v: number) => PLAN[v] ?? v },
    { title: '成员数', dataIndex: 'member_count' },
    { title: '项目数', dataIndex: 'project_count', render: (v?: number) => v ?? 0 },
    { title: '创建时间', dataIndex: 'created_at', render: (v?: string) => (v ? v.slice(0, 10) : '—') },
    { title: '区域', dataIndex: 'region_code', render: (v?: string) => v || '—' },
    { title: '状态', dataIndex: 'status', render: (v: number) => <Tag color={STATUS[v]?.color}>{STATUS[v]?.text ?? v}</Tag> },
    {
      title: '操作',
      width: 180,
      render: (_, r) => (
        <Space>
          <Button size="small" onClick={() => void openDetail(r.id)}>详情</Button>
          <Button size="small" danger disabled={r.status !== 0} onClick={() => setAction({ cube: r, kind: 'ban' })}>封禁</Button>
          <Button size="small" danger disabled={r.status === 2} onClick={() => setAction({ cube: r, kind: 'dissolve' })}>解散</Button>
        </Space>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Input.Search placeholder="名称 / ID" style={{ width: 240 }} allowClear onSearch={setQ} />
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      <DataTable rowKey="id" loading={loading} columns={columns} dataSource={rows} />

      <Modal
        title={action?.kind === 'ban' ? `封禁立方 ${action?.cube.id}` : `解散立方 ${action?.cube.id}`}
        open={!!action}
        onOk={() => void confirm()}
        onCancel={() => setAction(null)}
        okText="确认"
        okButtonProps={{ danger: true }}
      >
        <Input.TextArea rows={3} placeholder="原因（必填，写入审计）" value={reason} onChange={(e) => setReason(e.target.value)} />
      </Modal>

      <Drawer
        title={`猫立方详情 · ${detail?.cube.name ?? ''}`}
        width={960}
        open={!!detail}
        loading={detailLoading}
        onClose={() => setDetail(null)}
      >
        {detail ? (
          <Tabs
            items={[
              {
                key: 'info',
                label: '基本信息',
                children: (
                  <Descriptions column={2} size="small" bordered>
                    <Descriptions.Item label="立方 ID">{detail.cube.id}</Descriptions.Item>
                    <Descriptions.Item label="状态"><Tag color={STATUS[detail.cube.status]?.color}>{STATUS[detail.cube.status]?.text}</Tag></Descriptions.Item>
                    <Descriptions.Item label="名称">{detail.cube.name}</Descriptions.Item>
                    <Descriptions.Item label="订阅档位">{PLAN[detail.cube.plan_level] ?? detail.cube.plan_level}</Descriptions.Item>
                    <Descriptions.Item label="猫王">{detail.cube.leader_id}</Descriptions.Item>
                    <Descriptions.Item label="猫蜜">{detail.cube.honey_id || '—'}</Descriptions.Item>
                    <Descriptions.Item label="成员数">{detail.cube.member_count}</Descriptions.Item>
                    <Descriptions.Item label="区域">{detail.cube.region_code || '—'}</Descriptions.Item>
                    <Descriptions.Item label="行业">{detail.cube.industry_codes?.join(' / ') || '—'}</Descriptions.Item>
                    <Descriptions.Item label="阶段/方向">{`${detail.cube.chain_stage || '—'} / ${detail.cube.direction || '—'}`}</Descriptions.Item>
                    <Descriptions.Item label="资源">{detail.cube.resources || '—'}</Descriptions.Item>
                    <Descriptions.Item label="优势">{detail.cube.strength || '—'}</Descriptions.Item>
                    <Descriptions.Item label="合作诉求" span={2}>{detail.cube.cooperation || '—'}</Descriptions.Item>
                  </Descriptions>
                ),
              },
              {
                key: 'members',
                label: `成员（${detail.members.length}）`,
                children: (
                  <Table rowKey="user_id" size="small" dataSource={detail.members} pagination={{ pageSize: 10 }} columns={[
                    { title: '用户 ID', dataIndex: 'user_id' },
                    { title: '昵称', dataIndex: 'nickname' },
                    { title: '角色', dataIndex: 'role', render: (v: number) => <Tag color={ROLE[v]?.color}>{ROLE[v]?.text ?? v}</Tag> },
                    { title: '加入时间', dataIndex: 'joined_at', render: fmt },
                    {
                      title: '操作',
                      render: (_, r) => (
                        <Space>
                          <Button size="small" disabled={r.role === 1} onClick={() => setLeader(r.user_id, r.nickname)}>设为猫王</Button>
                          <Button size="small" disabled={r.role === 1 || r.role === 2} onClick={() => setHoney(r.user_id, r.nickname)}>设为猫蜜</Button>
                          <Button size="small" danger disabled={r.role === 1 || r.role === 2} onClick={() => { setRemoveTarget({ uid: r.user_id, name: r.nickname }); setRemoveReason('') }}>移除</Button>
                        </Space>
                      ),
                    },
                  ]} />
                ),
              },
              {
                key: 'projects',
                label: `项目（${detail.projects.length}）`,
                children: (
                  <Table rowKey="id" size="small" dataSource={detail.projects} pagination={{ pageSize: 10 }} columns={[
                    { title: '项目 ID', dataIndex: 'id' },
                    { title: '名称', dataIndex: 'name' },
                    { title: '类型', dataIndex: 'type', render: (v: number) => (v === 1 ? '研究' : '承接') },
                    { title: '进度', dataIndex: 'progress', render: (v: number) => `${v}%` },
                    { title: '状态', dataIndex: 'status', render: (v: number) => (v === 0 ? <Tag color="green">进行中</Tag> : <Tag>已关闭</Tag>) },
                    { title: '创建时间', dataIndex: 'created_at', render: fmt },
                  ]} />
                ),
              },
              {
                key: 'chats',
                label: `群聊记录（${chats?.items.length ?? 0}）`,
                children: chats?.items.length ? (
                  <Collapse
                    items={chats.items.map((s) => ({
                      key: s.session_id,
                      label: `${s.title || s.biz_ref}（${s.messages.length} 条）`,
                      children: s.messages.length ? (
                        <Table rowKey="id" size="small" pagination={{ pageSize: 8 }} dataSource={s.messages} columns={[
                          { title: '时间', dataIndex: 'created_at', render: fmt, width: 180 },
                          { title: '发送者', dataIndex: 'sender_name', render: (v: string, r) => v || r.sender_id, width: 160 },
                          { title: '内容', dataIndex: 'content' },
                        ]} />
                      ) : <Empty description="暂无消息" />,
                    }))}
                  />
                ) : <Empty description="暂无群聊记录" />,
              },
            ]}
          />
        ) : null}
      </Drawer>

      <Modal
        title={`移除成员 · ${removeTarget?.name ?? removeTarget?.uid ?? ''}`}
        open={!!removeTarget}
        onOk={() => void submitRemove()}
        onCancel={() => setRemoveTarget(null)}
        okText="移除"
        okButtonProps={{ danger: true }}
      >
        <p style={{ color: '#888' }}>移除后该成员将退出立方并收到通知；原因必填并写入审计。</p>
        <Input.TextArea rows={3} placeholder="移除原因" value={removeReason} onChange={(e) => setRemoveReason(e.target.value)} />
      </Modal>
    </>
  )
}
