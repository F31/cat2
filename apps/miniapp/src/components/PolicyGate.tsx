import type { ReactNode } from 'react'
import { View, Text } from '@tarojs/components'
import { hintFor } from '@catcube/policy-hints'

export interface PolicyGateProps {
  allowed: boolean
  reason?: string
  unlockText?: string
  onUnlock?: () => void
  children: ReactNode
}

// 受限能力统一入口：自动灰显 + 展示后端 Decision.Reason + 解锁动作；页面内不写「是否会员」判断。
export default function PolicyGate({ allowed, reason, unlockText = '去解锁', onUnlock, children }: PolicyGateProps) {
  if (allowed) return <>{children}</>
  return (
    <View className="policy-gate" onClick={onUnlock}>
      <Text className="policy-gate__hint">{hintFor(reason)}</Text>
      <Text className="policy-gate__action">{unlockText}</Text>
    </View>
  )
}
