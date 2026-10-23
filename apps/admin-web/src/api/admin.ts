import type { adminComponents } from '@catcube/api-client'
import { getToken, setToken } from '../auth'

// DTO 由契约生成（contracts/openapi/admin.yaml → make gen），避免前后端漂移。
type Schemas = adminComponents['schemas']
type Req<K extends keyof Schemas, F extends keyof Schemas[K]> = Required<Pick<Schemas[K], F>>

interface Envelope<T> {
  code: number
  message: string
  request_id: string
  data?: T | null
}

export class ApiError extends Error {
  constructor(
    public code: number,
    message: string,
  ) {
    super(message)
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const token = getToken()
  // 后台经 Nginx Basic Auth 时 Authorization 被占用，令牌走独立头；后端兼容两者
  if (token) headers['X-Admin-Token'] = token
  const res = await fetch('/api' + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const env = (await res.json()) as Envelope<T>
  if (env.code !== 0) throw new ApiError(env.code, env.message)
  return env.data as T
}

export type AdminUser = Req<
  'AdminUser',
  'id' | 'nickname' | 'status' | 'identity_type' | 'realname_status' | 'phone_masked' | 'created_at'
>

export type AdminContent = Req<'ContentItem', 'id' | 'kind' | 'title' | 'status'> & {
  reject_reason?: string
  owner_ref?: { type: string; id: string }
  region_code?: string
}

export interface AdminProductInput {
  owner_id: string
  title: string
  region_code?: string
  industry_codes?: string[]
  need_codes?: string[]
  offer_codes?: string[]
  fields?: Record<string, unknown>
}

export type AdminOrder = Req<'Order', 'id' | 'biz_type' | 'amount_fen' | 'status' | 'pay_expire_at'>

export type AdminMember = Req<'AdminMember', 'user_id' | 'level' | 'status' | 'started_at' | 'expires_at'>

export type ReviewTask = Req<
  'ReviewTask',
  'id' | 'source' | 'subject_type' | 'subject_id' | 'risk_level' | 'status' | 'sla_overdue'
> & { reason?: string }

export type ReviewMetrics = Req<'SLAMetric', 'pending' | 'overdue' | 'avg_review_seconds'>

export interface AdminMenu {
  key: string
  label: string
  path: string
  module: string
  parent_key?: string
  group?: string
  group_label?: string
  group_order?: number
  order?: number
}

export interface AuditEntry {
  id: number
  actor_id: string
  actor_name?: string
  actor_type: number
  channel?: string
  action: string
  action_label?: string
  target_ref?: string
  detail?: Record<string, unknown>
  result: number
  ip?: string
  request_id?: string
  hash?: string
  prev_hash?: string
  created_at?: number
  archived?: boolean
}

export interface ReviewSuggestion {
  available: boolean
  category?: string
  risk?: number
  summary?: string
}

export interface SettingItem {
  key: string
  type: string
  value: string
  default?: string
  description?: string
  sensitive?: boolean
}

export interface FeatureFlag {
  key: string
  enabled: boolean
  ratio?: number
  users?: number[]
}

export interface DeadLetter {
  event_id: string
  topic: string
  space_id: number
  occurred_at: string
}

export interface NoticeTemplate {
  code: string
  module: string
  title_tpl: string
  content_tpl: string
  channels: string[]
  merge_rule?: string
  closeable: boolean
}

export const adminApi = {
  // 开发态登录：微信登录适配器以手机号为 code。
  loginPassword(phone: string, password: string) {
    return call<{ user_id: string; tokens: { access_token: string } }>('POST', '/admin/v1/auth/password', {
      phone,
      password,
    }).then((r) => {
      setToken(r.tokens.access_token)
      return r
    })
  },

  changePassword(oldPassword: string, newPassword: string) {
    return call<{ changed: boolean }>('POST', '/admin/v1/auth/password/change', {
      old_password: oldPassword,
      new_password: newPassword,
    })
  },

  resetPasswordCode(phone: string) {
    return call<{ sent: boolean }>('POST', '/admin/v1/auth/password/reset/code', { phone })
  },

  resetPassword(phone: string, code: string, newPassword: string) {
    return call<{ changed: boolean }>('POST', '/admin/v1/auth/password/reset', {
      phone,
      code,
      new_password: newPassword,
    })
  },

  login(phone: string) {
    return call<{ user_id: string; tokens: { access_token: string } }>('POST', '/mp/v1/auth/wechat', {
      code: phone,
      phone_code: phone,
      device: 'admin-web',
    }).then((r) => {
      setToken(r.tokens.access_token)
      return r
    })
  },

  createUser(body: {
    nickname: string
    phone: string
    identity_type: number
    real_name?: string
    id_card_no?: string
    region_code?: string
  }) {
    return call<{ id: string }>('POST', '/admin/v1/users', body)
  },
  updateUser(id: string, body: {
    nickname: string
    phone?: string
    identity_type: number
    real_name?: string
    id_card_no?: string
    region_code?: string
  }) {
    return call<{ id: string }>('PUT', `/admin/v1/users/${id}`, body)
  },
  deleteUser(id: string) {
    return call<{ deleted: boolean }>('DELETE', `/admin/v1/users/${id}`)
  },
  adminCard(id: string) {
    return call<AdminCard>('GET', `/admin/v1/users/${id}/card`)
  },
  adminUpdateOrgCard(id: string, body: {
    org_type_code: string
    name: string
    region_code: string
    industry_codes: string[]
    need_codes: string[]
    offer_codes: string[]
    core_strength: string
    intro: string
  }) {
    return call<AdminCard>('PUT', `/admin/v1/users/${id}/card`, body)
  },
  orgs(f: { q?: string; category?: string; status?: number; limit?: number } = {}) {
    const qs = new URLSearchParams()
    Object.entries(f).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') qs.set(k, String(v))
    })
    return call<{ items: AdminOrg[]; total: number }>('GET', `/admin/v1/orgs?${qs.toString()}`)
  },
  orgDetail(id: string) {
    return call<AdminOrgDetail>('GET', `/admin/v1/orgs/${id}`)
  },
  orgMembers(id: string) {
    return call<{ items: AdminOrgMember[] }>('GET', `/admin/v1/orgs/${id}/members`)
  },
  addOrgMember(id: string, body: { user_id: string; role?: string; proof_file_ids?: number[] }) {
    return call<AdminOrgMember>('POST', `/admin/v1/orgs/${id}/members`, body)
  },
  removeOrgMember(id: string, userId: string) {
    return call<{ removed: boolean }>('DELETE', `/admin/v1/orgs/${id}/members/${userId}`)
  },
  orgAdmins(id: string) {
    return call<{ items: Array<{ org_id: string; user_id: string; status: number }> }>('GET', `/admin/v1/orgs/${id}/admins`)
  },
  submitOrgCert(id: string, body: {
    cert_type: number
    agreed: boolean
    material_file_ids: number[]
    extra?: Record<string, string>
  }) {
    return call<AdminCert>('POST', `/admin/v1/orgs/${id}/certifications`, body)
  },
  async uploadAdminMedia(file: File) {
    const form = new FormData()
    form.append('file', file)
    const headers: Record<string, string> = {}
    const token = getToken()
    if (token) headers['X-Admin-Token'] = token
    const res = await fetch('/api/admin/v1/media/upload', { method: 'POST', headers, body: form })
    const env = (await res.json()) as Envelope<{ file_id: string }>
    if (env.code !== 0) throw new ApiError(env.code, env.message || '上传失败')
    return env.data?.file_id ?? ''
  },
  adminMediaURL(id: string) {
    return call<{ url: string }>('GET', `/admin/v1/media/${id}`)
  },
  async adminMediaObjectURL(id: string) {
    const meta = await call<{ url: string }>('GET', `/admin/v1/media/${id}`)
    if (!meta.url.startsWith('/api/')) return meta.url
    const headers: Record<string, string> = {}
    const token = getToken()
    if (token) headers['X-Admin-Token'] = token
    const res = await fetch(meta.url, { headers })
    if (!res.ok) throw new ApiError(9, '加载材料失败')
    return URL.createObjectURL(await res.blob())
  },

