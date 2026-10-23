import Taro from '@tarojs/taro'

const KEY = 'catcube.agreement'
export const CONSENT_VERSION = '2026-10-24'

// 协议勾选持久化：按版本记录，协议更新后自动重新引导。
export function getAgreed(): boolean {
  return Taro.getStorageSync<string>(KEY) === CONSENT_VERSION
}

export function setAgreed(): void {
  Taro.setStorageSync(KEY, CONSENT_VERSION)
}
