// 领域类型首版：与后端 contract 包一一对应。后续由契约生成管线产出，勿手写扩展。

export * from './events'
export * from './enums.generated'

// 注意：状态枚举由 contracts/enums/*.json 生成（make gen），不得在此手写。

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