  users(params: { status?: number; q?: string; limit?: number } = {}) {
    const qs = new URLSearchParams()
    if (params.status !== undefined) qs.set('status', String(params.status))
    if (params.q) qs.set('q', params.q)
    qs.set('limit', String(params.limit ?? 100))
    return call<{ items: AdminUser[] }>('GET', `/admin/v1/users?${qs.toString()}`)
  },
  banUser(id: string, reason: string) {
    return call<{ status: number }>('POST', `/admin/v1/users/${id}/ban`, { reason })
  },
  unbanUser(id: string) {
    return call<{ status: number }>('POST', `/admin/v1/users/${id}/unban`)
  },

  contents(params: { kind?: string; status?: number } = {}) {
    const qs = new URLSearchParams()
    if (params.kind) qs.set('kind', params.kind)
    if (params.status !== undefined) qs.set('status', String(params.status))
    qs.set('limit', '100')
    return call<{ items: AdminContent[] }>('GET', `/admin/v1/contents?${qs.toString()}`)
  },
  contentDetail(id: string) {
    return call<AdminContent & { fields?: Record<string, unknown> }>('GET', `/admin/v1/contents/${id}`)
  },
  createProduct(body: AdminProductInput) {
    return call<AdminContent & { fields?: Record<string, unknown> }>('POST', '/admin/v1/products', body)
  },
  reviewContent(id: string, approve: boolean, reason: string) {
    return call<AdminContent>('POST', `/admin/v1/contents/${id}/review`, { approve, reason })
  },
  takedownContent(id: string, reason: string) {
    return call<{ id: string }>('POST', `/admin/v1/contents/${id}/takedown`, { reason })
  },

