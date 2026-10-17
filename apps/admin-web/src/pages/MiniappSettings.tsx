import SettingsCenter from '../components/SettingsCenter'

// 系统 → 小程序设置：AppID/AppSecret/小程序码环境（super admin 可写，热生效）。
export default function MiniappSettings() {
  return <SettingsCenter only="miniapp" />
}
