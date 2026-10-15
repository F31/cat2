import { useState } from 'react'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import type { CubePlan, CubeApportion } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'
import PayButton from '../../components/PayButton'

const APP_STATUS: Record<number, string> = { 0: '认筹中', 1: '已成团', 2: '未成团', 3: '已终结' }

// 订阅与分摊：三档购买（PayButton）；猫王发起分摊（人数 N，每人=档价÷N）；认筹进度/退出。
export default function CubePlanPage() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [plans, setPlans] = useState<CubePlan[]>([])
  const [app, setApp] = useState<CubeApportion | null>(null)
  const [level, setLevel] = useState(2)
  const [n, setN] = useState(2)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.cubePlans()
      setPlans(res.items ?? [])
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const initiate = async () => {
    try {
      const g = await api.initiateApportion(id, level, n)
      setApp(g)
    } catch {
      Taro.showToast({ title: '仅猫王可发起分摊', icon: 'none' })
    }
  }

  const pledge = async () => {
    if (!app) return
    try {
      const { order_id } = await api.pledgeApportion(id, app.id)
      const params = await api.payOrder(order_id)
      Taro.showToast({ title: `订单 ${params.out_trade_no} 已创建`, icon: 'none' })
      void api.cubeApportion(app.id).then(setApp)
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 130014 ? '分摊已结束/已成团' : '认筹失败', icon: 'none' })
    }
  }

  const exit = async () => {
    if (!app) return
    try {
      await api.exitApportion(id, app.id)
      void api.cubeApportion(app.id).then(setApp)
    } catch {
      Taro.showToast({ title: '退出失败', icon: 'none' })
    }
  }

  return (
    <View className="page">
      <StateView loading={loading} error={error} onRetry={() => void load()}>
        <View className="detail-block">
          <Text className="detail-block__title">订阅三档（后台可配）</Text>
          {plans.map((p) => (
            <View key={p.level} className="menu__row" style={{ padding: '10px 0' }} onClick={() => setLevel(p.level)}>
              <Text>
                {p.name} · {p.price_fen === 0 ? '免费' : `¥${(p.price_fen / 100).toFixed(0)}/年`} · 可建/加入 {p.cube_limit === 0 ? '不限' : p.cube_limit} · 成员上限 {p.member_limit === 0 ? '不限' : p.member_limit}
                {level === p.level ? '（已选）' : ''}
              </Text>
              {p.price_fen > 0 ? (
                <PayButton
                  label="直接开通"
                  className="btn-secondary"
                  createOrder={async () => (await api.changeCubePlan(id, p.level, 1)).order_id}
                  onPaid={() => Taro.showToast({ title: '已下单，支付后开通', icon: 'none' })}
                />
              ) : null}
            </View>
          ))}
        </View>

        <View className="detail-block">
          <Text className="detail-block__title">分摊（猫王设人数不设价，每人=档价÷N，默认 7 天）</Text>
          <View className="search-row">
            <Text className="form-field__label">人数 N</Text>
            <Input
              className="search-row__input"
              type="number"
              value={String(n)}
              onInput={(e) => setN(Number(e.detail.value) || 2)}
            />
            <Button className="btn-secondary" size="mini" onClick={() => void initiate()}>
              发起分摊
            </Button>
          </View>
          {app ? (
            <View>
              <Text className="detail-block__body">
                进度 {app.pledged_count}/{app.target_n} · 每人 ¥{(app.per_fen / 100).toFixed(2)} · 截止 {app.deadline.slice(0, 10)} · {APP_STATUS[app.status] ?? app.status}
              </Text>
              <View className="detail-actions">
                <Button className="btn-secondary" onClick={() => void exit()}>
                  退出退款
                </Button>
                <Button className="btn-primary" onClick={() => void pledge()}>
                  认筹
                </Button>
              </View>
            </View>
          ) : null}
        </View>
      </StateView>
    </View>
  )
}
