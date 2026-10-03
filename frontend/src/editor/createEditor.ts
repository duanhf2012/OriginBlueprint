import { ClassicPreset, NodeEditor, type Scope } from 'rete'
import { AreaExtensions, AreaPlugin, Drag } from 'rete-area-plugin'
import { ConnectionPlugin, Presets as ConnectionPresets } from 'rete-connection-plugin'
import { getDOMSocketPosition, type SocketPositionWatcher } from 'rete-render-utils'
import { Presets as VuePresets, VuePlugin } from 'rete-vue-plugin'
import BlueprintControl from './BlueprintControl.vue'
import BlueprintConnectionComponent from './BlueprintConnection.vue'
import BlueprintNodeComponent from './BlueprintNode.vue'
import BlueprintSocket from './BlueprintSocket.vue'
import { applyTimerFunctionMetadata, applyVariableNodePresentation, createFunctionCallNode, createFunctionEntryNode as createFunctionEntryNodeFromSpec, createFunctionReturnNode as createFunctionReturnNodeFromSpec, createLegacyNode, createNode, createSetTimerByFunctionNode, createVariableNode, findNodeDefinition, hasNodeDefinition, nodeTitleWidth, resolveNodeLegacyClass, signaturePortTips } from './nodeRegistry'
import { normalizeSocketName } from './socketTheme'
import { BlueprintNode, RefSelectControl, type Schemes } from './types'
import { describeEntryBinding, entryBindingCandidateGroups, isEntryOutputConnection, type EntryBindingNode } from './implicitEntryLinks'
import { refreshNodePortStates } from './portVisualState'
import { pathIntersectsRect, rectsIntersect, type Rect } from './selectionGeometry'
import { execOutputReplacementIds } from './connectionPolicy'
import { normalizeNodeInputDefault, type ConnectionSnapshot, type FunctionNodeMetadata, type FunctionSignature, type GraphDocument, type GraphSnapshot, type GraphVariable, type GraphVariableGroup, type GroupSnapshot, type CommentSnapshot, type MacroRefBoundary, type MacroRefSnapshot, type LegacyGraphState, type NodeProperties, type NodeSnapshot, type RestoreLossReport } from './document'
import { splitSnapshotForPersistence, snapshotHasMacroRef, type MacroMirrorPayload, type MacroMirrorSite } from './macroRefs'
import { buildRestorePlan, normalizeDynamicOutputCount } from './restorePlan'
import { pushBoundedHistory } from './history'
import { functionEntryTypeId, functionReturnTypeId, isCopyableFunctionNode, isPasteableFunctionNode, planFunctionTerminalDeletion } from './functionTerminalPolicy'
import { nodeSelectionPointerIntent, shouldCollapsePreservedSelection } from './nodeSelectionPolicy'
import { cloneGraphJSONValue } from '../graphJSON'

export type { FunctionSignature, FunctionSignaturePort, GraphDocument, GraphVariable, GraphVariableGroup, ValidationIssue, VariableScope, VariableType } from './document'

type AreaExtra = import('rete-vue-plugin').VueArea2D<Schemes>
type Position = { x: number; y: number }
type SocketWatcher = SocketPositionWatcher<Scope<never, [AreaExtra]>>
const nodeLocateZoomScale = 0.48
const nodeLocateMinZoomScale = 0.34
const nodeLocateViewportAnchor = { x: 0.5, y: 0.28 }
const issueHighlightZoomScale = nodeLocateZoomScale
const maxDynamicSequenceOutputs = 256

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
  functionEntries: number
  functionReturns: number
}

export interface SelectedNodeInfo {
  id: string
  typeId: string
  label: string
  description?: string
  values: Record<string, unknown>
  variableId?: string
  inputs?: Array<{ key: string; label: string; tip?: string; portId?: number }>
}

interface EditorHistorySnapshot {
  graph: GraphSnapshot
  legacy?: LegacyGraphState
}

export interface AddNodeOptions {
  allowEntryNodes?: boolean
}

export interface InsertMacroRefPayload extends MacroMirrorPayload {
  clientPosition?: Position
}

export interface BlueprintEditorHandle {
  destroy(): void
  resetView(): void
  addNode(typeId: string, clientPosition?: Position, options?: AddNodeOptions): Promise<void>
  addFunctionCallNode(spec: FunctionNodeMetadata, clientPosition?: Position): Promise<void>
  addFunctionEntryNode(spec: FunctionNodeMetadata, clientPosition?: Position): Promise<void>
  addFunctionReturnNode(spec: FunctionNodeMetadata, clientPosition?: Position): Promise<void>
  syncFunctionSignature(spec: FunctionNodeMetadata): Promise<void>
  refreshNodeTypeAnnotations(typeId: string): Promise<number>
  refreshFunctionNodeAnnotations(functionId: string, description?: string, portTipsById?: Map<string, string>, refsById?: Map<string, string>): Promise<number>
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
  loadDocument(document: GraphDocument): Promise<RestoreLossReport>
  newDocument(): Promise<void>
  align(mode: 'horizontal-center' | 'vertical-center' | 'left' | 'right' | 'top' | 'bottom' | 'horizontal-distribute' | 'vertical-distribute'): Promise<void>
  groupSelected(): Promise<void>
  ungroupSelected(): Promise<void>
  toggleGroupSelected(): Promise<void>
  addCommentAt(position?: { x: number; y: number }): Promise<void>
  commentAroundSelection(): Promise<void>
  insertMacroRef(payload: InsertMacroRefPayload): Promise<boolean>
  selectedMacroRef(): { macroId: string; label: string; pathHint: string; nodeCount: number; missing: boolean } | null
  hasMacroRef(macroId: string): boolean
  setMacroResolver(resolver: ((macroId: string) => Promise<MacroMirrorPayload | null>) | null): void
  refreshMacroRef(macroId: string, payload: MacroMirrorPayload): Promise<boolean>
  refreshMacroFrames(macroId: string, label: string): boolean
  searchNodes(query: string): Array<{ nodeId: string; title: string; typeId: string; detail: string }>
  focusComment(id: string): Promise<void>
  commentCount(): number
  fitSelected(): Promise<void>
  setVariables(variables: GraphVariable[], variableGroups?: GraphVariableGroup[], refreshNodes?: boolean): Promise<void>
  refreshVariableNodePresentation(variable: GraphVariable): Promise<void>
  setCallableFunctions(functions: FunctionNodeMetadata[]): Promise<void>
  focusNode(id: string): Promise<void>
  highlightNodesByType(typeId: string): Promise<number>
  highlightFunctionReferences(functionKey: string): Promise<number>
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
  locale?(): string
}

