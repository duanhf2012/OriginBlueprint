import { beforeEach, describe, expect, it } from 'vitest'
import { createFunctionEntryNode, createNode, registerNodeSchemas, resolveNodeLegacyClass } from '../src/editor/nodeRegistry'
import { describeEntryBinding, entrySourceColor } from '../src/editor/implicitEntryLinks'
import { parseNodeSchemaDocument } from '../src/editor/runtimeNodeSchemas'

const schemas = parseNodeSchemaDocument([
  {
    name: 'GetObjectInfo',
    title: '获取目标信息',
    package: '获取信息',
    inputs: [
      { name: '', type: 'exec', port_id: 0 },
      { name: '目标Id', type: 'data', data_type: 'Integer', port_id: 1 }
    ],
    outputs: [
      { name: '', type: 'exec', port_id: 0 },
      { name: 'Lev', type: 'data', data_type: 'Integer', port_id: 1 }
    ]
  },
  {
    name: 'AddInt',
    title: '整数相加',
    package: '数学',
    inputs: [
      { name: 'A', type: 'data', data_type: 'Integer', port_id: 0 },
      { name: 'B', type: 'data', data_type: 'Integer', port_id: 1 }
    ],
    outputs: [
      { name: '结果', type: 'data', data_type: 'Integer', port_id: 0 }
    ]
  }
])

describe('runtime node legacyClass', () => {
  beforeEach(() => registerNodeSchemas(schemas))

  it('copies the source runtime class onto new custom nodes', () => {
    expect(createNode('origin.custom.get-object-info').legacyClass).toBe('GetObjectInfo')
  })

  it('does not add a legacy class to built-in runtime nodes', () => {
    expect(createNode('origin.math.add-integer').legacyClass).toBeUndefined()
  })

  it('uses the registry class when persisted data is missing or blank', () => {
    expect(resolveNodeLegacyClass('origin.custom.get-object-info', undefined)).toBe('GetObjectInfo')
    expect(resolveNodeLegacyClass('origin.custom.get-object-info', '  ')).toBe('GetObjectInfo')
  })

  it('preserves an explicitly persisted runtime class', () => {
    expect(resolveNodeLegacyClass('origin.custom.get-object-info', 'HistoricalGetObjectInfo')).toBe('HistoricalGetObjectInfo')
  })
})

describe('function entry source color', () => {
  it('uses the stable function id for both the Entry diamond and binding badges', () => {
    const entry = createFunctionEntryNode({
      functionRole: 'entry',
      functionId: 'fn_calculate',
      functionName: 'Calculate',
      functionSignature: { inputs: [], outputs: [] }
    })

    expect(entry.entrySourceKey).toBe('fn_calculate')
    expect(entry.entrySourceColor).toBe(entrySourceColor('fn_calculate'))
  })
})

describe('custom workspace entrance identity color', () => {
  const entranceSchemas = parseNodeSchemaDocument([
    {
      name: '分组元素数量变化入口',
      title: '分组元素数量变化入口',
      package: '测试入口',
      outputs: [
        { name: '', type: 'exec', port_id: 0 },
        { name: '地图实例Id', type: 'data', data_type: 'Integer', has_input: true, port_id: 1 }
      ]
    }
  ])

  beforeEach(() => registerNodeSchemas([...schemas, ...entranceSchemas]))

  it('gives Chinese-named custom entrances the same stable color for icon and badge without an entry key', () => {
    const entrance = createNode('origin.custom.')
    expect(entrance.entrySourceColor).toBe(entrySourceColor('分组元素数量变化入口'))
    expect(entrance.entrySourceKey).toBeUndefined()
  })

  it('binding badges reuse the entrance color instead of hashing the degenerate custom typeId', () => {
    const entrance = createNode('origin.custom.')
    const binding = describeEntryBinding(
      { source: 'entrance', sourceOutput: 'out1', target: 'timer', targetInput: 'duration' },
      id => id === 'entrance'
        ? { id, typeId: entrance.typeId, label: entrance.label, entrySourceColor: entrance.entrySourceColor, inputs: {}, outputs: { out1: { label: '地图实例Id', socket: 'integer' } } }
        : { id, typeId: 'origin.timer.create', label: '创建定时器', inputs: { duration: { label: '时间（毫秒）', socket: 'integer' } }, outputs: {} }
    )
    expect(binding?.entrySourceColor).toBe(entrySourceColor('分组元素数量变化入口'))
  })
})
