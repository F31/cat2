import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { View, Button } from '@tarojs/components'
import StateView from './StateView'

export interface CursorListProps<T> {
  load: (cursor: string | null) => Promise<{ items: T[]; next_cursor: string | null }>
  renderItem: (item: T) => ReactNode
  keyOf: (item: T) => string
  emptyText?: string
  refreshKey?: unknown
}

// 游标分页列表（列表一律游标分页：cursor='' 首页，next_cursor=null 到底）；
// 统一加载/空/错误态与「加载更多」。
export default function CursorList<T>({ load, renderItem, keyOf, emptyText, refreshKey }: CursorListProps<T>) {
  const [items, setItems] = useState<T[]>([])
  const [cursor, setCursor] = useState<string | null>(null)
  const [hasMore, setHasMore] = useState(true)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const fetchPage = useCallback(
    async (next: string | null, append: boolean) => {
      setLoading(true)
      try {
        const res = await load(next)
        setItems((prev) => (append ? [...prev, ...res.items] : res.items))
        setCursor(res.next_cursor)
        setHasMore(!!res.next_cursor)
        setError(false)
      } catch {
        setError(true)
      } finally {
        setLoading(false)
      }
    },
    [load],
  )

  useEffect(() => {
    void fetchPage(null, false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshKey])

  return (
    <>
      <StateView
        loading={loading && items.length === 0}
        error={error && items.length === 0}
        empty={!error && items.length === 0}
        emptyText={emptyText}
        onRetry={() => void fetchPage(null, false)}
      >
        <View>
          {items.map((it) => (
            <View key={keyOf(it)}>{renderItem(it)}</View>
          ))}
        </View>
      </StateView>
      {hasMore && items.length > 0 ? (
        <Button className="btn-more" loading={loading} onClick={() => void fetchPage(cursor, true)}>
          加载更多
        </Button>
      ) : null}
    </>
  )
}
