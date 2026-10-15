import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { FeedData } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import ContentCard from '../../components/ContentCard'
import ProfilePromptBar from '../../components/ProfilePromptBar'

const PAGE = 20

// 广场 Tab：搜索框 + 完善资料提示条 + 匹配卡片流 + 换一批（单次聚合接口，读预计算候选）。
export default function Square() {
  const [feed, setFeed] = useState<FeedData | null>(null)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(false)

  const load = async (nextOffset: number, append: boolean) => {
    setLoading(true)
    try {
      await ensureLogin()
      const data = nextOffset === 0 ? await api.getFeed(PAGE, 0) : await api.refreshFeed(PAGE, nextOffset)
      setFeed((prev) =>
        append && prev ? { ...data, items: [...prev.items, ...data.items] } : data,
      )
      setOffset(nextOffset)
    } catch {
      Taro.showToast({ title: '加载失败，请重试', icon: 'none' })
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load(0, false)
  })

  const items = feed?.items ?? []

  return (
    <View className="page">
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

      {items.length === 0 && !loading ? (
        <View className="empty">
          <Text className="empty__text">暂无匹配，完善资料后为你推荐</Text>
        </View>
      ) : (
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
      )}

      <Button className="btn-more" loading={loading} onClick={() => void load(offset + PAGE, true)}>
        换一批
      </Button>
    </View>
  )
}
