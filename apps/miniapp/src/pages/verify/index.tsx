import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { Cert } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import FormField from '../../components/FormField'
import StateView from '../../components/StateView'

// 实名认证：真实姓名 + 身份证号 + 协议勾选（第三方二要素实时核验，免证件照上传）。
// 实名是发布、建圈、建猫立方、申请认证的门槛；浏览/搜索/详情不受影响。
export default function Verify() {
  const [name, setName] = useState('')
  const [idNo, setIdNo] = useState('')
  const [agreed, setAgreed] = useState(false)
  const [cert, setCert] = useState<Cert | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.myCerts()
      const latest = (res.items ?? []).find((c) => c.cert_type === 1) ?? null
      setCert(latest)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const submit = async () => {
    if (!name.trim() || !idNo.trim()) {
      Taro.showToast({ title: '请填写姓名与身份证号', icon: 'none' })
      return
    }
    if (!agreed) {
      Taro.showToast({ title: '请先勾选实名认证服务协议', icon: 'none' })
      return
    }
    setSubmitting(true)
    try {
      await ensureLogin()
      const res = await api.submitCert({ cert_type: 1, real_name: name.trim(), id_card_no: idNo.trim(), agreed })
      setCert(res)
      Taro.showToast({ title: res.status === 1 ? '实名认证通过' : '已提交审核', icon: 'success' })
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 100002 ? '姓名与证件号不一致' : '认证失败，请核对信息', icon: 'none' })
    } finally {
      setSubmitting(false)
    }
  }

  const passed = cert?.status === 1
  const rejected = cert?.status === 2

  return (
    <View className="page">
      <StateView loading={loading} error={error} onRetry={() => void load()}>
        {passed ? (
          <View className="detail-block">
            <Text className="detail-block__title">已实名认证</Text>
            <Text className="detail-block__body">你已完成实名认证，可发布内容、创建圈子与猫立方。</Text>
          </View>
        ) : (
          <>
            <View className="detail-block">
              <Text className="detail-block__title">实名认证</Text>
              <Text className="detail-block__body">
                仅用于发布内容、创建圈子与猫立方等操作；浏览、搜索、查看详情无需实名。
              </Text>
            </View>
            {rejected ? (
              <View className="policy-gate">
                <Text className="policy-gate__hint">上次认证未通过：{cert?.reject_reason || '信息不一致'}</Text>
              </View>
            ) : null}
            <FormField label="真实姓名" required value={name} onChange={setName} />
            <FormField label="身份证号" required value={idNo} onChange={setIdNo} />
            <View className="agreement" onClick={() => setAgreed(!agreed)}>
              <Text className={agreed ? 'checkbox checkbox--on' : 'checkbox'}>{agreed ? '✓' : ''}</Text>
              <Text className="agreement__text">我已阅读并同意《实名认证服务协议》</Text>
            </View>
            <Button className="btn-primary" loading={submitting} onClick={() => void submit()}>
              提交认证
            </Button>
          </>
        )}
      </StateView>
    </View>
  )
}
