import { useEffect, useState } from 'react'
import Taro, { useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import FormField from '../../components/FormField'
import TagPicker from '../../components/TagPicker'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'

// 编辑猫立方「关于我们」（猫王/猫蜜）。
export default function CubeEdit() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [name, setName] = useState('')
  const [region, setRegion] = useState('')
  const [industry, setIndustry] = useState<string[]>([])
  const [chain, setChain] = useState('')
  const [direction, setDirection] = useState('')
  const [resources, setResources] = useState('')
  const [strength, setStrength] = useState('')
  const [cooperation, setCooperation] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    void (async () => {
      try {
        await ensureLogin()
        const c = await api.cubeDetail(id)
        setName(c.name)
        setRegion(c.region_code ?? '')
        setIndustry(c.industry_codes ?? [])
        setChain(c.chain_stage ?? '')
        setDirection(c.direction ?? '')
        setResources(c.resources ?? '')
        setStrength(c.strength ?? '')
        setCooperation(c.cooperation ?? '')
      } catch {
        Taro.showToast({ title: '加载失败', icon: 'none' })
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  const submit = async () => {
    if (!name.trim()) {
      Taro.showToast({ title: '请填写组名', icon: 'none' })
      return
    }
    setSaving(true)
    try {
      await ensureLogin()
      await api.updateCube(id, {
        name: name.trim(),
        region_code: region || undefined,
        industry_codes: industry,
        chain_stage: chain,
        direction,
        resources,
        strength,
        cooperation,
      })
      Taro.showToast({ title: '已保存', icon: 'success' })
      setTimeout(() => Taro.navigateBack(), 500)
    } catch {
      Taro.showToast({ title: '仅猫王/猫蜜可编辑', icon: 'none' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <View className="page">
      <FormField label="组名" required value={name} onChange={setName} />
      <TagPicker label="区域" dimension="region" multiple={false} value={region ? [region] : []} onChange={(v) => setRegion(v[0] ?? '')} />
      <TagPicker label="行业" dimension="industry" value={industry} onChange={setIndustry} />
      <FormField label="产业链阶段" value={chain} onChange={setChain} />
      <FormField label="方向" value={direction} onChange={setDirection} />
      <FormField label="有啥（资源）" value={resources} onChange={setResources} />
      <FormField label="优势" value={strength} onChange={setStrength} />
      <FormField label="合作诉求" value={cooperation} onChange={setCooperation} />
      <Button className="btn-primary" loading={saving} onClick={() => void submit()}>
        保存
      </Button>
    </View>
  )
}
