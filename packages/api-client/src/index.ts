// 本包 schema.d.ts 由 contracts/openapi/*.yaml 经 `make gen` 生成（openapi-typescript），勿手改。
// 本文件为基于生成类型的薄客户端（信封解包、游标分页、Idempotency-Key）。
import type { paths, components } from './schema'
import type { components as adminComponents, paths as adminPaths } from './admin-schema'

export type { paths, components }
// 运营后台契约类型（contracts/openapi/admin.yaml 生成）
export type { adminPaths, adminComponents }

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

export interface ContentItem {
  id: string
  kind: string
  owner_ref: { type: string; id: string }
  title: string
  cover_file_id?: string
  region_code?: string
  status: number
  reject_reason?: string
  published_at?: string | null
  fields: Record<string, unknown>
  member_gate?: string[]
  unlock_reason?: string
}

/** AI 生成草稿字段。 */
/** 单位归属（一人多单位）。 */
export interface Affiliation {
  id: string
  user_id: string
  org_id: string
  role?: string
  is_primary: boolean
  status: number
  reject_reason?: string
}

/** 组织管理员。 */
export interface OrgAdmin {
  org_id: string
  user_id: string
  status: number
  confirmed_at?: number
}

export interface AiField {
  field: string
  value: string
}

/** AI 生成草稿（入驻名片 / 发布内容）。 */
export interface AiDraft {
  call_id: string
  title?: string
  fields: AiField[]
  tokens?: number
}

/** AI 解析出的结构化检索条件。 */
export interface AiSearchQuery {
  call_id: string
  keyword?: string
  type?: string
  region?: string
  industry?: string[]
  need?: string[]
  offer?: string[]
  tokens?: number
}

/** AI 能力开关（按用户灰度）。 */
export interface AiFeatures {
  features: Record<string, boolean>
}

/** 智能客服回复（escalated=true 表示超出知识范围，转人工）。 */
export interface AiCSReply {
  call_id: string
  answer: string
  escalated?: boolean
  tokens?: number
}

/** 渠道运营商身份卡。 */
export interface ChannelOperator {
  id: string
  user_id: string
  region_code: string
  region_name?: string
  name: string
  status: number // 0申请中/1已开通/2已停用
  referrer_operator_id?: string
  opened_at?: number
}

export interface ChannelAttribution {
  id: string
  operator_id: string
  region_code: string
  source: number
  bound_at: number
}

export interface ChannelCommission {
  id: string
  order_id: string
  biz_type: string
  base_fen: number
  ratio_bp: number
  amount_fen: number
  month: string
  status: number
}

export interface ChannelOverview {
  operator: ChannelOperator
  month: string
  month_fen: number
  total_fen: number
  target: { year: number; paid_member_count: number; landing_project_count: number }
  commissions: ChannelCommission[]
}

export interface ChannelLink {
  code: string
  path: string
  region_code: string
}

export interface DetailData {
  ref: string
  type: string
  card?: Card
  content?: ContentItem
  owner?: Card
  badges?: { code: string; label: string; level: number }[]
  member?: Member
  following: boolean
  from_circle?: { id: string; name: string; status: number; joined: boolean; can_join: boolean }
  preview_only?: boolean
}

export interface Session {
  id: string
  kind: number
  peer?: { user_id: string; title?: string; cover?: string }
  last_seq: number
  unread: number
  title?: string
  last_preview?: string
  last_sender_id?: string
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
  // 微信支付调起参数（生产下发；dev 缺省，仅有 out_trade_no）。
  timeStamp?: string
  nonceStr?: string
  package?: string
  signType?: string
  paySign?: string
}

export interface Cert {
  id: string
  user_id: string
  cert_type: number
  target_org_id?: string
  status: number
  reject_reason?: string
  submitted_at: string
  reviewed_at?: string | null
}

