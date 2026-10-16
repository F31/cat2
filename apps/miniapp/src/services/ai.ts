import { api } from './request'

// AI 能力门禁（p5 框架）：默认关闭；关闭或上游不可用（030001）时一律回退原表单/关键词搜索。
const FLAG_ONBOARD = 'ai.onboard'
const FLAG_PUBLISH = 'ai.publish'
const FLAG_SEARCH = 'ai.search'

let cache: Record<string, boolean> | null = null

/** 拉取能力开关（按当前用户灰度评估），失败按全关处理。 */
export async function aiFeatures(force = false): Promise<Record<string, boolean>> {
  if (cache && !force) return cache
  try {
    const res = await api.aiFeatures()
    cache = res.features ?? {}
  } catch {
    cache = {}
  }
  return cache
}

export const AI_ONBOARD = FLAG_ONBOARD
export const AI_PUBLISH = FLAG_PUBLISH
export const AI_SEARCH = FLAG_SEARCH

/** 是否 AI 不可用错误（030001：开关关闭/上游失败）。 */
export function isAiUnavailable(e: unknown): boolean {
  return !!e && typeof e === 'object' && 'code' in e && (e as { code: number }).code === 30001
}

/** 生成草稿（发布/入驻共用）；不可用时抛错，调用方回退原表单。 */
export async function draftPublish(kind: string, text: string) {
  return api.aiPublishDraft(kind, text)
}

export async function draftOnboard(text: string) {
  return api.aiOnboardDraft(text)
}

/** 自然语言 → 结构化检索；不可用时抛错，调用方按关键词搜索。 */
export async function parseSearch(text: string) {
  return api.aiSearch(text)
}
