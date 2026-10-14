// 通用工具：金额一律分（int64），前端仅在此换算为展示用元。

/** 分 → 元字符串（保留两位小数）。 */
export function fenToYuan(fen: number | string): string {
  const n = typeof fen === 'string' ? parseInt(fen, 10) : fen
  if (!Number.isFinite(n)) return '0.00'
  return (n / 100).toFixed(2)
}

/** 手机号脱敏展示（与后端 mask 规则一致：前三后四）。 */
export function maskPhone(phone: string): string {
  if (phone.length < 7) return '***'
  return phone.slice(0, 3) + '****' + phone.slice(-4)
}

/** RFC3339(UTC) → 本地显示（YYYY-MM-DD HH:mm）。 */
export function formatDateTime(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
