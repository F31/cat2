import { useCallback, useEffect, useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import type { FollowItem } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

type TabKey = 'following' | 'followers'

// 关注与粉丝：两个 Tab；点击进对方详情；不设互相关注统计。
export default function Follows() {
  const [tab, setTab] = useState<TabKey>('following')
  const [items, setItems] = useState<FollowItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = useCallback(async (t: TabKey) => {
    setLoading(true)
    try {
      await ensureLogin()
      const res = t === 'following' ? await api.following() : await api.followers()
      setItems(res.items ?? [])
      setError(false)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load(tab)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab])

  return (
    <View className="page">
      <View className="tabs">
        <Text className={tab === 'following' ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setTab('following')}>
          我的关注
        </Text>
        <Text className={tab === 'followers' ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setTab('followers')}>
          我的粉丝
        </Text>
      </View>

      <StateView
        loading={loading && items.length === 0}
        error={error}
        empty={!error && items.length === 0}
        emptyText={tab === 'following' ? '还没有关注的人' : '还没有粉丝'}
        onRetry={() => void load(tab)}
      >
        <View className="menu">
          {items.map((it, i) => (
            <View
              key={`${it.follower_id}-${it.target.type}-${it.target.id}-${i}`}
              className="menu__item"
              onClick={() =>
                Taro.navigateTo({
                  url: `/pages/content/detail?ref=${encodeURIComponent(`${it.target.type}:${it.target.id}`)}`,
                })
              }
            >
              <Text>{it.title || `${it.target.type}:${it.target.id}`}</Text>
            </View>
          ))}
        </View>
      </StateView>
    </View>
  )
}
