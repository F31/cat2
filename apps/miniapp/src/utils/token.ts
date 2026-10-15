import Taro from '@tarojs/taro'
import type { Tokens } from '@catcube/api-client'

const KEY = 'catcube.tokens'

export function getTokens(): Tokens | null {
  const t = Taro.getStorageSync<Tokens | ''>(KEY)
  return t && t.access_token ? t : null
}

export function getToken(): string | null {
  return getTokens()?.access_token ?? null
}

export function setTokens(t: Tokens): void {
  Taro.setStorageSync(KEY, t)
}

export function clearTokens(): void {
  Taro.removeStorageSync(KEY)
}
