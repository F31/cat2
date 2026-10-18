import { useState } from 'react'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import type { Cube, CubeMember } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

const ROLE: Record<number, string> = { 1: '猫王', 2: '猫蜜', 3: '成员', 4: '共建成员' }

// 猫立方详情：协作模块 + 关于我们 + 成员管理 + 群聊 + 订阅分摊。
export default function CubeDetail() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [cube, setCube] = useState<Cube | null>(null)
  const [members, setMembers] = useState<CubeMember[]>([])
  const [myUserId, setMyUserId] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [removing, setRemoving] = useState<CubeMember | null>(null)
  const [reason, setReason] = useState('')
  const [confirmDissolve, setConfirmDissolve] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const [c, m, me] = await Promise.all([api.cubeDetail(id), api.cubeMembers(id), api.me()])
      setCube(c)
      setMembers(m.items ?? [])
      setMyUserId(me.user_id)
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
    try {
      const t = await api.cubeInvite(id)
      Taro.setClipboardData({ data: t.code })
      Taro.showToast({ title: '邀请码已复制', icon: 'success' })
    } catch {
      Taro.showToast({ title: '仅猫王/猫蜜可邀请', icon: 'none' })
    }
  }

  const openChat = async (kind: 'cube' | 'project', projectId?: string) => {
    try {
      const s = kind === 'cube'
        ? await api.ensureCubeChat(id)
        : await api.ensureCubeProjectChat(id, projectId ?? '')
      Taro.navigateTo({ url: `/pages/msg/chat?session=${encodeURIComponent(s.session_id)}&peer=${encodeURIComponent(s.title)}` })
    } catch {
      Taro.showToast({ title: '仅成员可进入群聊', icon: 'none' })
    }
  }

  const removeMember = async () => {
    if (!removing) return
    if (!reason.trim()) {
      Taro.showToast({ title: '请填写移除原因', icon: 'none' })
      return
    }
    try {
      await api.removeCubeMember(id, removing.user_id, reason.trim())
      Taro.showToast({ title: '已移除', icon: 'success' })
      setRemoving(null)
      setReason('')
      void load()
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 130002 ? '猫王/猫蜜需先调整' : '移除失败', icon: 'none' })
    }
  }

  const transfer = async (m: CubeMember) => {
    const res = await Taro.showModal({ title: '转让猫王', content: `确认将猫王转让给 ${m.nickname || m.user_id}？` })
    if (!res.confirm) return
    try {
      await api.transferCube(id, m.user_id)
      Taro.showToast({ title: '已转让', icon: 'success' })
      void load()
    } catch {
      Taro.showToast({ title: '转让失败', icon: 'none' })
    }
  }

  const setHoney = async (m: CubeMember) => {
    try {
      await api.setCubeHoney(id, m.user_id)
      Taro.showToast({ title: '已设为猫蜜', icon: 'success' })
      void load()
    } catch {
      Taro.showToast({ title: '仅猫王可调整猫蜜', icon: 'none' })
    }
  }

  const openMember = (m: CubeMember) => {
    if (!cube) return
    const my = cube.my_role ?? 0
    if (m.user_id === myUserId) return
    const actions: string[] = []
    if (my === 1 && m.user_id !== cube.leader_id) actions.push('转让猫王')
    if (my === 1 && m.user_id !== cube.honey_id) actions.push('设为猫蜜')
    if ((my === 1 || my === 2) && m.user_id !== cube.honey_id) actions.push('移除成员')
    if (actions.length === 0) return
    void Taro.showActionSheet({ itemList: actions })
      .then((r) => {
        const action = actions[r.tapIndex]
        if (action === '转让猫王') void transfer(m)
        else if (action === '设为猫蜜') void setHoney(m)
        else if (action === '移除成员') setRemoving(m)
      })
      .catch(() => {
        /* 取消 */
      })
  }

  const leave = async () => {
    const res = await Taro.showModal({ title: '退出猫立方', content: '退出后将无法查看立方内容，确认退出？' })
    if (!res.confirm) return
    try {
      await api.leaveCube(id)
      Taro.showToast({ title: '已退出', icon: 'success' })
      setTimeout(() => Taro.navigateBack(), 600)
    } catch {
      Taro.showToast({ title: '猫王须先转让', icon: 'none' })
    }
  }

  const dissolve = async () => {
    try {
      await api.dissolveCube(id)
      Taro.showToast({ title: '已解散', icon: 'success' })
      setTimeout(() => Taro.navigateBack(), 600)
    } catch {
      Taro.showToast({ title: '仅猫王可解散', icon: 'none' })
    }
  }

  if (!cube && (loading || error)) {
    return (
      <View className="page">
        <StateView loading={loading} error={error} onRetry={() => void load()} />
      </View>
    )
  }
  if (!cube) return null
  const my = cube.my_role ?? 0
  const canManage = my === 1 || my === 2

  return (
    <View className="page">
      <View className="detail-head">
        <Text className="detail-head__title">{cube.name}</Text>
        <Text className="detail-head__type">
          {cube.member_count} 人 · {cube.region_code || '未设区域'} · {cube.plan_level === 1 ? '基础档' : cube.plan_level === 2 ? '进阶档' : '高阶档'} · {ROLE[my] ?? '访客'}
        </Text>
      </View>

      <View className="menu">
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: `/pages/cube/project?id=${id}&type=1` })}>
          <Text>重点合作闭环研究</Text>
        </View>
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: `/pages/cube/project?id=${id}&type=2` })}>
          <Text>重点项目合作承接</Text>
        </View>
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: `/pages/cube/task?id=${id}` })}>
          <Text>内部工作安排 / 任务</Text>
        </View>
        <View className="menu__item" onClick={() => void openChat('cube')}>
          <Text>进入立方工作群</Text>
        </View>
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: `/pages/cube/plan?id=${id}` })}>
          <Text>订阅与分摊</Text>
        </View>
      </View>

      <View className="detail-block">
        <View className="menu__row">
          <Text className="detail-block__title">关于我们</Text>
          {canManage ? (
            <Text className="row-action" onClick={() => Taro.navigateTo({ url: `/pages/cube/edit?id=${id}` })}>
              编辑
            </Text>
          ) : null}
        </View>
        <Text className="detail-block__body">
          方向：{cube.direction || '—'}{'\n'}
          资源：{cube.resources || '—'}{'\n'}
          优势：{cube.strength || '—'}{'\n'}
          合作诉求：{cube.cooperation || '—'}
        </Text>
      </View>

      <View className="detail-block">
        <Text className="detail-block__title">成员（{members.length}）</Text>
        {members.map((m) => (
          <View key={m.user_id} className="menu__row" style={{ padding: '8px 0' }} onClick={() => openMember(m)}>
            <Text>{m.nickname || `用户 ${m.user_id}`}</Text>
            <Text className="row-tag">{ROLE[m.role] ?? m.role}</Text>
          </View>
        ))}
      </View>

      <View className="detail-actions">
        <Button className="btn-secondary" onClick={() => void invite()}>
          邀请成员
        </Button>
        <Button className="btn-primary" onClick={() => Taro.navigateTo({ url: `/pages/cube/plan?id=${id}` })}>
          订阅 / 分摊
        </Button>
      </View>
      {my === 1 ? (
        <Button className="btn-danger" onClick={() => setConfirmDissolve(true)}>
          解散猫立方
        </Button>
      ) : my !== 0 ? (
        <Button className="btn-secondary" onClick={() => void leave()}>
          退出猫立方
        </Button>
      ) : null}

      {removing ? (
        <View className="auth-mask">
          <View className="auth-modal">
            <Text className="auth-modal__title">移除成员</Text>
            <Text className="auth-modal__row">成员：{removing.nickname || removing.user_id}</Text>
            <Input className="search-row__input" value={reason} placeholder="请填写移除原因" onInput={(e) => setReason(e.detail.value)} />
            <View className="auth-modal__actions">
              <Button className="btn-secondary auth-modal__btn" onClick={() => { setRemoving(null); setReason('') }}>
                取消
              </Button>
              <Button className="btn-danger auth-modal__btn" onClick={() => void removeMember()}>
                确认移除
              </Button>
            </View>
          </View>
        </View>
      ) : null}

      {confirmDissolve ? (
        <View className="auth-mask">
          <View className="auth-modal">
            <Text className="auth-modal__title">确认解散？</Text>
            <Text className="auth-modal__row">成员将收到通知；解散后不可恢复。</Text>
            <View className="auth-modal__actions">
              <Button className="btn-secondary auth-modal__btn" onClick={() => setConfirmDissolve(false)}>
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
