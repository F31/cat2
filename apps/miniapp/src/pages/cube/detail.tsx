import { useState } from 'react'
import { useRouter } from '@tarojs/taro'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { Cube, CubeMember } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

const ROLE: Record<number, string> = { 1: '猫王', 2: '猫蜜', 3: '成员', 4: '共建成员' }

// 猫立方详情：四模块入口（研究/承接/任务/关于我们）+ 成员/订阅与分摊。
export default function CubeDetail() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [cube, setCube] = useState<Cube | null>(null)
  const [members, setMembers] = useState<CubeMember[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const [c, m] = await Promise.all([api.cubeDetail(id), api.cubeMembers(id)])
      setCube(c)
      setMembers(m.items ?? [])
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const invite = async () => {
    try {
      const t = await api.cubeInvite(id)
      Taro.setClipboardData({ data: t.code })
      Taro.showToast({ title: '邀请码已复制', icon: 'success' })
    } catch {
      Taro.showToast({ title: '仅猫王/猫蜜可邀请', icon: 'none' })
    }
  }

  if (!cube && (loading || error)) {
    return (
      <View className="page">
        <StateView loading={loading} error={error} onRetry={() => void load()} />
      </View>
    )
  }
  if (!cube) return null

  return (
    <View className="page">
      <View className="detail-head">
        <Text className="detail-head__title">{cube.name}</Text>
        <Text className="detail-head__type">
          {cube.member_count} 人 · {cube.region_code || '未设区域'} · {cube.plan_level === 1 ? '基础档' : cube.plan_level === 2 ? '进阶档' : '高阶档'}
        </Text>
      </View>

      <View className="menu">
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: `/pages/cube/project?id=${id}&type=1` })}>
          <Text>重点合作闭环研究</Text>
        </View>
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: `/pages/cube/project?id=${id}&type=2` })}>
          <Text>重点项目合作承接</Text>
        </View>
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: `/pages/cube/task?id=${id}` })}>
          <Text>内部工作安排 / 任务</Text>
        </View>
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: `/pages/cube/plan?id=${id}` })}>
          <Text>订阅与分摊</Text>
        </View>
      </View>

      <View className="detail-block">
        <Text className="detail-block__title">关于我们</Text>
        <Text className="detail-block__body">
          方向：{cube.intro || cube.name}{'\n'}
          优势：{cube.intro || '—'}
        </Text>
      </View>

      <View className="detail-block">
        <Text className="detail-block__title">成员（{members.length}）</Text>
        {members.map((m) => (
          <Text key={m.user_id} className="detail-block__body">
            用户 {m.user_id} · {ROLE[m.role] ?? m.role}
          </Text>
        ))}
      </View>

      <View className="detail-actions">
        <Button className="btn-secondary" onClick={() => void invite()}>
          邀请成员
        </Button>
        <Button className="btn-primary" onClick={() => Taro.navigateTo({ url: `/pages/cube/plan?id=${id}` })}>
          订阅 / 分摊
        </Button>
      </View>
    </View>
  )
}
