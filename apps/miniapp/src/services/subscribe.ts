import Taro from '@tarojs/taro'
import { api } from './request'

// askSubscribe 在用户手势上下文中请求微信订阅授权，并把授权额度记到服务端。
export async function askSubscribe(): Promise<void> {
  try {
    const { items } = await api.subscribeTemplates()
    if (!items.length) {
      Taro.showToast({ title: '通知模板尚未配置', icon: 'none' })
      return
    }
    const picked = items.slice(0, 3)
    const res = (await Taro.requestSubscribeMessage({ tmplIds: picked.map((it) => it.template_id) } as never)) as Record<string, string>
    let accepted = 0
    for (const it of picked) {
      if (res[it.template_id] === 'accept') {
        await api.applySubscribeQuota(it.code, 1)
        accepted++
      }
    }
    Taro.showToast({ title: accepted > 0 ? '已开启通知' : '未开启通知', icon: 'none' })
  } catch {
    Taro.showToast({ title: '通知授权失败，请稍后重试', icon: 'none' })
  }
}
