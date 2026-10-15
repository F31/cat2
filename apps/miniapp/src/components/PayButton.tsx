import { useState } from 'react'
import Taro from '@tarojs/taro'
import { Button } from '@tarojs/components'
import { api } from '../services/request'
import { ensureLogin } from '../services/auth'

export interface PayButtonProps {
  label?: string
  disabled?: boolean
  className?: string
  // createOrder 返回订单号（内部完成下单）；已存在的订单可直接传 orderId。
  createOrder?: () => Promise<string>
  orderId?: string
  onPaid?: (orderId: string) => void
  onError?: (e: unknown) => void
}

// 支付按钮：下单 → 取支付参数 → 调起微信支付（dev 无签名参数时仅确认订单已创建）。
export default function PayButton({
  label = '立即支付',
  disabled,
  className = 'btn-primary',
  createOrder,
  orderId,
  onPaid,
  onError,
}: PayButtonProps) {
  const [paying, setPaying] = useState(false)

  const pay = async () => {
    setPaying(true)
    try {
      await ensureLogin()
      const id = orderId ?? (createOrder ? await createOrder() : '')
      if (!id) throw new Error('no order')
      const params = await api.payOrder(id)
      if (params.paySign) {
        await Taro.requestPayment({
          timeStamp: params.timeStamp ?? '',
          nonceStr: params.nonceStr ?? '',
          package: params.package ?? '',
          signType: (params.signType ?? 'RSA') as 'MD5' | 'HMAC-SHA256' | 'RSA',
          paySign: params.paySign,
        })
      } else {
        // dev：无微信支付签名，订单已创建（回调由后端支付通道触发）。
        Taro.showToast({ title: `订单 ${params.out_trade_no} 已创建`, icon: 'none' })
      }
      onPaid?.(id)
    } catch (e) {
      if (onError) {
        onError(e)
      } else {
        const msg = e instanceof Error && /cancel/i.test(e.message) ? '已取消支付' : '支付失败，请重试'
        Taro.showToast({ title: msg, icon: 'none' })
      }
    } finally {
      setPaying(false)
    }
  }

  return (
    <Button className={className} loading={paying} disabled={disabled} onClick={() => void pay()}>
      {label}
    </Button>
  )
}
