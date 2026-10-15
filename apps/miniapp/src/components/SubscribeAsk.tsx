import { View, Text } from '@tarojs/components'

export interface SubscribeAskProps {
  visible: boolean
  text?: string
  onAction?: () => void
  actionText?: string
}

// 关注=订阅 提示条：关注成功后提示「Ta 发新内容会在消息-系统通知提醒你」。
export default function SubscribeAsk({
  visible,
  text = '已关注，Ta 发布新内容时会在「消息-系统通知」提醒你（关注即订阅）',
  onAction,
  actionText,
}: SubscribeAskProps) {
  if (!visible) return null
  return (
    <View className="subscribe-ask">
      <Text className="subscribe-ask__text">{text}</Text>
      {actionText && onAction ? (
        <Text className="subscribe-ask__action" onClick={onAction}>
          {actionText}
        </Text>
      ) : null}
    </View>
  )
}
