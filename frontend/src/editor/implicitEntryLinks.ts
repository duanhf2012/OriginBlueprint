export interface EntryBindingPort {
  label?: string
  socket: string
}

export interface EntryBindingNode {
  id: string
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

function cleanLabel(value: string | undefined, fallback: string) {
  return String(value ?? '').trim() || fallback
}

function hashText(value: string) {
  let hash = 2166136261
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  hash ^= hash >>> 16
  hash = Math.imul(hash, 0x7feb352d)
  hash ^= hash >>> 15
  hash = Math.imul(hash, 0x846ca68b)
  hash ^= hash >>> 16
  return hash >>> 0
}

function hslToHex(hue: number, saturation: number, lightness: number) {
  const chroma = (1 - Math.abs(2 * lightness - 1)) * saturation
  const segment = hue / 60
  const second = chroma * (1 - Math.abs(segment % 2 - 1))
  const offset = lightness - chroma / 2
  const [red, green, blue] =
    segment < 1 ? [chroma, second, 0] :
    segment < 2 ? [second, chroma, 0] :
    segment < 3 ? [0, chroma, second] :
    segment < 4 ? [0, second, chroma] :
    segment < 5 ? [second, 0, chroma] :
    [chroma, 0, second]
  return [red, green, blue]
    .map(value => Math.round((value + offset) * 255).toString(16).padStart(2, '0'))
    .join('')
    .replace(/^/, '#')
}

export function entrySourceColor(sourceKey?: string) {
  const key = String(sourceKey ?? '').trim()
  if (!key) return ''
  const hash = hashText(key)
  const hue = hash % 360
  const saturation = 0.66 + ((hash >>> 9) % 18) / 100
  const lightness = 0.58 + ((hash >>> 17) % 12) / 100
  return hslToHex(hue, saturation, lightness)
}

function looksLikeEntryName(value: string | undefined) {
  const text = String(value ?? '').trim()
  const lower = text.toLowerCase()
  return lower.startsWith('entrance') || lower.includes('_entrance') || lower.includes('.entrance-') || lower.includes('entrance-') || text.endsWith('入口') || text.includes('入口(')
}

export function isEntryNode(node?: EntryBindingNode) {
  return Boolean(
    node?.typeId?.startsWith('origin.event.') ||
    node?.typeId?.startsWith('origin.entry.') ||
    looksLikeEntryName(node?.typeId) ||
    looksLikeEntryName(node?.legacyClass) ||
    looksLikeEntryName(node?.label)
  )
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
