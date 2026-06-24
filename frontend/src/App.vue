<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { toPng } from 'html-to-image'
import { createBlueprintEditor, type BlueprintEditorHandle, type EditorMetrics, type GraphDocument, type GraphVariable, type GraphVariableGroup, type SelectedNodeInfo, type ValidationIssue, type VariableType } from './editor/createEditor'
import { getNodeDefinitions, registerNodeSchemas, type NodeDefinition } from './editor/nodeRegistry'
import { platform, type NodeReferenceResult, type WorkspaceEntry } from './platform'

interface GraphTab { id: string; title: string; path: string; dirty: boolean; document: GraphDocument | null }
interface WorkspaceTreeNode extends WorkspaceEntry { children: WorkspaceTreeNode[]; loaded: boolean; loading: boolean }
interface VisibleWorkspaceNode { node: WorkspaceTreeNode; depth: number }
type UnsavedCloseAction = 'save' | 'discard' | 'cancel'
interface ModuleNodeMenuState { visible: boolean; x: number; y: number; node: NodeDefinition | null }
interface NodeReferenceSearchState { visible: boolean; loading: boolean; nodeTitle: string; typeId: string; results: NodeReferenceResult[] }
interface FileContextMenuState { visible: boolean; x: number; y: number; path: string }

const canvas = ref<HTMLElement | null>(null)
const zoomLabel = ref('100%')
const status = ref('Ready')
const metrics = ref<EditorMetrics>({ nodes: 0, connections: 0 })
const activeMenu = ref<string | null>(null)
const contextMenu = ref({ visible: false, x: 0, y: 0, clientX: 0, clientY: 0, search: '' })
const moduleNodeMenu = ref<ModuleNodeMenuState>({ visible: false, x: 0, y: 0, node: null })
const nodeReferenceSearch = ref<NodeReferenceSearchState>({ visible: false, loading: false, nodeTitle: '', typeId: '', results: [] })
const fileContextMenu = ref<FileContextMenuState>({ visible: false, x: 0, y: 0, path: '' })
const referencePanelHeight = ref(savedReferencePanelHeight())
const referencePanelCollapsed = ref(false)
const tabs = ref<GraphTab[]>([{ id: crypto.randomUUID(), title: 'Untitled-1 Graph', path: '', dirty: false, document: null }])
const activeTabId = ref(tabs.value[0].id)
const recentFiles = ref<string[]>([])
const workspaceRoot = ref('')
const workspaceTree = ref<WorkspaceTreeNode[]>([])
const workspaceSearch = ref('')
const expandedWorkspacePaths = ref<Set<string>>(new Set())
const selectedWorkspacePath = ref('')
const fileBrowserWidth = ref(savedPanelWidth('origin-blueprint-file-browser-width', 210))
const leftToolsWidth = ref(savedPanelWidth('origin-blueprint-left-tools-width', 210))
const rightSidebarWidth = ref(savedPanelWidth('origin-blueprint-right-sidebar-width', 230, 160, 460))
const functionPanelHeight = ref(savedPanelSize('origin-blueprint-function-panel-height', 112, 70, 260))
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
const selectedVariableId = ref<string | null>(null)
const selectedNode = ref<SelectedNodeInfo | null>(null)
const validationIssues = ref<ValidationIssue[]>([])
const unsavedCloseDialog = ref<{ visible: boolean; names: string[]; resolve?: (action: UnsavedCloseAction) => void }>({ visible: false, names: [] })
let untitledCount = 1
let editor: BlueprintEditorHandle | null = null
let unsubscribeCloseRequest = () => {}
let closingApplication = false
let nodePointerDrag: { typeId: string; startX: number; startY: number; lastX: number; lastY: number; moved: boolean } | null = null
let removeNodePointerListeners = () => {}
let workspaceLoadToken = 0

