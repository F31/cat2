import { useEffect, useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import { aiFeatures, askSupport, AI_CS, isAiUnavailable } from '../../services/ai'

// 人工客服电话与工作时间（p5 §5.1：工作时间 9:00-18:00 人工电话不变）。
const SUPPORT_PHONE = '400-000-0000'
const WORK_HOURS = '9:00-18:00'

interface Turn {
  role: 'user' | 'ai'
  text: string
  escalated?: boolean
}

// 客服页：人工客服信息常驻；智能客服（平台规则/操作指引，越界转人工）按 ai.cs 开关展示。
export default function Support() {
  const [on, setOn] = useState(false)
  const [question, setQuestion] = useState('')
  const [busy, setBusy] = useState(false)
  const [turns, setTurns] = useState<Turn[]>([])

  useEffect(() => {
    let alive = true
    void aiFeatures().then((f) => {
      if (alive) setOn(!!f[AI_CS])
    })
    return () => {
      alive = false
    }
  }, [])

  const send = async () => {
    const text = question.trim()
    if (!text) {
      Taro.showToast({ title: '请输入你的问题', icon: 'none' })
      return
    }
    setTurns((t) => [...t, { role: 'user', text }])
    setQuestion('')
    setBusy(true)
    try {
      const r = await askSupport(text)
      setTurns((t) => [...t, { role: 'ai', text: r.answer, escalated: r.escalated }])
    } catch (e) {
      setTurns((t) => [
        ...t,
        {
          role: 'ai',
          text: isAiUnavailable(e)
            ? '智能客服暂不可用，请拨打上方人工客服电话。'
            : '回答失败，请稍后重试或联系人工客服。',
        },
      ])
    } finally {
      setBusy(false)
    }
  }

  return (
    <View className="page">
      <View className="support-head">
        <Text className="support-head__title">人工客服</Text>
        <Text className="support-head__sub">
          电话 {SUPPORT_PHONE} · 工作时间 {WORK_HOURS}
        </Text>
      </View>

      {on ? (
        <View className="support-chat">
          <Text className="ai-entry__hint">智能客服（仅平台规则与操作指引，其他问题转人工）</Text>
          {turns.map((t, i) => (
            <View
              key={`${t.role}-${i}`}
              className={t.role === 'user' ? 'support-msg support-msg--user' : 'support-msg support-msg--ai'}
            >
              <Text>{t.text}</Text>
              {t.escalated ? (
                <Text className="support-msg__hint">已为你转人工，请拨打上方电话或在此留言</Text>
              ) : null}
            </View>
          ))}
          <Input
            className="search-row__input"
            value={question}
            placeholder="请输入你的问题"
            onInput={(e) => setQuestion(e.detail.value)}
          />
          <Button className="btn-secondary" size="mini" disabled={busy} onClick={() => void send()}>
            {busy ? '回复中…' : '发送'}
          </Button>
        </View>
      ) : (
        <View className="support-chat">
          <Text className="ai-entry__hint">如需帮助，请拨打上方人工客服电话。</Text>
        </View>
      )}
    </View>
  )
}
