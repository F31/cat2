import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

interface SubAccount {
  id: string
  sub_user_id: string
  status: number
  invited_at: string
}

// 企业版子账号：列表 + 按用户 ID 邀请 + 移除；仅企业版主账号可用。
export default function SubAccounts() {
  const [items, setItems] = useState<SubAccount[]>([])
  const [userId, setUserId] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      await ensureLogin()
      const res = await api.subAccounts()
      setItems((res.items ?? []) as SubAccount[])
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

  const invite = async () => {
    const id = userId.trim()
    if (!id) {
      Taro.showToast({ title: '请输入对方用户 ID', icon: 'none' })
      return
    }
    try {
      await api.inviteSub(id)
      setUserId('')
      Taro.showToast({ title: '已邀请', icon: 'success' })
      void load()
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({
        title: code === 120004 ? '仅企业版主账号可邀请' : code === 120005 ? '子账号已达上限（3）' : code === 120006 ? '对方须已实名' : '邀请失败',
        icon: 'none',
      })
    }
  }

  const remove = async (sub: SubAccount) => {
    const r = await Taro.showModal({ title: '移除子账号', content: `确认移除用户 ${sub.sub_user_id}？` })
    if (!r.confirm) return
    try {
      await api.removeSub(sub.sub_user_id)
      Taro.showToast({ title: '已移除', icon: 'success' })
      void load()
    } catch {
      Taro.showToast({ title: '移除失败', icon: 'none' })
    }
  }

  return (
    <View className="page">
      <View className="detail-block">
        <Text className="detail-block__title">邀请子账号</Text>
        <Text className="form-field__label">仅企业版主账号可邀请，最多 3 个；对方须已注册并完成实名。</Text>
        <Input
          className="search-row__input"
          value={userId}
          placeholder="输入对方用户 ID"
          onInput={(e) => setUserId(e.detail.value)}
        />
        <Button className="btn-primary" style={{ marginTop: '16px' }} onClick={() => void invite()}>
          发送邀请
        </Button>
      </View>

      <StateView
        loading={loading}
        error={error}
        empty={!error && items.length === 0}
        emptyText="还没有子账号"
        onRetry={() => void load()}
      >
        <View className="menu">
          {items.map((s) => (
            <View key={s.id} className="menu__item">
              <View className="menu__row">
                <Text>用户 {s.sub_user_id}</Text>
                <Text className="row-tag">{s.status === 1 ? '生效中' : '已移除'}</Text>
              </View>
              <Text className="form-field__label">邀请时间：{s.invited_at?.slice(0, 19).replace('T', ' ') || '—'}</Text>
              {s.status === 1 ? (
                <Button className="btn-secondary" size="mini" style={{ marginTop: '12px' }} onClick={() => void remove(s)}>
                  移除
                </Button>
              ) : null}
            </View>
          ))}
        </View>
      </StateView>
    </View>
  )
}
