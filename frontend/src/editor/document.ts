export type VariableType = 'boolean' | 'integer' | 'float' | 'string' | 'array' | 'timerhandle'

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

export type FunctionNodeRole = 'call' | 'entry' | 'return' | 'timer'
export type FunctionNodeSource = 'current' | 'workspace'

export interface FunctionNodeMetadata {
  functionRole: FunctionNodeRole
  functionId: string
  functionName: string
  functionSource?: FunctionNodeSource
  functionSignature?: FunctionSignature
}

export interface NodeProperties {
  label?: string
  variableId?: string
  variableAccess?: 'get' | 'set'
  dynamicOutputCount?: number
  functionRole?: FunctionNodeRole
  functionId?: string
  functionName?: string
  functionSource?: FunctionNodeSource
  functionSignature?: FunctionSignature
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
  entryConnectionVisible?: boolean
}

export interface LegacyResidualNodeDefaults {
  class: string
  values: Record<string, unknown>
}

export interface LegacyGraphState {
  format?: 'vgf' | string
  time?: string
  hiddenNodes?: LegacyNodeSnapshot[]
  hiddenEdges?: LegacyEdgeSnapshot[]
  hiddenEdgeOrdinals?: number[]
  groups?: Array<{ title: string; nodes: string[] }>
  variables?: Array<Record<string, unknown>>
  residualNodeDefaults?: Record<string, LegacyResidualNodeDefaults>
  extraRootFields?: Record<string, unknown>
  extraNodeFields?: Record<string, { class: string; fields: Record<string, unknown> }>
  extraEdgeFields?: Record<string, Record<string, unknown>>
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
  legacyEdgeId?: string
  legacyOrdinal?: number
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

export interface RestoreDroppedNode {
  id: string
  typeId: string
  reason: 'missing-type-id' | 'unknown-node-type'
}

export interface RestoreDroppedConnection {
  source: string
  sourceOutput: string
  target: string
  targetInput: string
  reason: 'missing-endpoint' | 'missing-source-port' | 'missing-target-port'
}

export interface RestoreAlteredNode {
  id: string
  typeId: string
  reason: 'invalid-dynamic-output-count'
  originalValue: unknown
  restoredValue: number
}

export interface RestoreLossReport {
  droppedNodes: RestoreDroppedNode[]
  droppedConnections: RestoreDroppedConnection[]
  alteredNodes: RestoreAlteredNode[]
}

export interface GraphDocument extends GraphSnapshot {
  schemaVersion: 1
  graphName: string
  functionId?: string
  functionCategory?: string
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
  sourcePath?: string
  blocksSave?: boolean
  blocksRun?: boolean
  target?: string
}
