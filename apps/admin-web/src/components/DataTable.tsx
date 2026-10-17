import { Form, Input, InputNumber, Switch, Table } from 'antd'
import type { TableProps } from 'antd'

/** 统一数据表（后台默认紧凑密度；分页/滚动/空态沿用 antd 约定）。 */
export default function DataTable<T extends object>(props: TableProps<T>) {
  return <Table<T> size="small" {...props} />
}
