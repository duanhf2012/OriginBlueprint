import type { NodeSchema, PortSchema } from './nodeRegistry'
import type { DynamicBranchConfig } from './types'
import type { NodeKind } from './types'

type SocketType = 'exec' | 'integer' | 'boolean' | 'string' | 'float' | 'array' | 'any'

interface LegacyNodeDefinition {
  name?: string
  title?: string
  package?: string
  description?: string
  width?: number
  inputs?: LegacyPortDefinition[]
  outputs?: LegacyPortDefinition[]
}

interface LegacyPortDefinition {
  name?: string
  type?: string
  data_type?: string
  has_input?: boolean
  pin_widget?: string
  port_id?: number | string
  hide_icon?: boolean
}

interface LegacyNodeSpec {
  typeId: string
  inputs?: string[]
  outputs?: string[]
}

const legacyNodeSpecs: Record<string, LegacyNodeSpec> = {
  BeginNode: { typeId: 'origin.event.begin', outputs: ['exec'] },
  ForLoop: { typeId: 'origin.flow.for-loop', inputs: ['exec', 'start', 'end'], outputs: ['body', 'index', 'completed'] },
  Foreach: { typeId: 'origin.flow.for-loop', inputs: ['exec', 'start', 'end'], outputs: ['body', 'completed', 'index'] },
  BranchNode: { typeId: 'origin.flow.branch', inputs: ['exec', 'condition'], outputs: ['false', 'true'] },
  BoolIf: { typeId: 'origin.flow.branch', inputs: ['exec', 'condition'], outputs: ['false', 'true'] },
  PrintNode: { typeId: 'origin.action.print', inputs: ['exec', 'value'], outputs: ['exec'] },
  'int -> str': { typeId: 'origin.cast.integer-string', inputs: ['value'], outputs: ['result'] },
  Integer2String: { typeId: 'origin.cast.integer-string', inputs: ['value'], outputs: ['result'] },
  'float -> str': { typeId: 'origin.cast.float-string', inputs: ['value'], outputs: ['result'] },
  AddInt: { typeId: 'origin.math.add-integer', inputs: ['a', 'b'], outputs: ['result'] },
  '+ (Integer)': { typeId: 'origin.math.add-integer', inputs: ['a', 'b'], outputs: ['result'] },
  SubInt: { typeId: 'origin.math.subtract-integer', inputs: ['a', 'b', 'absolute'], outputs: ['result'] },
  MulInt: { typeId: 'origin.math.multiply-integer', inputs: ['a', 'b'], outputs: ['result'] },
  DivInt: { typeId: 'origin.math.divide-integer', inputs: ['a', 'b', 'round'], outputs: ['result'] },
  ModInt: { typeId: 'origin.math.modulo-integer', inputs: ['a', 'b'], outputs: ['result'] },
  RandNumber: { typeId: 'origin.math.random-integer', inputs: ['seed', 'min', 'max'], outputs: ['result'] },
  Sequence: { typeId: 'origin.flow.sequence', inputs: ['exec'], outputs: ['then0', 'then1', 'then2'] },
  GreaterThanInteger: { typeId: 'origin.flow.greater-integer', inputs: ['exec', 'orEqual', 'a', 'b'], outputs: ['false', 'true'] },
  LessThanInteger: { typeId: 'origin.flow.less-integer', inputs: ['exec', 'orEqual', 'a', 'b'], outputs: ['false', 'true'] },
  EqualInteger: { typeId: 'origin.flow.equal-integer', inputs: ['exec', 'a', 'b'], outputs: ['false', 'true'] },
  RangeCompare: { typeId: 'origin.flow.range-compare', inputs: ['exec', 'value', 'ranges'], outputs: ['otherwise', 'case0', 'case1', 'case2', 'case3', 'case4'] },
  EqualSwitch: { typeId: 'origin.flow.equal-switch', inputs: ['exec', 'value', 'cases'], outputs: ['otherwise', 'case0', 'case1', 'case2', 'case3', 'case4'] },
  GetArrayInt: { typeId: 'origin.array.get-integer', inputs: ['array', 'index'], outputs: ['value'] },
  GetArrayString: { typeId: 'origin.array.get-string', inputs: ['array', 'index'], outputs: ['value'] },
  GetArrayLen: { typeId: 'origin.array.length', inputs: ['array'], outputs: ['length'] },
  'Length (Array)': { typeId: 'origin.array.length', inputs: ['array'], outputs: ['length'] },
  CreateIntArray: { typeId: 'origin.array.create-integer', inputs: ['items'], outputs: ['array'] },
  CreateStringArray: { typeId: 'origin.array.create-string', inputs: ['items'], outputs: ['array'] },
  StringArray: { typeId: 'origin.array.create-string', inputs: ['items'], outputs: ['array'] },
  AppendStringToArray: { typeId: 'origin.array.append-string', inputs: ['array', 'value'], outputs: ['array'] },
  AppendIntegerToArray: { typeId: 'origin.array.append-integer', inputs: ['array', 'value'], outputs: ['array'] },
  AppendIntReturn: { typeId: 'origin.result.append-integer', inputs: ['exec', 'value'], outputs: ['exec'] },
  AppendStringReturn: { typeId: 'origin.result.append-string', inputs: ['exec', 'value'], outputs: ['exec'] },
  Entrance_ArrayParam_000002: { typeId: 'origin.event.entry-array', outputs: ['exec', 'objectId', 'params'] },
  Entrance_IntParam_000001: { typeId: 'origin.event.entry-two-integers', outputs: ['exec', 'objectId', 'param1', 'param2'] },
  Entrance_Timer_000003: { typeId: 'origin.event.timer', outputs: ['exec', 'timerId', 'params'] },
  CreateTimer: { typeId: 'origin.timer.create', inputs: ['exec', 'milliseconds', 'params'], outputs: ['exec', 'timerId'] },
  CloseTimer: { typeId: 'origin.timer.close', inputs: ['exec', 'timerId'], outputs: ['exec'] },
  ForeachIntArray: { typeId: 'origin.flow.foreach-integer-array', inputs: ['exec', 'array'], outputs: ['body', 'completed', 'index', 'value'] },
  Probability: { typeId: 'origin.flow.probability', inputs: ['exec', 'probability'], outputs: ['miss', 'hit'] },
  DebugOutput: { typeId: 'origin.debug.output', inputs: ['exec', 'integer', 'string', 'array'], outputs: ['exec'] },
  StringNode: { typeId: 'origin.literal.string', inputs: ['value'], outputs: ['value'] },
  AddNode: { typeId: 'origin.math.add-float', inputs: ['a', 'b'], outputs: ['result'] },
  MinusNode: { typeId: 'origin.math.subtract-float', inputs: ['a', 'b'], outputs: ['result'] },
  MultiplyNode: { typeId: 'origin.math.multiply-float', inputs: ['a', 'b'], outputs: ['result'] },
  DivideNode: { typeId: 'origin.math.divide-float', inputs: ['a', 'b'], outputs: ['result'] },
  GreaterIntegerNode: { typeId: 'origin.compare.greater-integer', inputs: ['a', 'b'], outputs: ['result', 'a', 'b'] },
  WhileNode: { typeId: 'origin.flow.while', inputs: ['exec', 'condition'], outputs: ['body', 'completed'] },
  ForLoopWithBreak: { typeId: 'origin.flow.for-loop-break', inputs: ['exec', 'start', 'end', 'break'], outputs: ['body', 'index', 'completed'] },
  ForEahcNode: { typeId: 'origin.flow.foreach-array', inputs: ['exec', 'array'], outputs: ['body', 'index', 'value', 'completed'] },
  Split: { typeId: 'origin.string.split', inputs: ['exec', 'text', 'delimiter'], outputs: ['exec', 'array'] },
  'Get (Array)': { typeId: 'origin.array.get-any', inputs: ['array', 'index'], outputs: ['value'] },
  'Cast To': { typeId: 'origin.cast.any-string', inputs: ['exec', 'value'], outputs: ['exec', 'result'] },
  CastingNode_str: { typeId: 'origin.cast.any-string', inputs: ['exec', 'value'], outputs: ['exec', 'valid', 'result'] },
}