  orders(params: { status?: number } = {}) {
    const qs = new URLSearchParams()
    if (params.status !== undefined) qs.set('status', String(params.status))
    qs.set('limit', '100')
    return call<{ items: AdminOrder[] }>('GET', `/admin/v1/orders?${qs.toString()}`)
  },

  members(params: { level?: number; status?: number } = {}) {
    const qs = new URLSearchParams()
    if (params.level !== undefined) qs.set('level', String(params.level))
    if (params.status !== undefined) qs.set('status', String(params.status))
    qs.set('limit', '100')
    return call<{ items: AdminMember[] }>('GET', `/admin/v1/members?${qs.toString()}`)
  },

  reviewTasks() {
    return call<{ items: ReviewTask[] }>('GET', '/admin/v1/review-tasks?limit=100')
  },
  resolveReview(id: string, confirmed: boolean, reason: string) {
    return call<unknown>('POST', `/admin/v1/review-tasks/${id}/resolve`, { confirmed, reason })
  },
  reviewMetrics() {
    return call<ReviewMetrics>('GET', '/admin/v1/review-metrics')
  },

  noticeTemplates() {
    return call<{ items: NoticeTemplate[] }>('GET', '/admin/v1/notice-templates')
  },
  updateNoticeTemplate(code: string, body: { title_tpl: string; content_tpl: string; channels: string[]; closeable: boolean }) {
    return call<{ saved: boolean }>('PUT', `/admin/v1/notice-templates/${encodeURIComponent(code)}`, body)
  },

  // ---- 字典 / 画像字段模板 ----
  dictSubjectTypes() {
    return call<DictSubjectType[]>('GET', '/mp/v1/dict/subject-types')
  },
  dictOptions(dimension: string) {
    return call<DictOption[]>('GET', `/mp/v1/dict/options?dimension=${encodeURIComponent(dimension)}`)
  },
  profileTemplates() {
    return call<{ items: ProfileTemplate[]; field_pool: FieldDef[] }>('GET', '/admin/v1/profile-templates')
  },

