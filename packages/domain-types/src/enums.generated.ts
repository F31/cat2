// 本文件由 contracts/enums/*.json 生成（make gen），请勿手写。

/** CircleStatus（circle）：圈子状态 */
export const CircleStatus = {
  Active: 0, // 进行中
  Ended: 1, // 已结束
  Dissolved: 2, // 已解散
} as const
export type CircleStatus = (typeof CircleStatus)[keyof typeof CircleStatus]
export const CircleStatusLabel: Record<number, string> = {
  [CircleStatus.Active]: '进行中',
  [CircleStatus.Ended]: '已结束',
  [CircleStatus.Dissolved]: '已解散',
}

/** CircleType（circle）：圈子类型 */
export const CircleType = {
  Permanent: 1, // 常设
  Temporary: 2, // 临时
} as const
export type CircleType = (typeof CircleType)[keyof typeof CircleType]
export const CircleTypeLabel: Record<number, string> = {
  [CircleType.Permanent]: '常设',
  [CircleType.Temporary]: '临时',
}

/** ContentStatus（content）：内容状态 */
export const ContentStatus = {
  Reviewing: 0, // 审核中
  Published: 1, // 展示中
  TakenDown: 2, // 已下架
} as const
export type ContentStatus = (typeof ContentStatus)[keyof typeof ContentStatus]
export const ContentStatusLabel: Record<number, string> = {
  [ContentStatus.Reviewing]: '审核中',
  [ContentStatus.Published]: '展示中',
  [ContentStatus.TakenDown]: '已下架',
}

/** CubeMemberRole（cube）：立方成员角色 */
export const CubeMemberRole = {
  None: 0, // 非成员
  Leader: 1, // 猫王
  Honey: 2, // 猫蜜
  Member: 3, // 普通成员
  CoBuild: 4, // 共建成员
} as const
export type CubeMemberRole = (typeof CubeMemberRole)[keyof typeof CubeMemberRole]
export const CubeMemberRoleLabel: Record<number, string> = {
  [CubeMemberRole.None]: '非成员',
  [CubeMemberRole.Leader]: '猫王',
  [CubeMemberRole.Honey]: '猫蜜',
  [CubeMemberRole.Member]: '普通成员',
  [CubeMemberRole.CoBuild]: '共建成员',
}

/** CubeStatus（cube）：猫立方状态 */
export const CubeStatus = {
  Active: 0, // 正常
  Banned: 1, // 封禁
  Dissolved: 2, // 已解散
} as const
export type CubeStatus = (typeof CubeStatus)[keyof typeof CubeStatus]
export const CubeStatusLabel: Record<number, string> = {
  [CubeStatus.Active]: '正常',
  [CubeStatus.Banned]: '封禁',
  [CubeStatus.Dissolved]: '已解散',
}

/** MemberLevel（membership）：会员档位 */
export const MemberLevel = {
  VIP: 1, // VIP
  SVIP: 2, // SVIP（预留不上架）
  Enterprise: 3, // 企业版
} as const
export type MemberLevel = (typeof MemberLevel)[keyof typeof MemberLevel]
export const MemberLevelLabel: Record<number, string> = {
  [MemberLevel.VIP]: 'VIP',
  [MemberLevel.SVIP]: 'SVIP（预留不上架）',
  [MemberLevel.Enterprise]: '企业版',
}

/** ModerationResult（trust）：内容机检结论 */
export const ModerationResult = {
  Pending: 0, // 审核中
  Passed: 1, // 通过
  Rejected: 2, // 驳回
  Retry: 3, // 机检超时待重试
} as const
export type ModerationResult = (typeof ModerationResult)[keyof typeof ModerationResult]
export const ModerationResultLabel: Record<number, string> = {
  [ModerationResult.Pending]: '审核中',
  [ModerationResult.Passed]: '通过',
  [ModerationResult.Rejected]: '驳回',
  [ModerationResult.Retry]: '机检超时待重试',
}

/** OrderStatus（trade）：订单状态 */
export const OrderStatus = {
  Pending: 0, // 待支付
  Paid: 1, // 已支付
  Closed: 2, // 已关闭
} as const
export type OrderStatus = (typeof OrderStatus)[keyof typeof OrderStatus]
export const OrderStatusLabel: Record<number, string> = {
  [OrderStatus.Pending]: '待支付',
  [OrderStatus.Paid]: '已支付',
  [OrderStatus.Closed]: '已关闭',
}

/** PlanLevel（cube）：订阅档位 */
export const PlanLevel = {
  Basic: 1, // 基础
  Plus: 2, // 进阶
  Premium: 3, // 高阶
} as const
export type PlanLevel = (typeof PlanLevel)[keyof typeof PlanLevel]
export const PlanLevelLabel: Record<number, string> = {
  [PlanLevel.Basic]: '基础',
  [PlanLevel.Plus]: '进阶',
  [PlanLevel.Premium]: '高阶',
}

/** UserStatus（identity）：账号状态 */
export const UserStatus = {
  Active: 0, // 正常
  Limited: 1, // 限制
  Banned: 2, // 封禁
  Deactivated: 3, // 已注销
} as const
export type UserStatus = (typeof UserStatus)[keyof typeof UserStatus]
export const UserStatusLabel: Record<number, string> = {
  [UserStatus.Active]: '正常',
  [UserStatus.Limited]: '限制',
  [UserStatus.Banned]: '封禁',
  [UserStatus.Deactivated]: '已注销',
}
