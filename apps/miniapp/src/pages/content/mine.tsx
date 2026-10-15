import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

interface MineItem {
  id: string
  kind: string
  title: string
  status: number
}

const STATUS_LABEL: Record<number, string> = { 0: '审核中', 1: '展示中', 2: '已下架' }

// 我的发布：按类型筛选，点进入详情，右上发布入口。
export default function MyContents() {
  const [kind, setKind] = useState('')
  const [items, setItems] = useState<MineItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async (k: string) => {
    setLoading(true)
    try {
      await ensureLogin()
      const res = await api.myContents(k || undefined)
      setItems((res.items ?? []) as unknown as MineItem[])
      setError(false)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load(kind)
  })

  const kinds = [
    { key: '', label: '全部' },
    { key: 'project', label: '项目' },
    { key: 'product', label: '产品' },
    { key: 'meeting', label: '会议' },
    { key: 'event', label: '活动' },
  ]

  return (
    <View className="page">
      <View className="tabs">
        {kinds.map((k) => (
          <Text
            key={k.key || 'all'}
            className={k.key === kind ? 'tabs__item tabs__item--active' : 'tabs__item'}
            onClick={() => {
              setKind(k.key)
              void load(k.key)
            }}
          >
            {k.label}
          </Text>
        ))}
      </View>

      <StateView
        loading={loading && items.length === 0}
        error={error}
        empty={!error && items.length === 0}
        emptyText="还没有发布内容"
        onRetry={() => void load(kind)}
      >
        <View className="menu">
          {items.map((it) => (
            <View
              key={it.id}
              className="menu__item"
              onClick={() =>
                Taro.navigateTo({ url: `/pages/content/detail?ref=${encodeURIComponent(`content.${it.kind}:${it.id}`)}` })
              }
            >
              <Text>{it.title}</Text>
              <Text className="form-field__label"> {STATUS_LABEL[it.status] ?? it.status}</Text>
            </View>
          ))}
        </View>
      </StateView>

      <Button className="btn-primary" onClick={() => Taro.navigateTo({ url: `/pages/content/publish${kind ? `?kind=${kind}` : ''}` })}>
        发布新内容
      </Button>
    </View>
  )
}