  // ---- 认证审核（verify）----
  pendingCerts(reviewerType = 1) {
    return call<{ items: AdminCert[] }>('GET', `/admin/v1/certs/pending?reviewer_type=${reviewerType}&limit=100`)
  },
  reviewCert(id: string, pass: boolean, reason: string) {
    return call<{ reviewed: boolean }>('POST', `/admin/v1/certs/${id}/review`, { pass, reason })
  },
  realnameDetail(certId: string) {
    return call<RealnameDetail>('GET', `/admin/v1/certs/${certId}/realname`)
  },

  // ---- 猫立方 / 圈子管理 ----
  cubes(q?: string) {
    const qs = new URLSearchParams()
    if (q) qs.set('q', q)
    qs.set('limit', '100')
    return call<{ items: AdminCube[] }>('GET', `/admin/v1/cubes?${qs.toString()}`)
  },
  circles(q?: string, status?: number) {
    const qs = new URLSearchParams()
    if (q) qs.set('q', q)
    if (status != null) qs.set('status', String(status))
    qs.set('limit', '100')
    return call<{ items: AdminCircle[] }>('GET', `/admin/v1/circles?${qs.toString()}`)
  },
  batchCircleStatus(action: 'ban' | 'archive' | 'unban' | 'restore', ids: string[], reason: string) {
    return call<{ items: Array<{ id: string; ok: boolean; before?: number; after?: number; error?: string }>; succeeded: number; failed: number }>(
      'POST',
      '/admin/v1/circles/batch',
      { action, ids, reason },
    )
  },

  // ---- 圈子后台详情/概览 ----
  circleStats(days = 30) {
    return call<{
      days: number
      new_total: number
      new_permanent: number
      new_temporary: number
      active_circles: number
      members_total: number
      demands_total: number
      demand_valid_rate: number
      member_trend: Array<{ date: string; new_members: number }>
    }>('GET', `/admin/v1/circles/stats?days=${days}`)
  },
  circleDetail(id: string) {
    return call<{
      circle: {
        id: string
        name: string
        intro: string
        type: number
        status: number
        owner_user_id: string
        member_count: number
        region_code?: string
        created_at: number
        end_at?: number
      }
      owner_name: string
      members: Array<{ user_id: string; nickname: string; joined_via: number; joined_at: number }>
      demands: Array<{ id: string; author_id: string; author_name: string; title: string; content: string; status: number; created_at: number }>
      feeds: Array<{ id: string; author_id: string; author_name: string; text: string; is_announcement: boolean; status: number; created_at: number }>
      mounts: Array<{ id: string; item_ref: string; mount_type: number; approval_status: number; status: number; created_at: number }>
    }>('GET', `/admin/v1/circles/${id}`)
  },
  removeCircleContent(circleId: string, kind: 'demands' | 'feeds' | 'mounts', itemId: string, reason: string) {
    return call<{ removed: boolean }>('POST', `/admin/v1/circles/${circleId}/${kind}/${itemId}/remove`, { reason })
  },

  // ---- 立方后台详情/成员/项目/群聊 ----
  cubeDetail(id: string) {
    return call<{
      cube: {
        id: string
        name: string
        leader_id: string
        honey_id: string
        plan_level: number
        plan_expire_at?: string
        status: number
        member_count: number
        region_code?: string
        org_id?: string
        industry_codes?: string[]
        chain_stage?: string
        direction?: string
        resources?: string
        strength?: string
        cooperation?: string
      }
      members: Array<{ user_id: string; nickname: string; role: number; status: number; joined_at: number }>
      projects: AdminCubeProject[]
    }>('GET', `/admin/v1/cubes/${id}`)
  },
  cubeProjects(id: string) {
    return call<{ items: AdminCubeProject[] }>('GET', `/admin/v1/cubes/${id}/projects`)
  },
  cubeRemoveMember(id: string, uid: string, reason: string) {
    return call<{ removed: boolean }>('POST', `/admin/v1/cubes/${id}/members/${uid}/remove`, { reason })
  },
  cubeSetLeader(id: string, uid: string) {
    return call<{ updated: boolean }>('POST', `/admin/v1/cubes/${id}/leader`, { user_id: uid })
  },
  cubeSetHoney(id: string, uid: string) {
    return call<{ updated: boolean }>('POST', `/admin/v1/cubes/${id}/honey`, { user_id: uid })
  },
  cubeChats(id: string) {
    return call<{
      items: Array<{
        session_id: string
        biz_ref: string
        title: string
        messages: Array<{ id: string; seq: number; sender_id: string; sender_name: string; msg_type: number; content: string; created_at: number }>
      }>
    }>('GET', `/admin/v1/cubes/${id}/chats`)
  },

