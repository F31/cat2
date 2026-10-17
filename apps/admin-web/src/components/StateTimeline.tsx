import { Steps } from 'antd'

export interface StateStep {
  key: string
  label: string
}

/** 状态流转时间线（只读展示；用于结算/订单等单据进度）。 */
export default function StateTimeline({ steps, current }: { steps: StateStep[]; current: string }) {
  const idx = Math.max(0, steps.findIndex((s) => s.key === current))
  return <Steps size="small" current={idx} items={steps.map((s) => ({ title: s.label }))} />
}