function controlValues(node: BlueprintNode) {
  const values: Record<string, unknown> = {}
  for (const [key, port] of Object.entries(node.inputs)) {
    const control = port?.control as ClassicPreset.InputControl<'text' | 'number'> | null | undefined
    if (control) values[key] = normalizeNodeInputDefault(port?.socket.name ?? '', control.value)
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
  let accumulateNodeSelection = false
  const selectable = AreaExtensions.selectableNodes(area, selector, { accumulating: { active: () => accumulateNodeSelection } })
  const undoStack: EditorHistorySnapshot[] = []
  const redoStack: EditorHistorySnapshot[] = []
  const groups: GroupSnapshot[] = []
  const groupElements = new Map<string, HTMLElement>()
  const comments: CommentSnapshot[] = []
  // 宏引用（v2 引用模型）：引用图只存 macroId + 边界连线；宏内容以 .obpm 源为准，
  // 在编辑器里展开为只读镜像节点画在宏框内（保存时由 splitSnapshotForPersistence 拆回）。
  interface MacroRefState {
    macroId: string
    pathHint: string
    commentId: string
    label: string
    frame: { x: number; y: number; width: number; height: number }
    boundary: MacroRefBoundary[]
    /** 宏源节点顺序 → 镜像节点 id（''=该位置无镜像）。 */
    mirrorIds: string[]
    missing: boolean
  }
  const macroRefs: MacroRefState[] = []
  const macroMirrorByNodeId = new Map<string, { ref: MacroRefState; sourceIndex: number }>()
  let macroResolver: ((macroId: string) => Promise<MacroMirrorPayload | null>) | null = null
  // buildMacroMirrors 程序化重建宏内部连线期间置位：镜像↔镜像连线拦截只针对用户手拉。
  let buildingMacroMirrors = false
  const commentElements = new Map<string, HTMLElement>()
  let selectedCommentId: string | null = null
  let editingCommentId: string | null = null
  const selectedConnectionIds = new Set<string>()
  let selectedGroupId: string | null = null
  let editingGroupId: string | null = null
  let preservedMultiNodeSelection: string[] = []
  let preservedMultiNodeSelectionPickedId = ''
  let dragSnapshot: EditorHistorySnapshot | null = null
  let nodeDragMoved = false
  let clipboard: ClipboardGraph | null = null
  let restoring = false
  let transactionActive = false
  let initializing = true
  let pendingConnectionSnapshot: EditorHistorySnapshot | null = null
  let pendingExecReplacementSnapshot: EditorHistorySnapshot | null = null
  let replacingExecConnection = false
  let controlEditSnapshot: EditorHistorySnapshot | null = null
  let controlEditChanged = false
  let currentVariables: GraphVariable[] = []
  let currentGraphName = ''
  let currentVariableGroups: GraphVariableGroup[] = []
  let currentLegacy: LegacyGraphState | undefined
  let callableFunctions: FunctionNodeMetadata[] = []
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
      accumulateNodeSelection = event.ctrlKey || event.metaKey
      if (!target.closest('.blueprint-node') || target.closest('.blueprint-socket, input, textarea, select, button')) {
        preservedMultiNodeSelection = []
        preservedMultiNodeSelectionPickedId = ''
        return
      }
      const pickedNodeId = nodeIdFromEventTarget(target)
      const ids = selectedNodes().map(node => node.id)
      const intent = nodeSelectionPointerIntent(ids, pickedNodeId, accumulateNodeSelection)
      preservedMultiNodeSelection = intent.preservedIds
      preservedMultiNodeSelectionPickedId = intent.preservedIds.length ? pickedNodeId : ''
    }
    const resetAccumulation = () => { accumulateNodeSelection = false }
    container.addEventListener('pointerdown', rememberSelection, true)
    window.addEventListener('pointerup', resetAccumulation, true)
    window.addEventListener('pointercancel', resetAccumulation, true)
    return () => {
      container.removeEventListener('pointerdown', rememberSelection, true)
      window.removeEventListener('pointerup', resetAccumulation, true)
      window.removeEventListener('pointercancel', resetAccumulation, true)
    }
  }

  function nodeIdFromEventTarget(target: HTMLElement) {
    for (const [id, view] of area.nodeViews) {
      if (view.element.contains(target)) return id
    }
    return ''
  }

  async function restoreMultiSelectionAfterNodePick(pickedId: string) {
    const ids = preservedMultiNodeSelection
    if (ids.length < 2 || !ids.includes(pickedId)) return
    for (const id of ids) {
      if (editor.getNode(id)) await selectable.select(id, true)
    }
    callbacks.onStatus(`Selected ${ids.length} node(s)`)
  }

  async function finishPreservedMultiSelectionClick(pickedId: string) {
    const shouldCollapse = pickedId === preservedMultiNodeSelectionPickedId
      && shouldCollapsePreservedSelection(preservedMultiNodeSelection, pickedId, nodeDragMoved)
    preservedMultiNodeSelection = []
    preservedMultiNodeSelectionPickedId = ''
    if (!shouldCollapse || !editor.getNode(pickedId)) return
    await selector.unselectAll()
    await selectable.select(pickedId, false)
    const node = editor.getNode(pickedId)
    callbacks.onSelection(node ? selectedNodeInfo(node) : null)
    callbacks.onStatus('Node selected')
  }

  const stopNodeDragFeedback = () => setInteractionClass('is-dragging-node', false)

  function startNodeDragFeedback() {
    setInteractionClass('is-dragging-node', true)
    window.addEventListener('pointerup', stopNodeDragFeedback, { once: true })
    window.addEventListener('pointercancel', stopNodeDragFeedback, { once: true })
  }

  function updateMetrics() {
    const nodes = editor.getNodes()
    callbacks.onMetrics({
      nodes: nodes.length,
      connections: editor.getConnections().length,
      functionEntries: nodes.filter(node => node.typeId === functionEntryTypeId).length,
      functionReturns: nodes.filter(node => node.typeId === functionReturnTypeId).length
    })
  }

  async function clearConnectionSelection(exceptId?: string) {
    for (const id of [...selectedConnectionIds]) {
      if (id === exceptId) continue
      selectedConnectionIds.delete(id)
      const item = editor.getConnection(id)
      if (item) { item.selected = false; await area.update('connection', id) }
    }
  }

  async function clearGroupSelection() {
    if (!selectedGroupId) return
    selectedGroupId = null
    selectedCommentId = null
    updateGroupSelectionClasses()
  }

  async function selectGroup(id: string) {
    await clearConnectionSelection()
    await selector.unselectAll()
    selectedGroupId = id
    selectedCommentId = null
    updateGroupSelectionClasses()
    updateCommentSelectionClasses()
    callbacks.onSelection(null)
  }

  // === 画布注释便签：多行文本 + 拖动/缩放，双击编辑，Delete 删除，随文档保存。 ===
  function renderComments() {
    for (const element of commentElements.values()) element.remove()
    commentElements.clear()
    for (const comment of comments) {
      // 宏框单独打标：巨大元素上的 backdrop-filter 在平移重绘时会出现分块接缝，需关闭。
      const isMacroFrame = macroRefs.some(ref => ref.commentId === comment.id)
      const element = document.createElement('div')
      element.className = `node-comment${isMacroFrame ? ' macro-frame' : ''}${selectedCommentId === comment.id ? ' selected' : ''}`
      element.style.width = `${comment.width}px`
      element.style.height = `${comment.height}px`
      element.style.transform = `translate(${comment.x}px, ${comment.y}px)`
      const text = document.createElement('div')
      text.className = 'node-comment-text'
      const grip = document.createElement('div')
      grip.className = 'node-comment-resize'
      element.append(text, grip)
      area.area.content.holder.prepend(element)
      commentElements.set(comment.id, element)
      element.addEventListener('pointerdown', event => {
        void selectComment(comment.id)
        event.stopPropagation()
      })
      if (editingCommentId === comment.id) {
        const input = document.createElement('textarea')
        input.className = 'node-comment-input'
        input.value = comment.text
        text.replaceChildren(input)
        const finish = (save: boolean) => {
          if (save) {
            comment.text = input.value
            callbacks.onDirty()
          }
          editingCommentId = null
          renderComments()
        }
        input.onkeydown = event => {
          event.stopPropagation()
          if (event.key === 'Escape') finish(false)
        }
        input.onblur = () => finish(true)
        requestAnimationFrame(() => input.focus())
      } else {
        text.textContent = comment.text
      }
      text.ondblclick = event => {
        event.stopPropagation()
        // 宏框是引用的整体呈现：标题是宏名的实时显示，不可编辑（改宏名请编辑源宏）。
        if (macroRefs.some(ref => ref.commentId === comment.id)) {
          callbacks.onStatus('宏框标题来自宏名（实时显示），不可编辑；修改宏名请编辑源宏')
          return
        }
        editingCommentId = comment.id
        renderComments()
      }
      grip.onpointerdown = event => beginCommentDrag(event, comment, true)
      element.onpointerdown = event => beginCommentDrag(event, comment, false)
    }
  }

  function updateCommentSelectionClasses() {
    for (const [id, element] of commentElements) element.classList.toggle('selected', selectedCommentId === id)
  }

  async function selectComment(id: string) {
    await clearConnectionSelection()
    await selector.unselectAll()
    selectedGroupId = null
    selectedCommentId = null
    selectedCommentId = id
    updateGroupSelectionClasses()
    updateCommentSelectionClasses()
    callbacks.onSelection(null)
  }

  function beginCommentDrag(event: PointerEvent, comment: CommentSnapshot, resize: boolean) {
    void selectComment(comment.id)
    event.stopPropagation(); event.preventDefault()
    const before = historySnapshot()
    const start = { x: event.clientX, y: event.clientY, gx: comment.x, gy: comment.y, width: comment.width, height: comment.height }
    // 同虚幻：拖动注释框时，完全在框内的节点跟随移动。
    const containedNodes = resize ? [] : editor.getNodes().filter(node => {
      const position = area.nodeViews.get(node.id)?.position ?? { x: 0, y: 0 }
      const size = nodeSize(node)
      return position.x >= comment.x && position.y >= comment.y
        && position.x + size.width <= comment.x + comment.width
        && position.y + size.height <= comment.y + comment.height
    })
    const nodeStarts = new Map(containedNodes.map(node => [node.id, { ...(area.nodeViews.get(node.id)?.position ?? { x: 0, y: 0 }) }]))
    const move = (next: PointerEvent) => {
      const dx = (next.clientX - start.x) / area.area.transform.k
      const dy = (next.clientY - start.y) / area.area.transform.k
      if (resize) {
        comment.width = Math.max(120, start.width + dx); comment.height = Math.max(60, start.height + dy)
      } else {
        comment.x = start.gx + dx; comment.y = start.gy + dy
        for (const [id, position] of nodeStarts) void area.translate(id, { x: position.x + dx, y: position.y + dy })
      }
      const element = commentElements.get(comment.id)
      if (element) { element.style.width = `${comment.width}px`; element.style.height = `${comment.height}px`; element.style.transform = `translate(${comment.x}px, ${comment.y}px)` }
    }
    const up = () => {
      window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up)
      pushUndoHistory(before); redoStack.length = 0; callbacks.onDirty(); callbacks.onStatus(resize ? 'Comment resized' : 'Comment moved')
    }
    window.addEventListener('pointermove', move); window.addEventListener('pointerup', up)
  }

  // 画布内搜索：节点标题/类型/副标题/参数值 + 注释文本；命中后可居中定位。
  function searchNodes(query: string): Array<{ nodeId: string; title: string; typeId: string; detail: string }> {
    const text = query.trim().toLowerCase()
    if (!text) return []
    const results: Array<{ nodeId: string; title: string; typeId: string; detail: string }> = []
    for (const node of editor.getNodes()) {
      const title = node.label || node.typeId || ''
      const haystack = [
        title,
        node.typeId ?? '',
        node.subtitle ?? '',
        node.functionId ?? '',
        ...Object.values(controlValues(node)).map(value => String(value ?? ''))
      ].join('\n').toLowerCase()
      if (!haystack.includes(text)) continue
      const detail = node.typeId ?? title
      results.push({ nodeId: node.id, title, typeId: node.typeId ?? '', detail })
      if (results.length >= 50) break
    }
    for (const comment of comments) {
      if (!comment.text.toLowerCase().includes(text)) continue
      results.push({ nodeId: `comment:${comment.id}`, title: comment.text.split('\n')[0] ?? '注释', typeId: '注释', detail: '注释便签' })
      if (results.length >= 50) break
    }
    return results
  }

  async function focusComment(id: string) {
    const comment = comments.find(item => item.id === id)
    if (!comment) return
    await selector.unselectAll()
    selectedCommentId = comment.id
    updateCommentSelectionClasses()
    const centerX = comment.x + comment.width / 2
    const centerY = comment.y + comment.height / 2
    const rect = container.getBoundingClientRect()
    const transform = area.area.transform
    await area.area.translate(-centerX * transform.k + rect.width / 2, -centerY * transform.k + rect.height / 2)
  }

  function commentCount() {
    return comments.length
  }

  async function commentAroundSelection() {
    const nodes = selectedNodes()
    if (!nodes.length) return addCommentAt()
    const entries = nodes.map(node => ({ position: area.nodeViews.get(node.id)?.position ?? { x: 0, y: 0 }, size: nodeSize(node) }))
    const minX = Math.min(...entries.map(e => e.position.x)), minY = Math.min(...entries.map(e => e.position.y))
    const maxX = Math.max(...entries.map(e => e.position.x + e.size.width)), maxY = Math.max(...entries.map(e => e.position.y + e.size.height))
    await mutate('Comment added', async () => {
      comments.push({ id: crypto.randomUUID(), text: '', x: minX - 24, y: minY - 34, width: maxX - minX + 48, height: maxY - minY + 62 })
      editingCommentId = comments[comments.length - 1]!.id
      renderComments()
    })
  }

  async function addCommentAt(position?: { x: number; y: number }) {
    const view = position ?? (() => {
      const rect = container.getBoundingClientRect()
      return graphPosition({ x: rect.left + rect.width * 0.42, y: rect.top + rect.height * 0.36 })
    })()
    await mutate('Comment added', async () => {
      comments.push({ id: crypto.randomUUID(), text: '', x: view.x - 90, y: view.y - 40, width: 200, height: 110 })
      editingCommentId = comments[comments.length - 1]!.id
      renderComments()
    })
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
    await clearGroupSelection()
    callbacks.onSelection(null)
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
    return value ? cloneGraphJSONValue(value) : undefined
  }

  function historySnapshot(): EditorHistorySnapshot {
    return { graph: snapshot(), legacy: cloneLegacyState(currentLegacy) }
  }

  function pushUndoHistory(value: EditorHistorySnapshot) {
    pushBoundedHistory(undoStack, value)
  }

  function pushRedoHistory(value: EditorHistorySnapshot) {
    pushBoundedHistory(redoStack, value)
  }

  async function restoreHistory(value: EditorHistorySnapshot) {
    currentLegacy = cloneLegacyState(value.legacy)
    await restore(value.graph)
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
		node.legacyClass = properties?.legacyClass || node.legacyClass || resolveNodeLegacyClass(node.typeId, properties?.legacyClass)
		node.legacyModule = properties?.legacyModule || node.legacyModule
		if (properties?.legacyInputs) node.legacyInputs = properties.legacyInputs.map(port => ({ ...port }))
		if (properties?.legacyOutputs) node.legacyOutputs = properties.legacyOutputs.map(port => ({ ...port }))
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

  function createTimerFunctionNodeFromProperties(properties?: NodeProperties) {
    const selected = properties?.functionId || properties?.functionName ? { ...functionMetadataFromProperties(properties), functionRole: 'timer' as const } : undefined
    return createSetTimerByFunctionNode(callableFunctions, selected, callbacks.locale?.() ?? 'zh-CN', (node, functionId) => { void changeTimerFunction(node, functionId) })
  }

	function createRetiredTimerNode(typeId: string, properties?: NodeProperties) {
		const ports: Record<string, { legacyClass: string; inputs: Array<[string, string]>; outputs: Array<[string, string]> }> = {
			'origin.timer.clear': { legacyClass: 'ClearTimer', inputs: [['exec', 'exec'], ['timerHandle', 'timerhandle'], ['cancelRunningCallback', 'boolean']], outputs: [['then', 'exec'], ['success', 'boolean']] },
			'origin.timer.pause': { legacyClass: 'PauseTimer', inputs: [['exec', 'exec'], ['timerHandle', 'timerhandle']], outputs: [['then', 'exec'], ['success', 'boolean']] },
			'origin.timer.unpause': { legacyClass: 'UnpauseTimer', inputs: [['exec', 'exec'], ['timerHandle', 'timerhandle']], outputs: [['then', 'exec'], ['success', 'boolean']] },
			'origin.timer.is-active': { legacyClass: 'IsTimerActive', inputs: [['timerHandle', 'timerhandle']], outputs: [['active', 'boolean']] },
			'origin.timer.is-paused': { legacyClass: 'IsTimerPaused', inputs: [['timerHandle', 'timerhandle']], outputs: [['paused', 'boolean']] },
			'origin.timer.is-valid': { legacyClass: 'IsTimerValid', inputs: [['timerHandle', 'timerhandle']], outputs: [['valid', 'boolean']] },
			'origin.timer.remaining': { legacyClass: 'GetTimerRemaining', inputs: [['timerHandle', 'timerhandle']], outputs: [['remaining', 'integer']] },
			'origin.timer.elapsed': { legacyClass: 'GetTimerElapsed', inputs: [['timerHandle', 'timerhandle']], outputs: [['elapsed', 'integer']] }
		}
		if (typeId === 'origin.timer.set-by-function') {
			const dynamicInputs: Array<[string, string]> = [['exec', 'exec'], ['time', 'integer'], ['looping', 'boolean'], ['firstDelay', 'integer']]
			properties?.functionSignature?.inputs.forEach((port, index) => dynamicInputs.push([functionPortKey('input', port, index), normalizeSocketName(port.type)]))
			ports[typeId] = { legacyClass: 'SetTimerByFunction', inputs: dynamicInputs, outputs: [['then', 'exec'], ['timerHandle', 'timerhandle']] }
		}
		const spec = ports[typeId]
		if (!spec) return null
		return createLegacyNode({
			...properties,
			label: properties?.label || spec.legacyClass,
			legacyClass: properties?.legacyClass || spec.legacyClass,
			legacyModule: properties?.legacyModule || 'retired-timer',
			legacyInputs: properties?.legacyInputs ?? spec.inputs.map(([key, type]) => ({ key, label: key, type })),
			legacyOutputs: properties?.legacyOutputs ?? spec.outputs.map(([key, type]) => ({ key, label: key, type }))
		})
	}

  async function changeTimerFunction(node: BlueprintNode, functionId: string) {
    const metadata = callableFunctions.find(item => item.functionId === functionId)
    if (!metadata) {
      callbacks.onStatus('选择的函数不存在或尚未加载')
      return
    }
    await mutate('Timer callback changed', async () => {
      const previousValues = controlValues(node)
      const nextInputs = new Map((metadata.functionSignature?.inputs ?? []).map((port, index) => [
        functionPortKey('input', port, index),
        normalizeSocketName(port.type),
      ]))
      for (const connection of [...editor.getConnections()]) {
        if (connection.target !== node.id || !String(connection.targetInput).startsWith('input_')) continue
        const key = String(connection.targetInput)
        const currentType = normalizeSocketName(node.inputs[key]?.socket.name)
        if (!nextInputs.has(key) || nextInputs.get(key) !== currentType) await editor.removeConnection(connection.id)
      }
      applyTimerFunctionMetadata(node, { ...metadata, functionRole: 'timer' })
      setControlValues(node, previousValues)
      node.functionReferenceMissing = false
      await area.update('node', node.id)
      await refreshPortStates(true)
      callbacks.onSelection(selectedNodeInfo(node))
    })
  }

  function createRestoredNode(item: Pick<NodeSnapshot, 'typeId' | 'properties'>, typeId: string, variablesPool: GraphVariable[] = currentVariables) {
    const variableAccess = item.properties?.variableAccess ?? (typeId === 'origin.variable.set' ? 'set' : 'get')
    const variable = variablesPool.find(entry => entry.id === item.properties?.variableId)
    if (typeId.startsWith('origin.variable.')) {
      return createVariableNode(
        variable ?? { id: item.properties?.variableId ?? '', name: 'Missing Variable', type: 'string', defaultValue: '', groupId: 'default' },
        variableAccess
      )
    }
		const retiredTimer = createRetiredTimerNode(typeId, item.properties)
		if (retiredTimer) return retiredTimer
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
    const inputs = new Map<string, string>()
    const outputs = new Map<string, string>()
    if (node.typeId === 'origin.function.entry') {
      outputs.set('exec', 'exec')
      signature.inputs.forEach((port, index) => outputs.set(functionPortKey('input', port, index), normalizeSocketName(port.type)))
    } else if (node.typeId === 'origin.function.return') {
      inputs.set('exec', 'exec')
      signature.outputs.forEach((port, index) => inputs.set(functionPortKey('output', port, index), normalizeSocketName(port.type)))
    } else if (node.typeId === 'origin.function.call') {
      inputs.set('exec', 'exec')
      outputs.set('exec', 'exec')
      signature.inputs.forEach((port, index) => inputs.set(functionPortKey('input', port, index), normalizeSocketName(port.type)))
      signature.outputs.forEach((port, index) => outputs.set(functionPortKey('output', port, index), normalizeSocketName(port.type)))
    } else if (node.typeId === 'origin.timer.set-by-function') {
      inputs.set('exec', 'exec')
      inputs.set('time', 'integer')
      inputs.set('looping', 'boolean')
      inputs.set('firstDelay', 'integer')
      outputs.set('then', 'exec')
      outputs.set('timerHandle', 'timerhandle')
      signature.inputs.forEach((port, index) => inputs.set(functionPortKey('input', port, index), normalizeSocketName(port.type)))
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
      const sourceType = sourcePorts?.outputs.get(connection.sourceOutput)
        ?? normalizeSocketName(editor.getNode(connection.source)?.outputs[connection.sourceOutput]?.socket.name)
      const targetType = targetPorts?.inputs.get(connection.targetInput)
        ?? normalizeSocketName(editor.getNode(connection.target)?.inputs[connection.targetInput]?.socket.name)
		const callbackToExec = sourceType === 'callback' && targetType === 'exec'
      if (sourceType && targetType && !callbackToExec && sourceType !== targetType && sourceType !== 'any' && targetType !== 'any') return false
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
        },
        ...(node.mirrorMacro ? { mirrorMacro: true } : {})
      })),
      connections: editor.getConnections().map(item => ({
        source: item.source,
        sourceOutput: String(item.sourceOutput),
        target: item.target,
        targetInput: String(item.targetInput),
        legacyEdgeId: item.legacyEdgeId,
        legacyOrdinal: item.legacyOrdinal,
        ...(visibleEntryConnectionIds.has(item.id) ? { entryConnectionVisible: true } : {})
      })),
      groups: groups.map(item => ({ ...item, nodeIds: [...item.nodeIds] })),
      comments: comments.map(item => ({ ...item })),
      // 宏框注释与镜像一样是运行时对象：撤销快照原样保留，持久化时由拆分器剔除。
      macroRefs: macroRefs.map(ref => ({
        macroId: ref.macroId,
        pathHint: ref.pathHint,
        commentId: ref.commentId,
        frame: liveMacroFrame(ref),
        boundary: ref.boundary.map(item => ({ ...item })),
        mirrorIds: [...ref.mirrorIds]
      }))
    }
  }

  function liveMacroFrame(ref: MacroRefState) {
    const comment = comments.find(item => item.id === ref.commentId)
    if (comment) return { x: comment.x, y: comment.y, width: comment.width, height: comment.height }
    return { ...ref.frame }
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
      element.innerHTML = `<div class="node-group-title"></div><div class="node-group-resize"></div>`
      area.area.content.holder.prepend(element)
      groupElements.set(group.id, element)

      element.addEventListener('pointerdown', event => {
        void selectGroup(group.id)
        event.stopPropagation()
      })
      const title = element.querySelector('.node-group-title') as HTMLElement
      if (editingGroupId === group.id) {
        const input = document.createElement('input')
        input.className = 'node-group-title-input'
        input.value = group.title
        title.replaceChildren(input)
        const finish = (save: boolean) => {
          if (save && input.value.trim()) {
            group.title = input.value.trim()
            callbacks.onDirty()
          }
          editingGroupId = null
          renderGroups()
        }
        input.onkeydown = event => {
          if (event.key === 'Enter') finish(true)
          if (event.key === 'Escape') finish(false)
        }
        input.onblur = () => finish(true)
        requestAnimationFrame(() => { input.focus(); input.select() })
      } else {
        title.textContent = group.title
      }
      title.ondblclick = event => {
        event.stopPropagation()
        editingGroupId = group.id
        renderGroups()
      }
      title.onpointerdown = event => beginGroupDrag(event, group, false)
      const grip = element.querySelector('.node-group-resize') as HTMLElement
      grip.onpointerdown = event => beginGroupDrag(event, group, true)
    }
  }

  function updateGroupSelectionClasses() {
    for (const [id, element] of groupElements) element.classList.toggle('selected', selectedGroupId === id)
  }

  function beginGroupDrag(event: PointerEvent, group: GroupSnapshot, resize: boolean) {
    void selectGroup(group.id)
    event.stopPropagation(); event.preventDefault()
    const before = historySnapshot()
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
      pushUndoHistory(before); redoStack.length = 0; callbacks.onDirty(); callbacks.onStatus(resize ? 'Group resized' : 'Group moved')
    }
    window.addEventListener('pointermove', move); window.addEventListener('pointerup', up)
  }

  async function restore(data: GraphSnapshot) {
    restoring = true
    try {
      visibleEntryConnectionIds.clear()
      selectedConnectionIds.clear()
      await selector.unselectAll()
      await editor.clear()
      groups.splice(0, groups.length, ...(data.groups ?? []).map(item => ({ ...item, nodeIds: [...item.nodeIds] })))
      comments.splice(0, comments.length, ...(data.comments ?? []).map(item => ({ ...item })))
      restoreMacroRefStates(data.macroRefs ?? [])
      selectedCommentId = null
      editingCommentId = null
      const plan = buildRestorePlan(data, (item, typeId) => {
        const node = createRestoredNode(item, typeId)
        if (!node) return null
        const alteredNodes: RestoreLossReport['alteredNodes'] = []
        applyNodeProperties(node, item.properties)
        if (node.dynamicOutputs) {
          const requested = item.properties?.dynamicOutputCount ?? 3
          const restoredValue = normalizeDynamicOutputCount(requested)
          if (requested !== 0 && (!Number.isFinite(requested) || !Number.isInteger(requested) || requested < 1 || requested > maxDynamicSequenceOutputs)) {
            alteredNodes.push({ id: item.id, typeId, reason: 'invalid-dynamic-output-count', originalValue: requested, restoredValue })
          }
          setDynamicOutputCount(node, restoredValue)
        }
        node.id = item.id
        if (item.properties?.label && !typeId.startsWith('origin.variable.') && !item.properties.legacyClass) {
          node.label = item.properties.label
          node.width = Math.max(node.width ?? 230, nodeTitleWidth(node.label))
        }
        setControlValues(node, item.values)
        syncDynamicBranchOutputs(node, dynamicBranchValueCount(node))
        if (item.mirrorMacro) node.mirrorMacro = true
        return {
          snapshot: item,
          node,
          inputKeys: Object.keys(node.inputs),
          outputKeys: Object.keys(node.outputs),
          alteredNodes,
        }
      })
      const nodes = new Map<string, BlueprintNode>()
      for (const prepared of plan.nodes) {
        const { snapshot: item, node } = prepared
        await editor.addNode(node)
        await area.translate(node.id, item.position)
        nodes.set(node.id, node)
      }
      for (const item of plan.connections) {
        const source = nodes.get(item.source)
        const target = nodes.get(item.target)
        if (!source || !target) continue
        const connection = createConnection(source, item.sourceOutput, target, item.targetInput)
        connection.legacyEdgeId = item.legacyEdgeId
        connection.legacyOrdinal = item.legacyOrdinal
        if (item.entryConnectionVisible) {
          visibleEntryConnectionIds.add(connection.id)
          updateConnectionPresentation(connection)
        }
        await editor.addConnection(connection)
      }
      await refreshPortStates(true)
      renderGroups()
      // 注释便签/宏框是手绘 DOM（不在 Rete 里），restore 后必须重绘，
      // 否则上一个图的宏框残留在画布、新文档的注释也不显示。
      renderComments()
      updateMetrics()
      callbacks.onSelection(null)
      emitFunctionSignatureFromSnapshot(data)
      return plan.report
    } finally {
      restoring = false
    }
  }

  // 依据快照/文档中的 macroRefs 重建编辑器引用状态。撤销快照带 mirrorIds（镜像原样还原）；
  // 磁盘文档没有 mirrorIds（镜像待展开，展开点在 loadDocument/expandMacroRefs）。
  function restoreMacroRefStates(items: MacroRefSnapshot[]) {
    macroRefs.length = 0
    macroMirrorByNodeId.clear()
    for (const item of items) {
      const macroId = String(item.macroId ?? '').trim()
      if (!macroId) continue
      const ref: MacroRefState = {
        macroId,
        pathHint: String(item.pathHint ?? ''),
        commentId: String(item.commentId ?? ''),
        label: '',
        frame: item.frame ? { ...item.frame } : { x: 0, y: 0, width: 240, height: 160 },
        boundary: (item.boundary ?? []).map(boundary => ({ ...boundary })),
        mirrorIds: (item.mirrorIds ?? []).map(id => String(id ?? '')),
        missing: false
      }
      macroRefs.push(ref)
      for (const [sourceIndex, nodeId] of ref.mirrorIds.entries()) {
        if (nodeId) macroMirrorByNodeId.set(nodeId, { ref, sourceIndex })
      }
    }
  }

  async function mutate(label: string, operation: () => Promise<void>) {
    if (!restoring) pushUndoHistory(historySnapshot())
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

  editor.addPipe(async context => {
    if (context.type !== 'connectioncreate' || restoring) return context
    const types = connectionTypes(context.data)
		const targetNode = editor.getNode(context.data.target)
    // 宏镜像之间不允许用户新建连线：宏内部连线以源宏为准（保存时也会被拆分器丢弃）。
    // buildingMacroMirrors 期间是程序化重建宏内部连线，必须放行。
    if (!buildingMacroMirrors && macroMirrorByNodeId.has(context.data.source) && macroMirrorByNodeId.has(context.data.target)) {
      callbacks.onStatus('宏内部连线请在源宏中修改（镜像为只读投影）')
      return
    }
		if ((targetNode?.typeId === 'origin.timer.create' || targetNode?.typeId === 'origin.timer.clear-by-key') && String(context.data.targetInput) === 'timerKey') {
			callbacks.onStatus('Connection rejected: Timer Key must be entered directly')
			return
		}
    ;(context.data as Schemes['Connection']).socketType = normalizeSocketName(types.source ?? types.target)
		const callbackToExec = types.source === 'callback' && types.target === 'exec'
    if (types.source && types.target && !callbackToExec && types.source !== types.target && types.source !== 'any' && types.target !== 'any') {
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
    const replacementIds = execOutputReplacementIds({
      source: context.data.source,
      sourceOutput: String(context.data.sourceOutput)
    }, editor.getConnections().map(item => ({
      id: item.id,
      source: item.source,
      sourceOutput: String(item.sourceOutput)
    })), types.source)
    if (replacementIds.length) {
      const trackStandaloneEdit = !transactionActive && !controlEditSnapshot && !initializing
      if (trackStandaloneEdit) {
        pendingExecReplacementSnapshot = historySnapshot()
        replacingExecConnection = true
      }
      try {
        for (const id of replacementIds) await editor.removeConnection(id)
      } catch (error) {
        pendingExecReplacementSnapshot = null
        replacingExecConnection = false
        callbacks.onStatus(`Connection replacement failed: ${error instanceof Error ? error.message : String(error)}`)
        return
      }
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
    const node = typeId === 'origin.timer.set-by-function' ? createTimerFunctionNodeFromProperties() : createNode(typeId)
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

  async function setCallableFunctions(functions: FunctionNodeMetadata[]) {
    callableFunctions = functions.map(item => ({
      ...item,
      functionSignature: item.functionSignature ? {
        inputs: item.functionSignature.inputs.map(port => ({ ...port })),
        outputs: item.functionSignature.outputs.map(port => ({ ...port }))
      } : { inputs: [], outputs: [] }
    }))
    const options = callableFunctions.map(item => ({ id: item.functionId, label: item.functionName }))
    const timerNodes = editor.getNodes().filter(node => node.typeId === 'origin.timer.set-by-function')
    for (const node of timerNodes) {
      node.functionOptions = options
      node.functionSelectorLabel = callbacks.locale?.() === 'en-US' ? 'Callback Function' : '回调函数'
      node.functionMissingLabel = callbacks.locale?.() === 'en-US' ? 'Missing function' : '函数不存在'
      node.functionReferenceMissing = Boolean(node.functionId && !options.some(option => option.id === node.functionId))
      await area.update('node', node.id)
    }
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
    if (editor.getNodes().some(node => node.typeId === functionEntryTypeId)) {
      callbacks.onStatus(callbacks.locale?.() === 'en-US' ? 'The function entry already exists' : '函数入口节点已存在')
      return
    }
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
        if (!node.typeId.startsWith('origin.function.') && node.typeId !== 'origin.timer.set-by-function') continue
        const role = node.properties?.functionRole
        const isTerminal = role === 'entry' || role === 'return'
        const isMatchingCall = (role === 'call' || role === 'timer') && sameFunctionReference(node.properties, spec)
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

  function hiddenLegacyEdgeIndexes(nodeIds: Set<string>) {
    const edges = currentLegacy?.hiddenEdges ?? []
    return edges.flatMap((edge, index) => nodeIds.has(edge.source_node_id) || nodeIds.has(edge.des_node_id) ? [index] : [])
  }

  function pruneHiddenLegacyEdges(nodeIds: Set<string>) {
    const edges = currentLegacy?.hiddenEdges
    if (!currentLegacy || !edges?.length) return 0
    const removed = new Set(hiddenLegacyEdgeIndexes(nodeIds))
    if (!removed.size) return 0
    currentLegacy.hiddenEdges = edges.filter((_, index) => !removed.has(index))
    const ordinals = currentLegacy.hiddenEdgeOrdinals
    if (ordinals?.length === edges.length) currentLegacy.hiddenEdgeOrdinals = ordinals.filter((_, index) => !removed.has(index))
    return removed.size
  }

  async function deleteSelected() {
    if (selectedGroupId) {
      await ungroupSelected()
      return
    }
    if (selectedCommentId) {
      const commentId = selectedCommentId
      // 宏框被删除 = 移除整个宏引用（镜像节点 + 边界连线一起清）。
      const macroRef = macroRefs.find(item => item.commentId === commentId)
      if (macroRef) {
        await mutate('Macro reference removed', async () => {
          await removeMacroRef(macroRef)
        })
        callbacks.onStatus('已移除宏引用（引用图中的宏内容不落盘，重插可恢复）')
        return
      }
      await mutate('Comment deleted', async () => {
        const index = comments.findIndex(item => item.id === commentId)
        if (index >= 0) comments.splice(index, 1)
        selectedCommentId = null
        renderComments()
      })
      return
    }
    const selected = selectedNodes()
    const selectedConnections = new Set(selectedConnectionIds)
    if (!selected.length && !selectedConnections.size) return
    // 宏镜像节点不可单独删除：选中镜像即视为移除其所属宏引用。
    const macroRefsToRemove = new Set<MacroRefState>()
    for (const node of selected) {
      const site = macroMirrorByNodeId.get(node.id)
      if (site) macroRefsToRemove.add(site.ref)
    }
    const deletableSelection = selected.filter(node => !macroMirrorByNodeId.has(node.id))
    const deletionPlan = planFunctionTerminalDeletion(editor.getNodes(), deletableSelection)
    const deletableIds = new Set(deletionPlan.deletableIds)
    const deletableNodes = deletableSelection.filter(node => deletableIds.has(node.id))
    const protectedEntries = deletionPlan.protectedEntryIds.length
    const protectedReturns = deletionPlan.protectedReturnIds.length
    const english = callbacks.locale?.() === 'en-US'
    const protectionParts = [
      protectedEntries ? (english ? 'a function must keep one entry node' : '函数必须保留一个入口节点') : '',
      protectedReturns ? (english ? 'a function must keep at least one return node' : '函数必须至少保留一个返回节点') : ''
    ].filter(Boolean)
    if (!deletableNodes.length && !selectedConnections.size && !macroRefsToRemove.size) {
      callbacks.onStatus(protectionParts.join(english ? '; ' : '；'))
      return
    }
    const ids = new Set(deletableNodes.map(node => node.id))
    const hiddenLegacyConnections = hiddenLegacyEdgeIndexes(ids).length
    const parts = [
      macroRefsToRemove.size ? `${macroRefsToRemove.size} macro reference(s)` : '',
      deletableNodes.length ? `${deletableNodes.length} node(s)` : '',
      selectedConnections.size ? `${selectedConnections.size} connection(s)` : '',
      hiddenLegacyConnections ? `${hiddenLegacyConnections} hidden legacy connection(s)` : ''
    ].filter(Boolean)
    const status = [`Deleted ${parts.join(' and ')}`, ...protectionParts].join(english ? '; ' : '；')
    await mutate(status, async () => {
      for (const ref of macroRefsToRemove) await removeMacroRef(ref)
      pruneHiddenLegacyEdges(ids)
      for (const item of editor.getConnections()) {
        if (selectedConnections.has(item.id) || ids.has(item.source) || ids.has(item.target)) await editor.removeConnection(item.id)
      }
      for (const node of deletableNodes) await editor.removeNode(node.id)
      selectedConnectionIds.clear()
      const protectedSelection = selected.find(node => !deletableIds.has(node.id) && !macroMirrorByNodeId.has(node.id))
      callbacks.onSelection(protectedSelection ? selectedNodeInfo(protectedSelection) : null)
    })
  }

  function copy() {
    const mirrorSelected = selectedNodes().filter(node => node.mirrorMacro)
    const selected = selectedNodes().filter(isCopyableFunctionNode).filter(node => !node.mirrorMacro)
    if (mirrorSelected.length) callbacks.onStatus('宏镜像节点属于源宏，不能复制（请到源宏中编辑）')
    if (!selected.length && selectedNodes().some(node => node.typeId === functionEntryTypeId)) {
      callbacks.onStatus(callbacks.locale?.() === 'en-US' ? 'Function entry nodes cannot be copied' : '函数入口节点不能复制')
      return
    }
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

  // === 宏引用（v2）：引用语义——图里只落 macroId + 边界连线，内容以 .obpm 源为准。 ===
  // 镜像节点是宏源的只读投影：可选中/拖动/连线（外部↔镜像连线持久化为 boundary），
  // 不进 nodes、不进撤销外的任何持久化路径；改内容请编辑源宏，保存后所有引用图热更。

  function macroFrameComment(ref: MacroRefState) {
    return comments.find(item => item.id === ref.commentId)
  }

  function ensureMacroFrame(ref: MacroRefState, label: string): CommentSnapshot {
    let comment = macroFrameComment(ref)
    if (!comment) {
      comment = {
        id: crypto.randomUUID(),
        text: `宏：${label}`,
        x: ref.frame.x,
        y: ref.frame.y,
        width: Math.max(ref.frame.width, 240),
        height: Math.max(ref.frame.height, 140)
      }
      comments.push(comment)
      ref.commentId = comment.id
    }
    return comment
  }

  // 按镜像包围盒收紧宏框（保留用户拖出的框左上角，尺寸随宏内容自适应）。
  // 必须等渲染帧：节点刚 addNode 时 DOM 尚未布局，立刻测量会得到 0 尺寸把框算小。
  async function fitMacroFrameToMirrors(ref: MacroRefState) {
    const mirrors = ref.mirrorIds.flatMap(id => {
      try { const node = editor.getNode(id); return node ? [node] : [] } catch { return [] }
    })
    if (!mirrors.length) return
    const comment = macroFrameComment(ref)
    if (!comment) return
    await nextFrame()
    await nextFrame()
    await nextFrame()
    const placed = mirrors.map(node => {
      const position = area.nodeViews.get(node.id)?.position ?? { x: 0, y: 0 }
      const rect = area.nodeViews.get(node.id)?.element?.getBoundingClientRect()
      const zoom = area.area.transform.k || 1
      // DOM 未就绪（rect 为 0）时退回节点声明尺寸，避免把框算小。
      const width = rect && rect.width > 10 ? rect.width / zoom : (node.width ?? 230)
      const height = rect && rect.height > 10 ? rect.height / zoom : 90
      return { position, size: { width, height } }
    })
    const minX = Math.min(...placed.map(item => item.position.x)), minY = Math.min(...placed.map(item => item.position.y))
    const maxX = Math.max(...placed.map(item => item.position.x + item.size.width)), maxY = Math.max(...placed.map(item => item.position.y + item.size.height))
    comment.x = minX - 24; comment.y = minY - 34
    comment.width = maxX - minX + 48; comment.height = maxY - minY + 62
    ref.frame = { x: comment.x, y: comment.y, width: comment.width, height: comment.height }
  }

  // 构建宏镜像：按宏源节点顺序创建只读镜像 + 重建宏内连线 + 重接边界连线。
  // anchor 为宏框内左上落点；跳过的节点在 mirrorIds 中记 ''（下标仍对齐宏源顺序）。
  async function buildMacroMirrors(ref: MacroRefState, payload: MacroMirrorPayload, anchor: Position) {
    buildingMacroMirrors = true
    try {
      await buildMacroMirrorsInner(ref, payload, anchor)
    } finally {
      buildingMacroMirrors = false
    }
  }

  async function buildMacroMirrorsInner(ref: MacroRefState, payload: MacroMirrorPayload, anchor: Position) {
    const sourceNodes = payload.snapshot.nodes ?? []
    const variablesPool = payload.variables ?? []
    const placed = sourceNodes.filter(item => typeof item.typeId === 'string' && item.typeId)
    const srcMinX = Math.min(...placed.map(item => item.position?.x ?? 0))
    const srcMinY = Math.min(...placed.map(item => item.position?.y ?? 0))
    const mirrorBySourceIndex: Array<BlueprintNode | null> = []
    const mirrorBySourceId = new Map<string, BlueprintNode>()
    for (const [index, item] of sourceNodes.entries()) {
      const typeId = typeof item.typeId === 'string' ? item.typeId : ''
      if (!typeId || !isPasteableFunctionNode({ id: '', typeId })) { mirrorBySourceIndex[index] = null; continue }
      const node = createRestoredNode(item, typeId, variablesPool)
      if (!node) { mirrorBySourceIndex[index] = null; continue }
      node.mirrorMacro = true
      applyNodeProperties(node, item.properties)
      if (node.dynamicOutputs) setDynamicOutputCount(node, item.properties?.dynamicOutputCount ?? 3)
      if (item.properties?.label && !typeId.startsWith('origin.variable.') && !item.properties.legacyClass) {
        node.label = item.properties.label
        node.width = Math.max(node.width ?? 230, nodeTitleWidth(node.label))
      }
      setControlValues(node, item.values)
      syncDynamicBranchOutputs(node, dynamicBranchValueCount(node))
      await editor.addNode(node)
      await area.translate(node.id, { x: anchor.x + (item.position.x - srcMinX), y: anchor.y + (item.position.y - srcMinY) })
      mirrorBySourceIndex[index] = node
      mirrorBySourceId.set(item.id, node)
    }
    ref.mirrorIds = mirrorBySourceIndex.map(node => node?.id ?? '')
    for (const [sourceIndex, node] of mirrorBySourceIndex.entries()) {
      if (node) macroMirrorByNodeId.set(node.id, { ref, sourceIndex })
    }
    for (const connection of payload.snapshot.connections ?? []) {
      const source = mirrorBySourceId.get(connection.source)
      const target = mirrorBySourceId.get(connection.target)
      if (!source || !target) continue
      try { await editor.addConnection(createConnection(source, connection.sourceOutput, target, connection.targetInput)) } catch { /* 端口类型变化后个别内部连线可能不再合法 */ }
    }
    for (const boundary of ref.boundary) {
      const mirror = mirrorBySourceIndex[boundary.macroNodeIndex] ?? null
      let external: BlueprintNode | null = null
      try { external = boundary.externalNode ? editor.getNode(boundary.externalNode) ?? null : null } catch { external = null }
      if (!mirror || !external) continue
      try {
        const connection = boundary.intoMacro
          ? createConnection(external, boundary.externalPort, mirror, boundary.macroPort)
          : createConnection(mirror, boundary.macroPort, external, boundary.externalPort)
        await editor.addConnection(connection)
      } catch { /* 宏端口变化后该边界连线不再合法，保留记录待用户重连 */ }
    }
  }

  async function removeMacroMirrorNodes(ref: MacroRefState) {
    for (const id of ref.mirrorIds) {
      macroMirrorByNodeId.delete(id)
      if (!id) continue
      for (const item of [...editor.getConnections()]) {
        if (item.source === id || item.target === id) await editor.removeConnection(item.id)
      }
      try { if (editor.getNode(id)) await editor.removeNode(id) } catch { /* 已被删除则跳过 */ }
    }
    ref.mirrorIds = []
    // boundary 记录保留：热更重建（refreshMacroRef）要按它重接边界连线；
    // 整引用删除（removeMacroRef）时记录随引用一起丢弃。
  }

  async function removeMacroRef(ref: MacroRefState) {
    await removeMacroMirrorNodes(ref)
    const index = macroRefs.indexOf(ref)
    if (index >= 0) macroRefs.splice(index, 1)
    const frameIndex = comments.findIndex(item => item.id === ref.commentId)
    if (frameIndex >= 0) comments.splice(frameIndex, 1)
    if (selectedCommentId === ref.commentId) selectedCommentId = null
    renderComments()
  }

  // 插入宏引用：只登记 macroId + 建镜像 + 画宏框，不复制任何节点/连线/变量。
  async function insertMacroRef(payload: InsertMacroRefPayload): Promise<boolean> {
    const existing = macroRefs.find(item => item.macroId === payload.macroId)
    if (existing) {
      callbacks.onStatus('该宏已引用于当前蓝图，已定位到现有宏框（重复插入已跳过）')
      await selectComment(existing.commentId)
      await focusComment(existing.commentId)
      return false
    }
    const base = payload.clientPosition ? graphPosition(payload.clientPosition) : graphPosition()
    const ref: MacroRefState = {
      macroId: payload.macroId,
      pathHint: payload.pathHint ?? '',
      commentId: '',
      label: payload.label,
      frame: { x: base.x, y: base.y, width: 240, height: 140 },
      boundary: [],
      mirrorIds: [],
      missing: false
    }
    await mutate(`Macro referenced: ${payload.label}`, async () => {
      macroRefs.push(ref)
      await buildMacroMirrors(ref, payload, { x: base.x + 24, y: base.y + 34 })
      ensureMacroFrame(ref, payload.label)
      await fitMacroFrameToMirrors(ref)
      renderComments()
      await refreshPortStates(true)
    })
    if (ref.commentId) await selectComment(ref.commentId)
    return true
  }

  // 宏热更：源宏保存后用最新内容重建镜像（宏框位置与边界连线保留）。
  // 外部真值刷新，不进撤销历史；端口失效的边界记录跳过重连但保留在记录里待重连。
  async function refreshMacroRef(macroId: string, payload: MacroMirrorPayload): Promise<boolean> {
    let touched = false
    for (const ref of [...macroRefs]) {
      if (ref.macroId !== macroId) continue
      touched = true
      const frame = macroFrameComment(ref) ?? ensureMacroFrame(ref, payload.label)
      const anchor = { x: frame.x + 24, y: frame.y + 34 }
      restoring = true
      try {
        await removeMacroMirrorNodes(ref)
        ref.label = payload.label
        ref.missing = false
        if (payload.pathHint) ref.pathHint = payload.pathHint
        await buildMacroMirrors(ref, payload, anchor)
        frame.text = `宏：${payload.label}`
        await fitMacroFrameToMirrors(ref)
        renderComments()
        await refreshPortStates(true)
      } finally {
        restoring = false
      }
    }
    if (touched) updateMetrics()
    return touched
  }

  // 宏改名传播（仅刷框文本，内容不动）：宏未展开（找不到源）时也把框标成未找到提示。
  function refreshMacroFrames(macroId: string, label: string): boolean {
    let touched = false
    for (const ref of macroRefs) {
      if (ref.macroId !== macroId) continue
      const frame = macroFrameComment(ref) ?? ensureMacroFrame(ref, label)
      if (frame.text !== `宏：${label}`) {
        frame.text = `宏：${label}`
        touched = true
      }
      ref.label = label
    }
    if (touched) renderComments()
    return touched
  }

  // 加载后展开：为没有镜像的宏引用按当前磁盘源展开（切标签页/撤销重做后的兜底）。
  async function expandMacroRefsInternal(): Promise<void> {
    if (!macroRefs.length) return
    const missing: string[] = []
    for (const ref of macroRefs) {
      const hasMirrors = ref.mirrorIds.some(id => id)
      let payload: MacroMirrorPayload | null = null
      if (macroResolver) {
        try { payload = await macroResolver(ref.macroId) } catch { payload = null }
      }
      const frame = macroFrameComment(ref) ?? ensureMacroFrame(ref, payload?.label ?? '')
      if (!payload) {
        ref.missing = true
        const hint = `宏：未找到（${ref.pathHint || ref.macroId}）`
        if (frame.text !== hint) { frame.text = hint; renderComments() }
        missing.push(ref.pathHint || ref.macroId)
        continue
      }
      ref.missing = false
      ref.label = payload.label
      // pathHint 是运行时解析结果（详情面板显示/打开源宏用），不持久化。
      if (payload.pathHint) ref.pathHint = payload.pathHint
      if (frame.text !== `宏：${payload.label}`) { frame.text = `宏：${payload.label}`; renderComments() }
      if (hasMirrors) continue
      restoring = true
      try {
        await buildMacroMirrors(ref, payload, { x: frame.x + 24, y: frame.y + 34 })
        // 宏内容自保存后可能已变化（加节点等）：展开后按镜像包围盒重算宏框。
        await fitMacroFrameToMirrors(ref)
        renderComments()
      } finally {
        restoring = false
      }
    }
    if (macroRefs.some(ref => !ref.mirrorIds.some(id => id))) await refreshPortStates(true)
    updateMetrics()
    if (missing.length) callbacks.onStatus(`宏源未找到：${missing.join('、')}（引用已保留，请检查工作区宏文件）`)
  }

  function selectedMacroRef(): { macroId: string; label: string; pathHint: string; nodeCount: number; missing: boolean } | null {
    if (!selectedCommentId) return null
    const ref = macroRefs.find(item => item.commentId === selectedCommentId)
    if (!ref) return null
    return { macroId: ref.macroId, label: ref.label, pathHint: ref.pathHint, nodeCount: ref.mirrorIds.filter(id => id).length, missing: ref.missing }
  }

  async function paste() {
    if (!clipboard) return
    await mutate(`Pasted ${clipboard.nodes.length} node(s)`, async () => {
      const base = graphPosition()
      const nodes = new Map<number, BlueprintNode>()
      // 与宏插入同规则：重复入口的下游逻辑接到已有入口，而不是丢弃连线留孤儿。
      const pasteEntryRemap = new Map<number, BlueprintNode>()
      await selector.unselectAll()
      for (const [index, item] of clipboard!.nodes.entries()) {
        const typeId = typeof item.typeId === 'string' ? item.typeId : ''
        if (!typeId) continue
        if (!isPasteableFunctionNode({ id: '', typeId })) continue
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
        const source = (nodes.get(item.sourceIndex) ?? pasteEntryRemap.get(item.sourceIndex))
        const target = (nodes.get(item.targetIndex) ?? pasteEntryRemap.get(item.targetIndex))
        if (source && target) await editor.addConnection(createConnection(source, item.sourceOutput, target, item.targetInput))
      }
    })
  }

  async function undo() {
    const previous = undoStack.pop()
    if (!previous) return
    pushRedoHistory(historySnapshot())
    await restoreHistory(previous)
    callbacks.onStatus('Undo')
    callbacks.onDirty()
  }

  async function redo() {
    const next = redoStack.pop()
    if (!next) return
    pushUndoHistory(historySnapshot())
    await restoreHistory(next)
    callbacks.onStatus('Redo')
    callbacks.onDirty()
  }

  function getDocument(graphName = 'Untitled', variables?: GraphVariable[], variableGroups?: GraphVariableGroup[]): GraphDocument {
    const data = snapshot()
    // 持久化拆分：宏镜像节点与跨宏连线退回 macroRefs（引用图不落宏内容）。
    const persisted = splitSnapshotForPersistence(data)
    return {
      schemaVersion: 1,
      // 编辑器持有载入时的 graphName（宏显示名），调用方参数（通常是标签页标题=文件名）只做兜底，
      // 防止显示名被文件名覆盖。
      graphName: currentGraphName || graphName,
      ...data,
      nodes: persisted.nodes,
      connections: persisted.connections,
      groups: persisted.groups,
      comments: persisted.comments,
      macroRefs: persisted.macroRefs,
      variables: (variables ?? currentVariables).map(item => ({ ...item })),
      variableGroups: (variableGroups ?? currentVariableGroups).map(item => ({ ...item })),
      view: { x: area.area.transform.x, y: area.area.transform.y, zoom: area.area.transform.k },
      legacy: cloneLegacyState(currentLegacy)
    }
  }

  async function loadDocument(document: GraphDocument) {
    undoStack.length = 0; redoStack.length = 0
    currentGraphName = String(document.graphName ?? '').trim()
    controlEditSnapshot = null; controlEditChanged = false
    visibleEntryConnectionIds.clear()
    currentVariables = (document.variables ?? []).map(item => ({ ...item }))
    currentVariableGroups = (document.variableGroups ?? []).map(item => ({ ...item }))
    currentLegacy = cloneLegacyState(document.legacy)
    callbacks.onVariables(currentVariables.map(item => ({ ...item })))
    callbacks.onVariableGroups(currentVariableGroups.map(item => ({ ...item })))
    const report = await restore({ nodes: document.nodes ?? [], connections: document.connections ?? [], groups: document.groups ?? [], comments: document.comments ?? [], macroRefs: document.macroRefs ?? [] })
    // 宏引用展开：镜像不在文档里，按当前磁盘源展开（切标签页即取最新宏，天然热更）。
    await expandMacroRefsInternal()
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
    return report
  }

  async function newDocument() {
    undoStack.length = 0; redoStack.length = 0; controlEditSnapshot = null; controlEditChanged = false; groups.length = 0; selectedGroupId = null
    visibleEntryConnectionIds.clear()
    currentVariables = []; currentVariableGroups = [{ id: 'default', name: 'Default' }]; currentLegacy = undefined; insertionOffset = 0; callbacks.onVariables([]); callbacks.onVariableGroups(currentVariableGroups.map(item => ({ ...item }))); callbacks.onSelection(null)
    restoring = true; await selector.unselectAll(); await editor.clear(); restoring = false; comments.length = 0; macroRefs.length = 0; macroMirrorByNodeId.clear(); currentGraphName = ''; renderGroups(); renderComments(); updateMetrics()
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
          if (mode === 'horizontal-center') y = centerY - entry.size.height / 2
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
      groups.push(group)
      await selector.unselectAll()
      selectedGroupId = group.id; renderGroups()
      callbacks.onSelection(null)
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
    const nodes = selectedNodes()
    if (nodes.length) { await groupSelected(); return }
    if (selectedGroupId) { await ungroupSelected(); return }
    await groupSelected()
  }

  async function fitSelected() {
    const nodes = selectedNodes()
    await AreaExtensions.zoomAt(area, nodes.length ? nodes : editor.getNodes(), { scale: 0.9 })
  }

  async function selectAll() {
    await clearConnectionSelection()
    await clearGroupSelection()
    let count = 0
    for (const node of editor.getNodes()) {
      if (node.mirrorMacro) continue
      await selectable.select(node.id, true)
      count++
    }
    callbacks.onStatus(`Selected ${count} node(s)`)
  }

  async function deselectAll() {
    await selector.unselectAll()
    await clearConnectionSelection()
    await clearGroupSelection()
    callbacks.onSelection(null)
    callbacks.onStatus('Selection cleared')
  }

  function selectedNodeInfo(node: BlueprintNode): SelectedNodeInfo {
    const inputs = Object.entries(node.inputs)
      .filter((entry): entry is [string, NonNullable<typeof entry[1]>] => Boolean(entry[1]?.label))
      .map(([key, port]) => ({ key, label: port.label ?? key, tip: node.inputTips?.[key], portId: node.inputPortIds?.[key] }))
    return { id: node.id, typeId: node.typeId ?? '', label: node.label, description: node.subtitle, values: controlValues(node), variableId: node.variableId, inputs }
  }

  async function refreshNodeTypeAnnotations(typeId: string) {
    const definition = findNodeDefinition(typeId)
    const affected: BlueprintNode[] = []
    for (const node of editor.getNodes()) {
      if (node.typeId !== typeId) continue
      affected.push(node)
      if (definition) {
        node.subtitle = definition.description
        node.inputPortIds = definition.inputPorts ? inputPortIdsFromDefinition(definition.inputPorts) : undefined
        node.inputTips = definition.inputPorts ? inputTipsFromDefinition(definition.inputPorts) : undefined
        for (const port of definition.inputPorts ?? []) ensureInputControl(node, port.key, port.refTable)
      }
      await area.update('node', node.id)
    }
    const selected = affected.find(node => node.selected)
    if (selected) callbacks.onSelection(selectedNodeInfo(selected))
    return affected.length
  }

  function inputTipsFromDefinition(ports: Array<{ key: string; tip?: string }>) {
    const tips: Record<string, string> = {}
    for (const port of ports) {
      if (port.tip) tips[port.key] = port.tip
    }
    return Object.keys(tips).length ? tips : undefined
  }

  // 按定义刷新输入控件：带 ref 的端口换成引用选择控件，去掉 ref 的换回普通整数控件。
  // 只替换控件、不重建端口对象，连线保持不变；值原样迁移。
  function ensureInputControl(node: BlueprintNode, key: string, refTable: string | undefined) {
    const port = node.inputs[key]
    if (!port) return
    const current = (port as unknown as { control?: { tableKey?: string; value?: unknown; integer?: boolean } }).control
    const wantRef = Boolean(refTable)
    const hasRef = typeof current?.tableKey === 'string' && current.tableKey !== ''
    if (wantRef === hasRef && (!wantRef || current?.tableKey === refTable)) return
    const previousValue = current?.value
    port.removeControl()
    if (wantRef) {
      port.addControl(new RefSelectControl(refTable!, true, (previousValue ?? '') as string | number))
      return
    }
    if (previousValue !== undefined) {
      const control = new ClassicPreset.InputControl('text', { initial: previousValue as never }) as ClassicPreset.InputControl<'text'> & { integer?: boolean }
      control.integer = true
      port.addControl(control)
    }
  }

  function inputPortIdsFromDefinition(ports: Array<{ key: string; portId?: number }>) {
    const ids: Record<string, number> = {}
    for (const port of ports) {
      if (typeof port.portId === 'number') ids[port.key] = port.portId
    }
    return Object.keys(ids).length ? ids : undefined
  }

  async function refreshFunctionNodeAnnotations(functionId: string, description?: string, portTipsById?: Map<string, string>, refsById?: Map<string, string>) {
    const text = description?.trim()
    let updated = 0
    for (const node of editor.getNodes()) {
      if (node.functionId !== functionId) continue
      const fallback = node.typeId === 'origin.function.entry' ? 'Function entry' : node.typeId === 'origin.function.return' ? 'Function return' : 'Function call'
      node.subtitle = text || fallback
      if (node.functionSignature) {
        for (const port of [...node.functionSignature.inputs, ...node.functionSignature.outputs]) {
          if (portTipsById) {
            const tip = portTipsById.get(port.id)
            if (tip !== undefined) {
              if (tip.trim()) port.description = tip.trim()
              else delete port.description
            }
          }
          if (refsById) {
            const ref = refsById.get(port.id)
            if (ref !== undefined) {
              if (ref.trim()) port.ref = ref.trim()
              else delete port.ref
            }
          }
        }
        node.inputTips = signaturePortTips(node.functionSignature)
        // 签名端口的 ref 决定输入端口是否用引用选择控件（调用节点 input_*，返回节点 output_*）。
        node.functionSignature.inputs.forEach((port, index) => {
          ensureInputControl(node, functionPortKey('input', port, index), port.ref)
        })
        node.functionSignature.outputs.forEach((port, index) => {
          ensureInputControl(node, functionPortKey('output', port, index), port.ref)
        })
      }
      await area.update('node', node.id)
      updated++
    }
    return updated
  }

  function setDynamicOutputCount(node: BlueprintNode, requested: number) {
    if (!node.dynamicOutputs) return
    const count = normalizeDynamicOutputCount(requested)
    for (const key of Object.keys(node.outputs).filter(key => key.startsWith('then'))) node.removeOutput(key)
    for (let index = 0; index < count; index++) node.addOutput(`then${index}`, new ClassicPreset.Output(new ClassicPreset.Socket('exec'), `Then ${index}`))
    node.dynamicOutputCount = count
  }

  async function changeDynamicOutputs(nodeId: string, delta: number) {
    const node = editor.getNode(nodeId)
    if (!node?.dynamicOutputs) return
    const current = node.dynamicOutputCount ?? 1
    const next = Math.max(1, Math.min(maxDynamicSequenceOutputs, current + delta))
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
    const detail = (event as CustomEvent<{ nodeId: string; count: number; countChanged?: boolean; commit?: boolean }>).detail
    if (!detail) return
    void (async () => {
      try {
        if (detail.countChanged) await pruneDynamicBranchConnections(detail.nodeId, detail.count)
        const node = editor.getNode(detail.nodeId)
        if (node) syncDynamicBranchOutputs(node, detail.count)
        await refreshPortStates(Boolean(node))
        if (node) {
          await area.update('node', node.id)
          if (node.selected) callbacks.onSelection(selectedNodeInfo(node))
        }
        callbacks.onDirty()
      } finally {
        if (detail.commit) document.dispatchEvent(new CustomEvent('origin-control-edit-commit'))
      }
    })()
  }
  const controlChangeListener = () => {
    if (restoring) return
    if (controlEditSnapshot) controlEditChanged = true
    callbacks.onDirty()
    void refreshPortStates(true)
  }
  const controlEditStartListener = () => {
    if (restoring || controlEditSnapshot) return
    controlEditSnapshot = historySnapshot()
    controlEditChanged = false
  }
  const controlEditCommitListener = () => {
    if (!controlEditSnapshot) return
    if (controlEditChanged) {
      pushUndoHistory(controlEditSnapshot)
      redoStack.length = 0
    }
    controlEditSnapshot = null
    controlEditChanged = false
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
  document.addEventListener('origin-control-edit-start', controlEditStartListener)
  document.addEventListener('origin-control-edit-commit', controlEditCommitListener)
  window.addEventListener('pointerdown', hideEntryBindingMenu)
  const destroyPanFeedback = setupCanvasPanFeedback()
  const destroyMultiSelectionDragPreserver = setupMultiSelectionDragPreserver()

  async function setVariables(variables: GraphVariable[], variableGroups?: GraphVariableGroup[], refreshNodes = false) {
    const before = snapshot()
    currentVariables = variables.map(item => ({ ...item }))
    if (variableGroups) currentVariableGroups = variableGroups.map(item => ({ ...item }))
    if (refreshNodes && before.nodes.some(item => item.typeId.startsWith('origin.variable.'))) await restore(before)
    callbacks.onVariables(currentVariables.map(item => ({ ...item })))
    callbacks.onVariableGroups(currentVariableGroups.map(item => ({ ...item })))
  }

  async function refreshVariableNodePresentation(variable: GraphVariable) {
    const index = currentVariables.findIndex(item => item.id === variable.id)
    if (index >= 0) currentVariables[index] = { ...variable }
    const nodes = editor.getNodes().filter(node => node.variableId === variable.id)
    for (const node of nodes) applyVariableNodePresentation(node, variable)
    await Promise.all(nodes.map(node => area.update('node', node.id)))
    callbacks.onDirty()
  }

  async function focusNode(id: string) {
    // 宏镜像不可单独选中：定位请求（搜索结果等）落到所属宏框，保持"宏=整体"的交互。
    const mirrorSite = macroMirrorByNodeId.get(id)
    if (mirrorSite) return focusComment(mirrorSite.ref.commentId)
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

  async function highlightFunctionReferences(functionKey: string) {
    const key = functionKey.trim()
    const matches = editor.getNodes().filter(node =>
      (node.typeId === 'origin.function.call' || node.typeId === 'origin.timer.set-by-function')
      && key
      && (node.functionId === key || node.functionName === key),
    )
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

  // 宏镜像整体化交互：宏加入蓝图后不可拆分——镜像节点不允许单独选中/拖动/删除，
  // 点击镜像即选中所属宏框（Delete=整体删除；拖宏框=整体移动）；仅端口保留连线交互（宏边界）。
  function setupMacroMirrorInteraction() {
    const down = (event: PointerEvent) => {
      if (event.button !== 0) return
      const target = event.target as HTMLElement
      const mirror = target.closest('.blueprint-node.mirror-macro') as HTMLElement | null
      if (!mirror) return
      if (target.closest('.blueprint-socket')) return
      event.stopPropagation()
      const nodeId = nodeIdFromEventTarget(mirror)
      const site = nodeId ? macroMirrorByNodeId.get(nodeId) : undefined
      if (site) void selectComment(site.ref.commentId)
    }
    container.addEventListener('pointerdown', down, true)
    return () => container.removeEventListener('pointerdown', down, true)
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
      if (!event.ctrlKey) {
        await selector.unselectAll()
        await clearGroupSelection()
      }
      const connectionIds = connectionIdsInClientRect(selectionRect)
      const selectedConnections = await selectConnections(connectionIds, event.ctrlKey)
      let selectedNodes = 0
      for (const node of editor.getNodes()) {
        // 宏镜像不可被框选单独命中（宏只能整体选中：点击宏框）。
        if (node.mirrorMacro) continue
        const bounds = area.nodeViews.get(node.id)?.element.getBoundingClientRect()
        if (bounds && rectsIntersect(bounds, selectionRect)) {
          await selectable.select(node.id, true)
          selectedNodes++
        }
      }
      if (!selectedNodes) callbacks.onSelection(null)
      if (selectedNodes || selectedConnections) callbacks.onStatus(`Selected ${selectedNodes} node(s), ${selectedConnections} connection(s)`)
      await clearGroupSelection()
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
    if (context.type === 'connectioncreate' || context.type === 'connectionremove') {
      pendingConnectionSnapshot = !restoring && !transactionActive && !controlEditSnapshot && !initializing && !replacingExecConnection ? historySnapshot() : null
    }
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
      if (context.type === 'connectioncreated' && pendingExecReplacementSnapshot) {
        pushUndoHistory(pendingExecReplacementSnapshot)
        redoStack.length = 0
        pendingExecReplacementSnapshot = null
        pendingConnectionSnapshot = null
        replacingExecConnection = false
        callbacks.onDirty()
        callbacks.onStatus('Execution connection replaced')
      } else if (!restoring && !transactionActive && !controlEditSnapshot && !initializing && pendingConnectionSnapshot) {
        pushUndoHistory(pendingConnectionSnapshot); redoStack.length = 0; pendingConnectionSnapshot = null
        callbacks.onDirty(); callbacks.onStatus(context.type === 'connectioncreated' ? 'Connection created' : 'Connection removed')
      }
    }
    if (context.type === 'nodepicked') {
      void clearConnectionSelection()
      await clearGroupSelection()
      startNodeDragFeedback()
      dragSnapshot = historySnapshot()
      nodeDragMoved = false
      await restoreMultiSelectionAfterNodePick(context.data.id)
      requestAnimationFrame(() => {
        const node = editor.getNode(context.data.id)
        callbacks.onSelection(node ? selectedNodeInfo(node) : null)
      })
    }
    if (context.type === 'nodetranslated' && dragSnapshot) nodeDragMoved = true
    if (context.type === 'nodedragged' && dragSnapshot) {
      stopNodeDragFeedback()
      if (nodeDragMoved) {
        pushUndoHistory(dragSnapshot); redoStack.length = 0; callbacks.onDirty(); callbacks.onStatus('Node moved')
      }
      dragSnapshot = null
      await finishPreservedMultiSelectionClick(context.data.id)
      nodeDragMoved = false
    }
    return context
  })
  setupRubberBandSelection()
  const destroyMacroMirrorInteraction = setupMacroMirrorInteraction()
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
      document.removeEventListener('origin-control-edit-start', controlEditStartListener)
      document.removeEventListener('origin-control-edit-commit', controlEditCommitListener)
      window.removeEventListener('pointerdown', hideEntryBindingMenu)
      window.removeEventListener('pointerup', stopNodeDragFeedback)
      window.removeEventListener('pointercancel', stopNodeDragFeedback)
      destroyPanFeedback()
      destroyMultiSelectionDragPreserver()
      destroyMacroMirrorInteraction()
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
    refreshNodeTypeAnnotations,
    refreshFunctionNodeAnnotations,
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
    addCommentAt,
    commentAroundSelection,
    insertMacroRef,
    selectedMacroRef,
    hasMacroRef: (macroId: string) => snapshotHasMacroRef(snapshot(), macroId),
    setMacroResolver(resolver: ((macroId: string) => Promise<MacroMirrorPayload | null>) | null) {
      macroResolver = resolver
    },
    refreshMacroRef,
    refreshMacroFrames,
    searchNodes,
    focusComment,
    commentCount,
    fitSelected,
    setVariables,
    refreshVariableNodePresentation,
    setCallableFunctions,
    focusNode,
    highlightNodesByType,
    highlightFunctionReferences,
    highlightIssueNode,
    highlightIssueNodes
  }
}
