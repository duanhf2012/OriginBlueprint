<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { toPng } from 'html-to-image'
import { createBlueprintEditor, type BlueprintEditorHandle, type EditorMetrics, type FunctionSignature, type FunctionSignaturePort, type GraphDocument, type GraphVariable, type GraphVariableGroup, type SelectedNodeInfo, type ValidationIssue, type VariableType } from './editor/createEditor'
import type { FunctionNodeMetadata, NodeSnapshot } from './editor/document'
import { getNodeDefinitions, registerNodeSchemas, type NodeDefinition } from './editor/nodeRegistry'
import { menuLocales, normalizeLocale, type LocaleId } from './i18n'
import { platform, type NodeReferenceResult, type WorkspaceEntry } from './platform'

interface GraphTab { id: string; title: string; path: string; dirty: boolean; document: GraphDocument | null }
interface WorkspaceTreeNode extends WorkspaceEntry { children: WorkspaceTreeNode[]; loaded: boolean; loading: boolean }
interface VisibleWorkspaceNode { node: WorkspaceTreeNode; depth: number }
type UnsavedCloseAction = 'save' | 'discard' | 'cancel'
interface ModuleNodeMenuState { visible: boolean; x: number; y: number; node: NodeDefinition | null }
interface NodeReferenceSearchState { visible: boolean; loading: boolean; nodeTitle: string; typeId: string; results: NodeReferenceResult[] }
interface FileContextMenuState { visible: boolean; x: number; y: number; path: string; isDir: boolean; isFunction: boolean }
interface BlueprintFunction { id: string; name: string; readonly?: boolean }
interface FunctionLibraryItem { id: string; name: string; path: string; source: 'current' | 'workspace' }
interface ModuleLibraryItem extends NodeDefinition { functionPlaceholder?: boolean; functionSource?: FunctionLibraryItem['source']; functionItem?: FunctionLibraryItem; path?: string }

const canvas = ref<HTMLElement | null>(null)
const tabStrip = ref<HTMLElement | null>(null)
const zoomLabel = ref('100%')
const status = ref('Ready')
const metrics = ref<EditorMetrics>({ nodes: 0, connections: 0 })
const activeMenu = ref<string | null>(null)
const contextMenu = ref({ visible: false, x: 0, y: 0, clientX: 0, clientY: 0, search: '' })
const moduleNodeMenu = ref<ModuleNodeMenuState>({ visible: false, x: 0, y: 0, node: null })
const nodeReferenceSearch = ref<NodeReferenceSearchState>({ visible: false, loading: false, nodeTitle: '', typeId: '', results: [] })
const fileContextMenu = ref<FileContextMenuState>({ visible: false, x: 0, y: 0, path: '', isDir: false, isFunction: false })
const testPanelHeight = ref(savedPanelSize('origin-blueprint-test-panel-height', 155, 96, 360))
const testPanelCollapsed = ref(false)
const referencePanelHeight = ref(savedReferencePanelHeight())
const referencePanelCollapsed = ref(false)
const currentLocale = ref<LocaleId>(normalizeLocale(localStorage.getItem('origin-blueprint-locale')))
const tabs = ref<GraphTab[]>([{ id: crypto.randomUUID(), title: 'Untitled-1 Graph', path: '', dirty: false, document: null }])
const activeTabId = ref(tabs.value[0].id)
const recentFiles = ref<string[]>([])
const workspaceRoot = ref('')
const workspaceTree = ref<WorkspaceTreeNode[]>([])
const workspaceSearch = ref('')
const expandedWorkspacePaths = ref<Set<string>>(new Set())
const selectedWorkspacePath = ref('')
const functionTitleByPath = ref<Record<string, string>>({})
const fileBrowserWidth = ref(savedPanelWidth('origin-blueprint-file-browser-width', 210))
const leftToolsWidth = ref(savedPanelWidth('origin-blueprint-left-tools-width', 210, 160, 520))
const rightSidebarWidth = ref(savedPanelWidth('origin-blueprint-right-sidebar-width', 230, 160, 460))
const variablePanelHeight = ref(savedPanelSize('origin-blueprint-variable-panel-height', 300, 130, 520))
const showTools = ref(true)
const showRight = ref(true)
const showLogger = ref(false)
const showAbout = ref(false)
const nodeLibrary = ref<NodeDefinition[]>(getNodeDefinitions())
const moduleSearch = ref('')
const expandedModuleCategories = ref<Set<string>>(new Set())
const variables = ref<GraphVariable[]>([])
const variableGroups = ref<GraphVariableGroup[]>([{ id: 'default', name: 'Default' }])
const functionSignature = ref<FunctionSignature>(emptyFunctionSignature())
const functionTitle = ref('')
const functionSignatureTypeOptions: Array<{ value: VariableType; label: string }> = [
  { value: 'boolean', label: 'Boolean' },
  { value: 'integer', label: 'Integer' },
  { value: 'float', label: 'Float' },
  { value: 'string', label: 'String' },
  { value: 'array', label: 'Array' }
]
const blueprintFunctions = ref<BlueprintFunction[]>([])
const selectedFunctionId = ref('')
const selectedVariableId = ref<string | null>(null)
const selectedNode = ref<SelectedNodeInfo | null>(null)
const validationIssues = ref<ValidationIssue[]>([])
const selectedValidationIssueKey = ref('')
const unsavedCloseDialog = ref<{ visible: boolean; names: string[]; resolve?: (action: UnsavedCloseAction) => void }>({ visible: false, names: [] })
let untitledCount = 1
const tabDragIndex = ref(-1)
const tabDragOverIndex = ref(-1)
let editor: BlueprintEditorHandle | null = null
let unsubscribeCloseRequest = () => {}
let closingApplication = false
let nodePointerDrag: { item: ModuleLibraryItem; startX: number; startY: number; lastX: number; lastY: number; moved: boolean } | null = null
let removeNodePointerListeners = () => {}
let workspaceLoadToken = 0
let validationIssueClickTimer: ReturnType<typeof window.setTimeout> | undefined
const loadingFunctionTitles = new Set<string>()

const activeTab = computed(() => tabs.value.find(tab => tab.id === activeTabId.value)!)
const selectedVariable = computed(() => variables.value.find(variable => variable.id === selectedVariableId.value) ?? null)
const isFunctionBlueprintTab = computed(() => isFunctionBlueprintPath(activeTab.value?.path || activeTab.value?.title || ''))
const groupedVariables = computed(() => variableGroups.value.map(group => ({
  group,
  variables: variables.value.filter(variable => variable.groupId === group.id)
})))
const functionLibraryItems = computed(() => collectFunctionLibraryItems(workspaceTree.value))
const callableFunctionItems = computed<FunctionLibraryItem[]>(() => [
  ...blueprintFunctions.value.map(item => ({ id: item.id, name: item.name, path: activeTab.value?.path || activeTab.value?.title || '', source: 'current' as const })),
  ...functionLibraryItems.value
])
const functionModuleItems = computed<ModuleLibraryItem[]>(() => callableFunctionItems.value.map(item => ({
  id: `origin.function.${item.source}.${item.id}`,
  title: item.name,
  category: menuText.value.module.functionCategory,
  kind: 'function',
  functionPlaceholder: true,
  functionSource: item.source,
  functionItem: item,
  path: item.path,
  create() {
    throw new Error('Function call nodes are not implemented yet')
  }
})))
const categories = computed(() => {
  const result = new Map<string, ModuleLibraryItem[]>()
  const search = moduleSearch.value.trim().toLowerCase()
  for (const definition of nodeLibrary.value.filter(item => !search || `${item.title} ${item.category} ${item.id}`.toLowerCase().includes(search))) {
    const items = result.get(definition.category) ?? []; items.push(definition); result.set(definition.category, items)
  }
  for (const definition of functionModuleItems.value.filter(item => !search || `${item.title} ${item.category} ${item.id} ${item.path ?? ''}`.toLowerCase().includes(search))) {
    const items = result.get(definition.category) ?? []; items.push(definition); result.set(definition.category, items)
  }
  return Array.from(result.entries())
})
const filteredDefinitions = computed(() => {
  const search = contextMenu.value.search.trim().toLowerCase()
  return search ? nodeLibrary.value.filter(item => `${item.title} ${item.category}`.toLowerCase().includes(search)) : nodeLibrary.value
})
const moduleSearchActive = computed(() => Boolean(moduleSearch.value.trim()))
const menuText = computed(() => menuLocales[currentLocale.value])
const workspaceStyle = computed(() => ({
  '--file-browser-width': `${fileBrowserWidth.value}px`,
  '--left-tools-width': `${leftToolsWidth.value}px`,
  '--right-sidebar-width': `${rightSidebarWidth.value}px`
}))
const referencePanelStyle = computed(() => ({ height: `${referencePanelCollapsed.value ? 34 : referencePanelHeight.value}px` }))
const testPanelStyle = computed(() => ({ height: `${testPanelCollapsed.value ? 34 : testPanelHeight.value}px` }))
const variablePanelStyle = computed(() => ({ flex: `0 0 ${variablePanelHeight.value}px` }))
const visibleWorkspaceNodes = computed(() => {
  const search = workspaceSearch.value.trim().toLowerCase()
  return flattenWorkspaceNodes(workspaceTree.value, 0, search)
})

onMounted(async () => {
  if (!canvas.value) return
  const nodeLoadStatus = await loadRuntimeNodeLibrary()
  editor = await createBlueprintEditor(canvas.value, {
    onZoom(value) { zoomLabel.value = `${Math.round(value * 100)}%` },
    onStatus(value) { status.value = value },
    onMetrics(value) { metrics.value = value },
    onDirty() { if (activeTab.value) activeTab.value.dirty = true },
    onFunctionSignature(value) {
      if (isFunctionBlueprintTab.value) functionSignature.value = normalizeFunctionSignature(value)
    },
    onVariables(value) { variables.value = value },
    onVariableGroups(value) { variableGroups.value = value.length ? value : [{ id: 'default', name: 'Default' }] },
    onSelection(value) {
      selectedNode.value = value ? { ...value, values: { ...value.values } } : null
      if (value) selectedVariableId.value = null
    }
  })
  await editor.newDocument()
  if (nodeLoadStatus) status.value = nodeLoadStatus
  recentFiles.value = await platform.recentFiles()
  const initialWorkspace = await platform.currentWorkingDirectory()
  if (initialWorkspace) await loadWorkspace(initialWorkspace)
  unsubscribeCloseRequest = platform.onCloseRequest(() => { void handleCloseRequest() })
  window.addEventListener('keydown', onKeyDown)
  window.addEventListener('pointerdown', closeFloatingMenus)
  window.addEventListener('beforeunload', onBeforeWindowUnload)
})

function savedPanelWidth(key: string, fallback: number, min = 140, max = 360) {
  const value = Number.parseInt(localStorage.getItem(key) ?? '', 10)
  return Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : fallback
}

function savedPanelSize(key: string, fallback: number, min: number, max: number) {
  const value = Number.parseInt(localStorage.getItem(key) ?? '', 10)
  return Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : fallback
}

function savedReferencePanelHeight() {
  const value = Number.parseInt(localStorage.getItem('origin-blueprint-reference-panel-height') ?? '', 10)
  return Number.isFinite(value) ? Math.min(360, Math.max(96, value)) : 155
}

function setLocale(locale: LocaleId) {
  currentLocale.value = locale
  localStorage.setItem('origin-blueprint-locale', locale)
}

async function loadRuntimeNodeLibrary() {
  let result
  try {
    result = await platform.loadNodeSchemas()
  } catch (error) {
    return `Node library load failed: ${error instanceof Error ? error.message : String(error)}`
  }
  if (result.nodes.length) {
    registerNodeSchemas(result.nodes)
    nodeLibrary.value = getNodeDefinitions()
  }
  if (result.errors.length) return `Loaded ${result.nodes.length} node template(s), ${result.errors.length} JSON error(s)`
  if (result.nodes.length) return `Loaded ${result.nodes.length} node template(s) from nodes`
  if (!result.documentCount) return 'No node JSON files found in nodes directory'
  return ''
}

