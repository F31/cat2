import { useCallback, useEffect, useState } from 'react'
import { Button, Checkbox, Form, Input, Modal, Space, Switch, Table, Tag, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { adminApi } from '../api/admin'

interface RoleRow {
  code: string
  name: string
  protected: boolean
  builtin: boolean
  perms?: string[]
  region_codes?: string[]
  include_subtree?: boolean
}

// 角色与权限（p4 §3，ADR-002）：五内置角色为默认矩阵；超管固定不可改；其余可配置、支持自定义角色。
export default function Roles() {
  const [roles, setRoles] = useState<RoleRow[]>([])
  const [perms, setPerms] = useState<Array<{ perm: string; label: string }>>([])
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<RoleRow | null>(null)
  const [checked, setChecked] = useState<string[]>([])
  const [creating, setCreating] = useState(false)
  const [createForm] = Form.useForm()
  const [scopeEditing, setScopeEditing] = useState<RoleRow | null>(null)
  const [scopeForm] = Form.useForm()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [r, p] = await Promise.all([adminApi.rbacRoles(), adminApi.rbacPermissions()])
      setRoles(r.items ?? [])
      setPerms(p.items ?? [])
    } catch (e) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const openPerms = async (r: RoleRow) => {
    setEditing(r)
    try {
      const res = await adminApi.rbacRolePerms(r.code)
      setChecked(res.perms ?? [])
    } catch {
      setChecked(r.perms ?? [])
    }
  }

  const savePerms = async () => {
    if (!editing) return
    try {
      await adminApi.rbacSetRolePerms(editing.code, checked)
      message.success('已保存（即时生效）')
      setEditing(null)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  const createRole = async () => {
    const v = await createForm.validateFields()
    try {
      await adminApi.rbacCreateRole(v.code, v.name)
      message.success('已创建')
      setCreating(false)
      createForm.resetFields()
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '创建失败')
    }
  }

  const removeRole = async (r: RoleRow) => {
    try {
      await adminApi.rbacDeleteRole(r.code)
      message.success('已删除')
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '删除失败')
    }
  }

  const openScope = (r: RoleRow) => {
    setScopeEditing(r)
    scopeForm.setFieldsValue({
      region_codes: (r.region_codes ?? []).join(','),
      include_subtree: !!r.include_subtree,
    })
  }

  const saveScope = async () => {
    if (!scopeEditing) return
    const v = await scopeForm.validateFields()
    const codes = String(v.region_codes ?? '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    try {
      await adminApi.rbacSetRoleScope(scopeEditing.code, codes, !!v.include_subtree)
      message.success('已保存（数据范围）')
      setScopeEditing(null)
      void load()
    } catch (e) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  const columns: ColumnsType<RoleRow> = [
    { title: '角色', dataIndex: 'code' },
    { title: '名称', dataIndex: 'name' },
    {
      title: '属性',
      render: (_, r) => (
        <Space>
          {r.protected ? <Tag color="red">超管（固定）</Tag> : null}
          {r.builtin ? <Tag>内置</Tag> : <Tag color="blue">自定义</Tag>}
        </Space>
      ),
    },
    {
      title: '权限数',
      render: (_, r) => (r.protected ? '全部（隐含）' : `${r.perms?.length ?? 0} 项`),
    },
    {
      title: '操作',
      render: (_, r) => (
        <Space>
          <Button size="small" disabled={r.protected} onClick={() => void openPerms(r)}>
            配置权限
          </Button>
          <Button size="small" disabled={r.protected} onClick={() => openScope(r)}>
            数据范围
          </Button>
          <Button size="small" danger disabled={r.protected || r.builtin} onClick={() => void removeRole(r)}>
            删除
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <>
      <Space style={{ marginBottom: 16 }}>
        <Button onClick={() => void load()}>刷新</Button>
        <Button type="primary" onClick={() => setCreating(true)}>
          新建角色
        </Button>
        <span style={{ color: '#888' }}>用户与后台角色在「用户管理 → 角色」中关联；超管由运维 grant-admin 授予。</span>
      </Space>
      <Table rowKey="code" loading={loading} columns={columns} dataSource={roles} size="small" />

      <Modal title={`配置权限 · ${editing?.name ?? ''}`} open={!!editing} onOk={() => void savePerms()} onCancel={() => setEditing(null)} okText="保存">
        <Checkbox.Group
          value={checked}
          onChange={(v) => setChecked(v as string[])}
          options={perms.map((p) => ({ label: `${p.label}（${p.perm}）`, value: p.perm }))}
          style={{ display: 'flex', flexDirection: 'column', gap: 8, maxHeight: 420, overflow: 'auto' }}
        />
      </Modal>

      <Modal title="新建角色" open={creating} onOk={() => void createRole()} onCancel={() => setCreating(false)} okText="创建">
        <Form form={createForm} layout="vertical">
          <Form.Item name="code" label="角色编码（英文）" rules={[{ required: true }]}>
            <Input placeholder="如 regional_ops" />
          </Form.Item>
          <Form.Item name="name" label="角色名称" rules={[{ required: true }]}>
            <Input placeholder="如 区域运营" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`数据范围 · ${scopeEditing?.name ?? ''}`}
        open={!!scopeEditing}
        onOk={() => void saveScope()}
        onCancel={() => setScopeEditing(null)}
        okText="保存"
      >
        <Form form={scopeForm} layout="vertical">
          <Form.Item
            name="region_codes"
            label="区域代码（逗号分隔；留空=不限制）"
            extra="如 310000,330000；配合「含子区域」用于渠道经理多区域/省市级过滤（D5）"
          >
            <Input placeholder="310000,330000" />
          </Form.Item>
          <Form.Item name="include_subtree" label="含子区域" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
