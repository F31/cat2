import { View } from '@tarojs/components'
import CircleChannel from '../../components/CircleChannel'

// 兼容旧路径：圈子已并入广场，此页保留用于旧链接/分享直达。
export default function HomeCircle() {
  return (
    <View className="page">
      <CircleChannel />
    </View>
  )
}
