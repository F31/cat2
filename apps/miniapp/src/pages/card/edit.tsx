import { useEffect, useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import type { Card } from '@catcube/api-client'
import { api } from '../../services/request'
import { aiFeatures, draftOnboard, isAiUnavailable, AI_ONBOARD } from '../../services/ai'
import { ensureLogin } from '../../services/auth'
import FormField from '../../components/FormField'
import TagPicker from '../../components/TagPicker'
import Uploader from '../../components/Uploader'
import StateView from '../../components/StateView'

interface FormState {
  subject_type_code: string
  name: string
  avatar_file_id: string
  region_code: string
  industry_codes: string[]
  need_codes: string[]
  offer_codes: string[]
  advantage_field: string
  background: string
  target_customers: string
  target_channels: string
  core_strength: string
  intro: string
  contact_phone: string
}

const EMPTY: FormState = {
  subject_type_code: 'person',
  name: '',
  avatar_file_id: '',
  region_code: '',
  industry_codes: [],
  need_codes: [],
  offer_codes: [],
  advantage_field: '',
  background: '',
  target_customers: '',
  target_channels: '',
  core_strength: '',
  intro: '',
  contact_phone: '',
}

// AI 一句话入驻入口的提示文案（开关关闭时入口不展示）
// 名片编辑：第二层五项（区域/行业/合作需求/合作供给/简介）+ 增值画像与联系方式。
export default function CardEdit() {
  const [form, setForm] = useState<FormState>(EMPTY)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [aiOn, setAiOn] = useState(false)
  const [aiText, setAiText] = useState('')
  const [aiBusy, setAiBusy] = useState(false)
  const [aiCallID, setAiCallID] = useState('')

  // AI 一句话入驻入口：仅开关开启时展示，关闭即纯手工填写
  useEffect(() => {
    void aiFeatures().then((f) => setAiOn(!!f[AI_ONBOARD]))
  }, [])

  // 生成名片草稿（仅预填，保存仍走原字段校验）；不可用时提示手动填写
  const genDraft = async () => {
    if (!aiText.trim()) {
      Taro.showToast({ title: '请先一句话介绍自己/组织', icon: 'none' })
      return
    }
    setAiBusy(true)
    try {
      const d = await draftOnboard(aiText)
      const patch: Partial<FormState> = {}
      for (const x of d.fields ?? []) {
        if (x.field === 'real_name') patch.name = x.value
        if (x.field === 'intro') patch.intro = x.value
        if (x.field === 'core_strength') patch.core_strength = x.value
        if (x.field === 'background') patch.background = x.value
      }
      if (d.title && !patch.name) patch.name = d.title
      setForm((f) => ({ ...f, ...patch }))
      setAiCallID(d.call_id)
      Taro.showToast({ title: '草稿已生成，请确认后保存', icon: 'none' })
    } catch (e) {
      Taro.showToast({ title: isAiUnavailable(e) ? 'AI 暂不可用，请手动填写' : '生成失败，请手动填写', icon: 'none' })
    } finally {
      setAiBusy(false)
    }
  }

  const set = <K extends keyof FormState>(key: K, v: FormState[K]) => setForm((f) => ({ ...f, [key]: v }))

  const load = () => {
    setLoading(true)
    setError(false)
    void (async () => {
      try {
        await ensureLogin()
        const c: Card = await api.getMyCard()
        setForm({
          ...EMPTY,
          subject_type_code: c.subject_type_code || (c.identity_type === 2 ? 'organ' : 'person'),
          name: c.name || '',
          avatar_file_id: c.avatar_file_id || '',
          region_code: c.region_code || '',
          industry_codes: c.industry_codes ?? [],
          need_codes: c.need_codes ?? [],
          offer_codes: c.offer_codes ?? [],
          advantage_field: c.advantage_field ?? '',
          background: c.background ?? '',
          target_customers: c.target_customers ?? '',
          target_channels: c.target_channels ?? '',
          core_strength: c.core_strength ?? '',
          intro: c.intro ?? '',
        })
      } catch {
        setError(true)
      } finally {
        setLoading(false)
      }
    })()
  }

  useDidShow(() => load())

  const save = async () => {
    if (!form.name.trim()) {
      Taro.showToast({ title: '请填写名称', icon: 'none' })
      return
    }
    setSaving(true)
    try {
      await ensureLogin()
      await api.updateMyCard({
        subject_type_code: form.subject_type_code,
        name: form.name,
        avatar_file_id: form.avatar_file_id || undefined,
        region_code: form.region_code || undefined,
        industry_codes: form.industry_codes,
        need_codes: form.need_codes,
        offer_codes: form.offer_codes,
        advantage_field: form.advantage_field || undefined,
        background: form.background || undefined,
        target_customers: form.target_customers || undefined,
        target_channels: form.target_channels || undefined,
        core_strength: form.core_strength || undefined,
        intro: form.intro || undefined,
        contact_phone: form.contact_phone || undefined,
      })
      Taro.showToast({ title: '已保存', icon: 'success' })
      setTimeout(() => Taro.navigateBack(), 600)
    } catch {
      Taro.showToast({ title: '保存失败', icon: 'none' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <View className="page">
      <StateView loading={loading} error={error} onRetry={() => load()} />
      {!loading && !error ? (
        <>
      <View className="tabs">
        <Text
          className={form.subject_type_code === 'person' ? 'tabs__item tabs__item--active' : 'tabs__item'}
          onClick={() => set('subject_type_code', 'person')}
        >
          个人
        </Text>
        <Text
          className={form.subject_type_code === 'organ' ? 'tabs__item tabs__item--active' : 'tabs__item'}
          onClick={() => set('subject_type_code', 'organ')}
        >
          组织
        </Text>
      </View>

      {aiOn ? (
        <View className="ai-entry">
          <Text className="ai-entry__hint">一句话介绍，自动生成名片草稿（保存前请确认）</Text>
          <Input
            className="search-row__input"
            value={aiText}
            placeholder="例如：我是华东重工采购总监，寻找钣金供应商"
            onInput={(e) => setAiText(e.detail.value)}
          />
          <Button className="btn-secondary" size="mini" disabled={aiBusy} onClick={() => void genDraft()}>
            {aiBusy ? '生成中…' : 'AI 生成名片草稿'}
          </Button>
        </View>
      ) : null}

      <FormField label="名称" required value={form.name} onChange={(v) => set('name', v)} />
      <Uploader
        label="头像 / Logo"
        max={1}
        value={form.avatar_file_id ? [form.avatar_file_id] : []}
        onChange={(v) => set('avatar_file_id', v[0] ?? '')}
      />
      <TagPicker label="区域" dimension="region" multiple={false} value={form.region_code ? [form.region_code] : []} onChange={(v) => set('region_code', v[0] ?? '')} />
      <TagPicker label="行业" dimension="industry" value={form.industry_codes} onChange={(v) => set('industry_codes', v)} />
      <TagPicker label="合作需求" dimension="cooperation_demand" value={form.need_codes} onChange={(v) => set('need_codes', v)} />
      <TagPicker label="合作供给" dimension="cooperation_supply" value={form.offer_codes} onChange={(v) => set('offer_codes', v)} />
      <FormField label="简介" value={form.intro} multiline onChange={(v) => set('intro', v)} />
      <FormField label="核心优势" value={form.core_strength} multiline onChange={(v) => set('core_strength', v)} />
      <FormField label="优势领域" value={form.advantage_field} onChange={(v) => set('advantage_field', v)} />
      <FormField label="背景" value={form.background} onChange={(v) => set('background', v)} />
      <FormField label="目标客户" value={form.target_customers} onChange={(v) => set('target_customers', v)} />
      <FormField label="目标渠道" value={form.target_channels} onChange={(v) => set('target_channels', v)} />
      <FormField label="联系方式" value={form.contact_phone} placeholder="手机号（仅会员可见）" onChange={(v) => set('contact_phone', v)} />

      <Button className="btn-primary" loading={saving} onClick={() => void save()}>
        保存
      </Button>
        </>
      ) : null}
    </View>
  )
}
