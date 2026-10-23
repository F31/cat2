import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { FeedData } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import ContentCard from '../../components/ContentCard'
import ProfilePromptBar from '../../components/ProfilePromptBar'
import StateView from '../../components/StateView'
import CircleChannel from '../../components/CircleChannel'

const PAGE = 20
const CHANNEL_KEY = 'catcube.discover.channel'

// 发现 Tab：外层导航壳，内层分「广场 / 圈子」两个频道；广场承载个体/组织发布与合作。
export default function Square() {
  const [feed, setFeed] = useState<FeedData | null>(null)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)
  const [channel, setChannel] = useState<'square' | 'circle'>('square')
  const [circleRefresh, setCircleRefresh] = useState(0)

  const load = async (nextOffset: number, append: boolean) => {
    setLoading(true)
    try {
      await ensureLogin()
      const data = nextOffset === 0 ? await api.getFeed(PAGE, 0) : await api.refreshFeed(PAGE, nextOffset)
      setFeed((prev) =>
        append && prev ? { ...data, items: [...prev.items, ...data.items] } : data,
      )
      setOffset(nextOffset)
      setError(false)
    } catch {
      if (!append) setError(true)
      Taro.showToast({ title: '加载失败，请重试', icon: 'none' })
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    const target = Taro.getStorageSync<string>(CHANNEL_KEY)
    if (target === 'circle') {
      setChannel('circle')
      Taro.removeStorageSync(CHANNEL_KEY)
    }
    setCircleRefresh((k) => k + 1)
    void load(0, false)
  })

  const openChannel = (next: 'square' | 'circle') => {
    setChannel(next)
    if (next === 'circle') setCircleRefresh((k) => k + 1)
  }

  const items = feed?.items ?? []

  return (
    <View className="page">
      <View className="channel-switch">
        <Text
          className={channel === 'square' ? 'channel-switch__item channel-switch__item--active' : 'channel-switch__item'}
          onClick={() => openChannel('square')}
        >
          广场
        </Text>
        <Text
          className={channel === 'circle' ? 'channel-switch__item channel-switch__item--active' : 'channel-switch__item'}
          onClick={() => openChannel('circle')}
        >
          圈子
        </Text>
      </View>
      <Text className="channel-hint">
        {channel === 'square' ? '个体与组织的公开发布与合作' : '主题化群体聚合与合作'}
      </Text>

      {channel === 'square' ? (
        <>
          <View className="search-row">
            <View className="searchbar" onClick={() => Taro.navigateTo({ url: '/pages/search/index' })}>
              <Text className="searchbar__placeholder">搜索人 / 企业 / 项目 / 产品 / 会议 / 活动</Text>
            </View>
            <Button className="search-row__btn" size="mini" onClick={() => Taro.navigateTo({ url: '/pages/content/publish' })}>
              发布
            </Button>
          </View>

          <ProfilePromptBar
            completion={feed?.profile_progress}
            onTap={() => Taro.switchTab({ url: '/pages/mine/index' })}
          />

          <StateView
            loading={loading && items.length === 0}
            error={error}
            empty={!error && items.length === 0}
            emptyText="暂无匹配，完善资料后为你推荐"
            onRetry={() => void load(0, false)}
          >
            <View className="card-list">
              {items.map((c) => (
                <ContentCard
                  key={c.ref}
                  card={c}
                  onClick={(card) =>
                    Taro.navigateTo({ url: `/pages/content/detail?ref=${encodeURIComponent(card.ref)}` })
                  }
                />
              ))}
            </View>
          </StateView>

          <Button className="btn-more" loading={loading} onClick={() => void load(offset + PAGE, true)}>
            换一批
          </Button>
        </>
      ) : (
        <CircleChannel refreshKey={circleRefresh} />
      )}
    </View>
  )
}