  // ---- 退款 / 履约补偿 ----
  refund(orderId: string, amountFen: number, reason: string) {
    return call<{ id: string }>('POST', '/admin/v1/refunds', { order_id: orderId, amount_fen: amountFen, reason })
  },
  retryFulfillment(orderId: string) {
    return call<{ reset: boolean }>('POST', `/admin/v1/fulfillments/${orderId}/retry`)
  },

  // ---- 会员代操作 ----
  forceRemoveSub(masterId: string, subUserId: string, reason: string) {
    return call<{ removed: boolean }>('POST', '/admin/v1/memberships/sub-accounts/remove', {
      master_id: masterId,
      sub_user_id: subUserId,
      reason,
    })
  },

  menus(group?: string) {
    const qs = group ? `?group=${encodeURIComponent(group)}` : ''
    return call<{ items: AdminMenu[]; groups?: Array<{ key: string; label: string; order: number }> }>(
      'GET',
      `/admin/v1/menus${qs}`,
    )
  },
  me() {
    return call<{ user_id: string; nickname: string; super: boolean; roles: Array<{ code: string; name: string }> }>(
      'GET',
      '/admin/v1/me',
    )
  },

  // ---- 后台 RBAC（p4 §3，ADR-002）----
  rbacPermissions() {
    return call<{ items: Array<{ perm: string; label: string }> }>('GET', '/admin/v1/permissions')
  },
  rbacRoles() {
    return call<{ items: Array<{ code: string; name: string; protected: boolean; builtin: boolean; perms?: string[] }> }>(
      'GET',
      '/admin/v1/roles',
    )
  },
  rbacRolePerms(code: string) {
    return call<{ perms: string[] }>('GET', `/admin/v1/roles/${code}/permissions`)
  },
  rbacSetRolePerms(code: string, perms: string[]) {
    return call<{ ok: boolean }>('PUT', `/admin/v1/roles/${code}/permissions`, { perms })
  },
  rbacCreateRole(code: string, name: string) {
    return call<{ ok: boolean }>('POST', '/admin/v1/roles', { code, name })
  },
  rbacDeleteRole(code: string) {
    return call<{ ok: boolean }>('DELETE', `/admin/v1/roles/${code}`)
  },
  rbacSetRoleScope(code: string, regionCodes: string[], includeSubtree: boolean) {
    return call<{ ok: boolean }>('PUT', `/admin/v1/roles/${code}/scope`, {
      region_codes: regionCodes,
      include_subtree: includeSubtree,
    })
  },
  rbacAdminRoles(userId: string) {
    return call<{ roles: string[]; super: boolean }>('GET', `/admin/v1/admin-users/${userId}/roles`)
  },
  rbacAssignRoles(userId: string, roles: string[]) {
    return call<{ ok: boolean }>('PUT', `/admin/v1/admin-users/${userId}/roles`, { roles })
  },

