import { ClassicPreset, NodeEditor, type Scope } from 'rete'
import { AreaExtensions, AreaPlugin, Drag } from 'rete-area-plugin'
import { ConnectionPlugin, Presets as ConnectionPresets } from 'rete-connection-plugin'
import { getDOMSocketPosition, type SocketPositionWatcher } from 'rete-render-utils'
import { Presets as VuePresets, VuePlugin } from 'rete-vue-plugin'
import BlueprintControl from './BlueprintControl.vue'
import BlueprintConnectionComponent from './BlueprintConnection.vue'
import BlueprintNodeComponent from './BlueprintNode.vue'
import BlueprintSocket from './BlueprintSocket.vue'
import { createFunctionCallNode, createFunctionEntryNode as createFunctionEntryNodeFromSpec, createFunctionReturnNode as createFunctionReturnNodeFromSpec, createLegacyNode, createNode, createVariableNode, hasNodeDefinition, nodeTitleWidth } from './nodeRegistry'
import { normalizeSocketName } from './socketTheme'
import { BlueprintNode, type Schemes } from './types'
import { describeEntryBinding, entryBindingCandidateGroups, isEntryOutputConnection, type EntryBindingNode } from './implicitEntryLinks'
import { refreshNodePortStates } from './portVisualState'
import { pathIntersectsRect, rectsIntersect, type Rect } from './selectionGeometry'
import type { ConnectionSnapshot, FunctionNodeMetadata, FunctionSignature, GraphDocument, GraphSnapshot, GraphVariable, GraphVariableGroup, GroupSnapshot, LegacyGraphState, NodeProperties, NodeSnapshot } from './document'

export type { FunctionSignature, FunctionSignaturePort, GraphDocument, GraphVariable, GraphVariableGroup, ValidationIssue, VariableType } from './document'

type AreaExtra = import('rete-vue-plugin').VueArea2D<Schemes>
type Position = { x: number; y: number }
type SocketWatcher = SocketPositionWatcher<Scope<never, [AreaExtra]>>
const nodeLocateZoomScale = 0.48
const nodeLocateMinZoomScale = 0.34
const nodeLocateViewportAnchor = { x: 0.5, y: 0.28 }
const issueHighlightZoomScale = nodeLocateZoomScale

interface ClipboardGraph {
  nodes: Omit<NodeSnapshot, 'id'>[]
  connections: Array<Omit<ConnectionSnapshot, 'source' | 'target'> & { sourceIndex: number; targetIndex: number }>
}

type SnapshotPort = ClassicPreset.Input<ClassicPreset.Socket> | ClassicPreset.Output<ClassicPreset.Socket> | undefined

function createFrameSocketPositionWatcher(): SocketWatcher {
  const base = getDOMSocketPosition<Schemes, AreaExtra>()
  const pending = new Set<{ active: boolean; latest: Position | null; emit: (position: Position) => void }>()
  let frame = 0

  function flush() {
    frame = 0
    const entries = [...pending]
    pending.clear()
    for (const entry of entries) {
      if (!entry.active || !entry.latest) continue
      const latest = entry.latest
      entry.latest = null
      entry.emit(latest)
    }
  }

  function schedule(entry: { active: boolean; latest: Position | null; emit: (position: Position) => void }, position: Position) {
    entry.latest = position
    pending.add(entry)
    if (!frame) frame = requestAnimationFrame(flush)
  }

  return {
    attach(scope) {
      base.attach(scope)
    },
    listen(nodeId, side, key, onChange) {
      const entry = { active: true, latest: null as Position | null, emit: onChange }
      const unlisten = base.listen(nodeId, side, key, position => schedule(entry, position))
      return () => {
        entry.active = false
        entry.latest = null
        pending.delete(entry)
        unlisten()
        if (!pending.size && frame) {
          cancelAnimationFrame(frame)
          frame = 0
        }
      }
    }
  }
}

export interface EditorMetrics {
  nodes: number
  connections: number
}

export interface SelectedNodeInfo {
  id: string
  typeId: string
  label: string
  description?: string
  values: Record<string, unknown>
  variableId?: string
}

export interface AddNodeOptions {
  allowEntryNodes?: boolean
}

export interface BlueprintEditorHandle {
  destroy(): void
  resetView(): void
  addNode(typeId: string, clientPosition?: Position, options?: AddNodeOptions): Promise<void>
  addFunctionCallNode(spec: FunctionNodeMetadata, clientPosition?: Position): Promise<void>
  addFunctionEntryNode(spec: FunctionNodeMetadata, clientPosition?: Position): Promise<void>
  addFunctionReturnNode(spec: FunctionNodeMetadata, clientPosition?: Position): Promise<void>
  syncFunctionSignature(spec: FunctionNodeMetadata): Promise<void>
  addVariableNode(variable: GraphVariable, access: 'get' | 'set', clientPosition?: Position): Promise<void>
  deleteSelected(): Promise<void>
  selectAll(): Promise<void>
  deselectAll(): Promise<void>
  copy(): void
  cut(): Promise<void>
  paste(): Promise<void>
  undo(): Promise<void>
  redo(): Promise<void>
  getDocument(graphName?: string, variables?: GraphVariable[], variableGroups?: GraphVariableGroup[]): GraphDocument
  loadDocument(document: GraphDocument): Promise<void>
  newDocument(): Promise<void>
  align(mode: 'horizontal-center' | 'vertical-center' | 'left' | 'right' | 'top' | 'bottom' | 'horizontal-distribute' | 'vertical-distribute' | 'straighten'): Promise<void>
  groupSelected(): Promise<void>
  ungroupSelected(): Promise<void>
  toggleGroupSelected(): Promise<void>
  fitSelected(): Promise<void>
  setVariables(variables: GraphVariable[], variableGroups?: GraphVariableGroup[], refreshNodes?: boolean): Promise<void>
  updateSelectedNode(label: string, values: Record<string, unknown>): Promise<void>
  focusNode(id: string): Promise<void>
  highlightNodesByType(typeId: string): Promise<number>
  highlightIssueNode(id: string): Promise<number>
  highlightIssueNodes(ids: string[]): Promise<number>
}

interface Callbacks {
  onZoom(value: number): void
  onStatus(value: string): void
  onMetrics(metrics: EditorMetrics): void
  onDirty(): void
  onFunctionSignature(value: FunctionSignature): void
  onVariables(variables: GraphVariable[]): void
  onVariableGroups(groups: GraphVariableGroup[]): void
  onSelection(node: SelectedNodeInfo | null): void
  canAddEntryNodes?(): boolean
}

function controlValues(node: BlueprintNode) {
  const values: Record<string, unknown> = {}
  for (const [key, port] of Object.entries(node.inputs)) {
    const control = port?.control as ClassicPreset.InputControl<'text' | 'number'> | null | undefined
    if (control) values[key] = control.value
  }
  return values
}

function setControlValues(node: BlueprintNode, values: Record<string, unknown>) {
  for (const [key, value] of Object.entries(values)) {
    const control = node.inputs[key]?.control as ClassicPreset.InputControl<'text' | 'number'> | null | undefined
    if (control) control.setValue(value as never)
  }
}

function dynamicBranchValueCount(node: BlueprintNode) {
  const key = node.dynamicBranch?.controlInput
  const control = key ? node.inputs[key]?.control as { value?: unknown } | undefined : undefined
  const value = control?.value
  return Array.isArray(value) ? value.length : 0
}

function dynamicBranchOutputSocket(node: BlueprintNode) {
  const config = node.dynamicBranch
  const template = config?.outputTemplate
  const socketName = template?.type === 'data' ? template.data_type : template?.type
  return new ClassicPreset.Socket(socketName ?? node.outputs[config?.defaultOutput ?? '']?.socket.name ?? 'exec')
}

function syncDynamicBranchOutputs(node: BlueprintNode, requestedCount: number) {
  const config = node.dynamicBranch
  if (!config) return
  const count = Math.max(0, Math.min(config.maxBranches, Math.floor(requestedCount)))
  const first = config.outputStartIndex
  const last = first + count - 1
  const socket = dynamicBranchOutputSocket(node)
  for (let index = first; index <= last; index++) {
    const outputKey = `${config.outputPrefix}${index}`
    if (!node.outputs[outputKey]) node.addOutput(outputKey, new ClassicPreset.Output(socket, config.outputTemplate?.label ?? ''))
  }
  for (const key of Object.keys(node.outputs)) {
    if (!key.startsWith(config.outputPrefix)) continue
    const index = Number(key.slice(config.outputPrefix.length))
    if (Number.isFinite(index) && index >= first && index > last) node.removeOutput(key)
  }
}

function nextFrame() {
  return new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
}

