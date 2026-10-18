import { useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import FormField from '../../components/FormField'
import TagPicker from '../../components/TagPicker'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'

// 创建猫立方：组名/区域/行业/方向/有啥/优势/合作 + 必配猫蜜（用户 ID）。
export default function CubeCreate() {
  const [name, setName] = useState('')
  const [region, setRegion] = useState('')
  const [industry, setIndustry] = useState<string[]>([])
  const [direction, setDirection] = useState('')
  const [resources, setResources] = useState('')
  const [strength, setStrength] = useState('')
  const [cooperation, setCooperation] = useState('')
  const [honey, setHoney] = useState('')
  const [honeyPhone, setHoneyPhone] = useState('')
  const [honeyName, setHoneyName] = useState('')
  const [saving, setSaving] = useState(false)
  const [searching, setSearching] = useState(false)

  const findHoney = async () => {
    if (!honeyPhone.trim()) {
      Taro.showToast({ title: '请输入手机号', icon: 'none' })
      return
    }
    setSearching(true)
    try {
      await ensureLogin()
      const r = await api.lookupCubeHoney(honeyPhone.trim())
      if (!r.found || !r.user_id) {
        setHoney('')
        setHoneyName('')
        Taro.showToast({ title: '未找到已注册用户', icon: 'none' })
        return
      }
      setHoney(r.user_id)
      setHoneyName(r.nickname || r.user_id)
      Taro.showToast({ title: `已选择 ${r.nickname || r.user_id}`, icon: 'success' })
    } catch {
      Taro.showToast({ title: '查找失败', icon: 'none' })
    } finally {
      setSearching(false)
    }
  }

  const submit = async () => {
    if (!name.trim()) {
      Taro.showToast({ title: '请填写组名', icon: 'none' })
      return
    }
    if (!honey.trim()) {
      Taro.showToast({ title: '请先按手机号查找猫蜜', icon: 'none' })
      return
    }
    setSaving(true)
    try {
      await ensureLogin()
      const cube = await api.createCube({
        name,
        region_code: region || undefined,
        industry_codes: industry,
        direction, resources, strength, cooperation,
        honey_user_id: honey,
      })
      Taro.showToast({ title: '创建成功', icon: 'success' })
      setTimeout(() => Taro.redirectTo({ url: `/pages/cube/detail?id=${cube.id}` }), 600)
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 130003 ? '需先完成实名认证' : '创建失败', icon: 'none' })
      if (code === 130003) void Taro.navigateTo({ url: '/pages/verify/index' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <View className="page">
      <FormField label="组名" required value={name} onChange={setName} />
      <TagPicker label="区域" dimension="region" multiple={false} value={region ? [region] : []} onChange={(v) => setRegion(v[0] ?? '')} />
      <TagPicker label="行业" dimension="industry" value={industry} onChange={setIndustry} />
      <FormField label="方向" value={direction} onChange={setDirection} />
      <FormField label="有啥（资源）" value={resources} onChange={setResources} />
      <FormField label="优势" value={strength} onChange={setStrength} />
      <FormField label="合作诉求" value={cooperation} onChange={setCooperation} />
      <FormField label="猫蜜手机号" required value={honeyPhone} onChange={setHoneyPhone} placeholder="输入对方注册手机号" />
      <Button className="btn-secondary" size="mini" loading={searching} onClick={() => void findHoney()}>
        查找猫蜜
      </Button>
      <Text className="form-field__label">
        {honey ? `已选择：${honeyName || `用户 ${honey}`}（${honey}）` : '仅支持已注册且实名的用户；也可在下方直接填写用户 ID'}
      </Text>
      <FormField label="猫蜜用户 ID（备选）" value={honey} onChange={setHoney} placeholder="按手机号查找后自动填入" />
      <Button className="btn-primary" loading={saving} onClick={() => void submit()}>
        创建
      </Button>
    </View>
  )
}
