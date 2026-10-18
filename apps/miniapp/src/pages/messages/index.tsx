import { useCallback, useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import type { Session } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import CursorList from '../../components/CursorList'

// 消息 Tab：系统通知置顶 + 单聊/群聊会话列表；群聊显示 session.title。
export default function Messages() {
  const [refreshKey, setRefreshKey] = useState(0)
  const [noticeUnread, setNoticeUnread] = useState(0)

  // 列表统一游标分页（当前后端一次性返回，next_cursor=null 即到底）。
  const load = useCallback(async (_cursor: string | null) => {
    await ensureLogin()
    const res = await api.listSessions()
    return { items: res.items ?? [], next_cursor: null as string | null }
  }, [])

  useDidShow(() => {
    setRefreshKey((k) => k + 1)
    void ensureLogin()
      .then(() => api.listNotices(true, 1))
      .then((r) => setNoticeUnread(r.unread ?? 0))
      .catch(() => setNoticeUnread(0))
  })

  const openChat = (s: Session) => {
    const title = s.title || s.peer?.title || `用户 ${s.peer?.user_id ?? ''}`
    void Taro.navigateTo({
      url: `/pages/msg/chat?session=${encodeURIComponent(s.id)}&peer=${encodeURIComponent(title)}`,
    })
  }

  return (
    <View className="page">
      <View className="session-item" onClick={() => Taro.navigateTo({ url: '/pages/msg/notices' })}>
        <View className="session-item__main">
          <Text className="session-item__title">系统通知</Text>
          <Text className="session-item__sub">入圈审核、任务报备、分摊结果等</Text>
        </View>
        {noticeUnread > 0 ? <Text className="badge">{noticeUnread}</Text> : null}
      </View>

      <CursorList<Session>
        refreshKey={refreshKey}
        load={load}
        keyOf={(s) => s.id}
        emptyText="暂无消息"
        renderItem={(s) => (
          <View className="session-item session-list__row" onClick={() => openChat(s)}>
            <View className="session-item__main">
              <Text className="session-item__title">{s.title || s.peer?.title || `用户 ${s.peer?.user_id ?? ''}`}</Text>
              <Text className="session-item__sub">共 {s.last_seq} 条</Text>
            </View>
            {s.unread > 0 ? <Text className="badge">{s.unread}</Text> : null}
          </View>
        )}
      />
    </View>
  )
}
