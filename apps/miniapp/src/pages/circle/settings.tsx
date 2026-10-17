import { useState } from 'react'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { Circle } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

// 圈设置：圈主可转常设（到期后）与解散（二次确认 + 成员数提示，永久删除不可恢复）。
export default function CircleSettings() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [circle, setCircle] = useState<Circle | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [confirming, setConfirming] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      setCircle(await api.circleDetail(id))
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const convert = async () => {
    try {
      await api.convertCircle(id)
      Taro.showToast({ title: '已转为常设圈', icon: 'success' })
      void load()
    } catch {
      Taro.showToast({ title: '仅圈主可操作', icon: 'none' })
    }
  }

  const resetInvite = async () => {
    try {
      const r = await api.resetCircleInvite(id)
      Taro.showToast({ title: `已重置（${r.invalidated} 条旧邀请失效）`, icon: 'success' })
    } catch {
      Taro.showToast({ title: '仅圈主可重置', icon: 'none' })
    }
  }

  const dissolve = async () => {
    try {
      await api.dissolveCircle(id)
      Taro.showToast({ title: '圈子已解散', icon: 'success' })
      setTimeout(() => Taro.navigateBack(), 700)
    } catch {
      Taro.showToast({ title: '仅圈主可解散', icon: 'none' })
    }
  }

  return (
    <View className="page">
      <StateView loading={loading} error={error} onRetry={() => void load()}>
        {circle ? (
          <>
            <View className="detail-block">
              <Text className="detail-block__title">{circle.name}</Text>
              <Text className="detail-block__body">
                {circle.type === 1 ? '常设圈' : '临时圈'} · {circle.member_count} 名成员 ·{' '}
                {circle.status === 0 ? '进行中' : circle.status === 1 ? '已结束' : '已解散'}
              </Text>
            </View>
            {circle.status === 1 ? (
              <Button className="btn-secondary" onClick={() => void convert()}>
                转常设圈（不再到期）
              </Button>
            ) : null}
            {circle.status === 0 ? (
              <Button className="btn-secondary" onClick={() => void resetInvite()}>
                重置邀请（旧链接/二维码失效）
              </Button>
            ) : null}
            <Button className="btn-danger" onClick={() => setConfirming(true)}>
              解散圈子
            </Button>
          </>
        ) : null}
      </StateView>

      {confirming ? (
        <View className="auth-mask">
          <View className="auth-modal">
            <Text className="auth-modal__title">确认解散？</Text>
            <Text className="auth-modal__row">
              当前 {circle?.member_count ?? 0} 名成员将收到通知；解散后永久删除不可恢复。
            </Text>
            <View className="auth-modal__actions">
              <Button className="btn-secondary auth-modal__btn" onClick={() => setConfirming(false)}>
                取消
              </Button>
              <Button className="btn-danger auth-modal__btn" onClick={() => void dissolve()}>
                确认解散
              </Button>
            </View>
          </View>
        </View>
      ) : null}
    </View>
  )
}
