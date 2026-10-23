import { useCallback, useEffect, useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import type { Circle, CircleDiscoverItem } from '@catcube/api-client'
import { api } from '../services/request'
import { ensureLogin } from '../services/auth'
import StateView from './StateView'

const APPLY_LABEL: Record<number, string> = { 1: '审核中', 2: '已通过', 3: '未通过' }

interface Props {
  refreshKey?: number
}

// 广场-圈子频道：发现圈子 + 我的圈子 + 创建/邀请码加入。
export default function CircleChannel({ refreshKey = 0 }: Props) {
  const [tab, setTab] = useState<'discover' | 'mine'>('discover')
  const [discover, setDiscover] = useState<CircleDiscoverItem[]>([])
  const [circles, setCircles] = useState<Circle[]>([])
  const [q, setQ] = useState('')
  const [code, setCode] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const loadMine = useCallback(async () => {
    const res = await api.listCircles()
    setCircles(res.items ?? [])
  }, [])

  const loadDiscover = useCallback(async (keyword = '') => {
    const res = await api.discoverCircles({ q: keyword.trim() || undefined, limit: 50 })
    setDiscover(res.items ?? [])
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      await Promise.all([loadDiscover(''), loadMine()])
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }, [loadDiscover, loadMine])

  useEffect(() => {
    void load()
  }, [load, refreshKey])

  const join = async () => {
    if (!code.trim()) return
    try {
      await ensureLogin()
      const c = await api.joinCircle(code.trim())
      setCode('')
      Taro.navigateTo({ url: `/pages/circle/detail?id=${c.id}` })
    } catch {
      Taro.showToast({ title: '邀请链接无效或已失效', icon: 'none' })
    }
  }

  const active = circles.filter((c) => c.status === 0)
  const ended = circles.filter((c) => c.status !== 0)

  const section = (title: string, list: Circle[]) =>
    list.length > 0 ? (
      <View className="detail-block">
        <Text className="detail-block__title">{title}</Text>
        {list.map((c) => (
          <View key={c.id} className="menu__item menu__row" onClick={() => Taro.navigateTo({ url: `/pages/circle/detail?id=${c.id}` })}>
            <Text>{c.name}</Text>
            <Text className="row-tag">
              {c.type === 1 ? '常设' : '临时'} · {c.member_count} 人 · {c.status === 0 ? '进行中' : c.status === 1 ? '已结束' : '已解散'}
            </Text>
          </View>
        ))}
      </View>
    ) : null

  return (
    <View>
      <View className="tabs">
        <Text className={tab === 'discover' ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setTab('discover')}>
          发现圈子
        </Text>
        <Text className={tab === 'mine' ? 'tabs__item tabs__item--active' : 'tabs__item'} onClick={() => setTab('mine')}>
          我的圈子
        </Text>
      </View>

      <View className="search-row">
        <Button className="btn-primary" size="mini" onClick={() => Taro.navigateTo({ url: '/pages/circle/create' })}>
          创建圈子
        </Button>
        <Input className="search-row__input" value={code} placeholder="输入邀请码加入" onInput={(e) => setCode(e.detail.value)} />
        <Button className="btn-secondary" size="mini" onClick={() => void join()}>
          加入
        </Button>
      </View>

      {tab === 'discover' ? (
        <>
          <View className="search-row">
            <Input
              className="search-row__input"
              value={q}
              placeholder="搜索圈名 / 简介"
              confirmType="search"
              onInput={(e) => setQ(e.detail.value)}
              onConfirm={() => void loadDiscover(q)}
            />
            <Button className="search-row__btn" size="mini" onClick={() => void loadDiscover(q)}>
              搜索
            </Button>
          </View>
          <StateView
            loading={loading}
            error={error}
            empty={discover.length === 0}
            emptyText="还没有可发现的圈子，创建一个吧"
            onRetry={() => void load()}
          >
            <View className="menu">
              {discover.map((c) => (
                <View key={c.id} className="menu__item" onClick={() => Taro.navigateTo({ url: `/pages/circle/detail?id=${c.id}` })}>
                  <View className="menu__row">
                    <Text>{c.name}</Text>
                    <Text className="row-tag">
                      {c.joined ? '已加入' : APPLY_LABEL[c.apply_status ?? 0] ?? '查看'}
                    </Text>
                  </View>
                  <Text className="form-field__label">
                    {c.intro || '暂无简介'} · {c.member_count} 人{c.owner_name ? ` · 圈主 ${c.owner_name}` : ''}
                  </Text>
                </View>
              ))}
            </View>
          </StateView>
        </>
      ) : (
        <StateView loading={loading} error={error} empty={circles.length === 0} emptyText="还没有圈子，创建一个或输入邀请码加入" onRetry={() => void load()}>
          {section('进行中', active)}
          {section('已结束 / 已解散', ended)}
        </StateView>
      )}
    </View>
  )
}
