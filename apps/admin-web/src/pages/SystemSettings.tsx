import SettingsCenter from '../components/SettingsCenter'

// 系统设置：由 SettingsSchema 自动渲染（§8.2；分组自动、无需手写页面）。
export default function SystemSettings() {
  return <SettingsCenter exclude={['channel', 'membership', 'miniapp', 'sms', 'ai']} />
}
