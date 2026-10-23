import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Button,
  Descriptions,
  Drawer,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Steps,
  Table,
  Tag,
  Upload,
  message,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { UploadFile, UploadProps } from 'antd/es/upload/interface'
import {
  adminApi,
  type AdminContent,
  type AdminOrg,
  type DictOption,
} from '../api/admin'

const STATUS: Record<number, { text: string; color: string }> = {
  0: { text: '审核中', color: 'orange' },
  1: { text: '展示中', color: 'green' },
  2: { text: '已下架', color: 'default' },
}

const FIELD_LABELS: Record<string, string> = {
  carousel_file_ids: '产品图片',
  target_customers: '目标客户',
  target_channels: '目标渠道',
  core_strength: '核心优势',
  key_points: '关键点描述',
  intro: '产品简介',
  contact_phone: '合作联系',
}

type ProductForm = {
  title: string
  region_code?: string
  industry_codes?: string[]
  need_codes?: string[]
  offer_codes?: string[]
  target_customers?: string
  target_channels?: string
  core_strength?: string
  key_points?: string
  intro?: string
  contact_phone?: string
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : '操作失败'
}

function optLabel(options: DictOption[], code?: string): string {
  return options.find((o) => o.code === code)?.label ?? code ?? '—'
}

// 产品库：列表 / 三步入库 / 审核 / 详情。
export default function Products() {
  const [rows, setRows] = useState<AdminContent[]>([])
  const [loading, setLoading] = useState(false)
  const [status, setStatus] = useState<number | undefined>()

  const [createOpen, setCreateOpen] = useState(false)
  const [step, setStep] = useState(0)
  const [orgQ, setOrgQ] = useState('')
  const [orgs, setOrgs] = useState<AdminOrg[]>([])
  const [orgId, setOrgId] = useState('')
  const [selectedOrg, setSelectedOrg] = useState<AdminOrg | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<ProductForm>()
  const [fileList, setFileList] = useState<UploadFile[]>([])

  const [dicts, setDicts] = useState<Record<string, DictOption[]>>({})
  const [detail, setDetail] = useState<(AdminContent & { fields?: Record<string, unknown> }) | null>(null)
  const [review, setReview] = useState<{ row: AdminContent; approve: boolean } | null>(null)
  const [reason, setReason] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.contents({ kind: 'product', status })
      setRows(res.items ?? [])
    } catch (e) {
      message.error(errText(e))
    } finally {
      setLoading(false)
    }
  }, [status])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    void Promise.all(['region', 'industry', 'need', 'offer'].map((d) => adminApi.dictOptions(d)))
      .then(([region, industry, need, offer]) => setDicts({ region, industry, need, offer }))
      .catch(() => undefined)
  }, [])

  useEffect(() => {
    if (!createOpen) return
    const timer = setTimeout(() => {
      void adminApi.orgs({ q: orgQ.trim() || undefined, limit: 30 })
        .then((r) => setOrgs(r.items ?? []))
        .catch(() => undefined)
    }, 250)
    return () => clearTimeout(timer)
  }, [orgQ, createOpen])

  const resetCreate = () => {
    setCreateOpen(false)
    setStep(0)
    setOrgId('')
    setSelectedOrg(null)
    setOrgQ('')
    form.resetFields()
    setFileList([])
  }

  const pickOrg = (id: string) => {
    const org = orgs.find((o) => o.id === id) ?? null
    setOrgId(id)
    setSelectedOrg(org)
    if (org) {
      form.setFieldsValue({
        region_code: org.region_code,
        industry_codes: org.industry_codes ?? [],
      })
    }
  }

  const next = async () => {
    if (step === 0) {
      if (!orgId) {
        message.warning('请先选择企业')
        return
      }
      setStep(1)
      return
    }
    try {
      await form.validateFields()
      setStep(2)
    } catch {
      /* 表单错误由 AntD 展示 */
    }
  }

  const customRequest: UploadProps['customRequest'] = async ({ file, onSuccess, onError }) => {
    try {
      const id = await adminApi.uploadAdminMedia(file as File)
      onSuccess?.(id)
    } catch (e) {
      onError?.(e as Error)
    }
  }

  const submit = async () => {
    const v = await form.validateFields()
    setSaving(true)
    try {
      const images = fileList.map((f) => String(f.response ?? '')).filter(Boolean)
      await adminApi.createProduct({
        owner_id: orgId,
        title: v.title,
        region_code: v.region_code,
        industry_codes: v.industry_codes ?? [],
        need_codes: v.need_codes ?? [],
        offer_codes: v.offer_codes ?? [],
        fields: {
          carousel_file_ids: images,
          target_customers: v.target_customers,
          target_channels: v.target_channels,
          core_strength: v.core_strength,
          key_points: v.key_points,
          intro: v.intro,
          contact_phone: v.contact_phone,
        },
      })
      message.success('产品已入库，等待审核')
      resetCreate()
      void load()
    } catch (e) {
      message.error(errText(e))
    } finally {
      setSaving(false)
    }
  }

  const openDetail = async (row: AdminContent) => {
    try {
      setDetail(await adminApi.contentDetail(row.id))
    } catch (e) {
      message.error(errText(e))
    }
  }

  const submitReview = async () => {
    if (!review) return
    if (!review.approve && !reason.trim()) {
      message.warning('请填写驳回原因')
      return
    }
    try {
      await adminApi.reviewContent(review.row.id, review.approve, reason.trim())
      message.success(review.approve ? '已通过审核' : '已驳回')
      setReview(null)
      setReason('')
      void load()
    } catch (e) {
      message.error(errText(e))
    }
  }

  const columns: ColumnsType<AdminContent> = [
    { title: '产品名称', dataIndex: 'title' },
    { title: '企业用户 ID', render: (_, r) => r.owner_ref?.id ?? '—' },
    { title: '区域', dataIndex: 'region_code', render: (v?: string) => optLabel(dicts.region ?? [], v) },
    {
      title: '状态',
      dataIndex: 'status',
      render: (s: number) => <Tag color={STATUS[s]?.color}>{STATUS[s]?.text ?? s}</Tag>,
    },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" onClick={() => void openDetail(r)}>详情</Button>
          {r.status === 0 ? (
            <>
              <Button
                size="small"
                type="primary"
                onClick={() => {
                  setReason('')
                  setReview({ row: r, approve: true })
                }}
              >
                审核
              </Button>
              <Button
                size="small"
                danger
                onClick={() => {
                  setReason('')
                  setReview({ row: r, approve: false })
                }}
              >
                驳回
              </Button>
            </>
          ) : null}
        </Space>
      ),
    },
  ]

  const orgOptions = useMemo(
    () => orgs.map((o) => ({ value: o.id, label: `${o.name}（${o.region_code || '未设置区域'}）` })),
    [orgs],
  )

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 160 }}
          value={status}
          onChange={setStatus}
          options={[
            { value: 0, label: '审核中' },
            { value: 1, label: '展示中' },
            { value: 2, label: '已下架' },
          ]}
        />
        <Button type="primary" onClick={() => setCreateOpen(true)}>添加产品</Button>
        <Button onClick={() => void load()}>刷新</Button>
      </Space>

      <Table rowKey="id" loading={loading} columns={columns} dataSource={rows} size="small" />

      <Modal
        title="产品入库"
        width={860}
        open={createOpen}
        onCancel={resetCreate}
        footer={null}
        destroyOnClose
      >
        <Steps current={step} size="small" style={{ marginBottom: 24 }} items={[
          { title: '企业信息' },
          { title: '产品信息' },
          { title: '完成' },
        ]} />
        {step === 0 ? (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Select
              showSearch
              filterOption={false}
              placeholder="搜索并选择企业"
              style={{ width: '100%' }}
              value={orgId || undefined}
              onSearch={setOrgQ}
              onChange={pickOrg}
              options={orgOptions}
            />
            {selectedOrg ? (
              <Descriptions size="small" column={2} bordered>
                <Descriptions.Item label="企业名称">{selectedOrg.name}</Descriptions.Item>
                <Descriptions.Item label="区域">{selectedOrg.region_code || '—'}</Descriptions.Item>
                <Descriptions.Item label="账号 ID">{selectedOrg.id}</Descriptions.Item>
                <Descriptions.Item label="认证状态">{selectedOrg.realname_status === 1 ? '已实名' : '未实名'}</Descriptions.Item>
              </Descriptions>
            ) : null}
          </Space>
        ) : null}

        <div style={{ display: step === 1 ? 'block' : 'none' }}>
          <Form form={form} layout="vertical">
            <Form.Item name="title" label="产品名称" rules={[{ required: true, message: '请输入产品名称' }, { max: 50 }]}>
              <Input />
            </Form.Item>
            <Form.Item name="region_code" label="区域">
              <Select
                showSearch
                allowClear
                optionFilterProp="label"
                options={(dicts.region ?? []).map((o) => ({ value: o.code, label: o.label }))}
              />
            </Form.Item>
            <Form.Item name="industry_codes" label="产品领域标签">
              <Select mode="multiple" optionFilterProp="label" options={(dicts.industry ?? []).map((o) => ({ value: o.code, label: o.label }))} />
            </Form.Item>
            <Form.Item name="need_codes" label="合作需求">
              <Select mode="multiple" optionFilterProp="label" options={(dicts.need ?? []).map((o) => ({ value: o.code, label: o.label }))} />
            </Form.Item>
            <Form.Item name="offer_codes" label="合作提供">
              <Select mode="multiple" optionFilterProp="label" options={(dicts.offer ?? []).map((o) => ({ value: o.code, label: o.label }))} />
            </Form.Item>
            <Form.Item name="target_customers" label="目标客户">
              <Input placeholder="例如：政府、央国企、金融" />
            </Form.Item>
            <Form.Item name="target_channels" label="目标渠道">
              <Input placeholder="例如：代理渠道、城市总代理" />
            </Form.Item>
            <Form.Item name="core_strength" label="产品核心优势">
              <Input.TextArea rows={2} maxLength={500} showCount />
            </Form.Item>
            <Form.Item name="key_points" label="产品关键点描述">
              <Input.TextArea rows={2} maxLength={500} showCount />
            </Form.Item>
            <Form.Item name="intro" label="产品简介">
              <Input.TextArea rows={3} maxLength={2000} showCount />
            </Form.Item>
            <Form.Item name="contact_phone" label="合作联系">
              <Input placeholder="手机号（会员可见）" />
            </Form.Item>
            <Form.Item label="产品图片（轮播）">
              <Upload
                listType="picture-card"
                fileList={fileList}
                maxCount={8}
                customRequest={customRequest}
                onChange={({ fileList: fl }) => setFileList(fl)}
              >
                {fileList.length >= 8 ? null : <div>上传</div>}
              </Upload>
            </Form.Item>
          </Form>
        </div>

        {step === 2 ? (
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label="企业">{selectedOrg?.name ?? orgId}</Descriptions.Item>
            <Descriptions.Item label="产品名称">{form.getFieldValue('title')}</Descriptions.Item>
            <Descriptions.Item label="区域">{optLabel(dicts.region ?? [], form.getFieldValue('region_code'))}</Descriptions.Item>
            <Descriptions.Item label="图片">{fileList.length} 张</Descriptions.Item>
            <Descriptions.Item label="提交后">进入「审核中」，通过后展示</Descriptions.Item>
          </Descriptions>
        ) : null}

        <Space style={{ marginTop: 24 }}>
          {step > 0 ? <Button onClick={() => setStep(step - 1)}>上一步</Button> : null}
          {step < 2 ? (
            <Button type="primary" onClick={() => void next()}>下一步</Button>
          ) : (
            <Button type="primary" loading={saving} onClick={() => void submit()}>提交入库</Button>
          )}
        </Space>
      </Modal>

      <Modal
        title={review?.approve ? '通过产品审核' : '驳回产品'}
        open={!!review}
        onCancel={() => setReview(null)}
        onOk={() => void submitReview()}
        okText="确认"
        okButtonProps={{ danger: !review?.approve }}
      >
        <p>{review?.row.title}</p>
        <Input.TextArea
          rows={3}
          placeholder={review?.approve ? '备注（可选）' : '驳回原因（必填）'}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </Modal>

      <Drawer title="产品详情" width={560} open={!!detail} onClose={() => setDetail(null)}>
        {detail ? (
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="产品名称">{detail.title}</Descriptions.Item>
            <Descriptions.Item label="企业用户 ID">{detail.owner_ref?.id ?? '—'}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={STATUS[detail.status]?.color}>{STATUS[detail.status]?.text ?? detail.status}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="区域">{optLabel(dicts.region ?? [], detail.region_code)}</Descriptions.Item>
            {Object.entries(detail.fields ?? {}).map(([k, v]) => (
              <Descriptions.Item key={k} label={FIELD_LABELS[k] ?? k}>
                {Array.isArray(v) ? v.join('、') : String(v ?? '—')}
              </Descriptions.Item>
            ))}
            {detail.reject_reason ? (
              <Descriptions.Item label="驳回原因">{detail.reject_reason}</Descriptions.Item>
            ) : null}
          </Descriptions>
        ) : null}
      </Drawer>
    </>
  )
}
