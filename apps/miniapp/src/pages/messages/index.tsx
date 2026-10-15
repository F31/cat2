import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import type { Session } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'

// 消息 Tab：会话列表（系统通知置顶留后续）；未读数来自后端 SQL（最大序号−游标）。
export default function Messages() {
  const [sessions, setSessions] = useState<Session[]>([])

  const load = async () => {
    try {
      await ensureLogin()
      const res = await api.listSessions()
      setSessions(res.items ?? [])
    } catch {
      Taro.showToast({ title: '加载失败', icon: 'none' })
    }
  }

  useDidShow(() => {
    void load()
  })

  return (
    <View className="page">
      {sessions.length === 0 ? (
        <View className="empty">
          <Text className="empty__text">暂无消息</Text>
        </View>
      ) : (
        <View className="session-list">
          {sessions.map((s) => (
            <View
              key={s.id}
              className="session-item"
              onClick={() => Taro.showToast({ title: '会话页后续接入', icon: 'none' })}
            >
              <View className="session-item__main">
                <Text className="session-item__title">{s.peer?.title || `用户 ${s.peer?.user_id ?? ''}`}</Text>
                <Text className="session-item__sub">共 {s.last_seq} 条</Text>
              </View>
              {s.unread > 0 ? <Text className="badge">{s.unread}</Text> : null}
            </View>
          ))}
        </View>
      )}
    </View>
  )
}
