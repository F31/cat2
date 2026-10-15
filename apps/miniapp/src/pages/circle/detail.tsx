import { useState } from 'react'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { Circle, CircleDemand, CircleMount } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

const DEMAND_STATUS: Record<number, string> = { 0: '有效', 1: '已过期', 2: '已删除' }

// 圈详情：成员见四区块（关于本圈/圈内动态/挂载内容/成员）；非成员（含未登录）仅「关于本圈」+「申请加入」。
export default function CircleDetail() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [circle, setCircle] = useState<Circle | null>(null)
  const [demands, setDemands] = useState<CircleDemand[]>([])
  const [mounts, setMounts] = useState<CircleMount[]>([])
  const [members, setMembers] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      const c = await api.circleDetail(id) // 游客可看
      setCircle(c)
      const joined = c.joined || (c.my_role ?? 0) > 0
      if (joined) {
        await ensureLogin()
        const [d, m, ms] = await Promise.all([api.listCircleDemands(id), api.circleMounts(id), api.circleMembers(id)])
        setDemands(d.items ?? [])
        setMounts(m.items ?? [])
        setMembers(ms.items ?? [])
      }
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const join = async () => {
    try {
      await ensureLogin()
      await api.applyCircle(id)
      Taro.showToast({ title: '已提交申请', icon: 'success' })
      void load()
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 140003 ? '需先完成实名认证' : '申请失败', icon: 'none' })
    }
  }

  const share = async () => {
    try {
      await ensureLogin()
      const t = await api.circleInvite(id)
      Taro.setClipboardData({ data: t.code })
      Taro.showToast({ title: '邀请链接已复制（来自本圈）', icon: 'success' })
    } catch {
      Taro.showToast({ title: '仅成员可邀请', icon: 'none' })
    }
  }

  if (!circle && (loading || error)) {
    return (
      <View className="page">
        <StateView loading={loading} error={error} onRetry={() => void load()} />
      </View>
    )
  }
  if (!circle) return null
  const joined = circle.joined || (circle.my_role ?? 0) > 0

  return (
    <View className="page">
      <View className="detail-head">
        <Text className="detail-head__title">{circle.name}</Text>
        <Text className="detail-head__type">
          {circle.type === 1 ? '常设圈' : '临时圈'} · {circle.member_count} 人 · {circle.status === 0 ? '进行中' : circle.status === 1 ? '已结束' : '已解散'}
        </Text>
      </View>

      <View className="detail-block">
        <Text className="detail-block__title">关于本圈</Text>
        <Text className="detail-block__body">{circle.intro || '暂无简介'}</Text>
      </View>

      {!joined ? (
        <>
          <View className="detail-actions">
            <Button className="btn-secondary" onClick={() => void share()}>
              邀请 / 分享
            </Button>
            <Button className="btn-primary" onClick={() => void join()}>
              申请加入
            </Button>
          </View>
          <Text className="empty__hint">非成员仅可见「关于本圈」；加入后可查看动态、挂载与成员。</Text>
        </>
      ) : (
        <>
          <View className="detail-block">
            <Text className="detail-block__title">圈内动态 / 需求</Text>
            {demands.map((d) => (
              <Text key={d.id} className="detail-block__body">
                {d.title} · {DEMAND_STATUS[d.status] ?? d.status}
                {d.transfer_ref ? ' · 已转入猫立方办' : ''}
              </Text>
            ))}
            {demands.length === 0 ? <Text className="form-field__label">暂无内容</Text> : null}
          </View>

          <View className="detail-block">
            <Text className="detail-block__title">挂载内容（{mounts.length}）</Text>
            {mounts.map((m) => (
              <Text key={m.id} className="detail-block__body" onClick={() => Taro.navigateTo({ url: `/pages/content/detail?ref=${encodeURIComponent(m.item_ref)}` })}>
                {m.item_ref} {m.approval_status === 0 ? '（待确认）' : ''}
              </Text>
            ))}
            {mounts.length === 0 ? <Text className="form-field__label">暂无挂载</Text> : null}
          </View>

          <View className="detail-block">
            <Text className="detail-block__title">成员（{members.length}）</Text>
            <Text className="detail-block__body">{members.map((u) => `用户${u}`).join('、')}</Text>
          </View>

          <View className="detail-actions">
            <Button className="btn-secondary" onClick={() => void share()}>
              邀请 / 分享
            </Button>
            <Button className="btn-primary" onClick={() => Taro.navigateTo({ url: `/pages/circle/publish?id=${id}` })}>
              发布需求 / 动态
            </Button>
          </View>
          <Button className="btn-secondary" onClick={() => Taro.navigateTo({ url: `/pages/circle/settings?id=${id}` })}>
            圈设置
          </Button>
        </>
      )}
    </View>
  )
}
