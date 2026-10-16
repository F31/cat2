// gen-enums 由 contracts/enums/*.json 生成 packages/domain-types/src/enums.generated.ts。
// 契约即事实来源：枚举变更先改 contracts，再 `make gen`，禁止手写生成文件。
import { readFileSync, writeFileSync, readdirSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const dir = join(root, 'contracts/enums')
const out = join(root, 'packages/domain-types/src/enums.generated.ts')

const files = readdirSync(dir).filter((f) => f.endsWith('.json')).sort()
const chunks = [
  '// 本文件由 contracts/enums/*.json 生成（make gen），请勿手写。',
  '',
]

for (const f of files) {
  const doc = JSON.parse(readFileSync(join(dir, f), 'utf8'))
  if (!doc.name || !Array.isArray(doc.values) || doc.values.length === 0) {
    throw new Error(`${f}: 缺少 name 或 values`)
  }
  const keys = new Set()
  const values = new Set()
  chunks.push(`/** ${doc.name}（${doc.module}）：${doc.note ?? ''} */`)
  chunks.push(`export const ${doc.name} = {`)
  for (const v of doc.values) {
    if (keys.has(v.key) || values.has(v.value)) {
      throw new Error(`${f}: 键或值重复（${v.key}/${v.value}）`)
    }
    keys.add(v.key)
    values.add(v.value)
    chunks.push(`  ${v.key}: ${v.value}, // ${v.label ?? ''}`)
  }
  chunks.push('} as const')
  chunks.push(`export type ${doc.name} = (typeof ${doc.name})[keyof typeof ${doc.name}]`)
  chunks.push(`export const ${doc.name}Label: Record<number, string> = {`)
  for (const v of doc.values) {
    chunks.push(`  [${doc.name}.${v.key}]: '${(v.label ?? '').replace(/'/g, "\\'")}',`)
  }
  chunks.push('}')
  chunks.push('')
}

writeFileSync(out, chunks.join('\n'))
console.log(`gen-enums: ${files.length} 个枚举 → packages/domain-types/src/enums.generated.ts`)
