import { useEffect, useState, type ReactNode } from 'react'
import Taro, { useRouter, useShareAppMessage } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { ApiError, type DetailData, type SignupState } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import { getToken } from '../../utils/token'
import StateView from '../../components/StateView'
import SubscribeAsk from '../../components/SubscribeAsk'
import PolicyGate from '../../components/PolicyGate'

const LEVEL_LABEL: Record<number, string> = { 1: 'VIP 会员', 2: 'SVIP 会员', 3: '企业版' }
const SIGNUP_KINDS = new Set(['event', 'meeting'])
const TIMED_KEYS = new Set(['start_at', 'end_at', 'location', 'venue_point', 'participant_limit', 'participant_count', 'signup_method', 'poster_file_id', 'intro'])
const FIELD_LABELS: Record<string, string> = {
  amount_wan: '金额（万元）',
  stage: '阶段',
  core_strength: '核心优势',
  key_points: '关键点',
  intro: '简介',
  target_customers: '目标客户',
  target_channels: '目标渠道',
  carousel_file_ids: '产品图片',
  venue_point: '场馆点位',
  participant_limit: '人数上限',
  participant_count: '已报名人数',
  signup_method: '报名方式',
  start_at: '开始时间',
  end_at: '结束时间',
  location: '地点',
}
const HIDDEN_FIELDS = new Set(['warehouse_status'])

