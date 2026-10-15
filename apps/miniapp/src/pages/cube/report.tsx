import { useState } from 'react'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { CubeReport } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'
import FormField from '../../components/FormField'

const STATUS: Record<number, string> = { 0: '待确认', 1: '已确认', 2: '已驳回' }

// 重点客户报备：按状态分组展示；提交（重复仅提示）；猫王/猫蜜确认或驳回（驳回须原因）。
export default function CubeReportPage() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const project = router.params.project ?? ''
  const [items, setItems] = useState<CubeReport[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [name, setName] = useState('')
  const [contact, setContact] = useState('')
  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.cubeReports(id, project || undefined)
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

  const submit = async () => {
    if (!name.trim()) {
      Taro.showToast({ title: '请填写客户名称', icon: 'none' })
      return
    }
    try {
      const res = await api.submitCubeReport(id, {
        project_id: project || undefined,
        customer_name: name,
        contact_phone: contact,
        amount_wan: Number(amount) || 0,
        note,
      })
      if (res.duplicate) Taro.showToast({ title: '已存在同名报备（仅提示）', icon: 'none' })
      setName(''); setContact(''); setAmount(''); setNote('')
      void load()
    } catch {
      Taro.showToast({ title: '提交失败', icon: 'none' })
    }
  }

  const decide = async (r: CubeReport, approve: boolean) => {
    try {
      const reason = approve ? '' : '信息不全'
      await api.decideCubeReport(id, r.id, approve, reason)
      void load()
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 130011 ? '驳回须填写原因' : '操作失败', icon: 'none' })
    }
  }

  return (
    <View className="page">
      <View className="detail-block">
        <Text className="detail-block__title">新增报备</Text>
        <FormField label="客户名称" required value={name} onChange={setName} />
        <FormField label="联系人手机号" value={contact} onChange={setContact} />
        <FormField label="预计金额（万元）" value={amount} onChange={setAmount} />
        <FormField label="说明（≤200 字）" value={note} onChange={setNote} />
        <Button className="btn-primary" size="mini" onClick={() => void submit()}>
          提交企业确认
        </Button>
      </View>
      <StateView loading={loading} error={error} empty={items.length === 0} emptyText="暂无报备" onRetry={() => void load()}>
        <View className="menu">
          {items.map((r) => (
            <View key={r.id} className="menu__item menu__row">
              <Text>
                {r.customer_name} · {STATUS[r.status] ?? r.status}
              </Text>
              {r.status === 0 ? (
                <View className="menu__row-extra">
                  <Text className="row-action" onClick={() => void decide(r, true)}>
                    确认
                  </Text>
                  <Text className="row-action" onClick={() => void decide(r, false)}>
                    驳回
                  </Text>
                </View>
              ) : null}
            </View>
          ))}
        </View>
      </StateView>
    </View>
  )
}
