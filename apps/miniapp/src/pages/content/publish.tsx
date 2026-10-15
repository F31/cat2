import { useState } from 'react'
import Taro, { useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import FormField from '../../components/FormField'
import TagPicker from '../../components/TagPicker'
import Uploader from '../../components/Uploader'

interface FieldDef {
  key: string
  label: string
  required?: boolean
  multiline?: boolean
  number?: boolean
}

const KINDS: { key: string; label: string }[] = [
  { key: 'project', label: '项目' },
  { key: 'product', label: '产品' },
  { key: 'meeting', label: '会议' },
  { key: 'event', label: '活动' },
]

// 四类字段各自独立（会议与活动共用「时间/地点/报名」组件但字段不合并）。
const FIELDS: Record<string, FieldDef[]> = {
  project: [
    { key: 'amount_wan', label: '金额（万元）', required: true, number: true },
    { key: 'stage', label: '阶段' },
    { key: 'core_strength', label: '核心优势', multiline: true },
    { key: 'key_points', label: '关键点', multiline: true },
    { key: 'intro', label: '简介', multiline: true },
    { key: 'target_customers', label: '目标客户' },
    { key: 'target_channels', label: '目标渠道' },
    { key: 'contact_phone', label: '联系方式' },
  ],
  product: [
    { key: 'warehouse_status', label: '库存状态', number: true },
    { key: 'target_customers', label: '目标客户' },
    { key: 'target_channels', label: '目标渠道' },
    { key: 'core_strength', label: '核心优势', multiline: true },
    { key: 'key_points', label: '关键点', multiline: true },
    { key: 'intro', label: '简介', multiline: true },
    { key: 'contact_phone', label: '联系方式' },
  ],
  meeting: [
    { key: 'start_at', label: '开始时间（ISO）', required: true },
    { key: 'end_at', label: '结束时间（ISO）', required: true },
    { key: 'location', label: '地点', required: true },
    { key: 'venue_point', label: '场馆点位' },
    { key: 'participant_limit', label: '人数上限', number: true },
    { key: 'signup_method', label: '报名方式' },
    { key: 'intro', label: '简介', multiline: true },
  ],
  event: [
    { key: 'start_at', label: '开始时间（ISO）', required: true },
    { key: 'end_at', label: '结束时间（ISO）', required: true },
    { key: 'location', label: '地点', required: true },
    { key: 'participant_limit', label: '人数上限', number: true },
    { key: 'participant_count', label: '已报名人数', number: true },
    { key: 'signup_method', label: '报名方式' },
    { key: 'intro', label: '简介', multiline: true },
  ],
}

export default function Publish() {
  const router = useRouter()
  const [kind, setKind] = useState(router.params.kind && FIELDS[router.params.kind] ? router.params.kind : 'project')
  const [title, setTitle] = useState('')
  const [cover, setCover] = useState('')
  const [region, setRegion] = useState('')
  const [industry, setIndustry] = useState<string[]>([])
  const [need, setNeed] = useState<string[]>([])
  const [offer, setOffer] = useState<string[]>([])
  const [fields, setFields] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)

  const defs = FIELDS[kind] ?? []
  const setField = (k: string, v: string) => setFields((f) => ({ ...f, [k]: v }))

  const submit = async () => {
    if (!title.trim()) {
      Taro.showToast({ title: '请填写标题', icon: 'none' })
      return
    }
    for (const d of defs) {
      if (d.required && !fields[d.key]) {
        Taro.showToast({ title: `请填写${d.label}`, icon: 'none' })
        return
      }
    }
    setSubmitting(true)
    try {
      await ensureLogin()
      const payload: Record<string, unknown> = {}
      for (const d of defs) {
        const raw = fields[d.key]
        if (raw === undefined || raw === '') continue
        payload[d.key] = d.number ? Number(raw) : raw
      }
      const item = await api.publishContent({
        kind,
        title,
        cover_file_id: cover || undefined,
        region_code: region || undefined,
        tagset: { industry, need, offer },
        fields: payload,
      })
      const id = String(item.id ?? '')
      if (id) await api.submitContent(id)
      Taro.showToast({ title: '已提交审核', icon: 'success' })
      setTimeout(() => Taro.navigateBack(), 700)
    } catch {
      Taro.showToast({ title: '发布失败（需实名）', icon: 'none' })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <View className="page">
      <View className="tabs">
        {KINDS.map((k) => (
          <Text
            key={k.key}
            className={k.key === kind ? 'tabs__item tabs__item--active' : 'tabs__item'}
            onClick={() => {
              setKind(k.key)
              setFields({})
            }}
          >
            {k.label}
          </Text>
        ))}
      </View>

      <FormField label="标题" required value={title} onChange={setTitle} />
      <Uploader label="封面" max={1} value={cover ? [cover] : []} onChange={(v) => setCover(v[0] ?? '')} />
      <TagPicker label="区域" dimension="region" multiple={false} value={region ? [region] : []} onChange={(v) => setRegion(v[0] ?? '')} />
      <TagPicker label="行业" dimension="industry" value={industry} onChange={setIndustry} />
      <TagPicker label="合作需求" dimension="cooperation_demand" value={need} onChange={setNeed} />
      <TagPicker label="合作供给" dimension="cooperation_supply" value={offer} onChange={setOffer} />

      {defs.map((d) => (
        <FormField
          key={d.key}
          label={d.label}
          required={d.required}
          multiline={d.multiline}
          value={fields[d.key] ?? ''}
          onChange={(v) => setField(d.key, v)}
        />
      ))}

      <Button className="btn-primary" loading={submitting} onClick={() => void submit()}>
        提交审核
      </Button>
    </View>
  )
}
