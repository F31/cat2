import { useEffect, useState, type ReactNode } from 'react'
import Taro, { useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { ApiError, type DetailData } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

const LEVEL_LABEL: Record<number, string> = { 1: 'VIP 会员', 2: 'SVIP 会员', 3: '企业版' }

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

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      const data = await api.getDetail(ref)
      setDetail(data)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const toggleFollow = async () => {
    if (!detail) return
    try {
      await ensureLogin()
      if (detail.following) await api.unfollow(ref)
      else await api.follow(ref)
      setDetail({ ...detail, following: !detail.following })
    } catch {
      Taro.showToast({ title: '操作失败', icon: 'none' })
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
          {Object.entries(content.fields ?? {}).map(([k, v]) => (
            <Block key={k} title={k}>
              {typeof v === 'string' || typeof v === 'number' ? String(v) : JSON.stringify(v)}
            </Block>
          ))}
          {content.member_gate && content.member_gate.length > 0 ? (
            <View className="policy-gate">
              <Text className="policy-gate__hint">开通会员可见：{content.member_gate.join('、')}</Text>
              <Text className="policy-gate__action" onClick={() => Taro.navigateTo({ url: '/pages/member/open' })}>
                去开通
              </Text>
            </View>
          ) : null}
          {detail.owner ? <Block title="发布方">{detail.owner.name}</Block> : null}
        </>
      ) : null}

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

function Block({ title, children }: { title: string; children: ReactNode }) {
  return (
    <View className="detail-block">
      <Text className="detail-block__title">{title}</Text>
      <Text className="detail-block__body">{children}</Text>
    </View>
  )
}
