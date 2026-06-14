import { ClassicPreset, NodeEditor } from 'rete'
import { AreaExtensions, AreaPlugin, Drag } from 'rete-area-plugin'
import { ConnectionPlugin, Presets as ConnectionPresets } from 'rete-connection-plugin'
import { Presets as VuePresets, VuePlugin } from 'rete-vue-plugin'
import BlueprintControl from './BlueprintControl.vue'
import BlueprintNodeComponent from './BlueprintNode.vue'
import BlueprintSocket from './BlueprintSocket.vue'
import { createNode } from './nodeRegistry'
import { BlueprintNode, type Schemes } from './types'

type AreaExtra = import('rete-vue-plugin').VueArea2D<Schemes>
type Position = { x: number; y: number }

export interface NodeSnapshot {
  id: string
  typeId: string
  position: Position
  values: Record<string, unknown>
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

interface GraphSnapshot {
  nodes: NodeSnapshot[]
  connections: ConnectionSnapshot[]
  groups: GroupSnapshot[]
}

export interface GraphDocument extends GraphSnapshot {
  schemaVersion: 1
  graphName: string
  variables: unknown[]
  view: { x: number; y: number; zoom: number }
}

interface ClipboardGraph {
  nodes: Omit<NodeSnapshot, 'id'>[]
  connections: Array<Omit<ConnectionSnapshot, 'source' | 'target'> & { sourceIndex: number; targetIndex: number }>
}

export interface EditorMetrics {
  nodes: number
  connections: number
}

export interface BlueprintEditorHandle {
  destroy(): void
  resetView(): void
  addNode(typeId: string, clientPosition?: Position): Promise<void>
  deleteSelected(): Promise<void>
  selectAll(): Promise<void>
  deselectAll(): Promise<void>
  copy(): void
  cut(): Promise<void>
  paste(): Promise<void>
  undo(): Promise<void>
  redo(): Promise<void>
  getDocument(graphName?: string): GraphDocument
  loadDocument(document: GraphDocument): Promise<void>
  newDocument(): Promise<void>
  align(mode: 'horizontal-center' | 'vertical-center' | 'left' | 'right' | 'top' | 'bottom' | 'horizontal-distribute' | 'vertical-distribute' | 'straighten'): Promise<void>
  groupSelected(): Promise<void>
  ungroupSelected(): Promise<void>
  fitSelected(): Promise<void>
}

interface Callbacks {
  onZoom(value: number): void
  onStatus(value: string): void
  onMetrics(metrics: EditorMetrics): void
  onDirty(): void
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
  let selectedGroupId: string | null = null
  let dragSnapshot: GraphSnapshot | null = null
  let clipboard: ClipboardGraph | null = null
  let restoring = false

