import { useEffect, useState } from 'react'
import { View, Text } from '@tarojs/components'
import type { DictItem } from '@catcube/api-client'
import { api } from '../services/request'

export interface TagPickerProps {
  label: string
  dimension: string
  value: string[]
  multiple?: boolean
  parentCode?: string
  onChange: (codes: string[]) => void
}

// 标签/字典选择器（行业/区域/合作需求/合作供给）；经 dict 端口拉取，单选时覆盖。
export default function TagPicker({ label, dimension, value, multiple = true, parentCode, onChange }: TagPickerProps) {
  const [items, setItems] = useState<DictItem[]>([])

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

  const toggle = (code: string) => {
    if (!multiple) {
      onChange([code])
      return
    }
    onChange(value.includes(code) ? value.filter((c) => c !== code) : [...value, code])
  }

  return (
    <View className="form-field">
      <Text className="form-field__label">{label}</Text>
      <View className="tabs">
        {items.map((it) => (
          <Text
            key={it.code}
            className={value.includes(it.code) ? 'tabs__item tabs__item--active' : 'tabs__item'}
            onClick={() => toggle(it.code)}
          >
            {it.name}
          </Text>
        ))}
      </View>
    </View>
  )
}