onBeforeUnmount(() => {
  clearValidationIssueClickTimer()
  removeNodePointerListeners()
  unsubscribeCloseRequest()
  window.removeEventListener('keydown', onKeyDown); window.removeEventListener('pointerdown', closeFloatingMenus); window.removeEventListener('beforeunload', onBeforeWindowUnload); editor?.destroy()
})

function hasDirtyTabs() {
  persistActive()
  return tabs.value.some(tab => tab.dirty)
}

function onBeforeWindowUnload(event: BeforeUnloadEvent) {
  if (!hasDirtyTabs() || closingApplication) return
  event.preventDefault()
  event.returnValue = ''
}

function closeFloatingMenus(event: PointerEvent) {
  const target = event.target as HTMLElement
  if (!target.closest('.menu-root')) activeMenu.value = null
  if (!target.closest('.node-context-menu')) contextMenu.value.visible = false
  if (!target.closest('.file-context-menu')) fileContextMenu.value.visible = false
}

function onKeyDown(event: KeyboardEvent) {
  const target = event.target as HTMLElement
  if (target.matches('input, textarea, select')) return
  const ctrl = event.ctrlKey || event.metaKey
  const key = event.key.toLowerCase()
  if (ctrl && event.shiftKey && key === 'n') run(() => platform.newWindow(), event)
  else if (ctrl && key === 'n') run(newGraph, event)
  else if (ctrl && key === 'o') run(() => openGraph(), event)
  else if (ctrl && key === 's' && event.shiftKey) run(() => saveGraph(true), event)
  else if (ctrl && key === 's') run(() => saveGraph(false), event)
  else if (ctrl && key === 'a') run(() => editor?.selectAll(), event)
  else if (ctrl && key === 'd') run(() => editor?.deselectAll(), event)
  else if (ctrl && key === 'c') run(() => editor?.copy(), event)
  else if (ctrl && key === 'x') run(() => editor?.cut(), event)
  else if (ctrl && key === 'v') run(() => editor?.paste(), event)
  else if (ctrl && key === 'z') run(() => editor?.undo(), event)
  else if (ctrl && key === 'y') run(() => editor?.redo(), event)
  else if (ctrl && key === 'g') run(() => editor?.groupSelected(), event)
  else if (event.key === 'F5') run(testGraph, event)
  else if (event.altKey && event.shiftKey && key === 'b') { showLogger.value = !showLogger.value; event.preventDefault() }
  else if (event.altKey && event.shiftKey && key === 'l') { showTools.value = !showTools.value; event.preventDefault() }
  else if (event.altKey && event.shiftKey && key === 'r') { showRight.value = !showRight.value; event.preventDefault() }
  else if (event.altKey && key === 'g') run(() => editor?.ungroupSelected(), event)
  else if (event.shiftKey && key === 'l') run(() => editor?.align('left'), event)
  else if (event.shiftKey && key === 'r') run(() => editor?.align('right'), event)
  else if (event.shiftKey && key === 't') run(() => editor?.align('top'), event)
  else if (event.shiftKey && key === 'b') run(() => editor?.align('bottom'), event)
  else if (event.shiftKey && key === 'h') run(() => editor?.align('horizontal-distribute'), event)
  else if (event.shiftKey && key === 'v') run(() => editor?.align('vertical-distribute'), event)
  else if (key === 'h') run(() => editor?.align('horizontal-center'), event)
  else if (key === 'v') run(() => editor?.align('vertical-center'), event)
  else if (key === 'q') run(() => editor?.align('straighten'), event)
  else if (event.key === 'Delete' || key === 'x') run(() => editor?.deleteSelected(), event)
}

function run(action: () => void | Promise<void>, event?: Event) { event?.preventDefault(); activeMenu.value = null; void action() }
function toggleMenu(name: string) { activeMenu.value = activeMenu.value === name ? null : name }
function persistActive() { if (editor && activeTab.value) activeTab.value.document = documentWithFunctionSignature(editor.getDocument(activeTab.value.title, variables.value, variableGroups.value)) }

async function newGraph() {
  persistActive(); untitledCount++
  const tab: GraphTab = { id: crypto.randomUUID(), title: `Untitled-${untitledCount} Graph`, path: '', dirty: false, document: null }
  tabs.value.push(tab); activeTabId.value = tab.id; selectedVariableId.value = null; functionSignature.value = emptyFunctionSignature(); functionTitle.value = ''; await editor?.newDocument()
}

async function switchTab(id: string) {
  if (id === activeTabId.value) return
  persistActive(); activeTabId.value = id; selectedVariableId.value = null
  const tab = activeTab.value
  if (tab.document) await editor?.loadDocument(tab.document); else await editor?.newDocument()
  functionSignature.value = normalizeFunctionSignature(tab.document?.functionSignature)
  functionTitle.value = isFunctionBlueprintPath(tab.path || tab.title) ? functionTitleFromDocument(tab.document, tab.path || tab.title, tab.title) : ''
  nextTick(() => scrollActiveTabIntoView())
}

async function closeTab(id: string, event: MouseEvent) {
  event.stopPropagation()
  const tab = tabs.value.find(item => item.id === id)
  if (!tab || (tab.dirty && !window.confirm(`Close ${tab.title} without saving?`))) return
  const wasActive = id === activeTabId.value
  tabs.value = tabs.value.filter(item => item.id !== id)
  if (!tabs.value.length) { await newGraph(); return }
  if (wasActive) {
    activeTabId.value = tabs.value[0].id
    selectedVariableId.value = null
    functionSignature.value = normalizeFunctionSignature(tabs.value[0].document?.functionSignature)
    functionTitle.value = isFunctionBlueprintPath(tabs.value[0].path || tabs.value[0].title) ? functionTitleFromDocument(tabs.value[0].document, tabs.value[0].path || tabs.value[0].title, tabs.value[0].title) : ''
    await editor?.loadDocument(tabs.value[0].document ?? blankDocument(tabs.value[0].title))
  }
}

// --- Tab strip scroll helpers ---
function scrollActiveTabIntoView() {
  const strip = tabStrip.value
  if (!strip) return
  const activeEl = strip.querySelector('.graph-tab.active') as HTMLElement | null
  if (!activeEl) return
  const margin = 8
  const stripRect = strip.getBoundingClientRect()
  const elRect = activeEl.getBoundingClientRect()
  if (elRect.left < stripRect.left + margin) {
    strip.scrollBy({ left: elRect.left - stripRect.left - margin, behavior: 'smooth' })
  } else if (elRect.right > stripRect.right - margin) {
    strip.scrollBy({ left: elRect.right - stripRect.right + margin, behavior: 'smooth' })
  }
}

function scrollTabStrip(direction: number) {
  const strip = tabStrip.value
  if (!strip) return
  strip.scrollBy({ left: direction * 220, behavior: 'smooth' })
}

// --- Tab drag-to-reorder ---
function onTabDragStart(event: DragEvent, index: number) {
  tabDragIndex.value = index
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', String(index))
  }
}

function onTabDragOver(event: DragEvent, index: number) {
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  tabDragOverIndex.value = index
}

function onTabDragLeave() {
  tabDragOverIndex.value = -1
}

function onTabDrop(event: DragEvent, targetIndex: number) {
  event.preventDefault()
  tabDragOverIndex.value = -1
  const fromIndex = tabDragIndex.value
  if (fromIndex < 0 || fromIndex === targetIndex) return
  const arr = [...tabs.value]
  const [moved] = arr.splice(fromIndex, 1)
  arr.splice(targetIndex, 0, moved)
  tabs.value = arr
}

function onTabDragEnd() {
  tabDragIndex.value = -1
  tabDragOverIndex.value = -1
}

function blankDocument(name: string): GraphDocument {
  return { schemaVersion: 1, graphName: name, nodes: [], connections: [], groups: [], variables: [], variableGroups: [{ id: 'default', name: 'Default' }], view: { x: 0, y: 0, zoom: 1 } }
}

function functionTerminalNodes(name: string, signature = emptyFunctionSignature(), functionPath = workspaceFunctionPath(name)) {
  const entryId = crypto.randomUUID()
  const returnId = crypto.randomUUID()
  const metadata = (role: FunctionNodeMetadata['functionRole']) => ({
    functionRole: role,
    functionId: name,
    functionName: name,
    functionSource: 'workspace' as const,
    functionPath,
    functionSignature: normalizeFunctionSignature(signature)
  })
  const entry: NodeSnapshot = {
    id: entryId,
    typeId: 'origin.function.entry',
    position: { x: -320, y: 0 },
    values: {},
    properties: { label: `${name} Entry`, ...metadata('entry') }
  }
  const exit: NodeSnapshot = {
    id: returnId,
    typeId: 'origin.function.return',
    position: { x: 120, y: 0 },
    values: {},
    properties: { label: `${name} Return`, ...metadata('return') }
  }
  return {
    nodes: [entry, exit],
    connections: [{ source: entryId, sourceOutput: 'exec', target: returnId, targetInput: 'exec' }]
  }
}

function emptyFunctionSignature(): FunctionSignature {
  return { inputs: [], outputs: [] }
}

function normalizeFunctionSignature(value: unknown): FunctionSignature {
  const source = value as Partial<FunctionSignature> | undefined
  return {
    inputs: normalizeFunctionSignaturePorts(source?.inputs),
    outputs: normalizeFunctionSignaturePorts(source?.outputs)
  }
}

function normalizeFunctionSignaturePorts(value: unknown) {
  if (!Array.isArray(value)) return []
  return value.map((port, index): FunctionSignaturePort => {
    const item = port as Partial<FunctionSignaturePort>
    return {
      id: String(item.id ?? crypto.randomUUID()),
      name: String(item.name ?? `Param${index + 1}`).trim() || `Param${index + 1}`,
      type: normalizeFunctionSignaturePortType(item.type)
    }
  })
}

function normalizeFunctionSignaturePortType(value: unknown): VariableType {
  const type = normalizeVariableType(value)
  return functionSignatureTypeOptions.some(option => option.value === type) ? type : 'string'
}

function functionTitleFromDocument(document: GraphDocument | null | undefined, path: string, fallback = 'Function') {
  const title = String(document?.graphName ?? '').trim()
  return title || functionNameFromPath(path, fallback)
}

function activeFunctionTitle() {
  return functionTitle.value.trim() || functionNameFromPath(activeTab.value.path || activeTab.value.title, activeTab.value.title)
}

function documentWithFunctionSignature(document: GraphDocument, tab = activeTab.value) {
  if (isFunctionBlueprintPath(tab.path || tab.title)) {
    document.graphName = activeFunctionTitle()
    document.functionSignature = normalizeFunctionSignature(functionSignature.value)
  }
  return document
}

function hasFunctionNodes(document: GraphDocument) {
  return (document.nodes ?? []).some(node => String(node.typeId ?? '').startsWith('origin.function.'))
}

function documentRequiresNativePersistence(document: GraphDocument) {
  const signature = normalizeFunctionSignature(document.functionSignature)
  return hasFunctionNodes(document) || signature.inputs.length > 0 || signature.outputs.length > 0
}

function functionPortKey(prefix: 'input' | 'output', port: FunctionSignaturePort, index: number) {
  const key = String(port.id || port.name || `${index + 1}`).trim().replace(/[^a-zA-Z0-9_-]+/g, '-').replace(/^-+|-+$/g, '')
  return `${prefix}_${key || index + 1}`
}