export function parseNodeSchemaDocument(value: unknown): NodeSchema[] {
  const definitions = Array.isArray(value) ? value : isRecord(value) && Array.isArray(value.nodes) ? value.nodes : [value]
  return definitions.flatMap((definition, index) => {
    if (!isRecord(definition)) throw new Error(`node ${index}: expected object`)
    if (isNodeSchema(definition)) return [definition]
    return [convertLegacyNodeDefinition(definition as LegacyNodeDefinition, index)]
  })
}

function convertLegacyNodeDefinition(definition: LegacyNodeDefinition, index: number): NodeSchema {
  const name = String(definition.name ?? '').trim()
  if (!name) throw new Error(`node ${index}: missing name`)

  const spec = legacyNodeSpecs[name]
  const id = spec?.typeId || `origin.custom.${slug(name)}`
  const inputKeys = portKeys(spec?.inputs, 'in')
  const outputKeys = portKeys(spec?.outputs, 'out')
  const inputs = sortedLegacyPorts(definition.inputs).map((port, portIndex) => convertLegacyPort(port, inputKeys, portIndex, 'in', true))
  const outputs = sortedLegacyPorts(definition.outputs).map((port, portIndex) => convertLegacyPort(port, outputKeys, portIndex, 'out', false))

  return {
    id,
    title: firstNonEmpty(definition.title, name),
    category: firstNonEmpty(definition.package, 'Custom'),
    kind: inferNodeKind(id, inputs, outputs),
    subtitle: firstNonEmpty(definition.description, definition.package),
    width: definition.width,
    inputs,
    outputs,
    dynamicOutputs: id === 'origin.flow.sequence',
    dynamicBranch: dynamicBranchForType(id),
    custom: !spec
  }
}

