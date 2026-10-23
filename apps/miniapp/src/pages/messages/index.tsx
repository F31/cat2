import { useCallback, useEffect, useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import type { Session } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import CursorList from '../../components/CursorList'

function formatSessionTime(value?: string | null): string {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  const now = new Date()
  const sameDay =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate()
  if (sameDay) {
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
  }
  return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

// 消息 Tab：系统通知置顶 + 单聊/群聊会话列表；展示最后消息与时间，并同步 Tab 未读角标。
export default function Messages() {
  const [refreshKey, setRefreshKey] = useState(0)
  const [noticeUnread, setNoticeUnread] = useState(0)
  const [sessionUnread, setSessionUnread] = useState(0)

  // 列表统一游标分页（当前后端一次性返回，next_cursor=null 即到底）。
  const load = useCallback(async (_cursor: string | null) => {
    await ensureLogin()
    const res = await api.listSessions()
    const items = res.items ?? []
    setSessionUnread(items.reduce((sum, s) => sum + (s.unread || 0), 0))
    return { items, next_cursor: null as string | null }
  }, [])

  useEffect(() => {
    const total = noticeUnread + sessionUnread
    if (total > 0) {
      void Taro.setTabBarBadge({ index: 2, text: total > 99 ? '99+' : String(total) }).catch(() => undefined)
    } else {
      void Taro.removeTabBarBadge({ index: 2 }).catch(() => undefined)
    }
  }, [noticeUnread, sessionUnread])

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
              <Text className="session-item__preview">{s.last_preview || '暂无消息'}</Text>
            </View>
            <View className="session-item__meta">
              <Text className="session-item__time">{formatSessionTime(s.last_message_at)}</Text>
              {s.unread > 0 ? <Text className="badge">{s.unread}</Text> : null}
            </View>
          </View>
        )}
      />
    </View>
  )
}
