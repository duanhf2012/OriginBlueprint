import { beforeEach, describe, expect, it } from 'vitest'
import { createNode, registerNodeSchemas, resolveNodeLegacyClass } from '../src/editor/nodeRegistry'
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
