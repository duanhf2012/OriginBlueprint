import type { BlueprintNode, NodePortVisualStates, Schemes } from './types'
import { describeEntryBinding, type EntryBindingNode } from './implicitEntryLinks'

type PortSide = keyof NodePortVisualStates
type ConnectionLike = Pick<Schemes['Connection'], 'source' | 'target' | 'sourceOutput' | 'targetInput'>

function controlHasValue(control: unknown) {
  const value = (control as { value?: unknown } | null | undefined)?.value
  if (value === null || value === undefined) return false
  if (Array.isArray(value)) return value.length > 0
  if (typeof value === 'string') return value.trim().length > 0
  if (typeof value === 'number') return !Number.isNaN(value)
  if (typeof value === 'boolean') return value
  if (typeof value === 'object') return Object.keys(value).length > 0
  return true
}

function createPortStates(node: BlueprintNode): NodePortVisualStates {
  const states: NodePortVisualStates = { inputs: {}, outputs: {} }
  for (const [key, port] of Object.entries(node.inputs)) {
    states.inputs[key] = { connected: false, filled: controlHasValue(port?.control) }
  }
  for (const key of Object.keys(node.outputs)) {
    states.outputs[key] = { connected: false, filled: false }
  }
  return states
}

function fillPort(node: BlueprintNode | undefined, side: PortSide, key: string) {
  const state = node?.portStates?.[side][key]
  if (state) {
    state.connected = true
    state.filled = true
  }
}

function entryBindingNode(node?: BlueprintNode): EntryBindingNode | undefined {
  if (!node) return undefined
  const inputs = Object.fromEntries(Object.entries(node.inputs).flatMap(([key, port]) => port ? [[key, { label: port.label, socket: port.socket.name }]] : []))
  const outputs = Object.fromEntries(Object.entries(node.outputs).flatMap(([key, port]) => port ? [[key, { label: port.label, socket: port.socket.name }]] : []))
  return { id: node.id, typeId: node.typeId, legacyClass: node.legacyClass, label: node.label, inputs, outputs }
}

export function refreshNodePortStates(
  nodes: BlueprintNode[],
  connections: ConnectionLike[],
  getNode: (id: string) => BlueprintNode | undefined
) {
  for (const node of nodes) node.portStates = createPortStates(node)

  for (const connection of connections) {
    fillPort(getNode(connection.source), 'outputs', String(connection.sourceOutput))
    fillPort(getNode(connection.target), 'inputs', String(connection.targetInput))
    const binding = describeEntryBinding({
      source: connection.source,
      sourceOutput: String(connection.sourceOutput),
      target: connection.target,
      targetInput: String(connection.targetInput)
    }, id => entryBindingNode(getNode(id)))
    if (binding) {
      const target = getNode(connection.target)
      const state = target?.portStates?.inputs[String(connection.targetInput)]
      if (state) state.entryBinding = binding
    }
  }
}
