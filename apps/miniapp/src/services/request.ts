import Taro from '@tarojs/taro'
import { createClient } from '@catcube/api-client'
import { getToken } from '../utils/token'

declare const process: { env: { NODE_ENV?: string } }

// 后端 MP 网关：开发直连本机，生产走域名 HTTPS（微信服务器域名白名单需同步配置）。
export const BASE_URL =
  process.env.NODE_ENV === 'development'
    ? 'http://127.0.0.1:8080/api/mp/v1'
    : 'https://cat2.mindbase.fun/api/mp/v1'

interface FetchInit {
  method?: string
  headers?: Record<string, string>
  body?: string
}

// fetch 适配器：小程序无 fetch，桥接 Taro.request。
async function taroFetch(input: string, init?: FetchInit): Promise<{ json: () => Promise<unknown> }> {
  const res = await Taro.request({
    url: input,
    method: (init?.method ?? 'GET') as never,
    header: init?.headers,
    data: init?.body ? JSON.parse(init.body) : undefined,
  })
  return { json: async () => res.data }
}

export const api = createClient({
  baseUrl: BASE_URL,
  getToken,
  fetchImpl: taroFetch as unknown as typeof fetch,
})


// 小程序统一图片上传：走服务端 multipart 落库（未配置 OSS 时也可用）。
export async function uploadMedia(filePath: string): Promise<string> {
  const token = getToken()
  const res = await Taro.uploadFile({
    url: `${BASE_URL}/media/upload`,
    filePath,
    name: 'file',
    header: token ? { Authorization: `Bearer ${token}` } : {},
  })
  const env = JSON.parse(res.data) as { code: number; message: string; data?: { file_id: string } }
  if (env.code !== 0 || !env.data?.file_id) throw new Error(env.message || '上传失败')
  return env.data.file_id
}
