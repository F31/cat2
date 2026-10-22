import type { DictOption } from '../api/admin'

export interface TreeNode {
  value: string
  title: string
  children?: TreeNode[]
}

// 把 parent_code 扁平选项组装成树；孤儿节点提升为根，避免字典缺父级时丢失。
export function buildDictTree(options: DictOption[]): TreeNode[] {
  const byCode = new Map<string, TreeNode>()
  const roots: TreeNode[] = []
  for (const o of options) {
    byCode.set(o.code, { value: o.code, title: o.label })
  }
  for (const o of options) {
    const node = byCode.get(o.code)!
    const parent = o.parent_code ? byCode.get(o.parent_code) : undefined
    if (parent) {
      parent.children = parent.children ?? []
      parent.children.push(node)
    } else {
      roots.push(node)
    }
  }
  return roots
}

export function dictLabelMap(options: DictOption[]): Map<string, string> {
  return new Map(options.map((o) => [o.code, o.label]))
}

export function labelsOf(codes: string[] | undefined, map: Map<string, string>): string {
  if (!codes || codes.length === 0) return '—'
  return codes.map((c) => map.get(c) ?? c).join('、')
}
