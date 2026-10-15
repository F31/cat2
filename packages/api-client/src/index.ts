// 本包 schema.d.ts 由 contracts/openapi/*.yaml 经 `make gen` 生成（openapi-typescript），勿手改。
// 本文件为基于生成类型的薄客户端（信封解包、游标分页、Idempotency-Key）。
import type { paths, components } from './schema'

export type { paths, components }

/** 统一响应信封（与 contracts/openapi 约定一致）。 */
export interface Envelope<T = unknown> {
  code: number
  message: string
  request_id: string
  data?: T | null
}

/** 游标分页响应。 */
export interface CursorPage<T> {
  items: T[]
  next_cursor: string | null
}

export class ApiError extends Error {
  constructor(
    public code: number,
    message: string,
    public requestId: string,
  ) {
    super(message)
  }
}

// ---- DTO（与后端契约对齐；ID 一律字符串）----

export interface Tokens {
  access_token: string
  refresh_token: string
  expires_in: number
}

export interface LoginResult {
  user_id: string
  new: boolean
  tokens: Tokens
}

export interface Completion {
  complete: boolean
  missing: string[]
  tier: number
}

export interface Card {
  user_id: string
  subject_type_code?: string
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
  profile_tier?: number
  has_contact?: boolean
}

export interface MatchReason {
  dimension: string
  text: string
}

export interface DiscoveryCard {
  ref: string
  type: string
  title: string
  cover?: string
  region?: string
  score: number
  member_level: number
  verified: boolean
  reason?: MatchReason[]
}

export interface UnreadData {
  follow: number
  msg: number
  total: number
}

export interface FeedData {
  items: DiscoveryCard[]
  profile_progress: Completion
  unread: UnreadData
}

export interface DetailData {
  ref: string
  type: string
  card?: Card
  content?: Record<string, unknown>
  owner?: Card
  badges?: { code: string; label: string; level: number }[]
  member?: Member
  following: boolean
}

export interface Session {
  id: string
  kind: number
  peer?: { user_id: string; title?: string; cover?: string }
  last_seq: number
  unread: number
  last_message_at?: string | null
}

export interface Message {
  id: string
  session_id: string
  seq: number
  sender_id: string
  msg_type: number
  content: string
  created_at: string
}

export interface Target {
  type: string
  id: string
}

export interface FollowItem {
  follower_id: string
  target: Target
  title?: string
  cover?: string
  created_at: string
}

export interface FavoriteItem {
  id: string
  target: Target
  kind: 'person' | 'company' | 'project' | 'product' | 'meeting' | 'event'
  title?: string
  cover?: string
  created_at: string
}

export interface MembershipQuote {
  level: number
  years: number
  unit_fen: number
  original_fen: number
  discount_rate: number
  total_fen: number
}

export interface Member {
  user_id: string
  level: number
  status: number
  expires_at: string
}

export interface Order {
  id: string
  biz_type: string
  amount_fen: number
  status: number
  pay_expire_at: string
}

export interface PayParams {
  order_id: string
  out_trade_no: string
  prepay_id: string
  amount_fen: number
}

export interface DictItem {
  code: string
  name: string
  parent_code?: string | null
  sort: number
}

export interface SearchParams {
  q?: string
  type?: string
  region?: string
  industry?: string[]
  need?: string[]
  offer?: string[]
  limit?: number
}

export interface ClientOptions {
  baseUrl: string // 如 https://api.example.com/api/mp/v1
  getToken?: () => string | null
  fetchImpl?: typeof fetch
}