const activeTab = computed(() => tabs.value.find(tab => tab.id === activeTabId.value)!)
const selectedVariable = computed(() => variables.value.find(variable => variable.id === selectedVariableId.value) ?? null)
const groupedVariables = computed(() => variableGroups.value.map(group => ({
  group,
  variables: variables.value.filter(variable => variable.groupId === group.id)
})))
const categories = computed(() => {
  const result = new Map<string, NodeDefinition[]>()
  const search = moduleSearch.value.trim().toLowerCase()
  for (const definition of nodeLibrary.value.filter(item => !search || `${item.title} ${item.category} ${item.id}`.toLowerCase().includes(search))) {
    const items = result.get(definition.category) ?? []; items.push(definition); result.set(definition.category, items)
  }
  return Array.from(result.entries())
})
const filteredDefinitions = computed(() => {
  const search = contextMenu.value.search.trim().toLowerCase()
  return search ? nodeLibrary.value.filter(item => `${item.title} ${item.category}`.toLowerCase().includes(search)) : nodeLibrary.value
})
const moduleSearchActive = computed(() => Boolean(moduleSearch.value.trim()))
const workspaceStyle = computed(() => ({
  '--file-browser-width': `${fileBrowserWidth.value}px`,
  '--left-tools-width': `${leftToolsWidth.value}px`,
  '--right-sidebar-width': `${rightSidebarWidth.value}px`
}))
const referencePanelStyle = computed(() => ({ height: `${referencePanelCollapsed.value ? 34 : referencePanelHeight.value}px` }))
const functionPanelStyle = computed(() => ({ flex: `0 0 ${functionPanelHeight.value}px` }))
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
function persistActive() { if (editor && activeTab.value) activeTab.value.document = editor.getDocument(activeTab.value.title, variables.value, variableGroups.value) }

async function newGraph() {
  persistActive(); untitledCount++
  const tab: GraphTab = { id: crypto.randomUUID(), title: `Untitled-${untitledCount} Graph`, path: '', dirty: false, document: null }
  tabs.value.push(tab); activeTabId.value = tab.id; selectedVariableId.value = null; await editor?.newDocument()
}

async function switchTab(id: string) {
  if (id === activeTabId.value) return
  persistActive(); activeTabId.value = id; selectedVariableId.value = null
  const tab = activeTab.value
  if (tab.document) await editor?.loadDocument(tab.document); else await editor?.newDocument()
}

async function closeTab(id: string, event: MouseEvent) {
  event.stopPropagation()
  const tab = tabs.value.find(item => item.id === id)
  if (!tab || (tab.dirty && !window.confirm(`Close ${tab.title} without saving?`))) return
  const wasActive = id === activeTabId.value
  tabs.value = tabs.value.filter(item => item.id !== id)
  if (!tabs.value.length) { await newGraph(); return }
  if (wasActive) { activeTabId.value = tabs.value[0].id; selectedVariableId.value = null; await editor?.loadDocument(tabs.value[0].document ?? blankDocument(tabs.value[0].title)) }
}

function blankDocument(name: string): GraphDocument {
  return { schemaVersion: 1, graphName: name, nodes: [], connections: [], groups: [], variables: [], variableGroups: [{ id: 'default', name: 'Default' }], view: { x: 0, y: 0, zoom: 1 } }
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

async function selectVariable(variable: GraphVariable) {
  await editor?.deselectAll()
  selectedVariableId.value = variable.id
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
    view: value.view ?? { x: 0, y: 0, zoom: 1 },
    legacy: value.legacy
  }
}

function isLegacyGraphPath(path: string) {
  return /\.vgf$/i.test(path)
}

async function testGraph() {
  if (!editor) return
  const document = editor.getDocument(activeTab.value.title, variables.value, variableGroups.value)
  validationIssues.value = await platform.validateGraph(JSON.stringify(document))
  showLogger.value = true
  const errors = validationIssues.value.filter(issue => issue.severity === 'error').length
  const warnings = validationIssues.value.filter(issue => issue.severity === 'warning').length
  status.value = validationIssues.value.length ? `Test found ${errors} error(s), ${warnings} warning(s)` : 'Blueprint test passed'
}

async function focusIssue(issue: ValidationIssue) { if (issue.nodeId) await editor?.focusNode(issue.nodeId) }

