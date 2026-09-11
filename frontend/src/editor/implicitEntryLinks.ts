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

// 入口身份色调色板：50 个候选色（25 色相 × 2 档亮度，S=0.82、L=0.54/0.68）在 CIELAB 空间
// 做最远点排序后的结果——排在前面的颜色彼此区分度最大，实际项目入口数远小于 50，
// 按 rank 取色即可保证同图内入口两两区分明显。红色系（与事件节点的红色标题栏同族，
// 菱形图标衬在红条上辨识度差）统一延后到第 42 位起，只有入口数超过 42 时才会用到。
export const entrySourcePalette = [
  '#39ea2a', '#392aea', '#2ad3ea', '#ea2a86', '#eab42a',
  '#6a80f0', '#2aeaa5', '#ea2ae2', '#d6f06a', '#2aa5ea',
  '#f06acb', '#b56af0', '#2aead3', '#75f06a', '#eae22a',
  '#95ea2a', '#f0cb6a', '#ea2ab4', '#952aea', '#2a48ea',
  '#756af0', '#f06aab', '#c3ea2a', '#f06aeb', '#6ac0f0',
  '#c32aea', '#6af0c0', '#2aea76', '#6aa0f0', '#b5f06a',
  '#f0eb6a', '#d66af0', '#6af080', '#2aea48', '#6af0a0',
  '#6af0e0', '#2a76ea', '#95f06a', '#67ea2a', '#956af0',
  '#6ae0f0', '#672aea', '#ea582a', '#f08b6a', '#ea2a58',
  '#ea862a', '#f0ab6a', '#f06a6a', '#f06a8b', '#ea2a2a',
]

// 调色板分配结果：入口 key 按稳定排序后，第 i 个入口使用 entrySourcePalette[i % 50]。
// 分配必须是确定性的：同一组入口 key 在任何会话中得到完全一致的颜色。
let paletteAssignment = new Map<string, string>()

export function assignEntrySourcePalette(keys: string[]) {
  const sorted = [...new Set(keys.map(key => String(key ?? '').trim()).filter(Boolean))].sort()
  const assigned = new Map<string, string>()
  sorted.forEach((key, index) => assigned.set(key, entrySourcePalette[index % entrySourcePalette.length]))
  paletteAssignment = assigned
}

export function entrySourceColor(sourceKey?: string) {
  const key = String(sourceKey ?? '').trim()
  if (!key) return ''
  const assigned = paletteAssignment.get(key)
  if (assigned) return assigned
  return hashEntryColor(key)
}

// 未经批量分配的 key（函数入口、legacy 占位入口等）继续用全量哈希取色，保持稳定身份色。
function hashEntryColor(key: string) {
  const hash = hashText(key)
  const hue = hash % 360
  const saturation = 0.66 + ((hash >>> 9) % 18) / 100
  const lightness = 0.58 + ((hash >>> 17) % 12) / 100
  return hslToHex(hue, saturation, lightness)
}

export function looksLikeEntryName(value: string | undefined) {
  const text = String(value ?? '').trim()
  const lower = text.toLowerCase()
  return lower.startsWith('entrance') || lower.includes('_entrance') || lower.includes('.entrance-') || lower.includes('entrance-') || text.endsWith('入口') || text.includes('入口(')
}

export function isEntryNode(node?: EntryBindingNode) {
  return Boolean(
    node?.typeId === 'origin.function.entry' ||
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
