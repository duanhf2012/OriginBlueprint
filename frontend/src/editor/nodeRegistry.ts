import { ClassicPreset } from 'rete'
import { ArrayControl, BlueprintNode, type DynamicBranchConfig, type NodeKind } from './types'
import type { FunctionNodeMetadata, FunctionSignature, FunctionSignaturePort, GraphVariable, NodeProperties } from './document'

export interface NodeDefinition {
  id: string
  title: string
  category: string
  kind: NodeKind
  create(): BlueprintNode
}

const sockets = {
  exec: new ClassicPreset.Socket('exec'),
  integer: new ClassicPreset.Socket('integer'),
  boolean: new ClassicPreset.Socket('boolean'),
  string: new ClassicPreset.Socket('string'),
  float: new ClassicPreset.Socket('float'),
  array: new ClassicPreset.Socket('array'),
  any: new ClassicPreset.Socket('any')
}

type SocketType = keyof typeof sockets
type PortKind = SocketType | 'data'
export interface PortSchema { key: string; label: string; type: PortKind; data_type?: string; defaultValue?: unknown; arrayItemType?: 'string' | 'number'; hideIcon?: boolean }
export interface NodeSchema {
  id: string
  title: string
  category: string
  kind?: NodeKind
  subtitle?: string
  width?: number
  inputs?: PortSchema[]
  outputs?: PortSchema[]
  dynamicOutputs?: boolean
  dynamicBranch?: DynamicBranchConfig
  custom?: boolean
}

let allNodeDefinitions: NodeDefinition[] = []
const hiddenNodeTypes = new Set(['origin.event.timer', 'origin.timer.create', 'origin.timer.close'])
export let nodeDefinitions: NodeDefinition[] = []

function input(socket: ClassicPreset.Socket, label: string, value?: unknown, arrayItemType: 'string' | 'number' = 'string') {
  const port = new ClassicPreset.Input(socket, label)
  if (Array.isArray(value)) {
    port.addControl(new ArrayControl(arrayItemType, value))
  } else if (value !== undefined) {
    port.addControl(new ClassicPreset.InputControl(typeof value === 'number' ? 'number' : 'text', { initial: value }))
  }
  return port
}

export function nodeTitleWidth(title: string) {
  let units = 0
  for (const char of title) units += /[\u4e00-\u9fff\uff00-\uffef]/.test(char) ? 1 : 0.58
  return Math.max(230, Math.min(520, Math.ceil(units * 16 + 72)))
}

function node(typeId: string, title: string, kind: NodeKind, subtitle: string, width: number) {
  const result = new BlueprintNode(title, kind, subtitle)
  result.typeId = typeId
  result.width = Math.max(width, nodeTitleWidth(title))
  return result
}

function socketTypeForPort(port: PortSchema): SocketType {
  if (port.type === 'exec') return 'exec'
  if (port.type === 'data') return socketTypeForDataType(port.data_type)
  return sockets[port.type] ? port.type : socketTypeForDataType(port.data_type)
}

function socketTypeForDataType(value?: string): SocketType {
  switch (String(value ?? '').trim().toLowerCase()) {
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
    case 'string':
      return 'string'
    case 'array':
    case 'list':
      return 'array'
    case 'any':
    default:
      return 'any'
  }
}

function inferKind(schema: NodeSchema): NodeKind {
  if (schema.kind) return schema.kind
  if (schema.id.startsWith('origin.variable.')) return 'variable'
  if (schema.id.startsWith('origin.event.')) return 'event'
  const inputs = schema.inputs ?? []
  const outputs = schema.outputs ?? []
  const hasExecInput = inputs.some(port => socketTypeForPort(port) === 'exec')
  const hasExecOutput = outputs.some(port => socketTypeForPort(port) === 'exec')
  if (hasExecOutput && !hasExecInput) return 'event'
  if (hasExecInput || hasExecOutput || schema.id.startsWith('origin.flow.')) return 'flow'
  return 'function'
}

function fromSchema(schema: NodeSchema): NodeDefinition {
  const kind = inferKind(schema)
  return {
    id: schema.id,
    title: schema.title,
    category: schema.category,
    kind,
    create() {
      const result = node(schema.id, schema.title, kind, schema.subtitle ?? schema.category, schema.width ?? 230)
      result.dynamicOutputs = schema.dynamicOutputs
      result.dynamicBranch = schema.dynamicBranch
      if (schema.dynamicOutputs) result.dynamicOutputCount = schema.outputs?.filter(port => port.key.startsWith('then')).length ?? 1
      for (const port of schema.inputs ?? []) {
        const socketType = socketTypeForPort(port)
        const defaultValue = schema.dynamicBranch?.controlInput === port.key && port.defaultValue === undefined ? [] : port.defaultValue
        result.addInput(port.key, input(sockets[socketType] ?? sockets.any, port.label, defaultValue, port.arrayItemType))
      }
      for (const port of schema.outputs ?? []) {
        const socketType = socketTypeForPort(port)
        result.addOutput(port.key, new ClassicPreset.Output(sockets[socketType] ?? sockets.any, port.label))
      }
      return result
    }
  }
}

function visibleNodeDefinitions() {
  return allNodeDefinitions.filter(item => !hiddenNodeTypes.has(item.id))
}

export function getNodeDefinitions() {
  return nodeDefinitions
}

export function registerNodeSchemas(schemas: NodeSchema[]) {
  const byId = new Map<string, NodeDefinition>()
  for (const schema of schemas) {
    if (!schema.id || !schema.title) continue
    byId.set(schema.id, fromSchema(schema))
  }
  allNodeDefinitions = Array.from(byId.values())
  nodeDefinitions = visibleNodeDefinitions()
}