// 六类详情页共用壳（详细设计 5.6）：基本信息/认证标识/标签/需求供给/核心优势/简介；
// 底部「关注Ta / 联系Ta（会员）」；游客可看，联系方式仅会员可见。
export default function ContentDetail() {
  const router = useRouter()
  const ref = decodeURIComponent(router.params.ref ?? '')
  const [detail, setDetail] = useState<DetailData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [contactOpen, setContactOpen] = useState(false)
  const [phone, setPhone] = useState<string | null>(null)
  const [justFollowed, setJustFollowed] = useState(false)
  const [signup, setSignup] = useState<SignupState | null>(null)
  const [myUserId, setMyUserId] = useState('')
  const [mounting, setMounting] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      const data = await api.getDetail(ref)
      setDetail(data)
      const kind = data.content?.kind
      if (getToken()) {
        try {
          const me = await api.me()
          setMyUserId(me.user_id)
        } catch {
          setMyUserId('')
        }
      }
      if (data.content && kind && SIGNUP_KINDS.has(kind) && getToken()) {
        try {
          setSignup(await api.signupState(data.content.id))
        } catch {
          setSignup(null)
        }
      }
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    void Taro.showShareMenu({ withShareTicket: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useShareAppMessage(() => {
    const fc = detail?.from_circle
    const base = `/pages/content/detail?ref=${encodeURIComponent(ref)}`
    return {
      title: fc ? `来自「${fc.name}」的分享` : detail?.card?.name ?? String(detail?.content?.title ?? '猫平方'),
      path: fc ? `${base}&from_circle=${fc.id}` : base,
    }
  })

  const joinFromCircle = async () => {
    if (!detail?.from_circle) return
    if (detail.from_circle.joined) {
      Taro.navigateTo({ url: `/pages/circle/detail?id=${detail.from_circle.id}` })
      return
    }
    try {
      await ensureLogin()
      await api.applyCircle(detail.from_circle.id)
      Taro.showToast({ title: '已申请加入', icon: 'success' })
      void load()
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 140003 ? '需先完成实名认证' : '加入失败', icon: 'none' })
    }
  }

  const toggleFollow = async () => {
    if (!detail) return
    try {
      await ensureLogin()
      if (detail.following) {
        await api.unfollow(ref)
        setJustFollowed(false)
      } else {
        await api.follow(ref)
        setJustFollowed(true) // 关注=订阅：提示后续动态通知
      }
      setDetail({ ...detail, following: !detail.following })
    } catch {
      Taro.showToast({ title: '操作失败', icon: 'none' })
    }
  }

  const toggleSignup = async () => {
    if (!detail?.content || detail.preview_only) return
    try {
      await ensureLogin()
      const id = detail.content.id
      const res = signup?.signed_up ? await api.cancelSignup(id) : await api.signupContent(id)
      setSignup(res)
      if (res.signed_up) {
        Taro.showToast({ title: '报名成功', icon: 'success' })
      } else {
        Taro.showToast({ title: '已取消报名', icon: 'success' })
      }
    } catch (e) {
      if (e instanceof ApiError && e.code === 110007) {
        Taro.showToast({ title: '名额已满', icon: 'none' })
      } else if (e instanceof ApiError && e.code === 110008) {
        Taro.showToast({ title: '该内容不支持报名', icon: 'none' })
      } else if (e instanceof ApiError && e.code === 110009) {
        Taro.showToast({ title: '报名已截止', icon: 'none' })
      } else {
        Taro.showToast({ title: '操作失败，请重试', icon: 'none' })
      }
    }
  }

  const mountToCircle = async () => {
    if (!detail?.content || mounting) return
    setMounting(true)
    try {
      await ensureLogin()
      const res = await api.listCircles()
      const circles = res.items ?? []
      if (circles.length === 0) {
        Taro.showToast({ title: '请先创建或加入圈子', icon: 'none' })
        return
      }
      const pick = await Taro.showActionSheet({ itemList: circles.map((c) => c.name) }).catch(() => null)
      if (!pick) return
      const circle = circles[pick.tapIndex]
      if (!circle) return
      const r = await api.mountCircleItem(circle.id, ref)
      Taro.showToast({ title: r.direct ? '已挂载到圈子' : '已提交挂载申请', icon: 'success' })
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({
        title: code === 140010 ? '该圈挂载已满' : code === 140002 ? '请先加入该圈子' : '挂载失败',
        icon: 'none',
      })
    } finally {
      setMounting(false)
    }
  }

  // 联系Ta：会员权益（揭示手机号）；非会员引导开通。
  const revealContact = async () => {
    try {
      await ensureLogin()
      const res = await api.contactOf(ref)
      setPhone(res.phone)
      setContactOpen(true)
    } catch (e) {
      if (e instanceof ApiError && e.code === 120007) {
        Taro.navigateTo({ url: '/pages/member/open' })
      } else if (e instanceof ApiError && e.code === 2) {
        Taro.showToast({ title: '请先登录', icon: 'none' })
      } else {
        Taro.showToast({ title: '操作失败，请重试', icon: 'none' })
      }
    }
  }

  const openSession = async () => {
    try {
      await ensureLogin()
      const res = await api.ensureSession(ref)
      const title = detail?.card?.name ?? String(detail?.content?.title ?? '会话')
      Taro.navigateTo({
        url: `/pages/msg/chat?session=${encodeURIComponent(res.session.id)}&peer=${encodeURIComponent(title)}`,
      })
    } catch (e) {
      if (e instanceof ApiError && e.code === 70004) {
        Taro.navigateTo({ url: '/pages/member/open' })
      } else {
        Taro.showToast({ title: '发起会话失败', icon: 'none' })
      }
    }
  }

  if (!detail) {
    return (
      <View className="page">
        <StateView loading={loading} error={error} onRetry={() => void load()} />
      </View>
    )
  }

  const card = detail.card
  const content = detail.content
  const title = card?.name ?? String(content?.title ?? '未命名')
  const signupEnded = !!content && SIGNUP_KINDS.has(content.kind) && isPast(content.fields?.end_at)
  const isOwner = !!content && !!myUserId && detail.owner?.user_id === myUserId

  return (
    <View className="page">
      <View className="detail-head">
        <Text className="detail-head__title">{title}</Text>
        <Text className="detail-head__type">
          {card ? (card.identity_type === 2 ? '企业/机构' : '个人') : detail.type}
          {card?.region_code ? ` · ${card.region_code}` : ''}
          {content?.region_code ? ` · ${content.region_code}` : ''}
        </Text>
        {detail.badges && detail.badges.length > 0 ? (
          <View className="detail-badges">
            {detail.badges.map((b) => (
              <Text key={b.code} className="tag tag--verified">
                {b.label}
              </Text>
            ))}
            {detail.member ? (
              <Text className="tag tag--member">{LEVEL_LABEL[detail.member.level] ?? '会员'}</Text>
            ) : null}
          </View>
        ) : null}
      </View>

      {detail.from_circle ? (
        <View className="detail-block" style={{ background: '#f6f8ff', borderRadius: '12px', padding: '16px' }}>
          <Text className="detail-block__title">来自「{detail.from_circle.name}」</Text>
          <Text className="detail-block__body">
            {detail.preview_only ? '加入圈子后可查看全文' : detail.from_circle.joined ? '你已在该圈中' : '加入圈子，参与讨论'}
          </Text>
          <Button className="btn-primary" disabled={!detail.from_circle.can_join && !detail.from_circle.joined} onClick={() => void joinFromCircle()}>
            {detail.from_circle.joined ? '进入圈子' : detail.from_circle.can_join ? '加入圈子' : '该圈已结束'}
          </Button>
        </View>
      ) : null}

      {card ? (
        <>
          {card.industry_codes && card.industry_codes.length > 0 ? (
            <Block title="行业标签">
              <View className="chips">
                {card.industry_codes.map((c) => (
                  <Text key={c} className="chip chip--tag">
                    {c}
                  </Text>
                ))}
              </View>
            </Block>
          ) : null}
          {card.target_customers ? <Block title="目标客户">{card.target_customers}</Block> : null}
          {card.need_codes && card.need_codes.length > 0 ? (
            <Block title="需求">
              <View className="chips">
                {card.need_codes.map((c) => (
                  <Text key={c} className="chip chip--tag">
                    {c}
                  </Text>
                ))}
              </View>
            </Block>
          ) : null}
          {card.offer_codes && card.offer_codes.length > 0 ? (
            <Block title="供给">
              <View className="chips">
                {card.offer_codes.map((c) => (
                  <Text key={c} className="chip chip--tag">
                    {c}
                  </Text>
                ))}
              </View>
            </Block>
          ) : null}
          {card.advantage_field ? <Block title="优势领域">{card.advantage_field}</Block> : null}
          {card.background ? <Block title="背景身份">{card.background}</Block> : null}
          {card.core_strength ? <Block title="核心优势">{card.core_strength}</Block> : null}
          {card.intro ? <Block title="简介">{card.intro}</Block> : null}
        </>
      ) : null}

      {content ? (
        <>
          {content.region_code ? <Block title="区域">{content.region_code}</Block> : null}
          {SIGNUP_KINDS.has(content.kind) ? (
            <View className="detail-block">
              <Text className="detail-block__title">活动信息</Text>
              <Text className="detail-block__body">{timedInfo(content.fields ?? {})}</Text>
              <Text className="detail-block__body" style={{ marginTop: '12px' }}>
                已报名 {signup?.count ?? Number(content.fields?.participant_count ?? 0)} 人
                {(signup?.limit ?? Number(content.fields?.participant_limit ?? 0)) > 0
                  ? ` / 上限 ${signup?.limit ?? Number(content.fields?.participant_limit ?? 0)} 人`
                  : ''}
              </Text>
              {!detail.preview_only ? (
                <Button
                  className="btn-primary"
                  style={{ marginTop: '20px' }}
                  disabled={!signup?.signed_up && (signupEnded || ((signup?.limit ?? 0) > 0 && (signup?.count ?? 0) >= (signup?.limit ?? 0)))}
                  onClick={() => void toggleSignup()}
                >
                  {signup?.signed_up
                    ? '已报名，取消报名'
                    : signupEnded
                      ? '报名已截止'
                      : (signup?.limit ?? 0) > 0 && (signup?.count ?? 0) >= (signup?.limit ?? 0)
                        ? '名额已满'
                        : '报名参加'}
                </Button>
              ) : null}
              {isOwner ? (
                <Button
                  className="btn-secondary"
                  style={{ marginTop: '16px' }}
                  onClick={() => Taro.navigateTo({ url: `/pages/content/signups?id=${content.id}` })}
                >
                  查看报名名单
                </Button>
              ) : null}
            </View>
          ) : null}
          {Object.entries(content.fields ?? {})
            .filter(([k]) => !HIDDEN_FIELDS.has(k) && (!SIGNUP_KINDS.has(content.kind) || !TIMED_KEYS.has(k)))
            .map(([k, v]) => (
              <Block key={k} title={FIELD_LABELS[k] ?? k}>
                {formatField(k, v)}
              </Block>
            ))}
          <PolicyGate
            allowed={!(content.member_gate && content.member_gate.length > 0)}
            reason="member.required"
            unlockText="去开通"
            onUnlock={() => Taro.navigateTo({ url: '/pages/member/open' })}
          >
            <View />
          </PolicyGate>
          {isOwner ? (
            <Button className="btn-secondary" loading={mounting} style={{ marginTop: '20px' }} onClick={() => void mountToCircle()}>
              挂载到我的圈子
            </Button>
          ) : null}
          {detail.owner ? <Block title="发布方">{detail.owner.name}</Block> : null}
        </>
      ) : null}

      <SubscribeAsk visible={justFollowed && detail.following} />

      <View className="detail-actions">
        <Button className="btn-secondary" onClick={() => void toggleFollow()}>
          {detail.following ? '已关注' : '关注Ta'}
        </Button>
        <Button className="btn-primary" onClick={() => void revealContact()}>
          联系Ta（会员）
        </Button>
      </View>

      {contactOpen ? (
        <View className="auth-mask">
          <View className="auth-modal">
            <Text className="auth-modal__title">联系方式</Text>
            <Text className="auth-modal__row">{phone ? phone : '对方暂未配置联系方式'}</Text>
            <View className="auth-modal__actions">
              <Button className="btn-secondary auth-modal__btn" onClick={() => setContactOpen(false)}>
                关闭
              </Button>
              <Button className="btn-primary auth-modal__btn" onClick={() => void openSession()}>
                发消息
              </Button>
            </View>
          </View>
        </View>
      ) : null}
    </View>
  )
}

