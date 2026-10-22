import { useEffect, useState } from 'react'
import { Image, Tag } from 'antd'
import { adminApi } from '../api/admin'

// 后台资质/材料缩略图：按 file_id 换取签名 URL。
export default function MediaThumb({ id }: { id: number }) {
  const [url, setUrl] = useState('')
  useEffect(() => {
    void adminApi
      .adminMediaObjectURL(String(id))
      .then((u) => setUrl(u))
      .catch(() => setUrl(''))
  }, [id])
  if (!url) return <Tag>#{id}</Tag>
  return <Image src={url} width={56} height={40} style={{ objectFit: 'cover', borderRadius: 4 }} />
}
