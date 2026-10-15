import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { Card } from '@catcube/api-client'
import { api } from '../../services/request'
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

// 名片编辑：第二层五项（区域/行业/合作需求/合作供给/简介）+ 增值画像与联系方式。
export default function CardEdit() {
  const [form, setForm] = useState<FormState>(EMPTY)
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

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
