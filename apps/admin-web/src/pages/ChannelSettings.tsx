import SettingsCenter from '../components/SettingsCenter'

// 运营管理 → 渠道与佣金：渠道佣金比例/范围/保证金等配置。
export default function ChannelSettings() {
  return <SettingsCenter only="channel" />
}
