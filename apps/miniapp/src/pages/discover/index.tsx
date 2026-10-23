import { useState } from 'react'
import Taro, { useDidShow, usePullDownRefresh, useReachBottom } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { FeedData } from '@catcube/api-client'
import { api } from '../../services/request'
import ContentCard from '../../components/ContentCard'
import ProfilePromptBar from '../../components/ProfilePromptBar'
import StateView from '../../components/StateView'
import CircleChannel from '../../components/CircleChannel'
import { getToken } from '../../utils/token'

const PAGE = 20
const CHANNEL_KEY = 'catcube.discover.channel'
const FEED_TYPES = [
  { value: '', label: '全部' },
  { value: 'user', label: '人' },
  { value: 'org', label: '企业' },
  { value: 'content.project', label: '项目' },
  { value: 'content.product', label: '产品' },
  { value: 'content.meeting', label: '会议' },
  { value: 'content.event', label: '活动' },
]

// 发现 Tab：外层导航壳，内层分「广场 / 圈子」两个频道；广场承载个体/组织发布与合作。
export default function Square() {
  const [feed, setFeed] = useState<FeedData | null>(null)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)
  const [channel, setChannel] = useState<'square' | 'circle'>('square')
  const [circleRefresh, setCircleRefresh] = useState(0)
  const [feedType, setFeedType] = useState('')
  const [hasMore, setHasMore] = useState(true)

  const load = async (nextOffset: number, append: boolean, typ = feedType) => {
    setLoading(true)
    try {
      const data = nextOffset === 0
        ? await api.getFeed(PAGE, 0, typ || undefined)
        : await api.refreshFeed(PAGE, nextOffset, typ || undefined)
      setFeed((prev) =>
        append && prev ? { ...data, items: [...prev.items, ...data.items] } : data,
      )
      setHasMore(data.items.length >= PAGE)
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
    void load(0, false, feedType)
  })

  const openChannel = (next: 'square' | 'circle') => {
    setChannel(next)
    if (next === 'circle') setCircleRefresh((k) => k + 1)
  }

  const selectType = (next: string) => {
    setFeedType(next)
    setOffset(0)
    setHasMore(true)
    void load(0, false, next)
  }

  useReachBottom(() => {
    if (channel === 'square' && hasMore && !loading) void load(offset + PAGE, true)
  })

  usePullDownRefresh(async () => {
    if (channel === 'square') await load(0, false)
    Taro.stopPullDownRefresh()
  })

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

          {getToken() && feed ? (
            <ProfilePromptBar
              completion={feed.profile_progress}
              onTap={() => Taro.navigateTo({ url: '/pages/card/edit' })}
            />
          ) : null}

          <View className="feed-filter">
            {FEED_TYPES.map((t) => (
              <Text
                key={t.value || 'all'}
                className={feedType === t.value ? 'feed-filter__item feed-filter__item--active' : 'feed-filter__item'}
                onClick={() => selectType(t.value)}
              >
                {t.label}
              </Text>
            ))}
          </View>

          <StateView
            loading={loading && items.length === 0}
            error={error}
            empty={!error && items.length === 0}
            emptyText={feedType ? '该类型暂无内容' : getToken() ? '暂无匹配，完善资料后为你推荐' : '暂无公开内容，登录后可获得匹配推荐'}
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

          {hasMore ? (
            <Button className="btn-more" loading={loading} onClick={() => void load(offset + PAGE, true)}>
              换一批
            </Button>
          ) : null}
        </>
      ) : (
        <CircleChannel refreshKey={circleRefresh} />
      )}
    </View>
  )
}
