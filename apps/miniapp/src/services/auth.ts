import Taro from '@tarojs/taro'
import { api } from './request'
import { getAgreed, setAgreed } from '../utils/agreement'
import { getToken, setTokens } from '../utils/token'

let inflight: Promise<string> | null = null

// ensureLogin 登录注册一体化：已登录复用；未登录且已勾选协议则静默微信登录；
// 未勾选协议 → 跳登录引导页（协议勾选/注册弹窗/身份选择），由用户确认后登录。
export async function ensureLogin(): Promise<string> {
  const token = getToken()
  if (token) return token
  if (!getAgreed()) {
    redirectConsent()
    throw new Error('NEED_CONSENT')
  }
  if (inflight) return inflight
  inflight = silentLogin()
  try {
    return await inflight
  } finally {
    inflight = null
  }
}

// silentLogin 静默登录（已同意协议的老用户无感续期/换机登录）。
export async function silentLogin(): Promise<string> {
  const { code } = await Taro.login()
  const res = await api.wechatLogin(code)
  setTokens(res.tokens)
  return res.tokens.access_token
}

let redirecting = false

function redirectConsent(): void {
  const pages = Taro.getCurrentPages()
  const top = pages[pages.length - 1]
  if (top && top.route === 'pages/auth/enter') return
  if (redirecting) return
  redirecting = true
  Taro.navigateTo({ url: '/pages/auth/enter', complete: () => (redirecting = false) })
}

// consentLogin 引导页确认后的登录：勾选协议 + 携带注册身份（仅新用户生效）。
export async function consentLogin(identityType: 1 | 2, phoneCode?: string): Promise<void> {
  const { code } = await Taro.login()
  const res = await api.wechatLogin(code, phoneCode, undefined, identityType)
  setTokens(res.tokens)
  setAgreed()
}
