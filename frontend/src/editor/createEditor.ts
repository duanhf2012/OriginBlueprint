import { ClassicPreset, NodeEditor } from 'rete'
import { AreaExtensions, AreaPlugin, Drag } from 'rete-area-plugin'
import { ConnectionPlugin, Presets as ConnectionPresets } from 'rete-connection-plugin'
import { Presets as VuePresets, VuePlugin } from 'rete-vue-plugin'
import BlueprintControl from './BlueprintControl.vue'
import BlueprintConnectionComponent from './BlueprintConnection.vue'
import BlueprintNodeComponent from './BlueprintNode.vue'
import BlueprintSocket from './BlueprintSocket.vue'
import { createLegacyNode, createNode, createVariableNode, hasNodeDefinition } from './nodeRegistry'
import { normalizeSocketName } from './socketTheme'
import { BlueprintNode, type Schemes } from './types'
import { refreshNodePortStates } from './portVisualState'
import { pathIntersectsRect, rectsIntersect, type Rect } from './selectionGeometry'
import type { ConnectionSnapshot, GraphDocument, GraphSnapshot, GraphVariable, GraphVariableGroup, GroupSnapshot, LegacyGraphState, NodeSnapshot } from './document'

export type { GraphDocument, GraphVariable, GraphVariableGroup, ValidationIssue, VariableType } from './document'

type AreaExtra = import('rete-vue-plugin').VueArea2D<Schemes>
type Position = { x: number; y: number }

interface ClipboardGraph {
  nodes: Omit<NodeSnapshot, 'id'>[]
  connections: Array<Omit<ConnectionSnapshot, 'source' | 'target'> & { sourceIndex: number; targetIndex: number }>
}

export interface EditorMetrics {
  nodes: number
  connections: number
}

export interface SelectedNodeInfo {
  id: string
  typeId: string
  label: string
  values: Record<string, unknown>
  variableId?: string
}

export interface BlueprintEditorHandle {
  destroy(): void
  resetView(): void
  addNode(typeId: string, clientPosition?: Position): Promise<void>
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
  fitSelected(): Promise<void>
  setVariables(variables: GraphVariable[], variableGroups?: GraphVariableGroup[], refreshNodes?: boolean): Promise<void>
  updateSelectedNode(label: string, values: Record<string, unknown>): Promise<void>
  focusNode(id: string): Promise<void>
  setExecutionStates(states: Array<{ nodeId: string; state: 'idle' | 'running' | 'completed' | 'error' }>): Promise<void>
  clearExecutionStates(): Promise<void>
}

interface Callbacks {
  onZoom(value: number): void
  onStatus(value: string): void
  onMetrics(metrics: EditorMetrics): void
  onDirty(): void
  onVariables(variables: GraphVariable[]): void
  onVariableGroups(groups: GraphVariableGroup[]): void
  onSelection(node: SelectedNodeInfo | null): void
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