/** 极简类型化客户端：按 path 取生成类型，信封 code!==0 抛 ApiError。 */
export function createClient(opts: ClientOptions) {
  const f = opts.fetchImpl ?? fetch

  async function call<T>(
    method: string,
    path: string,
    body?: unknown,
    idempotencyKey?: string,
  ): Promise<T> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    const token = opts.getToken?.()
    if (token) headers['Authorization'] = `Bearer ${token}`
    if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey
    const res = await f(opts.baseUrl + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const env = (await res.json()) as Envelope<T>
    if (env.code !== 0) throw new ApiError(env.code, env.message, env.request_id)
    return env.data as T
  }

  const qs = (params: Record<string, string | number | undefined>) => {
    const pairs = Object.entries(params)
      .filter(([, v]) => v !== undefined && v !== '')
      .map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`)
    return pairs.length ? `?${pairs.join('&')}` : ''
  }

  return {
    // ---- 认证 ----
    wechatLogin(code: string, phoneCode?: string, device?: string) {
      return call<LoginResult>('POST', '/auth/wechat', { code, phone_code: phoneCode, device })
    },
    smsCode(phone: string) {
      return call<{ sent: boolean }>('POST', '/auth/phone/code', { phone })
    },
    smsLogin(phone: string, code: string, device?: string) {
      return call<LoginResult>('POST', '/auth/phone', { phone, code, device })
    },
    me() {
      return call<{ user_id: string; status: number }>('GET', '/users/me')
    },
    deactivate() {
      return call<Record<string, never>>('POST', '/users/me/deactivate')
    },

    // ---- 名片 ----
    getMyCard() {
      return call<Card>('GET', '/cards/me')
    },
    updateMyCard(patch: Partial<Card> & { visibility_settings?: Record<string, boolean>; contact_phone?: string }) {
      return call<Card>('PUT', '/cards/me', patch)
    },
    getCard(ref: string) {
      return call<Card>('GET', `/cards/${encodeURIComponent(ref)}`)
    },
    getCompletion() {
      return call<Completion>('GET', '/cards/me/completion')
    },

    // ---- 广场 / 搜索 / 聚合 ----
    getFeed(limit = 20, offset = 0) {
      return call<FeedData>('GET', `/feed${qs({ limit, offset })}`)
    },
    refreshFeed(limit = 20, offset = 0) {
      return call<FeedData>('POST', `/feed/refresh${qs({ limit, offset })}`)
    },
    search(p: SearchParams) {
      return call<{ items: DiscoveryCard[] }>(
        'GET',
        `/search${qs({
          q: p.q,
          type: p.type,
          region: p.region,
          industry: p.industry?.join(','),
          need: p.need?.join(','),
          offer: p.offer?.join(','),
          limit: p.limit,
        })}`,
      )
    },
    getDetail(ref: string) {
      return call<DetailData>('GET', `/aggregate/detail${qs({ ref })}`)
    },
    explainMatch(ref: string) {
      return call<{ ref: string; score: number; reason: MatchReason[] }>(
        'GET',
        `/matches/${encodeURIComponent(ref)}/explain`,
      )
    },

    // ---- 内容 ----
    publishContent(body: Record<string, unknown>) {
      return call<Record<string, unknown>>('POST', '/contents', body)
    },
    submitContent(id: string) {
      return call<Record<string, unknown>>('POST', `/contents/${id}/submit`)
    },
    getContent(id: string) {
      return call<Record<string, unknown>>('GET', `/contents/${id}`)
    },
    myContents(kind?: string) {
      return call<{ items: Record<string, unknown>[] }>('GET', `/contents/mine${qs({ kind })}`)
    },
    closeContent(id: string) {
      return call<Record<string, unknown>>('POST', `/contents/${id}/close`)
    },

    // ---- 关注 / 收藏 ----
    follow(targetRef: string) {
      return call<{ following: boolean }>('POST', '/follows', { target_ref: targetRef })
    },
    unfollow(ref: string) {
      return call<{ following: boolean }>('DELETE', `/follows/${encodeURIComponent(ref)}`)
    },
    following() {
      return call<{ items: FollowItem[] }>('GET', '/follows')
    },
    followers() {
      return call<{ items: FollowItem[] }>('GET', '/followers')
    },
    favorite(targetRef: string) {
      return call<{ favorited: boolean }>('POST', '/favorites', { target_ref: targetRef })
    },
    unfavorite(ref: string) {
      return call<{ favorited: boolean }>('DELETE', `/favorites/${encodeURIComponent(ref)}`)
    },
    favorites(type = '') {
      return call<{ items: FavoriteItem[] }>('GET', `/favorites${qs({ type })}`)
    },
    followUnread() {
      return call<{ unread: number }>('GET', '/follow-unread')
    },

    // ---- 消息 ----
    ensureSession(targetRef: string) {
      return call<{ session: Session; created: boolean }>('POST', '/sessions', { target_ref: targetRef })
    },
    listSessions() {
      return call<{ items: Session[] }>('GET', '/sessions')
    },
    sendMessage(sessionId: string, content: string, msgType = 1) {
      return call<Message>('POST', `/sessions/${sessionId}/messages`, { msg_type: msgType, content })
    },
    syncMessages(sessionId: string, cursor = 0, limit = 20) {
      return call<{ items: Message[]; next_cursor: number; has_more: boolean }>(
        'GET',
        `/sessions/${sessionId}/messages${qs({ cursor, limit })}`,
      )
    },
    markRead(sessionId: string, readSeq: number) {
      return call<{ read: boolean }>('POST', `/sessions/${sessionId}/read`, { read_seq: readSeq })
    },

    // ---- 会员 ----
    membershipQuote(level: number, years: number) {
      return call<MembershipQuote>('GET', `/membership/quote${qs({ level, years })}`)
    },
    openMembership(level: number, years: number) {
      return call<{ order_id: string; amount_fen: number; original_fen: number }>(
        'POST',
        '/membership/orders',
        { level, years },
      )
    },
    myMembership() {
      return call<{ member: Member; active: boolean }>('GET', '/membership/me')
    },
    subAccounts() {
      return call<{ items: unknown[] }>('GET', '/membership/sub-accounts')
    },
    inviteSub(subUserId: string) {
      return call<{ invited: boolean }>('POST', '/membership/sub-accounts', { sub_user_id: subUserId })
    },
    removeSub(subUserId: string) {
      return call<{ removed: boolean }>('DELETE', `/membership/sub-accounts/${subUserId}`)
    },

    // ---- 交易 ----
    createOrder(bizType: string, amountFen: number, priceSnapshot: Record<string, unknown>, idem: string) {
      return call<Order>('POST', '/orders', {
        biz_type: bizType,
        amount_fen: amountFen,
        price_snapshot: priceSnapshot,
      }, idem)
    },
    payOrder(orderId: string) {
      return call<PayParams>('POST', `/orders/${orderId}/pay`)
    },
    getOrder(orderId: string) {
      return call<Order>('GET', `/orders/${orderId}`)
    },

    // ---- 字典 ----
    listDict(dimension: string, parentCode?: string) {
      return call<{ items: DictItem[] }>('GET', `/dict/${dimension}${qs({ parent_code: parentCode })}`)
    },
  }
}

export type Client = ReturnType<typeof createClient>
