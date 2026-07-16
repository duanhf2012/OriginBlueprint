import type {
  ConnectionSnapshot,
  GraphSnapshot,
  NodeSnapshot,
  RestoreAlteredNode,
  RestoreLossReport,
} from './document'

export interface PreparedRestoreNode<T> {
  snapshot: NodeSnapshot
  node: T
  inputKeys: readonly string[]
  outputKeys: readonly string[]
  alteredNodes?: readonly RestoreAlteredNode[]
}

export interface RestorePlan<T> {
  nodes: PreparedRestoreNode<T>[]
  connections: ConnectionSnapshot[]
  report: RestoreLossReport
}

export function normalizeDynamicOutputCount(requested: number) {
  if (requested === 0 || !Number.isFinite(requested)) return 3
  return Math.max(1, Math.min(256, Math.floor(requested)))
}

export function buildRestorePlan<T>(
  snapshot: GraphSnapshot,
  prepare: (node: NodeSnapshot, typeId: string) => PreparedRestoreNode<T> | null,
): RestorePlan<T> {
  const report: RestoreLossReport = { droppedNodes: [], droppedConnections: [], alteredNodes: [] }
  const nodes: PreparedRestoreNode<T>[] = []
  const nodesById = new Map<string, PreparedRestoreNode<T>>()

  for (const item of snapshot.nodes) {
    const typeId = typeof item.typeId === 'string' ? item.typeId : ''
    if (!typeId) {
      report.droppedNodes.push({ id: item.id, typeId: '', reason: 'missing-type-id' })
      continue
    }
    const prepared = prepare(item, typeId)
    if (!prepared) {
      report.droppedNodes.push({ id: item.id, typeId, reason: 'unknown-node-type' })
      continue
    }
    nodes.push(prepared)
    nodesById.set(item.id, prepared)
    report.alteredNodes.push(...(prepared.alteredNodes ?? []))
  }

  const connections: ConnectionSnapshot[] = []
  for (const item of snapshot.connections) {
    const source = nodesById.get(item.source)
    const target = nodesById.get(item.target)
    if (!source || !target) {
      report.droppedConnections.push({ ...item, reason: 'missing-endpoint' })
      continue
    }
    if (!source.outputKeys.includes(item.sourceOutput)) {
      report.droppedConnections.push({ ...item, reason: 'missing-source-port' })
      continue
    }
    if (!target.inputKeys.includes(item.targetInput)) {
      report.droppedConnections.push({ ...item, reason: 'missing-target-port' })
      continue
    }
    connections.push(item)
  }

  return { nodes, connections, report }
}
