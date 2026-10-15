import { useEffect, useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import type { MembershipQuote } from '@catcube/api-client'
import { fenToYuan } from '@catcube/utils'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import PayButton from '../../components/PayButton'

const YEARS = [1, 3, 5]
const LEVEL_LABEL: Record<number, string> = { 1: 'VIP 会员', 3: '企业版' }

// 会员开通页：年限选择 + 原价与折后价同显 + 协议 + 支付前明示「开通后不支持退款」。
export default function MemberOpen() {
  const [level, setLevel] = useState(1)
  const [years, setYears] = useState(1)
  const [quote, setQuote] = useState<MembershipQuote | null>(null)
  const [agreed, setAgreed] = useState(false)
  const [isOrg, setIsOrg] = useState(false)

  useEffect(() => {
    void (async () => {
      try {
        await ensureLogin()
        const card = await api.getMyCard()
        setIsOrg(card.identity_type === 2)
      } catch {
        /* 未登录/无名片时不阻断 */
      }
    })()
  }, [])

  useEffect(() => {
    void (async () => {
      try {
        setQuote(await api.membershipQuote(level, years))
      } catch {
        setQuote(null)
      }
    })()
  }, [level, years])

  return (
    <View className="page">
      {isOrg ? (
        <View className="level-row">
          {[1, 3].map((lv) => (
            <Text
              key={lv}
              className={lv === level ? 'chip chip--active' : 'chip'}
              onClick={() => setLevel(lv)}
            >
              {LEVEL_LABEL[lv]}
            </Text>
          ))}
        </View>
      ) : null}

      <View className="level-row">
        {YEARS.map((y) => (
          <Text key={y} className={y === years ? 'chip chip--active' : 'chip'} onClick={() => setYears(y)}>
            {y} 年
          </Text>
        ))}
      </View>

      {quote ? (
        <View className="price">
          <Text className="price__total">¥{fenToYuan(quote.total_fen)}</Text>
          {quote.discount_rate < 10000 ? (
            <Text className="price__original">原价 ¥{fenToYuan(quote.original_fen)}</Text>
          ) : null}
          <Text className="price__note">开通后不支持退款</Text>
        </View>
      ) : null}

      <View className="agreement" onClick={() => setAgreed(!agreed)}>
        <Text className={agreed ? 'checkbox checkbox--on' : 'checkbox'}>{agreed ? '✓' : ''}</Text>
        <Text className="agreement__text">我已阅读并同意《会员服务协议》</Text>
      </View>

      <PayButton
        label="立即开通"
        disabled={!agreed}
        createOrder={async () => (await api.openMembership(level, years)).order_id}
      />
      {!agreed ? <Text className="price__note">请先勾选并同意《会员服务协议》</Text> : null}
    </View>
  )
}
