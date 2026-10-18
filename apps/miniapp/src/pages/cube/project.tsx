import { useState } from 'react'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import type { CubeLinkRequest, CubeMember, CubeProject, CubeTask, CubeResearch } from '@catcube/api-client'
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
  const [members, setMembers] = useState<CubeMember[]>([])
  const [links, setLinks] = useState<CubeLinkRequest[]>([])
  const [taskTitle, setTaskTitle] = useState('')
  const [taskAssignee, setTaskAssignee] = useState('')
  const [linkId, setLinkId] = useState('')
  const [rejecting, setRejecting] = useState<CubeTask | null>(null)
  const [rejectReason, setRejectReason] = useState('')

  const loadProjects = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const [res, ms] = await Promise.all([api.cubeProjects(id, type), api.cubeMembers(id)])
      setProjects(res.items ?? [])
      setMembers((ms.items ?? []).filter((m) => m.status === 0))
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  const openProject = async (p: CubeProject) => {
    setCurrent(p)
    try {
      const [ts, rs, lr] = await Promise.all([
        api.cubeTasks(id, p.id), api.cubeResearch(id, p.id), api.cubeLinkRequests(id),
      ])
      setTasks(ts.items ?? [])
      setResearch(rs.items ?? [])
      setLinks((lr.items ?? []).filter((l) => l.content_project_id === p.content_project_id || l.status === 0))
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

  const pickTaskAssignee = async () => {
    if (members.length === 0) return
    try {
      const r = await Taro.showActionSheet({ itemList: members.map((m) => m.nickname || m.user_id) })
      const m = members[r.tapIndex]
      if (m) setTaskAssignee(m.user_id)
    } catch {
      /* 取消 */
    }
  }

  const createTask = async () => {
    if (!current || !taskTitle.trim() || !taskAssignee) {
      Taro.showToast({ title: '请填写任务并选择负责人', icon: 'none' })
      return
    }
    try {
      await api.createCubeTask(id, { project_id: current.id, title: taskTitle, assignee_id: taskAssignee })
      setTaskTitle(''); setTaskAssignee('')
      const ts = await api.cubeTasks(id, current.id)
      setTasks(ts.items ?? [])
    } catch {
      Taro.showToast({ title: '仅猫王/猫蜜可建任务', icon: 'none' })
    }
  }

  const openChat = async () => {
    if (!current) return
    try {
      const s = await api.ensureCubeProjectChat(id, current.id)
      Taro.navigateTo({ url: `/pages/msg/chat?session=${encodeURIComponent(s.session_id)}&peer=${encodeURIComponent(s.title)}` })
    } catch {
      Taro.showToast({ title: '仅成员可进入群聊', icon: 'none' })
    }
  }

  const requestLink = async () => {
    if (!linkId.trim()) {
      Taro.showToast({ title: '请输入平台项目 ID', icon: 'none' })
      return
    }
    try {
      const r = await api.requestCubeLink(id, linkId.trim())
      Taro.showToast({ title: r.direct ? '已关联平台项目' : '已提交引入申请', icon: 'success' })
      setLinkId('')
      if (current) void openProject(current)
    } catch {
      Taro.showToast({ title: '关联失败（无权限或项目不存在）', icon: 'none' })
    }
  }

  const publish = async () => {
    if (!current) return
    try {
      const r = await api.publishCubeProject(id, current.id)
      Taro.showToast({ title: `已发布到平台（${r.content_project_id}）`, icon: 'success' })
      void openProject({ ...current, content_project_id: r.content_project_id })
    } catch {
      Taro.showToast({ title: '发布失败（仅猫王/猫蜜）', icon: 'none' })
    }
  }

  const decideLink = async (req: CubeLinkRequest, approve: boolean) => {
    try {
      await api.decideCubeLink(id, req.id, approve)
      if (current) void openProject(current)
    } catch {
      Taro.showToast({ title: '仅项目方/猫蜜可确认', icon: 'none' })
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
          {current.source_ref ? <Text className="form-field__label">来源：{current.source_ref}</Text> : null}
          {current.content_project_id ? <Text className="form-field__label">平台项目：{current.content_project_id}</Text> : null}
          <View className="detail-actions">
            <Button className="btn-secondary" size="mini" onClick={() => void openChat()}>
              项目群
            </Button>
            {!current.content_project_id ? (
              <Button className="btn-secondary" size="mini" onClick={() => void publish()}>
                发布到平台
              </Button>
            ) : null}
          </View>

          <View className="detail-block">
            <Text className="detail-block__title">分工待办 / 任务</Text>
            <FormField label="新任务" value={taskTitle} onChange={setTaskTitle} />
            <View className="menu__row" style={{ padding: '8px 0' }} onClick={() => void pickTaskAssignee()}>
              <Text>负责人</Text>
              <Text className="row-tag">{taskAssignee ? members.find((m) => m.user_id === taskAssignee)?.nickname || taskAssignee : '选择成员'}</Text>
            </View>
            <Button className="btn-secondary" size="mini" onClick={() => void createTask()}>
              新建任务
            </Button>
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
                    <>
                      <Text className="row-action" onClick={() => setRejecting(t)}>驳回</Text>
                      <Text className="row-action" onClick={() => void taskAct(t, 'confirm', { approve: true })}>确认</Text>
                    </>
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

          <View className="detail-block">
            <Text className="detail-block__title">平台项目关联 / 引入申请</Text>
            {current.content_project_id ? (
              <Text className="detail-block__body">已关联平台项目 {current.content_project_id}</Text>
            ) : (
              <>
                <FormField label="平台项目 ID" value={linkId} onChange={setLinkId} placeholder="输入 content.project 的数字 ID" />
                <Button className="btn-secondary" size="mini" onClick={() => void requestLink()}>
                  关联 / 申请引入
                </Button>
              </>
            )}
            {links.map((l) => (
              <View key={l.id} className="menu__row" style={{ padding: '8px 0' }}>
                <Text>引入申请 {l.content_project_id} · {l.status === 0 ? '待确认' : l.status === 1 ? '已同意' : '已拒绝'}</Text>
                {l.status === 0 ? (
                  <View className="menu__row-extra">
                    <Text className="row-action" onClick={() => void decideLink(l, false)}>拒绝</Text>
                    <Text className="row-action" onClick={() => void decideLink(l, true)}>同意</Text>
                  </View>
                ) : null}
              </View>
            ))}
          </View>

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

      {rejecting ? (
        <View className="auth-mask">
          <View className="auth-modal">
            <Text className="auth-modal__title">驳回任务</Text>
            <Input className="search-row__input" value={rejectReason} placeholder="请填写驳回原因" onInput={(e) => setRejectReason(e.detail.value)} />
            <View className="auth-modal__actions">
              <Button className="btn-secondary auth-modal__btn" onClick={() => { setRejecting(null); setRejectReason('') }}>
                取消
              </Button>
              <Button
                className="btn-danger auth-modal__btn"
                onClick={() => {
                  if (!rejectReason.trim()) {
                    Taro.showToast({ title: '请填写原因', icon: 'none' })
                    return
                  }
                  void taskAct(rejecting, 'confirm', { approve: false, reason: rejectReason.trim() })
                  setRejecting(null)
                  setRejectReason('')
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
