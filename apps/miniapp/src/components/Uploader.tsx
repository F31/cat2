import { useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { api } from '../services/request'
import { ensureLogin } from '../services/auth'

export interface UploaderProps {
  label: string
  value: string[]
  max?: number
  onChange: (fileIds: string[]) => void
}

const MAX_SIZE = 5 * 1024 * 1024

function contentTypeOf(path: string): string | null {
  const lower = path.toLowerCase()
  if (lower.endsWith('.jpg') || lower.endsWith('.jpeg')) return 'image/jpeg'
  if (lower.endsWith('.png')) return 'image/png'
  return null
}

// 图片上传：申请预签名 → 上传 → 回填 file_id（JPG/PNG ≤5MB）。
export default function Uploader({ label, value, max = 1, onChange }: UploaderProps) {
  const [busy, setBusy] = useState(false)

  const pick = async () => {
    if (value.length >= max) {
      Taro.showToast({ title: `最多 ${max} 张`, icon: 'none' })
      return
    }
    setBusy(true)
    try {
      await ensureLogin()
      const chosen = await Taro.chooseImage({ count: max - value.length })
      const ids = [...value]
      for (const file of chosen.tempFiles ?? []) {
        const path = file.path
        const contentType = contentTypeOf(path) ?? file.type
        if (contentType !== 'image/jpeg' && contentType !== 'image/png') {
          Taro.showToast({ title: '仅支持 JPG/PNG', icon: 'none' })
          continue
        }
        if (file.size > MAX_SIZE) {
          Taro.showToast({ title: '图片需 ≤5MB', icon: 'none' })
          continue
        }
        const filename = path.split('/').pop() ?? 'upload.jpg'
        const ticket = await api.createUpload(filename, contentType, file.size)
        await Taro.uploadFile({ url: ticket.upload_url, filePath: path, name: 'file' })
        ids.push(ticket.file_id)
      }
      onChange(ids)
    } catch {
      Taro.showToast({ title: '上传失败，请重试', icon: 'none' })
    } finally {
      setBusy(false)
    }
  }

  const remove = (id: string) => onChange(value.filter((v) => v !== id))

  return (
    <View className="form-field">
      <Text className="form-field__label">{label}</Text>
      <View className="uploader">
        {value.map((id) => (
          <View key={id} className="uploader__item" onClick={() => remove(id)}>
            <Text className="uploader__hint">已上传</Text>
          </View>
        ))}
        {value.length < max ? (
          <View className="uploader__add" onClick={() => void pick()}>
            <Text className="uploader__plus">{busy ? '…' : '+'}</Text>
          </View>
        ) : null}
      </View>
    </View>
  )
}
