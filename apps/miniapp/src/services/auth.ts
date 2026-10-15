import Taro from '@tarojs/taro'
import { api } from './request'
import { getToken, setTokens } from '../utils/token'

let inflight: Promise<string> | null = null

// ensureLogin 登录注册一体化：微信授权主推；已登录直接复用。
export async function ensureLogin(): Promise<string> {
  const token = getToken()
  if (token) return token
  if (inflight) return inflight
  inflight = (async () => {
    const { code } = await Taro.login()
    const res = await api.wechatLogin(code)
    setTokens(res.tokens)
    return res.tokens.access_token
  })()
  try {
    return await inflight
  } finally {
    inflight = null
  }
}