function dynamicBranchForType(id: string): DynamicBranchConfig | undefined {
  if (id === 'origin.flow.equal-switch') {
    return {
      controlInput: 'cases',
      defaultOutput: 'otherwise',
      outputPrefix: 'case',
      outputStartIndex: 1,
      maxBranches: 4,
      hiddenOutputKeys: ['case0']
    }
  }
  return undefined
}

function convertLegacyPort(port: LegacyPortDefinition, keys: Map<number, string>, fallbackIndex: number, prefix: string, input: boolean): PortSchema {
  const index = legacyPortIndex(port.port_id, fallbackIndex)
  const type = normalizeSocketType(port.type, port.data_type)
  const itemType = arrayItemType(port.pin_widget)
  return {
    key: keys.get(index) || `${prefix}${index}`,
    label: String(port.name ?? ''),
    type,
    defaultValue: input && type !== 'exec' && (port.has_input || itemType) ? defaultPortValue(type) : undefined,
    arrayItemType: itemType,
    hideIcon: port.hide_icon
  }
}

function isNodeSchema(value: unknown): value is NodeSchema {
  return isRecord(value) && typeof value.id === 'string' && typeof value.title === 'string' && typeof value.category === 'string'
}

function isRecord(value: unknown): value is Record<string, any> {
  return Boolean(value) && typeof value === 'object'
}

function sortedLegacyPorts(ports: LegacyPortDefinition[] = []) {
  return [...ports].sort((a, b) => legacyPortIndex(a.port_id, 0) - legacyPortIndex(b.port_id, 0))
}

function portKeys(keys: string[] = [], prefix: string) {
  const result = new Map<number, string>()
  keys.forEach((key, index) => result.set(index, key || `${prefix}${index}`))
  return result
}

function legacyPortIndex(value: unknown, fallback: number) {
  if (typeof value === 'number' && Number.isFinite(value)) return Math.trunc(value)
  const parsed = Number.parseInt(String(value ?? ''), 10)
  return Number.isFinite(parsed) ? parsed : fallback
}

function inferNodeKind(id: string, inputs: Array<{ type: string }>, outputs: Array<{ type: string }>): NodeKind {
  if (id.startsWith('origin.event.')) return 'event'
  const hasExecInput = inputs.some(port => port.type === 'exec')
  const hasExecOutput = outputs.some(port => port.type === 'exec')
  if (hasExecOutput && !hasExecInput) return 'event'
  if (hasExecInput || hasExecOutput || id.startsWith('origin.flow.')) return 'flow'
  return 'function'
}

function normalizeSocketType(portType?: string, dataType?: string): SocketType {
  if (String(portType ?? '').toLowerCase() === 'exec') return 'exec'
  switch (String(dataType ?? '').trim().toLowerCase()) {
    case 'int':
    case 'integer':
      return 'integer'
    case 'float':
    case 'double':
    case 'number':
      return 'float'
    case 'bool':
    case 'boolean':
      return 'boolean'
    case 'array':
    case 'list':
      return 'array'
    case 'any':
    case '':
      return 'any'
    default:
      return 'string'
  }
}

function defaultPortValue(type: SocketType): unknown {
  if (type === 'integer' || type === 'float') return 0
  if (type === 'boolean') return false
  if (type === 'array') return []
  return ''
}

function arrayItemType(widget?: string): 'string' | 'number' | undefined {
  if (widget === 'IntegerArrayWdg') return 'number'
  if (widget === 'StringArrayWdg') return 'string'
  return undefined
}

function slug(value: string) {
  return value.trim().replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
}

function firstNonEmpty(...values: Array<string | undefined>) {
  return values.find(value => String(value ?? '').trim())?.trim() ?? ''
}
