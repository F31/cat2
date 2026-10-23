import { useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { consentLogin } from '../../services/auth'

const IDENTITY = [
  { value: 1 as const, title: '个人用户', sub: '以个人身份发布项目、对接合作' },
  { value: 2 as const, title: '企业/机构', sub: '以组织身份入驻，可开通企业版' },
]

// 登录引导页：协议勾选 → 注册确认弹窗（含身份选择）→ 微信手机号授权登录（身份仅注册时生效）。
export default function AuthEnter() {
  const [agreed, setAgreed] = useState(false)
  const [identity, setIdentity] = useState<1 | 2>(1)
  const [confirming, setConfirming] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [phoneBusy, setPhoneBusy] = useState(false)

  const showAgreement = (title: string) => Taro.showToast({ title: `《${title}》内容页后续接入`, icon: 'none' })

  const tapLogin = () => {
    if (!agreed) {
      Taro.showToast({ title: '请先阅读并同意协议', icon: 'none' })
      return
    }
    setConfirming(true)
  }

  // 手机号快速验证：e.detail.code 传给后端换手机号；拒绝授权则提示。
  const onPhoneNumber = (e: { detail: { code?: string; errMsg?: string } }) => {
    const phoneCode = e.detail.code
    if (!phoneCode) {
      setPhoneBusy(false)
      const tooOften = (e.detail.errMsg ?? '').includes('frequently')
      Taro.showToast({ title: tooOften ? '请求过于频繁，请稍后再试' : '需授权手机号完成登录', icon: 'none' })
      return
    }
    void doLogin(phoneCode)
  }

  const doLogin = async (phoneCode?: string) => {
    setSubmitting(true)
    try {
      await consentLogin(identity, phoneCode)
      Taro.navigateBack()
    } catch {
      Taro.showToast({ title: '登录失败，请重试', icon: 'none' })
    } finally {
      setSubmitting(false)
      setPhoneBusy(false)
    }
  }

  return (
    <View className="auth-enter">
      <View className="auth-enter__logo">
        <Text className="auth-enter__title">猫平方</Text>
        <Text className="auth-enter__sub">六方合作服务平台 · 找对人，办成事</Text>
      </View>

      <Text className="auth-enter__label">选择身份（注册后本期不可更改）</Text>
      <View className="auth-enter__identity">
        {IDENTITY.map((it) => (
          <View
            key={it.value}
            className={identity === it.value ? 'auth-enter__card auth-enter__card--active' : 'auth-enter__card'}
            onClick={() => setIdentity(it.value)}
          >
            <Text className="auth-enter__card-title">{it.title}</Text>
            <Text className="auth-enter__card-sub">{it.sub}</Text>
          </View>
        ))}
      </View>

      <View className="agreement">
        <Text className={`checkbox ${agreed ? 'checkbox--on' : ''}`} onClick={() => setAgreed(!agreed)}>
          {agreed ? '✓' : ''}
        </Text>
        <Text className="agreement__text">
          已阅读并同意
          <Text className="auth-enter__link" onClick={() => showAgreement('用户服务协议')}>
            《用户服务协议》
          </Text>
          与
          <Text className="auth-enter__link" onClick={() => showAgreement('隐私政策')}>
            《隐私政策》
          </Text>
        </Text>
      </View>

      <Button className="btn-primary auth-enter__btn" onClick={() => tapLogin()}>
        微信授权登录
      </Button>

      {confirming ? (
        <View className="auth-mask">
          <View className="auth-modal">
            <Text className="auth-modal__title">确认注册信息</Text>
            <Text className="auth-modal__row">
              身份：{identity === 1 ? '个人用户' : '企业/机构（注册后可开通企业版）'}
            </Text>
            <Text className="auth-modal__row">注册即视为同意《用户服务协议》与《隐私政策》</Text>
            <View className="auth-modal__actions">
              <Button className="btn-secondary auth-modal__btn" onClick={() => setConfirming(false)}>
                再想想
              </Button>
              <Button
                className="btn-primary auth-modal__btn"
                openType="getPhoneNumber"
                onGetPhoneNumber={onPhoneNumber}
                loading={submitting || phoneBusy}
                disabled={submitting || phoneBusy}
                onClick={() => setPhoneBusy(true)}
              >
                确认并登录
              </Button>
            </View>
          </View>
        </View>
      ) : null}
    </View>
  )
}
