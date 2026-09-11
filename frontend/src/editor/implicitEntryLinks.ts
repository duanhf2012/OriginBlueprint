export interface EntryBindingPort {
  label?: string
  socket: string
}

export interface EntryBindingNode {
  id: string
  kind?: string
  typeId?: string
  legacyClass?: string
  label: string
  entrySourceKey?: string
  entrySourceColor?: string
  inputs?: Record<string, EntryBindingPort | undefined>
  outputs?: Record<string, EntryBindingPort | undefined>
}

export interface EntryBindingConnection {
  source: string
  sourceOutput: string
  target: string
  targetInput: string
}

export interface EntryPortBinding {
  sourceNodeId: string
  sourceNodeLabel: string
  sourceOutput: string
  sourceOutputLabel: string
  targetNodeId: string
  targetInput: string
  socket: string
  label: string
  entrySourceColor?: string
}

export interface EntryBindingCandidate {
  sourceNodeId: string
  sourceNodeLabel: string
  sourceOutput: string
  sourceOutputLabel: string
  socket: string
}

export interface EntryBindingCandidateGroup {
  sourceNodeId: string
  sourceNodeLabel: string
  candidates: EntryBindingCandidate[]
}

export interface EntryBindingMenuRect {
  left: number
  top: number
  width: number
  height: number
}

function cleanLabel(value: string | undefined, fallback: string) {
  return String(value ?? '').trim() || fallback
}

export const entrySourcePalette = [
  '#ef5350', '#42a5f5', '#ffee58', '#ab47bc', '#26c6da',
  '#ec407a', '#9ccc65', '#ff7043', '#5c6bc0', '#d4e157',
  '#f06292', '#29b6f6', '#ffca28', '#7e57c2', '#26a69a',
  '#e57373', '#66bb6a', '#ffa726', '#7986cb', '#c0ca33',
  '#d81b60', '#039be5', '#fdd835', '#8e24aa', '#00acc1',
  '#e53935', '#43a047', '#fb8c00', '#3949ab', '#7cb342',
  '#ff8a80', '#80d8ff', '#ffff8d', '#ea80fc', '#84ffff',
  '#ff80ab', '#b9f6ca', '#ffd180', '#8c9eff', '#ccff90',
  '#ff5252', '#40c4ff', '#ffea00', '#e040fb', '#18ffff',
  '#ff4081', '#69f0ae', '#ffab40', '#536dfe', '#b2ff59'
] as const

function hashText(value: string) {
  let hash = 2166136261
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return hash >>> 0
}

export function entrySourceColor(sourceKey?: string) {
  const key = String(sourceKey ?? '').trim()
  if (!key) return ''
  return entrySourcePalette[hashText(key) % entrySourcePalette.length]
}

function looksLikeEntryName(value: string | undefined) {
  const text = String(value ?? '').trim()
  const lower = text.toLowerCase()
  return lower.startsWith('entrance') || lower.includes('_entrance') || lower.includes('.entrance-') || lower.includes('entrance-') || text.endsWith('入口') || text.includes('入口(')
}

export function isEntryNode(node?: EntryBindingNode) {
  return Boolean(
    node?.kind === 'event' ||
    node?.entrySourceKey ||
    node?.typeId === 'origin.function.entry' ||
    node?.typeId?.startsWith('origin.event.') ||
    node?.typeId?.startsWith('origin.entry.') ||
    looksLikeEntryName(node?.typeId) ||
    looksLikeEntryName(node?.legacyClass) ||
    looksLikeEntryName(node?.label)
  )
}

export function entrySourceColorAssignments(nodes: EntryBindingNode[]) {
  const result = new Map<string, string>()
  const entries = nodes
    .filter(isEntryNode)
    .sort((left, right) => left.id < right.id ? -1 : left.id > right.id ? 1 : 0)
  entries.forEach((node, index) => result.set(node.id, entrySourcePalette[index % entrySourcePalette.length]))
  return result
}

export function entryBindingMenuPosition(
  clientX: number,
  clientY: number,
  container: EntryBindingMenuRect,
  menuWidth: number,
  menuHeight: number,
  margin = 6
) {
  return {
    left: Math.max(margin, Math.min(clientX - container.left, container.width - menuWidth - margin)),
    top: Math.max(margin, Math.min(clientY - container.top, container.height - menuHeight - margin))
  }
}

export function socketsCompatible(sourceSocket: string | undefined, targetSocket: string | undefined) {
  if (!sourceSocket || !targetSocket) return false
  if (sourceSocket === 'exec' || targetSocket === 'exec') return false
  return sourceSocket === targetSocket || sourceSocket === 'any' || targetSocket === 'any'
}

export function describeEntryBinding(
  connection: EntryBindingConnection,
  getNode: (id: string) => EntryBindingNode | undefined
): EntryPortBinding | undefined {
  const source = getNode(connection.source)
  const target = getNode(connection.target)
  if (!isEntryNode(source) || !target) return undefined

  const output = source?.outputs?.[connection.sourceOutput]
  const input = target.inputs?.[connection.targetInput]
  if (!output || !input || !socketsCompatible(output.socket, input.socket)) return undefined

  const sourceNodeLabel = cleanLabel(source?.label, '入口')
  const sourceOutputLabel = cleanLabel(output.label, connection.sourceOutput)
  const sourceColor = source.entrySourceColor || entrySourceColor(source.entrySourceKey || source.typeId || source.label)
  return {
    sourceNodeId: connection.source,
    sourceNodeLabel,
    sourceOutput: connection.sourceOutput,
    sourceOutputLabel,
    targetNodeId: connection.target,
    targetInput: connection.targetInput,
    socket: output.socket,
    label: sourceOutputLabel,
    entrySourceColor: sourceColor
  }
}

export function isEntryOutputConnection(
  connection: EntryBindingConnection,
  getNode: (id: string) => EntryBindingNode | undefined
) {
  return Boolean(describeEntryBinding(connection, getNode))
}

export function entryBindingCandidateGroups(
  targetNodeId: string,
  inputKey: string,
  nodes: EntryBindingNode[]
): EntryBindingCandidateGroup[] {
  const target = nodes.find(node => node.id === targetNodeId)
  const input = target?.inputs?.[inputKey]
  if (!target || !input || input.socket === 'exec') return []

  return nodes.flatMap(node => {
    if (node.id === targetNodeId || !isEntryNode(node)) return []
    const candidates = Object.entries(node.outputs ?? {}).flatMap(([outputKey, output]) => {
      if (!output || !socketsCompatible(output.socket, input.socket)) return []
      return [{
        sourceNodeId: node.id,
        sourceNodeLabel: cleanLabel(node.label, node.id),
        sourceOutput: outputKey,
        sourceOutputLabel: cleanLabel(output.label, outputKey),
        socket: output.socket
      }]
    })
    return candidates.length ? [{ sourceNodeId: node.id, sourceNodeLabel: cleanLabel(node.label, node.id), candidates }] : []
  })
}

export function entryBindingLabel(binding?: EntryPortBinding) {
  return binding ? binding.sourceOutputLabel : ''
}

export function entryBindingBadgeLabel(binding?: EntryPortBinding) {
  return entryBindingLabel(binding)
}

export function entryBindingTitle(binding?: EntryPortBinding) {
  return binding ? `${binding.sourceNodeLabel}/${binding.sourceOutputLabel}` : ''
}
