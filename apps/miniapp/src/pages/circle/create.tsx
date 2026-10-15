import { useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import FormField from '../../components/FormField'
import TagPicker from '../../components/TagPicker'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'

const DURATIONS = [7, 30, 90, 180]

// 建圈：常设/临时（临时时长选定不可改，默认 30 天）；已实名门槛。
export default function CircleCreate() {
  const [name, setName] = useState('')
  const [intro, setIntro] = useState('')
  const [region, setRegion] = useState('')
  const [type, setType] = useState(2)
  const [days, setDays] = useState(30)
  const [saving, setSaving] = useState(false)

  const submit = async () => {
    if (!name.trim()) {
      Taro.showToast({ title: '请填写圈名', icon: 'none' })
      return
    }
    setSaving(true)
    try {
      await ensureLogin()
      const c = await api.createCircle({ name, intro, region_code: region || undefined, type, duration_days: days })
      Taro.showToast({ title: '创建成功', icon: 'success' })
      setTimeout(() => Taro.redirectTo({ url: `/pages/circle/detail?id=${c.id}` }), 600)
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      if (code === 140003) {
        Taro.showToast({ title: '需先完成实名认证', icon: 'none' })
        void Taro.navigateTo({ url: '/pages/verify/index' })
      } else if (code === 140005) {
        Taro.showToast({ title: '建圈已达上限（1/日、最多 3 个）', icon: 'none' })
      } else {
        Taro.showToast({ title: '创建失败', icon: 'none' })
      }
    } finally {
      setSaving(false)
    }
  }

  return (
    <View className="page">
      <View className="tabs">
        <Text className={type === 2 ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setType(2)}>
          临时圈
        </Text>
        <Text className={type === 1 ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setType(1)}>
          常设圈
        </Text>
      </View>
      {type === 2 ? (
        <View className="tabs">
          {DURATIONS.map((d) => (
            <Text key={d} className={days === d ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setDays(d)}>
              {d} 天
            </Text>
          ))}
        </View>
      ) : null}
      <FormField label="圈名" required value={name} onChange={setName} />
      <FormField label="简介" value={intro} onChange={setIntro} />
      <TagPicker label="区域" dimension="region" multiple={false} value={region ? [region] : []} onChange={(v) => setRegion(v[0] ?? '')} />
      <Button className="btn-primary" loading={saving} onClick={() => void submit()}>
        创建圈子
      </Button>
    </View>
  )
}
