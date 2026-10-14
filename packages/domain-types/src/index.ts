// 领域类型首版：与后端 contract 包一一对应。后续由契约生成管线产出，勿手写扩展。

export * from './events'

/** EntityRef 实体引用：跨模块一律引用不复制。 */
export interface EntityRef {
  type: string
  id: string // int64 字符串
}

/** TagSet 通用标签（行业/区域/性质/需求供给四维）。 */
export interface TagSet {
  industries?: string[]
  regions?: string[]
  natures?: string[]
  demands?: string[]
  supplies?: string[]
}

/** Badge 统一认证标识（新增标识类型不改代码）。 */
export interface Badge {
  type: string
  label: string
  level: number
}

/** 账号状态（identity）。 */
export const UserStatus = {
  Active: 0,
  Disabled: 1,
  Deactivated: 3,
} as const
export type UserStatus = (typeof UserStatus)[keyof typeof UserStatus]

/** 审核结论（trust）。 */
export const ModerationResult = {
  Pending: 0,
  Passed: 1,
  Rejected: 2,
  Retry: 3,
} as const
export type ModerationResult = (typeof ModerationResult)[keyof typeof ModerationResult]
