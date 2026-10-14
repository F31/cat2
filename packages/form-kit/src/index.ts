// 表单字段模板驱动的校验与排序。字段定义池由后端 dict.field_def 下发，
// 前端只按模板渲染，不在代码里写死字段。

export interface FieldDef {
  field: string
  label: string
  value_type: 'text' | 'number' | 'select' | 'date' | 'tags'
}

export interface FieldTemplateItem {
  field: string
  required: boolean
  sort: number
}

export type FormValues = Record<string, unknown>

/** 按模板排序字段（缺失定义池条目的字段排最后）。 */
export function orderFields(template: FieldTemplateItem[]): FieldTemplateItem[] {
  return [...template].sort((a, b) => a.sort - b.sort)
}

/** 必填校验：返回第一个未通过的字段名，全部通过返回 null。 */
export function validateRequired(
  template: FieldTemplateItem[],
  values: FormValues,
): string | null {
  for (const item of orderFields(template)) {
    if (!item.required) continue
    const v = values[item.field]
    if (v === undefined || v === null || v === '') return item.field
    if (Array.isArray(v) && v.length === 0) return item.field
  }
  return null
}
