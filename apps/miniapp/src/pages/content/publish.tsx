import { useState } from 'react'
import Taro, { useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { api } from '../../services/request'
import { draftPublish, AI_PUBLISH } from '../../services/ai'
import { ensureLogin } from '../../services/auth'
import AiEntry from '../../ai-entry'
import FormField from '../../components/FormField'
import DateTimeField from '../../components/DateTimeField'
import TagPicker from '../../components/TagPicker'
import Uploader from '../../components/Uploader'

interface FieldDef {
  key: string
  label: string
  required?: boolean
  multiline?: boolean
  number?: boolean
  datetime?: boolean
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
    { key: 'start_at', label: '开始时间', required: true, datetime: true },
    { key: 'end_at', label: '结束时间', required: true, datetime: true },
    { key: 'location', label: '地点', required: true },
    { key: 'venue_point', label: '场馆点位' },
    { key: 'participant_limit', label: '人数上限', number: true },
    { key: 'signup_method', label: '报名方式' },
    { key: 'intro', label: '简介', multiline: true },
  ],
  event: [
    { key: 'start_at', label: '开始时间', required: true, datetime: true },
    { key: 'end_at', label: '结束时间', required: true, datetime: true },
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
  const [aiCallID, setAiCallID] = useState('')
  const [autoCircle, setAutoCircle] = useState(false)

  // 采纳反馈（仅在有草稿且已提交时记录）
  const markAdopted = (adopted: boolean) => {
    if (!aiCallID) return
    void api.aiFeedback(aiCallID, adopted).catch(() => undefined)
    setAiCallID('')
  }

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
      let circleCreated = false
      let circleFailed = false
      if (id && autoCircle && (kind === 'event' || kind === 'meeting')) {
        try {
          await api.createCircleFromContent(`content.${kind}:${id}`)
          circleCreated = true
        } catch {
          circleFailed = true
        }
      }
      markAdopted(true) // AI 草稿被采用（人在回路确认后）
      Taro.showToast({
        title: circleCreated ? '已提交并创建活动圈' : circleFailed ? '已提交，自动建圈失败' : '已提交审核',
        icon: circleCreated ? 'success' : 'none',
      })
      setTimeout(() => Taro.navigateBack(), 700)
    } catch (e) {
      // 110004：发布需先实名 → 引导实名认证；其余为表单/网络错误。
      if (e && typeof e === 'object' && 'code' in e && (e as { code: number }).code === 110004) {
        Taro.showToast({ title: '发布需先完成实名认证', icon: 'none' })
        void Taro.navigateTo({ url: '/pages/verify/index' })
      } else {
        Taro.showToast({ title: '发布失败，请重试', icon: 'none' })
      }
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
              setAutoCircle(false)
            }}
          >
            {k.label}
          </Text>
        ))}
      </View>

      <AiEntry
        flag={AI_PUBLISH}
        hint="一句话生成草稿（生成后请确认，提交仍走原校验）"
        placeholder="例如：华东重工寻钣金供应商，预算 100 万"
        buttonLabel="AI 生成草稿"
        unavailableHint="AI 暂不可用，请手动填写"
        run={async (aiText) => {
          const d = await draftPublish(kind, aiText)
          if (d.title) setTitle(d.title)
          setFields((f) => {
            const next = { ...f }
            for (const x of d.fields ?? []) {
              if (defs.some((dd) => dd.key === x.field)) next[x.field] = x.value
            }
            return next
          })
          setAiCallID(d.call_id)
          Taro.showToast({ title: '草稿已生成，请确认后提交', icon: 'none' })
        }}
      />

      <FormField label="标题" required value={title} onChange={setTitle} />
      <Uploader label="封面" max={1} value={cover ? [cover] : []} onChange={(v) => setCover(v[0] ?? '')} />
      <TagPicker label="区域" dimension="region" multiple={false} value={region ? [region] : []} onChange={(v) => setRegion(v[0] ?? '')} />
      <TagPicker label="行业" dimension="industry" value={industry} onChange={setIndustry} />
      <TagPicker label="合作需求" dimension="cooperation_demand" value={need} onChange={setNeed} />
      <TagPicker label="合作供给" dimension="cooperation_supply" value={offer} onChange={setOffer} />

      {defs.map((d) =>
        d.datetime ? (
          <DateTimeField
            key={d.key}
            label={d.label}
            required={d.required}
            value={fields[d.key] ?? ''}
            onChange={(v) => setField(d.key, v)}
          />
        ) : (
          <FormField
            key={d.key}
            label={d.label}
            required={d.required}
            multiline={d.multiline}
            value={fields[d.key] ?? ''}
            onChange={(v) => setField(d.key, v)}
          />
        ),
      )}

      {kind === 'event' || kind === 'meeting' ? (
        <View className="menu__item" onClick={() => setAutoCircle((v) => !v)}>
          <View className="menu__row">
            <Text>自动创建圈子</Text>
            <Text className="row-tag">{autoCircle ? '已开启' : '关闭'}</Text>
          </View>
          <Text className="form-field__label">开启后，报名者自动加入本活动圈子；活动结束后圈子保留。</Text>
        </View>
      ) : null}

      <Button className="btn-primary" loading={submitting} onClick={() => void submit()}>
        提交审核
      </Button>
    </View>
  )
}