  exports(all = false) {
    return call<{ items: Array<{ id: string; template: string; status: number; rows: number; error?: string }> }>(
      'GET',
      `/admin/v1/exports?all=${all ? 1 : 0}`,
    )
  },
  createExport(template: string, from: number, to: number) {
    return call<{ id: string; status: number }>('POST', '/admin/v1/exports', { template, from, to })
  },
  downloadExport(id: string) {
    return call<{ url: string }>('GET', `/admin/v1/exports/${id}/download`)
  },
  dashboard() {
    return call<{ metrics: Record<string, number>; sections: Record<string, Record<string, number>> }>(
      'GET',
      '/admin/v1/dashboard',
    )
  },
  dashboardFunnel(days = 7) {
    return call<{
      days: number
      steps: Array<{ key: string; label: string; users: number; conversion: number }>
    }>('GET', `/admin/v1/dashboard/funnel?days=${days}`)
  },
  jobs() {
    return call<{ items: string[] }>('GET', '/admin/v1/jobs')
  },
  runJob(name: string) {
    return call<{ ran: boolean; job: string }>('POST', `/admin/v1/jobs/${name}/run`)
  },
  broadcastNotice(userIds: string[], title: string, content: string, reason: string) {
    return call<{ sent: number }>('POST', '/admin/v1/notices/broadcast', {
      user_ids: userIds,
      title,
      content,
      reason,
    })
  },
  adjustMember(userId: string, level: number, reason: string, expireAt?: string) {
    return call<{ adjusted: boolean }>('POST', `/admin/v1/memberships/${userId}/adjust`, {
      level,
      expire_at: expireAt,
      reason,
    })
  },
  transferMaster(userId: string, toUserId: string, reason: string) {
    return call<{ transferred: boolean; sub_accounts_moved: number }>(
      'POST',
      `/admin/v1/memberships/${userId}/transfer-master`,
      { to_user_id: toUserId, reason },
    )
  },
  overdueAffiliations(days = 7) {
    return call<{ items: Array<{ id: string; user_id: string; org_id: string; role?: string }> }>(
      'GET',
      `/admin/v1/affiliations/overdue?days=${days}`,
    )
  },
  resolveAffiliation(id: string, pass: boolean, reason: string) {
    return call<{ resolved: boolean }>('POST', `/admin/v1/affiliations/${id}/resolve`, { pass, reason })
  },
  adminCubeStatus(id: string, action: 'ban' | 'dissolve', reason: string) {
    return call<{ cube_id: string; status: number }>('POST', `/admin/v1/cubes/${id}/${action}`, { reason })
  },
  adminDissolveCircle(id: string, reason: string) {
    return call<{ dissolved: boolean }>('POST', `/admin/v1/circles/${id}/dissolve`, { reason })
  },

  // ---- 渠道（运营商/结算）----
  channelOperators(status?: number) {
    return call<{ items: ChannelOperatorRow[] }>(
      'GET',
      `/admin/v1/channel/operators${status != null ? `?status=${status}` : ''}`,
    )
  },
  channelOpenOperator(id: string) {
    return call<{ ok: boolean }>('POST', `/admin/v1/channel/operators/${id}/open`)
  },
  channelSuspendOperator(id: string) {
    return call<{ ok: boolean }>('POST', `/admin/v1/channel/operators/${id}/suspend`)
  },
  channelOperatorFees(id: string) {
    return call<ChannelFees>('GET', `/admin/v1/channel/operators/${id}/fees`)
  },
  channelRecordFees(id: string, body: { agency_fee_fen: number; agency_fee_year: number; guarantee_fen: number }) {
    return call<{ ok: boolean }>('POST', `/admin/v1/channel/operators/${id}/fees`, body)
  },
  channelRefundGuarantee(id: string, reason: string) {
    return call<{ ok: boolean }>('POST', `/admin/v1/channel/operators/${id}/guarantee/refund`, { reason })
  },
  channelSettlements(status?: number) {
    return call<{ items: ChannelSettlementRow[] }>(
      'GET',
      `/admin/v1/channel/settlements${status != null ? `?status=${status}` : ''}`,
    )
  },
  channelGenerateSettlement(operatorId: string, month?: string) {
    return call<{ settlement_id: string }>('POST', '/admin/v1/channel/settlements/generate', {
      operator_id: operatorId,
      month,
    })
  },
  channelReviewSettlement(id: string) {
    return call<{ ok: boolean }>('POST', `/admin/v1/channel/settlements/${id}/review`)
  },
  channelRejectSettlement(id: string, reason: string) {
    return call<{ ok: boolean }>('POST', `/admin/v1/channel/settlements/${id}/reject`, { reason })
  },
  channelPaySettlement(id: string, voucherRef: string) {
    return call<{ ok: boolean }>('POST', `/admin/v1/channel/settlements/${id}/pay`, { voucher_ref: voucherRef })
  },

