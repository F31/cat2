import { useEffect, useState } from 'react'
import Taro, { useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { ApiError, type DetailData } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

// 六类详情页共用壳：名片/内容 + Badges + 简介 + 关注状态；底部「关注Ta / 联系Ta（会员）」。
export default function ContentDetail() {
  const router = useRouter()
  const ref = decodeURIComponent(router.params.ref ?? '')
  const [detail, setDetail] = useState<DetailData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

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

  const contact = async () => {
    if (!detail) return
    try {
      await ensureLogin()
      const res = await api.ensureSession(ref)
      const title = detail.card?.name ?? String(detail.content?.title ?? '会话')
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

  const title = detail.card?.name ?? String(detail.content?.title ?? '未命名')

  return (
    <View className="page">
      <View className="detail-head">
        <Text className="detail-head__title">{title}</Text>
        <Text className="detail-head__type">{detail.type}</Text>
      </View>

      {detail.badges && detail.badges.length > 0 ? (
        <View className="detail-badges">
          {detail.badges.map((b) => (
            <Text key={b.code} className="tag tag--verified">
              {b.label}
            </Text>
          ))}
        </View>
      ) : null}

      {detail.card?.intro ? <Text className="detail-intro">{detail.card.intro}</Text> : null}

      <View className="detail-actions">
        <Button className="btn-secondary" onClick={() => void toggleFollow()}>
          {detail.following ? '已关注' : '关注Ta'}
        </Button>
        <Button className="btn-primary" onClick={() => void contact()}>
          联系Ta（会员）
        </Button>
      </View>
    </View>
  )
}
