import { View, Text, Input, Textarea } from '@tarojs/components'

export interface FormFieldProps {
  label: string
  value: string
  required?: boolean
  placeholder?: string
  multiline?: boolean
  onChange: (v: string) => void
}

// 统一表单字段：必填标记 + 定位第一个未填项由页面在提交时处理。
export default function FormField({ label, value, required, placeholder, multiline, onChange }: FormFieldProps) {
  return (
    <View className="form-field">
      <Text className="form-field__label">
        {label}
        {required ? <Text className="form-field__req">*</Text> : null}
      </Text>
      {multiline ? (
        <Textarea
          className="form-field__input form-field__input--area"
          value={value}
          placeholder={placeholder ?? `请输入${label}`}
          onInput={(e) => onChange(e.detail.value)}
        />
      ) : (
        <Input
          className="form-field__input"
          value={value}
          placeholder={placeholder ?? `请输入${label}`}
          onInput={(e) => onChange(e.detail.value)}
        />
      )}
    </View>
  )
}