  governanceUsers(f: {
    q?: string
    phone?: string
    status?: string
    realname?: string
    level?: string
    from?: string
    to?: string
    limit?: number
    offset?: number
  }) {
    const qs = new URLSearchParams()
    Object.entries(f).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') qs.set(k, String(v))
    })
    return call<{
      items: Array<{
        id: string
        nickname: string
        status: number
        identity_type: number
        realname_status: number
        real_name?: string
        id_card_masked?: string
        region_code?: string
        industry_codes?: string[]
        created_at: number
        member_level: number
        member_active: boolean
      }>
    }>('GET', `/admin/v1/governance/users?${qs.toString()}`)
  },
  restrictUser(id: string, days: number, reason: string) {
    return call<{ restricted: boolean; days: number }>('POST', `/admin/v1/users/${id}/restrict`, { days, reason })
  },

  user360(id: string) {
    return call<{ user_id: string; sections: Record<string, Record<string, unknown>> }>(
      'GET',
      `/admin/v1/users/${id}/360`,
    )
  },
  badgeDefs() {
    return call<{ items: Array<{ code: string; label: string; level: number }> }>('GET', '/admin/v1/badges')
  },
  upsertBadge(code: string, label: string, level: number) {
    return call<{ saved: boolean }>('PUT', `/admin/v1/badges/${code}`, { label, level })
  },
  grantBadge(subjectType: string, subjectId: string, code: string, grant: boolean) {
    return call<{ ok: boolean }>('POST', '/admin/v1/badges/grant', {
      subject_type: subjectType,
      subject_id: subjectId,
      code,
      grant,
    })
  },

  audits(f: { actor_id?: string; action?: string; result?: string; target_ref?: string; limit?: number }) {
    const qs = new URLSearchParams()
    Object.entries(f).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') qs.set(k, String(v))
    })
    return call<{ items: AuditEntry[] }>('GET', `/admin/v1/audits?${qs.toString()}`)
  },
  auditDetail(id: number) {
    return call<AuditEntry>('GET', `/admin/v1/audits/${id}`)
  },

  reviewSuggestion(id: string) {
    return call<ReviewSuggestion>('GET', `/admin/v1/review-tasks/${id}/suggestion`)
  },

  settings() {
    return call<{ items: SettingItem[] }>('GET', '/admin/v1/settings')
  },
  setSetting(key: string, value: string) {
    return call<{ saved: boolean }>('PUT', `/admin/v1/settings/${key}`, { value })
  },

  featureFlags() {
    return call<{ items: FeatureFlag[]; known?: Array<{ key: string; label: string; description?: string }> }>(
      'GET',
      '/admin/v1/feature-flags',
    )
  },
  setFeatureFlag(key: string, enabled: boolean, ratio = 0, users: string[] = []) {
    return call<{ saved: boolean }>('PUT', `/admin/v1/feature-flags/${key}`, { enabled, ratio, users })
  },

  deadLetters() {
    return call<{ items: DeadLetter[] }>('GET', '/admin/v1/dead-letters')
  },
  replayDeadLetter(eventId: string) {
    return call<{ replayed: boolean }>('POST', `/admin/v1/dead-letters/${eventId}/replay`)
  },

  aiPrompts() {
    return call<{ items: AiPrompt[] }>('GET', '/admin/v1/ai/prompts')
  },
  aiSavePrompt(p: { feature: string; version: string; body: string; approved: boolean }) {
    return call<{ saved: boolean }>('PUT', '/admin/v1/ai/prompts', p)
  },
  aiUsage(days = 7) {
    return call<{ days: number; items: AiUsage[] }>('GET', `/admin/v1/ai/usage?days=${days}`)
  },
}

