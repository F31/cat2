import { useCallback, useEffect, useState } from 'react'
import { Button, Input, InputNumber, Modal, Select, Space, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useNavigate } from 'react-router-dom'
import { adminApi, type AdminUser } from '../api/admin'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '正常', color: 'green' },
  1: { text: '限制', color: 'orange' },
  2: { text: '停用', color: 'red' },
  3: { text: '注销', color: 'default' },
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : '操作失败'
}

// 用户检索与代操作（p4 §4.2/§4.3）：多条件筛选 + 封禁/解封/限制 + 用户 360。
export default function Users() {
  const [rows, setRows] = useState<Array<{
    id: string
    nickname: string
    status: number
    identity_type: number
    realname_status: number
    created_at: number
    member_level: number
    member_active: boolean
  }>>([])
  const [loading, setLoading] = useState(false)
  const [filters, setFilters] = useState<{ q?: string; phone?: string; status?: string; realname?: string; level?: string }>({})
  const [banTarget, setBanTarget] = useState<{ id: string; nickname?: string } | null>(null)
  const [restrictTarget, setRestrictTarget] = useState<{ id: string; nickname?: string } | null>(null)
  const [reason, setReason] = useState('')
  const [days, setDays] = useState(7)
  const [roleTarget, setRoleTarget] = useState<{ id: string; nickname?: string } | null>(null)
  const [roleOptions, setRoleOptions] = useState<Array<{ code: string; name: string }>>([])
  const [selectedRoles, setSelectedRoles] = useState<string[]>([])
  const [isSuper, setIsSuper] = useState(false)
  const [rolesLoading, setRolesLoading] = useState(false)
  const nav = useNavigate()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.governanceUsers({ ...filters, limit: 50 })
      setRows(res.items ?? [])
    } catch (e) {
      message.error(errText(e))
    } finally {
      setLoading(false)
    }
  }, [filters])

  useEffect(() => {
    void load()
  }, [load])

  const unban = async (u: { id: string }) => {
    try {
      await adminApi.unbanUser(u.id)
      message.success('已解封')
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const confirmBan = async () => {
    if (!banTarget) return
    try {
      await adminApi.banUser(banTarget.id, reason)
      message.success('已封禁')
      setBanTarget(null)
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const confirmRestrict = async () => {
    if (!restrictTarget) return
    if (!reason.trim()) {
      message.warning('限制必须填写原因')
      return
    }
    try {
      await adminApi.restrictUser(restrictTarget.id, days, reason)
      message.success(days > 0 ? `已限制 ${days} 天` : '已解除限制')
      setRestrictTarget(null)
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const openRoles = async (u: { id: string; nickname?: string }) => {
    setRoleTarget(u)
    setRolesLoading(true)
    try {
      const [all, cur] = await Promise.all([adminApi.rbacRoles(), adminApi.rbacAdminRoles(u.id)])
      setRoleOptions((all.items ?? []).filter((r) => !r.protected).map((r) => ({ code: r.code, name: r.name })))
      setSelectedRoles(cur.roles ?? [])
      setIsSuper(!!cur.super)
    } catch (e) {
      message.error(errText(e))
    } finally {
      setRolesLoading(false)
    }
  }

  const saveRoles = async () => {
    if (!roleTarget) return
    try {
      await adminApi.rbacAssignRoles(roleTarget.id, selectedRoles)
      message.success('后台角色已更新（下次请求即生效）')
      setRoleTarget(null)
    } catch (e) {
      message.error(errText(e))
    }
  }

  const columns: ColumnsType<(typeof rows)[number]> = [
    { title: 'ID', dataIndex: 'id' },
    { title: '昵称', dataIndex: 'nickname' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: number) => <Tag color={STATUS[s]?.color}>{STATUS[s]?.text ?? s}</Tag>,
    },
    {
      title: '实名',
      dataIndex: 'realname_status',
      render: (v: number) => (v === 1 ? <Tag color="green">已实名</Tag> : <Tag>未实名</Tag>),
    },
    {
      title: '会员',
      dataIndex: 'member_level',
      render: (v: number, r) => (r.member_active && v > 0 ? <Tag color="blue">L{v}</Tag> : '—'),
    },
    {
      title: '注册时间',
      dataIndex: 'created_at',
      render: (t: number) => (t ? new Date(t * 1000).toLocaleString() : ''),
    },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" onClick={() => nav(`/users/${r.id}/360`)}>
            360
          </Button>
          <Button size="small" onClick={() => void openRoles(r)}>
            角色
          </Button>
          <Button
            size="small"
            onClick={() => {
              setRestrictTarget(r)
              setReason('')
              setDays(7)
            }}
          >
            限制
          </Button>
          {r.status === 2 ? (
            <Button size="small" onClick={() => void unban(r)}>
              解封
            </Button>
          ) : (
            <Button
              size="small"
              danger
              onClick={() => {
                setBanTarget(r)
                setReason('')
              }}
            >
              封禁
            </Button>
          )}
        </Space>
      ),
    },
  ]

  return (
    <>
      <Space wrap style={{ marginBottom: 16 }}>
        <Input
          placeholder="昵称模糊"
          style={{ width: 160 }}
          onChange={(e) => setFilters((f) => ({ ...f, q: e.target.value || undefined }))}
          onPressEnter={() => void load()}
        />
        <Input
          placeholder="手机号（精确）"
          style={{ width: 160 }}
          onChange={(e) => setFilters((f) => ({ ...f, phone: e.target.value || undefined }))}
        />
        <Select
          placeholder="状态"
          style={{ width: 120 }}
          allowClear
          options={[
            { value: '0', label: '正常' },
            { value: '1', label: '限制' },
            { value: '2', label: '停用' },
            { value: '3', label: '注销' },
          ]}
          onChange={(v) => setFilters((f) => ({ ...f, status: v || undefined }))}
        />
        <Select
          placeholder="实名"
          style={{ width: 120 }}
          allowClear
          options={[
            { value: '1', label: '已实名' },
            { value: '0', label: '未实名' },
          ]}
          onChange={(v) => setFilters((f) => ({ ...f, realname: v || undefined }))}
        />
        <Select
          placeholder="会员等级"
          style={{ width: 140 }}
          allowClear
          options={[
            { value: '1', label: 'VIP' },
            { value: '2', label: 'SVIP' },
            { value: '3', label: '企业版' },
          ]}
          onChange={(v) => setFilters((f) => ({ ...f, level: v || undefined }))}
        />
        <Button type="primary" onClick={() => void load()}>
          查询
        </Button>
      </Space>
      <Table rowKey="id" loading={loading} columns={columns} dataSource={rows} size="small" />
      <Modal
        title={`限制发布 ${restrictTarget?.nickname ?? ''}`}
        open={!!restrictTarget}
        onOk={() => void confirmRestrict()}
        onCancel={() => setRestrictTarget(null)}
        okText="确认（审计留痕）"
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <InputNumber min={0} max={365} value={days} onChange={(v) => setDays(Number(v) || 0)} addonAfter="天（0=解除）" />
          <Input.TextArea rows={3} placeholder="限制原因（必填）" value={reason} onChange={(e) => setReason(e.target.value)} />
        </Space>
      </Modal>
      <Modal
        title={`后台角色 · ${roleTarget?.nickname ?? roleTarget?.id ?? ''}`}
        open={!!roleTarget}
        onOk={() => void saveRoles()}
        onCancel={() => setRoleTarget(null)}
        okText="保存"
        confirmLoading={rolesLoading}
        okButtonProps={{ disabled: isSuper }}
      >
        {isSuper ? (
          <Space direction="vertical">
            <Tag color="red">引导超管</Tag>
            <span style={{ color: '#888' }}>该账号由运维 `catcube grant-admin` 授予平台超管，隐含全部权限；无需再分配 RBAC 角色。</span>
          </Space>
        ) : (
          <>
            <p style={{ color: '#888' }}>后台角色决定可访问的模块与操作权限；数据范围在「角色与权限」中按角色配置。</p>
            <Select
              mode="multiple"
              style={{ width: '100%' }}
              loading={rolesLoading}
              value={selectedRoles}
              onChange={setSelectedRoles}
              options={roleOptions.map((r) => ({ label: `${r.name}（${r.code}）`, value: r.code }))}
              placeholder="选择后台角色"
            />
          </>
        )}
      </Modal>

      <Modal
        title="封禁用户"
        open={!!banTarget}
        onOk={() => void confirmBan()}
        onCancel={() => setBanTarget(null)}
        okText="确认封禁"
        okButtonProps={{ danger: true }}
      >
        <Input.TextArea
          rows={3}
          placeholder="封禁原因"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </Modal>
    </>
  )
}
