import { describe, expect, it } from 'vitest'
import { effect } from '@vue/reactivity'
import { configTableByKey, setConfigTables, type ConfigTable } from '../src/editor/configTables'

function table(name: string): ConfigTable {
  return { key: 'skills', name, file: 'skills.csv', rowCount: 1, idColumn: 'id', nameColumn: 'name', entries: [] }
}

describe('config tables runtime index', () => {
  it('setConfigTables 替换索引后，依赖索引的响应式读取自动刷新', () => {
    const seen: Array<string | undefined> = []
    effect(() => { seen.push(configTableByKey('skills')?.name) })
    setConfigTables([table('技能表A')])
    setConfigTables([table('技能表B')])
    // 初始读取（undefined）+ 两次替换各触发一次重算
    expect(seen).toEqual([undefined, '技能表A', '技能表B'])
  })
})
