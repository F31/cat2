import { useEffect, useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import { aiFeatures, isAiUnavailable } from '../services/ai'

// AI 入口通用组件（p5 §7.1）：嵌入入驻页/发布页/搜索页。
// - 开关关闭或 ai 未装载时自动隐藏（不影响手动填写/搜索）；
// - 统一处理空输入校验、加载态与「不可用/失败」提示，业务回调仅负责成功后的预填。
export interface AiEntryProps {
  /** 特性开关名（ai.onboard / ai.publish / ai.search）。 */
  flag: string
  hint: string
  buttonLabel: string
  busyLabel?: string
  /** 提供则渲染输入框（一句话描述）；省略则仅按钮（如搜索页复用关键词输入）。 */
  placeholder?: string
  /** 点击执行；抛错时组件按「不可用/失败」提示。 */
  run: (text: string) => Promise<void> | void
  /** 自定义校验：返回非空字符串则提示并中断（默认校验输入框非空）。 */
  validate?: (text: string) => string | null
  unavailableHint?: string
  failHint?: string
}

export default function AiEntry(props: AiEntryProps) {
  const [on, setOn] = useState(false)
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let alive = true
    void aiFeatures().then((f) => {
      if (alive) setOn(!!f[props.flag])
    })
    return () => {
      alive = false
    }
  }, [props.flag])

  if (!on) return null

  const go = async () => {
    const invalid = props.validate
      ? props.validate(text)
      : props.placeholder && !text.trim()
        ? '请先输入一句话描述'
        : null
    if (invalid) {
      Taro.showToast({ title: invalid, icon: 'none' })
      return
    }
    setBusy(true)
    try {
      await props.run(text)
    } catch (e) {
      Taro.showToast({
        title: isAiUnavailable(e)
          ? (props.unavailableHint ?? 'AI 暂不可用，请手动填写')
          : (props.failHint ?? '生成失败，请手动填写'),
        icon: 'none',
      })
    } finally {
      setBusy(false)
    }
  }

  return (
    <View className="ai-entry">
      <Text className="ai-entry__hint">{props.hint}</Text>
      {props.placeholder ? (
        <Input
          className="search-row__input"
          value={text}
          placeholder={props.placeholder}
          onInput={(e) => setText(e.detail.value)}
        />
      ) : null}
      <Button className="btn-secondary" size="mini" disabled={busy} onClick={() => void go()}>
        {busy ? (props.busyLabel ?? '生成中…') : props.buttonLabel}
      </Button>
    </View>
  )
}
