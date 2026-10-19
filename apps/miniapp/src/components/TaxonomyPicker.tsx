import { useEffect, useMemo, useState } from 'react'
import { View, Text, Input } from '@tarojs/components'
import type { DictItem } from '@catcube/api-client'
import { api } from '../services/request'

export interface TaxonomyPickerProps {
  label: string
  dimension: string
  value: string[]
  multiple?: boolean
  max?: number
  parentCode?: string
  searchable?: boolean
  onChange: (codes: string[]) => void
}

// 字典/标签选择器（行业/区域/合作需求/合作供给）：支持多选上限、父级联动、可搜索；
// 经 dict 端口按 dimension 拉取。单选时点选即覆盖。
export default function TaxonomyPicker({
  label,
  dimension,
  value,
  multiple = true,
  max,
  parentCode,
  searchable = false,
  onChange,
}: TaxonomyPickerProps) {
  const [items, setItems] = useState<DictItem[]>([])
  const [keyword, setKeyword] = useState('')

  useEffect(() => {
    void (async () => {
      try {
        const res = await api.listDict(dimension, parentCode)
        setItems(res.items ?? [])
      } catch {
        setItems([])
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dimension, parentCode])

  const needsSearch = searchable || dimension === 'region' || dimension === 'industry'
  const filtered = useMemo(() => {
    const kw = keyword.trim()
    return kw ? items.filter((it) => it.name.includes(kw) || it.code.includes(kw)) : items
  }, [items, keyword])
  const shown = useMemo(() => filtered.slice(0, 80), [filtered])
  const hidden = filtered.length - shown.length

  const toggle = (code: string) => {
    if (!multiple) {
      onChange([code])
      return
    }
    if (value.includes(code)) {
      onChange(value.filter((c) => c !== code))
      return
    }
    if (max && value.length >= max) return
    onChange([...value, code])
  }

  return (
    <View className="form-field">
      <Text className="form-field__label">
        {label}
        {max ? `（最多 ${max}）` : ''}
      </Text>
      {needsSearch ? (
        <Input
          className="search-row__input"
          value={keyword}
          placeholder="搜索"
          onInput={(e) => setKeyword(e.detail.value)}
        />
      ) : null}
      <View className="tabs">
        {shown.map((it) => (
          <Text
            key={it.code}
            className={value.includes(it.code) ? 'tabs__item tabs__item--active' : 'tabs__item'}
            onClick={() => toggle(it.code)}
          >
            {it.name}
          </Text>
        ))}
      </View>
      {hidden > 0 ? (
        <Text style={{ color: '#888', fontSize: '24rpx', marginTop: '12rpx', display: 'block' }}>
          共 {filtered.length} 条，已显示前 {shown.length} 条，请输入关键词搜索
        </Text>
      ) : null}
    </View>
  )
}
