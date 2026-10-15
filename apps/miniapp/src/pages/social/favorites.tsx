import { useCallback, useEffect, useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import type { FavoriteItem } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

const TABS: { label: string; value: string }[] = [
  { label: '全部', value: '' },
  { label: '人', value: 'person' },
  { label: '企业', value: 'company' },
  { label: '项目', value: 'project' },
  { label: '产品', value: 'product' },
  { label: '会议', value: 'meeting' },
  { label: '活动', value: 'event' },
]

const KIND_LABEL: Record<string, string> = {
  person: '人',
  company: '企业',
  project: '项目',
  product: '产品',
  meeting: '会议',
  event: '活动',
}

// 我的收藏：六类分 Tab；点击进详情，右侧取消收藏。
export default function Favorites() {
  const [kind, setKind] = useState('')
  const [items, setItems] = useState<FavoriteItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = useCallback(async (k: string) => {
    setLoading(true)
    try {
      await ensureLogin()
      const res = await api.favorites(k)
      setItems(res.items ?? [])
      setError(false)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load(kind)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind])

  const unfavorite = async (it: FavoriteItem) => {
    try {
      await api.unfavorite(`${it.target.type}:${it.target.id}`)
      setItems((prev) => prev.filter((x) => x.id !== it.id))
    } catch {
      Taro.showToast({ title: '操作失败', icon: 'none' })
    }
  }

  return (
    <View className="page">
      <View className="tabs">
        {TABS.map((t) => (
          <Text
            key={t.value || 'all'}
            className={t.value === kind ? 'tabs__item tabs__item--active' : 'tabs__item'}
            onClick={() => setKind(t.value)}
          >
            {t.label}
          </Text>
        ))}
      </View>

      <StateView
        loading={loading && items.length === 0}
        error={error}
        empty={!error && items.length === 0}
        emptyText="还没有收藏内容"
        onRetry={() => void load(kind)}
      >
        <View className="menu">
          {items.map((it) => (
            <View
              key={it.id}
              className="menu__item menu__row"
              onClick={() =>
                Taro.navigateTo({
                  url: `/pages/content/detail?ref=${encodeURIComponent(`${it.target.type}:${it.target.id}`)}`,
                })
              }
            >
              <Text>{it.title || `${it.target.type}:${it.target.id}`}</Text>
              <View className="menu__row-extra">
                <Text className="row-tag">{KIND_LABEL[it.kind] ?? it.kind}</Text>
                <Text
                  className="row-action"
                  onClick={(e) => {
                    e.stopPropagation()
                    void unfavorite(it)
                  }}
                >
                  取消收藏
                </Text>
              </View>
            </View>
          ))}
        </View>
      </StateView>
    </View>
  )
}
