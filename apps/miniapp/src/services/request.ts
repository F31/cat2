import Taro from '@tarojs/taro'
import { createClient } from '@catcube/api-client'
import { getToken } from '../utils/token'

// 后端 MP 网关（dev；生产由构建注入）。
const BASE_URL = 'http://127.0.0.1:8080/api/mp/v1'

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
