import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import type { Notice } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

// 系统通知：入圈审核、猫立方成员/任务/报备、分摊成团等。
export default function Notices() {
  const [items, setItems] = useState<Notice[]>([])
  const [unread, setUnread] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.listNotices(false, 50)
      setItems(res.items ?? [])
      setUnread(res.unread ?? 0)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const open = async (n: Notice) => {
    if (!n.is_read) {
      try {
        await api.readNotice(n.id)
        setItems((prev) => prev.map((x) => (x.id === n.id ? { ...x, is_read: true } : x)))
        setUnread((v) => Math.max(0, v - 1))
      } catch {
        /* 忽略 */
      }
    }
    const ref = n.ref || ''
    if (ref.startsWith('circle:')) {
      Taro.navigateTo({ url: `/pages/circle/detail?id=${ref.slice(7)}` })
    } else if (ref.startsWith('cube:')) {
      Taro.navigateTo({ url: `/pages/cube/detail?id=${ref.slice(5)}` })
    }
  }

  return (
    <View className="page">
      <Text className="form-field__label">未读 {unread} 条</Text>
      <StateView loading={loading} error={error} empty={items.length === 0} emptyText="暂无通知" onRetry={() => void load()}>
        <View className="menu">
          {items.map((n) => (
            <View key={n.id} className="menu__item" onClick={() => void open(n)}>
              <View className="menu__row">
                <Text>{n.title}</Text>
                {!n.is_read ? <Text className="row-tag">未读</Text> : null}
              </View>
              <Text className="detail-block__body">{n.content}</Text>
              <Text className="form-field__label">{n.created_at?.slice(0, 16).replace('T', ' ')}</Text>
            </View>
          ))}
        </View>
      </StateView>
    </View>
  )
}
