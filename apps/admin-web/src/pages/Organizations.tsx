import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  TreeSelect,
  Upload,
  message,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { UploadFile } from 'antd/es/upload/interface'
import {
  adminApi,
  type AdminCard,
  type AdminCert,
  type AdminOrg,
  type AdminOrgMember,
  type DictOption,
} from '../api/admin'
import DataTable from '../components/DataTable'
import MediaThumb from '../components/MediaThumb'
import { buildDictTree, dictLabelMap, labelsOf, type TreeNode } from '../utils/dictTree'

const ORG_TYPES = [
  { value: 'cooperation_circle', label: '合作圈' },
  { value: 'association', label: '商协会 / 校友会' },
  { value: 'research', label: '大学 / 科研' },
  { value: 'service_org', label: '服务机构' },
  { value: 'company', label: '企业' },
  { value: 'business_person', label: '商务人士' },
]
const TYPE_LABEL = new Map(ORG_TYPES.map((t) => [t.value, t.label]))
const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '正常', color: 'green' },
  1: { text: '限制', color: 'orange' },
  2: { text: '停用', color: 'red' },
  3: { text: '注销', color: 'default' },
}
const CERT_STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '待审核', color: 'orange' },
  1: { text: '已通过', color: 'green' },
  2: { text: '已驳回', color: 'red' },
}
const CERT_TYPE: Record<number, string> = { 4: '企业认证', 5: '机构认证' }
const MEMBER_STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '待核验', color: 'orange' },
  1: { text: '已核验', color: 'green' },
  2: { text: '已驳回', color: 'red' },
  3: { text: '已移除', color: 'default' },
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : '操作失败'
}

function typeLabel(code?: string): string {
  return code ? TYPE_LABEL.get(code) ?? code : '未分类'
}

function CertTag({ row }: { row: AdminOrg }) {
  if (row.cert_status === undefined) return <Tag>未认证</Tag>
  const s = CERT_STATUS[row.cert_status] ?? { text: String(row.cert_status), color: 'default' }
  return (
    <Tag color={s.color}>
      {CERT_TYPE[row.cert_type ?? 0] ?? '认证'} · {s.text}
    </Tag>
  )
}

