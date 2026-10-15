import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import type { Cube } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

// 猫立方 Tab：空态（创建/输入邀请码）或立方列表；点开进入详情。
export default function CubeTab() {
  const [cubes, setCubes] = useState<Cube[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [code, setCode] = useState('')

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.listCubes()
      setCubes(res.items ?? [])
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
      const cube = await api.joinCube(code.trim())
      setCode('')
      Taro.navigateTo({ url: `/pages/cube/detail?id=${cube.id}` })
    } catch (e) {
      const code2 = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code2 === 130006 ? '已被移除，须重新邀请' : '邀请码无效', icon: 'none' })
    }
  }

  return (
    <View className="page">
      <StateView loading={loading} error={error} onRetry={() => void load()}>
        {cubes.length === 0 && !error ? (
          <View className="empty">
            <Text className="empty__title">你还没有加入猫立方</Text>
            <Text className="empty__text">猫立方是多方伙伴的协作空间，让合作从「聊过」变成「办成」</Text>
            <Button className="btn-primary" onClick={() => Taro.navigateTo({ url: '/pages/cube/create' })}>
              创建猫立方
            </Button>
            <View className="search-row" style={{ marginTop: '20px' }}>
              <Input className="search-row__input" value={code} placeholder="输入邀请码" onInput={(e) => setCode(e.detail.value)} />
              <Button className="search-row__btn" size="mini" onClick={() => void join()}>
                加入
              </Button>
            </View>
            <Text className="empty__hint">被邀请 + 已实名即可进入，不要求会员等级</Text>
          </View>
        ) : (
          <View className="menu">
            {cubes.map((c) => (
              <View key={c.id} className="menu__item menu__row" onClick={() => Taro.navigateTo({ url: `/pages/cube/detail?id=${c.id}` })}>
                <Text>{c.name}</Text>
                <Text className="row-tag">{c.member_count} 人</Text>
              </View>
            ))}
          </View>
        )}
      </StateView>
    </View>
  )
}
