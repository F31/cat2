import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import type { Circle } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

// 圈子 Tab：我的圈子（我加入的/我创建的，进行中/已结束分区）+ 创建 + 输入邀请码加入。
export default function CircleTab() {
  const [circles, setCircles] = useState<Circle[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [code, setCode] = useState('')

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.listCircles()
      setCircles(res.items ?? [])
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
    if (!code.trim()) return
    try {
      await ensureLogin()
      const c = await api.joinCircle(code.trim())
      setCode('')
      Taro.navigateTo({ url: `/pages/circle/detail?id=${c.id}` })
    } catch {
      Taro.showToast({ title: '邀请链接无效或已失效', icon: 'none' })
    }
  }

  const active = circles.filter((c) => c.status === 0)
  const ended = circles.filter((c) => c.status !== 0)

  const section = (title: string, list: Circle[]) =>
    list.length > 0 ? (
      <View className="detail-block">
        <Text className="detail-block__title">{title}</Text>
        {list.map((c) => (
          <View key={c.id} className="menu__item menu__row" onClick={() => Taro.navigateTo({ url: `/pages/circle/detail?id=${c.id}` })}>
            <Text>{c.name}</Text>
            <Text className="row-tag">
              {c.type === 1 ? '常设' : '临时'} · {c.member_count} 人 · {c.status === 0 ? '进行中' : c.status === 1 ? '已结束' : '已解散'}
            </Text>
          </View>
        ))}
      </View>
    ) : null

  return (
    <View className="page">
      <View className="search-row">
        <Button className="btn-primary" size="mini" onClick={() => Taro.navigateTo({ url: '/pages/circle/create' })}>
          创建圈子
        </Button>
        <Input className="search-row__input" value={code} placeholder="输入邀请码加入" onInput={(e) => setCode(e.detail.value)} />
        <Button className="btn-secondary" size="mini" onClick={() => void join()}>
          加入
        </Button>
      </View>
      <StateView loading={loading} error={error} empty={circles.length === 0} emptyText="还没有圈子，创建一个或输入邀请码加入" onRetry={() => void load()}>
        {section('进行中', active)}
        {section('已结束 / 已解散', ended)}
      </StateView>
    </View>
  )
}