function functionNodePorts(node: NodeSnapshot) {
  const signature = normalizeFunctionSignature(node.properties?.functionSignature)
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

function sameFunctionReference(properties: NodeSnapshot['properties'] | undefined, metadata: FunctionNodeMetadata) {
  if (!properties) return false
  if (metadata.functionPath && properties.functionPath === metadata.functionPath) return true
  if (metadata.functionId && properties.functionId === metadata.functionId) return true
  return Boolean(metadata.functionName && properties.functionName === metadata.functionName)
}

function syncDocumentFunctionReferences(document: GraphDocument, metadata: FunctionNodeMetadata) {
  const signature = normalizeFunctionSignature(metadata.functionSignature)
  const updatedPorts = new Map<string, ReturnType<typeof functionNodePorts>>()
  for (const node of document.nodes ?? []) {
    if (node.typeId !== 'origin.function.call' || !sameFunctionReference(node.properties, metadata)) continue
    node.properties = {
      ...node.properties,
      label: metadata.functionName,
      functionRole: 'call',
      functionId: metadata.functionId,
      functionName: metadata.functionName,
      functionSource: metadata.functionSource,
      functionPath: metadata.functionPath,
      functionSignature: signature
    }
    updatedPorts.set(node.id, functionNodePorts(node))
  }
  if (!updatedPorts.size) return false
  document.connections = (document.connections ?? []).filter(connection => {
    const sourcePorts = updatedPorts.get(connection.source)
    if (sourcePorts && !sourcePorts.outputs.has(connection.sourceOutput)) return false
    const targetPorts = updatedPorts.get(connection.target)
    if (targetPorts && !targetPorts.inputs.has(connection.targetInput)) return false
    return true
  })
  return true
}

function syncFunctionTerminalsFromDocumentSignature(document: GraphDocument, path: string) {
  const signature = normalizeFunctionSignature(document.functionSignature)
  const functionName = functionNameFromPath(path, document.graphName)
  const changedPorts = new Map<string, ReturnType<typeof functionNodePorts>>()
  for (const node of document.nodes ?? []) {
    if (node.typeId !== 'origin.function.entry' && node.typeId !== 'origin.function.return') continue
    const role = node.typeId === 'origin.function.entry' ? 'entry' : 'return'
    node.properties = {
      ...node.properties,
      label: role === 'entry' ? `${functionName} Entry` : `${functionName} Return`,
      functionRole: role,
      functionId: path || document.graphName,
      functionName,
      functionSource: 'workspace',
      functionPath: path,
      functionSignature: signature
    }
    changedPorts.set(node.id, functionNodePorts(node))
  }
  if (!changedPorts.size) return false
  document.connections = (document.connections ?? []).filter(connection => {
    const sourcePorts = changedPorts.get(connection.source)
    if (sourcePorts && !sourcePorts.outputs.has(connection.sourceOutput)) return false
    const targetPorts = changedPorts.get(connection.target)
    if (targetPorts && !targetPorts.inputs.has(connection.targetInput)) return false
    return true
  })
  return true
}

async function loadFunctionSignatureForPath(path: string) {
  const opened = tabs.value.find(tab => tab.path === path)
  if (opened?.document) return normalizeFunctionSignature(opened.document.functionSignature)
  try {
    const file = await platform.openGraph(path)
    if (!file) return emptyFunctionSignature()
    const parsed = JSON.parse(file.content) as Partial<GraphDocument>
    return normalizeFunctionSignature(parsed.functionSignature)
  } catch (error) {
    status.value = `读取函数签名失败: ${error instanceof Error ? error.message : String(error)}`
    return emptyFunctionSignature()
  }
}

function functionNameFromPath(path: string, fallback = 'Function') {
  const name = (path || fallback).split(/[\\/]/).pop()?.replace(/\.(obpf|obp|vgf)$/i, '')
  return name?.trim() || fallback
}

async function refreshDocumentFunctionReferencesOnOpen(document: GraphDocument, path: string) {
  let changed = false
  if (isFunctionBlueprintPath(path || document.graphName)) {
    changed = syncFunctionTerminalsFromDocumentSignature(document, path) || changed
  }

  const functionCalls = new Map<string, string>()
  for (const node of document.nodes ?? []) {
    if (node.typeId !== 'origin.function.call' || !node.properties?.functionPath) continue
    if (node.properties.functionPath === path) continue
    functionCalls.set(node.properties.functionPath, node.properties.functionName || node.properties.label || 'Function')
  }
  for (const [functionPath, fallbackName] of functionCalls) {
    const signature = await loadFunctionSignatureForPath(functionPath)
    const metadata: FunctionNodeMetadata = {
      functionRole: 'call',
      functionId: functionPath,
      functionName: functionNameFromPath(functionPath, fallbackName),
      functionSource: 'workspace',
      functionPath,
      functionSignature: signature
    }
    changed = syncDocumentFunctionReferences(document, metadata) || changed
  }
  return changed
}

async function syncOpenFunctionReferences(metadata: FunctionNodeMetadata) {
  const normalizedMetadata: FunctionNodeMetadata = {
    ...metadata,
    functionRole: 'call',
    functionSignature: normalizeFunctionSignature(metadata.functionSignature)
  }
  for (const tab of tabs.value) {
    if (tab.id === activeTabId.value) {
      if (isFunctionBlueprintPath(tab.path || tab.title) || !editor) continue
      const activeDocument = editor.getDocument(tab.title, variables.value, variableGroups.value)
      if (!syncDocumentFunctionReferences(activeDocument, normalizedMetadata)) continue
      await editor?.syncFunctionSignature(normalizedMetadata)
      tab.document = editor.getDocument(tab.title, variables.value, variableGroups.value)
      tab.dirty = true
      continue
    }
    if (!tab.document) continue
    if (syncDocumentFunctionReferences(tab.document, normalizedMetadata)) tab.dirty = true
  }
}

function defaultVariableValue(type: VariableType) {
  if (type === 'boolean') return false
  if (type === 'integer' || type === 'float') return 0
  if (type === 'array') return []
  if (type === 'table') return { columns: [], rows: [] }
  if (type === 'dictionary') return {}
  return ''
}

async function syncVariables(refreshNodes = false) {
  await editor?.setVariables(variables.value, variableGroups.value, refreshNodes)
  activeTab.value.dirty = true
}

async function addVariable(groupId = 'default') {
  let index = variables.value.length + 1
  while (variables.value.some(item => item.name === `Variable${index}`)) index++
  const variable: GraphVariable = { id: crypto.randomUUID(), name: `Variable${index}`, type: 'integer', defaultValue: 0, groupId }
  variables.value.push(variable)
  await syncVariables()
  await selectVariable(variable)
}

async function updateVariable(variable: GraphVariable, previousType?: VariableType) {
  variable.name = variable.name.trim() || 'Variable'
  if (previousType && previousType !== variable.type) variable.defaultValue = defaultVariableValue(variable.type)
  await syncVariables(true)
}

async function changeVariableType(variable: GraphVariable) {
  variable.defaultValue = defaultVariableValue(variable.type)
  await updateVariable(variable)
}

async function setVariableArrayDefault(variable: GraphVariable, event: Event) {
  const text = (event.target as HTMLInputElement).value
  variable.defaultValue = text.split(',').map(item => item.trim()).filter(Boolean).map(item => /^-?\d+(\.\d+)?$/.test(item) ? Number(item) : item)
  await updateVariable(variable)
}

async function setVariableStructuredDefault(variable: GraphVariable, event: Event) {
  try {
    variable.defaultValue = JSON.parse((event.target as HTMLTextAreaElement).value)
    await updateVariable(variable)
  } catch {
    status.value = 'Invalid JSON default value'
  }
}

async function removeVariable(variable: GraphVariable) {
  const document = editor?.getDocument(activeTab.value.title, variables.value, variableGroups.value)
  const references = document?.nodes.filter(node => node.properties?.variableId === variable.id).length ?? 0
  if (references && !window.confirm(`${variable.name} is used by ${references} node(s). Delete it and leave those nodes invalid?`)) return
  variables.value = variables.value.filter(item => item.id !== variable.id)
  if (selectedVariableId.value === variable.id) selectedVariableId.value = null
  await syncVariables(true)
}

function selectBlueprintFunction(item: BlueprintFunction) {
  selectedFunctionId.value = item.id
  selectedVariableId.value = null
  status.value = `Selected function ${item.name}`
}

function sanitizeFunctionFileName(value: string) {
  return value.trim().replace(/\.(obpf|obp|vgf)$/i, '').replace(/[<>:"/\\|?*\x00-\x1f]+/g, '_').replace(/\s+/g, '_').replace(/^_+|_+$/g, '') || 'NewFunction'
}

function workspaceFunctionPath(name: string) {
  const root = workspaceRoot.value.replace(/[\\/]+$/, '')
  const separator = root.includes('\\') ? '\\' : '/'
  return `${root}${separator}functions${separator}${sanitizeFunctionFileName(name)}.obpf`
}

function joinWorkspacePath(directory: string, fileName: string) {
  const base = (directory || workspaceRoot.value).replace(/[\\/]+$/, '')
  const separator = base.includes('\\') ? '\\' : '/'
  return `${base}${separator}${fileName}`
}

async function refreshWorkspaceAfterFileCreate(savedPath: string) {
  if (workspaceRoot.value) await loadWorkspace(workspaceRoot.value)
  await openGraph(savedPath)
}

async function createBlueprintAtDirectory(directory: string) {
  const rawName = window.prompt('蓝图名称', 'NewBlueprint')
  if (!rawName) return
  const graphName = sanitizeFunctionFileName(rawName)
  const path = joinWorkspacePath(directory, `${graphName}.vgf`)
  const saved = await platform.saveGraph(path, JSON.stringify(blankDocument(graphName), null, 2))
  if (!saved) return
  await refreshWorkspaceAfterFileCreate(saved)
  status.value = `Created blueprint ${graphName}`
}

async function createFunctionAtDirectory(directory: string) {
  const rawName = window.prompt('工程函数名称', 'NewFunction')
  if (!rawName) return
  const graphName = sanitizeFunctionFileName(rawName)
  const path = joinWorkspacePath(directory, `${graphName}.obpf`)
  const document = blankDocument(graphName)
  document.functionSignature = emptyFunctionSignature()
  const terminals = functionTerminalNodes(graphName, document.functionSignature, path)
  document.nodes = terminals.nodes
  document.connections = terminals.connections
  const saved = await platform.saveGraph(path, JSON.stringify(document, null, 2))
  if (!saved) return
  await refreshWorkspaceAfterFileCreate(saved)
  status.value = `Created function ${graphName}`
}

function uniqueBlueprintFunctionName(base = 'New Function') {
  const names = new Set(blueprintFunctions.value.map(item => item.name.toLowerCase()))
  if (!names.has(base.toLowerCase())) return base
  let index = 2
  while (names.has(`${base} ${index}`.toLowerCase())) index++
  return `${base} ${index}`
}

function addBlueprintFunction() {
  const fallback = uniqueBlueprintFunctionName()
  const name = window.prompt('函数名称', fallback)?.trim()
  if (!name) return
  const item = { id: crypto.randomUUID(), name: uniqueBlueprintFunctionName(name) }
  blueprintFunctions.value.push(item)
  selectBlueprintFunction(item)
}

function renameBlueprintFunction(item: BlueprintFunction) {
  if (item.readonly) return
  const name = window.prompt('重命名函数', item.name)?.trim()
  if (!name || name === item.name) return
  item.name = uniqueBlueprintFunctionName(name)
  selectBlueprintFunction(item)
}

function removeBlueprintFunction(item: BlueprintFunction) {
  if (item.readonly) return
  if (!window.confirm(`删除函数 ${item.name}？`)) return
  blueprintFunctions.value = blueprintFunctions.value.filter(entry => entry.id !== item.id)
  if (selectedFunctionId.value === item.id) selectedFunctionId.value = blueprintFunctions.value[0]?.id ?? ''
  status.value = `Deleted function ${item.name}`
}

async function selectVariable(variable: GraphVariable) {
  await editor?.deselectAll()
  selectedVariableId.value = variable.id
  selectedFunctionId.value = ''
}

async function addVariableGroup() {
  const rawName = window.prompt('Variable group name', 'New Group')
  const name = rawName?.trim()
  if (!name) return
  if (variableGroups.value.some(group => group.name.toLowerCase() === name.toLowerCase())) {
    status.value = `Variable group already exists: ${name}`
    return
  }
  variableGroups.value.push({ id: crypto.randomUUID(), name })
  await syncVariables()
}

async function renameVariableGroup(group: GraphVariableGroup) {
  if (group.id === 'default') return
  const rawName = window.prompt('Rename variable group', group.name)
  const name = rawName?.trim()
  if (!name || name === group.name) return
  if (variableGroups.value.some(item => item.id !== group.id && item.name.toLowerCase() === name.toLowerCase())) {
    status.value = `Variable group already exists: ${name}`
    return
  }
  group.name = name
  await syncVariables()
}

async function removeVariableGroup(group: GraphVariableGroup) {
  if (group.id === 'default') return
  const count = variables.value.filter(variable => variable.groupId === group.id).length
  if (count && !window.confirm(`Move ${count} variable(s) from ${group.name} to Default and delete the group?`)) return
  for (const variable of variables.value) if (variable.groupId === group.id) variable.groupId = 'default'
  variableGroups.value = variableGroups.value.filter(item => item.id !== group.id)
  await syncVariables()
}

async function toggleVariableGroup(group: GraphVariableGroup) {
  group.collapsed = !group.collapsed
  await syncVariables()
}

function startVariableDrag(event: DragEvent, variable: GraphVariable) {
  event.dataTransfer?.setData('application/x-origin-variable', variable.id)
  event.dataTransfer?.setData('application/x-origin-variable-access', event.altKey ? 'set' : 'get')
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'copy'
}

async function createVariableNode(variable: GraphVariable, access: 'get' | 'set', position?: { x: number; y: number }) {
  await editor?.addVariableNode(variable, access, position)
}

async function applyNodeProperties() {
  if (!selectedNode.value) return
  await editor?.updateSelectedNode(selectedNode.value.label, selectedNode.value.values)
}

function setSelectedValue(key: string, event: Event) {
  if (!selectedNode.value) return
  const input = event.target as HTMLInputElement
  const current = selectedNode.value.values[key]
  selectedNode.value.values[key] = typeof current === 'number' ? Number(input.value) : input.value
}

function setSelectedArrayValue(key: string, event: Event) {
  if (!selectedNode.value) return
  selectedNode.value.values[key] = (event.target as HTMLInputElement).value.split(',').map(item => item.trim()).filter(Boolean).map(item => /^-?\d+(\.\d+)?$/.test(item) ? Number(item) : item)
}

function touchFunctionSignature() {
  if (isFunctionBlueprintTab.value) activeTab.value.dirty = true
}

async function syncFunctionTitleToGraph() {
  if (!isFunctionBlueprintTab.value) return
  functionTitle.value = activeFunctionTitle()
  if (activeTab.value.document) activeTab.value.document.graphName = functionTitle.value
  if (activeTab.value.path) functionTitleByPath.value = { ...functionTitleByPath.value, [activeTab.value.path]: functionTitle.value }
  await syncFunctionSignatureToGraph()
}

function activeFunctionMetadata(role: FunctionNodeMetadata['functionRole']): FunctionNodeMetadata {
  const functionName = activeFunctionTitle()
  return {
    functionRole: role,
    functionId: activeTab.value.path || activeTab.value.title,
    functionName,
    functionSource: 'workspace',
    functionPath: activeTab.value.path,
    functionSignature: normalizeFunctionSignature(functionSignature.value)
  }
}

async function syncFunctionSignatureToGraph() {
  touchFunctionSignature()
  if (!isFunctionBlueprintTab.value) return
  await editor?.syncFunctionSignature(activeFunctionMetadata('entry'))
  await syncOpenFunctionReferences(activeFunctionMetadata('call'))
}

function addFunctionSignaturePort(direction: 'inputs' | 'outputs') {
  const label = direction === 'inputs' ? 'Input' : 'Output'
  functionSignature.value[direction].push({ id: crypto.randomUUID(), name: `${label}${functionSignature.value[direction].length + 1}`, type: 'integer' })
  void syncFunctionSignatureToGraph()
}

function removeFunctionSignaturePort(direction: 'inputs' | 'outputs', port: FunctionSignaturePort) {
  functionSignature.value[direction] = functionSignature.value[direction].filter(item => item.id !== port.id)
  void syncFunctionSignatureToGraph()
}

function normalizeVariableType(value: unknown): VariableType {
  const type = String(value ?? '').toLowerCase()
  if (type === 'bool' || type === 'boolean') return 'boolean'
  if (type === 'int' || type === 'integer') return 'integer'
  if (type === 'float' || type === 'double' || type === 'number') return 'float'
  if (type === 'array' || type === 'list') return 'array'
  if (type === 'file') return 'file'
  if (type === 'dataframe' || type === 'table') return 'table'
  if (type === 'dict' || type === 'dictionary' || type === 'map') return 'dictionary'
  return 'string'
}

function normalizeDocument(value: any): GraphDocument {
  const sourceVariables = Array.isArray(value.variables) ? value.variables : []
  const groups: GraphVariableGroup[] = []
  const groupIds = new Set<string>()
  const groupNames = new Set<string>()
  const addGroup = (id: string, name: string, collapsed = false) => {
    const cleanId = id.trim()
    const cleanName = name.trim()
    if (!cleanId || !cleanName || groupIds.has(cleanId) || groupNames.has(cleanName.toLowerCase())) return
    groupIds.add(cleanId); groupNames.add(cleanName.toLowerCase()); groups.push({ id: cleanId, name: cleanName, collapsed })
  }
  addGroup('default', 'Default')
  for (const group of Array.isArray(value.variableGroups) ? value.variableGroups : []) {
    if (group?.id === 'default') {
      groups[0].collapsed = Boolean(group.collapsed)
      continue
    }
    addGroup(String(group?.id ?? ''), String(group?.name ?? ''), Boolean(group?.collapsed))
  }
  for (const variable of sourceVariables) {
    const legacyName = String(variable?.group ?? '').trim()
    if (legacyName && legacyName.toLowerCase() !== 'default' && !groupNames.has(legacyName.toLowerCase())) addGroup(crypto.randomUUID(), legacyName)
  }
  const groupByName = new Map(groups.map(group => [group.name.toLowerCase(), group.id]))
  const variables: GraphVariable[] = sourceVariables.map((variable: any, index: number) => {
    const type = normalizeVariableType(variable?.type)
    const requestedGroupId = String(variable?.groupId ?? '')
    const legacyGroupId = groupByName.get(String(variable?.group ?? 'Default').toLowerCase())
    return {
      id: String(variable?.id || crypto.randomUUID()),
      name: String(variable?.name || `Variable${index + 1}`),
      type,
      defaultValue: variable?.defaultValue ?? variable?.value ?? defaultVariableValue(type),
      groupId: groupIds.has(requestedGroupId) ? requestedGroupId : (legacyGroupId ?? 'default'),
      description: String(variable?.description ?? '')
    }
  })
  return {
    schemaVersion: 1,
    graphName: String(value.graphName ?? value.graph_name ?? 'Imported Graph'),
    nodes: Array.isArray(value.nodes) ? value.nodes : [],
    connections: Array.isArray(value.connections) ? value.connections : [],
    groups: Array.isArray(value.groups) ? value.groups : [],
    variables,
    variableGroups: groups,
    functionSignature: normalizeFunctionSignature(value.functionSignature),
    view: value.view ?? { x: 0, y: 0, zoom: 1 },
    legacy: value.legacy
  }
}

function isNativeGraphDocument(value: any) {
  if (value?.schemaVersion !== 1) return false
  if (!Array.isArray(value.nodes)) return true
  return value.nodes.every((node: any) => typeof node?.typeId === 'string')
}

function isLegacyGraphPath(path: string) {
  return /\.vgf$/i.test(path)
}

function isFunctionBlueprintPath(path: string) {
  return /\.obpf$/i.test(path)
}

async function testGraph() {
  if (!editor) return
  await editor.highlightIssueNodes([])
  const document = editor.getDocument(activeTab.value.title, variables.value, variableGroups.value)
  validationIssues.value = await platform.validateGraph(JSON.stringify(document))
  selectedValidationIssueKey.value = ''
  showLogger.value = true
  const errors = validationIssues.value.filter(issue => issue.severity === 'error').length
  const warnings = validationIssues.value.filter(issue => issue.severity === 'warning').length
  status.value = validationIssues.value.length ? `检查发现 ${errors} 个错误，${warnings} 个警告` : '蓝图检查通过'
}

function validationIssueKey(issue: ValidationIssue, index: number) {
  return `${index}:${issue.severity}:${issue.code}:${issueNodeIds(issue).join(',')}:${issue.message}`
}

function issueNodeIds(issue: ValidationIssue) {
  const ids = issue.nodeIds?.length ? issue.nodeIds : issue.nodeId ? [issue.nodeId] : []
  return [...new Set(ids.filter(Boolean))]
}

function clearValidationIssueClickTimer() {
  if (validationIssueClickTimer) window.clearTimeout(validationIssueClickTimer)
  validationIssueClickTimer = undefined
}

function queueSelectIssue(issue: ValidationIssue, index: number) {
  clearValidationIssueClickTimer()
  validationIssueClickTimer = window.setTimeout(() => {
    void selectIssue(issue, index)
    validationIssueClickTimer = undefined
  }, 180)
}

async function selectIssue(issue: ValidationIssue, index: number) {
  selectedValidationIssueKey.value = validationIssueKey(issue, index)
  const ids = issueNodeIds(issue)
  if (ids.length === 1) await editor?.focusNode(ids[0])
}

async function highlightIssue(issue: ValidationIssue, index: number) {
  clearValidationIssueClickTimer()
  selectedValidationIssueKey.value = validationIssueKey(issue, index)
  const ids = issueNodeIds(issue)
  if (!ids.length) {
    status.value = '该问题没有对应结点'
    return
  }
  const count = await editor?.highlightIssueNodes(ids) ?? 0
  status.value = count ? `已用红色警示框标出 ${count} 个问题结点` : '未找到对应结点'
}

async function openGraph(path = '', highlightTypeId = '') {
  const file = await platform.openGraph(path)
  if (!file) return
  let parsed: any
  try { parsed = JSON.parse(file.content) } catch { status.value = 'Invalid graph file'; return }
  let document: GraphDocument
  if (isNativeGraphDocument(parsed)) document = normalizeDocument(parsed)
  else if (platform.isDesktop()) {
    try { document = normalizeDocument(JSON.parse(await platform.migrateLegacyGraph(file.content))) }
    catch (error) { status.value = error instanceof Error ? error.message : 'Legacy graph migration failed'; return }
  } else { status.value = 'Legacy graph migration requires the desktop runtime'; return }
  await refreshDocumentFunctionReferencesOnOpen(document, file.path)
  persistActive()
  const existing = tabs.value.find(tab => tab.path === file.path)
  if (existing) {
    persistActive()
    existing.document = document
    existing.dirty = false
    activeTabId.value = existing.id
    selectedVariableId.value = null
    functionSignature.value = normalizeFunctionSignature(document.functionSignature)
    functionTitle.value = isFunctionBlueprintPath(file.path) ? functionTitleFromDocument(document, file.path, existing.title) : ''
    if (isFunctionBlueprintPath(file.path)) functionTitleByPath.value = { ...functionTitleByPath.value, [file.path]: functionTitle.value }
    await editor?.loadDocument(document)
    if (highlightTypeId) {
      const count = await editor?.highlightNodesByType(highlightTypeId) ?? 0
      status.value = count ? `已高亮 ${count} 个引用结点` : '该蓝图中未找到引用结点'
    }
    return
  }
  const title = file.path.split(/[\\/]/).pop() ?? document.graphName
  const tab: GraphTab = { id: crypto.randomUUID(), title, path: file.path, dirty: false, document }
  tabs.value.push(tab)
  activeTabId.value = tab.id
  selectedVariableId.value = null
  functionSignature.value = normalizeFunctionSignature(document.functionSignature)
  functionTitle.value = isFunctionBlueprintPath(file.path) ? functionTitleFromDocument(document, file.path, title) : ''
  if (isFunctionBlueprintPath(file.path)) functionTitleByPath.value = { ...functionTitleByPath.value, [file.path]: functionTitle.value }
  await editor?.loadDocument(document)
  if (document.legacy?.format === 'vgf') {
    const hiddenCount = document.legacy.hiddenNodes?.length ?? 0
    status.value = `Loaded ${document.nodes.length} visible node(s), ${hiddenCount} hidden undefined node(s)`
  }
  if (highlightTypeId) {
    const count = await editor?.highlightNodesByType(highlightTypeId) ?? 0
    status.value = count ? `已高亮 ${count} 个引用结点` : '该蓝图中未找到引用结点'
  }
  recentFiles.value = await platform.recentFiles()
}

async function saveGraph(saveAs: boolean) {
  if (!editor) return
  const tab = activeTab.value
  const document = documentWithFunctionSignature(editor.getDocument(tab.title, variables.value, variableGroups.value), tab)
  const shouldSaveLegacy = !saveAs && isLegacyGraphPath(tab.path) && !documentRequiresNativePersistence(document)
  const content = shouldSaveLegacy ? await platform.exportLegacyGraph(JSON.stringify(document)) : JSON.stringify(document, null, 2)
  const path = await platform.saveGraph(saveAs ? '' : tab.path, content)
  if (!path) return
  tab.path = path; tab.title = path.split(/[\\/]/).pop() ?? tab.title; tab.document = document; tab.dirty = false
  if (isFunctionBlueprintPath(path)) {
    functionTitle.value = activeFunctionTitle()
    functionTitleByPath.value = { ...functionTitleByPath.value, [path]: functionTitle.value }
    await syncOpenFunctionReferences(activeFunctionMetadata('call'))
  }
  recentFiles.value = await platform.recentFiles(); status.value = `Saved ${tab.title}`
}

async function saveAll() {
  const active = activeTabId.value
  for (const tab of tabs.value) {
    if (!tab.dirty) continue
    await switchTab(tab.id); await saveGraph(false)
  }
  await switchTab(active)
}

async function confirmUnsavedBeforeClose() {
  if (!hasDirtyTabs()) return true
  const dirtyNames = tabs.value.filter(tab => tab.dirty).map(tab => tab.title)
  const action = await requestUnsavedCloseAction(dirtyNames)
  if (action === 'save') {
    await saveAll()
    return !hasDirtyTabs()
  }
  return action === 'discard'
}

function requestUnsavedCloseAction(names: string[]) {
  return new Promise<UnsavedCloseAction>(resolve => {
    unsavedCloseDialog.value = { visible: true, names, resolve }
  })
}

function resolveUnsavedCloseAction(action: UnsavedCloseAction) {
  const resolve = unsavedCloseDialog.value.resolve
  unsavedCloseDialog.value = { visible: false, names: [] }
  resolve?.(action)
}

async function handleCloseRequest() {
  if (closingApplication) return
  if (!(await confirmUnsavedBeforeClose())) return
  closingApplication = true
  await platform.quit()
}

async function chooseWorkspace() {
  const path = await platform.chooseWorkspace(); if (path) await loadWorkspace(path)
}

async function clearRecentFiles() {
  await platform.clearRecentFiles()
  recentFiles.value = []
  status.value = 'Recent graph list cleared'
}

async function quitApplication() {
  await handleCloseRequest()
}
async function loadWorkspace(path: string) {
  const token = ++workspaceLoadToken
  workspaceRoot.value = path
  expandedWorkspacePaths.value = new Set()
  workspaceTree.value = []
  workspaceTree.value = await loadWorkspaceTree(path)
  void hydrateWorkspaceTree(workspaceTree.value, 1, token)
}

async function loadWorkspaceTree(path: string, depth = 0): Promise<WorkspaceTreeNode[]> {
  if (depth > 8) return []
  const entries = await platform.listWorkspace(path)
  return entries.map(entry => ({ ...entry, children: [], loaded: !entry.isDir, loading: false }))
}

async function ensureWorkspaceChildren(node: WorkspaceTreeNode, depth: number) {
  if (!node.isDir || node.loaded || node.loading) return
  node.loading = true
  try {
    node.children = await loadWorkspaceTree(node.path, depth)
    node.loaded = true
  } catch (error) {
    status.value = `Workspace load failed: ${error instanceof Error ? error.message : String(error)}`
  } finally {
    node.loading = false
  }
}

function workspaceNodeDepth(path: string) {
  const root = workspaceRoot.value.replace(/[\\/]+$/, '')
  const relative = path.startsWith(root) ? path.slice(root.length).replace(/^[\\/]+/, '') : path
  return relative ? relative.split(/[\\/]/).filter(Boolean).length : 0
}

async function hydrateWorkspaceTree(nodes: WorkspaceTreeNode[], depth: number, token: number) {
  if (token !== workspaceLoadToken || depth > 8) return
  for (const node of nodes) {
    if (token !== workspaceLoadToken) return
    await ensureWorkspaceChildren(node, depth)
    if (node.children.length) await hydrateWorkspaceTree(node.children, depth + 1, token)
    await new Promise(resolve => setTimeout(resolve, 0))
  }
}

watch(workspaceSearch, value => {
  if (value.trim()) void hydrateWorkspaceTree(workspaceTree.value, 1, workspaceLoadToken)
})

watch(functionLibraryItems, items => {
  void loadFunctionLibraryTitles(items)
}, { immediate: true })

function flattenWorkspaceNodes(nodes: WorkspaceTreeNode[], depth: number, search: string): VisibleWorkspaceNode[] {
  const rows: VisibleWorkspaceNode[] = []
  for (const node of nodes) {
    const childRows = flattenWorkspaceNodes(node.children, depth + 1, search)
    const selfMatches = search ? !node.isDir && node.name.toLowerCase().includes(search) : true
    if (search) {
      if (selfMatches || childRows.length) rows.push({ node, depth }, ...childRows)
      continue
    }
    rows.push({ node, depth })
    if (node.isDir && expandedWorkspacePaths.value.has(node.path)) rows.push(...childRows)
  }
  return rows
}

function isFunctionResource(node: WorkspaceEntry) {
  const normalizedPath = node.path.replace(/\\/g, '/').toLowerCase()
  const name = node.name.toLowerCase()
  if (node.isDir) return name === 'functions' || name === 'function'
  return name.endsWith('.obpf') || normalizedPath.includes('/functions/') || normalizedPath.includes('/function/')
}

function functionResourceName(node: WorkspaceEntry) {
  return node.name.replace(/\.(obpf|obp|vgf)$/i, '')
}

function functionResourceTitle(node: WorkspaceEntry) {
  const opened = tabs.value.find(tab => tab.path === node.path)
  if (opened?.id === activeTabId.value && isFunctionBlueprintPath(opened.path || opened.title)) return activeFunctionTitle()
  const openedTitle = String(opened?.document?.graphName ?? '').trim()
  return openedTitle || functionTitleByPath.value[node.path] || functionResourceName(node)
}

function collectFunctionLibraryItems(nodes: WorkspaceTreeNode[]) {
  const items: FunctionLibraryItem[] = []
  const visit = (entry: WorkspaceTreeNode) => {
    if (!entry.isDir && isFunctionResource(entry)) {
      items.push({
        id: encodeURIComponent(entry.path).replace(/%/g, '_'),
        name: functionResourceTitle(entry),
        path: entry.path,
        source: 'workspace'
      })
    }
    for (const child of entry.children) visit(child)
  }
  for (const node of nodes) visit(node)
  return items
}

async function loadFunctionLibraryTitles(items: FunctionLibraryItem[]) {
  for (const item of items) {
    if (!item.path || functionTitleByPath.value[item.path] || loadingFunctionTitles.has(item.path)) continue
    const opened = tabs.value.find(tab => tab.path === item.path)
    if (opened?.document?.graphName) {
      functionTitleByPath.value = { ...functionTitleByPath.value, [item.path]: opened.document.graphName }
      continue
    }
    loadingFunctionTitles.add(item.path)
    try {
      const file = await platform.openGraph(item.path)
      if (!file) continue
      const parsed = JSON.parse(file.content) as Partial<GraphDocument>
      const title = String(parsed.graphName ?? '').trim()
      if (title) functionTitleByPath.value = { ...functionTitleByPath.value, [item.path]: title }
    } catch {
      // Function title loading is best-effort; fall back to the file name.
    } finally {
      loadingFunctionTitles.delete(item.path)
    }
  }
}

async function toggleWorkspaceNode(node: WorkspaceTreeNode) {
  selectedWorkspacePath.value = node.path
  if (!node.isDir) return
  const next = new Set(expandedWorkspacePaths.value)
  if (next.has(node.path)) next.delete(node.path); else {
    next.add(node.path)
    await ensureWorkspaceChildren(node, workspaceNodeDepth(node.path) + 1)
  }
  expandedWorkspacePaths.value = next
}

async function workspaceOpen(item: WorkspaceTreeNode) {
  selectedWorkspacePath.value = item.path
  if (item.isDir) await toggleWorkspaceNode(item); else await openGraph(item.path)
}

function openFileContextMenu(event: MouseEvent, node: WorkspaceTreeNode | WorkspaceEntry | NodeReferenceResult) {
  const isDir = 'isDir' in node ? node.isDir : false
  fileContextMenu.value = { visible: true, x: event.clientX, y: event.clientY, path: node.path, isDir, isFunction: isFunctionBlueprintPath(node.path) }
}

function workspaceIndent(depth: number) {
  return `${8 + depth * 16}px`
}

function beginLeftSidebarResize(event: PointerEvent) {
  if (event.button !== 0) return
  event.preventDefault()
  const startX = event.clientX
  const startFileWidth = fileBrowserWidth.value
  const startToolsWidth = leftToolsWidth.value
  const totalWidth = startFileWidth + startToolsWidth
  const minWidth = 140

  const move = (next: PointerEvent) => {
    const fileWidth = Math.min(totalWidth - minWidth, Math.max(minWidth, startFileWidth + next.clientX - startX))
    fileBrowserWidth.value = Math.round(fileWidth)
    leftToolsWidth.value = Math.round(totalWidth - fileWidth)
  }
  const up = () => {
    localStorage.setItem('origin-blueprint-file-browser-width', String(fileBrowserWidth.value))
    localStorage.setItem('origin-blueprint-left-tools-width', String(leftToolsWidth.value))
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', up)
}

function beginLeftToolsResize(event: PointerEvent) {
  if (event.button !== 0) return
  event.preventDefault()
  const startX = event.clientX
  const startWidth = leftToolsWidth.value
  const move = (next: PointerEvent) => {
    leftToolsWidth.value = Math.min(520, Math.max(160, Math.round(startWidth + next.clientX - startX)))
  }
  const up = () => {
    localStorage.setItem('origin-blueprint-left-tools-width', String(leftToolsWidth.value))
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', up)
}

async function exportImage(selected: boolean) {
  if (!canvas.value) return
  if (selected) await editor?.fitSelected(); else await editor?.resetView()
  await nextTick(); await new Promise(resolve => setTimeout(resolve, 120))
  const data = await toPng(canvas.value, { backgroundColor: '#202020', pixelRatio: 2, cacheBust: true })
  const path = await platform.exportPNG(data); status.value = path ? `Exported ${path}` : 'Export cancelled'
}

async function addNodeAt(typeId: string, position?: { x: number; y: number }) {
  try {
    await editor?.addNode(typeId, position ?? visibleCanvasInsertPosition())
  } catch (error) {
    status.value = error instanceof Error ? error.message : String(error)
  }
}

async function loadFunctionSignatureForModuleItem(item: ModuleLibraryItem) {
  if (item.functionSource !== 'workspace' || !item.path) return normalizeFunctionSignature(functionSignature.value)
  const opened = tabs.value.find(tab => tab.path === item.path)
  if (opened?.document) return normalizeFunctionSignature(opened.document.functionSignature)
  try {
    const file = await platform.openGraph(item.path)
    if (!file) return emptyFunctionSignature()
    const parsed = JSON.parse(file.content) as Partial<GraphDocument>
    return normalizeFunctionSignature(parsed.functionSignature)
  } catch (error) {
    status.value = `读取函数签名失败: ${error instanceof Error ? error.message : String(error)}`
    return emptyFunctionSignature()
  }
}

async function functionMetadataForModuleItem(item: ModuleLibraryItem): Promise<FunctionNodeMetadata> {
  const source = item.functionSource ?? 'current'
  return {
    functionRole: 'call',
    functionId: item.functionItem?.id ?? item.id,
    functionName: item.title,
    functionSource: source,
    functionPath: item.path,
    functionSignature: source === 'current' ? normalizeFunctionSignature(functionSignature.value) : await loadFunctionSignatureForModuleItem(item)
  }
}

function isSelfFunctionReference(item: ModuleLibraryItem) {
  if (!item.functionPlaceholder || !isFunctionBlueprintTab.value || !item.path || !activeTab.value.path) return false
  return item.path.replace(/\\/g, '/').toLowerCase() === activeTab.value.path.replace(/\\/g, '/').toLowerCase()
}

async function addModuleItemAt(item: ModuleLibraryItem, position?: { x: number; y: number }) {
  if (item.functionPlaceholder) {
    if (isSelfFunctionReference(item)) {
      status.value = '函数不能引用自身'
      return
    }
    try {
      await editor?.addFunctionCallNode(await functionMetadataForModuleItem(item), position ?? visibleCanvasInsertPosition())
    } catch (error) {
      status.value = error instanceof Error ? error.message : String(error)
    }
    return
  }
  await addNodeAt(item.id, position)
}

async function addFunctionEntryNodeToGraph() {
  if (!isFunctionBlueprintTab.value) return
  await editor?.addFunctionEntryNode(activeFunctionMetadata('entry'), visibleCanvasInsertPosition())
}

async function addFunctionReturnNodeToGraph() {
  if (!isFunctionBlueprintTab.value) return
  await editor?.addFunctionReturnNode(activeFunctionMetadata('return'), visibleCanvasInsertPosition())
}

function visibleCanvasInsertPosition() {
  const rect = canvas.value?.getBoundingClientRect()
  if (!rect) return undefined
  return { x: rect.left + rect.width * 0.42, y: rect.top + rect.height * 0.36 }
}

function beginModuleItemPointerDrag(event: PointerEvent, item: ModuleLibraryItem) {
  if (event.button !== 0) return
  removeNodePointerListeners()
  nodePointerDrag = { item, startX: event.clientX, startY: event.clientY, lastX: event.clientX, lastY: event.clientY, moved: false }
  status.value = `Dragging ${item.title}`

  const move = (next: PointerEvent) => {
    if (!nodePointerDrag) return
    nodePointerDrag.lastX = next.clientX
    nodePointerDrag.lastY = next.clientY
    const dx = next.clientX - nodePointerDrag.startX
    const dy = next.clientY - nodePointerDrag.startY
    if (Math.hypot(dx, dy) > 3) nodePointerDrag.moved = true
  }

  const up = (next: PointerEvent) => {
    const drag = nodePointerDrag
    removeNodePointerListeners()
    nodePointerDrag = null
    if (!drag?.moved) return
    const position = { x: next.clientX || drag.lastX, y: next.clientY || drag.lastY }
    if (isInsideCanvas(position.x, position.y)) void addModuleItemAt(drag.item, position)
    else status.value = 'Node drag cancelled'
  }

  removeNodePointerListeners = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', up)
    removeNodePointerListeners = () => {}
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', up)
}

function isInsideCanvas(clientX: number, clientY: number) {
  const rect = canvas.value?.getBoundingClientRect()
  return Boolean(rect && clientX >= rect.left && clientX <= rect.right && clientY >= rect.top && clientY <= rect.bottom)
}

function dropNode(event: DragEvent) {
  const variableId = event.dataTransfer?.getData('application/x-origin-variable')
  const variable = variables.value.find(item => item.id === variableId)
  if (!variable) return
  const access = event.dataTransfer?.getData('application/x-origin-variable-access') === 'set' ? 'set' : 'get'
  void createVariableNode(variable, access, { x: event.clientX, y: event.clientY })
}

function allowNodeDrop(event: DragEvent) {
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
}

function openContextMenu(event: MouseEvent) {
  if (event.ctrlKey) return
  if ((event.target as HTMLElement).closest('.blueprint-node, input, .node-group')) return
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  contextMenu.value = { visible: true, x: event.clientX - rect.left, y: event.clientY - rect.top, clientX: event.clientX, clientY: event.clientY, search: '' }
}
function createFromContext(typeId: string) { void addNodeAt(typeId, { x: contextMenu.value.clientX, y: contextMenu.value.clientY }); contextMenu.value.visible = false }

function openModuleNodeMenu(event: MouseEvent, node: NodeDefinition) {
  moduleNodeMenu.value = { visible: true, x: event.clientX, y: event.clientY, node }
}

function selectFunctionLibraryItem(item: ModuleLibraryItem) {
  if (!item.functionPlaceholder) return
  status.value = item.functionSource === 'workspace' ? `${menuText.value.module.workspaceFunctionLibrary}: ${item.title}` : `${menuText.value.module.currentBlueprintFunctions}: ${item.title}`
}

function closeModuleNodeMenu() {
  moduleNodeMenu.value.visible = false
}

async function findModuleNodeReferences(node = moduleNodeMenu.value.node) {
  closeModuleNodeMenu()
  if (!node) return
  if (!workspaceRoot.value) {
    status.value = '请先选择工程目录'
    return
  }
  nodeReferenceSearch.value = { visible: true, loading: true, nodeTitle: node.title, typeId: node.id, results: [] }
  try {
    const results = await platform.findNodeReferences(workspaceRoot.value, node.id)
    nodeReferenceSearch.value = { visible: true, loading: false, nodeTitle: node.title, typeId: node.id, results }
    status.value = `找到 ${results.length} 个引用蓝图`
  } catch (error) {
    nodeReferenceSearch.value.loading = false
    status.value = error instanceof Error ? error.message : String(error)
  }
}

async function openNodeReference(result: NodeReferenceResult) {
  await openGraph(result.path, nodeReferenceSearch.value.typeId)
}

function beginRightSidebarResize(event: PointerEvent) {
  if (event.button !== 0) return
  event.preventDefault()
  const startX = event.clientX
  const startWidth = rightSidebarWidth.value
  const move = (next: PointerEvent) => {
    rightSidebarWidth.value = Math.min(460, Math.max(160, Math.round(startWidth + startX - next.clientX)))
  }
  const up = () => {
    localStorage.setItem('origin-blueprint-right-sidebar-width', String(rightSidebarWidth.value))
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', up)
}

function beginVariablePanelHeightResize(event: PointerEvent) {
  if (event.button !== 0) return
  event.preventDefault()
  const startY = event.clientY
  const startHeight = variablePanelHeight.value
  const move = (next: PointerEvent) => {
    variablePanelHeight.value = Math.min(520, Math.max(130, Math.round(startHeight + next.clientY - startY)))
  }
  const up = () => {
    localStorage.setItem('origin-blueprint-variable-panel-height', String(variablePanelHeight.value))
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', up)
}

async function openFileContextGraph() {
  const path = fileContextMenu.value.path
  fileContextMenu.value.visible = false
  if (path && !fileContextMenu.value.isDir) await openGraph(path)
}

async function createBlueprintInFileContext() {
  const directory = fileContextMenu.value.path
  fileContextMenu.value.visible = false
  if (!directory || !fileContextMenu.value.isDir) return
  await createBlueprintAtDirectory(directory)
}

async function createFunctionInFileContext() {
  const directory = fileContextMenu.value.path
  fileContextMenu.value.visible = false
  if (!directory || !fileContextMenu.value.isDir) return
  await createFunctionAtDirectory(directory)
}

async function revealFileContextInFolder() {
  const path = fileContextMenu.value.path
  fileContextMenu.value.visible = false
  if (path) await revealFileInFolder(path)
}

async function revealFileInFolder(path: string) {
  try {
    await platform.revealInFolder(path)
    status.value = '已在文件夹中定位文件'
  } catch (error) {
    status.value = error instanceof Error ? error.message : String(error)
  }
}

function toggleReferencePanel() {
  referencePanelCollapsed.value = !referencePanelCollapsed.value
}

function toggleTestPanel() {
  testPanelCollapsed.value = !testPanelCollapsed.value
}

function beginTestPanelResize(event: PointerEvent) {
  if (event.button !== 0 || testPanelCollapsed.value) return
  event.preventDefault()
  const startY = event.clientY
  const startHeight = testPanelHeight.value
  const move = (next: PointerEvent) => {
    testPanelHeight.value = Math.min(360, Math.max(96, Math.round(startHeight + startY - next.clientY)))
  }
  const up = () => {
    localStorage.setItem('origin-blueprint-test-panel-height', String(testPanelHeight.value))
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', up)
}

function beginReferencePanelResize(event: PointerEvent) {
  if (event.button !== 0 || referencePanelCollapsed.value) return
  event.preventDefault()
  const startY = event.clientY
  const startHeight = referencePanelHeight.value
  const move = (next: PointerEvent) => {
    referencePanelHeight.value = Math.min(360, Math.max(96, Math.round(startHeight + startY - next.clientY)))
  }
  const up = () => {
    localStorage.setItem('origin-blueprint-reference-panel-height', String(referencePanelHeight.value))
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', up)
}

function isModuleCategoryExpanded(category: string) {
  return moduleSearchActive.value || expandedModuleCategories.value.has(category)
}

function toggleModuleCategory(category: string) {
  const next = new Set(expandedModuleCategories.value)
  if (next.has(category)) next.delete(category); else next.add(category)
  expandedModuleCategories.value = next
}
</script>

<template>
  <main class="application-shell" :class="{ 'tools-hidden': !showTools, 'right-hidden': !showRight }" @pointerdown="closeModuleNodeMenu">
    <header class="menu-bar">
      <div class="menu-items">
        <div class="menu-root"><button @click.stop="toggleMenu('file')">{{ menuText.menu.file.title }}</button><div v-if="activeMenu === 'file'" class="dropdown-menu">
          <button @click="run(newGraph)">{{ menuText.menu.file.newGraph }} <kbd>Ctrl+N</kbd></button><button @click="run(() => platform.newWindow())">{{ menuText.menu.file.newWindow }} <kbd>Ctrl+Shift+N</kbd></button><div class="menu-separator"></div><button @click="run(() => openGraph())">{{ menuText.menu.file.open }} <kbd>Ctrl+O</kbd></button>
          <div v-if="recentFiles.length" class="menu-subtitle">{{ menuText.menu.file.recent }}</div><button v-for="file in recentFiles" :key="file" class="recent-item" @click="run(() => openGraph(file))">{{ file.split(/[\\/]/).pop() }}</button>
          <button :disabled="!recentFiles.length" @click="run(clearRecentFiles)">{{ menuText.menu.file.clearRecent }}</button><div class="menu-separator"></div><button @click="run(chooseWorkspace)">{{ menuText.menu.file.setWorkspace }}</button><div class="menu-separator"></div><button @click="run(() => saveGraph(false))">{{ menuText.menu.file.save }} <kbd>Ctrl+S</kbd></button><button @click="run(() => saveGraph(true))">{{ menuText.menu.file.saveAs }} <kbd>Ctrl+Shift+S</kbd></button><button @click="run(saveAll)">{{ menuText.menu.file.saveAll }} <kbd>Ctrl+Alt+S</kbd></button><div class="menu-separator"></div><button @click="run(quitApplication)">{{ menuText.menu.file.quit }} <kbd>Alt+F4</kbd></button>
        </div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('edit')">{{ menuText.menu.edit.title }}</button><div v-if="activeMenu === 'edit'" class="dropdown-menu">
          <button @click="run(() => editor?.undo())">{{ menuText.menu.edit.undo }} <kbd>Ctrl+Z</kbd></button><button @click="run(() => editor?.redo())">{{ menuText.menu.edit.redo }} <kbd>Ctrl+Y</kbd></button><div class="menu-separator"></div>
          <button @click="run(() => editor?.cut())">{{ menuText.menu.edit.cut }} <kbd>Ctrl+X</kbd></button><button @click="run(() => editor?.copy())">{{ menuText.menu.edit.copy }} <kbd>Ctrl+C</kbd></button><button @click="run(() => editor?.paste())">{{ menuText.menu.edit.paste }} <kbd>Ctrl+V</kbd></button><button @click="run(() => editor?.deleteSelected())">{{ menuText.menu.edit.delete }} <kbd>Delete</kbd></button>
          <button @click="run(() => editor?.groupSelected())">{{ menuText.menu.edit.group }} <kbd>Ctrl+G</kbd></button><button @click="run(() => editor?.ungroupSelected())">{{ menuText.menu.edit.ungroup }} <kbd>Alt+G</kbd></button><div class="menu-separator"></div>
          <button @click="run(() => editor?.selectAll())">{{ menuText.menu.edit.selectAll }} <kbd>Ctrl+A</kbd></button><button @click="run(() => editor?.deselectAll())">{{ menuText.menu.edit.deselectAll }} <kbd>Ctrl+D</kbd></button>
        </div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('align')">{{ menuText.menu.align.title }}</button><div v-if="activeMenu === 'align'" class="dropdown-menu">
          <button @click="run(() => editor?.align('vertical-center'))">{{ menuText.menu.align.verticalCenter }} <kbd>V</kbd></button><button @click="run(() => editor?.align('horizontal-center'))">{{ menuText.menu.align.horizontalCenter }} <kbd>H</kbd></button>
          <button @click="run(() => editor?.align('vertical-distribute'))">{{ menuText.menu.align.verticalDistribute }} <kbd>Shift+V</kbd></button><button @click="run(() => editor?.align('horizontal-distribute'))">{{ menuText.menu.align.horizontalDistribute }} <kbd>Shift+H</kbd></button>
          <button @click="run(() => editor?.align('left'))">{{ menuText.menu.align.left }} <kbd>Shift+L</kbd></button><button @click="run(() => editor?.align('right'))">{{ menuText.menu.align.right }} <kbd>Shift+R</kbd></button><button @click="run(() => editor?.align('top'))">{{ menuText.menu.align.top }} <kbd>Shift+T</kbd></button><button @click="run(() => editor?.align('bottom'))">{{ menuText.menu.align.bottom }} <kbd>Shift+B</kbd></button><button @click="run(() => editor?.align('straighten'))">{{ menuText.menu.align.straighten }} <kbd>Q</kbd></button>
        </div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('view')">{{ menuText.menu.view.title }}</button><div v-if="activeMenu === 'view'" class="dropdown-menu"><button @click="showLogger = !showLogger">{{ menuText.menu.view.showTestResults }} <kbd>Alt+Shift+B</kbd></button><button @click="showTools = !showTools">{{ menuText.menu.view.showLeftSidebar }} <kbd>Alt+Shift+L</kbd></button><button @click="showRight = !showRight">{{ menuText.menu.view.showModuleLibrary }} <kbd>Alt+Shift+R</kbd></button><div class="menu-separator"></div><div class="menu-subtitle">{{ menuText.menu.view.language }}</div><button @click="setLocale('zh-CN')">{{ currentLocale === 'zh-CN' ? '✓ ' : '' }}{{ menuText.menu.view.chinese }}</button><button @click="setLocale('en-US')">{{ currentLocale === 'en-US' ? '✓ ' : '' }}{{ menuText.menu.view.english }}</button></div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('render')">{{ menuText.menu.render.title }}</button><div v-if="activeMenu === 'render'" class="dropdown-menu"><button @click="run(() => exportImage(true))">{{ menuText.menu.render.selectedNodes }} <kbd>Ctrl+Alt+R</kbd></button><button @click="run(() => exportImage(false))">{{ menuText.menu.render.graph }} <kbd>Ctrl+Shift+R</kbd></button></div></div>
        <button @click="run(testGraph)">{{ menuText.menu.test }}</button><button @click="showAbout = true">{{ menuText.menu.help }}</button>
      </div>
    </header>

    <section class="workspace" :style="workspaceStyle">
      <aside class="sidebar sidebar-file-browser">
        <div class="panel workspace-panel">
          <div class="panel-title"><span class="chevron">⌄</span> 文件浏览器<button class="panel-action" @click="chooseWorkspace">…</button></div>
          <div class="workspace-search"><input v-model="workspaceSearch" placeholder="搜索文件..." /></div>
          <div class="workspace-tree">
            <button v-for="row in visibleWorkspaceNodes" :key="row.node.path" class="workspace-entry" :class="{ selected: selectedWorkspacePath === row.node.path, folder: row.node.isDir }" :style="{ paddingLeft: workspaceIndent(row.depth) }" :title="row.node.path" @click="toggleWorkspaceNode(row.node)" @contextmenu.stop.prevent="openFileContextMenu($event, row.node)" @dblclick="!row.node.isDir && workspaceOpen(row.node)">
              <span class="workspace-arrow">{{ row.node.loading ? '…' : row.node.isDir ? (workspaceSearch || expandedWorkspacePaths.has(row.node.path) ? '⌄' : '›') : '' }}</span>
              <span v-if="isFunctionResource(row.node)" class="workspace-icon function" :class="{ folder: row.node.isDir }"></span>
              <span v-else class="workspace-icon" :class="{ folder: row.node.isDir }"></span>
              <span class="workspace-name">{{ row.node.name }}</span>
            </button>
            <div v-if="!visibleWorkspaceNodes.length" class="empty-panel">{{ workspaceSearch ? '没有匹配的文件' : '没有可显示的文件' }}</div>
          </div>
        </div>
      </aside>
      <div v-show="showTools" class="sidebar-splitter" @pointerdown="beginLeftSidebarResize"></div>
      <aside v-show="showTools" class="sidebar sidebar-left">
        <div class="panel grow variable-panel" :style="variablePanelStyle"><div class="panel-title"><span class="chevron">⌄</span> 变量 <span class="panel-title-spacer"></span><button class="panel-action" title="添加变量组" @click="addVariableGroup">▣＋</button><button class="panel-action" title="添加变量" @click="addVariable()">＋</button></div>
          <div v-if="!variables.length" class="empty-panel">尚未创建变量</div>
          <section v-for="entry in groupedVariables" :key="entry.group.id" class="variable-group">
            <div class="variable-group-header">
              <button class="group-toggle" @click="toggleVariableGroup(entry.group)">{{ entry.group.collapsed ? '›' : '⌄' }}</button>
              <span class="variable-group-name" @dblclick="renameVariableGroup(entry.group)">{{ entry.group.name }}</span><small>{{ entry.variables.length }}</small>
              <button title="在此组添加变量" @click="addVariable(entry.group.id)">＋</button><button v-if="entry.group.id !== 'default'" title="重命名组" @click="renameVariableGroup(entry.group)">✎</button><button v-if="entry.group.id !== 'default'" title="删除组" @click="removeVariableGroup(entry.group)">×</button>
            </div>
            <div v-if="!entry.group.collapsed" class="variable-group-list">
              <div v-for="variable in entry.variables" :key="variable.id" class="variable-row" :class="{ selected: selectedVariableId === variable.id }" draggable="true" @click="selectVariable(variable)" @dragstart="startVariableDrag($event, variable)">
                <div class="variable-heading"><span class="variable-type-dot" :class="`type-${variable.type}`"></span><span class="variable-name">{{ variable.name }}</span><span class="variable-kind">{{ variable.type }}</span><button title="Get" @click.stop="createVariableNode(variable, 'get')">G</button><button title="Set" @click.stop="createVariableNode(variable, 'set')">S</button><button title="Delete" @click.stop="removeVariable(variable)">×</button></div>
              </div>
              <button v-if="!entry.variables.length" class="empty-variable-group" @click="addVariable(entry.group.id)">＋ 添加变量</button>
            </div>
          </section>
          <button class="add-variable" @click="addVariable()">＋ 添加变量</button>
        </div>
        <div class="panel-height-splitter" @pointerdown="beginVariablePanelHeightResize"></div>
        <div class="panel grow detail-panel sidebar-detail-panel">
          <div class="panel-title"><span class="chevron">⌄</span> 详情</div>
          <div v-if="isFunctionBlueprintTab && !selectedNode && !selectedVariable" class="node-detail function-signature-editor">
            <label>Title<input v-model="functionTitle" placeholder="函数显示名" @change="syncFunctionTitleToGraph" /></label>
            <div class="detail-section-title">函数签名</div>
            <div class="function-terminal-actions"><button @click="addFunctionEntryNodeToGraph">＋ 入口节点</button><button @click="addFunctionReturnNodeToGraph">＋ 出口节点</button></div>
            <section class="signature-port-section">
              <header><span>输入参数</span><button @click="addFunctionSignaturePort('inputs')">＋</button></header>
              <div v-for="port in functionSignature.inputs" :key="port.id" class="signature-port-row">
                <input v-model="port.name" placeholder="参数名" @change="syncFunctionSignatureToGraph" />
                <select v-model="port.type" @change="syncFunctionSignatureToGraph"><option v-for="option in functionSignatureTypeOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select>
                <button title="删除参数" @click="removeFunctionSignaturePort('inputs', port)">×</button>
              </div>
              <button v-if="!functionSignature.inputs.length" class="empty-signature-port" @click="addFunctionSignaturePort('inputs')">＋ 添加输入参数</button>
            </section>
            <section class="signature-port-section">
              <header><span>输出参数</span><button @click="addFunctionSignaturePort('outputs')">＋</button></header>
              <div v-for="port in functionSignature.outputs" :key="port.id" class="signature-port-row">
                <input v-model="port.name" placeholder="参数名" @change="syncFunctionSignatureToGraph" />
                <select v-model="port.type" @change="syncFunctionSignatureToGraph"><option v-for="option in functionSignatureTypeOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select>
                <button title="删除参数" @click="removeFunctionSignaturePort('outputs', port)">×</button>
              </div>
              <button v-if="!functionSignature.outputs.length" class="empty-signature-port" @click="addFunctionSignaturePort('outputs')">＋ 添加输出参数</button>
            </section>
          </div>
          <div v-else-if="selectedVariable" class="node-detail variable-detail"><div class="detail-section-title">变量属性</div><label>Variable ID<input :value="selectedVariable.id" disabled /></label><label>名称<input v-model="selectedVariable.name" /></label><label>类型<select v-model="selectedVariable.type" @change="changeVariableType(selectedVariable)"><option value="boolean">Boolean</option><option value="integer">Integer</option><option value="float">Float</option><option value="string">String</option><option value="array">Array</option><option value="file">File</option><option value="table">Table</option><option value="dictionary">Dictionary</option></select></label><label>分组<select v-model="selectedVariable.groupId"><option v-for="group in variableGroups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label><label>说明<textarea v-model="selectedVariable.description" rows="4" placeholder="变量用途和约束"></textarea></label><label>默认值<input v-if="selectedVariable.type === 'boolean'" v-model="selectedVariable.defaultValue" type="checkbox" /><input v-else-if="selectedVariable.type === 'string' || selectedVariable.type === 'file'" v-model="selectedVariable.defaultValue" type="text" /><input v-else-if="selectedVariable.type === 'array'" :value="Array.isArray(selectedVariable.defaultValue) ? selectedVariable.defaultValue.join(', ') : ''" placeholder="1, 2, text" @change="setVariableArrayDefault(selectedVariable, $event)" /><textarea v-else-if="selectedVariable.type === 'table' || selectedVariable.type === 'dictionary'" :value="JSON.stringify(selectedVariable.defaultValue, null, 2)" rows="6" @change="setVariableStructuredDefault(selectedVariable, $event)"></textarea><input v-else v-model.number="selectedVariable.defaultValue" type="number" /></label><button class="apply-properties" @click="updateVariable(selectedVariable)">应用变量属性</button><button class="delete-properties" @click="removeVariable(selectedVariable)">删除变量</button></div>
          <div v-else-if="selectedNode" class="node-detail"><label>Node ID<input :value="selectedNode.id" disabled /></label><label>Type<input :value="selectedNode.typeId" disabled /></label><label>Title<input v-model="selectedNode.label" :disabled="Boolean(selectedNode.variableId)" /></label><label v-if="selectedNode.description">说明<textarea :value="selectedNode.description" rows="4" readonly></textarea></label><button class="apply-properties" @click="applyNodeProperties">Apply</button></div>
          <div v-else class="empty-detail">选择节点或变量以查看属性</div>
        </div>
      </aside>
      <div v-show="showTools" class="left-tools-splitter" @pointerdown="beginLeftToolsResize"></div>

      <section class="editor-column">
         <div class="tab-strip-wrap">
           <button class="tab-scroll-arrow left" @click="scrollTabStrip(-1)">◀</button>
           <div ref="tabStrip" class="tab-strip" @wheel.prevent="(e: WheelEvent) => { const s = e.currentTarget as HTMLElement; s.scrollLeft += e.deltaY; }">
             <div v-for="(tab, idx) in tabs" :key="tab.id" class="graph-tab" :class="{ active: tab.id === activeTabId, 'drag-over': tabDragOverIndex === idx }" draggable="true" @click="switchTab(tab.id)" @dragstart="onTabDragStart($event, idx)" @dragover="onTabDragOver($event, idx)" @dragleave="onTabDragLeave" @drop="onTabDrop($event, idx)" @dragend="onTabDragEnd"><span class="tab-mark"></span>{{ tab.title }}<span v-if="tab.dirty" class="dirty-mark">●</span><button class="tab-close" @click="closeTab(tab.id, $event)">×</button></div>
             <button class="new-tab" @click="newGraph">＋</button>
           </div>
           <button class="tab-scroll-arrow right" @click="scrollTabStrip(1)">▶</button>
         </div>
        <div class="canvas-wrap" @contextmenu.prevent @dragenter="allowNodeDrop" @dragover="allowNodeDrop" @drop.prevent="dropNode"><div ref="canvas" class="rete-canvas"></div><div class="canvas-toolbar"><button title="Select">⌖</button><button title="Reset view" @click="editor?.resetView()">⌂</button></div><div class="canvas-hint">Right drag: pan&nbsp;&nbsp; Middle drag: pan&nbsp;&nbsp; Ctrl: multi-select&nbsp;&nbsp; Ctrl + right drag: cut connections&nbsp;&nbsp; Connection: click + Delete</div></div>
        <div v-show="showLogger" class="logger-panel bottom-panel" :class="{ collapsed: testPanelCollapsed }" :style="testPanelStyle">
          <div class="bottom-panel-resizer" @pointerdown="beginTestPanelResize"></div>
          <div class="bottom-panel-title">
            <strong class="bottom-panel-target">Test Results</strong>
            <small>{{ validationIssues.length ? `${validationIssues.length} 条问题` : '无问题' }}</small>
            <button class="bottom-panel-action" title="重新检查蓝图" @click="testGraph">Test</button>
            <button class="bottom-panel-tool-button" :title="testPanelCollapsed ? '展开 Test Results' : '收起 Test Results'" @click="toggleTestPanel">{{ testPanelCollapsed ? '▴' : '▾' }}</button>
            <button class="bottom-panel-tool-button close" title="关闭 Test Results" @click="showLogger = false">×</button>
          </div>
          <div v-show="!testPanelCollapsed" class="logger-results">
            <div v-if="!validationIssues.length" class="logger-line">没有发现蓝图问题。</div>
            <button v-for="(issue, index) in validationIssues" :key="validationIssueKey(issue, index)" class="logger-issue" :class="[issue.severity, { selected: selectedValidationIssueKey === validationIssueKey(issue, index) }]" @click="queueSelectIssue(issue, index)" @dblclick.stop.prevent="highlightIssue(issue, index)"><strong>{{ issue.severity === 'error' ? '错误' : '警告' }}</strong><span>{{ issue.message }}</span><small>{{ issue.code }}</small></button>
          </div>
        </div>
        <div v-if="nodeReferenceSearch.visible" class="reference-panel bottom-panel" :class="{ collapsed: referencePanelCollapsed }" :style="referencePanelStyle">
          <div class="bottom-panel-resizer" @pointerdown="beginReferencePanelResize"></div>
          <div class="bottom-panel-title">
            <strong class="bottom-panel-target" :title="nodeReferenceSearch.nodeTitle">查找目标：{{ nodeReferenceSearch.nodeTitle }}</strong>
            <small>{{ nodeReferenceSearch.loading ? '扫描中...' : `${nodeReferenceSearch.results.length} 个蓝图` }}</small>
            <button class="bottom-panel-tool-button" :title="referencePanelCollapsed ? '展开引用结果' : '收起引用结果'" @click="toggleReferencePanel">{{ referencePanelCollapsed ? '▴' : '▾' }}</button>
            <button class="bottom-panel-tool-button close" title="关闭引用结果" @click="nodeReferenceSearch.visible = false">×</button>
          </div>
          <div v-show="!referencePanelCollapsed" class="reference-results">
            <div v-if="nodeReferenceSearch.loading" class="reference-empty">正在扫描当前工程下的 .vgf / .obp 文件...</div>
            <div v-else-if="!nodeReferenceSearch.results.length" class="reference-empty">没有找到引用该结点的蓝图</div>
            <template v-else>
              <button v-for="result in nodeReferenceSearch.results" :key="result.path" class="reference-row" :title="result.path" @contextmenu.stop.prevent="openFileContextMenu($event, result)" @dblclick="openNodeReference(result)">
                <span>{{ result.name }}</span>
                <small>{{ result.count }} 次</small>
                <code>{{ result.path }}</code>
              </button>
            </template>
          </div>
        </div>
        <footer class="status-bar"><span>{{ status }}</span><span>Nodes {{ metrics.nodes }} · Connections {{ metrics.connections }}</span><button @click="editor?.resetView()">{{ zoomLabel }}</button></footer>
      </section>

      <div v-show="showRight" class="right-sidebar-splitter" @pointerdown="beginRightSidebarResize"></div>
      <aside v-show="showRight" class="sidebar sidebar-right">
        <div class="panel module-panel">
          <div class="panel-title"><span class="chevron">⌄</span> 模块库</div>
          <div class="search-box">⌕ <input v-model="moduleSearch" placeholder="搜索模块..." /></div>
          <div class="module-list">
            <section v-for="[category, items] in categories" :key="category" class="module-category-section" :class="{ open: isModuleCategoryExpanded(category) }">
              <button class="module-category" :aria-expanded="isModuleCategoryExpanded(category)" @click="toggleModuleCategory(category)">
                <span class="module-arrow">{{ isModuleCategoryExpanded(category) ? '⌄' : '›' }}</span>
                <span class="module-category-name">{{ category }}</span>
                <small>{{ items.length }}</small>
              </button>
              <div v-if="isModuleCategoryExpanded(category)" class="module-items">
                <button v-for="item in items" :key="item.id" class="module-item" :class="{ 'function-placeholder': item.functionPlaceholder }" :title="item.path || item.title" @click="selectFunctionLibraryItem(item)" @pointerdown.stop="beginModuleItemPointerDrag($event, item)" @contextmenu.stop.prevent="!item.functionPlaceholder && openModuleNodeMenu($event, item)" @dblclick="addModuleItemAt(item)"><span>{{ item.title }}</span><small v-if="item.functionPlaceholder">{{ item.functionSource === 'workspace' ? menuText.module.workspaceFunctionLibrary : menuText.module.currentBlueprintFunctions }}</small></button>
              </div>
            </section>
            <div v-if="!functionLibraryItems.length" class="function-library-empty">{{ menuText.module.noFunctionLibrary }}</div>
            <div v-if="!categories.length" class="empty-panel">{{ status || '没有匹配的模块' }}</div>
          </div>
        </div>
      </aside>
    </section>
    <div v-if="moduleNodeMenu.visible" class="module-node-menu" :style="{ left: `${moduleNodeMenu.x}px`, top: `${moduleNodeMenu.y}px` }" @pointerdown.stop>
      <div class="module-node-menu-title">{{ moduleNodeMenu.node?.title }}</div>
      <button @click="findModuleNodeReferences()">查找所有引用</button>
    </div>
    <div v-if="fileContextMenu.visible" class="file-context-menu" :style="{ left: `${fileContextMenu.x}px`, top: `${fileContextMenu.y}px` }" @pointerdown.stop>
      <button v-if="!fileContextMenu.isDir" @click="openFileContextGraph">{{ fileContextMenu.isFunction ? '打开函数' : '打开蓝图' }}</button>
      <button v-if="fileContextMenu.isDir" @click="createBlueprintInFileContext">新建蓝图</button>
      <button v-if="fileContextMenu.isDir" @click="createFunctionInFileContext">新建函数</button>
      <button @click="revealFileContextInFolder">在资源管理器中定位</button>
    </div>
    <div v-if="unsavedCloseDialog.visible" class="unsaved-close-backdrop">
      <section class="unsaved-close-dialog">
        <header>有未保存的蓝图</header>
        <p>{{ unsavedCloseDialog.names.join('，') }}</p>
        <footer>
          <button class="primary" @click="resolveUnsavedCloseAction('save')">保存</button>
          <button @click="resolveUnsavedCloseAction('discard')">不保存</button>
          <button @click="resolveUnsavedCloseAction('cancel')">取消</button>
        </footer>
      </section>
    </div>
    <div v-if="showAbout" class="about-backdrop" @click.self="showAbout = false"><section class="about-dialog"><header><strong>Origin Blueprint</strong><button @click="showAbout = false">×</button></header><p>Cross-platform blueprint editor built with Go, Wails, Vue 3 and Rete.js.</p><dl><dt>Canvas</dt><dd>Right drag or middle drag pans, mouse wheel zooms, left drag selects.</dd><dt>Connections</dt><dd>Click a connection then Delete, or Ctrl + right-drag to cut lines.</dd><dt>Editing</dt><dd>Ctrl+C/X/V, Ctrl+Z/Y, Ctrl+G and alignment shortcuts match OriginNodeEditor.</dd><dt>Test</dt><dd>F5 checks structure, unreachable flow nodes, missing entries, and possible execution loops.</dd></dl><footer><button @click="showAbout = false">Close</button></footer></section></div>
  </main>
</template>
