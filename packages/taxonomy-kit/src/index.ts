// 字典维度前端工具：把扁平的 DictItem 列表构建为级联树。

export interface DictItem {
  code: string
  name: string
  parent_code?: string | null
  sort: number
}

export interface DictNode extends DictItem {
  children: DictNode[]
}

/** 扁平列表 → 树（按 parent_code 归组，按 sort 排序）。 */
export function buildTree(items: DictItem[]): DictNode[] {
  const nodes = new Map<string, DictNode>()
  for (const it of items) nodes.set(it.code, { ...it, children: [] })
  const roots: DictNode[] = []
  for (const node of nodes.values()) {
    const parent = node.parent_code ? nodes.get(node.parent_code) : undefined
    if (parent) parent.children.push(node)
    else roots.push(node)
  }
  const bySort = (a: DictNode, b: DictNode) => a.sort - b.sort || a.code.localeCompare(b.code)
  const sortRec = (ns: DictNode[]) => {
    ns.sort(bySort)
    ns.forEach((n) => sortRec(n.children))
  }
  sortRec(roots)
  return roots
}

/** 一级选项（如省、行业大类）。 */
export function topLevel(items: DictItem[]): DictItem[] {
  return items.filter((i) => !i.parent_code).sort((a, b) => a.sort - b.sort)
}

/** 某节点的直接子级。 */
export function childrenOf(items: DictItem[], parentCode: string): DictItem[] {
  return items.filter((i) => i.parent_code === parentCode).sort((a, b) => a.sort - b.sort)
}
