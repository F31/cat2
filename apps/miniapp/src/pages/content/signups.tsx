import { useState } from 'react'
import { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { type SignupUser } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

// 主办方查看活动/会议报名名单（仅内容所有者可访问）。
export default function ContentSignups() {
  const id = useRouter().params.id ?? ''
  const [items, setItems] = useState<SignupUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      await ensureLogin()
      const res = await api.contentSignups(id)
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

  return (
    <View className="page">
      <View className="detail-block">
        <Text className="detail-block__title">报名人数：{items.length}</Text>
        <Text className="form-field__label">仅主办方可见；名单按报名时间排序。</Text>
      </View>
      <StateView
        loading={loading}
        error={error}
        empty={!error && items.length === 0}
        emptyText="还没有人报名"
        onRetry={() => void load()}
      >
        <View className="menu">
          {items.map((u) => (
            <View key={u.user_id} className="menu__item">
              <View className="menu__row">
                <Text>{u.nickname || `用户 ${u.user_id}`}</Text>
                <Text className="row-tag">已报名</Text>
              </View>
              <Text className="form-field__label">
                报名时间：{u.signed_up_at?.slice(0, 16).replace('T', ' ') || '—'}
              </Text>
            </View>
          ))}
        </View>
      </StateView>
    </View>
  )
}
