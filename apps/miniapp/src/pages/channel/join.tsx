import { useEffect, useState } from 'react'
import Taro, { useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { api } from '../../services/request'

// 邀请落地页（p4 §1.3/§1.4）：按邀请码绑定归属（幂等，单一有效）。
export default function ChannelJoin() {
  const router = useRouter()
  const code = String(router.params.code ?? '')
  const [state, setState] = useState<'loading' | 'ok' | 'fail'>('loading')
  const [msg, setMsg] = useState('')

  useEffect(() => {
    if (!code) {
      setState('fail')
      setMsg('缺少邀请码')
      return
    }
    void api
      .channelBindAttribution(code)
      .then(() => {
        setState('ok')
        setMsg('邀请归属已绑定，感谢支持！')
      })
      .catch((e) => {
        setState('fail')
        setMsg(e instanceof Error ? e.message : '绑定失败')
      })
  }, [code])

  return (
    <View className="page">
      <View className="support-head">
        <Text className="support-head__title">邀请加入</Text>
        <Text className="support-head__sub">{state === 'loading' ? '处理中…' : msg}</Text>
      </View>
      <View className="support-chat">
        <Button className="btn-secondary" size="mini" onClick={() => Taro.reLaunch({ url: '/pages/discover/index' })}>
          返回首页
        </Button>
      </View>
    </View>
  )
}
