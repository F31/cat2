import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { type CircleMount } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

// 待我确认的挂载：内容所有者跨圈处理，不要求是目标圈成员。
export default function PendingMounts() {
  const [items, setItems] = useState<CircleMount[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      await ensureLogin()
      const res = await api.pendingMounts()
      setItems(res.items ?? [])
      setError(false)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const decide = async (m: CircleMount, approve: boolean) => {
    try {
      await api.decideCircleMount(m.id, approve)
      Taro.showToast({ title: approve ? '已同意挂载' : '已拒绝挂载', icon: 'success' })
      void load()
    } catch {
      Taro.showToast({ title: '操作失败', icon: 'none' })
    }
  }

  return (
    <View className="page">
      <View className="detail-block">
        <Text className="detail-block__title">待确认挂载：{items.length}</Text>
        <Text className="form-field__label">其他圈成员申请挂载你的内容，同意后展示在对应圈子。</Text>
      </View>
      <StateView
        loading={loading}
        error={error}
        empty={!error && items.length === 0}
        emptyText="暂无待确认挂载"
        onRetry={() => void load()}
      >
        <View className="menu">
          {items.map((m) => (
            <View key={m.id} className="menu__item">
              <View className="menu__row">
                <Text>{m.title || m.item_ref}</Text>
                <Text className="row-tag">{m.circle_name || '圈子'}</Text>
              </View>
              <Text className="form-field__label">
                {m.kind === 'event' ? '活动' : m.kind === 'meeting' ? '会议' : m.kind === 'project' ? '项目' : '产品'}
                {m.start_at ? ` · ${m.start_at.slice(0, 16).replace('T', ' ')}` : ''}
                {m.location ? ` · ${m.location}` : ''}
              </Text>
              <View className="menu__row-extra">
                <Button className="btn-primary" size="mini" onClick={() => void decide(m, true)}>
                  同意
                </Button>
                <Button className="btn-secondary" size="mini" onClick={() => void decide(m, false)}>
                  拒绝
                </Button>
              </View>
            </View>
          ))}
        </View>
      </StateView>
    </View>
  )
}
