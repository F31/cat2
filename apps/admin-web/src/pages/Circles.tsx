import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Col, Descriptions, Drawer, Input, Modal, Row, Select, Space, Statistic, Table, Tabs, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi, type AdminCircle } from '../api/admin'
import DataTable from '../components/DataTable'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '进行中', color: 'green' },
  1: { text: '已结束', color: 'default' },
  2: { text: '已解散', color: 'red' },
  3: { text: '已封禁', color: 'volcano' },
  4: { text: '已归档', color: 'purple' },
}

type Action = 'ban' | 'archive' | 'unban' | 'restore'
const ACTION_LABEL: Record<Action, string> = { ban: '批量封禁', archive: '批量归档', unban: '批量解封', restore: '批量恢复' }

const fmt = (v?: number) => (v ? new Date(v * 1000).toLocaleString() : '—')

// 广场 → 圈子管理：列表/批量处置 + 详情（成员/动态/需求/挂载）+ 内容处置。
export default function Circles() {
  const [rows, setRows] = useState<AdminCircle[]>([])
  const [loading, setLoading] = useState(false)
  const [q, setQ] = useState('')
  const [status, setStatus] = useState<number | undefined>()
  const [selected, setSelected] = useState<string[]>([])
  const [action, setAction] = useState<Action | null>(null)
  const [reason, setReason] = useState('')
  const [stats, setStats] = useState<Awaited<ReturnType<typeof adminApi.circleStats>> | null>(null)
  const [detail, setDetail] = useState<Awaited<ReturnType<typeof adminApi.circleDetail>> | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [removeTarget, setRemoveTarget] = useState<{ kind: 'demands' | 'feeds' | 'mounts'; id: string; label: string } | null>(null)
  const [removeReason, setRemoveReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [res, st] = await Promise.all([adminApi.circles(q, status), adminApi.circleStats(30)])
      setRows(res.items ?? [])
      setStats(st)
      setSelected([])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [q, status])

  useEffect(() => {
    void load()
  }, [load])

  const submit = async () => {
    if (!action) return
    if (!reason.trim()) {
      message.warning('必须填写原因')
      return
    }
    try {
      const r = await adminApi.batchCircleStatus(action, selected, reason)
      if (r.failed > 0) {
        const fails = (r.items ?? []).filter((i) => !i.ok).map((i) => `${i.id}:${i.error ?? '失败'}`).slice(0, 3)
        message.warning(`成功 ${r.succeeded}，失败 ${r.failed}（${fails.join('；')}）`)
      } else {
        message.success(`已处理 ${r.succeeded} 个圈子`)
      }
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
      setDetail(await adminApi.circleDetail(id))
    } catch (e) {
      message.error(e instanceof Error ? e.message : '详情加载失败')
    } finally {
      setDetailLoading(false)
    }
  }

  const submitRemove = async () => {
    if (!removeTarget || !detail) return
    if (!removeReason.trim()) {
      message.warning('必须填写原因')
      return
    }
    try {
      await adminApi.removeCircleContent(detail.circle.id, removeTarget.kind, removeTarget.id, removeReason.trim())
      message.success('已删除并留痕')
      setRemoveTarget(null)
      setRemoveReason('')
      await openDetail(detail.circle.id)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '删除失败')
    }
  }

  const removeBtn = (kind: 'demands' | 'feeds' | 'mounts', id: string, label: string) => (
    <Button size="small" danger onClick={() => { setRemoveTarget({ kind, id, label }); setRemoveReason('') }}>
      删除
    </Button>
  )

  const columns: ColumnsType<AdminCircle> = [
    { title: '圈子ID', dataIndex: 'id' },
    { title: '名称', dataIndex: 'name' },
    { title: '类型', dataIndex: 'type', render: (v: number) => (v === 1 ? '常设' : '临时') },
    { title: '圈主', dataIndex: 'owner_user_id' },
    { title: '成员数', dataIndex: 'member_count' },
    { title: '区域', dataIndex: 'region_code', render: (v?: string) => v || '—' },
    { title: '创建时间', dataIndex: 'created_at', render: fmt },
    { title: '状态', dataIndex: 'status', render: (v: number) => <Tag color={STATUS[v]?.color}>{STATUS[v]?.text ?? v}</Tag> },
    {
      title: '操作',
      width: 90,
      render: (_, r) => (
        <Button size="small" onClick={() => void openDetail(r.id)}>
          详情
        </Button>
      ),
    },
  ]

  return (
    <>
      {stats ? (
        <Row gutter={16} style={{ marginBottom: 16 }}>
          <Col flex="1"><Card><Statistic title="新增圈数（30天）" value={stats.new_total} /></Card></Col>
          <Col flex="1"><Card><Statistic title="常设 / 临时" value={`${stats.new_permanent} / ${stats.new_temporary}`} /></Card></Col>
          <Col flex="1"><Card><Statistic title="活跃圈（30天）" value={stats.active_circles} /></Card></Col>
          <Col flex="1"><Card><Statistic title="成员总数" value={stats.members_total} /></Card></Col>
          <Col flex="1"><Card><Statistic title="需求发布（30天）" value={stats.demands_total} suffix={`有效率 ${stats.demand_valid_rate.toFixed(0)}%`} /></Card></Col>
        </Row>
      ) : null}
      <Space wrap style={{ marginBottom: 16 }}>
        <Input.Search placeholder="名称 / ID" style={{ width: 220 }} allowClear onSearch={setQ} />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 140 }}
          value={status}
          onChange={setStatus}
          options={Object.entries(STATUS).map(([v, s]) => ({ value: Number(v), label: s.text }))}
        />
        <Button onClick={() => void load()}>刷新</Button>
        <Button danger disabled={selected.length === 0} onClick={() => setAction('ban')}>批量封禁</Button>
        <Button disabled={selected.length === 0} onClick={() => setAction('archive')}>批量归档</Button>
        <Button disabled={selected.length === 0} onClick={() => setAction('unban')}>批量解封</Button>
        <Button disabled={selected.length === 0} onClick={() => setAction('restore')}>批量恢复</Button>
        {selected.length > 0 ? <Tag color="blue">已选 {selected.length}</Tag> : null}
      </Space>
      <DataTable
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={rows}
        rowSelection={{ selectedRowKeys: selected, onChange: (keys) => setSelected(keys as string[]) }}
      />

      <Modal
        title={`${action ? ACTION_LABEL[action] : ''}（${selected.length} 个）`}
        open={!!action}
        onOk={() => void submit()}
        onCancel={() => setAction(null)}
        okText="确认"
        okButtonProps={{ danger: action === 'ban' }}
      >
        <p style={{ color: '#888' }}>
          封禁：前台隐藏、禁止加入/发布，可解封；归档：后台隐藏、只读，可恢复；原因逐圈写入审计。
        </p>
        <Input.TextArea rows={3} placeholder="原因（必填）" value={reason} onChange={(e) => setReason(e.target.value)} />
      </Modal>

      <Drawer
        title={`圈子详情 · ${detail?.circle.name ?? ''}`}
        width={900}
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
                    <Descriptions.Item label="圈子 ID">{detail.circle.id}</Descriptions.Item>
                    <Descriptions.Item label="状态"><Tag color={STATUS[detail.circle.status]?.color}>{STATUS[detail.circle.status]?.text}</Tag></Descriptions.Item>
                    <Descriptions.Item label="名称">{detail.circle.name}</Descriptions.Item>
                    <Descriptions.Item label="类型">{detail.circle.type === 1 ? '常设' : '临时'}</Descriptions.Item>
                    <Descriptions.Item label="圈主">{detail.owner_name || detail.circle.owner_user_id}</Descriptions.Item>
                    <Descriptions.Item label="成员数">{detail.circle.member_count}</Descriptions.Item>
                    <Descriptions.Item label="区域">{detail.circle.region_code || '—'}</Descriptions.Item>
                    <Descriptions.Item label="创建时间">{fmt(detail.circle.created_at)}</Descriptions.Item>
                    <Descriptions.Item label="简介" span={2}>{detail.circle.intro || '—'}</Descriptions.Item>
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
                    { title: '加入方式', dataIndex: 'joined_via', render: (v: number) => ({ 1: '申请', 2: '邀请', 3: '组织', 4: '活动' } as Record<number, string>)[v] ?? v },
                    { title: '加入时间', dataIndex: 'joined_at', render: fmt },
                  ]} />
                ),
              },
              {
                key: 'feeds',
                label: `动态（${detail.feeds.length}）`,
                children: (
                  <Table rowKey="id" size="small" dataSource={detail.feeds} pagination={{ pageSize: 8 }} columns={[
                    { title: 'ID', dataIndex: 'id' },
                    { title: '作者', dataIndex: 'author_name', render: (v: string, r) => v || r.author_id },
                    { title: '内容', dataIndex: 'text', ellipsis: true },
                    { title: '公告', dataIndex: 'is_announcement', render: (v: boolean) => (v ? <Tag color="blue">公告</Tag> : '—') },
                    { title: '时间', dataIndex: 'created_at', render: fmt },
                    { title: '操作', render: (_, r) => removeBtn('feeds', r.id, r.text) },
                  ]} />
                ),
              },
              {
                key: 'demands',
                label: `需求（${detail.demands.length}）`,
                children: (
                  <Table rowKey="id" size="small" dataSource={detail.demands} pagination={{ pageSize: 8 }} columns={[
                    { title: 'ID', dataIndex: 'id' },
                    { title: '作者', dataIndex: 'author_name', render: (v: string, r) => v || r.author_id },
                    { title: '标题', dataIndex: 'title' },
                    { title: '内容', dataIndex: 'content', ellipsis: true },
                    { title: '时间', dataIndex: 'created_at', render: fmt },
                    { title: '操作', render: (_, r) => removeBtn('demands', r.id, r.title) },
                  ]} />
                ),
              },
              {
                key: 'mounts',
                label: `挂载（${detail.mounts.length}）`,
                children: (
                  <Table rowKey="id" size="small" dataSource={detail.mounts} pagination={{ pageSize: 8 }} columns={[
                    { title: 'ID', dataIndex: 'id' },
                    { title: '内容引用', dataIndex: 'item_ref', render: (v: string) => <code>{v}</code> },
                    { title: '类型', dataIndex: 'mount_type', render: (v: number) => (v === 1 ? '本人' : '申请') },
                    { title: '状态', dataIndex: 'approval_status', render: (v: number) => ({ 0: '待确认', 1: '已同意', 2: '已拒绝' } as Record<number, string>)[v] ?? v },
                    { title: '时间', dataIndex: 'created_at', render: fmt },
                    { title: '操作', render: (_, r) => removeBtn('mounts', r.id, r.item_ref) },
                  ]} />
                ),
              },
            ]}
          />
        ) : null}
      </Drawer>

      <Modal
        title={`删除内容 · ${removeTarget?.label ?? ''}`}
        open={!!removeTarget}
        onOk={() => void submitRemove()}
        onCancel={() => setRemoveTarget(null)}
        okText="删除"
        okButtonProps={{ danger: true }}
      >
        <p style={{ color: '#888' }}>删除后将从前台隐藏，并写入审计（原因必填）。</p>
        <Input.TextArea rows={3} placeholder="删除原因" value={removeReason} onChange={(e) => setRemoveReason(e.target.value)} />
      </Modal>
    </>
  )
}