export interface CertifyInput {
  cert_type: number
  real_name?: string
  id_card_no?: string
  agreed: boolean
  target_org_id?: string
  material_file_ids?: number[]
  extra?: Record<string, string>
}

// ---- p3 协作（cube/circle/notice）----

export interface Cube {
  id: string
  name: string
  leader_id: string
  honey_id: string
  plan_level: number
  plan_expire_at?: string | null
  status: number
  member_count: number
  region_code?: string
  my_role?: number
  intro?: string
  industry_codes?: string[]
  chain_stage?: string
  direction?: string
  resources?: string
  strength?: string
  cooperation?: string
}

export interface CubeMember {
  user_id: string
  nickname?: string
  role: number
  status: number
  joined_at: string
}

export interface CubeProject {
  id: string
  cube_id: string
  type: number
  content_project_id?: string
  name: string
  related_ref?: string
  related_customer?: string
  leader_id: string
  participants?: string[]
  progress: number
  todo_count: number
  status: number
  source_ref?: string
  platform_status?: string
  created_at?: string
}

export interface CubeTask {
  id: string
  cube_id: string
  project_id?: string | null
  title: string
  assignee_id: string
  progress: number
  confirm_user_id?: string
  due_at?: string | null
  status: number
  reject_reason?: string
  reopened_count: number
}

export interface CubeReport {
  id: string
  cube_id: string
  project_id?: string | null
  reporter_id: string
  customer_name: string
  customer_type?: string
  region_code?: string
  amount_wan?: number
  note?: string
  status: number
  reject_reason?: string
}

export interface CubeResearch {
  id: string
  project_id: string
  content: string
  decision?: string
  operator_id: string
  created_at: string
}

export interface CubePlan {
  level: number
  name: string
  cube_limit: number
  member_limit: number
  price_fen: number
}

export interface CubeApportion {
  id: string
  cube_id: string
  per_fen: number
  target_n: number
  pledged_count: number
  deadline: string
  status: number
}

export interface Circle {
  id: string
  name: string
  type: number
  status: number
  owner_user_id: string
  member_count: number
  end_at?: string | null
  intro?: string
  region_code?: string
  my_role?: number
  joined?: boolean
  recommendable?: boolean
  apply_status?: number
  apply_reason?: string
  created_at?: string
}

export interface CircleDiscoverItem extends Circle {
  intro?: string
  region_code?: string
  owner_name?: string
  created_at?: string
}

export interface CircleMember {
  user_id: string
  nickname?: string
  role: number
  joined_via?: number
  joined_at: string
}

export interface CircleFeed {
  id: string
  circle_id: string
  author_id: string
  author_name?: string
  text: string
  image_file_ids?: string[]
  is_announcement: boolean
  created_at: string
}

export interface CircleApplication {
  id: string
  circle_id: string
  applicant_id: string
  nickname?: string
  status: number
  reason?: string
  created_at: string
}

export interface CubeLinkRequest {
  id: string
  cube_id: string
  content_project_id: string
  initiator_id: string
  target_role: number
  status: number
  created_at: string
}

export interface CircleDemand {
  id: string
  circle_id: string
  author_id: string
  title: string
  content: string
  demand_type?: string
  region_code?: string
  industry_codes?: string[]
  expire_at: string
  scope: number
  status: number
  contact_phone?: string
  transfer_ref?: string
  created_at: string
}

export interface CircleMount {
  id: string
  circle_id: string
  item_ref: string
  mounted_by: string
  mount_type: number
  approval_status: number
  status: number
}

export interface Notice {
  id: string
  template_code: string
  title: string
  content: string
  ref?: string
  channel: string
  is_read: boolean
  created_at: string
}

export interface NoticePreference {
  template_code: string
  enabled: boolean
  closeable: boolean
}

