import { View, Text } from '@tarojs/components'
import type { DiscoveryCard } from '@catcube/api-client'

const TYPE_LABEL: Record<string, string> = {
  user: '人',
  org: '企业',
  'content.project': '项目',
  'content.product': '产品',
  'content.meeting': '会议',
  'content.event': '活动',
}

const LEVEL_LABEL: Record<number, string> = { 1: 'VIP', 2: 'SVIP', 3: '企业版' }

export interface ContentCardProps {
  card: DiscoveryCard
  onClick?: (card: DiscoveryCard) => void
}

// 六类共用卡片壳（类型标签 + 标题 + 区域 + 会员/认证标识 + 推荐理由）。
export default function ContentCard({ card, onClick }: ContentCardProps) {
  return (
    <View className="content-card" onClick={() => onClick?.(card)}>
      <View className="content-card__head">
        <Text className="content-card__type">{TYPE_LABEL[card.type] ?? card.type}</Text>
        <Text className="content-card__title">{card.title || '未命名'}</Text>
      </View>
      <View className="content-card__meta">
        {card.region ? <Text className="content-card__region">{card.region}</Text> : null}
        {card.verified ? <Text className="tag tag--verified">已认证</Text> : null}
        {card.member_level > 0 ? (
          <Text className="tag tag--member">{LEVEL_LABEL[card.member_level] ?? '会员'}</Text>
        ) : null}
      </View>
      {card.reason && card.reason.length > 0 ? (
        <View className="content-card__reasons">
          {card.reason.map((r) => (
            <Text key={r.dimension} className="content-card__reason">
              {r.text}
            </Text>
          ))}
        </View>
      ) : null}
    </View>
  )
}
