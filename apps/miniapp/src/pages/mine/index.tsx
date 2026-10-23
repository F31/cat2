import { useState } from 'react'
import Taro, { useDidShow, useShareAppMessage } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { Card, Completion, Member } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

const LEVEL_LABEL: Record<number, string> = { 1: 'VIP 会员', 2: 'SVIP 会员', 3: '企业版' }

// 我的 Tab：名片摘要 + 会员状态 + 完善资料 + 收藏/发布/关注入口。
export default function Mine() {
  const [card, setCard] = useState<Card | null>(null)
  const [member, setMember] = useState<{ member: Member; active: boolean } | null>(null)
  const [completion, setCompletion] = useState<Completion | null>(null)
  const [userId, setUserId] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      await ensureLogin()
      const [c, m, comp, me] = await Promise.all([api.getMyCard(), api.myMembership(), api.getCompletion(), api.me()])
      setCard(c)
      setMember(m)
      setCompletion(comp)
      setUserId(me.user_id)
      setError(false)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
    void Taro.showShareMenu({ withShareTicket: true }).catch(() => undefined)
  })

  useShareAppMessage(() => ({
    title: card?.name ? `${card.name}的猫平方名片` : '猫平方名片',
    path: userId ? `/pages/content/detail?ref=${encodeURIComponent(`user:${userId}`)}` : '/pages/discover/index',
  }))

  const memberActive = member?.active ?? false

  const openCircleChannel = () => {
    Taro.setStorageSync('catcube.discover.channel', 'circle')
    Taro.switchTab({ url: '/pages/discover/index' })
  }

  return (
    <View className="page">
      <StateView loading={loading} error={error} onRetry={() => void load()}>
        <View className="profile-head">
          <Text className="profile-head__name">{card?.name || '未设置名称'}</Text>
          <Text className="profile-head__sub">
            {card?.region_code ? card.region_code : '未设置区域'}
            {completion ? ` · 资料完成度 ${completion.tier}/3` : ''}
          </Text>
          <Button className="btn-secondary" size="mini" openType="share" style={{ marginTop: '16px' }}>
            分享我的名片
          </Button>
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

        <Text className="menu-section__title">账号与认证</Text>
        <View className="menu">
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/card/edit' })}>
            <Text>完善资料 / 编辑名片</Text>
          </View>
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/verify/index' })}>
            <Text>实名认证</Text>
          </View>
          {memberActive && member?.member.level === 3 ? (
            <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/subaccounts/index' })}>
              <Text>子账号管理</Text>
            </View>
          ) : null}
          <View className="menu__item" onClick={() => openCircleChannel()}>
            <Text>我的圈子</Text>
          </View>
          <View className="menu__item" onClick={() => Taro.switchTab({ url: '/pages/home-cube/index' })}>
            <Text>我的猫立方</Text>
          </View>
        </View>

        <Text className="menu-section__title">内容与社交</Text>
        <View className="menu">
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/mine/activities' })}>
            <Text>我的活动</Text>
          </View>
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/content/mine' })}>
            <Text>我的发布</Text>
          </View>
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/circle/mounts' })}>
            <Text>挂载申请</Text>
          </View>
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/social/favorites' })}>
            <Text>我的收藏</Text>
          </View>
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/social/follows' })}>
            <Text>关注与粉丝</Text>
          </View>
        </View>

        <Text className="menu-section__title">服务与设置</Text>
        <View className="menu">
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/msg/notices' })}>
            <Text>系统通知</Text>
          </View>
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/support/index' })}>
            <Text>客服</Text>
          </View>
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/settings/index' })}>
            <Text>设置</Text>
          </View>
          <View className="menu__item" onClick={() => Taro.navigateTo({ url: '/pages/channel/index' })}>
            <Text>运营商中心</Text>
          </View>
        </View>
      </StateView>
    </View>
  )
}
