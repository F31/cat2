import { Picker, Text, View } from '@tarojs/components'

export interface DateTimeFieldProps {
  label: string
  value: string
  required?: boolean
  onChange: (v: string) => void
}

const pad = (n: number) => String(n).padStart(2, '0')

function today(): string {
  const d = new Date()
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function split(value: string): { date: string; time: string } {
  const m = /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2})/.exec(value)
  if (m) return { date: m[1], time: m[2] }
  const m2 = /^(\d{4}-\d{2}-\d{2})$/.exec(value)
  if (m2) return { date: m2[1], time: '09:00' }
  return { date: '', time: '09:00' }
}

// 会议/活动时间：日期 + 时间选择，输出带 +08:00 的 ISO 字符串（后端 time.Time 可直接解析）。
export default function DateTimeField({ label, value, required, onChange }: DateTimeFieldProps) {
  const { date, time } = split(value)
  const emit = (nextDate: string, nextTime: string) => {
    if (!nextDate) {
      onChange('')
      return
    }
    onChange(`${nextDate}T${nextTime || '09:00'}:00+08:00`)
  }
  return (
    <View className="form-field">
      <Text className="form-field__label">
        {label}
        {required ? <Text className="form-field__req">*</Text> : null}
      </Text>
      <View className="datetime-field">
        <Picker
          mode="date"
          value={date || today()}
          start={today()}
          onChange={(e) => emit(String(e.detail.value), time || '09:00')}
        >
          <View className="datetime-field__item">
            <Text className={date ? 'datetime-field__value' : 'datetime-field__placeholder'}>
              {date || '选择日期'}
            </Text>
          </View>
        </Picker>
        <Picker mode="time" value={time} onChange={(e) => emit(date || today(), String(e.detail.value))}>
          <View className="datetime-field__item">
            <Text className={date ? 'datetime-field__value' : 'datetime-field__placeholder'}>
              {date ? time : '选择时间'}
            </Text>
          </View>
        </Picker>
      </View>
    </View>
  )
}