function timedInfo(fields: Record<string, unknown>): string {
  const start = fmtTime(fields.start_at)
  const end = fmtTime(fields.end_at)
  const time = start && end ? `${start} 至 ${end}` : start || end || '待定'
  const lines = [`时间：${time}`]
  if (fields.location) lines.push(`地点：${String(fields.location)}`)
  if (fields.venue_point) lines.push(`场地：${String(fields.venue_point)}`)
  if (fields.signup_method) lines.push(`报名方式：${String(fields.signup_method)}`)
  if (fields.intro) lines.push(`简介：${String(fields.intro)}`)
  return lines.join('\n')
}

function fmtTime(v: unknown): string {
  if (typeof v !== 'string' || !v) return ''
  return v.slice(0, 16).replace('T', ' ')
}

function formatField(key: string, v: unknown): string {
  if (key === 'amount_wan') return `${String(v)} 万`
  if (key === 'carousel_file_ids' && Array.isArray(v)) return v.length > 0 ? `已上传 ${v.length} 张图片` : '未上传'
  if (typeof v === 'string' || typeof v === 'number') return String(v)
  if (Array.isArray(v)) return v.join('、')
  return JSON.stringify(v)
}

function isPast(v: unknown): boolean {
  if (typeof v !== 'string' || !v) return false
  const t = Date.parse(v)
  return Number.isFinite(t) && t < Date.now()
}

function Block({ title, children }: { title: string; children: ReactNode }) {
  return (
    <View className="detail-block">
      <Text className="detail-block__title">{title}</Text>
      <Text className="detail-block__body">{children}</Text>
    </View>
  )
}
