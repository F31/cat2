import { useEffect, useRef, useState } from 'react'
import Taro, { useRouter } from '@tarojs/taro'
import { View, Text, Input, Button, ScrollView } from '@tarojs/components'
import type { Message } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

// 单聊会话页：cursor 增量同步；前台 3 秒轮询、无新消息指数退避（至 30 秒）；发送后本地追加。
export default function Chat() {
  const router = useRouter()
  const sessionId = router.params.session ?? ''
  const peer = decodeURIComponent(router.params.peer ?? '')

  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [lastId, setLastId] = useState('')
  const cursorRef = useRef(0)

  const append = (incoming: Message[]) => {
    if (incoming.length === 0) return false
    setMessages((prev) => {
      const seen = new Set(prev.map((m) => m.id))
      const merged = [...prev, ...incoming.filter((m) => !seen.has(m.id))]
      if (merged.length) setLastId(merged[merged.length - 1].id)
      return merged
    })
    return true
  }

  const initialLoad = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.syncMessages(sessionId, 0, 50)
      cursorRef.current = res.next_cursor
      append(res.items)
      if (res.next_cursor) void api.markRead(sessionId, res.next_cursor)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (peer) Taro.setNavigationBarTitle({ title: peer })
    void initialLoad()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId])

  // 轮询（指数退避）
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | null = null
    let delay = 3000
    let cancelled = false
    const tick = async () => {
      try {
        const res = await api.syncMessages(sessionId, cursorRef.current, 20)
        if (append(res.items)) {
          cursorRef.current = res.next_cursor
          void api.markRead(sessionId, res.next_cursor)
          delay = 3000
        } else {
          delay = Math.min(delay * 2, 30000)
        }
      } catch {
        /* 轮询失败保持静默，下次重试 */
      }
      if (!cancelled) timer = setTimeout(tick, delay)
    }
    void tick()
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId])

  const send = async () => {
    const content = input.trim()
    if (!content) return
    try {
      await ensureLogin()
      const msg = await api.sendMessage(sessionId, content)
      append([msg])
      setInput('')
    } catch {
      Taro.showToast({ title: '发送失败', icon: 'none' })
    }
  }

  return (
    <View className="chat">
      <StateView loading={loading} error={error} onRetry={() => void initialLoad()}>
        <ScrollView scrollY className="chat__list" scrollIntoView={lastId ? `m-${lastId}` : ''} scrollWithAnimation>
          {messages.map((m) => (
            <View key={m.id} id={`m-${m.id}`} className="chat__bubble-wrap">
              <Text className="chat__bubble">{m.content}</Text>
            </View>
          ))}
        </ScrollView>
      </StateView>
      <View className="chat__input-row">
        <Input
          className="chat__input"
          value={input}
          placeholder="输入消息"
          confirmType="send"
          onInput={(e) => setInput(e.detail.value)}
          onConfirm={() => void send()}
        />
        <Button className="chat__send" size="mini" onClick={() => void send()}>
          发送
        </Button>
      </View>
    </View>
  )
}
