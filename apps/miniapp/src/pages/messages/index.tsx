import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import type { Session } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

// 消息 Tab：系统通知置顶 + 单聊会话列表；未读数来自后端 SQL（最大序号−游标）。
export default function Messages() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.listSessions()
      setSessions(res.items ?? [])
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
      <View className="session-item" onClick={() => Taro.showToast({ title: '通知中心后续接入', icon: 'none' })}>
        <View className="session-item__main">
          <Text className="session-item__title">系统通知</Text>
          <Text className="session-item__sub">关注的人发布、审核结果等</Text>
        </View>
      </View>

      <StateView loading={loading} error={error} empty={sessions.length === 0} emptyText="暂无消息" onRetry={() => void load()}>
        <View className="session-list">
          {sessions.map((s) => {
            const title = s.peer?.title || `用户 ${s.peer?.user_id ?? ''}`
            return (
              <View
                key={s.id}
                className="session-item"
                onClick={() =>
                  Taro.navigateTo({
                    url: `/pages/msg/chat?session=${encodeURIComponent(s.id)}&peer=${encodeURIComponent(title)}`,
                  })
                }
              >
                <View className="session-item__main">
                  <Text className="session-item__title">{title}</Text>
                  <Text className="session-item__sub">共 {s.last_seq} 条</Text>
                </View>
                {s.unread > 0 ? <Text className="badge">{s.unread}</Text> : null}
              </View>
            )
          })}
        </View>
      </StateView>
    </View>
  )
}
