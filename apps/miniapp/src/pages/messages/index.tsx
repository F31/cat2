import { useCallback, useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import type { Session } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import CursorList from '../../components/CursorList'

// 消息 Tab：系统通知置顶 + 单聊会话列表；未读数来自后端 SQL（最大序号−游标）。
export default function Messages() {
  const [refreshKey, setRefreshKey] = useState(0)

  // 列表统一游标分页（当前后端一次性返回，next_cursor=null 即到底）。
  const load = useCallback(async (_cursor: string | null) => {
    await ensureLogin()
    const res = await api.listSessions()
    return { items: res.items ?? [], next_cursor: null as string | null }
  }, [])

  useDidShow(() => {
    setRefreshKey((k) => k + 1)
  })

  const openChat = (s: Session) => {
    const title = s.peer?.title || `用户 ${s.peer?.user_id ?? ''}`
    void Taro.navigateTo({
      url: `/pages/msg/chat?session=${encodeURIComponent(s.id)}&peer=${encodeURIComponent(title)}`,
    })
  }

  return (
    <View className="page">
      <View className="session-item" onClick={() => Taro.showToast({ title: '通知中心后续接入', icon: 'none' })}>
        <View className="session-item__main">
          <Text className="session-item__title">系统通知</Text>
          <Text className="session-item__sub">关注的人发布、审核结果等</Text>
        </View>
      </View>

      <CursorList<Session>
        refreshKey={refreshKey}
        load={load}
        keyOf={(s) => s.id}
        emptyText="暂无消息"
        renderItem={(s) => (
          <View className="session-item session-list__row" onClick={() => openChat(s)}>
            <View className="session-item__main">
              <Text className="session-item__title">{s.peer?.title || `用户 ${s.peer?.user_id ?? ''}`}</Text>
              <Text className="session-item__sub">共 {s.last_seq} 条</Text>
            </View>
            {s.unread > 0 ? <Text className="badge">{s.unread}</Text> : null}
          </View>
        )}
      />
    </View>
  )
}