export async function createBlueprintEditor(container: HTMLElement, callbacks: Callbacks): Promise<BlueprintEditorHandle> {
  const editor = new NodeEditor<Schemes>()
  const area = new AreaPlugin<Schemes, AreaExtra>(container)
  const connection = new ConnectionPlugin<Schemes, AreaExtra>()
  const render = new VuePlugin<Schemes, AreaExtra>()
  const selector = AreaExtensions.selector()
  const selectable = AreaExtensions.selectableNodes(area, selector, { accumulating: AreaExtensions.accumulateOnCtrl() })
  const undoStack: GraphSnapshot[] = []
  const redoStack: GraphSnapshot[] = []
  const groups: GroupSnapshot[] = []
  const groupElements = new Map<string, HTMLElement>()
  const selectedConnectionIds = new Set<string>()
  let selectedGroupId: string | null = null
  let preservedMultiNodeSelection: string[] = []
  let dragSnapshot: GraphSnapshot | null = null
  let clipboard: ClipboardGraph | null = null
  let restoring = false
  let transactionActive = false
  let initializing = true
  let pendingConnectionSnapshot: GraphSnapshot | null = null
  let currentVariables: GraphVariable[] = []
  let currentVariableGroups: GraphVariableGroup[] = []
  let currentLegacy: LegacyGraphState | undefined
  let insertionOffset = 0
  const visibleEntryConnectionIds = new Set<string>()

  render.addPreset(VuePresets.classic.setup({
    socketPositionWatcher: createFrameSocketPositionWatcher(),
    customize: {
      node: () => BlueprintNodeComponent,
      connection: () => BlueprintConnectionComponent,
      socket: () => BlueprintSocket,
      control: () => BlueprintControl
    }
  }))
  connection.addPreset(ConnectionPresets.classic.setup())
  editor.use(area)
  area.use(connection)
  area.use(render)
  AreaExtensions.simpleNodesOrder(area)
  area.area.content.holder.classList.add('blueprint-area-content')

  // Match graph editors like Unreal: right-drag or middle-drag pans the empty canvas.
  area.area.setDragHandler(new Drag({
    down: canStartCanvasPan,
    move: () => true
  }))

  function canStartCanvasPan(event: PointerEvent) {
    if (event.pointerType !== 'mouse') return true
    if (event.button === 1) return true
    if (event.button !== 2 || event.ctrlKey) return false
    const target = event.target as HTMLElement
    return !target.closest('.blueprint-node, .blueprint-socket, .blueprint-connection, .node-group, input, textarea, select, button')
  }

  function setInteractionClass(name: string, active: boolean) {
    container.classList.toggle(name, active)
    container.classList.toggle('is-interacting', container.classList.contains('is-panning') || container.classList.contains('is-dragging-node'))
  }

  function setupCanvasPanFeedback() {
    const stop = () => setInteractionClass('is-panning', false)
    const down = (event: PointerEvent) => {
      if (!canStartCanvasPan(event)) return
      setInteractionClass('is-panning', true)
      window.addEventListener('pointerup', stop, { once: true })
      window.addEventListener('pointercancel', stop, { once: true })
    }
    container.addEventListener('pointerdown', down)
    return () => {
      container.removeEventListener('pointerdown', down)
      window.removeEventListener('pointerup', stop)
      window.removeEventListener('pointercancel', stop)
    }
  }

  function setupMultiSelectionDragPreserver() {
    const rememberSelection = (event: PointerEvent) => {
      if (event.button !== 0) return
      const target = event.target as HTMLElement
      if (!target.closest('.blueprint-node')) {
        preservedMultiNodeSelection = []
        return
      }
      const pickedNodeId = nodeIdFromEventTarget(target)
      const ids = selectedNodes().map(node => node.id)
      preservedMultiNodeSelection = pickedNodeId && ids.length > 1 && ids.includes(pickedNodeId) ? ids : []
    }
    container.addEventListener('pointerdown', rememberSelection, true)
    return () => container.removeEventListener('pointerdown', rememberSelection, true)
  }

  function nodeIdFromEventTarget(target: HTMLElement) {
    for (const [id, view] of area.nodeViews) {
      if (view.element.contains(target)) return id
    }
    return ''
  }

  async function restoreMultiSelectionAfterNodePick(pickedId: string) {
    const ids = preservedMultiNodeSelection
    preservedMultiNodeSelection = []
    if (ids.length < 2 || !ids.includes(pickedId)) return
    for (const id of ids) {
      if (editor.getNode(id)) await selectable.select(id, true)
    }
    callbacks.onStatus(`Selected ${ids.length} node(s)`)
  }

  const stopNodeDragFeedback = () => setInteractionClass('is-dragging-node', false)

  function startNodeDragFeedback() {
    setInteractionClass('is-dragging-node', true)
    window.addEventListener('pointerup', stopNodeDragFeedback, { once: true })
    window.addEventListener('pointercancel', stopNodeDragFeedback, { once: true })
  }

  function updateMetrics() {
    callbacks.onMetrics({ nodes: editor.getNodes().length, connections: editor.getConnections().length })
  }

  async function clearConnectionSelection(exceptId?: string) {
    for (const id of [...selectedConnectionIds]) {
      if (id === exceptId) continue
      selectedConnectionIds.delete(id)
      const item = editor.getConnection(id)
      if (item) { item.selected = false; await area.update('connection', id) }
    }
  }

  async function selectConnection(id: string, additive: boolean) {
    const item = editor.getConnection(id)
    if (!item) return
    if (!additive) await clearConnectionSelection(id)
    const selected = additive ? !selectedConnectionIds.has(id) : true
    item.selected = selected
    if (selected) selectedConnectionIds.add(id); else selectedConnectionIds.delete(id)
    await area.update('connection', id)
    await selector.unselectAll()
    selectedGroupId = null; renderGroups(); callbacks.onSelection(null)
    callbacks.onStatus(selected ? 'Connection selected' : 'Connection deselected')
  }

  async function selectConnections(ids: string[], additive: boolean) {
    if (!additive) await clearConnectionSelection()
    let selected = 0
    for (const id of ids) {
      const item = editor.getConnection(id)
      if (!item || selectedConnectionIds.has(id)) continue
      item.selected = true
      selectedConnectionIds.add(id)
      selected++
      await area.update('connection', id)
    }
    return selected
  }

  function graphPosition(clientPosition?: Position): Position {
    if (!clientPosition) {
      const rect = container.getBoundingClientRect()
      const offset = insertionOffset % 8 * 24
      insertionOffset++
      clientPosition = { x: rect.left + rect.width / 2 + offset, y: rect.top + rect.height / 2 + offset }
    }
    const rect = container.getBoundingClientRect()
    const transform = area.area.transform
    return {
      x: (clientPosition.x - rect.left - transform.x) / transform.k,
      y: (clientPosition.y - rect.top - transform.y) / transform.k
    }
  }

  function cloneLegacyState(value?: LegacyGraphState): LegacyGraphState | undefined {
    return value ? JSON.parse(JSON.stringify(value)) as LegacyGraphState : undefined
  }

  function cloneFunctionSignatureFromProperties(signature?: Partial<FunctionSignature>): FunctionSignature | undefined {
    if (!signature) return undefined
    const inputs = Array.isArray(signature.inputs) ? signature.inputs.map(port => ({ ...port })) : []
    const outputs = Array.isArray(signature.outputs) ? signature.outputs.map(port => ({ ...port })) : []
    return inputs.length || outputs.length ? { inputs, outputs } : undefined
  }

  function emitFunctionSignatureFromSnapshot(data: GraphSnapshot) {
    for (const node of data.nodes) {
      if (node.typeId !== 'origin.function.entry' && node.typeId !== 'origin.function.return') continue
      const signature = node.properties?.functionSignature
      const inputs = signature?.inputs
      const outputs = signature?.outputs
      callbacks.onFunctionSignature({
        inputs: Array.isArray(inputs) ? inputs.map(port => ({ ...port })) : [],
        outputs: Array.isArray(outputs) ? outputs.map(port => ({ ...port })) : []
      })
      return
    }
  }

  function applyNodeProperties(node: BlueprintNode, properties?: NodeProperties) {
    node.functionRole = properties?.functionRole
    node.functionId = properties?.functionId
    node.functionName = properties?.functionName
    node.functionSource = properties?.functionSource
    node.functionSignature = cloneFunctionSignatureFromProperties(properties?.functionSignature)
    node.legacyClass = properties?.legacyClass
    node.legacyModule = properties?.legacyModule
    node.legacyInputs = properties?.legacyInputs?.map(port => ({ ...port }))
    node.legacyOutputs = properties?.legacyOutputs?.map(port => ({ ...port }))
  }

  function functionMetadataFromProperties(properties?: NodeProperties): FunctionNodeMetadata {
    return {
      functionRole: properties?.functionRole ?? 'call',
      functionId: properties?.functionId ?? properties?.functionName ?? 'function',
      functionName: properties?.functionName ?? properties?.label ?? 'Function',
      functionSource: properties?.functionSource,
      functionSignature: properties?.functionSignature
    }
  }

  function createFunctionNodeFromProperties(properties?: NodeProperties) {
    const metadata = functionMetadataFromProperties(properties)
    if (metadata.functionRole === 'entry') return createFunctionEntryNodeFromSpec(metadata)
    if (metadata.functionRole === 'return') return createFunctionReturnNodeFromSpec(metadata)
    return createFunctionCallNode(metadata)
  }

  function createRestoredNode(item: Pick<NodeSnapshot, 'typeId' | 'properties'>, typeId: string) {
    const variableAccess = item.properties?.variableAccess ?? (typeId === 'origin.variable.set' ? 'set' : 'get')
    const variable = currentVariables.find(entry => entry.id === item.properties?.variableId)
    if (typeId.startsWith('origin.variable.')) {
      return createVariableNode(
        variable ?? { id: item.properties?.variableId ?? '', name: 'Missing Variable', type: 'string', defaultValue: '', groupId: 'default' },
        variableAccess
      )
    }
    if (typeId.startsWith('origin.function.')) return createFunctionNodeFromProperties(item.properties)
    if (typeId === 'origin.legacy.placeholder') return createLegacyNode(item.properties ?? {})
    if (hasNodeDefinition(typeId)) return createNode(typeId)
    if (item.properties?.legacyClass) return createLegacyNode(item.properties)
    return null
  }

  function functionPortKey(prefix: 'input' | 'output', port: NonNullable<FunctionSignature['inputs']>[number], index: number) {
    const key = String(port.id || port.name || `${index + 1}`).trim().replace(/[^a-zA-Z0-9_-]+/g, '-').replace(/^-+|-+$/g, '')
    return `${prefix}_${key || index + 1}`
  }

  function functionNodePortsFromSnapshot(node: NodeSnapshot) {
    const signature = cloneFunctionSignatureFromProperties(node.properties?.functionSignature) ?? { inputs: [], outputs: [] }
    const inputs = new Set<string>()
    const outputs = new Set<string>()
    if (node.typeId === 'origin.function.entry') {
      outputs.add('exec')
      signature.inputs.forEach((port, index) => outputs.add(functionPortKey('input', port, index)))
    } else if (node.typeId === 'origin.function.return') {
      inputs.add('exec')
      signature.outputs.forEach((port, index) => inputs.add(functionPortKey('output', port, index)))
    } else if (node.typeId === 'origin.function.call') {
      inputs.add('exec')
      outputs.add('exec')
      signature.inputs.forEach((port, index) => inputs.add(functionPortKey('input', port, index)))
      signature.outputs.forEach((port, index) => outputs.add(functionPortKey('output', port, index)))
    }
    return { inputs, outputs }
  }

  function pruneFunctionSignatureConnections(data: GraphSnapshot, changedNodeIds: Set<string>) {
    const portsByNode = new Map(data.nodes.filter(node => changedNodeIds.has(node.id)).map(node => [node.id, functionNodePortsFromSnapshot(node)]))
    return data.connections.filter(connection => {
      const sourcePorts = portsByNode.get(connection.source)
      if (sourcePorts && !sourcePorts.outputs.has(connection.sourceOutput)) return false
      const targetPorts = portsByNode.get(connection.target)
      if (targetPorts && !targetPorts.inputs.has(connection.targetInput)) return false
      return true
    })
  }

  function legacyPortType(port?: SnapshotPort) {
    const type = normalizeSocketName(port?.socket?.name)
    return type === 'number' ? 'integer' : type
  }

  function legacyPortsFromNodePorts(ports: Record<string, SnapshotPort>) {
    return Object.entries(ports).map(([key, port]) => ({
      key,
      label: String(port?.label ?? ''),
      type: legacyPortType(port)
    }))
  }

  function shouldSnapshotLegacyPorts(node: BlueprintNode) {
    return Boolean(node.legacyClass) || String(node.typeId ?? '').startsWith('origin.custom.')
  }

  function legacyInputsForSnapshot(node: BlueprintNode) {
    return node.legacyInputs ?? (shouldSnapshotLegacyPorts(node) ? legacyPortsFromNodePorts(node.inputs) : undefined)
  }

  function legacyOutputsForSnapshot(node: BlueprintNode) {
    return node.legacyOutputs ?? (shouldSnapshotLegacyPorts(node) ? legacyPortsFromNodePorts(node.outputs) : undefined)
  }

  function functionPropertiesForSnapshot(node: BlueprintNode): Pick<NodeProperties, 'functionRole' | 'functionId' | 'functionName' | 'functionSource' | 'functionSignature'> {
    return {
      functionRole: node.functionRole,
      functionId: node.functionId,
      functionName: node.functionName,
      functionSource: node.functionSource,
      functionSignature: node.functionSignature
    }
  }

  function snapshot(): GraphSnapshot {
    return {
      nodes: editor.getNodes().map(node => ({
        id: node.id,
        typeId: node.typeId ?? '',
        position: { ...(area.nodeViews.get(node.id)?.position ?? { x: 0, y: 0 }) },
        values: controlValues(node),
        properties: {
          label: node.label,
          variableId: node.variableId,
          variableAccess: node.variableAccess,
          dynamicOutputCount: node.dynamicOutputCount,
          ...functionPropertiesForSnapshot(node),
          legacyClass: node.legacyClass,
          legacyModule: node.legacyModule,
          legacyInputs: legacyInputsForSnapshot(node),
          legacyOutputs: legacyOutputsForSnapshot(node)
        }
      })),
      connections: editor.getConnections().map(item => ({
        source: item.source,
        sourceOutput: String(item.sourceOutput),
        target: item.target,
        targetInput: String(item.targetInput),
        ...(visibleEntryConnectionIds.has(item.id) ? { entryConnectionVisible: true } : {})
      })),
      groups: groups.map(item => ({ ...item, nodeIds: [...item.nodeIds] }))
    }
  }

  function renderGroups() {
    for (const element of groupElements.values()) element.remove()
    groupElements.clear()
    for (const group of groups) {
      const element = document.createElement('div')
      element.className = `node-group${selectedGroupId === group.id ? ' selected' : ''}`
      element.style.width = `${group.width}px`
      element.style.height = `${group.height}px`
      element.style.transform = `translate(${group.x}px, ${group.y}px)`
      element.innerHTML = `<div class="node-group-title">${group.title}</div><div class="node-group-resize"></div>`
      area.area.content.holder.prepend(element)
      groupElements.set(group.id, element)

      element.addEventListener('pointerdown', event => {
        void clearConnectionSelection()
        selectedGroupId = group.id
        renderGroups()
        event.stopPropagation()
      })
      const title = element.querySelector('.node-group-title') as HTMLElement
      title.ondblclick = event => {
        event.stopPropagation()
        const next = window.prompt('Group title', group.title)
        if (next?.trim()) { group.title = next.trim(); renderGroups(); callbacks.onDirty() }
      }
      title.onpointerdown = event => beginGroupDrag(event, group, false)
      const grip = element.querySelector('.node-group-resize') as HTMLElement
      grip.onpointerdown = event => beginGroupDrag(event, group, true)
    }
  }

  function beginGroupDrag(event: PointerEvent, group: GroupSnapshot, resize: boolean) {
    event.stopPropagation(); event.preventDefault()
    const before = snapshot()
    const start = { x: event.clientX, y: event.clientY, gx: group.x, gy: group.y, width: group.width, height: group.height }
    const nodeStarts = new Map(group.nodeIds.map(id => [id, { ...(area.nodeViews.get(id)?.position ?? { x: 0, y: 0 }) }]))
    const move = (next: PointerEvent) => {
      const dx = (next.clientX - start.x) / area.area.transform.k
      const dy = (next.clientY - start.y) / area.area.transform.k
      if (resize) {
        group.width = Math.max(160, start.width + dx); group.height = Math.max(100, start.height + dy)
      } else {
        group.x = start.gx + dx; group.y = start.gy + dy
        for (const [id, position] of nodeStarts) void area.translate(id, { x: position.x + dx, y: position.y + dy })
      }
      const element = groupElements.get(group.id)
      if (element) { element.style.width = `${group.width}px`; element.style.height = `${group.height}px`; element.style.transform = `translate(${group.x}px, ${group.y}px)` }
    }
    const up = () => {
      window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up)
      undoStack.push(before); redoStack.length = 0; callbacks.onDirty(); callbacks.onStatus(resize ? 'Group resized' : 'Group moved')
    }
    window.addEventListener('pointermove', move); window.addEventListener('pointerup', up)
  }

  async function restore(data: GraphSnapshot) {
    restoring = true
    visibleEntryConnectionIds.clear()
    selectedConnectionIds.clear()
    await selector.unselectAll()
    await editor.clear()
    groups.splice(0, groups.length, ...(data.groups ?? []).map(item => ({ ...item, nodeIds: [...item.nodeIds] })))
    const nodes = new Map<string, BlueprintNode>()
    for (const item of data.nodes) {
      const typeId = typeof item.typeId === 'string' ? item.typeId : ''
      if (!typeId) continue
      const node = createRestoredNode(item, typeId)
      if (!node) continue
      applyNodeProperties(node, item.properties)
      if (node.dynamicOutputs) setDynamicOutputCount(node, item.properties?.dynamicOutputCount ?? 3)
      node.id = item.id
      if (item.properties?.label && !typeId.startsWith('origin.variable.') && !item.properties.legacyClass) {
        node.label = item.properties.label
        node.width = Math.max(node.width ?? 230, nodeTitleWidth(node.label))
      }
      setControlValues(node, item.values)
      syncDynamicBranchOutputs(node, dynamicBranchValueCount(node))
      await editor.addNode(node)
      await area.translate(node.id, item.position)
      nodes.set(node.id, node)
    }
    for (const item of data.connections) {
      const source = nodes.get(item.source)
      const target = nodes.get(item.target)
      if (source && target && source.outputs[item.sourceOutput] && target.inputs[item.targetInput]) {
        const connection = createConnection(source, item.sourceOutput, target, item.targetInput)
        if (item.entryConnectionVisible) {
          visibleEntryConnectionIds.add(connection.id)
          updateConnectionPresentation(connection)
        }
        await editor.addConnection(connection)
      }
    }
    await refreshPortStates(true)
    restoring = false
    renderGroups()
    updateMetrics()
    callbacks.onSelection(null)
    emitFunctionSignatureFromSnapshot(data)
  }

  async function mutate(label: string, operation: () => Promise<void>) {
    if (!restoring) undoStack.push(snapshot())
    redoStack.length = 0
    transactionActive = true
    try { await operation() } finally { transactionActive = false }
    updateMetrics()
    callbacks.onStatus(label)
    callbacks.onDirty()
  }

  function connectionTypes(item: { source: string; sourceOutput: string; target: string; targetInput: string }) {
    const source = editor.getNode(item.source)
    const target = editor.getNode(item.target)
    return {
      source: source?.outputs[item.sourceOutput]?.socket.name,
      target: target?.inputs[item.targetInput]?.socket.name
    }
  }

  function connectionSocketType(item: { source: string; sourceOutput: string; target: string; targetInput: string }) {
    const types = connectionTypes(item)
    return normalizeSocketName(types.source ?? types.target)
  }

  function entryBindingNode(node?: BlueprintNode): EntryBindingNode | undefined {
    if (!node) return undefined
    const inputs = Object.fromEntries(Object.entries(node.inputs).flatMap(([key, port]) => port ? [[key, { label: port.label, socket: port.socket.name }]] : []))
    const outputs = Object.fromEntries(Object.entries(node.outputs).flatMap(([key, port]) => port ? [[key, { label: port.label, socket: port.socket.name }]] : []))
    return { id: node.id, typeId: node.typeId, legacyClass: node.legacyClass, label: node.label, inputs, outputs }
  }

  function updateConnectionPresentation(item: Schemes['Connection']) {
    const implicitEntryConnection = isEntryOutputConnection({
      source: item.source,
      sourceOutput: String(item.sourceOutput),
      target: item.target,
      targetInput: String(item.targetInput)
    }, id => entryBindingNode(editor.getNode(id)))
    const hidden = implicitEntryConnection && !visibleEntryConnectionIds.has(item.id)
    const changed = item.hidden !== hidden
    item.hidden = hidden
    return changed
  }

  function createConnection(source: BlueprintNode, sourceOutput: string, target: BlueprintNode, targetInput: string) {
    const item = new ClassicPreset.Connection(source, sourceOutput, target, targetInput) as Schemes['Connection']
    item.socketType = connectionSocketType({ source: source.id, sourceOutput, target: target.id, targetInput })
    updateConnectionPresentation(item)
    return item
  }

  function decorateConnection(item: Schemes['Connection']) {
    item.socketType = connectionSocketType({
      source: item.source,
      sourceOutput: String(item.sourceOutput),
      target: item.target,
      targetInput: String(item.targetInput)
    })
    updateConnectionPresentation(item)
  }

  function refreshInputControlVisibility(nodeIds?: Set<string>) {
    const connectedInputs = new Set(editor.getConnections().map(item => `${item.target}:${String(item.targetInput)}`))
    for (const node of editor.getNodes()) {
      if (nodeIds && !nodeIds.has(node.id)) continue
      for (const [key, input] of Object.entries(node.inputs)) {
        if (input?.control) input.showControl = !connectedInputs.has(`${node.id}:${key}`)
      }
    }
  }

  async function refreshPortStates(updateNodes = false, onlyNodeIds?: Iterable<string>) {
    const nodes = editor.getNodes()
    const nodeIds = onlyNodeIds ? new Set(onlyNodeIds) : undefined
    const connections = editor.getConnections()
    const changedConnections = connections.filter(updateConnectionPresentation)
    refreshNodePortStates(nodes, connections, id => editor.getNode(id))
    refreshInputControlVisibility(nodeIds)
    if (updateNodes) {
      const updates = nodeIds ? nodes.filter(node => nodeIds.has(node.id)) : nodes
      await Promise.all(updates.map(node => area.update('node', node.id)))
      await Promise.all(changedConnections.map(item => area.update('connection', item.id)))
    }
  }

  async function pruneDynamicBranchConnections(nodeId: string, count: number) {
    const node = editor.getNode(nodeId)
    const config = node?.dynamicBranch
    if (!node || !config) return
    const firstHiddenIndex = config.outputStartIndex + count
    const stale = editor.getConnections().filter(item => {
      if (item.source !== nodeId) return false
      const output = String(item.sourceOutput)
      if (!output.startsWith(config.outputPrefix)) return false
      const index = Number(output.slice(config.outputPrefix.length))
      return Number.isFinite(index) && index >= firstHiddenIndex
    })
    for (const item of stale) await editor.removeConnection(item.id)
  }

  async function fitGraphAfterRender() {
    const nodes = editor.getNodes()
    if (!nodes.length) return
    await nextFrame()
    await nextFrame()
    await nextFrame()
    await nextFrame()
    const zoom = area.area.transform.k || 1
    const entries = nodes.map(node => {
      const view = area.nodeViews.get(node.id)
      const position = view?.position ?? { x: 0, y: 0 }
      const rect = view?.element.getBoundingClientRect()
      return {
        position,
        width: rect ? Math.max(rect.width, 1) / zoom : (node.width || 230),
        height: rect ? Math.max(rect.height, 1) / zoom : 90
      }
    })
    const left = Math.min(...entries.map(entry => entry.position.x))
    const top = Math.min(...entries.map(entry => entry.position.y))
    const right = Math.max(...entries.map(entry => entry.position.x + entry.width))
    const bottom = Math.max(...entries.map(entry => entry.position.y + entry.height))
    const graphWidth = Math.max(1, right - left)
    const graphHeight = Math.max(1, bottom - top)
    const rect = container.getBoundingClientRect()
    const padding = 120
    const nextZoom = Math.min(1, Math.max(0.25, Math.min((rect.width - padding) / graphWidth, (rect.height - padding) / graphHeight)))
    const centerX = left + graphWidth / 2
    const centerY = top + graphHeight / 2
    await area.area.zoom(nextZoom)
    await area.area.translate(rect.width / 2 - centerX * nextZoom, rect.height / 2 - centerY * nextZoom)
  }

  function automaticConverter(source?: string, target?: string) {
    if (source === 'integer' && target === 'string' && hasNodeDefinition('origin.cast.integer-string')) return 'origin.cast.integer-string'
    if (source === 'float' && target === 'string' && hasNodeDefinition('origin.cast.float-string')) return 'origin.cast.float-string'
    return ''
  }

  async function insertAutomaticConverter(item: { source: string; sourceOutput: string; target: string; targetInput: string }, typeId: string) {
    const source = editor.getNode(item.source), target = editor.getNode(item.target)
    if (!source || !target) return
    await mutate('Automatic converter inserted', async () => {
      const sourcePosition = area.nodeViews.get(source.id)?.position ?? { x: 0, y: 0 }
      const targetPosition = area.nodeViews.get(target.id)?.position ?? { x: sourcePosition.x + 360, y: sourcePosition.y }
      const converter = createNode(typeId)
      await editor.addNode(converter)
      await area.translate(converter.id, { x: (sourcePosition.x + targetPosition.x) / 2, y: (sourcePosition.y + targetPosition.y) / 2 })
      await editor.addConnection(createConnection(source, item.sourceOutput, converter, 'value'))
      await editor.addConnection(createConnection(converter, 'result', target, item.targetInput))
    })
  }

  editor.addPipe(context => {
    if (context.type !== 'connectioncreate' || restoring) return context
    const types = connectionTypes(context.data)
    ;(context.data as Schemes['Connection']).socketType = normalizeSocketName(types.source ?? types.target)
    if (types.source && types.target && types.source !== types.target && types.source !== 'any' && types.target !== 'any') {
      const converter = automaticConverter(types.source, types.target)
      if (converter) {
        const item = { ...context.data, sourceOutput: String(context.data.sourceOutput), targetInput: String(context.data.targetInput) }
        queueMicrotask(() => void insertAutomaticConverter(item, converter))
        callbacks.onStatus(`Inserting converter: ${types.source} to ${types.target}`)
        return
      }
      callbacks.onStatus(`Connection rejected: ${types.source ?? 'unknown'} cannot connect to ${types.target ?? 'unknown'}`)
      return
    }
    return context
  })

  function selectedNodes() {
    return editor.getNodes().filter(node => node.selected)
  }

  function isDuplicateEntryNode(node: BlueprintNode) {
    if (!node.entrySourceKey) return false
    return editor.getNodes().some(item => item.entrySourceKey === node.entrySourceKey || item.typeId === node.typeId)
  }

  function canAddOrdinaryEntryNode(options?: AddNodeOptions) {
    return options?.allowEntryNodes ?? callbacks.canAddEntryNodes?.() ?? true
  }

  async function addNode(typeId: string, clientPosition?: Position, options?: AddNodeOptions) {
    const node = createNode(typeId)
    if (node.entrySourceKey && !canAddOrdinaryEntryNode(options)) throw new Error('函数蓝图不能添加普通入口节点')
    if (isDuplicateEntryNode(node)) throw new Error('该入口节点已存在，不能重复添加')
    await mutate('Node created', async () => {
      await clearConnectionSelection()
      await editor.addNode(node)
      await area.translate(node.id, graphPosition(clientPosition))
      await refreshPortStates(true)
      await selector.unselectAll()
      await selectable.select(node.id, false)
      callbacks.onSelection(selectedNodeInfo(node))
    })
  }

  async function addFunctionCallNode(spec: FunctionNodeMetadata, clientPosition?: Position) {
    await mutate('Function call node created', async () => {
      await clearConnectionSelection()
      const node = createFunctionCallNode(spec)
      await editor.addNode(node)
      await area.translate(node.id, graphPosition(clientPosition))
      await refreshPortStates(true)
      await selector.unselectAll()
      await selectable.select(node.id, false)
      callbacks.onSelection(selectedNodeInfo(node))
    })
  }

  async function addFunctionEntryNode(spec: FunctionNodeMetadata, clientPosition?: Position) {
    await mutate('Function entry node created', async () => {
      await clearConnectionSelection()
      const node = createFunctionEntryNodeFromSpec(spec)
      await editor.addNode(node)
      await area.translate(node.id, graphPosition(clientPosition))
      await refreshPortStates(true)
      await selector.unselectAll()
      await selectable.select(node.id, false)
      callbacks.onSelection(selectedNodeInfo(node))
    })
  }

  async function addFunctionReturnNode(spec: FunctionNodeMetadata, clientPosition?: Position) {
    await mutate('Function return node created', async () => {
      await clearConnectionSelection()
      const node = createFunctionReturnNodeFromSpec(spec)
      await editor.addNode(node)
      await area.translate(node.id, graphPosition(clientPosition))
      await refreshPortStates(true)
      await selector.unselectAll()
      await selectable.select(node.id, false)
      callbacks.onSelection(selectedNodeInfo(node))
    })
  }

  function sameFunctionReference(properties: NodeProperties | undefined, spec: FunctionNodeMetadata) {
    if (!properties) return false
    return Boolean(spec.functionId && properties.functionId === spec.functionId)
  }

  async function syncFunctionSignature(spec: FunctionNodeMetadata) {
    await mutate('Function signature synchronized', async () => {
      const data = snapshot()
      const changedNodeIds = new Set<string>()
      let changed = false
      for (const node of data.nodes) {
        if (!node.typeId.startsWith('origin.function.')) continue
        const role = node.properties?.functionRole
        const isTerminal = role === 'entry' || role === 'return'
        const isMatchingCall = role === 'call' && sameFunctionReference(node.properties, spec)
        if (!isTerminal && !isMatchingCall) continue
        node.properties = {
          ...node.properties,
          functionId: spec.functionId,
          functionName: spec.functionName,
          functionSource: spec.functionSource,
          functionSignature: spec.functionSignature,
          label: role === 'entry'
            ? `${spec.functionName} Entry`
            : role === 'return'
              ? `${spec.functionName} Return`
              : node.properties?.label
        }
        changedNodeIds.add(node.id)
        changed = true
      }
      data.connections = pruneFunctionSignatureConnections(data, changedNodeIds)
      if (changed) await restore(data)
    })
  }

  async function addVariableNode(variable: GraphVariable, access: 'get' | 'set', clientPosition?: Position) {
    await mutate(`${access === 'get' ? 'Get' : 'Set'} variable node created`, async () => {
      await clearConnectionSelection()
      const node = createVariableNode(variable, access)
      await editor.addNode(node)
      await area.translate(node.id, graphPosition(clientPosition))
      await refreshPortStates(true)
      await selector.unselectAll()
      await selectable.select(node.id, false)
      callbacks.onSelection(selectedNodeInfo(node))
    })
  }

  async function deleteSelected() {
    const selected = selectedNodes()
    const selectedConnections = new Set(selectedConnectionIds)
    if (!selected.length && !selectedConnections.size) return
    const parts = [selected.length ? `${selected.length} node(s)` : '', selectedConnections.size ? `${selectedConnections.size} connection(s)` : ''].filter(Boolean)
    await mutate(`Deleted ${parts.join(' and ')}`, async () => {
      const ids = new Set(selected.map(node => node.id))
      for (const item of editor.getConnections()) {
        if (selectedConnections.has(item.id) || ids.has(item.source) || ids.has(item.target)) await editor.removeConnection(item.id)
      }
      for (const node of selected) await editor.removeNode(node.id)
      selectedConnectionIds.clear()
      callbacks.onSelection(null)
    })
  }

  function copy() {
    const selected = selectedNodes()
    if (!selected.length) return
    const ids = new Map(selected.map((node, index) => [node.id, index]))
    const positions = selected.map(node => area.nodeViews.get(node.id)?.position ?? { x: 0, y: 0 })
    const minX = Math.min(...positions.map(item => item.x))
    const minY = Math.min(...positions.map(item => item.y))
    clipboard = {
      nodes: selected.map((node, index) => ({
        typeId: node.typeId ?? '',
        position: { x: positions[index].x - minX, y: positions[index].y - minY },
        values: controlValues(node),
        properties: { label: node.label, variableId: node.variableId, variableAccess: node.variableAccess, dynamicOutputCount: node.dynamicOutputCount, ...functionPropertiesForSnapshot(node), legacyClass: node.legacyClass, legacyModule: node.legacyModule, legacyInputs: legacyInputsForSnapshot(node), legacyOutputs: legacyOutputsForSnapshot(node) }
      })),
      connections: editor.getConnections().flatMap(item => {
        const sourceIndex = ids.get(item.source)
        const targetIndex = ids.get(item.target)
        return sourceIndex === undefined || targetIndex === undefined ? [] : [{
          sourceIndex,
          sourceOutput: String(item.sourceOutput),
          targetIndex,
          targetInput: String(item.targetInput)
        }]
      })
    }
    callbacks.onStatus(`Copied ${selected.length} node(s)`)
  }

  async function cut() {
    copy()
    await deleteSelected()
  }

  async function paste() {
    if (!clipboard) return
    await mutate(`Pasted ${clipboard.nodes.length} node(s)`, async () => {
      const base = graphPosition()
      const nodes = new Map<number, BlueprintNode>()
      await selector.unselectAll()
      for (const [index, item] of clipboard!.nodes.entries()) {
        const typeId = typeof item.typeId === 'string' ? item.typeId : ''
        if (!typeId) continue
        const node = createRestoredNode(item, typeId)
        if (!node) continue
        if (node.entrySourceKey && (!canAddOrdinaryEntryNode() || isDuplicateEntryNode(node))) continue
        applyNodeProperties(node, item.properties)
        if (node.dynamicOutputs) setDynamicOutputCount(node, item.properties?.dynamicOutputCount ?? 3)
        if (item.properties?.label && !typeId.startsWith('origin.variable.') && !item.properties.legacyClass) {
          node.label = item.properties.label
          node.width = Math.max(node.width ?? 230, nodeTitleWidth(node.label))
        }
        setControlValues(node, item.values)
        syncDynamicBranchOutputs(node, dynamicBranchValueCount(node))
        await editor.addNode(node)
        await area.translate(node.id, { x: base.x + item.position.x, y: base.y + item.position.y })
        await selectable.select(node.id, true)
        nodes.set(index, node)
      }
      for (const item of clipboard!.connections) {
        const source = nodes.get(item.sourceIndex)
        const target = nodes.get(item.targetIndex)
        if (source && target) await editor.addConnection(createConnection(source, item.sourceOutput, target, item.targetInput))
      }
    })
  }

  async function undo() {
    const previous = undoStack.pop()
    if (!previous) return
    redoStack.push(snapshot())
    await restore(previous)
    callbacks.onStatus('Undo')
  }

  async function redo() {
    const next = redoStack.pop()
    if (!next) return
    undoStack.push(snapshot())
    await restore(next)
    callbacks.onStatus('Redo')
  }

  function getDocument(graphName = 'Untitled', variables?: GraphVariable[], variableGroups?: GraphVariableGroup[]): GraphDocument {
    const data = snapshot()
    return {
      schemaVersion: 1,
      graphName,
      ...data,
      variables: (variables ?? currentVariables).map(item => ({ ...item })),
      variableGroups: (variableGroups ?? currentVariableGroups).map(item => ({ ...item })),
      view: { x: area.area.transform.x, y: area.area.transform.y, zoom: area.area.transform.k },
      legacy: cloneLegacyState(currentLegacy)
    }
  }

  async function loadDocument(document: GraphDocument) {
    undoStack.length = 0; redoStack.length = 0
    visibleEntryConnectionIds.clear()
    currentVariables = (document.variables ?? []).map(item => ({ ...item }))
    currentVariableGroups = (document.variableGroups ?? []).map(item => ({ ...item }))
    currentLegacy = cloneLegacyState(document.legacy)
    callbacks.onVariables(currentVariables.map(item => ({ ...item })))
    callbacks.onVariableGroups(currentVariableGroups.map(item => ({ ...item })))
    await restore({ nodes: document.nodes ?? [], connections: document.connections ?? [], groups: document.groups ?? [] })
    if (document.nodes?.length) await fitGraphAfterRender()
    else if (document.view) {
      await area.area.translate(document.view.x, document.view.y)
      await area.area.zoom(document.view.zoom || 1)
    }
    await refreshPortStates()
    // Safety: center view on nodes directly from document data (bypasses rete area timing issues)
    const docNodes = document.nodes
    if (docNodes?.length) {
      await centerViewOnDocument(docNodes)
    }
    callbacks.onStatus('Graph loaded')
  }

  async function newDocument() {
    undoStack.length = 0; redoStack.length = 0; groups.length = 0; selectedGroupId = null
    visibleEntryConnectionIds.clear()
    currentVariables = []; currentVariableGroups = [{ id: 'default', name: 'Default' }]; currentLegacy = undefined; insertionOffset = 0; callbacks.onVariables([]); callbacks.onVariableGroups(currentVariableGroups.map(item => ({ ...item }))); callbacks.onSelection(null)
    restoring = true; await selector.unselectAll(); await editor.clear(); restoring = false; renderGroups(); updateMetrics()
    await area.area.translate(0, 0); await area.area.zoom(1)
  callbacks.onStatus('New graph')
}