  render.addPreset(VuePresets.classic.setup({
    customize: {
      node: () => BlueprintNodeComponent,
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

  function graphPosition(clientPosition?: Position): Position {
    if (!clientPosition) {
      const rect = container.getBoundingClientRect()
      clientPosition = { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 }
    }
    const rect = container.getBoundingClientRect()
    const transform = area.area.transform
    return {
      x: (clientPosition.x - rect.left - transform.x) / transform.k,
      y: (clientPosition.y - rect.top - transform.y) / transform.k
    }
  }

  function snapshot(): GraphSnapshot {
    return {
      nodes: editor.getNodes().map(node => ({
        id: node.id,
        typeId: node.typeId ?? '',
        position: { ...(area.nodeViews.get(node.id)?.position ?? { x: 0, y: 0 }) },
        values: controlValues(node)
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
    await selector.unselectAll()
    await editor.clear()
    groups.splice(0, groups.length, ...(data.groups ?? []).map(item => ({ ...item, nodeIds: [...item.nodeIds] })))
    const nodes = new Map<string, BlueprintNode>()
    for (const item of data.nodes) {
      const node = createNode(item.typeId)
      node.id = item.id
      setControlValues(node, item.values)
      await editor.addNode(node)
      await area.translate(node.id, item.position)
      nodes.set(node.id, node)
    }
    for (const item of data.connections) {
      const source = nodes.get(item.source)
      const target = nodes.get(item.target)
      if (source && target && source.outputs[item.sourceOutput] && target.inputs[item.targetInput]) {
        await editor.addConnection(new ClassicPreset.Connection(source, item.sourceOutput, target, item.targetInput))
      }
    }
    restoring = false
    renderGroups()
    updateMetrics()
  }

  async function mutate(label: string, operation: () => Promise<void>) {
    if (!restoring) undoStack.push(snapshot())
    redoStack.length = 0
    await operation()
    updateMetrics()
    callbacks.onStatus(label)
    callbacks.onDirty()
  }

  function selectedNodes() {
    return editor.getNodes().filter(node => node.selected)
  }

  async function addNode(typeId: string, clientPosition?: Position) {
    await mutate('Node created', async () => {
      const node = createNode(typeId)
      await editor.addNode(node)
      await area.translate(node.id, graphPosition(clientPosition))
      await selector.unselectAll()
      await selectable.select(node.id, false)
    })
  }

  async function deleteSelected() {
    const selected = selectedNodes()
    if (!selected.length) return
    await mutate(`Deleted ${selected.length} node(s)`, async () => {
      const ids = new Set(selected.map(node => node.id))
      for (const item of editor.getConnections()) {
        if (ids.has(item.source) || ids.has(item.target)) await editor.removeConnection(item.id)
      }
      for (const node of selected) await editor.removeNode(node.id)
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
        values: controlValues(node)
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
        const node = createNode(item.typeId)
        setControlValues(node, item.values)
        await editor.addNode(node)
        await area.translate(node.id, { x: base.x + item.position.x, y: base.y + item.position.y })
        await selectable.select(node.id, true)
        nodes.push(node)
      }
      for (const item of clipboard!.connections) {
        const source = nodes[item.sourceIndex]
        const target = nodes[item.targetIndex]
        await editor.addConnection(new ClassicPreset.Connection(source, item.sourceOutput, target, item.targetInput))
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

  function getDocument(graphName = 'Untitled'): GraphDocument {
    const data = snapshot()
    return {
      schemaVersion: 1,
      graphName,
      ...data,
      variables: [],
      view: { x: area.area.transform.x, y: area.area.transform.y, zoom: area.area.transform.k }
    }
  }

  async function loadDocument(document: GraphDocument) {
    undoStack.length = 0; redoStack.length = 0
    await restore({ nodes: document.nodes ?? [], connections: document.connections ?? [], groups: document.groups ?? [] })
    if (document.view) {
      await area.area.translate(document.view.x, document.view.y)
      await area.area.zoom(document.view.zoom || 1)
    } else if (document.nodes?.length) await AreaExtensions.zoomAt(area, editor.getNodes(), { scale: 0.9 })
    callbacks.onStatus('Graph loaded')
  }

  async function newDocument() {
    undoStack.length = 0; redoStack.length = 0; groups.length = 0; selectedGroupId = null
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
    for (const node of editor.getNodes()) await selectable.select(node.id, true)
    callbacks.onStatus(`Selected ${editor.getNodes().length} node(s)`)
  }

  async function deselectAll() {
    await selector.unselectAll()
    callbacks.onStatus('Selection cleared')
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
      const left = rect.left + Math.min(start.x, current.x)
      const top = rect.top + Math.min(start.y, current.y)
      const right = rect.left + Math.max(start.x, current.x)
      const bottom = rect.top + Math.max(start.y, current.y)
      if (!event.ctrlKey) await selector.unselectAll()
      for (const node of editor.getNodes()) {
        const bounds = area.nodeViews.get(node.id)?.element.getBoundingClientRect()
        if (bounds && bounds.right >= left && bounds.left <= right && bounds.bottom >= top && bounds.top <= bottom) {
          await selectable.select(node.id, true)
        }
      }
      start = null
      rectangle.style.display = 'none'
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
    }

    container.addEventListener('pointerdown', event => {
      const target = event.target as HTMLElement
      if (event.button !== 0 || target.closest('.blueprint-node, .blueprint-socket, input')) return
      const rect = container.getBoundingClientRect()
      start = { x: event.clientX - rect.left, y: event.clientY - rect.top }
      window.addEventListener('pointermove', move)
      window.addEventListener('pointerup', up)
    })
  }

  area.addPipe(context => {
    if (context.type === 'zoomed') callbacks.onZoom(context.data.zoom)
    if (context.type === 'connectioncreated') updateMetrics()
    if (context.type === 'connectionremoved') updateMetrics()
    if (context.type === 'nodepicked') dragSnapshot = snapshot()
    if (context.type === 'nodedragged' && dragSnapshot) {
      undoStack.push(dragSnapshot); redoStack.length = 0; dragSnapshot = null; callbacks.onDirty(); callbacks.onStatus('Node moved')
    }
    return context
  })
  setupRubberBandSelection()

  const initial = [
    { typeId: 'origin.event.begin', position: { x: 90, y: 115 } },
    { typeId: 'origin.flow.for-loop', position: { x: 390, y: 90 } },
    { typeId: 'origin.flow.branch', position: { x: 720, y: 65 } },
    { typeId: 'origin.cast.integer-string', position: { x: 720, y: 235 } },
    { typeId: 'origin.action.print', position: { x: 1040, y: 100 } }
  ]
  const initialNodes: BlueprintNode[] = []
  for (const item of initial) {
    const node = createNode(item.typeId)
    await editor.addNode(node)
    await area.translate(node.id, item.position)
    initialNodes.push(node)
  }
  await editor.addConnection(new ClassicPreset.Connection(initialNodes[0], 'exec', initialNodes[1], 'exec'))
  await editor.addConnection(new ClassicPreset.Connection(initialNodes[1], 'body', initialNodes[2], 'exec'))
  await editor.addConnection(new ClassicPreset.Connection(initialNodes[1], 'index', initialNodes[3], 'value'))
  await editor.addConnection(new ClassicPreset.Connection(initialNodes[2], 'true', initialNodes[4], 'exec'))
  await editor.addConnection(new ClassicPreset.Connection(initialNodes[3], 'result', initialNodes[4], 'value'))
  updateMetrics()

  async function resetView() {
    await AreaExtensions.zoomAt(area, editor.getNodes(), { scale: 0.9 })
    callbacks.onStatus('View fitted')
  }
  requestAnimationFrame(() => resetView())

  return {
    destroy() { area.destroy() },
    resetView,
    addNode,
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
    fitSelected
  }
}