  render.addPreset(VuePresets.classic.setup({
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

  // OriginNodeEditor pans with the middle mouse button. Node dragging remains left-button based.
  area.area.setDragHandler(new Drag({
    down: event => event.pointerType !== 'mouse' || event.button === 1,
    move: () => true
  }))

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
          legacyClass: node.legacyClass,
          legacyModule: node.legacyModule,
          legacyInputs: node.legacyInputs,
          legacyOutputs: node.legacyOutputs
        }
      })),
      connections: editor.getConnections().map(item => ({
        source: item.source,
        sourceOutput: String(item.sourceOutput),
        target: item.target,
        targetInput: String(item.targetInput)
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
    selectedConnectionIds.clear()
    await selector.unselectAll()
    await editor.clear()
    groups.splice(0, groups.length, ...(data.groups ?? []).map(item => ({ ...item, nodeIds: [...item.nodeIds] })))
    const nodes = new Map<string, BlueprintNode>()
    for (const item of data.nodes) {
      const variableAccess = item.properties?.variableAccess ?? (item.typeId === 'origin.variable.set' ? 'set' : 'get')
      const variable = currentVariables.find(entry => entry.id === item.properties?.variableId)
      if (!item.typeId.startsWith('origin.variable.') && item.typeId !== 'origin.legacy.placeholder' && !hasNodeDefinition(item.typeId)) continue
      const node = item.typeId.startsWith('origin.variable.')
        ? createVariableNode(variable ?? { id: item.properties?.variableId ?? '', name: 'Missing Variable', type: 'string', defaultValue: '', groupId: 'default' }, variableAccess)
        : item.typeId === 'origin.legacy.placeholder'
          ? createLegacyNode(item.properties ?? {})
        : createNode(item.typeId)
      if (node.dynamicOutputs) setDynamicOutputCount(node, item.properties?.dynamicOutputCount ?? 3)
      node.id = item.id
      if (item.properties?.label && !item.typeId.startsWith('origin.variable.') && !item.properties.legacyClass) node.label = item.properties.label
      setControlValues(node, item.values)
      await editor.addNode(node)
      await area.translate(node.id, item.position)
      nodes.set(node.id, node)
    }
    for (const item of data.connections) {
      const source = nodes.get(item.source)
      const target = nodes.get(item.target)
      if (source && target && source.outputs[item.sourceOutput] && target.inputs[item.targetInput]) {
        await editor.addConnection(createConnection(source, item.sourceOutput, target, item.targetInput))
      }
    }
    await refreshPortStates(true)
    restoring = false
    renderGroups()
    updateMetrics()
    callbacks.onSelection(null)
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

  function createConnection(source: BlueprintNode, sourceOutput: string, target: BlueprintNode, targetInput: string) {
    const item = new ClassicPreset.Connection(source, sourceOutput, target, targetInput) as Schemes['Connection']
    item.socketType = connectionSocketType({ source: source.id, sourceOutput, target: target.id, targetInput })
    return item
  }

  function decorateConnection(item: Schemes['Connection']) {
    item.socketType = connectionSocketType({
      source: item.source,
      sourceOutput: String(item.sourceOutput),
      target: item.target,
      targetInput: String(item.targetInput)
    })
  }

  async function refreshPortStates(updateNodes = false) {
    const nodes = editor.getNodes()
    refreshNodePortStates(nodes, editor.getConnections(), id => editor.getNode(id))
    if (updateNodes) await Promise.all(nodes.map(node => area.update('node', node.id)))
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

  async function addNode(typeId: string, clientPosition?: Position) {
    await mutate('Node created', async () => {
      await clearConnectionSelection()
      const node = createNode(typeId)
      await editor.addNode(node)
      await area.translate(node.id, graphPosition(clientPosition))
      await refreshPortStates(true)
      await selector.unselectAll()
      await selectable.select(node.id, false)
      callbacks.onSelection(selectedNodeInfo(node))
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
        properties: { label: node.label, variableId: node.variableId, variableAccess: node.variableAccess, dynamicOutputCount: node.dynamicOutputCount, legacyClass: node.legacyClass, legacyModule: node.legacyModule, legacyInputs: node.legacyInputs, legacyOutputs: node.legacyOutputs }
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
      const nodes: BlueprintNode[] = []
      await selector.unselectAll()
      for (const item of clipboard!.nodes) {
        const variableAccess = item.properties?.variableAccess ?? (item.typeId === 'origin.variable.set' ? 'set' : 'get')
        const variable = currentVariables.find(entry => entry.id === item.properties?.variableId)
        const node = item.typeId.startsWith('origin.variable.')
          ? createVariableNode(variable ?? { id: item.properties?.variableId ?? '', name: 'Missing Variable', type: 'string', defaultValue: '', groupId: 'default' }, variableAccess)
          : item.typeId === 'origin.legacy.placeholder'
            ? createLegacyNode(item.properties ?? {})
          : createNode(item.typeId)
        if (node.dynamicOutputs) setDynamicOutputCount(node, item.properties?.dynamicOutputCount ?? 3)
        if (item.properties?.label && !item.typeId.startsWith('origin.variable.') && !item.properties.legacyClass) node.label = item.properties.label
        setControlValues(node, item.values)
        await editor.addNode(node)
        await area.translate(node.id, { x: base.x + item.position.x, y: base.y + item.position.y })
        await selectable.select(node.id, true)
        nodes.push(node)
      }
      for (const item of clipboard!.connections) {
        const source = nodes[item.sourceIndex]
        const target = nodes[item.targetIndex]
        await editor.addConnection(createConnection(source, item.sourceOutput, target, item.targetInput))
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
    currentVariables = (document.variables ?? []).map(item => ({ ...item }))
    currentVariableGroups = (document.variableGroups ?? []).map(item => ({ ...item }))
    currentLegacy = cloneLegacyState(document.legacy)
    callbacks.onVariables(currentVariables.map(item => ({ ...item })))
    callbacks.onVariableGroups(currentVariableGroups.map(item => ({ ...item })))
    await restore({ nodes: document.nodes ?? [], connections: document.connections ?? [], groups: document.groups ?? [] })
    if (document.legacy?.format === 'vgf' && document.nodes?.length) {
      await AreaExtensions.zoomAt(area, editor.getNodes(), { scale: 0.9 })
    } else if (document.view) {
      await area.area.translate(document.view.x, document.view.y)
      await area.area.zoom(document.view.zoom || 1)
    } else if (document.nodes?.length) await AreaExtensions.zoomAt(area, editor.getNodes(), { scale: 0.9 })
    callbacks.onStatus('Graph loaded')
  }

  async function newDocument() {
    undoStack.length = 0; redoStack.length = 0; groups.length = 0; selectedGroupId = null
    currentVariables = []; currentVariableGroups = [{ id: 'default', name: 'Default' }]; currentLegacy = undefined; insertionOffset = 0; callbacks.onVariables([]); callbacks.onVariableGroups(currentVariableGroups.map(item => ({ ...item }))); callbacks.onSelection(null)
    restoring = true; await selector.unselectAll(); await editor.clear(); restoring = false; renderGroups(); updateMetrics()
    await area.area.translate(0, 0); await area.area.zoom(1)
    callbacks.onStatus('New graph')
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
    return { id: node.id, typeId: node.typeId ?? '', label: node.label, values: controlValues(node), variableId: node.variableId }
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
  container.addEventListener('origin-connection-select', connectionSelectListener)
  container.addEventListener('origin-connection-delete', connectionDeleteListener)
  document.addEventListener('origin-control-change', controlChangeListener)

  async function updateSelectedNode(label: string, values: Record<string, unknown>) {
    const node = selectedNodes()[0]
    if (!node) return
    await mutate('Node properties updated', async () => {
      node.label = label.trim() || node.label
      setControlValues(node, values)
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
    await AreaExtensions.zoomAt(area, [node], { scale: 0.9 })
  }

  async function setExecutionStates(states: Array<{ nodeId: string; state: 'idle' | 'running' | 'completed' | 'error' }>) {
    for (const item of states) {
      const node = editor.getNode(item.nodeId)
      if (!node) continue
      node.executionState = item.state
      await area.update('node', node.id)
    }
  }

  async function clearExecutionStates() {
    await setExecutionStates(editor.getNodes().map(node => ({ nodeId: node.id, state: 'idle' as const })))
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

  area.addPipe(context => {
    if (context.type === 'zoomed') callbacks.onZoom(context.data.zoom)
    if ((context.type === 'connectioncreate' || context.type === 'connectionremove') && !restoring && !transactionActive && !initializing) pendingConnectionSnapshot = snapshot()
    if (context.type === 'connectioncreated' || context.type === 'connectionremoved') {
      if (context.type === 'connectioncreated') {
        decorateConnection(context.data)
        void area.update('connection', context.data.id)
      }
      if (context.type === 'connectionremoved') {
        selectedConnectionIds.delete(context.data.id)
      }
      queueMicrotask(() => void refreshPortStates(true))
      updateMetrics()
      if (!restoring && !transactionActive && !initializing && pendingConnectionSnapshot) {
        undoStack.push(pendingConnectionSnapshot); redoStack.length = 0; pendingConnectionSnapshot = null
        callbacks.onDirty(); callbacks.onStatus(context.type === 'connectioncreated' ? 'Connection created' : 'Connection removed')
      }
    }
    if (context.type === 'nodepicked') {
      void clearConnectionSelection()
      dragSnapshot = snapshot()
      requestAnimationFrame(() => {
        const node = editor.getNode(context.data.id)
        callbacks.onSelection(node ? selectedNodeInfo(node) : null)
      })
    }
    if (context.type === 'nodedragged' && dragSnapshot) {
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
    await AreaExtensions.zoomAt(area, editor.getNodes(), { scale: 0.9 })
    callbacks.onStatus('View fitted')
  }
  requestAnimationFrame(() => resetView())

  return {
    destroy() {
      container.removeEventListener('origin-dynamic-output', dynamicOutputListener)
      container.removeEventListener('origin-connection-select', connectionSelectListener)
      container.removeEventListener('origin-connection-delete', connectionDeleteListener)
      document.removeEventListener('origin-control-change', controlChangeListener)
      destroyCuttingLine()
      area.destroy()
    },
    resetView,
    addNode,
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
    fitSelected,
    setVariables,
    updateSelectedNode,
    focusNode,
    setExecutionStates,
    clearExecutionStates
  }
}
