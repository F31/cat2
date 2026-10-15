import { useState } from 'react'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { CubeTask, CubeMember } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'
import FormField from '../../components/FormField'

const STATUS: Record<number, string> = { 0: '待办', 1: '进行中', 2: '待确认', 3: '已完成' }

// 内部工作安排：任务可不挂项目；猫王/猫蜜建任务指派负责人并（默认）确认。
export default function CubeTaskPage() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [tasks, setTasks] = useState<CubeTask[]>([])
  const [members, setMembers] = useState<CubeMember[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [title, setTitle] = useState('')
  const [assignee, setAssignee] = useState('')

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const [ts, ms] = await Promise.all([api.cubeTasks(id), api.cubeMembers(id)])
      setTasks(ts.items ?? [])
      setMembers(ms.items ?? [])
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const create = async () => {
    if (!title.trim() || !assignee.trim()) {
      Taro.showToast({ title: '请填写任务与负责人', icon: 'none' })
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
        <FormField label="负责人用户 ID" required value={assignee} onChange={setAssignee} />
        <Text className="form-field__label">成员：{members.map((m) => m.user_id).join('、')}</Text>
        <Button className="btn-primary" size="mini" onClick={() => void create()}>
          新建任务
        </Button>
      </View>
      <StateView loading={loading} error={error} empty={tasks.length === 0} emptyText="暂无任务" onRetry={() => void load()}>
        <View className="menu">
          {tasks.map((t) => (
            <View key={t.id} className="menu__item menu__row">
              <Text>
                {t.title} · {STATUS[t.status] ?? t.status} · {t.progress}%
              </Text>
              <View className="menu__row-extra">
                {t.status === 0 ? (
                  <Text className="row-action" onClick={() => void act(t, 'start')}>开始</Text>
                ) : null}
                {t.status === 1 ? (
                  <Text className="row-action" onClick={() => void act(t, 'submit', { progress: 100 })}>提交</Text>
                ) : null}
                {t.status === 2 ? (
                  <Text className="row-action" onClick={() => void act(t, 'confirm', { approve: true })}>确认</Text>
                ) : null}
                {t.status === 3 ? (
                  <Text className="row-action" onClick={() => void act(t, 'reopen')}>重开</Text>
                ) : null}
              </View>
            </View>
          ))}
        </View>
      </StateView>
    </View>
  )
}
