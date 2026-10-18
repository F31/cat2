import { useMemo, useState } from 'react'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import type { CubeTask, CubeMember } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'
import FormField from '../../components/FormField'

const STATUS: Record<number, string> = { 0: '待办', 1: '进行中', 2: '待确认', 3: '已完成' }

// 内部工作安排：成员选择器指派负责人；四态推进；驳回必填原因。
export default function CubeTaskPage() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [tasks, setTasks] = useState<CubeTask[]>([])
  const [members, setMembers] = useState<CubeMember[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [title, setTitle] = useState('')
  const [assignee, setAssignee] = useState('')
  const [rejecting, setRejecting] = useState<CubeTask | null>(null)
  const [reason, setReason] = useState('')

  const nameOf = useMemo(() => {
    const map = new Map(members.map((m) => [m.user_id, m.nickname || m.user_id]))
    return (uid?: string | null) => (uid ? map.get(uid) || uid : '—')
  }, [members])

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const [ts, ms] = await Promise.all([api.cubeTasks(id), api.cubeMembers(id)])
      setTasks(ts.items ?? [])
      setMembers((ms.items ?? []).filter((m) => m.status === 0))
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const pickAssignee = async () => {
    if (members.length === 0) {
      Taro.showToast({ title: '暂无成员', icon: 'none' })
      return
    }
    try {
      const r = await Taro.showActionSheet({ itemList: members.map((m) => `${m.nickname || m.user_id}`) })
      const m = members[r.tapIndex]
      if (m) setAssignee(m.user_id)
    } catch {
      /* 取消 */
    }
  }

  const create = async () => {
    if (!title.trim() || !assignee) {
      Taro.showToast({ title: '请填写任务并选择负责人', icon: 'none' })
      return
    }
    try {
      await api.createCubeTask(id, { title, assignee_id: assignee })
      setTitle(''); setAssignee('')
      void load()
    } catch {
      Taro.showToast({ title: '仅猫王/猫蜜可建任务', icon: 'none' })
    }
  }

  const act = async (t: CubeTask, action: string, body: Record<string, unknown> = {}) => {
    try {
      await api.cubeTaskAction(id, t.id, action, body)
      void load()
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 130011 ? '驳回须填写原因' : '操作失败', icon: 'none' })
    }
  }

  return (
    <View className="page">
      <View className="detail-block">
        <Text className="detail-block__title">新建任务（可不挂项目）</Text>
        <FormField label="任务项" required value={title} onChange={setTitle} />
        <View className="menu__row" style={{ padding: '10px 0' }} onClick={() => void pickAssignee()}>
          <Text>负责人</Text>
          <Text className="row-tag">{assignee ? nameOf(assignee) : '选择成员'}</Text>
        </View>
        <Button className="btn-primary" size="mini" onClick={() => void create()}>
          新建任务
        </Button>
      </View>
      <StateView loading={loading} error={error} empty={tasks.length === 0} emptyText="暂无任务" onRetry={() => void load()}>
        <View className="menu">
          {tasks.map((t) => (
            <View key={t.id} className="menu__item">
              <View className="menu__row">
                <Text>{t.title} · {STATUS[t.status] ?? t.status} · {t.progress}%</Text>
                <Text className="row-tag">负责人 {nameOf(t.assignee_id)}</Text>
              </View>
              {t.reject_reason ? <Text className="form-field__label">驳回原因：{t.reject_reason}</Text> : null}
              <View className="menu__row-extra">
                {t.status === 0 ? <Text className="row-action" onClick={() => void act(t, 'start')}>开始</Text> : null}
                {t.status === 1 ? <Text className="row-action" onClick={() => void act(t, 'submit', { progress: 100 })}>提交</Text> : null}
                {t.status === 2 ? (
                  <>
                    <Text className="row-action" onClick={() => setRejecting(t)}>驳回</Text>
                    <Text className="row-action" onClick={() => void act(t, 'confirm', { approve: true })}>确认</Text>
                  </>
                ) : null}
                {t.status === 3 ? <Text className="row-action" onClick={() => void act(t, 'reopen')}>重开</Text> : null}
              </View>
            </View>
          ))}
        </View>
      </StateView>

      {rejecting ? (
        <View className="auth-mask">
          <View className="auth-modal">
            <Text className="auth-modal__title">驳回任务</Text>
            <Input className="search-row__input" value={reason} placeholder="请填写驳回原因" onInput={(e) => setReason(e.detail.value)} />
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
                  void act(rejecting, 'confirm', { approve: false, reason: reason.trim() })
                  setRejecting(null)
                  setReason('')
                }}
              >
                确认驳回
              </Button>
            </View>
          </View>
        </View>
      ) : null}
    </View>
  )
}
