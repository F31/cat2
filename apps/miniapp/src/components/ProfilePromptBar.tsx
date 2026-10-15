import { View, Text } from '@tarojs/components'
import type { Completion } from '@catcube/api-client'

export interface ProfilePromptBarProps {
  completion?: Completion | null
  onTap?: () => void
}

// 完善资料提示条：第二层五项缺一即显示，填全自动消失；不引导实名、无关闭按钮、随页滚动不悬浮。
export default function ProfilePromptBar({ completion, onTap }: ProfilePromptBarProps) {
  if (!completion || completion.complete) return null
  return (
    <View className="profile-prompt" onClick={onTap}>
      <Text className="profile-prompt__text">完善资料，获得更精准的匹配</Text>
      <Text className="profile-prompt__action">去完善</Text>
    </View>
  )
}
