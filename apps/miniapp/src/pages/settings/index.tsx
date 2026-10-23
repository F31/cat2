import Taro from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { clearTokens } from '../../utils/token'

// 设置：编辑资料、使用须知、隐私协议、退出登录。
export default function Settings() {
  const logout = async () => {
    const r = await Taro.showModal({ title: '退出登录', content: '退出后仍可浏览公开内容，发布和互动需重新登录。' })
    if (!r.confirm) return
    clearTokens()
    Taro.reLaunch({ url: '/pages/discover/index' })
  }

  return (
    <View className="page">
      <View className="menu">
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/card/edit' })}>
          <Text>编辑资料</Text>
        </View>
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/settings/agreement?type=terms' })}>
          <Text>使用须知</Text>
        </View>
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/settings/agreement?type=privacy' })}>
          <Text>隐私协议</Text>
        </View>
      </View>
      <Button className="btn-danger" style={{ marginTop: '32px' }} onClick={() => void logout()}>
        退出登录
      </Button>
    </View>
  )
}
