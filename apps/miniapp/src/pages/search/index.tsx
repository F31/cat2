import { useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import type { DiscoveryCard } from '@catcube/api-client'
import { api } from '../../services/request'
import { parseSearch, isAiUnavailable, AI_SEARCH } from '../../services/ai'
import AiEntry from '../../ai-entry'
import ContentCard from '../../components/ContentCard'
import StateView from '../../components/StateView'

const TABS: { label: string; value: string }[] = [
  { label: '全部', value: '' },
  { label: '人', value: 'user' },
  { label: '企业', value: 'org' },
  { label: '项目', value: 'content.project' },
  { label: '产品', value: 'content.product' },
  { label: '会议', value: 'content.meeting' },
  { label: '活动', value: 'content.event' },
]

// 搜索页：关键词与标签独立可用；空态「没有找到相关内容」，错误可重试。
export default function Search() {
  const [q, setQ] = useState('')
  const [type, setType] = useState('')
  const [items, setItems] = useState<DiscoveryCard[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)

  // AI 智能搜索：自然语言 → 结构化条件；失败（030001）回退关键词搜索并提示
  const runAI = async () => {
    if (!q.trim()) {
      Taro.showToast({ title: '请先描述你要找什么', icon: 'none' })
      return
    }
    setLoading(true)
    try {
      const parsed = await parseSearch(q)
      const nextType = parsed.type ?? ''
      setType(nextType)
      if (parsed.keyword) setQ(parsed.keyword)
      const res = await api.search({
        q: parsed.keyword || q,
        type: nextType || undefined,
        region: parsed.region,
        industry: parsed.industry,
        need: parsed.need,
        offer: parsed.offer,
        limit: 20,
      })
      setItems(res.items ?? [])
      setError(false)
    } catch (e) {
      if (isAiUnavailable(e)) {
        Taro.showToast({ title: 'AI 暂不可用，已按关键词搜索', icon: 'none' })
        await run(type)
      } else {
        setError(true)
        Taro.showToast({ title: '搜索失败', icon: 'none' })
      }
    } finally {
      setLoading(false)
    }
  }

  const run = async (nextType = type) => {
    setLoading(true)
    try {
      const res = await api.search({ q, type: nextType || undefined, limit: 20 })
      setItems(res.items ?? [])
      setError(false)
    } catch {
      setError(true)
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
      <AiEntry
        flag={AI_SEARCH}
        hint="用一句话描述，例如「上海找钣金供应商」"
        buttonLabel="AI 智能搜索"
        validate={() => (q.trim() ? null : '请先描述你要找什么')}
        run={() => runAI()}
      />

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

      <StateView
        loading={loading && items === null}
        error={error && items === null}
        empty={items !== null && items.length === 0}
        emptyText="没有找到相关内容"
        onRetry={() => void run()}
      >
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
      </StateView>
      <View style={{ height: loading ? '0' : '0' }} />
    </View>
  )
}
