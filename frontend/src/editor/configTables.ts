import { shallowRef } from 'vue'

// 配置表运行时索引：由 App.vue 在工作区加载/刷新时填充，
// 节点上的引用选择控件按数据集 key 查询 id→名称。文档里始终只保存原始 ID。
export interface ConfigTableEntry {
  id: string
  name: string
  extra?: string
}

export interface ConfigTable {
  key: string
  name?: string
  file: string
  sheet?: string
  rowCount: number
  idColumn: string
  nameColumn: string
  missing?: boolean
  warning?: string
  entries: ConfigTableEntry[]
}

// shallowRef 让引用控件的 computed 依赖此索引：setConfigTables 替换后画布显示随之刷新。
const tablesByKey = shallowRef(new Map<string, ConfigTable>())

export function setConfigTables(tables: ConfigTable[]) {
  tablesByKey.value = new Map(tables.filter(table => table.key).map(table => [table.key, table]))
}

export function configTablesSnapshot(): ConfigTable[] {
  return Array.from(tablesByKey.value.values())
}

export function configTableByKey(key: string): ConfigTable | undefined {
  return tablesByKey.value.get(String(key ?? '').trim())
}

// 数据集显示名：节点控件 tooltip、下拉列表用它展示；key 是稳定绑定标识。
export function configTableLabel(key: string): string {
  const table = configTableByKey(key)
  return table?.name || String(key ?? '').trim()
}

export function lookupConfigName(key: string, id: unknown): string | undefined {
  const table = configTableByKey(key)
  if (!table) return undefined
  const target = String(id ?? '').trim()
  if (!target) return undefined
  return table.entries.find(entry => entry.id === target)?.name
}

// filterConfigEntries 支持三种查询形式：
// 1. 纯文本：id 或名称包含该字符串（不区分大小写，可通过 columns 限定列）；
// 2. ID 范围：如 "100-200" / "100~200"；
// 3. 数值比较：如 ">1000" / "<=50"。
export function filterConfigEntries(entries: ConfigTableEntry[], query: string, options?: { limit?: number; columns?: 'all' | 'id' | 'name' | 'custom' }): ConfigTableEntry[] {
  const limit = options?.limit
  const columns = options?.columns ?? 'all'
  const text = String(query ?? '').trim().toLowerCase()
  if (!text) return limit ? entries.slice(0, limit) : entries
  const range = text.match(/^(-?\d+)\s*[-~]\s*(-?\d+)$/)
  if (range) {
    const [min, max] = [Number(range[1]), Number(range[2])].sort((a, b) => a - b)
    return entries.filter(entry => {
      const id = Number(entry.id)
      return Number.isFinite(id) && id >= min && id <= max
    }).slice(0, limit ?? entries.length)
  }
  const comparison = text.match(/^(>=|<=|>|<)\s*(-?\d+)$/)
  if (comparison) {
    const threshold = Number(comparison[2])
    return entries.filter(entry => {
      const id = Number(entry.id)
      if (!Number.isFinite(id)) return false
      if (comparison[1] === '>') return id > threshold
      if (comparison[1] === '<') return id < threshold
      if (comparison[1] === '>=') return id >= threshold
      return id <= threshold
    }).slice(0, limit ?? entries.length)
  }
  const result: ConfigTableEntry[] = []
  for (const entry of entries) {
    if (columns === 'custom') {
      if ((entry.extra ?? '').toLowerCase().includes(text)) {
        result.push(entry)
        if (limit && result.length >= limit) break
      }
      continue
    }
    const idHit = columns !== 'name' && entry.id.toLowerCase().includes(text)
    const nameHit = columns !== 'id' && entry.name.toLowerCase().includes(text)
    const extraHit = columns === 'all' && (entry.extra ?? '').toLowerCase().includes(text)
    if (idHit || nameHit || extraHit) {
      result.push(entry)
      if (limit && result.length >= limit) break
    }
  }
  return result
}

export function searchConfigEntries(key: string, query: string, limit = 60): ConfigTableEntry[] {
  const table = configTableByKey(key)
  if (!table) return []
  return filterConfigEntries(table.entries, query, { limit })
}

export function configTableStatusLine(tables: ConfigTable[]): string {
  const identified = tables.filter(table => table.entries.length > 0)
  const warned = tables.filter(table => table.warning && !table.missing)
  const missing = tables.filter(table => table.missing)
  const entries = identified.reduce((total, table) => total + table.entries.length, 0)
  const base = `配置表 ${identified.length}/${tables.length} 个数据集，共 ${entries} 条 id·名称`
  if (warned.length) return `${base}，${warned.length} 个未识别（在“数据集管理”中调整表头识别）`
  if (missing.length) return `${base}，${missing.length} 个未找到来源文件`
  return base
}
