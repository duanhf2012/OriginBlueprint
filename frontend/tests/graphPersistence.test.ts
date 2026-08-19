import { describe, expect, it } from 'vitest'
import {
  applyFunctionPersistenceMetadata,
  completeGraphSavePath,
  documentRequiresNativePersistence,
  filenameStem,
  isFunctionBlueprintPath,
  prepareGraphSave,
  serializeGraphDocument,
} from '../src/graphPersistence'
import type { GraphDocument } from '../src/editor/document'
import { normalizeNodeInputDefault } from '../src/editor/document'
import { cloneGraphJSONValue, isPreciseJSONInteger, parseGraphJSON, stringifyGraphJSON } from '../src/graphJSON'
import { isValidIntegerDefault, normalizeIntegerInput } from '../src/editor/valueValidation'

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
	it('preserves full int64 values while keeping safe integers numeric', () => {
		const parsed = parseGraphJSON('{"safe":9007199254740991,"max":9223372036854775807,"min":-9223372036854775808,"text":"9223372036854775807"}') as Record<string, unknown>
		expect(parsed.safe).toBe(9007199254740991)
		expect(parsed.text).toBe('9223372036854775807')
		expect(isPreciseJSONInteger(parsed.max)).toBe(true)
		expect(String(parsed.max)).toBe('9223372036854775807')
		expect(String(parsed.min)).toBe('-9223372036854775808')
		expect(stringifyGraphJSON(parsed)).toBe('{"safe":9007199254740991,"max":9223372036854775807,"min":-9223372036854775808,"text":"9223372036854775807"}')
	})

	it('preserves opaque legacy and Any unsafe integers as JSON numbers through clone and save', () => {
		const source = '{"schemaVersion":1,"nodes":[{"id":"opaque","typeId":"origin.legacy.placeholder","position":{"x":0,"y":0},"values":{"value":9223372036854775807}}],"connections":[],"groups":[],"variables":[],"variableGroups":[],"view":{"x":0,"y":0,"zoom":1},"legacy":{"extraRootFields":{"future_root":9223372036854775807},"extraNodeFields":{"opaque":{"class":"Future","fields":{"future_node":-9223372036854775808}}},"extraEdgeFields":{"0":{"future_edge":9007199254740992}}}}'
		const parsed = parseGraphJSON(source) as GraphDocument
		parsed.legacy = cloneGraphJSONValue(parsed.legacy)
		const saved = serializeGraphDocument('Opaque.obp', parsed)
		expect(saved).toContain('"value":9223372036854775807')
		expect(saved).toContain('"future_root":9223372036854775807')
		expect(saved).toContain('"future_node":-9223372036854775808')
		expect(saved).toContain('"future_edge":9007199254740992')
		expect(saved).not.toContain('"9223372036854775807"')
	})

	it('normalizes integer edits without storing BigInt values in graph documents', () => {
		expect(normalizeIntegerInput('42')).toBe(42)
		expect(normalizeIntegerInput('9223372036854775807')).toBe('9223372036854775807')
		expect(normalizeIntegerInput('-9223372036854775808')).toBe('-9223372036854775808')
		expect(normalizeIntegerInput('9223372036854775808')).toBe('9223372036854775808')
		expect(isValidIntegerDefault('9223372036854775807')).toBe(true)
		expect(isValidIntegerDefault('9223372036854775808')).toBe(false)
		expect(isValidIntegerDefault(9007199254740992)).toBe(false)
		const parsed = parseGraphJSON('{"value":9223372036854775807}') as { value: unknown }
		expect(isValidIntegerDefault(parsed.value)).toBe(true)
		expect(normalizeNodeInputDefault('integer', parsed.value)).toBe('9223372036854775807')
		expect(normalizeNodeInputDefault('integer', '')).toBe(0)
	})

	it('keeps old variables legacy-compatible and requires .obp for instance variables', () => {
		const local = document('Local')
		local.variables = [{ id: 'count', name: 'Count', type: 'integer', defaultValue: 0, groupId: 'default' }]
		expect(documentRequiresNativePersistence(local)).toBe(false)
		expect(JSON.parse(serializeGraphDocument('Local.obp', local)).variables[0]).not.toHaveProperty('scope')

		const shared = document('Shared')
		shared.variables = [{ id: 'count', name: 'Count', type: 'integer', defaultValue: 0, groupId: 'default', scope: 'instance' }]
		expect(documentRequiresNativePersistence(shared)).toBe(true)
		expect(JSON.parse(serializeGraphDocument('Shared.obp', shared)).variables[0].scope).toBe('instance')
	})
  it('serializes ordinary save payloads from the selected final path without mutating editor state', () => {
    const source = document('Stale Historical Name')
    const compact = '{"schemaVersion":1,"nodes":[],"connections":[],"groups":[],"variables":[],"variableGroups":[],"view":{"x":0,"y":0,"zoom":1}}'

    const save = prepareGraphSave('Blueprints/Combat.obp', 'Blueprints/Renamed.vgf', source)

    expect(save).toEqual({ path: 'Blueprints/Renamed.vgf', documentJSON: compact, exportLegacy: true })
    expect(source.graphName).toBe('Stale Historical Name')
  })

  it('rejects save targets that change between ordinary and function blueprints', () => {
    expect(() => prepareGraphSave('Functions/ApplyDamage.obpf', 'ApplyDamage.obp', document('Apply Damage')))
      .toThrow('Function blueprints must be saved as .obpf files')
    expect(() => prepareGraphSave('Combat.obp', 'Combat.obpf', document('Combat')))
      .toThrow('Ordinary blueprints cannot be saved as .obpf files')
    expect(() => prepareGraphSave('', 'Untitled.obpf', document('Untitled')))
      .toThrow('Ordinary blueprints cannot be saved as .obpf files')
  })

  it('completes extensionless selected paths before persistence decisions', () => {
    expect(completeGraphSavePath('Functions/ApplyDamage', true, false)).toBe('Functions/ApplyDamage.obpf')
    expect(completeGraphSavePath('Blueprints/Timer', false, true)).toBe('Blueprints/Timer.obp')
    expect(completeGraphSavePath('Blueprints/Compatible', false, false)).toBe('Blueprints/Compatible.vgf')
    expect(completeGraphSavePath('Blueprints/Explicit.OBP', false, false)).toBe('Blueprints/Explicit.OBP')
  })

  it('preserves function metadata when the selected final target remains .obpf', () => {
    const source = applyFunctionPersistenceMetadata('Functions/ApplyDamage.obpf', document('Stale'), {
      graphName: 'Apply Damage',
      functionId: 'function-42',
      functionCategory: 'Combat',
      functionSignature: {
        inputs: [{ id: 'amount', name: 'Amount', type: 'integer' }],
        outputs: [{ id: 'applied', name: 'Applied', type: 'boolean' }],
      },
    })

    const save = prepareGraphSave('Functions/ApplyDamage.obpf', 'Functions/Renamed.obpf', source)

    expect(save.exportLegacy).toBe(false)
    expect(JSON.parse(save.documentJSON)).toMatchObject({
      graphName: 'Apply Damage',
      functionId: 'function-42',
      functionCategory: 'Combat',
      functionSignature: {
        inputs: [{ id: 'amount', name: 'Amount', type: 'integer' }],
        outputs: [{ id: 'applied', name: 'Applied', type: 'boolean' }],
      },
    })
  })

  it('hydrates function metadata before explicit validation serialization', () => {
    const source = document('Editor Placeholder')
    const hydrated = applyFunctionPersistenceMetadata('Functions/ApplyDamage.obpf', source, {
      graphName: 'Apply Damage',
      functionId: 'function-42',
      functionCategory: 'Combat',
      functionSignature: {
        inputs: [{ id: 'amount', name: 'Amount', type: 'integer' }],
        outputs: [],
      },
    })

    expect(JSON.parse(serializeGraphDocument('Functions/ApplyDamage.obpf', hydrated))).toMatchObject({
      graphName: 'Apply Damage',
      functionId: 'function-42',
      functionCategory: 'Combat',
      functionSignature: {
        inputs: [{ id: 'amount', name: 'Amount', type: 'integer' }],
        outputs: [],
      },
    })
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
