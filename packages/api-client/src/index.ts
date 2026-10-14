// 本包 schema.d.ts 由 contracts/openapi/*.yaml 经 `make gen` 生成（openapi-typescript），勿手改。
// 本文件为基于生成类型的薄客户端（信封解包、游标分页、Idempotency-Key）。
import type { paths, components } from './schema'

export type { paths, components }

/** 统一响应信封（与 contracts/openapi 约定一致）。 */
export interface Envelope<T = unknown> {
  code: number
  message: string
  request_id: string
  data?: T | null
}

/** 游标分页响应。 */
export interface CursorPage<T> {
  items: T[]
  next_cursor: string | null
}

export class ApiError extends Error {
  constructor(
    public code: number,
    message: string,
    public requestId: string,
  ) {
    super(message)
  }
}

export interface ClientOptions {
  baseUrl: string // 如 /api/mp/v1
  getToken?: () => string | null
  fetchImpl?: typeof fetch
}

type Op = keyof paths

/** 极简类型化客户端：按 path 取生成类型，信封 code!==0 抛 ApiError。 */
export function createClient(opts: ClientOptions) {
  const f = opts.fetchImpl ?? fetch
  async function call<T>(
    method: string,
    path: string,
    body?: unknown,
    idempotencyKey?: string,
  ): Promise<T> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    const token = opts.getToken?.()
    if (token) headers['Authorization'] = `Bearer ${token}`
    if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey
    const res = await f(opts.baseUrl + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const env = (await res.json()) as Envelope<T>
    if (env.code !== 0) throw new ApiError(env.code, env.message, env.request_id)
    return env.data as T
  }
  return {
    /** 微信登录。 */
    wechatLogin(body: { code: string; encrypted_data?: string; iv?: string }, idem: string) {
      return call<
        components['schemas']['Envelope'] & {
          data?: { user_id?: string; new?: boolean; tokens?: components['schemas']['Tokens'] }
        }
      >('POST', '/auth/wechat' satisfies Op, body, idem)
    },
    /** 查询字典维度。 */
    listDict(dimension: string, parentCode?: string) {
      const q = parentCode ? `?parent_code=${encodeURIComponent(parentCode)}` : ''
      return call<{ items?: components['schemas']['DictItem'][] }>(
        'GET',
        `/dict/${dimension}${q}`,
      )
    },
  }
}
