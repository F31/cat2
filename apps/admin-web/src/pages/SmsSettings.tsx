import SettingsCenter from '../components/SettingsCenter'

// 系统管理 → 系统配置 → 短信网关。
export default function SmsSettings() {
  return <SettingsCenter only="sms" />
}
