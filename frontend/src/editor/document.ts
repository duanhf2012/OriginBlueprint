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
  view: { x: number; y: number; zoom: number }
}

export interface ValidationIssue {
  severity: 'error' | 'warning'
  code: string
  message: string
  nodeId?: string
}
