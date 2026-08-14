import { describe, expect, it } from 'vitest'
import {
  filenameStem,
  isFunctionBlueprintPath,
  serializeGraphDocument,
} from '../src/graphPersistence'
import type { GraphDocument } from '../src/editor/document'

function document(name: string): GraphDocument {
  return {
    schemaVersion: 1,
    graphName: name,
    nodes: [],
    connections: [],
    groups: [],
    variables: [],
    variableGroups: [],
    view: { x: 0, y: 0, zoom: 1 },
  }
}

describe('graph persistence', () => {
  it('shares one non-mutating serialization contract across ordinary persistence boundaries', () => {
    const source = document('Stale Historical Name')
    const compact = '{"schemaVersion":1,"nodes":[],"connections":[],"groups":[],"variables":[],"variableGroups":[],"view":{"x":0,"y":0,"zoom":1}}'
    const indented = `{
  "schemaVersion": 1,
  "nodes": [],
  "connections": [],
  "groups": [],
  "variables": [],
  "variableGroups": [],
  "view": {
    "x": 0,
    "y": 0,
    "zoom": 1
  }
}`

    const boundaryPayloads = {
      validation: serializeGraphDocument('Blueprints/Combat.obp', source),
      recoverySnapshot: serializeGraphDocument('Blueprints/Combat.obp', source),
      nativeSave: serializeGraphDocument('Blueprints/Combat.obp', source, 2),
      legacyExport: serializeGraphDocument('Blueprints/Legacy.vgf', source),
    }

    expect(boundaryPayloads.validation).toBe(compact)
    expect(boundaryPayloads.recoverySnapshot).toBe(compact)
    expect(boundaryPayloads.nativeSave).toBe(indented)
    expect(boundaryPayloads.legacyExport).toBe(compact)
    expect(source.graphName).toBe('Stale Historical Name')
  })

  it('omits the transient graph name when serializing an ordinary .obp document', () => {
    const serialized = serializeGraphDocument('C:\\Blueprints\\Combat.obp', document('Combat'))

    expect(Object.prototype.hasOwnProperty.call(JSON.parse(serialized), 'graphName')).toBe(false)
  })

  it('omits the transient graph name when serializing a legacy .vgf document', () => {
    const serialized = serializeGraphDocument('Blueprints/Legacy.vgf', document('Legacy'))

    expect(Object.prototype.hasOwnProperty.call(JSON.parse(serialized), 'graphName')).toBe(false)
  })

  it('preserves function metadata when serializing an .obpf document', () => {
    const functionDocument: GraphDocument = {
      ...document('Apply Damage'),
      functionId: 'function-42',
      functionSignature: {
        inputs: [{ id: 'amount', name: 'Amount', type: 'integer' }],
        outputs: [{ id: 'applied', name: 'Applied', type: 'boolean' }],
      },
    }

    expect(JSON.parse(serializeGraphDocument('Blueprints/ApplyDamage.OBPF', functionDocument))).toMatchObject({
      graphName: 'Apply Damage',
      functionId: 'function-42',
      functionSignature: {
        inputs: [{ id: 'amount', name: 'Amount', type: 'integer' }],
        outputs: [{ id: 'applied', name: 'Applied', type: 'boolean' }],
      },
    })
  })

  it('derives filename stems from Windows and slash-separated paths for transient editor state', () => {
    expect(filenameStem('C:\\Blueprints\\Combat.obp')).toBe('Combat')
    expect(filenameStem('Blueprints/Functions/ApplyDamage.obpf')).toBe('ApplyDamage')
    expect(isFunctionBlueprintPath('Blueprints/Functions/ApplyDamage.OBPF')).toBe(true)
  })
})
