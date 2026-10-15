import { useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import type { DiscoveryCard } from '@catcube/api-client'
import { api } from '../../services/request'
import ContentCard from '../../components/ContentCard'

const TABS: { label: string; value: string }[] = [
  { label: '全部', value: '' },
  { label: '人', value: 'user' },
  { label: '企业', value: 'org' },
  { label: '项目', value: 'content.project' },
  { label: '产品', value: 'content.product' },
  { label: '会议', value: 'content.meeting' },
  { label: '活动', value: 'content.event' },
]

// 搜索页：关键词与标签独立可用；空态「没有找到相关内容」。
export default function Search() {
  const [q, setQ] = useState('')
  const [type, setType] = useState('')
  const [items, setItems] = useState<DiscoveryCard[] | null>(null)
  const [loading, setLoading] = useState(false)

  const run = async (nextType = type) => {
    setLoading(true)
    try {
      const res = await api.search({ q, type: nextType || undefined, limit: 20 })
      setItems(res.items ?? [])
    } catch {
      Taro.showToast({ title: '搜索失败', icon: 'none' })
    } finally {
      setLoading(false)
    }
  }

  return (
    <View className="page">
      <View className="search-row">
        <Input
          className="search-row__input"
          value={q}
          placeholder="输入名称关键词"
          confirmType="search"
          onInput={(e) => setQ(e.detail.value)}
          onConfirm={() => void run()}
        />
        <Button className="search-row__btn" size="mini" onClick={() => void run()}>
          搜索
        </Button>
      </View>

      <View className="tabs">
        {TABS.map((t) => (
          <Text
            key={t.value || 'all'}
            className={t.value === type ? 'tabs__item tabs__item--active' : 'tabs__item'}
            onClick={() => {
              setType(t.value)
              void run(t.value)
            }}
          >
            {t.label}
          </Text>
        ))}
      </View>

      {items && items.length === 0 ? (
        <View className="empty">
          <Text className="empty__text">没有找到相关内容</Text>
        </View>
      ) : (
        <View className="card-list">
          {(items ?? []).map((c) => (
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
      <View style={{ height: loading ? '0' : '0' }} />
    </View>
  )
}