// 运营管理 → 组织管理：六类主体、认证申请/审批、成员辅助加入。
export default function Organizations() {
  const [tab, setTab] = useState('all')
  const [q, setQ] = useState('')
  const [status, setStatus] = useState<number | undefined>()
  const [rows, setRows] = useState<AdminOrg[]>([])
  const [pending, setPending] = useState<AdminCert[]>([])
  const [loading, setLoading] = useState(false)

  const [editing, setEditing] = useState<AdminOrg | { id?: string } | null>(null)
  const [form] = Form.useForm()
  const [roleOptions, setRoleOptions] = useState<Array<{ code: string; name: string }>>([])

  const [regionTree, setRegionTree] = useState<TreeNode[]>([])
  const [industryTree, setIndustryTree] = useState<TreeNode[]>([])
  const [needTree, setNeedTree] = useState<TreeNode[]>([])
  const [offerTree, setOfferTree] = useState<TreeNode[]>([])
  const [regionLabels, setRegionLabels] = useState<Map<string, string>>(new Map())
  const [industryLabels, setIndustryLabels] = useState<Map<string, string>>(new Map())

  const [certTarget, setCertTarget] = useState<AdminOrg | null>(null)
  const [certForm] = Form.useForm()
  const [certFiles, setCertFiles] = useState<UploadFile[]>([])

  const [memberTarget, setMemberTarget] = useState<AdminOrg | null>(null)
  const [members, setMembers] = useState<AdminOrgMember[]>([])
  const [memberLoading, setMemberLoading] = useState(false)
  const [memberForm] = Form.useForm()

  const [reviewTarget, setReviewTarget] = useState<{ cert: AdminCert; pass: boolean } | null>(null)
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      if (tab === 'approval') {
        const res = await adminApi.pendingCerts(1)
        setPending((res.items ?? []).filter((c) => c.cert_type === 4 || c.cert_type === 5))
      } else {
        const res = await adminApi.orgs({
          q: q.trim() || undefined,
          category: tab === 'all' ? undefined : tab,
          status,
          limit: 200,
        })
        setRows(res.items ?? [])
      }
    } catch (e) {
      message.error(errText(e))
    } finally {
      setLoading(false)
    }
  }, [tab, q, status])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    void (async () => {
      try {
        const [region, industry, need, offer] = await Promise.all([
          adminApi.dictOptions('region'),
          adminApi.dictOptions('industry'),
          adminApi.dictOptions('need'),
          adminApi.dictOptions('offer'),
        ])
        setRegionTree(buildDictTree(region as DictOption[]))
        setIndustryTree(buildDictTree(industry as DictOption[]))
        setNeedTree(buildDictTree(need as DictOption[]))
        setOfferTree(buildDictTree(offer as DictOption[]))
        setRegionLabels(dictLabelMap(region as DictOption[]))
        setIndustryLabels(dictLabelMap(industry as DictOption[]))
      } catch {
        /* 字典不可用时仍可维护基础信息 */
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

  const openCreate = () => {
    setEditing({})
    form.resetFields()
    form.setFieldsValue({
      category: tab !== 'all' && tab !== 'approval' ? tab : 'company',
      industry_codes: [],
      need_codes: [],
      offer_codes: [],
      role_codes: [],
    })
  }

  const openEdit = async (row: AdminOrg) => {
    let card: AdminCard | null = null
    try {
      card = await adminApi.adminCard(row.id)
    } catch {
      /* 名片不存在时以账号资料回填 */
    }
    let roles: string[] = []
    try {
      const r = await adminApi.rbacAdminRoles(row.id)
      roles = r.roles ?? []
    } catch {
      /* 无权限时保持空 */
    }
    setEditing(row)
    form.setFieldsValue({
      name: card?.name || row.name,
      phone: '',
      category: card?.org_type_code || row.category || 'company',
      region_code: card?.region_code || row.region_code,
      industry_codes: card?.industry_codes ?? row.industry_codes ?? [],
      need_codes: card?.need_codes ?? [],
      offer_codes: card?.offer_codes ?? [],
      core_strength: card?.core_strength ?? '',
      intro: card?.intro ?? '',
      real_name: row.real_name || '',
      id_card_no: '',
      role_codes: roles,
    })
  }

  const save = async () => {
    const v = await form.validateFields()
    const name = String(v.name).trim()
    const phone = String(v.phone ?? '').trim()
    const realName = String(v.real_name ?? '').trim()
    const idCardNo = String(v.id_card_no ?? '').trim()
    if ((realName && !idCardNo) || (!realName && idCardNo)) {
      message.warning('负责人姓名与身份证号需同时填写')
      return
    }
    const regionCode = String(v.region_code ?? '')
    const base = {
      nickname: name,
      phone: phone || undefined,
      identity_type: 2,
      real_name: realName || undefined,
      id_card_no: idCardNo || undefined,
      region_code: regionCode || undefined,
    }
    try {
      let id = (editing as AdminOrg | null)?.id ?? ''
      if (id) {
        await adminApi.updateUser(id, base)
      } else {
        if (!phone) {
          message.warning('新增组织必须填写手机号')
          return
        }
        const created = await adminApi.createUser({ ...base, phone })
        id = created.id
      }
      await adminApi.adminUpdateOrgCard(id, {
        org_type_code: String(v.category),
        name,
        region_code: regionCode,
        industry_codes: (v.industry_codes ?? []) as string[],
        need_codes: (v.need_codes ?? []) as string[],
        offer_codes: (v.offer_codes ?? []) as string[],
        core_strength: String(v.core_strength ?? ''),
        intro: String(v.intro ?? ''),
      })
      if (roleOptions.length > 0) {
        try {
          await adminApi.rbacAssignRoles(id, (v.role_codes ?? []) as string[])
        } catch (e) {
          message.warning(`组织已保存，但角色更新失败：${errText(e)}`)
        }
      }
      message.success(editing && 'id' in editing ? '组织已更新' : '组织已创建')
      setEditing(null)
      form.resetFields()
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const toggleStatus = async (row: AdminOrg) => {
    try {
      if (row.status === 2) {
        await adminApi.unbanUser(row.id)
        message.success('已解禁')
      } else {
        await adminApi.banUser(row.id, '运营停用组织')
        message.success('已停用')
      }
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const remove = async (row: AdminOrg) => {
    try {
      await adminApi.deleteUser(row.id)
      message.success('已删除（软删除，保留历史数据）')
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const openCert = (row: AdminOrg) => {
    setCertTarget(row)
    setCertFiles([])
    certForm.resetFields()
    certForm.setFieldsValue({ cert_type: 4 })
  }

  const customUpload = async (options: Parameters<NonNullable<React.ComponentProps<typeof Upload>['customRequest']>>[0]) => {
    try {
      const fid = await adminApi.uploadAdminMedia(options.file as File)
      options.onSuccess?.({ file_id: fid })
    } catch (e) {
      options.onError?.(e as Error)
      message.error(errText(e))
    }
  }

  const submitCert = async () => {
    if (!certTarget) return
    const v = await certForm.validateFields()
    const done = certFiles.filter((f) => f.status === 'done')
    if (done.length === 0) {
      message.warning('请至少上传 1 张资质材料')
      return
    }
    const ids = done
      .map((f) => Number((f.response as { file_id?: string } | undefined)?.file_id))
      .filter((n) => Number.isFinite(n) && n > 0)
    if (ids.length === 0) {
      message.warning('材料尚未上传完成')
      return
    }
    try {
      await adminApi.submitOrgCert(certTarget.id, {
        cert_type: Number(v.cert_type),
        agreed: true,
        material_file_ids: ids,
        extra: {
          org_code: String(v.org_code ?? '').trim(),
          legal_name: String(v.legal_name ?? '').trim(),
        },
      })
      message.success('认证申请已提交，等待有权限的角色审批')
      setCertTarget(null)
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const openMembers = async (row: AdminOrg) => {
    setMemberTarget(row)
    memberForm.resetFields()
    setMemberLoading(true)
    try {
      const res = await adminApi.orgMembers(row.id)
      setMembers(res.items ?? [])
    } catch (e) {
      message.error(errText(e))
    } finally {
      setMemberLoading(false)
    }
  }

  const addMember = async () => {
    if (!memberTarget) return
    const v = await memberForm.validateFields()
    try {
      await adminApi.addOrgMember(memberTarget.id, {
        user_id: String(v.user_id).trim(),
        role: String(v.role ?? '').trim(),
      })
      message.success('已辅助加入（该用户须已实名）')
      memberForm.resetFields()
      const res = await adminApi.orgMembers(memberTarget.id)
      setMembers(res.items ?? [])
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const removeMember = async (m: AdminOrgMember) => {
    if (!memberTarget) return
    try {
      await adminApi.removeOrgMember(memberTarget.id, m.user_id)
      message.success('已移除成员')
      const res = await adminApi.orgMembers(memberTarget.id)
      setMembers(res.items ?? [])
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const review = async () => {
    if (!reviewTarget) return
    if (!reviewTarget.pass && !reason.trim()) {
      message.warning('驳回必须填写原因')
      return
    }
    try {
      await adminApi.reviewCert(reviewTarget.cert.id, reviewTarget.pass, reason)
      message.success('已处理')
      setReviewTarget(null)
      setReason('')
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const columns: ColumnsType<AdminOrg> = [
    { title: '组织名称', dataIndex: 'name', render: (v: string) => v || '—' },
    { title: '类型', dataIndex: 'category', render: (_: string, r) => r.category_label || typeLabel(r.category) },
    { title: '区域', dataIndex: 'region_code', render: (v?: string) => (v ? regionLabels.get(v) ?? v : '—') },
    { title: '行业', dataIndex: 'industry_codes', render: (v?: string[]) => labelsOf(v, industryLabels) },
    { title: '认证', render: (_, r) => <CertTag row={r} /> },
    {
      title: '成员',
      render: (_, r) => `${r.member_count ?? 0}${r.member_pending ? `（${r.member_pending} 待核验）` : ''}`,
    },
    { title: '管理员', dataIndex: 'admin_count', render: (v?: number) => v ?? 0 },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: number) => <Tag color={STATUS[s]?.color}>{STATUS[s]?.text ?? s}</Tag>,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      render: (t: number) => (t ? new Date(t * 1000).toLocaleString() : '—'),
    },
    {
      title: '操作',
      width: 280,
      render: (_, r) => (
        <Space wrap>
          <Button size="small" onClick={() => void openEdit(r)}>
            编辑
          </Button>
          <Button size="small" onClick={() => openCert(r)}>
            认证
          </Button>
          <Button size="small" onClick={() => void openMembers(r)}>
            成员
          </Button>
          <Button size="small" onClick={() => void toggleStatus(r)}>
            {r.status === 2 ? '解禁' : '停用'}
          </Button>
          <Popconfirm title="确认删除该组织？将软删除并保留历史数据" onConfirm={() => void remove(r)}>
            <Button size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const pendingColumns: ColumnsType<AdminCert> = [
    { title: '认证ID', dataIndex: 'id' },
    { title: '组织/用户ID', dataIndex: 'user_id' },
    { title: '类型', dataIndex: 'cert_type', render: (v: number) => CERT_TYPE[v] ?? v },
    {
      title: '申请信息',
      render: (_, r) => (
        <div style={{ fontSize: 12 }}>
          {r.extra?.org_code ? <div>代码：{r.extra.org_code}</div> : null}
          {r.extra?.legal_name ? <div>负责人：{r.extra.legal_name}</div> : null}
          {!r.extra?.org_code && !r.extra?.legal_name ? '—' : null}
        </div>
      ),
    },
    {
      title: '材料',
      render: (_, r) => (
        <Space wrap>
          {(r.material_file_ids ?? []).length === 0
            ? '—'
            : (r.material_file_ids ?? []).map((id) => <MediaThumb key={id} id={id} />)}
        </Space>
      ),
    },
    {
      title: '提交时间',
      dataIndex: 'submitted_at',
      render: (v: string) => (v ? v.slice(0, 19).replace('T', ' ') : '—'),
    },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" type="primary" onClick={() => setReviewTarget({ cert: r, pass: true })}>
            通过
          </Button>
          <Button size="small" danger onClick={() => setReviewTarget({ cert: r, pass: false })}>
            驳回
          </Button>
        </Space>
      ),
    },
  ]

  const memberColumns: ColumnsType<AdminOrgMember> = [
    { title: '用户ID', dataIndex: 'user_id' },
    { title: '职务', dataIndex: 'role', render: (v?: string) => v || '—' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: number) => <Tag color={MEMBER_STATUS[s]?.color}>{MEMBER_STATUS[s]?.text ?? s}</Tag>,
    },
    {
      title: '操作',
      render: (_, m) =>
        m.status === 3 ? null : (
          <Popconfirm title="确认移除该成员？" onConfirm={() => void removeMember(m)}>
            <Button size="small" danger>
              移除
            </Button>
          </Popconfirm>
        ),
    },
  ]

  const tabItems = [
    { key: 'all', label: '全部' },
    ...ORG_TYPES.map((t) => ({ key: t.value, label: t.label })),
    { key: 'approval', label: '认证审批' },
  ]

  return (
    <>
      <Tabs activeKey={tab} onChange={setTab} items={tabItems} />
      {tab === 'approval' ? (
        <>
          <Space style={{ marginBottom: 16 }}>
            <Button onClick={() => void load()} loading={loading}>
              刷新待审
            </Button>
            <span style={{ color: '#888' }}>审批需要「认证审核」权限；驳回必须填写原因。</span>
          </Space>
          <DataTable rowKey="id" loading={loading} columns={pendingColumns} dataSource={pending} />
        </>
      ) : (
        <>
          <Space style={{ marginBottom: 16 }} wrap>
            <Input
              allowClear
              style={{ width: 220 }}
              placeholder="组织名称 / 负责人"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              onPressEnter={() => void load()}
            />
            <Select
              allowClear
              style={{ width: 140 }}
              placeholder="状态"
              value={status}
              onChange={setStatus}
              options={[
                { value: 0, label: '正常' },
                { value: 2, label: '停用' },
                { value: 3, label: '注销' },
              ]}
            />
            <Button onClick={() => void load()} loading={loading}>
              查询
            </Button>
            <Button type="primary" onClick={openCreate}>
              + 新增组织
            </Button>
          </Space>
          <DataTable
            rowKey="id"
            loading={loading}
            columns={columns}
            dataSource={rows}
            scroll={{ x: 1500 }}
            pagination={{ pageSize: 20, showTotal: (n) => `共 ${n} 个组织` }}
          />
        </>
      )}

      <Modal
        title={editing && 'id' in editing ? '编辑组织' : '新增组织'}
        open={!!editing}
        onOk={() => void save()}
        onCancel={() => setEditing(null)}
        okText="保存"
        width={720}
      >
        <Form form={form} layout="vertical">
          <Space style={{ width: '100%' }} size={16}>
            <Form.Item name="name" label="组织名称" rules={[{ required: true, message: '请输入组织名称' }]} style={{ width: 320 }}>
              <Input maxLength={128} placeholder="组织全称" />
            </Form.Item>
            <Form.Item name="phone" label="手机号" extra={editing && 'id' in editing ? '留空不修改' : '用于登录/联系'} style={{ width: 320 }}>
              <Input maxLength={11} placeholder="11 位手机号" />
            </Form.Item>
          </Space>
          <Space style={{ width: '100%' }} size={16}>
            <Form.Item name="category" label="组织类型" rules={[{ required: true }]} style={{ width: 320 }}>
              <Select options={ORG_TYPES} />
            </Form.Item>
            <Form.Item name="region_code" label="所属区域" style={{ width: 320 }}>
              <TreeSelect treeData={regionTree} showSearch allowClear treeNodeFilterProp="title" placeholder="省 / 市 / 区县" />
            </Form.Item>
          </Space>
          <Form.Item name="industry_codes" label="所属行业">
            <TreeSelect treeData={industryTree} multiple showSearch allowClear treeNodeFilterProp="title" placeholder="可多选" />
          </Form.Item>
          <Space style={{ width: '100%' }} size={16}>
            <Form.Item name="offer_codes" label="主要交付" style={{ width: 320 }}>
              <TreeSelect treeData={offerTree} multiple showSearch allowClear treeNodeFilterProp="title" placeholder="可多选" />
            </Form.Item>
            <Form.Item name="need_codes" label="合作需求" style={{ width: 320 }}>
              <TreeSelect treeData={needTree} multiple showSearch allowClear treeNodeFilterProp="title" placeholder="可多选" />
            </Form.Item>
          </Space>
          <Form.Item name="core_strength" label="核心业务优势">
            <Input.TextArea rows={3} maxLength={2000} />
          </Form.Item>
          <Form.Item name="intro" label="组织简介">
            <Input.TextArea rows={4} maxLength={5000} />
          </Form.Item>
          <Space style={{ width: '100%' }} size={16}>
            <Form.Item name="real_name" label="负责人姓名" style={{ width: 320 }}>
              <Input maxLength={64} />
            </Form.Item>
            <Form.Item name="id_card_no" label="负责人身份证号" extra="与姓名同时填写；留空不修改" style={{ width: 320 }}>
              <Input maxLength={18} />
            </Form.Item>
          </Space>
          {roleOptions.length > 0 ? (
            <Form.Item name="role_codes" label="后台角色">
              <Select
                mode="multiple"
                allowClear
                options={roleOptions.map((r) => ({ label: `${r.name}（${r.code}）`, value: r.code }))}
              />
            </Form.Item>
          ) : null}
        </Form>
      </Modal>

      <Modal
        title={`认证申请 · ${certTarget?.name ?? ''}`}
        open={!!certTarget}
        onOk={() => void submitCert()}
        onCancel={() => setCertTarget(null)}
        okText="提交认证"
        width={620}
      >
        <Form form={certForm} layout="vertical">
          <Form.Item name="cert_type" label="认证类型" rules={[{ required: true }]}>
            <Select options={[{ value: 4, label: '企业认证' }, { value: 5, label: '机构认证' }]} />
          </Form.Item>
          <Form.Item name="org_code" label="统一社会信用代码 / 机构代码">
            <Input maxLength={64} placeholder="企业填统一社会信用代码；机构填机构代码/批文号" />
          </Form.Item>
          <Form.Item name="legal_name" label="法定代表人 / 负责人">
            <Input maxLength={64} />
          </Form.Item>
          <Form.Item label="资质材料（JPG/PNG，最多 3 张）" required>
            <Upload
              multiple
              accept="image/png,image/jpeg"
              maxCount={3}
              fileList={certFiles}
              customRequest={customUpload}
              onChange={({ fileList }) =>
                setCertFiles(
                  fileList.map((f) => ({
                    ...f,
                    url: f.url || (f.originFileObj ? URL.createObjectURL(f.originFileObj as File) : undefined),
                  })),
                )
              }
              listType="picture"
            >
              <Button>上传资质</Button>
            </Upload>
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`组织成员 · ${memberTarget?.name ?? ''}`}
        open={!!memberTarget}
        onCancel={() => setMemberTarget(null)}
        footer={null}
        width={760}
      >
        <Form form={memberForm} layout="inline" style={{ marginBottom: 12 }}>
          <Form.Item name="user_id" rules={[{ required: true, message: '用户ID必填' }]}>
            <Input placeholder="用户 ID（须已实名）" style={{ width: 220 }} />
          </Form.Item>
          <Form.Item name="role">
            <Input placeholder="职务/角色" style={{ width: 160 }} />
          </Form.Item>
          <Button type="primary" onClick={() => void addMember()}>
            辅助加入
          </Button>
        </Form>
        <p style={{ color: '#888' }}>小程序端仅实名人可申请加入；管理端辅助加入会直接核验通过。</p>
        <Table
          rowKey="id"
          loading={memberLoading}
          columns={memberColumns}
          dataSource={members}
          size="small"
          pagination={false}
        />
      </Modal>

      <Modal
        title={reviewTarget?.pass ? '通过认证' : '驳回认证'}
        open={!!reviewTarget}
        onOk={() => void review()}
        onCancel={() => setReviewTarget(null)}
        okText="确认"
        okButtonProps={{ danger: !reviewTarget?.pass }}
      >
        <p>认证ID：{reviewTarget?.cert.id}</p>
        <Input.TextArea
          rows={3}
          placeholder={reviewTarget?.pass ? '备注（可选）' : '驳回原因（必填）'}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </Modal>
    </>
  )
}
