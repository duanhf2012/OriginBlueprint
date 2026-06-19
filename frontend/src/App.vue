<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { toPng } from 'html-to-image'
import { createBlueprintEditor, type BlueprintEditorHandle, type EditorMetrics, type GraphDocument, type GraphVariable, type GraphVariableGroup, type SelectedNodeInfo, type ValidationIssue, type VariableType } from './editor/createEditor'
import { getNodeDefinitions, registerNodeSchemas, type NodeDefinition } from './editor/nodeRegistry'
import { platform, type ExecutionEvent, type ExecutionLog, type WorkspaceEntry } from './platform'

interface GraphTab { id: string; title: string; path: string; dirty: boolean; document: GraphDocument | null }
interface TableValue { columns: string[]; rows: unknown[][] }
interface TableExecutionResult { kind: 'table'; nodeId: string; table: TableValue }
interface WorkspaceTreeNode extends WorkspaceEntry { children: WorkspaceTreeNode[] }
interface VisibleWorkspaceNode { node: WorkspaceTreeNode; depth: number }

const canvas = ref<HTMLElement | null>(null)
const zoomLabel = ref('100%')
const status = ref('Ready')
const metrics = ref<EditorMetrics>({ nodes: 0, connections: 0 })
const activeMenu = ref<string | null>(null)
const contextMenu = ref({ visible: false, x: 0, y: 0, clientX: 0, clientY: 0, search: '' })
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
const showLeft = ref(true)
const showRight = ref(true)
const showLogger = ref(false)
const showAbout = ref(false)
const nodeLibrary = ref<NodeDefinition[]>(getNodeDefinitions())
const moduleSearch = ref('')
const variables = ref<GraphVariable[]>([])
const variableGroups = ref<GraphVariableGroup[]>([{ id: 'default', name: 'Default' }])
const selectedVariableId = ref<string | null>(null)
const selectedNode = ref<SelectedNodeInfo | null>(null)
const validationIssues = ref<ValidationIssue[]>([])
const executionLogs = ref<ExecutionLog[]>([])
const executionResults = ref<unknown[]>([])
const executionVariables = ref<Record<string, unknown>>({})
const executionSessionId = ref('')
const executionRunning = ref(false)
const previewTable = ref<TableExecutionResult | null>(null)
const previewSearch = ref('')
const previewPage = ref(1)
const previewPageSize = ref(100)
let untitledCount = 1
let editor: BlueprintEditorHandle | null = null
let unsubscribeExecution = () => {}
let nodePointerDrag: { typeId: string; startX: number; startY: number; lastX: number; lastY: number; moved: boolean } | null = null
let removeNodePointerListeners = () => {}

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
const tableResults = computed(() => executionResults.value.filter(isTableExecutionResult))
const filteredPreviewRows = computed(() => {
  const rows = previewTable.value?.table.rows ?? []
  const search = previewSearch.value.trim().toLowerCase()
  if (!search) return rows
  return rows.filter(row => row.some(cell => String(cell ?? '').toLowerCase().includes(search)))
})
const previewPageCount = computed(() => Math.max(1, Math.ceil(filteredPreviewRows.value.length / previewPageSize.value)))
const pagedPreviewRows = computed(() => {
  const page = Math.min(previewPage.value, previewPageCount.value)
  const start = (page - 1) * previewPageSize.value
  return filteredPreviewRows.value.slice(start, start + previewPageSize.value)
})
const workspaceStyle = computed(() => ({
  '--file-browser-width': `${fileBrowserWidth.value}px`,
  '--left-tools-width': `${leftToolsWidth.value}px`
}))
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
  unsubscribeExecution = platform.onExecution(handleExecutionEvent)
  recentFiles.value = await platform.recentFiles()
  const initialWorkspace = await platform.currentWorkingDirectory()
  if (initialWorkspace) await loadWorkspace(initialWorkspace)
  window.addEventListener('keydown', onKeyDown)
  window.addEventListener('pointerdown', closeFloatingMenus)
})

