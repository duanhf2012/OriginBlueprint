import { describe, expect, it } from 'vitest'
import { assignEntrySourcePalette, entrySourceColor, entrySourcePalette } from '../src/editor/implicitEntryLinks'
import { createNode, registerNodeSchemas } from '../src/editor/nodeRegistry'
import { parseNodeSchemaDocument } from '../src/editor/runtimeNodeSchemas'

describe('entry source palette', () => {
  it('ships 50 unique hand-ordered colors', () => {
    expect(entrySourcePalette).toHaveLength(50)
    expect(new Set(entrySourcePalette).size).toBe(50)
  })

  it('assigns palette colors by sorted key rank so entrances never share a color', () => {
    assignEntrySourcePalette(['分组元素数量变化入口', 'BeginNode', '进入地图'])
    expect(entrySourceColor('BeginNode')).toBe(entrySourcePalette[0])
    expect(entrySourceColor('分组元素数量变化入口')).toBe(entrySourcePalette[1])
    expect(entrySourceColor('进入地图')).toBe(entrySourcePalette[2])
  })

  it('wraps after 50 entrances deterministically', () => {
    const keys = Array.from({ length: 60 }, (_, index) => `entrance-${String(index).padStart(2, '0')}`)
    assignEntrySourcePalette(keys)
    expect(entrySourceColor('entrance-00')).toBe(entrySourcePalette[0])
    expect(entrySourceColor('entrance-49')).toBe(entrySourcePalette[49])
    expect(entrySourceColor('entrance-50')).toBe(entrySourcePalette[0])
    const first50 = keys.slice(0, 50).map(key => entrySourceColor(key))
    expect(new Set(first50).size).toBe(50)
  })

  it('falls back to the stable hash color for unassigned keys', () => {
    assignEntrySourcePalette(['a'])
    const color = entrySourceColor('unassigned-key')
    expect(color).toMatch(/^#[0-9a-f]{6}$/)
    expect(entrySourceColor('unassigned-key')).toBe(color)
  })

  it('gives builtin and custom entrances distinct palette colors at registration', () => {
    registerNodeSchemas(parseNodeSchemaDocument([
      {
        name: 'BeginNode',
        title: '开始',
        package: '入口',
        outputs: [{ name: '', type: 'exec', port_id: 0 }]
      },
      {
        name: '分组元素数量变化入口',
        title: '分组元素数量变化入口',
        package: '测试入口',
        outputs: [
          { name: '', type: 'exec', port_id: 0 },
          { name: '地图实例Id', type: 'data', data_type: 'Integer', has_input: true, port_id: 1 }
        ]
      }
    ]))
    const begin = createNode('origin.event.begin')
    const custom = createNode('origin.custom.')
    expect(entrySourcePalette).toContain(begin.entrySourceColor)
    expect(entrySourcePalette).toContain(custom.entrySourceColor)
    expect(begin.entrySourceColor).not.toBe(custom.entrySourceColor)
  })
})
