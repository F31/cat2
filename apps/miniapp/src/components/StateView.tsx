import type { ReactNode } from 'react'
import { View, Text } from '@tarojs/components'

export interface StateViewProps {
  loading?: boolean
  error?: boolean
  empty?: boolean
  loadingText?: string
  emptyText?: string
  errorText?: string
  onRetry?: () => void
  children?: ReactNode
}

// 统一空态/加载/错误态（详细设计 5.14）：加载用简洁提示（骨架屏后续补）；错误可点击重试；文案不暴露技术细节。
export default function StateView({
  loading,
  error,
  empty,
  loadingText = '加载中…',
  emptyText = '暂无内容',
  errorText = '加载失败，点击重试',
  onRetry,
  children,
}: StateViewProps) {
  if (loading) {
    return (
      <View className="state">
        <Text className="state__text">{loadingText}</Text>
      </View>
    )
  }
  if (error) {
    return (
      <View className="state" onClick={onRetry}>
        <Text className="state__text">{errorText}</Text>
      </View>
    )
  }
  if (empty) {
    return (
      <View className="state">
        <Text className="state__text">{emptyText}</Text>
      </View>
    )
  }
  return <>{children}</>
}