async function centerViewOnDocument(nodes: Array<{ position?: { x: number; y: number }; width?: number }>) {
  if (!nodes.length) return
  const minX = Math.min(...nodes.map(n => n.position?.x ?? 0))
  const maxX = Math.max(...nodes.map(n => (n.position?.x ?? 0) + (n.width || 230)))
  const minY = Math.min(...nodes.map(n => n.position?.y ?? 0))
  const maxY = Math.max(...nodes.map(n => (n.position?.y ?? 0) + 90))
  const graphW = Math.max(1, maxX - minX)
  const graphH = Math.max(1, maxY - minY)
  const canvasRect = container.getBoundingClientRect()
  const padding = 120
  const zoom = Math.min(1, Math.max(0.25, Math.min((canvasRect.width - padding) / graphW, (canvasRect.height - padding) / graphH)))
  const cx = minX + graphW / 2
  const cy = minY + graphH / 2
  await area.area.zoom(zoom)
  await area.area.translate(canvasRect.width / 2 - cx * zoom, canvasRect.height / 2 - cy * zoom)
}

function nodeSize(node: BlueprintNode) {
    const element = area.nodeViews.get(node.id)?.element
    const zoom = area.area.transform.k || 1
    return element ? { width: element.getBoundingClientRect().width / zoom, height: element.getBoundingClientRect().height / zoom } : { width: node.width ?? 230, height: 90 }
  }

  async function align(mode: Parameters<BlueprintEditorHandle['align']>[0]) {
    const nodes = selectedNodes()
    if (nodes.length < 2) return
    await mutate(`Align: ${mode}`, async () => {
      const entries = nodes.map(node => ({ node, position: { ...(area.nodeViews.get(node.id)?.position ?? { x: 0, y: 0 }) }, size: nodeSize(node) }))
      const minX = Math.min(...entries.map(e => e.position.x)), maxRight = Math.max(...entries.map(e => e.position.x + e.size.width))
      const minY = Math.min(...entries.map(e => e.position.y)), maxBottom = Math.max(...entries.map(e => e.position.y + e.size.height))
      const centerX = (minX + maxRight) / 2, centerY = (minY + maxBottom) / 2
      if (mode === 'horizontal-distribute') {
        const sorted = [...entries].sort((a, b) => a.position.x - b.position.x)
        const gap = (maxRight - minX - sorted.reduce((sum, e) => sum + e.size.width, 0)) / (sorted.length - 1)
        let x = minX; for (const entry of sorted) { await area.translate(entry.node.id, { x, y: entry.position.y }); x += entry.size.width + gap }
      } else if (mode === 'vertical-distribute') {
        const sorted = [...entries].sort((a, b) => a.position.y - b.position.y)
        const gap = (maxBottom - minY - sorted.reduce((sum, e) => sum + e.size.height, 0)) / (sorted.length - 1)
        let y = minY; for (const entry of sorted) { await area.translate(entry.node.id, { x: entry.position.x, y }); y += entry.size.height + gap }
      } else {
        for (const entry of entries) {
          let { x, y } = entry.position
          if (mode === 'left') x = minX
          if (mode === 'right') x = maxRight - entry.size.width
          if (mode === 'top') y = minY
          if (mode === 'bottom') y = maxBottom - entry.size.height
          if (mode === 'vertical-center') x = centerX - entry.size.width / 2
          if (mode === 'horizontal-center' || mode === 'straighten') y = centerY - entry.size.height / 2
          await area.translate(entry.node.id, { x, y })
        }
      }
    })
  }

  async function groupSelected() {
    const nodes = selectedNodes()
    if (!nodes.length) return
    await mutate('Group nodes', async () => {
      const entries = nodes.map(node => ({ node, position: area.nodeViews.get(node.id)?.position ?? { x: 0, y: 0 }, size: nodeSize(node) }))
      const minX = Math.min(...entries.map(e => e.position.x)), minY = Math.min(...entries.map(e => e.position.y))
      const maxX = Math.max(...entries.map(e => e.position.x + e.size.width)), maxY = Math.max(...entries.map(e => e.position.y + e.size.height))
      const group: GroupSnapshot = { id: crypto.randomUUID(), title: 'This is a group title', x: minX - 28, y: minY - 42, width: maxX - minX + 56, height: maxY - minY + 70, nodeIds: nodes.map(node => node.id) }
      groups.push(group); selectedGroupId = group.id; renderGroups()
    })
  }

  async function ungroupSelected() {
    if (!selectedGroupId) return
    await mutate('Ungroup nodes', async () => {
      const index = groups.findIndex(group => group.id === selectedGroupId)
      if (index >= 0) groups.splice(index, 1)
      selectedGroupId = null; renderGroups()
    })
  }

  async function toggleGroupSelected() {
    if (selectedGroupId) { await ungroupSelected(); return }
    await groupSelected()
  }

  async function fitSelected() {
    const nodes = selectedNodes()
    await AreaExtensions.zoomAt(area, nodes.length ? nodes : editor.getNodes(), { scale: 0.9 })
  }

  async function selectAll() {
    await clearConnectionSelection()
    for (const node of editor.getNodes()) await selectable.select(node.id, true)
    callbacks.onStatus(`Selected ${editor.getNodes().length} node(s)`)
  }

  async function deselectAll() {
    await selector.unselectAll()
    await clearConnectionSelection()
    callbacks.onSelection(null)
    callbacks.onStatus('Selection cleared')
  }

  function selectedNodeInfo(node: BlueprintNode): SelectedNodeInfo {
    return { id: node.id, typeId: node.typeId ?? '', label: node.label, description: node.subtitle, values: controlValues(node), variableId: node.variableId }
  }

  function setDynamicOutputCount(node: BlueprintNode, requested: number) {
    if (!node.dynamicOutputs) return
    const count = Math.max(1, Math.min(12, Math.floor(requested)))
    for (const key of Object.keys(node.outputs).filter(key => key.startsWith('then'))) node.removeOutput(key)
    for (let index = 0; index < count; index++) node.addOutput(`then${index}`, new ClassicPreset.Output(new ClassicPreset.Socket('exec'), `Then ${index}`))
    node.dynamicOutputCount = count
  }

  async function changeDynamicOutputs(nodeId: string, delta: number) {
    const node = editor.getNode(nodeId)
    if (!node?.dynamicOutputs) return
    const current = node.dynamicOutputCount ?? 1
    const next = Math.max(1, Math.min(12, current + delta))
    if (next === current) return
    await mutate('Sequence outputs changed', async () => {
      const data = snapshot()
      const item = data.nodes.find(entry => entry.id === nodeId)
      if (!item) return
      item.properties = { ...item.properties, dynamicOutputCount: next }
      data.connections = data.connections.filter(connection => connection.source !== nodeId || !connection.sourceOutput.startsWith('then') || Number(connection.sourceOutput.slice(4)) < next)
      await restore(data)
      const restored = editor.getNode(nodeId)
      if (restored) { await selectable.select(nodeId, false); callbacks.onSelection(selectedNodeInfo(restored)) }
    })
  }

  const dynamicOutputListener = (event: Event) => {
    const detail = (event as CustomEvent<{ nodeId: string; delta: number }>).detail
    if (detail) void changeDynamicOutputs(detail.nodeId, detail.delta)
  }

  function entryBindingGroups(targetNodeId: string, inputKey: string) {
    const nodes = editor.getNodes().flatMap(node => {
      const entry = entryBindingNode(node)
      return entry ? [entry] : []
    })
    return entryBindingCandidateGroups(targetNodeId, inputKey, nodes)
  }

  async function removeInputConnections(targetNodeId: string, inputKey: string) {
    for (const item of editor.getConnections()) {
      if (item.target === targetNodeId && String(item.targetInput) === inputKey) await editor.removeConnection(item.id)
    }
  }

  async function bindEntryOutput(targetNodeId: string, inputKey: string, sourceNodeId: string, sourceOutput: string) {
    const source = editor.getNode(sourceNodeId)
    const target = editor.getNode(targetNodeId)
    if (!source || !target || !source.outputs[sourceOutput] || !target.inputs[inputKey]) return
    await mutate('入口参数已绑定', async () => {
      await removeInputConnections(targetNodeId, inputKey)
      await editor.addConnection(createConnection(source, sourceOutput, target, inputKey))
      await refreshPortStates(true, [sourceNodeId, targetNodeId])
    })
  }

  async function clearEntryBinding(targetNodeId: string, inputKey: string) {
    await mutate('入口参数绑定已清除', async () => {
      await removeInputConnections(targetNodeId, inputKey)
      await refreshPortStates(true, [targetNodeId])
    })
  }

  function currentEntryBinding(targetNodeId: string, inputKey: string) {
    for (const item of editor.getConnections()) {
      if (item.target !== targetNodeId || String(item.targetInput) !== inputKey) continue
      const binding = describeEntryBinding({
        source: item.source,
        sourceOutput: String(item.sourceOutput),
        target: item.target,
        targetInput: String(item.targetInput)
      }, id => entryBindingNode(editor.getNode(id)))
      if (binding) return { binding, connectionId: item.id, visible: visibleEntryConnectionIds.has(item.id) }
    }
    return undefined
  }

  async function setEntryConnectionVisible(connectionId: string, visible: boolean) {
    await mutate(visible ? '入口连线显示为普通连线' : '入口连线折叠为标签', async () => {
      const item = editor.getConnection(connectionId)
      if (!item) return
      if (visible) visibleEntryConnectionIds.add(connectionId); else visibleEntryConnectionIds.delete(connectionId)
      updateConnectionPresentation(item)
      await area.update('connection', connectionId)
    })
  }

  const entryBindingMenu = document.createElement('div')
  entryBindingMenu.className = 'entry-binding-menu'
  entryBindingMenu.hidden = true
  entryBindingMenu.addEventListener('pointerdown', event => event.stopPropagation())
  entryBindingMenu.addEventListener('wheel', event => event.stopPropagation())
  container.appendChild(entryBindingMenu)

  function hideEntryBindingMenu() {
    entryBindingMenu.hidden = true
    entryBindingMenu.replaceChildren()
  }

  function addEntryBindingMenuButton(label: string, action: () => void | Promise<void>, className = '') {
    const button = document.createElement('button')
    if (className) button.className = className
    button.textContent = label
    button.onclick = () => { hideEntryBindingMenu(); void action() }
    entryBindingMenu.appendChild(button)
  }

  function showEntryBindingMenu(detail: { nodeId: string; inputKey: string; clientX: number; clientY: number }) {
    const target = editor.getNode(detail.nodeId)
    const input = target?.inputs[detail.inputKey]
    if (!target || !input || input.socket.name === 'exec') return
    entryBindingMenu.replaceChildren()

    const title = document.createElement('div')
    title.className = 'entry-binding-title'
    title.textContent = `${input.label || detail.inputKey} 绑定入口参数`
    entryBindingMenu.appendChild(title)

    const current = currentEntryBinding(detail.nodeId, detail.inputKey)
    if (current) {
      const currentLabel = document.createElement('div')
      currentLabel.className = 'entry-binding-current'
      currentLabel.textContent = `当前: ${current.binding.sourceNodeLabel} / ${current.binding.sourceOutputLabel}`
      entryBindingMenu.appendChild(currentLabel)
      addEntryBindingMenuButton('跳转到入口节点', () => focusNode(current.binding.sourceNodeId))
      addEntryBindingMenuButton(current.visible ? '折叠为入口标签' : '显示为普通连线', () => setEntryConnectionVisible(current.connectionId, !current.visible))
      addEntryBindingMenuButton('清除入口参数绑定', () => clearEntryBinding(detail.nodeId, detail.inputKey), 'danger')
    }

    const groups = entryBindingGroups(detail.nodeId, detail.inputKey)
    if (groups.length) {
      const heading = document.createElement('div')
      heading.className = 'entry-binding-group'
      heading.textContent = '可用入口参数'
      entryBindingMenu.appendChild(heading)
      for (const group of groups) {
        const source = document.createElement('div')
        source.className = 'entry-binding-source'
        source.textContent = group.sourceNodeLabel
        entryBindingMenu.appendChild(source)
        for (const candidate of group.candidates) {
          addEntryBindingMenuButton(candidate.sourceOutputLabel, () => bindEntryOutput(detail.nodeId, detail.inputKey, candidate.sourceNodeId, candidate.sourceOutput), 'entry-output')
        }
      }
    } else {
      const empty = document.createElement('div')
      empty.className = 'entry-binding-empty'
      empty.textContent = '没有类型匹配的入口参数'
      entryBindingMenu.appendChild(empty)
    }

    const rect = container.getBoundingClientRect()
    entryBindingMenu.style.left = `${Math.max(6, Math.min(detail.clientX - rect.left, rect.width - 250))}px`
    entryBindingMenu.style.top = `${Math.max(6, Math.min(detail.clientY - rect.top, rect.height - 260))}px`
    entryBindingMenu.hidden = false
  }

  const entryBindingMenuListener = (event: Event) => {
    const detail = (event as CustomEvent<{ nodeId: string; inputKey: string; clientX: number; clientY: number }>).detail
    if (detail) showEntryBindingMenu(detail)
  }
  const dynamicBranchListener = (event: Event) => {
    const detail = (event as CustomEvent<{ nodeId: string; count: number; countChanged?: boolean }>).detail
    if (!detail) return
    void (async () => {
      if (detail.countChanged) await pruneDynamicBranchConnections(detail.nodeId, detail.count)
      const node = editor.getNode(detail.nodeId)
      if (node) syncDynamicBranchOutputs(node, detail.count)
      await refreshPortStates(Boolean(node))
      if (node) {
        await area.update('node', node.id)
        if (node.selected) callbacks.onSelection(selectedNodeInfo(node))
      }
      callbacks.onDirty()
    })()
  }
  const controlChangeListener = () => {
    if (restoring) return
    callbacks.onDirty()
    void refreshPortStates(true)
  }
  const connectionSelectListener = (event: Event) => {
    const detail = (event as CustomEvent<{ id: string; additive: boolean }>).detail
    if (detail) void selectConnection(detail.id, detail.additive)
  }
  const connectionDeleteListener = (event: Event) => {
    const detail = (event as CustomEvent<{ id: string }>).detail
    if (!detail) return
    void (async () => {
      await clearConnectionSelection()
      const item = editor.getConnection(detail.id)
      if (!item) return
      item.selected = true; selectedConnectionIds.add(detail.id)
      await deleteSelected()
    })()
  }
  container.addEventListener('origin-dynamic-output', dynamicOutputListener)
  container.addEventListener('origin-entry-binding-menu', entryBindingMenuListener)
  document.addEventListener('origin-dynamic-branch-change', dynamicBranchListener)
  container.addEventListener('origin-connection-select', connectionSelectListener)
  container.addEventListener('origin-connection-delete', connectionDeleteListener)
  document.addEventListener('origin-control-change', controlChangeListener)
  window.addEventListener('pointerdown', hideEntryBindingMenu)
  const destroyPanFeedback = setupCanvasPanFeedback()
  const destroyMultiSelectionDragPreserver = setupMultiSelectionDragPreserver()

  async function updateSelectedNode(label: string, values: Record<string, unknown>) {
    const node = selectedNodes()[0]
    if (!node) return
    await mutate('Node properties updated', async () => {
      node.label = label.trim() || node.label
      node.width = Math.max(230, nodeTitleWidth(node.label))
      setControlValues(node, values)
      if (node.dynamicBranch) await pruneDynamicBranchConnections(node.id, dynamicBranchValueCount(node))
      syncDynamicBranchOutputs(node, dynamicBranchValueCount(node))
      await refreshPortStates()
      await area.update('node', node.id)
      callbacks.onSelection(selectedNodeInfo(node))
    })
  }

  async function setVariables(variables: GraphVariable[], variableGroups?: GraphVariableGroup[], refreshNodes = false) {
    const before = snapshot()
    currentVariables = variables.map(item => ({ ...item }))
    if (variableGroups) currentVariableGroups = variableGroups.map(item => ({ ...item }))
    if (refreshNodes && before.nodes.some(item => item.typeId.startsWith('origin.variable.'))) await restore(before)
    callbacks.onVariables(currentVariables.map(item => ({ ...item })))
    callbacks.onVariableGroups(currentVariableGroups.map(item => ({ ...item })))
  }

  async function focusNode(id: string) {
    const node = editor.getNode(id)
    if (!node) return
    await selector.unselectAll()
    await selectable.select(id, false)
    callbacks.onSelection(selectedNodeInfo(node))
    await centerNodesForReading([node])
  }

  async function centerNodesForReading(nodes: BlueprintNode[]) {
    if (!nodes.length) return
    const entries = nodes.map(node => {
      const position = area.nodeViews.get(node.id)?.position ?? { x: 0, y: 0 }
      return { position, size: nodeSize(node) }
    })
    const minX = Math.min(...entries.map(item => item.position.x))
    const minY = Math.min(...entries.map(item => item.position.y))
    const maxX = Math.max(...entries.map(item => item.position.x + item.size.width))
    const maxY = Math.max(...entries.map(item => item.position.y + item.size.height))
    const rect = container.getBoundingClientRect()
    const boxWidth = Math.max(1, maxX - minX)
    const boxHeight = Math.max(1, maxY - minY)
    const fitZoom = Math.min((rect.width * 0.72) / boxWidth, (rect.height * 0.58) / boxHeight)
    const nextZoom = Math.max(nodeLocateMinZoomScale, Math.min(nodeLocateZoomScale, fitZoom))
    const centerX = (minX + maxX) / 2
    const centerY = (minY + maxY) / 2
    await area.area.zoom(nextZoom, 0, 0)
    await area.area.translate(rect.width * nodeLocateViewportAnchor.x - centerX * nextZoom, rect.height * nodeLocateViewportAnchor.y - centerY * nextZoom)
  }

  async function highlightNodesByType(typeId: string) {
    const matches = editor.getNodes().filter(node => node.typeId === typeId)
    for (const node of editor.getNodes()) {
      const highlighted = matches.includes(node)
      if (node.referenceHighlighted !== highlighted) {
        node.referenceHighlighted = highlighted
        await area.update('node', node.id)
      }
    }
    callbacks.onSelection(null)
    if (matches.length) await centerNodesForReading(matches)
    return matches.length
  }

  async function highlightIssueNodes(ids: string[]) {
    const idSet = new Set(ids.filter(Boolean))
    const matches = editor.getNodes().filter(node => idSet.has(node.id))
    await selector.unselectAll()
    for (const item of editor.getNodes()) {
      const highlighted = idSet.has(item.id)
      if (item.issueHighlighted !== highlighted) {
        item.issueHighlighted = highlighted
        await area.update('node', item.id)
      }
    }
    callbacks.onSelection(null)
    if (matches.length) {
      await nextFrame()
      await centerNodesForReading(matches)
    }
    return matches.length
  }

  async function highlightIssueNode(id: string) {
    return highlightIssueNodes([id])
  }

  function setupRubberBandSelection() {
    const rectangle = document.createElement('div')
    rectangle.className = 'selection-rectangle'
    container.appendChild(rectangle)
    let start: Position | null = null

    const move = (event: PointerEvent) => {
      if (!start) return
      const rect = container.getBoundingClientRect()
      const current = { x: event.clientX - rect.left, y: event.clientY - rect.top }
      rectangle.style.display = 'block'
      rectangle.style.left = `${Math.min(start.x, current.x)}px`
      rectangle.style.top = `${Math.min(start.y, current.y)}px`
      rectangle.style.width = `${Math.abs(current.x - start.x)}px`
      rectangle.style.height = `${Math.abs(current.y - start.y)}px`
    }

    const up = async (event: PointerEvent) => {
      if (!start) return
      const rect = container.getBoundingClientRect()
      const current = { x: event.clientX - rect.left, y: event.clientY - rect.top }
      const selectionRect: Rect = {
        left: rect.left + Math.min(start.x, current.x),
        top: rect.top + Math.min(start.y, current.y),
        right: rect.left + Math.max(start.x, current.x),
        bottom: rect.top + Math.max(start.y, current.y)
      }
      if (!event.ctrlKey) await selector.unselectAll()
      const connectionIds = connectionIdsInClientRect(selectionRect)
      const selectedConnections = await selectConnections(connectionIds, event.ctrlKey)
      let selectedNodes = 0
      for (const node of editor.getNodes()) {
        const bounds = area.nodeViews.get(node.id)?.element.getBoundingClientRect()
        if (bounds && rectsIntersect(bounds, selectionRect)) {
          await selectable.select(node.id, true)
          selectedNodes++
        }
      }
      if (!selectedNodes) callbacks.onSelection(null)
      if (selectedNodes || selectedConnections) callbacks.onStatus(`Selected ${selectedNodes} node(s), ${selectedConnections} connection(s)`)
      start = null
      rectangle.style.display = 'none'
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
    }

    container.addEventListener('pointerdown', event => {
      const target = event.target as HTMLElement
      if (event.button !== 0 || target.closest('.blueprint-node, .blueprint-socket, .blueprint-connection, input')) return
      const rect = container.getBoundingClientRect()
      start = { x: event.clientX - rect.left, y: event.clientY - rect.top }
      window.addEventListener('pointermove', move)
      window.addEventListener('pointerup', up)
    })
  }

  function connectionIdsInClientRect(selectionRect: Rect) {
    return Array.from(container.querySelectorAll('.blueprint-connection')).flatMap(element => {
      const connectionElement = element as SVGSVGElement
      const id = connectionElement.dataset.connectionId
      const path = connectionElement.querySelector('.connection-line') as SVGPathElement | null
      if (!id || !path) return []
      const matrix = path.getScreenCTM()
      if (!matrix) return []
      const clientPath = {
        getTotalLength: () => path.getTotalLength(),
        getPointAtLength: (offset: number) => {
          const point = path.getPointAtLength(offset).matrixTransform(matrix)
          return { x: point.x, y: point.y }
        }
      }
      return pathIntersectsRect(clientPath, selectionRect) ? [id] : []
    })
  }

  function setupCuttingLine() {
    const namespace = 'http://www.w3.org/2000/svg'
    const overlay = document.createElementNS(namespace, 'svg')
    const line = document.createElementNS(namespace, 'polyline')
    overlay.classList.add('cutting-line-overlay')
    line.classList.add('cutting-line')
    overlay.appendChild(line)
    container.appendChild(overlay)
    let points: Position[] = []
    let cutting = false

    const localPoint = (event: PointerEvent): Position => {
      const rect = container.getBoundingClientRect()
      return { x: event.clientX - rect.left, y: event.clientY - rect.top }
    }
    const draw = () => line.setAttribute('points', points.map(point => `${point.x},${point.y}`).join(' '))
    const distanceToSegment = (point: Position, start: Position, end: Position) => {
      const dx = end.x - start.x, dy = end.y - start.y
      if (!dx && !dy) return Math.hypot(point.x - start.x, point.y - start.y)
      const t = Math.max(0, Math.min(1, ((point.x - start.x) * dx + (point.y - start.y) * dy) / (dx * dx + dy * dy)))
      return Math.hypot(point.x - (start.x + t * dx), point.y - (start.y + t * dy))
    }
    const intersectsCut = (path: SVGPathElement) => {
      if (points.length < 2) return false
      const matrix = path.getScreenCTM()
      const length = path.getTotalLength()
      if (!matrix || !length) return false
      const step = Math.max(3, Math.min(8, length / 45))
      for (let offset = 0; offset <= length; offset += step) {
        const sample = path.getPointAtLength(offset).matrixTransform(matrix)
        const local = { x: sample.x - container.getBoundingClientRect().left, y: sample.y - container.getBoundingClientRect().top }
        for (let index = 1; index < points.length; index++) if (distanceToSegment(local, points[index - 1], points[index]) <= 6) return true
      }
      return false
    }
    const move = (event: PointerEvent) => {
      if (!cutting) return
      points.push(localPoint(event)); draw()
    }
    const up = async (event: PointerEvent) => {
      if (!cutting || event.button !== 2) return
      cutting = false
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      const ids = Array.from(container.querySelectorAll('.blueprint-connection')).flatMap(element => {
        const connectionElement = element as SVGSVGElement
        const id = connectionElement.dataset.connectionId
        const path = connectionElement.querySelector('.connection-line') as SVGPathElement | null
        return id && path && intersectsCut(path) ? [id] : []
      })
      points = []; draw(); overlay.classList.remove('active'); container.classList.remove('cutting-mode')
      if (!ids.length) { callbacks.onStatus('No connections cut'); return }
      await mutate(`Cut ${ids.length} connection(s)`, async () => {
        for (const id of ids) if (editor.getConnection(id)) await editor.removeConnection(id)
        selectedConnectionIds.clear()
      })
    }
    const down = (event: PointerEvent) => {
      const target = event.target as HTMLElement
      if (event.button !== 2 || !event.ctrlKey || target.closest('.blueprint-node, .blueprint-socket, .blueprint-connection, input')) return
      event.preventDefault(); event.stopPropagation()
      void clearConnectionSelection()
      cutting = true; points = [localPoint(event)]; draw()
      overlay.classList.add('active'); container.classList.add('cutting-mode')
      window.addEventListener('pointermove', move)
      window.addEventListener('pointerup', up)
    }
    const preventMenu = (event: Event) => { if (cutting) event.preventDefault() }
    container.addEventListener('pointerdown', down, true)
    container.addEventListener('contextmenu', preventMenu, true)
    return () => {
      window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up)
      container.removeEventListener('pointerdown', down, true); container.removeEventListener('contextmenu', preventMenu, true)
      overlay.remove()
    }
  }

  area.addPipe(async context => {
    if (context.type === 'zoomed') callbacks.onZoom(context.data.zoom)
    if ((context.type === 'connectioncreate' || context.type === 'connectionremove') && !restoring && !transactionActive && !initializing) pendingConnectionSnapshot = snapshot()
    if (context.type === 'connectioncreated' || context.type === 'connectionremoved') {
      if (context.type === 'connectioncreated') {
        decorateConnection(context.data)
        void area.update('connection', context.data.id)
      }
      if (context.type === 'connectionremoved') {
        selectedConnectionIds.delete(context.data.id)
        visibleEntryConnectionIds.delete(context.data.id)
      }
      queueMicrotask(() => void refreshPortStates(true, [context.data.source, context.data.target]))
      updateMetrics()
      if (!restoring && !transactionActive && !initializing && pendingConnectionSnapshot) {
        undoStack.push(pendingConnectionSnapshot); redoStack.length = 0; pendingConnectionSnapshot = null
        callbacks.onDirty(); callbacks.onStatus(context.type === 'connectioncreated' ? 'Connection created' : 'Connection removed')
      }
    }
    if (context.type === 'nodepicked') {
      void clearConnectionSelection()
      startNodeDragFeedback()
      dragSnapshot = snapshot()
      await restoreMultiSelectionAfterNodePick(context.data.id)
      requestAnimationFrame(() => {
        const node = editor.getNode(context.data.id)
        callbacks.onSelection(node ? selectedNodeInfo(node) : null)
      })
    }
    if (context.type === 'nodedragged' && dragSnapshot) {
      stopNodeDragFeedback()
      undoStack.push(dragSnapshot); redoStack.length = 0; dragSnapshot = null; callbacks.onDirty(); callbacks.onStatus('Node moved')
    }
    return context
  })
  setupRubberBandSelection()
  const destroyCuttingLine = setupCuttingLine()

  await refreshPortStates(true)
  updateMetrics()
  initializing = false

  async function resetView() {
    await fitGraphAfterRender()
    callbacks.onStatus('View fitted')
  }
  requestAnimationFrame(() => resetView())

  return {
    destroy() {
      container.removeEventListener('origin-dynamic-output', dynamicOutputListener)
      container.removeEventListener('origin-entry-binding-menu', entryBindingMenuListener)
      document.removeEventListener('origin-dynamic-branch-change', dynamicBranchListener)
      container.removeEventListener('origin-connection-select', connectionSelectListener)
      container.removeEventListener('origin-connection-delete', connectionDeleteListener)
      document.removeEventListener('origin-control-change', controlChangeListener)
      window.removeEventListener('pointerdown', hideEntryBindingMenu)
      window.removeEventListener('pointerup', stopNodeDragFeedback)
      window.removeEventListener('pointercancel', stopNodeDragFeedback)
      destroyPanFeedback()
      destroyMultiSelectionDragPreserver()
      destroyCuttingLine()
      entryBindingMenu.remove()
      area.destroy()
    },
    resetView,
    addNode,
    addFunctionCallNode,
    addFunctionEntryNode,
    addFunctionReturnNode,
    syncFunctionSignature,
    addVariableNode,
    deleteSelected,
    selectAll,
    deselectAll,
    copy,
    cut,
    paste,
    undo,
    redo,
    getDocument,
    loadDocument,
    newDocument,
    align,
    groupSelected,
    ungroupSelected,
    toggleGroupSelected,
    fitSelected,
    setVariables,
    updateSelectedNode,
    focusNode,
    highlightNodesByType,
    highlightIssueNode,
    highlightIssueNodes
  }
}