function savedPanelWidth(key: string, fallback: number) {
  const value = Number.parseInt(localStorage.getItem(key) ?? '', 10)
  return Number.isFinite(value) ? Math.min(360, Math.max(140, value)) : fallback
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
  unsubscribeExecution(); window.removeEventListener('keydown', onKeyDown); window.removeEventListener('pointerdown', closeFloatingMenus); editor?.destroy()
})

function closeFloatingMenus(event: PointerEvent) {
  const target = event.target as HTMLElement
  if (!target.closest('.menu-root')) activeMenu.value = null
  if (!target.closest('.node-context-menu')) contextMenu.value.visible = false
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
  else if (event.key === 'F5' && event.shiftKey) run(stopGraph, event)
  else if (event.key === 'F5') run(runGraph, event)
  else if (event.altKey && event.shiftKey && key === 'b') { showLogger.value = !showLogger.value; event.preventDefault() }
  else if (event.altKey && event.shiftKey && key === 'l') { showLeft.value = !showLeft.value; event.preventDefault() }
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

function isTableExecutionResult(value: unknown): value is TableExecutionResult {
  if (!value || typeof value !== 'object') return false
  const result = value as Partial<TableExecutionResult>
  return result.kind === 'table' && Boolean(result.table) && Array.isArray(result.table?.columns) && Array.isArray(result.table?.rows)
}

function openTablePreview(result: TableExecutionResult) {
  previewTable.value = result
  previewSearch.value = ''
  previewPage.value = 1
}

function tableAsCSV(table: TableValue) {
  const escape = (value: unknown) => {
    const text = String(value ?? '')
    return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
  }
  return [table.columns, ...table.rows].map(row => row.map(escape).join(',')).join('\r\n')
}

async function copyPreviewCSV() {
  if (!previewTable.value) return
  const csv = tableAsCSV(previewTable.value.table)
  try {
    await navigator.clipboard.writeText(csv)
  } catch {
    const textarea = document.createElement('textarea')
    textarea.value = csv
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    textarea.remove()
  }
  status.value = 'Table CSV copied to clipboard'
}

function exportPreviewCSV() {
  if (!previewTable.value) return
  const blob = new Blob([tableAsCSV(previewTable.value.table)], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = 'table-preview.csv'
  anchor.click()
  URL.revokeObjectURL(url)
  status.value = 'Table preview exported'
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

async function validateGraph() {
  if (!editor) return
  const document = editor.getDocument(activeTab.value.title, variables.value, variableGroups.value)
  validationIssues.value = await platform.validateGraph(JSON.stringify(document))
  showLogger.value = true
  status.value = validationIssues.value.length ? `Validation found ${validationIssues.value.length} issue(s)` : 'Graph validation passed'
}

async function runGraph() {
  if (!editor || executionRunning.value) return
  const document = editor.getDocument(activeTab.value.title, variables.value, variableGroups.value)
  validationIssues.value = await platform.validateGraph(JSON.stringify(document))
  if (validationIssues.value.some(issue => issue.severity === 'error')) {
    showLogger.value = true; status.value = 'Execution blocked by validation errors'; return
  }
  executionLogs.value = []; executionResults.value = []; executionVariables.value = {}
  await editor.clearExecutionStates()
  showLogger.value = true; executionRunning.value = true; status.value = 'Starting graph...'
  try {
    const sessionId = await platform.startGraph(JSON.stringify(document))
    if (executionRunning.value) executionSessionId.value = sessionId
  }
  catch (error) { executionRunning.value = false; status.value = error instanceof Error ? error.message : String(error) }
}

async function stopGraph() {
  if (!executionSessionId.value) return
  status.value = 'Stopping graph...'
  await platform.stopGraph(executionSessionId.value)
}

function handleExecutionEvent(event: ExecutionEvent) {
  if (!event?.sessionId) return
  if (event.type === 'started' && !executionSessionId.value) executionSessionId.value = event.sessionId
  if (event.sessionId !== executionSessionId.value) return
  if (event.states?.length) void editor?.setExecutionStates(event.states)
  if (event.type === 'progress') executionLogs.value.push(...(event.logs ?? []))
  if (event.type === 'completed' || event.type === 'failed' || event.type === 'cancelled') {
    executionRunning.value = false
    executionLogs.value = event.logs ?? executionLogs.value
    executionResults.value = event.results ?? []
    executionVariables.value = event.variables ?? {}
    status.value = event.message ?? event.type
    executionSessionId.value = ''
  } else status.value = event.message ?? 'Graph running'
}

async function focusIssue(issue: ValidationIssue) { if (issue.nodeId) await editor?.focusNode(issue.nodeId) }
async function focusExecutionLog(log: ExecutionLog) { if (log.nodeId) await editor?.focusNode(log.nodeId) }

async function openGraph(path = '') {
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
  if (existing) return switchTab(existing.id)
  const title = file.path.split(/[\\/]/).pop() ?? document.graphName
  const tab: GraphTab = { id: crypto.randomUUID(), title, path: file.path, dirty: false, document }
  tabs.value.push(tab); activeTabId.value = tab.id; selectedVariableId.value = null; await editor?.loadDocument(document)
  if (document.legacy?.format === 'vgf') {
    const hiddenCount = document.legacy.hiddenNodes?.length ?? 0
    status.value = `Loaded ${document.nodes.length} visible node(s), ${hiddenCount} hidden undefined node(s)`
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

async function chooseWorkspace() {
  const path = await platform.chooseWorkspace(); if (path) await loadWorkspace(path)
}

async function clearRecentFiles() {
  await platform.clearRecentFiles()
  recentFiles.value = []
  status.value = 'Recent graph list cleared'
}

async function quitApplication() {
  if (tabs.value.some(tab => tab.dirty) && !window.confirm('There are unsaved graphs. Quit anyway?')) return
  await platform.quit()
}
async function loadWorkspace(path: string) {
  workspaceRoot.value = path
  workspaceTree.value = await loadWorkspaceTree(path)
  expandedWorkspacePaths.value = new Set(workspaceTree.value.filter(item => item.isDir).map(item => item.path))
}

async function loadWorkspaceTree(path: string, depth = 0): Promise<WorkspaceTreeNode[]> {
  if (depth > 8) return []
  const entries = await platform.listWorkspace(path)
  const nodes: WorkspaceTreeNode[] = []
  for (const entry of entries) {
    const node: WorkspaceTreeNode = { ...entry, children: [] }
    if (entry.isDir) node.children = await loadWorkspaceTree(entry.path, depth + 1)
    nodes.push(node)
  }
  return nodes
}

function flattenWorkspaceNodes(nodes: WorkspaceTreeNode[], depth: number, search: string): VisibleWorkspaceNode[] {
  const rows: VisibleWorkspaceNode[] = []
  for (const node of nodes) {
    const childRows = flattenWorkspaceNodes(node.children, depth + 1, search)
    const selfMatches = search ? !node.isDir && node.name.toLowerCase().startsWith(search) : true
    if (search) {
      if (selfMatches || childRows.length) rows.push({ node, depth }, ...childRows)
      continue
    }
    rows.push({ node, depth })
    if (node.isDir && expandedWorkspacePaths.value.has(node.path)) rows.push(...childRows)
  }
  return rows
}

function toggleWorkspaceNode(node: WorkspaceTreeNode) {
  selectedWorkspacePath.value = node.path
  if (!node.isDir) return
  const next = new Set(expandedWorkspacePaths.value)
  if (next.has(node.path)) next.delete(node.path); else next.add(node.path)
  expandedWorkspacePaths.value = next
}

async function workspaceOpen(item: WorkspaceTreeNode) {
  selectedWorkspacePath.value = item.path
  if (item.isDir) toggleWorkspaceNode(item); else await openGraph(item.path)
}

function workspaceIndent(depth: number) {
  return `${8 + depth * 16}px`
}

function workspaceRootName() {
  return workspaceRoot.value.split(/[\\/]/).filter(Boolean).pop() || workspaceRoot.value || 'Workspace'
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
    await editor?.addNode(typeId, position)
  } catch (error) {
    status.value = error instanceof Error ? error.message : String(error)
  }
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
</script>

<template>
  <main class="application-shell" :class="{ 'left-hidden': !showLeft, 'right-hidden': !showRight }">
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
        <div class="menu-root"><button @click.stop="toggleMenu('view')">View</button><div v-if="activeMenu === 'view'" class="dropdown-menu"><button @click="showLogger = !showLogger">Show Logger <kbd>Alt+Shift+B</kbd></button><button @click="showLeft = !showLeft">Show Left Sidebar <kbd>Alt+Shift+L</kbd></button><button @click="showRight = !showRight">Show Right Sidebar <kbd>Alt+Shift+R</kbd></button></div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('render')">Render</button><div v-if="activeMenu === 'render'" class="dropdown-menu"><button @click="run(() => exportImage(true))">Render Selected Nodes <kbd>Ctrl+Alt+R</kbd></button><button @click="run(() => exportImage(false))">Render Graph <kbd>Ctrl+Shift+R</kbd></button></div></div>
        <button @click="run(validateGraph)">Validate</button><button @click="showAbout = true">Help</button>
      </div><div class="run-toolbar"><button class="run-button" :disabled="executionRunning" title="运行蓝图 (F5)" @click="run(runGraph)">▶ Run</button><button class="stop-button" :disabled="!executionRunning" title="停止运行 (Shift+F5)" @click="run(stopGraph)">■ Stop</button></div><div class="window-title">Origin Blueprint</div>
    </header>

    <section class="workspace" :style="workspaceStyle">
      <aside v-show="showLeft" class="sidebar sidebar-file-browser">
        <div class="panel workspace-panel">
          <div class="panel-title"><span class="chevron">⌄</span> 文件浏览器<button class="panel-action" @click="chooseWorkspace">…</button></div>
          <div class="workspace-root" :title="workspaceRoot"><span class="workspace-root-arrow">⌄</span>{{ workspaceRootName() }}</div>
          <div class="workspace-search"><input v-model="workspaceSearch" placeholder="搜索文件..." /></div>
          <div class="workspace-tree">
            <button v-for="row in visibleWorkspaceNodes" :key="row.node.path" class="workspace-entry" :class="{ selected: selectedWorkspacePath === row.node.path, folder: row.node.isDir }" :style="{ paddingLeft: workspaceIndent(row.depth) }" :title="row.node.path" @click="toggleWorkspaceNode(row.node)" @dblclick="!row.node.isDir && workspaceOpen(row.node)">
              <span class="workspace-arrow">{{ row.node.isDir ? (workspaceSearch || expandedWorkspacePaths.has(row.node.path) ? '⌄' : '›') : '' }}</span>
              <span class="workspace-icon" :class="{ folder: row.node.isDir }"></span>
              <span class="workspace-name">{{ row.node.name }}</span>
            </button>
            <div v-if="!visibleWorkspaceNodes.length" class="empty-panel">{{ workspaceSearch ? '没有匹配的文件' : '没有可显示的文件' }}</div>
          </div>
        </div>
      </aside>
      <div v-show="showLeft" class="sidebar-splitter" @pointerdown="beginLeftSidebarResize"></div>
      <aside v-show="showLeft" class="sidebar sidebar-left">
        <div class="panel"><div class="panel-title"><span class="chevron">⌄</span> 函数</div><div class="tree-row"><span class="folder-dot blue"></span>Default</div></div>
        <div class="panel grow variable-panel"><div class="panel-title"><span class="chevron">⌄</span> 变量 <span class="panel-title-spacer"></span><button class="panel-action" title="添加变量组" @click="addVariableGroup">▣＋</button><button class="panel-action" title="添加变量" @click="addVariable()">＋</button></div>
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
      </aside>

      <section class="editor-column">
        <div class="tab-strip"><div v-for="tab in tabs" :key="tab.id" class="graph-tab" :class="{ active: tab.id === activeTabId }" @click="switchTab(tab.id)"><span class="tab-mark"></span>{{ tab.title }}<span v-if="tab.dirty" class="dirty-mark">●</span><button class="tab-close" @click="closeTab(tab.id, $event)">×</button></div><button class="new-tab" @click="newGraph">＋</button></div>
        <div class="canvas-wrap" @contextmenu.prevent @dragenter="allowNodeDrop" @dragover="allowNodeDrop" @drop.prevent="dropNode"><div ref="canvas" class="rete-canvas"></div><div class="canvas-toolbar"><button title="Select">⌖</button><button title="Reset view" @click="editor?.resetView()">⌂</button></div><div class="canvas-hint">Middle drag: pan&nbsp;&nbsp; Ctrl: multi-select&nbsp;&nbsp; Ctrl + right drag: cut connections&nbsp;&nbsp; Connection: click + Delete</div></div>
        <div v-show="showLogger" class="logger-panel"><div class="logger-title"><span :class="{ running: executionRunning }">{{ executionRunning ? 'Running Graph...' : 'Logger' }}</span><button @click="validateGraph">Validate</button><button :disabled="executionRunning" @click="runGraph">Run</button></div><div v-if="!validationIssues.length && !executionLogs.length && !executionResults.length" class="logger-line">No validation or execution messages.</div><button v-for="issue in validationIssues" :key="`${issue.code}-${issue.nodeId}`" class="logger-issue" :class="issue.severity" @click="focusIssue(issue)"><strong>{{ issue.severity.toUpperCase() }}</strong><span>{{ issue.message }}</span><small>{{ issue.code }}</small></button><button v-for="(log, index) in executionLogs" :key="`run-${index}-${log.nodeId}`" class="logger-issue execution-log" :class="log.level" @click="focusExecutionLog(log)"><strong>{{ log.level.toUpperCase() }}</strong><span>{{ log.message }}</span><small>{{ log.nodeId || 'runtime' }}</small></button><button v-for="result in tableResults" :key="result.nodeId" class="table-result" @click="openTablePreview(result)"><strong>TABLE</strong><span>{{ result.table.rows.length }} rows x {{ result.table.columns.length }} columns</span><small>Open preview</small></button><div v-if="executionResults.length && !tableResults.length" class="execution-summary"><strong>Results</strong><code>{{ JSON.stringify(executionResults) }}</code></div><div v-if="Object.keys(executionVariables).length" class="execution-summary"><strong>Variables</strong><code>{{ JSON.stringify(executionVariables) }}</code></div></div>
        <footer class="status-bar"><span>{{ status }}</span><span>Nodes {{ metrics.nodes }} · Connections {{ metrics.connections }}</span><button @click="editor?.resetView()">{{ zoomLabel }}</button></footer>
      </section>

      <aside v-show="showRight" class="sidebar sidebar-right"><div class="panel module-panel"><div class="panel-title"><span class="chevron">⌄</span> 模块库</div><div class="search-box">⌕ <input v-model="moduleSearch" placeholder="搜索模块..." /></div><div class="module-list"><section v-for="[category, items] in categories" :key="category"><div class="module-category"><span>⌄</span>{{ category }}</div><button v-for="item in items" :key="item.id" class="module-item" @pointerdown.stop="beginNodePointerDrag($event, item.id)" @dblclick="addNodeAt(item.id)">{{ item.title }}</button></section><div v-if="!categories.length" class="empty-panel">{{ status || '没有匹配的模块' }}</div></div></div><div class="panel grow detail-panel"><div class="panel-title"><span class="chevron">⌄</span> 详情</div><div v-if="selectedVariable" class="node-detail variable-detail"><div class="detail-section-title">变量属性</div><label>Variable ID<input :value="selectedVariable.id" disabled /></label><label>名称<input v-model="selectedVariable.name" /></label><label>类型<select v-model="selectedVariable.type" @change="changeVariableType(selectedVariable)"><option value="boolean">Boolean</option><option value="integer">Integer</option><option value="float">Float</option><option value="string">String</option><option value="array">Array</option><option value="file">File</option><option value="table">Table</option><option value="dictionary">Dictionary</option></select></label><label>分组<select v-model="selectedVariable.groupId"><option v-for="group in variableGroups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label><label>说明<textarea v-model="selectedVariable.description" rows="4" placeholder="变量用途和约束"></textarea></label><label>默认值<input v-if="selectedVariable.type === 'boolean'" v-model="selectedVariable.defaultValue" type="checkbox" /><input v-else-if="selectedVariable.type === 'string' || selectedVariable.type === 'file'" v-model="selectedVariable.defaultValue" type="text" /><input v-else-if="selectedVariable.type === 'array'" :value="Array.isArray(selectedVariable.defaultValue) ? selectedVariable.defaultValue.join(', ') : ''" placeholder="1, 2, text" @change="setVariableArrayDefault(selectedVariable, $event)" /><textarea v-else-if="selectedVariable.type === 'table' || selectedVariable.type === 'dictionary'" :value="JSON.stringify(selectedVariable.defaultValue, null, 2)" rows="6" @change="setVariableStructuredDefault(selectedVariable, $event)"></textarea><input v-else v-model.number="selectedVariable.defaultValue" type="number" /></label><button class="apply-properties" @click="updateVariable(selectedVariable)">应用变量属性</button><button class="delete-properties" @click="removeVariable(selectedVariable)">删除变量</button></div><div v-else-if="selectedNode" class="node-detail"><label>Node ID<input :value="selectedNode.id" disabled /></label><label>Type<input :value="selectedNode.typeId" disabled /></label><label>Title<input v-model="selectedNode.label" :disabled="Boolean(selectedNode.variableId)" /></label><div v-if="Object.keys(selectedNode.values).length" class="detail-section-title">Input Defaults</div><label v-for="(value, key) in selectedNode.values" :key="key">{{ key }}<input v-if="Array.isArray(value)" :value="value.join(', ')" type="text" placeholder="Comma-separated values" @input="setSelectedArrayValue(key, $event)" /><input v-else :value="value" :type="typeof value === 'number' ? 'number' : 'text'" @input="setSelectedValue(key, $event)" /></label><button class="apply-properties" @click="applyNodeProperties">Apply</button></div><div v-else class="empty-detail">选择节点或变量以查看属性</div></div></aside>
    </section>
    <div v-if="previewTable" class="table-preview-backdrop" @click.self="previewTable = null">
      <section class="table-preview-dialog">
        <header><strong>Table Preview</strong><span>{{ previewTable.table.rows.length }} rows x {{ previewTable.table.columns.length }} columns</span><button @click="copyPreviewCSV">Copy CSV</button><button @click="exportPreviewCSV">Export CSV</button><button @click="previewTable = null">Close</button></header>
        <div class="table-preview-tools"><input v-model="previewSearch" placeholder="Search all cells..." @input="previewPage = 1" /><label>Rows<select v-model.number="previewPageSize" @change="previewPage = 1"><option :value="50">50</option><option :value="100">100</option><option :value="250">250</option><option :value="500">500</option></select></label><span>{{ filteredPreviewRows.length }} matched</span></div>
        <div class="table-preview-scroll"><table><thead><tr><th class="row-number">#</th><th v-for="column in previewTable.table.columns" :key="column">{{ column }}</th></tr></thead><tbody><tr v-for="(row, rowIndex) in pagedPreviewRows" :key="rowIndex"><td class="row-number">{{ (previewPage - 1) * previewPageSize + rowIndex + 1 }}</td><td v-for="(cell, cellIndex) in row" :key="cellIndex" :title="String(cell ?? '')">{{ cell }}</td></tr></tbody></table><div v-if="!pagedPreviewRows.length" class="table-preview-empty">No matching rows.</div></div>
        <footer><button :disabled="previewPage <= 1" @click="previewPage--">Previous</button><span>Page {{ Math.min(previewPage, previewPageCount) }} / {{ previewPageCount }}</span><button :disabled="previewPage >= previewPageCount" @click="previewPage++">Next</button></footer>
      </section>
    </div>
    <div v-if="showAbout" class="about-backdrop" @click.self="showAbout = false"><section class="about-dialog"><header><strong>Origin Blueprint</strong><button @click="showAbout = false">×</button></header><p>Cross-platform blueprint editor built with Go, Wails, Vue 3 and Rete.js.</p><dl><dt>Canvas</dt><dd>Middle drag pans, mouse wheel zooms, left drag selects.</dd><dt>Connections</dt><dd>Click a connection then Delete, or Ctrl + right-drag to cut lines.</dd><dt>Editing</dt><dd>Ctrl+C/X/V, Ctrl+Z/Y, Ctrl+G and alignment shortcuts match OriginNodeEditor.</dd><dt>Run</dt><dd>F5 runs the graph; Shift+F5 stops it.</dd></dl><footer><button @click="showAbout = false">Close</button></footer></section></div>
  </main>
</template>