export interface UploadTicket {
  file_id: string
  upload_url: string
  expires_in: number
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
    wechatLogin(code: string, phoneCode?: string, device?: string, identityType?: 1 | 2) {
      return call<LoginResult>('POST', '/auth/wechat', {
        code,
        phone_code: phoneCode,
        device,
        identity_type: identityType,
      })
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
    contactOf(ref: string) {
      return call<{ ref: string; phone: string }>('GET', `/aggregate/contact${qs({ ref })}`)
    },
    // ---- AI（p5 框架；不可用时抛 code=30001，前端回退原表单/关键词搜索） ----
    // ---- 认证全类型与组织机制（p4 §6） ----
    reviewCertByOrg(id: string, pass: boolean, reason = '') {
      return call<{ reviewed: boolean }>('POST', `/certs/${id}/review`, { pass, reason })
    },
    applyAffiliation(orgId: string, role: string, proofFileIds: string[] = []) {
      return call<Affiliation>('POST', '/affiliations', { org_id: orgId, role, proof_file_ids: proofFileIds })
    },
    myAffiliations() {
      return call<{ items: Affiliation[] }>('GET', '/affiliations/mine')
    },
    pendingAffiliations(orgId: string, limit = 50) {
      return call<{ items: Affiliation[] }>('GET', `/affiliations/pending?org_id=${encodeURIComponent(orgId)}&limit=${limit}`)
    },
    verifyAffiliation(id: string, pass: boolean, reason = '') {
      return call<{ verified: boolean }>('POST', `/affiliations/${id}/verify`, { pass, reason })
    },
    setPrimaryAffiliation(id: string) {
      return call<{ primary: boolean }>('POST', `/affiliations/${id}/primary`, {})
    },
    orgAdmins(orgId: string) {
      return call<{ items: OrgAdmin[] }>('GET', `/orgs/${orgId}/admins`)
    },
    inviteOrgAdmin(orgId: string, userId: string) {
      return call<{ invited: boolean }>('POST', `/orgs/${orgId}/admins`, { user_id: userId })
    },
    confirmOrgAdmin(inviteId: string) {
      return call<{ confirmed: boolean }>('POST', `/org-admin-invites/${inviteId}/confirm`, {})
    },

    aiFeatures() {
      return call<AiFeatures>('GET', '/ai/features')
    },
    aiOnboardDraft(text: string) {
      return call<AiDraft>('POST', '/ai/onboard-draft', { text })
    },
    aiPublishDraft(kind: string, text: string) {
      return call<AiDraft>('POST', '/ai/publish-draft', { kind, text })
    },
    aiSearch(text: string) {
      return call<AiSearchQuery>('POST', '/ai/search', { text })
    },
    aiFeedback(callId: string, adopted: boolean) {
      return call<{ ok: boolean }>('POST', '/ai/feedback', { call_id: callId, adopted })
    },
    aiCustomerService(question: string) {
      return call<AiCSReply>('POST', '/ai/cs', { question })
    },

    // ---- 渠道（运营商）----
    channelOperator() {
      return call<{ operator: ChannelOperator; is_operator: boolean }>('GET', '/channel/operator')
    },
    channelApply(regionCode: string, name: string, referrerOperatorId?: string) {
      return call<{ operator: ChannelOperator }>('POST', '/channel/operator/apply', {
        region_code: regionCode,
        name,
        referrer_operator_id: referrerOperatorId,
      })
    },
    channelAttribution() {
      return call<{ attribution?: ChannelAttribution; bound: boolean }>('GET', '/channel/attribution')
    },
    channelBindAttribution(code: string) {
      return call<{ attribution: ChannelAttribution }>('POST', '/channel/attribution', { code })
    },
    channelOverview(month?: string) {
      return call<ChannelOverview>('GET', `/channel/overview${month ? `?month=${month}` : ''}`)
    },
    channelCommissions(month?: string) {
      return call<{ items: ChannelCommission[] }>('GET', `/channel/commissions${month ? `?month=${month}` : ''}`)
    },
    channelLink() {
      return call<ChannelLink>('GET', '/channel/link')
    },
    channelWithdraw() {
      return call<{ settlement_id: string }>('POST', '/channel/withdraw')
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

    // ---- 认证 ----
    submitCert(body: CertifyInput) {
      return call<Cert>('POST', '/certs', body)
    },
    myCerts() {
      return call<{ items: Cert[] }>('GET', '/certs/mine')
    },

    // ---- 媒体 ----
    createUpload(filename: string, contentType: string, size: number) {
      return call<UploadTicket>('POST', '/media/uploads', { filename, content_type: contentType, size })
    },

    // ---- 猫立方 ----
    listCubes() {
      return call<{ items: Cube[] }>('GET', '/cubes')
    },
    cubeHome() {
      return call<{ cubes: Cube[]; todo_count: number; unread: number }>('GET', '/aggregate/cube-home')
    },
    createCube(body: Record<string, unknown>) {
      return call<Cube>('POST', '/cubes', body)
    },
    cubeDetail(id: string) {
      return call<Cube>('GET', `/cubes/${id}`)
    },
    updateCube(id: string, body: Record<string, unknown>) {
      return call<{ updated: boolean }>('PUT', `/cubes/${id}`, body)
    },
    lookupCubeHoney(phone: string) {
      return call<{ found: boolean; user_id?: string; nickname?: string }>('GET', `/cubes/honey-lookup${qs({ phone })}`)
    },
    ensureCubeChat(id: string) {
      return call<{ session_id: string; title: string }>('POST', `/cubes/${id}/chat`)
    },
    ensureCubeProjectChat(id: string, pid: string) {
      return call<{ session_id: string; title: string }>('POST', `/cubes/${id}/projects/${pid}/chat`)
    },
    cubeInvite(id: string) {
      return call<{ code: string; expire_at: string }>('POST', `/cubes/${id}/invites`)
    },
    joinCube(code: string) {
      return call<Cube>('POST', '/cubes/join', { code })
    },
    cubeMembers(id: string) {
      return call<{ items: CubeMember[] }>('GET', `/cubes/${id}/members`)
    },
    removeCubeMember(id: string, uid: string, reason = '') {
      return call<{ removed: boolean }>('POST', `/cubes/${id}/members/${uid}/remove`, { reason })
    },
    leaveCube(id: string) {
      return call<{ left: boolean }>('POST', `/cubes/${id}/leave`)
    },
    transferCube(id: string, toUserId: string) {
      return call<{ transferred: boolean }>('POST', `/cubes/${id}/transfer`, { to_user_id: toUserId })
    },
    setCubeHoney(id: string, userId: string) {
      return call<{ honey_id: string }>('POST', `/cubes/${id}/honey`, { user_id: userId })
    },
    dissolveCube(id: string) {
      return call<{ dissolved: boolean }>('POST', `/cubes/${id}/dissolve`)
    },
    cubeProjects(id: string, type?: number) {
      return call<{ items: CubeProject[] }>('GET', `/cubes/${id}/projects${qs({ type })}`)
    },
    createCubeProject(id: string, body: Record<string, unknown>) {
      return call<CubeProject>('POST', `/cubes/${id}/projects`, body)
    },
    closeCubeProject(id: string, pid: string) {
      return call<{ closed: boolean }>('POST', `/cubes/${id}/projects/${pid}/close`)
    },
    reopenCubeProject(id: string, pid: string) {
      return call<{ reopened: boolean }>('POST', `/cubes/${id}/projects/${pid}/reopen`)
    },
    cubeResearch(id: string, pid: string) {
      return call<{ items: CubeResearch[] }>('GET', `/cubes/${id}/projects/${pid}/research`)
    },
    addCubeResearch(id: string, pid: string, content: string, decision = '') {
      return call<CubeResearch>('POST', `/cubes/${id}/projects/${pid}/research`, { content, decision })
    },
    cubeTasks(id: string, projectId?: string) {
      return call<{ items: CubeTask[] }>('GET', `/cubes/${id}/tasks${qs({ project_id: projectId })}`)
    },
    createCubeTask(id: string, body: Record<string, unknown>) {
      return call<CubeTask>('POST', `/cubes/${id}/tasks`, body)
    },
    cubeTaskAction(id: string, tid: string, action: string, body: Record<string, unknown> = {}) {
      return call<unknown>('POST', `/cubes/${id}/tasks/${tid}/${action}`, body)
    },
    submitCubeReport(id: string, body: Record<string, unknown>) {
      return call<{ report: CubeReport; duplicate: boolean }>('POST', `/cubes/${id}/reports`, body)
    },
    cubeReports(id: string, projectId?: string) {
      return call<{ items: CubeReport[] }>('GET', `/cubes/${id}/reports${qs({ project_id: projectId })}`)
    },
    decideCubeReport(id: string, rid: string, approve: boolean, reason = '') {
      return call<unknown>('POST', `/cubes/${id}/reports/${rid}/${approve ? 'confirm' : 'reject'}`, { reason })
    },
    cubePlans() {
      return call<{ items: CubePlan[] }>('GET', '/cubes/plans')
    },
    changeCubePlan(id: string, level: number, years = 1) {
      return call<{ order_id: string; amount_fen: number }>('POST', `/cubes/${id}/plan`, { level, years })
    },
    initiateApportion(id: string, level: number, n: number, memberIds: string[] = []) {
      return call<CubeApportion>('POST', `/cubes/${id}/apportion`, { level, n, member_ids: memberIds })
    },
    pledgeApportion(id: string, groupId: string) {
      return call<{ order_id: string }>('POST', `/cubes/${id}/apportion/pledge`, { group_id: groupId })
    },
    exitApportion(id: string, groupId: string) {
      return call<{ exited: boolean }>('POST', `/cubes/${id}/apportion/exit`, { group_id: groupId })
    },
    cubeApportion(groupId: string) {
      return call<CubeApportion>('GET', `/cube-apportions/${groupId}`)
    },
    requestCubeLink(id: string, contentProjectId: string) {
      return call<{ direct: boolean; project_id: string; request_id: string }>('POST', `/cubes/${id}/link-requests`, { content_project_id: contentProjectId })
    },
    cubeLinkRequests(id: string) {
      return call<{ items: CubeLinkRequest[] }>('GET', `/cubes/${id}/link-requests`)
    },
    decideCubeLink(id: string, reqId: string, approve: boolean) {
      return call<{ project_id: string; approved: boolean }>('POST', `/cubes/${id}/link-requests/${reqId}/decide`, { approve })
    },
    publishCubeProject(id: string, pid: string) {
      return call<{ content_project_id: string }>('POST', `/cubes/${id}/projects/${pid}/publish`)
    },

    // ---- 圈子 ----
    listCircles() {
      return call<{ items: Circle[] }>('GET', '/circles')
    },
    discoverCircles(p: { q?: string; region?: string; type?: number; sort?: string; cursor?: string; limit?: number } = {}) {
      return call<{ items: CircleDiscoverItem[]; next_cursor: string | null }>(
        'GET',
        `/circles/discover${qs({ q: p.q, region: p.region, type: p.type, sort: p.sort, cursor: p.cursor, limit: p.limit })}`,
      )
    },
    circleDetail(id: string) {
      return call<Circle>('GET', `/circles/${id}`)
    },
    circleDetailAgg(id: string) {
      return call<{ circle: Circle; owner?: unknown; member_count: number; joined: boolean }>('GET', `/aggregate/circle-detail${qs({ ref: `circle:${id}` })}`)
    },
    createCircle(body: Record<string, unknown>) {
      return call<Circle>('POST', '/circles', body)
    },
    applyCircle(id: string) {
      return call<{ applied: boolean }>('POST', `/circles/${id}/apply`)
    },
    circleInvite(id: string) {
      return call<{ code: string; expire_at: string }>('POST', `/circles/${id}/invite`)
    },
    resetCircleInvite(id: string) {
      return call<{ invalidated: number }>('POST', `/circles/${id}/invite/reset`)
    },
    circleCard(id: string) {
      return call<{
        circle_id: string
        name: string
        intro: string
        member_count: number
        owner_name: string
        invite_code: string
        qr_png_base64: string
      }>('GET', `/circles/${id}/card`)
    },
    joinCircle(code: string) {
      return call<Circle>('POST', '/circles/join', { code })
    },
    circleMembers(id: string) {
      return call<{ items: CircleMember[] }>('GET', `/circles/${id}/members`)
    },
    listCircleFeeds(id: string, limit = 30) {
      return call<{ items: CircleFeed[] }>('GET', `/circles/${id}/feeds${qs({ limit })}`)
    },
    listCircleApplications(id: string, status = 1) {
      return call<{ items: CircleApplication[] }>('GET', `/circles/${id}/applications${qs({ status })}`)
    },
    decideCircleApplication(id: string, uid: string, approve: boolean, reason = '') {
      return call<{ approved: boolean }>('POST', `/circles/${id}/applications/${uid}/decide`, { approve, reason })
    },
    dissolveCircle(id: string) {
      return call<{ dissolved: boolean }>('POST', `/circles/${id}/dissolve`)
    },
    convertCircle(id: string) {
      return call<{ converted: boolean }>('POST', `/circles/${id}/convert`)
    },
    listCircleDemands(id: string, limit = 20) {
      return call<{ items: CircleDemand[] }>('GET', `/circles/${id}/demands${qs({ limit })}`)
    },
    publishCircleDemand(id: string, body: Record<string, unknown>) {
      return call<CircleDemand>('POST', `/circles/${id}/demands`, body)
    },
    renewCircleDemand(did: string) {
      return call<{ renewed: boolean }>('POST', `/circles/demands/${did}/renew`)
    },
    publishCircleFeed(id: string, text: string, isAnnouncement = false) {
      return call<{ id: string }>('POST', `/circles/${id}/feeds`, { text, is_announcement: isAnnouncement })
    },
    mountCircleItem(id: string, itemRef: string) {
      return call<{ mount: CircleMount; direct: boolean }>('POST', `/circles/${id}/mounts`, { item_ref: itemRef })
    },
    circleMounts(id: string) {
      return call<{ items: CircleMount[] }>('GET', `/circles/${id}/mounts`)
    },
    decideCircleMount(mid: string, approve: boolean) {
      return call<{ approved: boolean }>('POST', `/circles/mounts/${mid}/decide`, { approve })
    },
    demandToCube(id: string, did: string, cubeId: string) {
      return call<{ project_id: string }>('POST', `/circles/${id}/demands/${did}/to-cube`, { cube_id: cubeId })
    },

    // ---- 通知中心 ----
    listNotices(unreadOnly = false, limit = 20) {
      return call<{ items: Notice[]; unread: number }>('GET', `/notices${qs({ unread: unreadOnly ? 1 : undefined, limit })}`)
    },
    readNotice(id: string) {
      return call<{ read: boolean }>('POST', `/notices/${id}/read`)
    },
    noticePreferences() {
      return call<{ items: NoticePreference[] }>('GET', '/notice-preferences')
    },
    setNoticePreference(templateCode: string, enabled: boolean) {
      return call<{ saved: boolean }>('PUT', '/notice-preferences', { template_code: templateCode, enabled })
    },
    applySubscribeQuota(templateCode: string, n = 1) {
      return call<{ applied: boolean }>('POST', '/subscribe-quota/apply', { template_code: templateCode, n })
    },
  }
}

export type Client = ReturnType<typeof createClient>
