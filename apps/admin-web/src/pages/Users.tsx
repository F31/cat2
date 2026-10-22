import { useCallback, useEffect, useState } from 'react'
import { Button, Form, Input, InputNumber, Modal, Select, Space, Table, Tag, TreeSelect, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useNavigate } from 'react-router-dom'
import { adminApi, type AdminCard, type DictOption } from '../api/admin'
import { buildDictTree, dictLabelMap, labelsOf, type TreeNode } from '../utils/dictTree'

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
    real_name?: string
    id_card_masked?: string
    phone_masked?: string
    region_code?: string
    industry_codes?: string[]
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
  const [editing, setEditing] = useState<{ id?: string; phone_masked?: string; realname_status?: number; card?: AdminCard | null } | null>(null)
  const [regionTree, setRegionTree] = useState<TreeNode[]>([])
  const [industryTree, setIndustryTree] = useState<TreeNode[]>([])
  const [regionLabels, setRegionLabels] = useState<Map<string, string>>(new Map())
  const [industryLabels, setIndustryLabels] = useState<Map<string, string>>(new Map())
  const [userForm] = Form.useForm()
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

  useEffect(() => {
    void (async () => {
      try {
        const [region, industry] = await Promise.all([
          adminApi.dictOptions('region'),
          adminApi.dictOptions('industry'),
        ])
        setRegionTree(buildDictTree(region as DictOption[]))
        setIndustryTree(buildDictTree(industry as DictOption[]))
        setRegionLabels(dictLabelMap(region as DictOption[]))
        setIndustryLabels(dictLabelMap(industry as DictOption[]))
      } catch {
        /* 字典不可用时仍可编辑基础字段 */
      }
    })()
    void (async () => {
      try {
        const all = await adminApi.rbacRoles()
        setRoleOptions((all.items ?? []).filter((r) => !r.protected).map((r) => ({ code: r.code, name: r.name })))
      } catch {
        /* 无 RBAC 管理权限时隐藏角色选择 */
      }
    })()
  }, [])

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

  const openCreate = () => {
    setEditing({ card: null })
    userForm.resetFields()
    userForm.setFieldsValue({ nickname: '', phone: '', identity_type: 1, industry_codes: [], role_codes: [] })
  }

  const openEdit = async (u: { id: string; nickname: string; identity_type: number; phone_masked?: string; realname_status: number; real_name?: string; region_code?: string; industry_codes?: string[] }) => {
    let card: AdminCard | null = null
    try {
      card = await adminApi.adminCard(u.id)
    } catch {
      /* 名片不存在时按账号资料回填 */
    }
    let roles: string[] = []
    try {
      const r = await adminApi.rbacAdminRoles(u.id)
      roles = r.roles ?? []
    } catch {
      /* 无权限时保持空 */
    }
    setEditing({ id: u.id, phone_masked: u.phone_masked, realname_status: u.realname_status, card })
    userForm.setFieldsValue({
      nickname: u.nickname,
      phone: '',
      identity_type: u.identity_type,
      real_name: u.real_name || '',
      id_card_no: '',
      region_code: card?.region_code || u.region_code,
      industry_codes: card?.industry_codes ?? u.industry_codes ?? [],
      role_codes: roles,
    })
  }

  const saveUser = async () => {
    const v = await userForm.validateFields()
    const identityType = Number(v.identity_type)
    const realName = String(v.real_name ?? '').trim()
    const idCardNo = String(v.id_card_no ?? '').trim()
    const verified = editing?.realname_status === 1
    if (!verified && ((realName && !idCardNo) || (!realName && idCardNo))) {
      message.warning('姓名与身份证号需同时填写')
      return
    }
    const regionCode = String(v.region_code ?? '').trim()
    const industryCodes = (v.industry_codes ?? []) as string[]
    const roleCodes = (v.role_codes ?? []) as string[]
    const changeRealname = !verified && !!realName && !!idCardNo
    const base = {
      nickname: String(v.nickname).trim(),
      phone: String(v.phone ?? '').trim() || undefined,
      identity_type: identityType,
      real_name: changeRealname ? realName : undefined,
      id_card_no: changeRealname ? idCardNo : undefined,
      region_code: regionCode || undefined,
    }
    try {
      let id = editing?.id ?? ''
      if (id) {
        await adminApi.updateUser(id, base)
      } else {
        if (!base.phone) {
          message.warning('新增用户必须填写手机号')
          return
        }
        const created = await adminApi.createUser({ ...base, phone: base.phone })
        id = created.id
      }
      const card = editing?.card
      try {
        await adminApi.adminUpdateOrgCard(id, {
          org_type_code: card?.org_type_code || (identityType === 2 ? 'company' : ''),
          name: card?.name || realName || base.nickname,
          region_code: regionCode,
          industry_codes: industryCodes,
          need_codes: card?.need_codes ?? [],
          offer_codes: card?.offer_codes ?? [],
          core_strength: card?.core_strength ?? '',
          intro: card?.intro ?? '',
        })
      } catch (e) {
        message.warning(`账号已保存，但区域/行业更新失败：${errText(e)}`)
      }
      if (roleOptions.length > 0) {
        try {
          await adminApi.rbacAssignRoles(id, roleCodes)
        } catch (e) {
          message.warning(`账号已保存，但角色更新失败：${errText(e)}`)
        }
      }
      message.success(editing?.id ? '用户信息已更新' : '用户已创建')
      setEditing(null)
      userForm.resetFields()
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
    { title: '姓名', dataIndex: 'real_name', render: (v?: string) => v || '—' },
    { title: '手机号', dataIndex: 'phone_masked', render: (v?: string) => v || '—' },
    {
      title: '身份证号',
      dataIndex: 'id_card_masked',
      render: (v?: string) =>
        v ? <span title="证件号已脱敏，仅展示前 4 后 4 位">{v}</span> : '—',
    },
    { title: '区域', dataIndex: 'region_code', render: (v?: string) => (v ? regionLabels.get(v) ?? v : '—') },
    { title: '行业', dataIndex: 'industry_codes', render: (v?: string[]) => labelsOf(v, industryLabels) },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: number) => <Tag color={STATUS[s]?.color}>{STATUS[s]?.text ?? s}</Tag>,
    },
    {
      title: '实名',
      dataIndex: 'realname_status',
      render: (v: number) =>
        v === 1 ? (
          <Tag color="green">已实名</Tag>
        ) : v === 2 ? (
          <Tag color="red">核验失败</Tag>
        ) : (
          <Tag>未实名</Tag>
        ),
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
          <Button size="small" onClick={() => openEdit(r)}>
            编辑
          </Button>
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
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, marginBottom: 16 }}>
      <Space wrap>
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
      <Button type="primary" onClick={openCreate}>
        + 新增用户
      </Button>
      </div>
      <Table
        rowKey="id"
        loading={loading}
        columns={columns}
        dataSource={rows}
        size="small"
        scroll={{ x: 1800 }}
      />
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
        title={editing?.id ? '编辑用户' : '新增用户'}
        open={!!editing}
        onOk={() => void saveUser()}
        onCancel={() => setEditing(null)}
        okText="保存"
      >
        <Form form={userForm} layout="vertical">
          <Form.Item name="nickname" label="昵称" rules={[{ required: true, message: '请输入昵称' }]}>
            <Input maxLength={32} placeholder="昵称" />
          </Form.Item>
          <Form.Item
            name="real_name"
            label="姓名"
            extra={editing?.realname_status === 1 ? '已实名，姓名不可直接修改' : '与身份证号同时填写；留空不修改'}
          >
            <Input maxLength={64} placeholder="真实姓名" disabled={editing?.realname_status === 1} />
          </Form.Item>
          <Form.Item
            name="id_card_no"
            label="身份证号"
            extra={
              editing?.realname_status === 1
                ? '已实名，证件号不可直接修改'
                : editing?.id
                  ? '留空不修改；填写则需同时填写姓名'
                  : '选填；填写则需同时填写姓名'
            }
          >
            <Input maxLength={18} placeholder="18 位身份证号" disabled={editing?.realname_status === 1} />
          </Form.Item>
          <Form.Item
            name="phone"
            label="手机号"
            rules={editing?.id ? [] : [{ required: true, message: '请输入手机号' }]}
            extra={editing?.id ? `当前：${editing.phone_masked || '—'}；留空则不修改` : '手机号全局唯一'}
          >
            <Input maxLength={11} placeholder={editing?.id ? '留空不修改' : '11 位手机号'} />
          </Form.Item>
          <Form.Item name="identity_type" label="主体类型" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 1, label: '个人' },
                { value: 2, label: '组织' },
              ]}
            />
          </Form.Item>
          <Form.Item name="region_code" label="所属区域">
            <TreeSelect
              treeData={regionTree}
              showSearch
              allowClear
              treeNodeFilterProp="title"
              placeholder="省 / 市 / 区县"
              style={{ width: '100%' }}
            />
          </Form.Item>
          <Form.Item name="industry_codes" label="所属行业">
            <TreeSelect
              treeData={industryTree}
              multiple
              showSearch
              allowClear
              treeNodeFilterProp="title"
              placeholder="可多选"
              style={{ width: '100%' }}
            />
          </Form.Item>
          {roleOptions.length > 0 ? (
            <Form.Item name="role_codes" label="后台角色">
              <Select
                mode="multiple"
                allowClear
                placeholder="选择后台角色"
                options={roleOptions.map((r) => ({ label: `${r.name}（${r.code}）`, value: r.code }))}
              />
            </Form.Item>
          ) : null}
        </Form>
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
