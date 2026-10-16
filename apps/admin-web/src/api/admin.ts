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
  if (token) headers['Authorization'] = `Bearer ${token}`
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
}

export interface AuditEntry {
  id: number
  actor_id: number
  actor_type: number
  channel?: string
  action: string
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

  menus() {
    return call<{ items: AdminMenu[] }>('GET', '/admin/v1/menus')
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
  rbacAdminRoles(userId: string) {
    return call<{ roles: string[] }>('GET', `/admin/v1/admin-users/${userId}/roles`)
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
    return call<{ items: FeatureFlag[] }>('GET', '/admin/v1/feature-flags')
  },
  setFeatureFlag(key: string, enabled: boolean, ratio = 0, users: number[] = []) {
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
