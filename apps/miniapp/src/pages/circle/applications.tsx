import { useState } from 'react'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import type { CircleApplication } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

const STATUS: Record<number, string> = { 1: '待审核', 2: '已通过', 3: '已拒绝' }

// 圈主审核入圈申请：通过即时入圈并通知；拒绝必须填写原因。
export default function CircleApplications() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [items, setItems] = useState<CircleApplication[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [rejecting, setRejecting] = useState<CircleApplication | null>(null)
  const [reason, setReason] = useState('')

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.listCircleApplications(id, 0)
      setItems(res.items ?? [])
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const decide = async (a: CircleApplication, approve: boolean, why = '') => {
    try {
      await api.decideCircleApplication(id, a.applicant_id, approve, why)
      Taro.showToast({ title: approve ? '已通过' : '已拒绝', icon: 'success' })
      setRejecting(null)
      setReason('')
      void load()
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 140002 ? '仅圈主可审核' : '操作失败', icon: 'none' })
    }
  }

  const pending = items.filter((a) => a.status === 1)
  const history = items.filter((a) => a.status !== 1)

  const row = (a: CircleApplication, actionable: boolean) => (
    <View key={a.id} className="menu__item">
      <View className="menu__row">
        <Text>{a.nickname || `用户 ${a.applicant_id}`}</Text>
        <Text className="row-tag">{STATUS[a.status] ?? a.status}</Text>
      </View>
      <Text className="form-field__label">申请时间 {a.created_at?.slice(0, 16).replace('T', ' ')}</Text>
      {a.reason ? <Text className="form-field__label">原因：{a.reason}</Text> : null}
      {actionable ? (
        <View className="detail-actions">
          <Button className="btn-secondary" size="mini" onClick={() => setRejecting(a)}>
            拒绝
          </Button>
          <Button className="btn-primary" size="mini" onClick={() => void decide(a, true)}>
            通过
          </Button>
        </View>
      ) : null}
    </View>
  )

  return (
    <View className="page">
      <StateView loading={loading} error={error} empty={items.length === 0} emptyText="暂无入圈申请" onRetry={() => void load()}>
        <View className="detail-block">
          <Text className="detail-block__title">待审核（{pending.length}）</Text>
          {pending.map((a) => row(a, true))}
          {pending.length === 0 ? <Text className="form-field__label">暂无待审核申请</Text> : null}
        </View>
        {history.length > 0 ? (
          <View className="detail-block">
            <Text className="detail-block__title">已处理</Text>
            {history.map((a) => row(a, false))}
          </View>
        ) : null}
      </StateView>

      {rejecting ? (
        <View className="auth-mask">
          <View className="auth-modal">
            <Text className="auth-modal__title">拒绝入圈申请</Text>
            <Text className="auth-modal__row">申请人：{rejecting.nickname || rejecting.applicant_id}</Text>
            <Input
              className="search-row__input"
              value={reason}
              placeholder="请填写拒绝原因"
              onInput={(e) => setReason(e.detail.value)}
            />
            <View className="auth-modal__actions">
              <Button className="btn-secondary auth-modal__btn" onClick={() => { setRejecting(null); setReason('') }}>
                取消
              </Button>
              <Button
                className="btn-danger auth-modal__btn"
                onClick={() => {
                  if (!reason.trim()) {
                    Taro.showToast({ title: '请填写原因', icon: 'none' })
                    return
                  }
                  void decide(rejecting, false, reason.trim())
                }}
              >
                确认拒绝
              </Button>
            </View>
          </View>
        </View>
      ) : null}
    </View>
  )
}
