import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { type SignupItem } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

const KIND_LABEL: Record<string, string> = { event: '活动', meeting: '会议' }

// 我的活动：当前用户已报名的活动/会议，点开进入内容详情。
export default function MyActivities() {
  const [items, setItems] = useState<SignupItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      await ensureLogin()
      const res = await api.mySignups()
      setItems(res.items ?? [])
      setError(false)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const open = (item: SignupItem) => {
    Taro.navigateTo({ url: `/pages/content/detail?ref=${encodeURIComponent(`content.${item.item.kind}:${item.item.id}`)}` })
  }

  return (
    <View className="page">
      <StateView
        loading={loading}
        error={error}
        empty={!error && items.length === 0}
        emptyText="还没有报名活动或会议"
        onRetry={() => void load()}
      >
        <View className="menu">
          {items.map((it) => (
            <View key={`${it.item.kind}:${it.item.id}`} className="menu__item" onClick={() => open(it)}>
              <View className="menu__row">
                <Text>{it.item.title}</Text>
                <Text className="row-tag">{KIND_LABEL[it.item.kind] ?? '内容'}</Text>
              </View>
              <Text className="form-field__label">
                {fmtRange(it.item.fields?.start_at, it.item.fields?.end_at)}
                {it.item.fields?.location ? ` · ${String(it.item.fields.location)}` : ''}
              </Text>
              <Text className="form-field__label">报名时间：{fmtTime(it.signed_up_at)}</Text>
            </View>
          ))}
        </View>
      </StateView>
    </View>
  )
}

function fmtRange(start: unknown, end: unknown): string {
  const a = fmtTime(start)
  const b = fmtTime(end)
  if (a && b) return `${a} 至 ${b}`
  return a || b || '时间待定'
}

function fmtTime(v: unknown): string {
  if (typeof v !== 'string' || !v) return ''
  return v.slice(0, 16).replace('T', ' ')
}
