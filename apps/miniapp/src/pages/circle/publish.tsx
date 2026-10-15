import { useState } from 'react'
import Taro, { useRouter } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import FormField from '../../components/FormField'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'

// 圈内发布：需求（标题≤30/内容≤500/有效期/可见范围普通仅本圈、会员全平台）或动态/公告。
export default function CirclePublish() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [tab, setTab] = useState<'demand' | 'feed'>('demand')
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [scope, setScope] = useState(1)
  const [announcement, setAnnouncement] = useState(false)
  const [saving, setSaving] = useState(false)

  const submit = async () => {
    setSaving(true)
    try {
      await ensureLogin()
      if (tab === 'demand') {
        if (!title.trim() || !content.trim()) {
          Taro.showToast({ title: '请填写标题与内容', icon: 'none' })
          return
        }
        await api.publishCircleDemand(id, { title, content, scope })
      } else {
        await api.publishCircleFeed(id, content, announcement)
      }
      Taro.showToast({ title: '已发布', icon: 'success' })
      setTimeout(() => Taro.navigateBack(), 600)
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({
        title: code === 140008 ? '今日发布额度已满（3/日）' : code === 140009 ? '公告额度已满（3/圈/日）' : '发布失败',
        icon: 'none',
      })
    } finally {
      setSaving(false)
    }
  }

  return (
    <View className="page">
      <View className="tabs">
        <Text className={tab === 'demand' ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setTab('demand')}>
          需求
        </Text>
        <Text className={tab === 'feed' ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setTab('feed')}>
          动态 / 公告
        </Text>
      </View>

      {tab === 'demand' ? (
        <>
          <FormField label="标题（≤30 字）" required value={title} onChange={setTitle} />
          <FormField label="内容（≤500 字）" required value={content} onChange={setContent} />
          <View className="tabs">
            <Text className={scope === 1 ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setScope(1)}>
              仅本圈
            </Text>
            <Text className={scope === 2 ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setScope(2)}>
              全平台（会员）
            </Text>
          </View>
        </>
      ) : (
        <>
          <FormField label="内容" required value={content} onChange={setContent} />
          <View className="agreement" onClick={() => setAnnouncement(!announcement)}>
            <Text className={announcement ? 'checkbox checkbox--on' : 'checkbox'}>{announcement ? '✓' : ''}</Text>
            <Text className="agreement__text">作为圈务公告（仅圈主）</Text>
          </View>
        </>
      )}
      <Button className="btn-primary" loading={saving} onClick={() => void submit()}>
        发布
      </Button>
    </View>
  )
}