async function openGraph(path = '', highlightTypeId = '') {
  const file = await platform.openGraph(path)
  if (!file) return
  let parsed: any
  try { parsed = JSON.parse(file.content) } catch { status.value = 'Invalid graph file'; return }
  let document: GraphDocument
  if (parsed.schemaVersion === 1) document = normalizeDocument(parsed)
  else if (platform.isDesktop()) {
    try { document = normalizeDocument(JSON.parse(await platform.migrateLegacyGraph(file.content))) }
    catch (error) { status.value = error instanceof Error ? error.message : 'Legacy graph migration failed'; return }
  } else { status.value = 'Legacy graph migration requires the desktop runtime'; return }
  persistActive()
  const existing = tabs.value.find(tab => tab.path === file.path)
  if (existing) {
    await switchTab(existing.id)
    if (highlightTypeId) {
      const count = await editor?.highlightNodesByType(highlightTypeId) ?? 0
      status.value = count ? `已高亮 ${count} 个引用结点` : '该蓝图中未找到引用结点'
    }
    return
  }
  const title = file.path.split(/[\\/]/).pop() ?? document.graphName
  const tab: GraphTab = { id: crypto.randomUUID(), title, path: file.path, dirty: false, document }
  tabs.value.push(tab); activeTabId.value = tab.id; selectedVariableId.value = null; await editor?.loadDocument(document)
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
  const document = editor.getDocument(tab.title, variables.value, variableGroups.value)
  const shouldSaveLegacy = !saveAs && isLegacyGraphPath(tab.path)
  const content = shouldSaveLegacy ? await platform.exportLegacyGraph(JSON.stringify(document)) : JSON.stringify(document, null, 2)
  const path = await platform.saveGraph(saveAs ? '' : tab.path, content)
  if (!path) return
  tab.path = path; tab.title = path.split(/[\\/]/).pop() ?? tab.title; tab.document = document; tab.dirty = false
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

function openFileContextMenu(event: MouseEvent, path: string) {
  fileContextMenu.value = { visible: true, x: event.clientX, y: event.clientY, path }
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

function visibleCanvasInsertPosition() {
  const rect = canvas.value?.getBoundingClientRect()
  if (!rect) return undefined
  return { x: rect.left + rect.width * 0.42, y: rect.top + rect.height * 0.36 }
}

function beginNodePointerDrag(event: PointerEvent, typeId: string) {
  if (event.button !== 0) return
  removeNodePointerListeners()
  nodePointerDrag = { typeId, startX: event.clientX, startY: event.clientY, lastX: event.clientX, lastY: event.clientY, moved: false }
  status.value = `Dragging ${typeId}`

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
    if (isInsideCanvas(position.x, position.y)) void addNodeAt(drag.typeId, position)
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

function beginLeftPanelHeightResize(panel: 'function' | 'variable', event: PointerEvent) {
  if (event.button !== 0) return
  event.preventDefault()
  const startY = event.clientY
  const state = panel === 'function' ? functionPanelHeight : variablePanelHeight
  const storageKey = panel === 'function' ? 'origin-blueprint-function-panel-height' : 'origin-blueprint-variable-panel-height'
  const min = panel === 'function' ? 70 : 130
  const max = panel === 'function' ? 260 : 520
  const startHeight = state.value
  const move = (next: PointerEvent) => {
    state.value = Math.min(max, Math.max(min, Math.round(startHeight + next.clientY - startY)))
  }
  const up = () => {
    localStorage.setItem(storageKey, String(state.value))
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
  if (path) await openGraph(path)
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
        <div class="menu-root"><button @click.stop="toggleMenu('file')">File</button><div v-if="activeMenu === 'file'" class="dropdown-menu">
          <button @click="run(newGraph)">New Graph <kbd>Ctrl+N</kbd></button><button @click="run(() => platform.newWindow())">New Window <kbd>Ctrl+Shift+N</kbd></button><div class="menu-separator"></div><button @click="run(() => openGraph())">Open <kbd>Ctrl+O</kbd></button>
          <div v-if="recentFiles.length" class="menu-subtitle">Recent</div><button v-for="file in recentFiles" :key="file" class="recent-item" @click="run(() => openGraph(file))">{{ file.split(/[\\/]/).pop() }}</button>
          <button :disabled="!recentFiles.length" @click="run(clearRecentFiles)">Clear Recent Files</button><div class="menu-separator"></div><button @click="run(chooseWorkspace)">Set Workspace Path</button><div class="menu-separator"></div><button @click="run(() => saveGraph(false))">Save <kbd>Ctrl+S</kbd></button><button @click="run(() => saveGraph(true))">Save As <kbd>Ctrl+Shift+S</kbd></button><button @click="run(saveAll)">Save All <kbd>Ctrl+Alt+S</kbd></button><div class="menu-separator"></div><button @click="run(quitApplication)">Quit <kbd>Alt+F4</kbd></button>
        </div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('edit')">Edit</button><div v-if="activeMenu === 'edit'" class="dropdown-menu">
          <button @click="run(() => editor?.undo())">Undo <kbd>Ctrl+Z</kbd></button><button @click="run(() => editor?.redo())">Redo <kbd>Ctrl+Y</kbd></button><div class="menu-separator"></div>
          <button @click="run(() => editor?.cut())">Cut <kbd>Ctrl+X</kbd></button><button @click="run(() => editor?.copy())">Copy <kbd>Ctrl+C</kbd></button><button @click="run(() => editor?.paste())">Paste <kbd>Ctrl+V</kbd></button><button @click="run(() => editor?.deleteSelected())">Delete <kbd>Delete</kbd></button>
          <button @click="run(() => editor?.groupSelected())">Group Nodes <kbd>Ctrl+G</kbd></button><button @click="run(() => editor?.ungroupSelected())">UnGroup <kbd>Alt+G</kbd></button><div class="menu-separator"></div>
          <button @click="run(() => editor?.selectAll())">Select All <kbd>Ctrl+A</kbd></button><button @click="run(() => editor?.deselectAll())">Deselect All <kbd>Ctrl+D</kbd></button>
        </div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('align')">Alignment</button><div v-if="activeMenu === 'align'" class="dropdown-menu">
          <button @click="run(() => editor?.align('vertical-center'))">Align Vertical Center <kbd>V</kbd></button><button @click="run(() => editor?.align('horizontal-center'))">Align Horizontal Center <kbd>H</kbd></button>
          <button @click="run(() => editor?.align('vertical-distribute'))">Vertical Distribution <kbd>Shift+V</kbd></button><button @click="run(() => editor?.align('horizontal-distribute'))">Horizontal Distribution <kbd>Shift+H</kbd></button>
          <button @click="run(() => editor?.align('left'))">Align Left <kbd>Shift+L</kbd></button><button @click="run(() => editor?.align('right'))">Align Right <kbd>Shift+R</kbd></button><button @click="run(() => editor?.align('top'))">Align Top <kbd>Shift+T</kbd></button><button @click="run(() => editor?.align('bottom'))">Align Bottom <kbd>Shift+B</kbd></button><button @click="run(() => editor?.align('straighten'))">Straighten Edge <kbd>Q</kbd></button>
        </div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('view')">View</button><div v-if="activeMenu === 'view'" class="dropdown-menu"><button @click="showLogger = !showLogger">Show Logger <kbd>Alt+Shift+B</kbd></button><button @click="showTools = !showTools">Show Tool Sidebar <kbd>Alt+Shift+L</kbd></button><button @click="showRight = !showRight">Show Right Sidebar <kbd>Alt+Shift+R</kbd></button></div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('render')">Render</button><div v-if="activeMenu === 'render'" class="dropdown-menu"><button @click="run(() => exportImage(true))">Render Selected Nodes <kbd>Ctrl+Alt+R</kbd></button><button @click="run(() => exportImage(false))">Render Graph <kbd>Ctrl+Shift+R</kbd></button></div></div>
        <button @click="run(testGraph)">Test</button><button @click="showAbout = true">Help</button>
      </div><div class="test-toolbar"><button class="test-button" title="检查蓝图 (F5)" @click="run(testGraph)">Test</button></div><div class="window-title">Origin Blueprint</div>
    </header>

    <section class="workspace" :style="workspaceStyle">
      <aside class="sidebar sidebar-file-browser">
        <div class="panel workspace-panel">
          <div class="panel-title"><span class="chevron">⌄</span> 文件浏览器<button class="panel-action" @click="chooseWorkspace">…</button></div>
          <div class="workspace-search"><input v-model="workspaceSearch" placeholder="搜索文件..." /></div>
          <div class="workspace-tree">
            <button v-for="row in visibleWorkspaceNodes" :key="row.node.path" class="workspace-entry" :class="{ selected: selectedWorkspacePath === row.node.path, folder: row.node.isDir }" :style="{ paddingLeft: workspaceIndent(row.depth) }" :title="row.node.path" @click="toggleWorkspaceNode(row.node)" @contextmenu.stop.prevent="!row.node.isDir && openFileContextMenu($event, row.node.path)" @dblclick="!row.node.isDir && workspaceOpen(row.node)">
              <span class="workspace-arrow">{{ row.node.loading ? '…' : row.node.isDir ? (workspaceSearch || expandedWorkspacePaths.has(row.node.path) ? '⌄' : '›') : '' }}</span>
              <span class="workspace-icon" :class="{ folder: row.node.isDir }"></span>
              <span class="workspace-name">{{ row.node.name }}</span>
            </button>
            <div v-if="!visibleWorkspaceNodes.length" class="empty-panel">{{ workspaceSearch ? '没有匹配的文件' : '没有可显示的文件' }}</div>
          </div>
        </div>
      </aside>
      <div v-show="showTools" class="sidebar-splitter" @pointerdown="beginLeftSidebarResize"></div>
      <aside v-show="showTools" class="sidebar sidebar-left">
        <div class="panel function-panel" :style="functionPanelStyle"><div class="panel-title"><span class="chevron">⌄</span> 函数</div><div class="tree-row"><span class="folder-dot blue"></span>Default</div></div>
        <div class="panel-height-splitter" @pointerdown="beginLeftPanelHeightResize('function', $event)"></div>
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
        <div class="panel-height-splitter" @pointerdown="beginLeftPanelHeightResize('variable', $event)"></div>
        <div class="panel grow detail-panel sidebar-detail-panel"><div class="panel-title"><span class="chevron">⌄</span> 详情</div><div v-if="selectedVariable" class="node-detail variable-detail"><div class="detail-section-title">变量属性</div><label>Variable ID<input :value="selectedVariable.id" disabled /></label><label>名称<input v-model="selectedVariable.name" /></label><label>类型<select v-model="selectedVariable.type" @change="changeVariableType(selectedVariable)"><option value="boolean">Boolean</option><option value="integer">Integer</option><option value="float">Float</option><option value="string">String</option><option value="array">Array</option><option value="file">File</option><option value="table">Table</option><option value="dictionary">Dictionary</option></select></label><label>分组<select v-model="selectedVariable.groupId"><option v-for="group in variableGroups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label><label>说明<textarea v-model="selectedVariable.description" rows="4" placeholder="变量用途和约束"></textarea></label><label>默认值<input v-if="selectedVariable.type === 'boolean'" v-model="selectedVariable.defaultValue" type="checkbox" /><input v-else-if="selectedVariable.type === 'string' || selectedVariable.type === 'file'" v-model="selectedVariable.defaultValue" type="text" /><input v-else-if="selectedVariable.type === 'array'" :value="Array.isArray(selectedVariable.defaultValue) ? selectedVariable.defaultValue.join(', ') : ''" placeholder="1, 2, text" @change="setVariableArrayDefault(selectedVariable, $event)" /><textarea v-else-if="selectedVariable.type === 'table' || selectedVariable.type === 'dictionary'" :value="JSON.stringify(selectedVariable.defaultValue, null, 2)" rows="6" @change="setVariableStructuredDefault(selectedVariable, $event)"></textarea><input v-else v-model.number="selectedVariable.defaultValue" type="number" /></label><button class="apply-properties" @click="updateVariable(selectedVariable)">应用变量属性</button><button class="delete-properties" @click="removeVariable(selectedVariable)">删除变量</button></div><div v-else-if="selectedNode" class="node-detail"><label>Node ID<input :value="selectedNode.id" disabled /></label><label>Type<input :value="selectedNode.typeId" disabled /></label><label>Title<input v-model="selectedNode.label" :disabled="Boolean(selectedNode.variableId)" /></label><label v-if="selectedNode.description">说明<textarea :value="selectedNode.description" rows="4" readonly></textarea></label><div v-if="Object.keys(selectedNode.values).length" class="detail-section-title">Input Defaults</div><label v-for="(value, key) in selectedNode.values" :key="key">{{ key }}<input v-if="Array.isArray(value)" :value="value.join(', ')" type="text" placeholder="Comma-separated values" @input="setSelectedArrayValue(key, $event)" /><input v-else :value="value" :type="typeof value === 'number' ? 'number' : 'text'" @input="setSelectedValue(key, $event)" /></label><button class="apply-properties" @click="applyNodeProperties">Apply</button></div><div v-else class="empty-detail">选择节点或变量以查看属性</div></div>
      </aside>

      <section class="editor-column">
        <div class="tab-strip"><div v-for="tab in tabs" :key="tab.id" class="graph-tab" :class="{ active: tab.id === activeTabId }" @click="switchTab(tab.id)"><span class="tab-mark"></span>{{ tab.title }}<span v-if="tab.dirty" class="dirty-mark">●</span><button class="tab-close" @click="closeTab(tab.id, $event)">×</button></div><button class="new-tab" @click="newGraph">＋</button></div>
        <div class="canvas-wrap" @contextmenu.prevent @dragenter="allowNodeDrop" @dragover="allowNodeDrop" @drop.prevent="dropNode"><div ref="canvas" class="rete-canvas"></div><div class="canvas-toolbar"><button title="Select">⌖</button><button title="Reset view" @click="editor?.resetView()">⌂</button></div><div class="canvas-hint">Right drag: pan&nbsp;&nbsp; Middle drag: pan&nbsp;&nbsp; Ctrl: multi-select&nbsp;&nbsp; Ctrl + right drag: cut connections&nbsp;&nbsp; Connection: click + Delete</div></div>
        <div v-show="showLogger" class="logger-panel"><div class="logger-title"><span>Test Results</span><button @click="testGraph">Test</button></div><div v-if="!validationIssues.length" class="logger-line">No blueprint issues found.</div><button v-for="issue in validationIssues" :key="`${issue.code}-${issue.nodeId}`" class="logger-issue" :class="issue.severity" @click="focusIssue(issue)"><strong>{{ issue.severity.toUpperCase() }}</strong><span>{{ issue.message }}</span><small>{{ issue.code }}</small></button></div>
        <div v-if="nodeReferenceSearch.visible" class="reference-panel" :class="{ collapsed: referencePanelCollapsed }" :style="referencePanelStyle">
          <div class="reference-resizer" @pointerdown="beginReferencePanelResize"></div>
          <div class="reference-title">
            <strong class="reference-target" :title="nodeReferenceSearch.nodeTitle">查找目标：{{ nodeReferenceSearch.nodeTitle }}</strong>
            <small>{{ nodeReferenceSearch.loading ? '扫描中...' : `${nodeReferenceSearch.results.length} 个蓝图` }}</small>
            <button class="reference-tool-button" :title="referencePanelCollapsed ? '展开引用结果' : '收起引用结果'" @click="toggleReferencePanel">{{ referencePanelCollapsed ? '▴' : '▾' }}</button>
            <button class="reference-tool-button close" title="关闭引用结果" @click="nodeReferenceSearch.visible = false">×</button>
          </div>
          <div v-show="!referencePanelCollapsed" class="reference-results">
            <div v-if="nodeReferenceSearch.loading" class="reference-empty">正在扫描当前工程下的 .vgf / .obp 文件...</div>
            <div v-else-if="!nodeReferenceSearch.results.length" class="reference-empty">没有找到引用该结点的蓝图</div>
            <template v-else>
              <button v-for="result in nodeReferenceSearch.results" :key="result.path" class="reference-row" :title="result.path" @contextmenu.stop.prevent="openFileContextMenu($event, result.path)" @dblclick="openNodeReference(result)">
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
                <button v-for="item in items" :key="item.id" class="module-item" :title="item.title" @pointerdown.stop="beginNodePointerDrag($event, item.id)" @contextmenu.stop.prevent="openModuleNodeMenu($event, item)" @dblclick="addNodeAt(item.id)">{{ item.title }}</button>
              </div>
            </section>
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
      <button @click="openFileContextGraph">打开蓝图</button>
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