export function hasNodeDefinition(typeId: string) {
  return allNodeDefinitions.some(item => item.id === typeId)
}

export function createNode(typeId: string) {
  const definition = allNodeDefinitions.find(item => item.id === typeId)
  if (!definition) throw new Error(`Unknown node type: ${typeId}`)
  return definition.create()
}

function variableSocket(type: GraphVariable['type']) {
  return sockets[type]
}

function cloneFunctionSignature(signature?: FunctionSignature): FunctionSignature {
  return {
    inputs: signature?.inputs?.map(port => ({ ...port })) ?? [],
    outputs: signature?.outputs?.map(port => ({ ...port })) ?? []
  }
}

function functionSocket(type: FunctionSignaturePort['type']) {
  return sockets[type] ?? sockets.any
}

function functionPortKey(prefix: 'input' | 'output', port: FunctionSignaturePort, index: number) {
  const key = String(port.id || port.name || `${index + 1}`).trim().replace(/[^a-zA-Z0-9_-]+/g, '-').replace(/^-+|-+$/g, '')
  return `${prefix}_${key || index + 1}`
}

function applyFunctionMetadata(node: BlueprintNode, metadata: FunctionNodeMetadata) {
  node.functionRole = metadata.functionRole
  node.functionId = metadata.functionId
  node.functionName = metadata.functionName
  node.functionSource = metadata.functionSource
  node.functionSignature = cloneFunctionSignature(metadata.functionSignature)
}

function normalizedFunctionMetadata(metadata: FunctionNodeMetadata): FunctionNodeMetadata {
  return {
    ...metadata,
    functionId: metadata.functionId || metadata.functionName || 'function',
    functionName: metadata.functionName || 'Function',
    functionSignature: cloneFunctionSignature(metadata.functionSignature)
  }
}

export function createFunctionCallNode(metadata: FunctionNodeMetadata) {
  const spec = normalizedFunctionMetadata({ ...metadata, functionRole: 'call' })
  const result = node('origin.function.call', spec.functionName, 'function', 'Function call', 245)
  result.addInput('exec', input(sockets.exec, ''))
  for (const [index, port] of spec.functionSignature?.inputs.entries() ?? []) {
    result.addInput(functionPortKey('input', port, index), input(functionSocket(port.type), port.name))
  }
  result.addOutput('exec', new ClassicPreset.Output(sockets.exec, ''))
  for (const [index, port] of spec.functionSignature?.outputs.entries() ?? []) {
    result.addOutput(functionPortKey('output', port, index), new ClassicPreset.Output(functionSocket(port.type), port.name))
  }
  applyFunctionMetadata(result, spec)
  return result
}

export function createFunctionEntryNode(metadata: FunctionNodeMetadata) {
  const spec = normalizedFunctionMetadata({ ...metadata, functionRole: 'entry' })
  const result = node('origin.function.entry', `${spec.functionName} Entry`, 'event', 'Function entry', 245)
  result.addOutput('exec', new ClassicPreset.Output(sockets.exec, ''))
  for (const [index, port] of spec.functionSignature?.inputs.entries() ?? []) {
    result.addOutput(functionPortKey('input', port, index), new ClassicPreset.Output(functionSocket(port.type), port.name))
  }
  applyFunctionMetadata(result, spec)
  return result
}

export function createFunctionReturnNode(metadata: FunctionNodeMetadata) {
  const spec = normalizedFunctionMetadata({ ...metadata, functionRole: 'return' })
  const result = node('origin.function.return', `${spec.functionName} Return`, 'flow', 'Function return', 245)
  result.addInput('exec', input(sockets.exec, ''))
  for (const [index, port] of spec.functionSignature?.outputs.entries() ?? []) {
    result.addInput(functionPortKey('output', port, index), input(functionSocket(port.type), port.name))
  }
  applyFunctionMetadata(result, spec)
  return result
}

export function createVariableNode(variable: GraphVariable, access: 'get' | 'set') {
  const typeId = `origin.variable.${access}`
  const title = `${access === 'get' ? 'Get' : 'Set'} ${variable.name}`
  const result = node(typeId, title, 'variable', `${variable.type} variable`, 220)
  result.variableId = variable.id
  result.variableAccess = access
  result.compact = access === 'get'
  const socket = variableSocket(variable.type)
  if (access === 'get') {
    result.addOutput('value', new ClassicPreset.Output(socket, variable.name))
  } else {
    result.addInput('exec', input(sockets.exec, ''))
    result.addInput('value', input(socket, variable.name, variable.defaultValue, variable.type === 'array' ? 'string' : 'string'))
    result.addOutput('exec', new ClassicPreset.Output(sockets.exec, ''))
    result.addOutput('value', new ClassicPreset.Output(socket, variable.name))
  }
  return result
}

export function createLegacyNode(properties: NodeProperties) {
  const result = node('origin.legacy.placeholder', properties.label || properties.legacyClass || 'Legacy Node', 'function', `Legacy: ${properties.legacyModule || 'unknown'}`, 245)
  result.legacyStyle = true
  result.legacyClass = properties.legacyClass
  result.legacyModule = properties.legacyModule
  result.legacyInputs = properties.legacyInputs?.map(port => ({ ...port })) ?? []
  result.legacyOutputs = properties.legacyOutputs?.map(port => ({ ...port })) ?? []
  for (const port of result.legacyInputs) result.addInput(port.key, input(sockets[port.type as SocketType] ?? sockets.any, port.label, port.type === 'exec' ? undefined : ''))
  for (const port of result.legacyOutputs) result.addOutput(port.key, new ClassicPreset.Output(sockets[port.type as SocketType] ?? sockets.any, port.label))
  return result
}