export interface AiPrompt {
  feature: string
  version: string
  body: string
  approved: boolean
}

export interface AiUsage {
  feature: string
  calls: number
  degraded: number
  adopted: number
  rejected: number
  adopt_rate: number
  tokens: number
  cost_fen: number
}

export interface DictSubjectType {
  code: string
  name: string
  can_publish: boolean
  home_visible: boolean
  sort: number
  enabled: boolean
}

export interface DictOption {
  dimension: string
  code: string
  label: string
  parent_code?: string
  sort: number
  enabled: boolean
}

export interface FieldDef {
  field: string
  label: string
  value_type: string
}

export interface ProfileTemplate {
  code: string
  name: string
  fields: Array<{ field: string; label: string; value_type: string; required: boolean; sort: number }>
}

export interface AdminCard {
  user_id: string
  subject_type_code?: string
  org_type_code?: string
  identity_type: number
  name: string
  avatar_file_id?: string
  region_code?: string
  industry_codes?: string[]
  need_codes?: string[]
  offer_codes?: string[]
  advantage_field?: string
  background?: string
  target_customers?: string
  target_channels?: string
  core_strength?: string
  intro?: string
  profile_tier: number
  has_contact: boolean
}

export interface RealnameDetail {
  cert_id: string
  user_id: string
  real_name: string
  id_card_no: string
  material_file_ids?: number[]
  status: number
  submitted_at: string
}

export interface AdminOrg {
  id: string
  name: string
  nickname: string
  status: number
  realname_status: number
  phone_masked?: string
  real_name?: string
  id_card_masked?: string
  category?: string
  category_label?: string
  region_code?: string
  industry_codes?: string[]
  cert_status?: number
  cert_type?: number
  cert_submitted_at?: string
  member_count?: number
  member_pending?: number
  admin_count?: number
  created_at: number
}

export interface AdminOrgMember {
  id: string
  user_id: string
  org_id: string
  role?: string
  is_primary: boolean
  status: number
  reject_reason?: string
}

export interface AdminOrgDetail {
  id: string
  user?: { id: string; nickname: string; status: number; identity_type: number }
  card?: AdminCard
  certs?: AdminCert[]
  members?: AdminOrgMember[]
  admins?: Array<{ org_id: string; user_id: string; status: number }>
}

export interface AdminCert {
  id: string
  user_id: string
  cert_type: number
  status: number
  reject_reason?: string
  material_file_ids?: number[]
  extra?: Record<string, string>
  submitted_at: string
}

export interface AdminCube {
  id: string
  name: string
  leader_id: string
  honey_id: string
  plan_level: number
  plan_expire_at?: string
  status: number
  member_count: number
  region_code?: string
  project_count?: number
  created_at?: string
}

export interface AdminCircle {
  id: string
  name: string
  type: number
  status: number
  owner_user_id: string
  member_count: number
  region_code?: string
  created_at: number
  prev_status?: number
}

export interface AdminCubeProject {
  id: string
  cube_id: string
  type: number
  name: string
  progress: number
  status: number
  leader_id: string
  todo_count?: number
  created_at?: string
  platform_status?: number
}

export interface ChannelOperatorRow {
  id: string
  user_id: string
  region_code: string
  region_name?: string
  name: string
  status: number
  referrer_operator_id: string
  opened_at?: number
}

export interface ChannelGuaranteeRow {
  id: number
  amount_fen: number
  paid_at: number
  refunded_at: number
  refund_reason: string
}

export interface ChannelFees {
  agency_fee_paid_fen: number
  agency_fee_year: number
  guarantee_fen: number
  guarantees: ChannelGuaranteeRow[]
}

export interface ChannelSettlementRow {
  id: number
  operator_id: number
  month: string
  commission_fen: number
  adjust_fen: number
  payable_fen: number
  status: number
  reviewed_by: number
  paid_by: number
  voucher_ref: string
}
