import { useState } from 'react'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { CubeProject, CubeTask, CubeResearch } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'
import FormField from '../../components/FormField'

const TASK_STATUS: Record<number, string> = { 0: '待办', 1: '进行中', 2: '待确认', 3: '已完成' }
const PLATFORM: Record<string, string> = { taken_down: '平台已下架' }

// 项目详情：任务（分工待办，按四态操作）、研究记录→决策结论、报备入口。
export default function CubeProjectPage() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const type = Number(router.params.type ?? '1')
  const [projects, setProjects] = useState<CubeProject[]>([])
  const [current, setCurrent] = useState<CubeProject | null>(null)
  const [tasks, setTasks] = useState<CubeTask[]>([])
  const [research, setResearch] = useState<CubeResearch[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [newName, setNewName] = useState('')
  const [note, setNote] = useState('')
  const [decision, setDecision] = useState('')

  const loadProjects = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.cubeProjects(id, type)
      setProjects(res.items ?? [])
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  const openProject = async (p: CubeProject) => {
    setCurrent(p)
    try {
      const [ts, rs] = await Promise.all([api.cubeTasks(id, p.id), api.cubeResearch(id, p.id)])
      setTasks(ts.items ?? [])
      setResearch(rs.items ?? [])
    } catch {
      Taro.showToast({ title: '加载失败', icon: 'none' })
    }
  }

  useDidShow(() => {
    void loadProjects()
  })

  const create = async () => {
    if (!newName.trim()) return
    try {
      const p = await api.createCubeProject(id, { type, name: newName })
      setNewName('')
      await loadProjects()
      void openProject(p)
    } catch {
      Taro.showToast({ title: '仅猫王/猫蜜可建项目', icon: 'none' })
    }
  }

  const addResearch = async () => {
    if (!current || !note.trim()) return
    try {
      await api.addCubeResearch(id, current.id, note, decision)
      setNote('')
      setDecision('')
      const rs = await api.cubeResearch(id, current.id)
      setResearch(rs.items ?? [])
    } catch {
      Taro.showToast({ title: '操作失败', icon: 'none' })
    }
  }

  const taskAct = async (t: CubeTask, action: string, body: Record<string, unknown> = {}) => {
    try {
      await api.cubeTaskAction(id, t.id, action, body)
      const ts = await api.cubeTasks(id, current!.id)
      setTasks(ts.items ?? [])
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 130011 ? '驳回须填写原因' : '操作失败', icon: 'none' })
    }
  }

  return (
    <View className="page">
      {!current ? (
        <StateView loading={loading} error={error} onRetry={() => void loadProjects()}>
          <View className="menu">
            <View className="menu__item">
              <FormField label={type === 1 ? '新建研究项目' : '新建承接项目'} value={newName} onChange={setNewName} />
              <Button className="btn-primary" size="mini" onClick={() => void create()}>
                新建
              </Button>
            </View>
            {projects.map((p) => (
              <View key={p.id} className="menu__item menu__row" onClick={() => void openProject(p)}>
                <Text>{p.name}</Text>
                <Text className="row-tag">
                  {p.status === 1 ? '已关闭' : '进行中'}
                  {p.platform_status ? ` · ${PLATFORM[p.platform_status] ?? ''}` : ''}
                </Text>
              </View>
            ))}
          </View>
        </StateView>
      ) : (
        <>
          <View className="detail-head">
            <Text className="detail-head__title">{current.name}</Text>
            <Text className="detail-head__type">{current.status === 1 ? '已关闭' : '进行中'} · 进度 {current.progress}%</Text>
          </View>

          <View className="detail-block">
            <Text className="detail-block__title">分工待办 / 任务</Text>
            {tasks.map((t) => (
              <View key={t.id} className="menu__row" style={{ padding: '10px 0' }}>
                <Text>
                  {t.title} · {TASK_STATUS[t.status] ?? t.status}
                </Text>
                <View className="menu__row-extra">
                  {t.status === 0 ? (
                    <Text className="row-action" onClick={() => void taskAct(t, 'start')}>
                      开始
                    </Text>
                  ) : null}
                  {t.status === 1 ? (
                    <Text className="row-action" onClick={() => void taskAct(t, 'submit', { progress: 80 })}>
                      提交
                    </Text>
                  ) : null}
                  {t.status === 2 ? (
                    <Text className="row-action" onClick={() => void taskAct(t, 'confirm', { approve: true })}>
                      确认
                    </Text>
                  ) : null}
                  {t.status === 3 ? (
                    <Text className="row-action" onClick={() => void taskAct(t, 'reopen')}>
                      重开
                    </Text>
                  ) : null}
                </View>
              </View>
            ))}
            {tasks.length === 0 ? <Text className="form-field__label">暂无任务</Text> : null}
          </View>

          {type === 1 ? (
            <View className="detail-block">
              <Text className="detail-block__title">研究记录 → 决策结论</Text>
              {research.map((r) => (
                <Text key={r.id} className="detail-block__body">
                  {r.content}
                  {r.decision ? `（决策：${r.decision}）` : ''}
                </Text>
              ))}
              <FormField label="研究记录" value={note} onChange={setNote} />
              <FormField label="决策结论" value={decision} onChange={setDecision} />
              <Button className="btn-secondary" size="mini" onClick={() => void addResearch()}>
                添加记录
              </Button>
            </View>
          ) : null}

          <View className="detail-actions">
            <Button className="btn-secondary" onClick={() => Taro.navigateTo({ url: `/pages/cube/report?id=${id}&project=${current.id}` })}>
              重点客户报备
            </Button>
            <Button
              className="btn-primary"
              onClick={async () => {
                await (current.status === 1 ? api.reopenCubeProject(id, current.id) : api.closeCubeProject(id, current.id))
                void openProject({ ...current, status: current.status === 1 ? 0 : 1 })
              }}
            >
              {current.status === 1 ? '重开项目' : '关闭项目'}
            </Button>
          </View>
        </>
      )}
    </View>
  )
}
