export type VariableType = 'boolean' | 'integer' | 'float' | 'string' | 'array' | 'file' | 'table' | 'dictionary'

export interface GraphVariable {
  id: string
  name: string
  type: VariableType
  defaultValue: unknown
  groupId: string
  description?: string
}

export interface GraphVariableGroup {
  id: string
  name: string
  collapsed?: boolean
}

export interface FunctionSignaturePort {
  id: string
  name: string
  type: VariableType
}

export interface FunctionSignature {
  inputs: FunctionSignaturePort[]
  outputs: FunctionSignaturePort[]
}

export interface NodeProperties {
  label?: string
  variableId?: string
  variableAccess?: 'get' | 'set'
  dynamicOutputCount?: number
  legacyClass?: string
  legacyModule?: string
  legacyInputs?: Array<{ key: string; label: string; type: string }>
  legacyOutputs?: Array<{ key: string; label: string; type: string }>
}

export interface LegacyNodeSnapshot {
  id: string
  class: string
  module: string
  pos: number[]
  port_defaultv: Record<string, unknown>
}

export interface LegacyEdgeSnapshot {
  edge_id?: string
  source_node_id: string
  source_port_index?: number
  source_port_id?: number | string
  des_node_id: string
  des_port_index?: number
  des_port_id?: number | string
}

export interface LegacyGraphState {
  format?: 'vgf' | string
  time?: string
  hiddenNodes?: LegacyNodeSnapshot[]
  hiddenEdges?: LegacyEdgeSnapshot[]
  groups?: Array<{ title: string; nodes: string[] }>
  variables?: Array<Record<string, unknown>>
}

export interface NodeSnapshot {
  id: string
  typeId: string
  position: { x: number; y: number }
  values: Record<string, unknown>
  properties?: NodeProperties
}

export interface ConnectionSnapshot {
  source: string
  sourceOutput: string
  target: string
  targetInput: string
  entryConnectionVisible?: boolean
}

export interface GroupSnapshot {
  id: string
  title: string
  x: number
  y: number
  width: number
  height: number
  nodeIds: string[]
}

export interface GraphSnapshot {
  nodes: NodeSnapshot[]
  connections: ConnectionSnapshot[]
  groups: GroupSnapshot[]
}

export interface GraphDocument extends GraphSnapshot {
  schemaVersion: 1
  graphName: string
  variables: GraphVariable[]
  variableGroups: GraphVariableGroup[]
  functionSignature?: FunctionSignature
  view: { x: number; y: number; zoom: number }
  legacy?: LegacyGraphState
}

export interface ValidationIssue {
  severity: 'error' | 'warning'
  code: string
  message: string
  nodeId?: string
  nodeIds?: string[]
}
