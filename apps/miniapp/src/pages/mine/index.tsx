import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import type { Card, Completion, Member } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'

const LEVEL_LABEL: Record<number, string> = { 1: 'VIP 会员', 2: 'SVIP 会员', 3: '企业版' }

// 我的 Tab：名片摘要 + 会员状态 + 完善资料 + 收藏/发布/关注入口。
export default function Mine() {
  const [card, setCard] = useState<Card | null>(null)
  const [member, setMember] = useState<{ member: Member; active: boolean } | null>(null)
  const [completion, setCompletion] = useState<Completion | null>(null)

  const load = async () => {
    try {
      await ensureLogin()
      const [c, m, comp] = await Promise.all([api.getMyCard(), api.myMembership(), api.getCompletion()])
      setCard(c)
      setMember(m)
      setCompletion(comp)
    } catch {
      Taro.showToast({ title: '加载失败', icon: 'none' })
    }
  }

  useDidShow(() => {
    void load()
  })

  const memberActive = member?.active ?? false

  const placeholder = (title: string) => Taro.showToast({ title: `${title}页后续接入`, icon: 'none' })

  return (
    <View className="page">
      <View className="profile-head">
        <Text className="profile-head__name">{card?.name || '未设置名称'}</Text>
        <Text className="profile-head__sub">
          {card?.region_code ? card.region_code : '未设置区域'}
          {completion ? ` · 资料完成度 ${completion.tier}/3` : ''}
        </Text>
      </View>

      <View className="member-card">
        <View className="member-card__info">
          <Text className="member-card__title">
            {memberActive ? LEVEL_LABEL[member!.member.level] ?? '会员' : '普通用户'}
          </Text>
          {memberActive ? (
            <Text className="member-card__sub">有效期至 {member!.member.expires_at.slice(0, 10)}</Text>
          ) : (
            <Text className="member-card__sub">开通会员，查看联系方式与核心优势</Text>
          )}
        </View>
        <Text
          className="member-card__action"
          onClick={() => Taro.navigateTo({ url: '/pages/member/open' })}
        >
          {memberActive ? '续费' : '去开通'}
        </Text>
      </View>

      <View className="menu">
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/card/edit' })}>
          <Text>完善资料 / 编辑名片</Text>
        </View>
        <View className="menu__item" onClick={() => placeholder('我的收藏')}>
          <Text>我的收藏</Text>
        </View>
        <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/content/mine' })}>
          <Text>我的发布</Text>
        </View>
        <View className="menu__item" onClick={() => placeholder('关注与粉丝')}>
          <Text>关注与粉丝</Text>
        </View>
      </View>
    </View>
  )
}
