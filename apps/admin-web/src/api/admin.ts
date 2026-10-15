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
}
