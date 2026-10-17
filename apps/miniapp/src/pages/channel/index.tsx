import { useEffect, useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import { api } from '../../services/request'
import type { ChannelCommission, ChannelLink, ChannelOperator, ChannelOverview } from '@catcube/api-client'

const STATUS_LABEL: Record<number, string> = { 0: '申请中', 1: '已开通', 2: '已停用' }

function yuan(fen: number) {
  return (fen / 100).toFixed(2)
}

// 运营商中心（p4 §1.3）：身份卡、推广链接/邀请码、业绩概览、佣金明细、提现申请与入驻申请。
export default function ChannelCenter() {
  const [operator, setOperator] = useState<ChannelOperator | null>(null)
  const [loading, setLoading] = useState(true)
  const [region, setRegion] = useState('')
  const [name, setName] = useState('')
  const [applying, setApplying] = useState(false)
  const [overview, setOverview] = useState<ChannelOverview | null>(null)
  const [commissions, setCommissions] = useState<ChannelCommission[]>([])
  const [link, setLink] = useState<ChannelLink | null>(null)

  const load = async () => {
    setLoading(true)
    try {
      const res = await api.channelOperator()
      setOperator(res.is_operator ? res.operator : null)
      if (res.is_operator && res.operator.status === 1) {
        const [ov, cs, lk] = await Promise.all([
          api.channelOverview().catch(() => null),
          api.channelCommissions().catch(() => ({ items: [] })),
          api.channelLink().catch(() => null),
        ])
        setOverview(ov)
        setCommissions(cs.items ?? [])
        setLink(lk)
      } else {
        setOverview(null)
        setCommissions([])
        setLink(null)
      }
    } catch (e) {
      Taro.showToast({ title: e instanceof Error ? e.message : '加载失败', icon: 'none' })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const apply = async () => {
    if (!region.trim() || !name.trim()) {
      Taro.showToast({ title: '请填写区域与名称', icon: 'none' })
      return
    }
    setApplying(true)
    try {
      await api.channelApply(region.trim(), name.trim())
      Taro.showToast({ title: '已提交申请', icon: 'success' })
      setRegion('')
      setName('')
      void load()
    } catch (e) {
      Taro.showToast({ title: e instanceof Error ? e.message : '申请失败', icon: 'none' })
    } finally {
      setApplying(false)
    }
  }

  const copyLink = async () => {
    if (!link) return
    try {
      await Taro.setClipboardData({ data: link.code })
      Taro.showToast({ title: '邀请码已复制', icon: 'success' })
    } catch {
      Taro.showToast({ title: '复制失败', icon: 'none' })
    }
  }

  const withdraw = async () => {
    try {
      await api.channelWithdraw()
      Taro.showToast({ title: '提现申请已提交', icon: 'success' })
    } catch (e) {
      Taro.showToast({ title: e instanceof Error ? e.message : '提现失败', icon: 'none' })
    }
  }

  const showApplyForm = !operator || operator.status === 2
  const showCenter = operator?.status === 1

  return (
    <View className="page">
      {operator ? (
        <View className="support-head">
          <Text className="support-head__title">{operator.name}</Text>
          <Text className="support-head__sub">
            区域 {operator.region_name || operator.region_code} · {STATUS_LABEL[operator.status] ?? operator.status}
          </Text>
        </View>
      ) : (
        <View className="support-head">
          <Text className="support-head__title">成为运营商</Text>
          <Text className="support-head__sub">按区域授权（一地一家）；提交后由平台审核开通。</Text>
        </View>
      )}

      {showApplyForm ? (
        <View className="support-chat">
          <Text className="ai-entry__hint">{operator?.status === 2 ? '已停用，可重新申请：' : '入驻申请：'}</Text>
          <Input className="search-row__input" value={region} placeholder="区域代码（如 310000）" onInput={(e) => setRegion(e.detail.value)} />
          <Input className="search-row__input" value={name} placeholder="运营商名称" onInput={(e) => setName(e.detail.value)} />
          <Button className="btn-secondary" size="mini" disabled={applying} onClick={() => void apply()}>
            {applying ? '提交中…' : '提交申请'}
          </Button>
        </View>
      ) : null}

      {showCenter ? (
        <>
          <View className="support-chat">
            <Text className="ai-entry__hint">推广邀请码（分享给用户，注册后归属本运营商）</Text>
            <Text>{link ? link.code : loading ? '加载中…' : '—'}</Text>
            <View className="search-row">
              <Button className="btn-secondary" size="mini" onClick={() => void copyLink()}>
                复制邀请码
              </Button>
              <Button className="btn-secondary" size="mini" onClick={() => void withdraw()}>
                申请提现
              </Button>
            </View>
          </View>

          <View className="support-chat">
            <Text className="ai-entry__hint">业绩概览</Text>
            <Text>本月佣金：¥{overview ? yuan(overview.month_fen) : '0.00'}</Text>
            <Text>累计佣金：¥{overview ? yuan(overview.total_fen) : '0.00'}</Text>
            <Text>
              保底任务（{overview?.target.year ?? new Date().getFullYear()}）：付费会员{' '}
              {overview?.target.paid_member_count ?? 0}/50，落地项目 {overview?.target.landing_project_count ?? 0}/3
            </Text>
          </View>

          <View className="support-chat">
            <Text className="ai-entry__hint">佣金明细</Text>
            {commissions.length === 0 ? (
              <Text className="ai-entry__hint">暂无佣金记录</Text>
            ) : (
              commissions.map((c) => (
                <Text key={c.id}>
                  {c.month} · 订单 {c.order_id} · ¥{yuan(c.amount_fen)}（{c.ratio_bp / 100}%）
                </Text>
              ))
            )}
          </View>
        </>
      ) : null}
    </View>
  )
}
