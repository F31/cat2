import Taro from '@tarojs/taro'

const KEY = 'catcube.agreement'

// 协议勾选持久化：首次进入走登录引导页（协议勾选 + 注册弹窗 + 身份选择）。
export function getAgreed(): boolean {
  return Taro.getStorageSync<boolean | ''>(KEY) === true
}

export function setAgreed(): void {
  Taro.setStorageSync(KEY, true)
}
