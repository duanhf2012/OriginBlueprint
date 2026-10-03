<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { toPng } from 'html-to-image'
import { createBlueprintEditor, type BlueprintEditorHandle, type EditorMetrics, type FunctionSignature, type FunctionSignaturePort, type GraphDocument, type GraphVariable, type GraphVariableGroup, type SelectedNodeInfo, type ValidationIssue, type VariableType } from './editor/createEditor'
import type { MacroMirrorPayload } from './editor/macroRefs'
import { workspaceRelativeHint } from './editor/macroRefs'
import { variableScope, type FunctionNodeMetadata, type NodeSnapshot, type RestoreLossReport, type VariableScope } from './editor/document'
import { applyVariableGroupDrop, matchingVariableGroupId, moveVariablesToDefaultGroup, normalizeVariableGroups, planVariableGroupDrop, variableGroupNameExists, variableGroupRemovalMessage, variableGroupsForScope, variableGroupScope, variableGroupUsage, type VariableGroupDropPlan } from './editor/variableGroups'
import { getNodeDefinitions, registerNodeSchemas, type NodeDefinition } from './editor/nodeRegistry'
import { configTableStatusLine, filterConfigEntries, setConfigTables, type ConfigTable, type ConfigTableEntry } from './editor/configTables'
import { menuLocales, normalizeLocale, type LocaleId } from './i18n'
import { platform, type NodeReferenceResult, type RecoverySnapshotResult, type WorkspaceEntry } from './platform'
import { compatibilitySaveOptions, findOpenTab, hasRestoreLoss, resolveCompatibilitySaveAction as resolveCompatibilityPersistenceAction, sourceRequiresProtection, type CompatibilitySaveAction } from './documentSafety'
import { autoSaveIntervalMs, isAutoSaveEligible, type AutoSaveMode } from './autoSavePolicy'
import { saveGateDecision } from './saveGate'
import { applyFunctionPersistenceMetadata, documentRequiresNativePersistence as graphDocumentRequiresNativePersistence, filenameStem, isFunctionBlueprintPath, prepareGraphSave, serializeGraphDocument } from './graphPersistence'
import { isValidIntegerDefault, normalizeIntegerInput } from './editor/valueValidation'
import { isPreciseJSONInteger, parseGraphJSON } from './graphJSON'
import { socketStyle } from './editor/socketTheme'

interface GraphTab { id: string; title: string; path: string; dirty: boolean; document: GraphDocument | null; restoreLoss?: RestoreLossReport | null; restoreFatal?: boolean; saveBlocked?: boolean }
interface WorkspaceTreeNode extends WorkspaceEntry { children: WorkspaceTreeNode[]; loaded: boolean; loading: boolean }
interface VisibleWorkspaceNode { node: WorkspaceTreeNode; depth: number }
type UnsavedCloseAction = 'save' | 'discard' | 'cancel'
interface ModuleNodeMenuState { visible: boolean; x: number; y: number; node: ModuleLibraryItem | null }
interface NodeReferenceSearchState { visible: boolean; loading: boolean; nodeTitle: string; typeId: string; results: NodeReferenceResult[] }
interface FileContextMenuState { visible: boolean; x: number; y: number; path: string; isDir: boolean; isFunction: boolean }
interface CanvasToastState { visible: boolean; message: string; x: number; y: number }
interface BlueprintFunction { id: string; name: string; readonly?: boolean }
interface FunctionLibraryItem { id: string; functionId: string; name: string; category: string; path: string; source: 'current' | 'workspace' }
interface ModuleLibraryItem extends NodeDefinition { functionPlaceholder?: boolean; functionSource?: FunctionLibraryItem['source']; functionItem?: FunctionLibraryItem; macroPlaceholder?: boolean; path?: string }
type UiScale = 'small' | 'normal' | 'large'
type NodeScale = 'normal' | 'large'
type ImageExportScale = 1 | 2 | 4
interface ConfigTableFilterSetting {
  mode: 'text' | 'range' | 'compare'
  column: 'all' | 'id' | 'name' | 'custom'
  text: string
  rangeMin: string
  rangeMax: string
  compareOp: string
  compareValue: string
  extraMatchRow: number
  extraKeyword: string
}

// 数据集是节点引用绑定的单元：key 是稳定标识（nodes/*.json 与 .obpf 的 ref 存的值，
// 创建后不变），name 是可随时修改的显示名；一张物理表（file+sheet）可派生多个数据集。
interface ConfigTableDatasetSetting {
  key: string
  name: string
  file: string
  sheet: string
  idMatchRow: number
  nameMatchRow: number
  idKeyword: string
  nameKeyword: string
  filter: ConfigTableFilterSetting
}

interface ProjectSettings {
  version: number
  appearance: { locale: LocaleId; uiScale: UiScale; nodeScale: NodeScale; moduleScale: UiScale }
  layout: {
    panels: { files: number; tools: number; library: number; variables: number; test: number; references: number }
    visible: { tools: boolean; library: boolean; test: boolean }
  }
  explorer: { expanded: string[]; selected: string; revealActiveFile: boolean; hideBuildFolders: boolean }
  editor: { autoSave: AutoSaveMode; validateBeforeSave: boolean }
  export: { imageScale: ImageExportScale; showGrid: boolean }
  configTables: {
    directories: string[]
    defaults: { idMatchRow: number; nameMatchRow: number; idKeyword: string; nameKeywords: string[] }
    datasets: ConfigTableDatasetSetting[]
  }
}
interface UpdateCheckState {
  autoCheck: boolean
  checking: boolean
  visible: boolean
  latestVersion: string
  currentVersion: string
  htmlUrl: string
  notes: string
  error: string
}

const canvas = ref<HTMLElement | null>(null)
const tabStrip = ref<HTMLElement | null>(null)
const zoomLabel = ref('100%')
const status = ref('Ready')
const metrics = ref<EditorMetrics>({ nodes: 0, connections: 0, functionEntries: 0, functionReturns: 0 })
const activeMenu = ref<string | null>(null)
const contextMenu = ref({ visible: false, x: 0, y: 0, clientX: 0, clientY: 0, search: '' })
const moduleNodeMenu = ref<ModuleNodeMenuState>({ visible: false, x: 0, y: 0, node: null })
const nodeReferenceSearch = ref<NodeReferenceSearchState>({ visible: false, loading: false, nodeTitle: '', typeId: '', results: [] })
const fileContextMenu = ref<FileContextMenuState>({ visible: false, x: 0, y: 0, path: '', isDir: false, isFunction: false })
const canvasToast = ref<CanvasToastState>({ visible: false, message: '', x: 50, y: 18 })
const functionReferenceSearchPrefix = 'function:'
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
const functionIdByPath = ref<Record<string, string>>({})
const functionCategoryByPath = ref<Record<string, string>>({})
const leftToolsDefaultWidth = 280
const leftToolsMinWidth = 240
const leftToolsMaxWidth = 520
const fileBrowserMinWidth = 140
const fileBrowserMaxWidth = 720
const fileBrowserWidth = ref(savedPanelWidth('origin-blueprint-file-browser-width', 210, fileBrowserMinWidth, fileBrowserMaxWidth))
const leftToolsWidth = ref(savedPanelWidth('origin-blueprint-left-tools-width', leftToolsDefaultWidth, leftToolsMinWidth, leftToolsMaxWidth))
const rightSidebarWidth = ref(savedPanelWidth('origin-blueprint-right-sidebar-width', 230, 160, 460))
const variablePanelHeight = ref(savedPanelSize('origin-blueprint-variable-panel-height', 300, 130, 520))
const variablePanelCollapsed = ref(false)
const detailPanelCollapsed = ref(false)
const showTools = ref(true)
const showRight = ref(true)
const showLogger = ref(false)
const canvasSearchVisible = ref(false)
const canvasSearchRef = ref<HTMLInputElement | null>(null)
const canvasSearchQuery = ref('')
const canvasSearchResults = computed(() => (canvasSearchVisible.value && canvasSearchQuery.value.trim() ? (editor?.searchNodes(canvasSearchQuery.value) ?? []) : []))

function jumpToCanvasSearchResult(nodeId: string) {
  if (nodeId.startsWith('comment:')) void editor?.focusComment(nodeId.slice('comment:'.length))
  else void editor?.focusNode(nodeId)
}

function closeCanvasSearch() {
  canvasSearchVisible.value = false
  canvasSearchQuery.value = ''
}
const showAbout = ref(false)
const showShortcuts = ref(false)
const showSettings = ref(false)
const updateCheckUrl = 'https://api.github.com/repos/duanhf2012/OriginBlueprint/releases/latest'
const releasePageUrl = 'https://github.com/duanhf2012/OriginBlueprint/releases/latest'
const appVersion = String(import.meta.env.VITE_APP_VERSION || '0.0.0')
const updateState = ref<UpdateCheckState>({
  autoCheck: localStorage.getItem('origin-blueprint-auto-check-updates') !== 'false',
  checking: false,
  visible: false,
  latestVersion: '',
  currentVersion: appVersion,
  htmlUrl: '',
  notes: '',
  error: ''
})
const projectSettingsPath = ref('')
const projectSettingsContent = ref<ProjectSettings>(defaultProjectSettings())
const nodeLibrary = ref<NodeDefinition[]>(getNodeDefinitions())
const moduleSearch = ref('')
const expandedModuleCategories = ref<Set<string>>(new Set())
const variables = ref<GraphVariable[]>([])
const variableGroups = ref<GraphVariableGroup[]>([{ id: 'default', name: 'Default' }])
const functionSignature = ref<FunctionSignature>(emptyFunctionSignature())
const functionTitle = ref('')
const functionId = ref('')
const functionCategory = ref('')
const functionDescription = ref('')
const functionDescriptionByPath = ref<Record<string, string>>({})
const functionCategoryDropdownOpen = ref(false)
const functionSignatureTypeOptions: Array<{ value: VariableType; label: string }> = [
  { value: 'boolean', label: 'Boolean' },
  { value: 'integer', label: 'Integer' },
  { value: 'float', label: 'Float' },
  { value: 'string', label: 'String' },
  { value: 'array', label: 'Array' },
  { value: 'timerhandle', label: 'Timer Handle' }
]
const blueprintFunctions = ref<BlueprintFunction[]>([])
const selectedFunctionId = ref('')
const selectedVariableId = ref<string | null>(null)
const selectedNode = ref<SelectedNodeInfo | null>(null)
const nodeSchemaDocuments = ref<Array<{ path: string; key?: string; content: string }>>([])
const nodeAnnotationDraft = ref({
  description: '',
  tips: {} as Record<string, string>,
  refs: {} as Record<string, string>,
  functionParams: [] as Array<{ id: string; name: string; direction: 'input' | 'output'; tip: string; type: string; ref: string }>
})
const nodeAnnotationDialog = ref<{ visible: boolean; typeId?: string; functionPath?: string; title: string } | null>(null)

function nodeDefinitionById(typeId: string) {
  return getNodeDefinitions().find(definition => definition.id === typeId)
}

function labeledInputPorts(ports: Array<{ key: string; label: string; tip?: string; portId?: number; refTable?: string; type?: string }> | undefined) {
  return (ports ?? []).filter(port => port.label)
}

const importedConfigTableOptions = computed(() => configTables.value.map(table => ({ key: table.key, name: table.name || table.key, entries: table.entries.length })))

function loadNodeAnnotationDraft(typeId: string) {
  const definition = nodeDefinitionById(typeId)
  nodeAnnotationDraft.value = {
    description: definition?.description ?? '',
    tips: Object.fromEntries(labeledInputPorts(definition?.inputPorts).map(port => [port.key, port.tip ?? ''])),
    refs: Object.fromEntries(labeledInputPorts(definition?.inputPorts).map(port => [port.key, port.refTable ?? ''])),
    functionParams: []
  }
}

// 点住对话框标题栏拖动窗口；偏移记录在 data 属性上，重开对话框由 v-if 重建自然复位。
function beginDialogDrag(event: PointerEvent) {
  if (event.button !== 0) return
  if ((event.target as HTMLElement).closest('button, input, select, textarea')) return
  const dialog = (event.currentTarget as HTMLElement).closest('.about-dialog') as HTMLElement | null
  if (!dialog) return
  const baseX = Number(dialog.dataset.dragX || 0)
  const baseY = Number(dialog.dataset.dragY || 0)
  const startX = event.clientX
  const startY = event.clientY
  const move = (next: PointerEvent) => {
    dialog.dataset.dragX = String(baseX + next.clientX - startX)
    dialog.dataset.dragY = String(baseY + next.clientY - startY)
    dialog.style.transform = `translate(${dialog.dataset.dragX}px, ${dialog.dataset.dragY}px)`
  }
  const up = () => window.removeEventListener('pointermove', move)
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up, { once: true })
  window.addEventListener('pointercancel', up, { once: true })
}

// 拖动对话框右下角缩放：宽度/高度记录在 style 上，重开对话框由 v-if 重建自然复位。
function beginDialogResize(event: PointerEvent) {
  if (event.button !== 0) return
  const dialog = (event.currentTarget as HTMLElement).closest('.about-dialog') as HTMLElement | null
  if (!dialog) return
  const startWidth = dialog.offsetWidth
  const startHeight = dialog.offsetHeight
  const startX = event.clientX
  const startY = event.clientY
  const move = (next: PointerEvent) => {
    const width = Math.max(560, Math.min(startWidth + next.clientX - startX, window.innerWidth - 40))
    const height = Math.max(420, Math.min(startHeight + next.clientY - startY, window.innerHeight - 40))
    dialog.style.width = `${width}px`
    dialog.style.height = `${height}px`
  }
  const up = () => window.removeEventListener('pointermove', move)
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up, { once: true })
  window.addEventListener('pointercancel', up, { once: true })
}

function openNodeAnnotationDialog(typeId: string) {
  const definition = nodeDefinitionById(typeId)
  if (!definition) return
  loadNodeAnnotationDraft(typeId)
  nodeAnnotationDialog.value = { visible: true, typeId, title: definition.title }
  moduleNodeMenu.value.visible = false
}

async function openFunctionAnnotationDialog(item: ModuleLibraryItem) {
  const path = item.functionItem?.path
  if (!path) return
  const signature = await loadFunctionSignatureForModuleItem(item)
  nodeAnnotationDraft.value = {
    description: functionDescriptionByPath.value[path] ?? '',
    tips: {},
    refs: {},
    functionParams: [
      ...signature.inputs.map(port => ({ id: port.id, name: port.name, direction: 'input' as const, tip: port.description ?? '', type: port.type, ref: port.ref ?? '' })),
      ...signature.outputs.map(port => ({ id: port.id, name: port.name, direction: 'output' as const, tip: port.description ?? '', type: port.type, ref: port.ref ?? '' }))
    ]
  }
  nodeAnnotationDialog.value = { visible: true, functionPath: path, title: item.title }
  moduleNodeMenu.value.visible = false
}

function closeNodeAnnotationDialog() {
  nodeAnnotationDialog.value = null
}

async function applyNodeAnnotationsFromDialog() {
  const dialog = nodeAnnotationDialog.value
  if (!dialog) return
  const saved = dialog.functionPath
    ? await saveFunctionAnnotation(dialog.functionPath)
    : dialog.typeId
      ? await saveNodeDefinitionAnnotations(dialog.typeId)
      : false
  if (saved) closeNodeAnnotationDialog()
}

function functionAnnotationPortTips() {
  const tips = new Map<string, string>()
  for (const param of nodeAnnotationDraft.value.functionParams) tips.set(param.id, param.tip)
  return tips
}

function applySignaturePortTips(signature: { inputs: FunctionSignaturePort[]; outputs: FunctionSignaturePort[] } | undefined, tipsById: Map<string, string>, refsById?: Map<string, string>) {
  if (!signature) return
  for (const port of [...signature.inputs, ...signature.outputs]) {
    const tip = tipsById.get(port.id)
    if (tip !== undefined) {
      if (tip.trim()) port.description = tip.trim()
      else delete port.description
    }
    if (refsById) {
      const ref = refsById.get(port.id)
      if (ref !== undefined) {
        if (ref.trim()) port.ref = ref.trim()
        else delete port.ref
      }
    }
  }
}

function functionAnnotationRefs() {
  const refs = new Map<string, string>()
  for (const param of nodeAnnotationDraft.value.functionParams) refs.set(param.id, param.ref)
  return refs
}

async function saveFunctionAnnotation(path: string) {
  const description = nodeAnnotationDraft.value.description.trim()
  const portTips = functionAnnotationPortTips()
  const openTab = tabs.value.find(tab => tab.path === path)
  if (openTab) {
    if (openTab.id === activeTabId.value) {
      functionDescription.value = description
      applySignaturePortTips(functionSignature.value, portTips, functionAnnotationRefs())
    }
    if (openTab.document) {
      if (description) openTab.document.functionDescription = description
      else delete openTab.document.functionDescription
      applySignaturePortTips(openTab.document.functionSignature, portTips, functionAnnotationRefs())
    }
    openTab.dirty = true
    functionDescriptionByPath.value = { ...functionDescriptionByPath.value, [path]: description }
    await syncCallableFunctionsToEditor()
    const functionId = openTab.id === activeTabId.value ? activeFunctionId() : functionIdFromDocument(openTab.document)
    if (functionId) await editor?.refreshFunctionNodeAnnotations(functionId, description, portTips, functionAnnotationRefs())
    status.value = `已在打开的函数蓝图“${openTab.title}”中更新说明，保存后生效`
    return true
  }
  try {
    const file = await platform.openGraph(path)
    if (!file) {
      status.value = '未找到函数蓝图文件'
      return false
    }
    const parsed = parseGraphJSON(file.content) as Partial<GraphDocument>
    if (description) parsed.functionDescription = description
    else delete parsed.functionDescription
    applySignaturePortTips(parsed.functionSignature, portTips, functionAnnotationRefs())
    const content = serializeGraphDocument(path, parsed as GraphDocument, 2)
    await platform.saveGraph(path, content)
  } catch (error) {
    status.value = `函数说明保存失败：${error instanceof Error ? error.message : String(error)}`
    return false
  }
  functionDescriptionByPath.value = { ...functionDescriptionByPath.value, [path]: description }
  await syncCallableFunctionsToEditor()
  const functionId = functionIdByPath.value[path]
  if (functionId) await editor?.refreshFunctionNodeAnnotations(functionId, description, portTips, functionAnnotationRefs())
  status.value = '函数说明已保存，悬停对应的函数节点可见'
  return true
}

function applyNodeDefinitionEdits(parsed: unknown, sourceName: string, description: string, tipsByPortId: Map<number, string>, refsByPortId: Map<number, string>) {
  const container = Array.isArray(parsed)
    ? parsed
    : parsed && typeof parsed === 'object' && Array.isArray((parsed as { nodes?: unknown[] }).nodes)
      ? (parsed as { nodes: unknown[] }).nodes
      : null
  if (!container) return false
  const entry = container.find(item => item && typeof item === 'object' && (item as { name?: unknown }).name === sourceName)
  if (!entry) return false
  const record = entry as Record<string, unknown>
  const descriptionText = description.trim()
  if (descriptionText) record.description = descriptionText
  else delete record.description
  if (Array.isArray(record.inputs)) {
    for (const input of record.inputs) {
      if (!input || typeof input !== 'object' || typeof (input as { port_id?: unknown }).port_id !== 'number') continue
      const portId = (input as { port_id: number }).port_id
      if (!tipsByPortId.has(portId)) continue
      const inputRecord = input as Record<string, unknown>
      const tip = String(tipsByPortId.get(portId) ?? '').trim()
      if (tip) inputRecord.tip = tip
      else delete inputRecord.tip
      if (refsByPortId.has(portId)) {
        const ref = String(refsByPortId.get(portId) ?? '').trim()
        if (ref) inputRecord.ref = ref
        else delete inputRecord.ref
      }
    }
  }
  return true
}

async function saveNodeDefinitionAnnotations(typeId: string) {
  const definition = nodeDefinitionById(typeId)
  if (!definition) {
    status.value = '未找到节点定义'
    return false
  }
  if (!definition.sourceKey || !definition.sourceName) {
    status.value = '该节点类型没有可编辑的 JSON 定义来源'
    return false
  }
  if (!workspaceRoot.value) {
    status.value = '编辑节点注解需要先打开工作区（文件 → 打开目录）'
    return false
  }
  const document = nodeSchemaDocuments.value.find(item => item.key === definition.sourceKey)
  if (!document) {
    status.value = `未找到节点定义文档 ${definition.sourceKey}`
    return false
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(document.content)
  } catch (error) {
    status.value = `节点定义 JSON 解析失败：${error instanceof Error ? error.message : String(error)}`
    return false
  }
  const tipsByPortId = new Map<number, string>()
  const refsByPortId = new Map<number, string>()
  for (const port of labeledInputPorts(definition.inputPorts)) {
    if (typeof port.portId !== 'number') continue
    tipsByPortId.set(port.portId, String(nodeAnnotationDraft.value.tips[port.key] ?? ''))
    if (port.type === 'integer') refsByPortId.set(port.portId, String(nodeAnnotationDraft.value.refs[port.key] ?? ''))
  }
  if (!applyNodeDefinitionEdits(parsed, definition.sourceName, nodeAnnotationDraft.value.description, tipsByPortId, refsByPortId)) {
    status.value = `节点定义文档中未找到 ${definition.sourceName}`
    return false
  }
  const content = JSON.stringify(parsed, null, '\t')
  try {
    await platform.writeNodeSchemaDocument(workspaceRoot.value, definition.sourceKey, content)
  } catch (error) {
    status.value = `节点定义保存失败：${error instanceof Error ? error.message : String(error)}`
    return false
  }
  await loadRuntimeNodeLibrary()
  const refreshed = await editor?.refreshNodeTypeAnnotations(typeId)
  status.value = `节点备注已保存到 ${definition.sourceKey}，所有蓝图生效${refreshed ? `，已刷新 ${refreshed} 个画布节点` : ''}`
  return true
}
const validationIssues = ref<ValidationIssue[]>([])
const selectedValidationIssueKey = ref('')
const unsavedCloseDialog = ref<{ visible: boolean; names: string[]; resolve?: (action: UnsavedCloseAction) => void }>({ visible: false, names: [] })
const compatibilitySaveDialog = ref<{ visible: boolean; droppedNodes: number; droppedConnections: number; alteredNodes: number; fatal: boolean; forceAllowed: boolean; resolve?: (action: CompatibilitySaveAction) => void }>({ visible: false, droppedNodes: 0, droppedConnections: 0, alteredNodes: 0, fatal: false, forceAllowed: false })
const recoveryDialog = ref<{ visible: boolean; snapshot: RecoverySnapshotResult | null }>({ visible: false, snapshot: null })
let recoveryQueue: RecoverySnapshotResult[] = []
let untitledCount = 1
const tabDragIndex = ref(-1)
const tabDragOverIndex = ref(-1)
const variableDragId = ref('')
const variableDropIndicator = ref<{ key: string; plan: VariableGroupDropPlan; label: string } | null>(null)
let editor: BlueprintEditorHandle | null = null
let unsubscribeCloseRequest = () => {}
let closingApplication = false
let nodePointerDrag: { item: ModuleLibraryItem; startX: number; startY: number; lastX: number; lastY: number; moved: boolean } | null = null
let removeNodePointerListeners = () => {}
let workspaceLoadToken = 0
let nodeSchemaLoadToken = 0
let workspaceRefreshInFlight = false
let workspaceRefreshTimer: ReturnType<typeof window.setInterval> | undefined
let validationIssueClickTimer: ReturnType<typeof window.setTimeout> | undefined
let canvasToastTimer: ReturnType<typeof window.setTimeout> | undefined
let updateCheckTimer: ReturnType<typeof window.setTimeout> | undefined
let autoSaveTimer: ReturnType<typeof window.setInterval> | undefined
let persistenceInFlight = false
let applyingProjectSettings = false
const loadingFunctionTitles = new Set<string>()
const workspaceRefreshIntervalKey = 'origin-blueprint-workspace-refresh-interval'
const workspaceRefreshIntervalMs = Math.max(1000, Number.parseInt(localStorage.getItem(workspaceRefreshIntervalKey) ?? '1500', 10) || 1500)

const activeTab = computed(() => tabs.value.find(tab => tab.id === activeTabId.value)!)
const selectedVariable = computed(() => variables.value.find(variable => variable.id === selectedVariableId.value) ?? null)
const isFunctionBlueprintTab = computed(() => isFunctionBlueprintPath(activeTab.value?.path || activeTab.value?.title || ''))
const variableScopeSections = computed(() => ([
  {
    scope: 'execution' as const,
    title: '局部变量',
    variables: variables.value.filter(variable => variableScope(variable) === 'execution')
  },
  {
    scope: 'instance' as const,
    title: '全局变量',
    variables: variables.value.filter(variable => variableScope(variable) === 'instance')
  }
].map(section => {
  const scopedGroups = variableGroupsForScope(variableGroups.value, section.scope)
  return {
    ...section,
    groups: scopedGroups.map(group => ({
      group,
      variables: section.variables.filter(variable => variable.groupId === group.id)
    })),
    hasCustomGroups: scopedGroups.some(group => group.id !== 'default')
  }
})))
const functionLibraryItems = computed(() => collectFunctionLibraryItems(workspaceTree.value))
const macroTitleByPath = ref<Record<string, string>>({})
const macroIdByPath = ref<Record<string, string>>({})
const isMacroBlueprintTab = computed(() => isMacroSourcePath(activeTab.value?.path ?? ''))
// 选中信息走 onSelection 回调刷新（编辑器选区不是 Vue 响应式源，computed 不会自动失效）。
const macroRefSelectionTick = ref(0)
const selectedMacroRefInfo = computed(() => {
  void macroRefSelectionTick.value
  return editor?.selectedMacroRef() ?? null
})

// === 宏源解析（v2 引用模型）：按 macroId 在工作区 .obpm 中定位宏，读出的载荷供镜像展开/热更。 ===
const macroPayloadCache = new Map<string, MacroMirrorPayload | null>()

function workspaceMacroPaths(): string[] {
  const paths: string[] = []
  const visit = (entry: WorkspaceTreeNode) => {
    if (!entry.isDir && /\.obpm$/i.test(entry.path)) paths.push(entry.path)
    for (const child of entry.children) visit(child)
  }
  for (const node of workspaceTree.value) visit(node)
  return paths
}

async function macroPayloadByPath(path: string): Promise<MacroMirrorPayload | null> {
  if (macroPayloadCache.has(path)) return macroPayloadCache.get(path) ?? null
  let payload: MacroMirrorPayload | null = null
  try {
    const file = await platform.openGraph(path)
    if (file?.content) {
      const raw: unknown = JSON.parse(file.content)
      if (isNativeGraphDocument(raw)) {
        const document = raw as GraphDocument
        const label = String(document.graphName ?? '').trim() || (path.split(/[\\/]/).pop() ?? path).replace(/\.obpm$/i, '')
        payload = {
          macroId: String(document.macroId ?? '').trim(),
          label,
          pathHint: path,
          snapshot: { nodes: document.nodes ?? [], connections: document.connections ?? [] },
          variables: (document.variables ?? []).map(item => ({ ...item }))
        }
      }
    }
  } catch { payload = null }
  macroPayloadCache.set(path, payload)
  return payload
}

async function resolveMacroById(macroId: string): Promise<MacroMirrorPayload | null> {
  const id = String(macroId ?? '').trim()
  if (!id) return null
  for (const path of workspaceMacroPaths()) {
    const payload = await macroPayloadByPath(path)
    if (payload?.macroId === id) return payload
  }
  return null
}

// 详情面板（宏引用）：打开源宏文件。pathHint 是本次会话按宏 ID 解析出的实时路径；
// 刷新是自动的（宏保存/切标签页即热更），无手动按钮。
async function openMacroSourceFromRef() {
  const info = selectedMacroRefInfo.value
  if (!info) return
  if (!info.pathHint) {
    status.value = '该引用本次未解析到来源文件（仅按宏 ID 解析），请在工作区找到对应 .obpm 打开'
    return
  }
  if (info.missing) {
    status.value = `宏 ${info.macroId} 在工作区未找到（记录的提示路径：${info.pathHint}），请检查宏文件是否在工作区内`
    return
  }
  await openGraph(info.pathHint)
}

// 宏名独立状态（同 functionTitle 模式）：保存/持久化时覆写文档 graphName，
// 否则编辑器重建文档时 graphName 会被 tab.title（文件名）覆盖。
const macroTitle = ref('')

function macroTitleFromTab(tab: GraphTab | null | undefined) {
  if (!tab || !isMacroSourcePath(tab.path || tab.title)) return ''
  return String(tab.document?.graphName ?? '').trim()
}

function syncMacroTitleToGraph() {
  const tab = activeTab.value
  if (!tab || !isMacroBlueprintTab.value) return
  const name = macroTitle.value.trim()
  if (tab.document) tab.document.graphName = name
  tab.dirty = true
  if (tab.path && name) {
    tab.title = `${name}.obpm`
    macroTitleByPath.value = { ...macroTitleByPath.value, [tab.path]: name }
    // 引用模型：改名即时刷新活动图中的宏框；其他引用图保存/打开时自动取最新宏名，无需改写磁盘。
    const macroId = tab.document?.macroId || macroIdByPath.value[tab.path] || ''
    if (macroId) editor?.refreshMacroFrames(macroId, name)
  }
}
const loadingMacroTitles = new Set<string>()

// 宏显示名与文件名分离（同函数）：优先文档 graphName，文件名兜底；未打开的文件懒加载标题。
async function loadMacroTitles(items: Array<{ path?: string }>) {
  for (const item of items) {
    if (!item.path || macroTitleByPath.value[item.path] || loadingMacroTitles.has(item.path)) continue
    loadingMacroTitles.add(item.path)
    try {
      const opened = tabs.value.find(tab => tab.path === item.path)
      let title = String(opened?.document?.graphName ?? '').trim()
      if (!title) {
        const file = await platform.openGraph(item.path)
        if (file?.content) {
          try { title = String((JSON.parse(file.content) as GraphDocument).graphName ?? '').trim() } catch { title = '' }
        }
      }
      if (title) macroTitleByPath.value = { ...macroTitleByPath.value, [item.path]: title }
    } finally {
      loadingMacroTitles.delete(item.path)
    }
  }
}

const macroModuleItems = computed<ModuleLibraryItem[]>(() => {
  const items: ModuleLibraryItem[] = []
  const visit = (entry: WorkspaceTreeNode) => {
    if (!entry.isDir && /\.obpm$/i.test(entry.path)) {
      // 工作区路径在 Windows 为反斜杠，文件名与父目录（分类）必须两种分隔符都切。
      const filename = entry.path.split(/[\\\/]/).pop() ?? entry.path
      const parent = entry.path.split(/[\\\/]/).slice(0, -1).pop() ?? ''
      const opened = tabs.value.find(tab => tab.path === entry.path)
      const title = String(opened?.document?.graphName ?? '').trim() || macroTitleByPath.value[entry.path] || filename.replace(/\.obpm$/i, '')
      items.push({
        id: `macro:${entry.path}`,
        title,
        category: `宏/${parent || '未分类'}`,
        kind: 'macro',
        macroPlaceholder: true,
        path: entry.path,
        create() { throw new Error('macro items are inserted, not created') }
      })
    }
    for (const child of entry.children) visit(child)
  }
  for (const node of workspaceTree.value) visit(node)
  return items
})
const callableFunctionItems = computed<FunctionLibraryItem[]>(() => [
  ...blueprintFunctions.value.map(item => ({ id: item.id, functionId: item.id, name: item.name, category: currentFunctionCategory(), path: activeTab.value?.path || activeTab.value?.title || '', source: 'current' as const })),
  ...functionLibraryItems.value
])
const functionModuleItems = computed<ModuleLibraryItem[]>(() => callableFunctionItems.value.map(item => ({
  id: `origin.function.${item.source}.${item.functionId || item.id}`,
  title: item.name,
  category: functionModuleCategory(item.category),
  kind: 'function',
  functionPlaceholder: true,
  functionSource: item.source,
  functionItem: item,
  path: item.path,
  create() {
    throw new Error('Function call nodes are not implemented yet')
  }
})))
const filteredModuleItems = computed(() => [
  ...nodeLibrary.value.filter(item => !isFunctionBlueprintTab.value || !item.ordinaryEntry),
  ...macroModuleItems.value
])
const functionCategoryOptions = computed(() => {
  const values = new Set<string>()
  const add = (value: unknown) => {
    const clean = String(value ?? '').trim()
    if (clean) values.add(clean)
  }
  add(functionCategory.value)
  for (const item of functionLibraryItems.value) add(item.category)
  for (const category of Object.values(functionCategoryByPath.value)) add(category)
  values.delete(defaultFunctionCategory())
  return [defaultFunctionCategory(), ...Array.from(values).sort((a, b) => a.localeCompare(b))]
})
const moduleSearchTokens = computed(() => moduleSearch.value.trim().split(/\s+/).filter(Boolean))
const categories = computed(() => {
  const ordinary = new Map<string, ModuleLibraryItem[]>()
  const functions = new Map<string, ModuleLibraryItem[]>()
  const tokens = moduleSearchTokens.value
  for (const definition of filteredModuleItems.value.filter(item => moduleItemMatchesSearch(item, tokens))) {
    const items = ordinary.get(definition.category) ?? []; items.push(definition); ordinary.set(definition.category, items)
  }
  for (const definition of functionModuleItems.value.filter(item => moduleItemMatchesSearch(item, tokens))) {
    const items = functions.get(definition.category) ?? []; items.push(definition); functions.set(definition.category, items)
  }
  return [...ordinary.entries(), ...functions.entries()].sort(([left], [right]) => functionCategoryOrder(left) - functionCategoryOrder(right))
})
const filteredDefinitions = computed(() => {
  const search = contextMenu.value.search.trim().toLowerCase()
  const items = filteredModuleItems.value
  return search ? items.filter(item => `${item.title} ${item.category}`.toLowerCase().includes(search)) : items
})
const moduleSearchActive = computed(() => Boolean(moduleSearch.value.trim()))
const menuText = computed(() => menuLocales[currentLocale.value])
const workspaceStyle = computed(() => ({
  '--file-browser-width': `${fileBrowserWidth.value}px`,
  '--left-tools-width': `${leftToolsWidth.value}px`,
  '--right-sidebar-width': `${rightSidebarWidth.value}px`
}))
const applicationClasses = computed(() => ({
  'tools-hidden': !showTools.value,
  'right-hidden': !showRight.value,
  'grid-hidden': !projectSettingsContent.value.export.showGrid,
  [`ui-scale-${projectSettingsContent.value.appearance.uiScale}`]: true,
  [`node-scale-${projectSettingsContent.value.appearance.nodeScale}`]: true,
  [`module-scale-${projectSettingsContent.value.appearance.moduleScale}`]: true
}))
const activeWorkspacePath = computed(() => activeTab.value?.path || '')
const validationIssueCountLabel = computed(() => validationIssues.value.length ? menuText.value.validation.issueCount.replace('{count}', String(validationIssues.value.length)) : menuText.value.validation.noIssues)
const groupedValidationIssues = computed(() => {
  const groups = new Map<string, { key: string; label: string; items: Array<{ issue: ValidationIssue; index: number }> }>()
  validationIssues.value.forEach((issue, index) => {
    const descriptor = validationIssueGroup(issue)
    const group = groups.get(descriptor.key) ?? { ...descriptor, items: [] }
    group.items.push({ issue, index })
    groups.set(descriptor.key, group)
  })
  return Array.from(groups.values())
})
const referencePanelStyle = computed(() => ({ height: `${referencePanelCollapsed.value ? 34 : referencePanelHeight.value}px` }))
const testPanelStyle = computed(() => ({ height: `${testPanelCollapsed.value ? 34 : testPanelHeight.value}px` }))
const collapsedPanelStyle = { flex: '0 0 30px', minHeight: '0px', overflow: 'hidden' }
const variablePanelStyle = computed(() => {
  if (variablePanelCollapsed.value) return collapsedPanelStyle
  return detailPanelCollapsed.value ? { flex: `1 1 ${variablePanelHeight.value}px` } : { flex: `0 0 ${variablePanelHeight.value}px` }
})
const detailPanelStyle = computed(() => (detailPanelCollapsed.value ? collapsedPanelStyle : undefined))
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
    onDirty() { if (activeTab.value) { activeTab.value.dirty = true; activeTab.value.saveBlocked = false } },
    onFunctionSignature(value) {
      if (isFunctionBlueprintTab.value) functionSignature.value = normalizeFunctionSignature(value)
    },
    onVariables(value) { variables.value = value },
    onVariableGroups(value) { variableGroups.value = value.length ? value : [{ id: 'default', name: 'Default' }] },
    onSelection(value) {
      selectedNode.value = value ? { ...value, values: { ...value.values } } : null
      if (value) selectedVariableId.value = null
      macroRefSelectionTick.value++
    },
    canAddEntryNodes() { return !isFunctionBlueprintTab.value },
    locale() { return currentLocale.value }
  })
  // 宏引用解析器：编辑器加载文档时按 macroId 展开宏镜像（切标签页即取磁盘最新宏）。
  editor.setMacroResolver(resolveMacroById)
  await editor.newDocument()
  if (nodeLoadStatus) status.value = nodeLoadStatus
  recentFiles.value = await platform.recentFiles()
  const initialWorkspace = (await platform.startupWorkspace()) || (await platform.currentWorkingDirectory())
  if (initialWorkspace) await loadWorkspace(initialWorkspace)
  await loadRecoverySnapshotPrompts()
  unsubscribeCloseRequest = platform.onCloseRequest(() => { void handleCloseRequest() })
  workspaceRefreshTimer = window.setInterval(() => { void refreshWorkspaceVisibleDirectories() }, workspaceRefreshIntervalMs)
  window.addEventListener('focus', refreshWorkspaceOnFocus)
  window.addEventListener('keydown', onKeyDown)
  window.addEventListener('pointerdown', closeFloatingMenus)
  window.addEventListener('beforeunload', onBeforeWindowUnload)
  if (updateState.value.autoCheck) {
    updateCheckTimer = window.setTimeout(() => { void checkForUpdates(false) }, 12000)
  }
  resetAutoSaveTimer()
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

function clampNumber(value: unknown, fallback: number, min: number, max: number) {
  const number = typeof value === 'number' ? value : Number.parseInt(String(value ?? ''), 10)
  return Number.isFinite(number) ? Math.min(max, Math.max(min, Math.round(number))) : fallback
}

function defaultProjectSettings(): ProjectSettings {
  return {
    version: 1,
    appearance: { locale: currentLocale.value, uiScale: 'normal', nodeScale: 'normal', moduleScale: 'small' },
    layout: {
      panels: {
        files: fileBrowserWidth.value,
        tools: leftToolsWidth.value,
        library: rightSidebarWidth.value,
        variables: variablePanelHeight.value,
        test: testPanelHeight.value,
        references: referencePanelHeight.value
      },
      visible: { tools: showTools.value, library: showRight.value, test: showLogger.value }
    },
    explorer: {
      expanded: Array.from(expandedWorkspacePaths.value),
      selected: selectedWorkspacePath.value,
      revealActiveFile: true,
      hideBuildFolders: false
    },
    editor: { autoSave: 'off', validateBeforeSave: false },
    export: { imageScale: 2, showGrid: true },
    configTables: {
      directories: ['configs'],
      defaults: { idMatchRow: 1, nameMatchRow: 1, idKeyword: 'id', nameKeywords: ['name', '名称'] },
      datasets: []
    }
  }
}

function normalizeProjectSettings(value: unknown): ProjectSettings {
  const source = (value && typeof value === 'object' ? value : {}) as Partial<ProjectSettings>
  const fallback = defaultProjectSettings()
  const appearance = source.appearance ?? fallback.appearance
  const layout = source.layout ?? fallback.layout
  const panels = layout.panels ?? fallback.layout.panels
  const visible = layout.visible ?? fallback.layout.visible
  const explorer = source.explorer ?? fallback.explorer
  const editorSettings = source.editor ?? fallback.editor
  const exportSettings = source.export ?? fallback.export
  const locale = normalizeLocale(appearance.locale)
  const uiScale: UiScale = appearance.uiScale === 'small' || appearance.uiScale === 'large' ? appearance.uiScale : 'normal'
  const nodeScale: NodeScale = appearance.nodeScale === 'large' ? 'large' : 'normal'
  const moduleScale: UiScale = appearance.moduleScale === 'normal' || appearance.moduleScale === 'large' ? appearance.moduleScale : 'small'
  const autoSave: AutoSaveMode = editorSettings.autoSave === '1m' || editorSettings.autoSave === '3m' || editorSettings.autoSave === '5m' ? editorSettings.autoSave : 'off'
  const imageScale: ImageExportScale = exportSettings.imageScale === 1 || exportSettings.imageScale === 4 ? exportSettings.imageScale : 2
  return {
    version: 1,
    appearance: { locale, uiScale, nodeScale, moduleScale },
    layout: {
      panels: {
        files: clampNumber(panels.files, fallback.layout.panels.files, fileBrowserMinWidth, fileBrowserMaxWidth),
        tools: clampNumber(panels.tools, fallback.layout.panels.tools, leftToolsMinWidth, leftToolsMaxWidth),
        library: clampNumber(panels.library, fallback.layout.panels.library, 160, 460),
        variables: clampNumber(panels.variables, fallback.layout.panels.variables, 130, 520),
        test: clampNumber(panels.test, fallback.layout.panels.test, 96, 360),
        references: clampNumber(panels.references, fallback.layout.panels.references, 96, 360)
      },
      visible: {
        tools: typeof visible.tools === 'boolean' ? visible.tools : fallback.layout.visible.tools,
        library: typeof visible.library === 'boolean' ? visible.library : fallback.layout.visible.library,
        test: typeof visible.test === 'boolean' ? visible.test : fallback.layout.visible.test
      }
    },
    explorer: {
      expanded: Array.isArray(explorer.expanded) ? explorer.expanded.filter(item => typeof item === 'string') : [],
      selected: typeof explorer.selected === 'string' ? explorer.selected : '',
      revealActiveFile: typeof explorer.revealActiveFile === 'boolean' ? explorer.revealActiveFile : true,
      hideBuildFolders: typeof explorer.hideBuildFolders === 'boolean' ? explorer.hideBuildFolders : false
    },
    editor: { autoSave, validateBeforeSave: Boolean(editorSettings.validateBeforeSave) },
    export: {
      imageScale,
      showGrid: typeof exportSettings.showGrid === 'boolean' ? exportSettings.showGrid : true
    },
    configTables: normalizeConfigTablesSettings(source.configTables, fallback.configTables)
  }
}

function normalizeConfigTablesSettings(value: unknown, fallback: ProjectSettings['configTables']): ProjectSettings['configTables'] {
  const source = (value && typeof value === 'object' ? value : {}) as Partial<ProjectSettings['configTables']> & { rootDirectory?: string; tables?: unknown }
  const rawDirectories = Array.isArray(source.directories)
    ? source.directories
    : (typeof source.rootDirectory === 'string' && source.rootDirectory.trim() ? [source.rootDirectory] : [])
  const directories = rawDirectories.map(item => String(item ?? '').trim()).filter(Boolean).slice(0, 16)
  const legacy = source as { matchRow?: unknown }
  const defaultsSource = (source.defaults && typeof source.defaults === 'object' ? source.defaults : {}) as Partial<ProjectSettings['configTables']['defaults']> & { matchRow?: unknown }
  const legacyMatchRow = clampNumber((defaultsSource as { matchRow?: unknown }).matchRow ?? legacy.matchRow, 0, 0, 50)
  const fallbackMatchRow = legacyMatchRow > 0 ? legacyMatchRow : fallback.defaults.idMatchRow
  const idMatchRow = clampNumber(defaultsSource.idMatchRow ?? fallbackMatchRow, fallbackMatchRow, 1, 50)
  const nameMatchRow = clampNumber(defaultsSource.nameMatchRow ?? idMatchRow, idMatchRow, 1, 50)
  const defaults = {
    idMatchRow,
    nameMatchRow,
    idKeyword: String(defaultsSource.idKeyword ?? '').trim() || fallback.defaults.idKeyword,
    nameKeywords: Array.isArray(defaultsSource.nameKeywords)
      ? defaultsSource.nameKeywords.map(item => String(item ?? '').trim()).filter(Boolean).slice(0, 8)
      : fallback.defaults.nameKeywords
  }
  // 旧版 tables 清单（一项=一张表）自动迁移为数据集：key 沿用旧表键，已有节点 ref 不受影响。
  const datasetSource = Array.isArray((source as { datasets?: unknown }).datasets)
    ? (source as { datasets: unknown[] }).datasets
    : (Array.isArray(source.tables) ? source.tables : [])
  const datasets = datasetSource.map(item => {
    const dataset = (item && typeof item === 'object' ? item : {}) as Partial<ConfigTableDatasetSetting> & { matchRow?: unknown }
    const legacyMatchRow = clampNumber((dataset as { matchRow?: unknown }).matchRow, 0, 0, 50)
    const key = String(dataset.key ?? '').trim()
    return {
      key,
      name: String(dataset.name ?? '').trim() || key,
      file: String(dataset.file ?? '').trim(),
      sheet: String(dataset.sheet ?? '').trim(),
      idMatchRow: clampNumber(dataset.idMatchRow ?? legacyMatchRow, 0, 0, 50),
      nameMatchRow: clampNumber(dataset.nameMatchRow ?? legacyMatchRow, 0, 0, 50),
      idKeyword: String(dataset.idKeyword ?? '').trim(),
      nameKeyword: String(dataset.nameKeyword ?? '').trim(),
      filter: normalizeConfigTableFilter(dataset.filter)
    }
  }).filter(dataset => dataset.key || dataset.file)
  return { directories: directories.length ? directories : fallback.directories, defaults, datasets }
}

function normalizeConfigTableFilter(value: unknown): ConfigTableFilterSetting {
  const source = (value && typeof value === 'object' ? value : {}) as Partial<ConfigTableFilterSetting>
  return {
    mode: source.mode === 'range' || source.mode === 'compare' ? source.mode : 'text',
    column: source.column === 'id' || source.column === 'name' || source.column === 'custom' ? source.column : 'all',
    text: String(source.text ?? ''),
    rangeMin: String(source.rangeMin ?? ''),
    rangeMax: String(source.rangeMax ?? ''),
    compareOp: ['>', '>=', '<', '<='].includes(String(source.compareOp)) ? String(source.compareOp) : '>',
    compareValue: String(source.compareValue ?? ''),
    extraMatchRow: clampNumber(source.extraMatchRow, 0, 0, 50),
    extraKeyword: String(source.extraKeyword ?? '').trim()
  }
}

function currentProjectSettings() {
  const current = normalizeProjectSettings(projectSettingsContent.value)
  current.appearance.locale = currentLocale.value
  current.layout.panels = {
    files: fileBrowserWidth.value,
    tools: leftToolsWidth.value,
    library: rightSidebarWidth.value,
    variables: variablePanelHeight.value,
    test: testPanelHeight.value,
    references: referencePanelHeight.value
  }
  current.layout.visible = { tools: showTools.value, library: showRight.value, test: showLogger.value }
  current.explorer.expanded = Array.from(expandedWorkspacePaths.value)
  current.explorer.selected = selectedWorkspacePath.value
  current.configTables = projectSettingsContent.value.configTables
  return current
}

function applyProjectSettings(settings: ProjectSettings) {
  applyingProjectSettings = true
  projectSettingsContent.value = normalizeProjectSettings(settings)
  const current = projectSettingsContent.value
  currentLocale.value = current.appearance.locale
  fileBrowserWidth.value = current.layout.panels.files
  leftToolsWidth.value = current.layout.panels.tools
  rightSidebarWidth.value = current.layout.panels.library
  variablePanelHeight.value = current.layout.panels.variables
  testPanelHeight.value = current.layout.panels.test
  referencePanelHeight.value = current.layout.panels.references
  showTools.value = current.layout.visible.tools
  showRight.value = current.layout.visible.library
  showLogger.value = current.layout.visible.test
  expandedWorkspacePaths.value = new Set(current.explorer.expanded)
  selectedWorkspacePath.value = current.explorer.selected
  applyingProjectSettings = false
}

async function loadProjectSettings(root: string) {
  const result = await platform.loadProjectSettings(root)
  if (!result?.content) return
  projectSettingsPath.value = result.path
  try {
    applyProjectSettings(normalizeProjectSettings(JSON.parse(result.content)))
  } catch {
    applyProjectSettings(defaultProjectSettings())
  }
}

async function saveProjectSettings() {
  if (!workspaceRoot.value || applyingProjectSettings) return
  const settings = currentProjectSettings()
  projectSettingsContent.value = settings
  const content = JSON.stringify(settings, null, 2)
  try {
    projectSettingsPath.value = await platform.saveProjectSettings(workspaceRoot.value, content)
  } catch (error) {
    status.value = `Project settings save failed: ${error instanceof Error ? error.message : String(error)}`
  }
}

function setLocale(locale: LocaleId) {
  currentLocale.value = locale
  localStorage.setItem('origin-blueprint-locale', locale)
  projectSettingsContent.value.appearance.locale = locale
	void loadRuntimeNodeLibrary()
	void syncCallableFunctionsToEditor()
  void saveProjectSettings()
}

function updateProjectSettings(mutator: (settings: ProjectSettings) => void) {
  const settings = currentProjectSettings()
  mutator(settings)
  applyProjectSettings(settings)
  void saveProjectSettings()
}

function setAutoCheckUpdates(enabled: boolean) {
  updateState.value.autoCheck = enabled
  localStorage.setItem('origin-blueprint-auto-check-updates', enabled ? 'true' : 'false')
  if (!enabled && updateCheckTimer) {
    window.clearTimeout(updateCheckTimer)
    updateCheckTimer = undefined
  }
  if (enabled) void checkForUpdates(false)
}

function normalizeVersion(value: string) {
  return value.trim().replace(/^[^\d]*/, '').split(/[.+-]/).map(part => Number.parseInt(part, 10) || 0)
}

function compareVersions(left: string, right: string) {
  const a = normalizeVersion(left)
  const b = normalizeVersion(right)
  const length = Math.max(a.length, b.length, 3)
  for (let index = 0; index < length; index += 1) {
    const delta = (a[index] ?? 0) - (b[index] ?? 0)
    if (delta !== 0) return delta
  }
  return 0
}

async function checkForUpdates(manual = true) {
  if (updateState.value.checking) return
  updateState.value.checking = true
  updateState.value.error = ''
  if (manual) status.value = menuText.value.update.checking
  try {
    const response = await fetch(updateCheckUrl, {
      headers: { Accept: 'application/vnd.github+json' },
      cache: 'no-store'
    })
    if (response.status === 404) {
      if (manual) status.value = menuText.value.update.noRelease
      return
    }
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const release = await response.json() as { tag_name?: string; name?: string; html_url?: string; body?: string; prerelease?: boolean }
    const latestVersion = release.tag_name || release.name || ''
    if (!latestVersion) throw new Error('Missing release version')
    if (compareVersions(latestVersion, appVersion) > 0) {
      updateState.value.visible = true
      updateState.value.latestVersion = latestVersion
      updateState.value.currentVersion = appVersion
      updateState.value.htmlUrl = release.html_url || releasePageUrl
      updateState.value.notes = (release.body || '').trim().slice(0, 800)
      status.value = menuText.value.update.available.replace('{version}', latestVersion)
    } else if (manual) {
      status.value = menuText.value.update.upToDate
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    updateState.value.error = message
    if (manual) status.value = `${menuText.value.update.checkFailed}: ${message}`
  } finally {
    updateState.value.checking = false
  }
}

function closeUpdateDialog() {
  updateState.value.visible = false
}

async function openUpdateRelease() {
  await platform.openExternalURL(updateState.value.htmlUrl || releasePageUrl)
  closeUpdateDialog()
}

async function loadRuntimeNodeLibrary(requestedWorkspace = workspaceRoot.value) {
  const token = ++nodeSchemaLoadToken
  let result
  try {
    result = await platform.loadNodeSchemas(requestedWorkspace)
  } catch (error) {
    if (token !== nodeSchemaLoadToken) return ''
    return `Node library load failed: ${error instanceof Error ? error.message : String(error)}`
  }
  if (token !== nodeSchemaLoadToken) return ''
  nodeSchemaDocuments.value = result.documents ?? []
  if (result.nodes.length) {
    registerNodeSchemas(result.nodes, currentLocale.value)
    nodeLibrary.value = getNodeDefinitions()
  }
  if (result.errors.length) return `Loaded ${result.nodes.length} node template(s), ${result.errors.length} JSON error(s)`
  if (result.nodes.length) return `Loaded ${result.nodes.length} node template(s) from nodes`
  if (!result.documentCount) return 'No node JSON files found in nodes directory'
  return ''
}

onBeforeUnmount(() => {
  clearValidationIssueClickTimer()
  if (canvasToastTimer) window.clearTimeout(canvasToastTimer)
  if (workspaceRefreshTimer) window.clearInterval(workspaceRefreshTimer)
  if (autoSaveTimer) window.clearInterval(autoSaveTimer)
  if (updateCheckTimer) window.clearTimeout(updateCheckTimer)
  removeNodePointerListeners()
  unsubscribeCloseRequest()
  window.removeEventListener('focus', refreshWorkspaceOnFocus)
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
  const ctrl = event.ctrlKey || event.metaKey
  const key = event.key.toLowerCase()
  if (ctrl && event.shiftKey && key === 'q') run(() => revealCurrentFileInFolder(), event)
  else if (target.matches('input, textarea, select')) return
  else if (ctrl && event.shiftKey && key === 'n') run(() => platform.newWindow(), event)
  else if (ctrl && key === 'n') run(newGraph, event)
  else if (ctrl && key === 'o') run(() => openGraph(), event)
  else if (ctrl && event.altKey && key === 's') run(saveAll, event)
  else if (ctrl && key === 's' && event.shiftKey) run(() => saveGraph(true), event)
  else if (ctrl && key === 's') run(() => saveGraph(false), event)
  else if (ctrl && event.altKey && key === 'r') run(() => exportImage(true), event)
  else if (ctrl && event.shiftKey && key === 'r') run(() => exportImage(false), event)
  else if (ctrl && key === 'a') run(() => editor?.selectAll(), event)
  else if (ctrl && key === 'd') run(() => editor?.deselectAll(), event)
  else if (ctrl && key === 'c') run(() => editor?.copy(), event)
  else if (ctrl && key === 'x') run(() => editor?.cut(), event)
  else if (ctrl && key === 'v') run(() => editor?.paste(), event)
  else if (ctrl && key === 'z') run(() => editor?.undo(), event)
  else if (ctrl && key === 'y') run(() => editor?.redo(), event)
  else if (ctrl && key === 'g') run(() => editor?.toggleGroupSelected(), event)
  else if (event.key === 'Home') run(() => editor?.resetView(), event)
  else if (key === 'c') run(() => editor?.commentAroundSelection(), event)
  else if (ctrl && key === 'f') { canvasSearchVisible.value = true; event.preventDefault(); requestAnimationFrame(() => canvasSearchRef.value?.focus()) }
  else if (event.key === 'F5') run(testGraph, event)
  else if (event.altKey && event.shiftKey && key === 'b') { showLogger.value = !showLogger.value; event.preventDefault() }
  else if (event.altKey && event.shiftKey && key === 'l') { showTools.value = !showTools.value; event.preventDefault() }
  else if (event.altKey && event.shiftKey && key === 'r') { showRight.value = !showRight.value; event.preventDefault() }
  else if (event.shiftKey && key === 'l') run(() => editor?.align('left'), event)
  else if (event.shiftKey && key === 'r') run(() => editor?.align('right'), event)
  else if (event.shiftKey && key === 't') run(() => editor?.align('top'), event)
  else if (event.shiftKey && key === 'b') run(() => editor?.align('bottom'), event)
  else if (event.shiftKey && key === 'h') run(() => editor?.align('horizontal-distribute'), event)
  else if (event.shiftKey && key === 'v') run(() => editor?.align('vertical-distribute'), event)
  else if (key === 'h') run(() => editor?.align('horizontal-center'), event)
  else if (key === 'v') run(() => editor?.align('vertical-center'), event)
  else if (event.key === 'Delete' || key === 'x') run(() => editor?.deleteSelected(), event)
}

function run(action: () => void | Promise<void>, event?: Event) { event?.preventDefault(); activeMenu.value = null; void action() }
function toggleMenu(name: string) { activeMenu.value = activeMenu.value === name ? null : name }
function persistActive() { if (editor && activeTab.value) activeTab.value.document = documentWithFunctionSignature(editor.getDocument(activeTab.value.title, variables.value, variableGroups.value)) }

async function newGraph() {
  persistActive(); untitledCount++
  const tab: GraphTab = { id: crypto.randomUUID(), title: `Untitled-${untitledCount} Graph`, path: '', dirty: false, document: null }
  tabs.value.push(tab); activeTabId.value = tab.id; selectedVariableId.value = null; functionSignature.value = emptyFunctionSignature(); functionTitle.value = ''; functionId.value = ''; functionCategory.value = ''; functionDescription.value = ''; await editor?.newDocument()
}

async function switchTab(id: string) {
  if (id === activeTabId.value) return
  persistActive(); activeTabId.value = id; selectedVariableId.value = null
  const tab = activeTab.value
  if (tab.document) {
    await syncCallableFunctionsToEditor()
    try { await editor?.loadDocument(tab.document) }
    catch (error) {
      tab.restoreFatal = true
      status.value = `Graph restore failed: ${error instanceof Error ? error.message : String(error)}`
    }
  } else await editor?.newDocument()
  functionSignature.value = normalizeFunctionSignature(tab.document?.functionSignature)
  functionTitle.value = isFunctionBlueprintPath(tab.path || tab.title) ? functionTitleFromDocument(tab.document, tab.path || tab.title, tab.title) : ''
  macroTitle.value = macroTitleFromTab(tab)
  functionId.value = isFunctionBlueprintPath(tab.path || tab.title) ? functionIdFromDocument(tab.document) : ''
  functionCategory.value = isFunctionBlueprintPath(tab.path || tab.title) ? functionCategoryFromDocument(tab.document, tab.path || tab.title) : ''
  functionDescription.value = isFunctionBlueprintPath(tab.path || tab.title) ? String(tab.document?.functionDescription ?? '') : ''
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
    macroTitle.value = macroTitleFromTab(tabs.value[0])
    functionId.value = isFunctionBlueprintPath(tabs.value[0].path || tabs.value[0].title) ? functionIdFromDocument(tabs.value[0].document) : ''
    functionCategory.value = isFunctionBlueprintPath(tabs.value[0].path || tabs.value[0].title) ? functionCategoryFromDocument(tabs.value[0].document, tabs.value[0].path || tabs.value[0].title) : ''
    functionDescription.value = isFunctionBlueprintPath(tabs.value[0].path || tabs.value[0].title) ? String(tabs.value[0].document?.functionDescription ?? '') : ''
    await syncCallableFunctionsToEditor(); await editor?.loadDocument(tabs.value[0].document ?? blankDocument(tabs.value[0].title))
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
  return { schemaVersion: 1, graphName: name, nodes: [], connections: [], groups: [], comments: [], macroRefs: [], variables: [], variableGroups: [{ id: 'default', name: 'Default' }], view: { x: 0, y: 0, zoom: 1 } }
}

function newFunctionId() {
  return `fn_${crypto.randomUUID().replace(/-/g, '').slice(0, 12)}`
}

function functionIdFromDocument(document: GraphDocument | null | undefined) {
  return String(document?.functionId ?? '').trim()
}

function activeFunctionId() {
  if (!functionId.value.trim()) functionId.value = newFunctionId()
  return functionId.value.trim()
}

function functionTerminalNodes(name: string, signature = emptyFunctionSignature(), id = newFunctionId()) {
  const entryId = crypto.randomUUID()
  const returnId = crypto.randomUUID()
  const metadata = (role: FunctionNodeMetadata['functionRole']) => ({
    functionRole: role,
    functionId: id,
    functionName: name,
    functionSource: 'workspace' as const,
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
      type: normalizeFunctionSignaturePortType(item.type),
      description: item.description ? String(item.description) : undefined
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

function defaultFunctionCategory() {
  return menuText.value.module.functionCategory
}

function normalizeFunctionCategory(value: unknown, fallback = defaultFunctionCategory()) {
  const clean = String(value ?? '').trim()
  return clean || fallback
}

function inferredFunctionCategoryFromPath(path: string) {
  const parts = path.replace(/\\/g, '/').split('/').filter(Boolean)
  const fileName = parts.pop() ?? ''
  const parent = parts[parts.length - 1] ?? ''
  if (!fileName || !parent || /^functions?$/i.test(parent)) return defaultFunctionCategory()
  return parent
}

function functionCategoryFromDocument(document: GraphDocument | Partial<GraphDocument> | null | undefined, path: string) {
  return normalizeFunctionCategory(document?.functionCategory, inferredFunctionCategoryFromPath(path))
}

function currentFunctionCategory() {
  return normalizeFunctionCategory(functionCategory.value)
}

function activeFunctionCategory() {
  functionCategory.value = currentFunctionCategory()
  return functionCategory.value
}

function functionModuleCategory(category: string) {
  return `ƒ ${normalizeFunctionCategory(category)}`
}

function functionCategoryOrder(category: string) {
  return category.startsWith('ƒ ') ? 1 : 0
}

function isFunctionModuleCategory(items: ModuleLibraryItem[]) {
  return items.some(item => item.functionPlaceholder)
}

function displayModuleCategoryName(category: string) {
  return category.startsWith('ƒ ') ? category.slice(2) : category
}

function compactModuleSearchText(value: string) {
  return value.toLowerCase().replace(/[\s_\-./\\()[\]{}:;'"`]+/g, '')
}

function moduleSearchText(value: unknown) {
  const text = String(value ?? '').toLowerCase()
  return `${text} ${compactModuleSearchText(text)}`
}

function moduleSearchFieldsMatch(fields: unknown[], tokens: string[]) {
  if (!tokens.length) return true
  const corpus = fields.map(moduleSearchText).join(' ')
  return tokens.every(token => {
    const raw = token.toLowerCase()
    const compact = compactModuleSearchText(raw)
    return Boolean(raw && (corpus.includes(raw) || (compact && corpus.includes(compact))))
  })
}

function moduleItemSearchFields(item: ModuleLibraryItem) {
  if (item.functionPlaceholder) return [item.title, item.functionItem?.category]
  return [item.title, item.category, item.description]
}

function moduleItemMatchesSearch(item: ModuleLibraryItem, tokens = moduleSearchTokens.value) {
  return moduleSearchFieldsMatch(moduleItemSearchFields(item), tokens)
}

function escapeHtml(value: string) {
  return value.replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char] ?? char))
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function renderModuleSearchText(value: unknown) {
  const text = String(value ?? '')
  const tokens = moduleSearchTokens.value
  if (!tokens.length || !text) return escapeHtml(text)
  const ranges: Array<[number, number]> = []
  const lower = text.toLowerCase()
  for (const token of tokens) {
    const raw = token.toLowerCase()
    if (!raw) continue
    const pattern = new RegExp(escapeRegExp(raw), 'g')
    for (const match of lower.matchAll(pattern)) {
      const start = match.index ?? -1
      if (start >= 0) ranges.push([start, start + raw.length])
    }
  }
  if (!ranges.length && moduleSearchFieldsMatch([text], tokens)) return `<mark class="module-search-highlight">${escapeHtml(text)}</mark>`
  if (!ranges.length) return escapeHtml(text)
  ranges.sort((left, right) => left[0] - right[0])
  const merged: Array<[number, number]> = []
  for (const range of ranges) {
    const previous = merged[merged.length - 1]
    if (previous && range[0] <= previous[1]) previous[1] = Math.max(previous[1], range[1])
    else merged.push([...range])
  }
  let cursor = 0
  let html = ''
  for (const [start, end] of merged) {
    html += escapeHtml(text.slice(cursor, start))
    html += `<mark class="module-search-highlight">${escapeHtml(text.slice(start, end))}</mark>`
    cursor = end
  }
  return html + escapeHtml(text.slice(cursor))
}

function openFunctionCategoryOptions() {
  if (isFunctionBlueprintTab.value) functionCategoryDropdownOpen.value = true
}

function closeFunctionCategoryOptions(event: FocusEvent) {
  const next = event.relatedTarget as Node | null
  const current = event.currentTarget as Node | null
  if (current && next && current.contains(next)) return
  functionCategoryDropdownOpen.value = false
}

function selectFunctionCategory(category: string) {
  functionCategory.value = normalizeFunctionCategory(category)
  functionCategoryDropdownOpen.value = false
  syncFunctionCategoryToGraph()
}

function documentWithFunctionSignature(document: GraphDocument, tab = activeTab.value) {
  const path = tab.path || tab.title
  if (isMacroSourcePath(path)) {
    const name = macroTitle.value.trim()
    // 宏身份（macroId）是引用图的稳定锚点：保存时缺失则补发一次并记住，之后保持稳定。
    let macroId = String(document.macroId ?? '').trim() || macroIdByPath.value[path] || ''
    if (!macroId) {
      macroId = crypto.randomUUID()
      macroIdByPath.value = { ...macroIdByPath.value, [path]: macroId }
    }
    const next: GraphDocument = { ...document, macroId }
    if (name) next.graphName = name
    return next
  }
  if (!isFunctionBlueprintPath(path)) return document
  return applyFunctionPersistenceMetadata(path, document, {
    graphName: activeFunctionTitle(),
    functionId: activeFunctionId(),
    functionCategory: activeFunctionCategory(),
    functionSignature: normalizeFunctionSignature(functionSignature.value),
    functionDescription: functionDescription.value,
  })
}

function hasFunctionNodes(document: GraphDocument) {
  return (document.nodes ?? []).some(node => String(node.typeId ?? '').startsWith('origin.function.') || node.typeId === 'origin.timer.set-by-function')
}

function documentRequiresNativePersistence(document: GraphDocument) {
  const hasNativeTimeNodes = (document.nodes ?? []).some(node =>
    node.typeId === 'origin.flow.delay' || String(node.typeId ?? '').startsWith('origin.timer.'),
  )
  const hasTimerHandleVariables = (document.variables ?? []).some(variable => variable.type === 'timerhandle')
  const hasInstanceVariables = (document.variables ?? []).some(variable => variableScope(variable) === 'instance')
  return hasFunctionNodes(document)
    || hasNativeTimeNodes
    || hasTimerHandleVariables
    || hasInstanceVariables
    || (document.macroRefs?.length ?? 0) > 0
    || graphDocumentRequiresNativePersistence(document)
}

function functionPortKey(prefix: 'input' | 'output', port: FunctionSignaturePort, index: number) {
  const key = String(port.id || port.name || `${index + 1}`).trim().replace(/[^a-zA-Z0-9_-]+/g, '-').replace(/^-+|-+$/g, '')
  return `${prefix}_${key || index + 1}`
}

function functionNodePorts(node: NodeSnapshot) {
  const signature = normalizeFunctionSignature(node.properties?.functionSignature)
  const inputs = new Map<string, string>()
  const outputs = new Map<string, string>()
  if (node.typeId === 'origin.function.entry') {
    outputs.set('exec', 'exec')
    signature.inputs.forEach((port, index) => outputs.set(functionPortKey('input', port, index), port.type))
  } else if (node.typeId === 'origin.function.return') {
    inputs.set('exec', 'exec')
    signature.outputs.forEach((port, index) => inputs.set(functionPortKey('output', port, index), port.type))
  } else if (node.typeId === 'origin.function.call') {
    inputs.set('exec', 'exec')
    outputs.set('exec', 'exec')
    signature.inputs.forEach((port, index) => inputs.set(functionPortKey('input', port, index), port.type))
    signature.outputs.forEach((port, index) => outputs.set(functionPortKey('output', port, index), port.type))
  } else if (node.typeId === 'origin.timer.set-by-function') {
    inputs.set('exec', 'exec'); inputs.set('time', 'integer'); inputs.set('looping', 'boolean'); inputs.set('firstDelay', 'integer')
    outputs.set('then', 'exec'); outputs.set('timerHandle', 'timerhandle')
    signature.inputs.forEach((port, index) => inputs.set(functionPortKey('input', port, index), port.type))
  }
  return { inputs, outputs }
}

type FunctionPortChange = { previous: ReturnType<typeof functionNodePorts>; next: ReturnType<typeof functionNodePorts> }

function pruneChangedFunctionConnections(document: GraphDocument, changes: Map<string, FunctionPortChange>) {
  document.connections = (document.connections ?? []).filter(connection => {
    const source = changes.get(connection.source)
    if (source) {
      const nextType = source.next.outputs.get(connection.sourceOutput)
      if (!nextType || (source.previous.outputs.get(connection.sourceOutput) && source.previous.outputs.get(connection.sourceOutput) !== nextType)) return false
    }
    const target = changes.get(connection.target)
    if (target) {
      const nextType = target.next.inputs.get(connection.targetInput)
      if (!nextType || (target.previous.inputs.get(connection.targetInput) && target.previous.inputs.get(connection.targetInput) !== nextType)) return false
    }
    return true
  })
}

function sameFunctionReference(properties: NodeSnapshot['properties'] | undefined, metadata: FunctionNodeMetadata) {
  if (!properties) return false
  return Boolean(metadata.functionId && properties.functionId === metadata.functionId)
}

function syncDocumentFunctionReferences(document: GraphDocument, metadata: FunctionNodeMetadata) {
  const signature = normalizeFunctionSignature(metadata.functionSignature)
  const updatedPorts = new Map<string, FunctionPortChange>()
  for (const node of document.nodes ?? []) {
    if ((node.typeId !== 'origin.function.call' && node.typeId !== 'origin.timer.set-by-function') || !sameFunctionReference(node.properties, metadata)) continue
    const previous = functionNodePorts(node)
    node.properties = {
      ...node.properties,
      label: metadata.functionName,
      functionRole: node.typeId === 'origin.timer.set-by-function' ? 'timer' : 'call',
      functionId: metadata.functionId,
      functionName: metadata.functionName,
      functionSource: metadata.functionSource,
      functionSignature: signature
    }
    updatedPorts.set(node.id, { previous, next: functionNodePorts(node) })
  }
  if (!updatedPorts.size) return false
  pruneChangedFunctionConnections(document, updatedPorts)
  return true
}

function syncFunctionTerminalsFromDocumentSignature(document: GraphDocument, path: string) {
  const signature = normalizeFunctionSignature(document.functionSignature)
  const functionName = functionTitleFromDocument(document, path, document.graphName)
  const id = String(document.functionId ?? '').trim()
  if (!id) return false
  const changedPorts = new Map<string, FunctionPortChange>()
  for (const node of document.nodes ?? []) {
    if (node.typeId !== 'origin.function.entry' && node.typeId !== 'origin.function.return') continue
    const previous = functionNodePorts(node)
    const role = node.typeId === 'origin.function.entry' ? 'entry' : 'return'
    node.properties = {
      ...node.properties,
      label: role === 'entry' ? `${functionName} Entry` : `${functionName} Return`,
      functionRole: role,
      functionId: id,
      functionName,
      functionSource: 'workspace',
      functionSignature: signature
    }
    changedPorts.set(node.id, { previous, next: functionNodePorts(node) })
  }
  if (!changedPorts.size) return false
  pruneChangedFunctionConnections(document, changedPorts)
  return true
}

function functionLibraryItemById(id: string) {
  const cleanId = id.trim()
  if (!cleanId) return undefined
  return functionLibraryItems.value.find(item => item.functionId === cleanId)
}

async function loadFunctionSignatureForId(id: string) {
  const item = functionLibraryItemById(id)
  if (!item?.path) return emptyFunctionSignature()
  const opened = tabs.value.find(tab => tab.path === item.path)
  if (opened?.document) return normalizeFunctionSignature(opened.document.functionSignature)
  try {
    const file = await platform.openGraph(item.path)
    if (!file) return emptyFunctionSignature()
    const parsed = parseGraphJSON(file.content) as Partial<GraphDocument>
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

  const functionCalls = new Set<string>()
  for (const node of document.nodes ?? []) {
    const id = String(node.properties?.functionId ?? '').trim()
    if ((node.typeId !== 'origin.function.call' && node.typeId !== 'origin.timer.set-by-function') || !id || id === document.functionId) continue
    functionCalls.add(id)
  }
  for (const id of functionCalls) {
    const item = functionLibraryItemById(id)
    if (!item) continue
    const signature = await loadFunctionSignatureForId(id)
    const metadata: FunctionNodeMetadata = {
      functionRole: 'call',
      functionId: id,
      functionName: item.name,
      functionSource: 'workspace',
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
	if (type === 'timerhandle') return null
  return ''
}

async function syncVariables(refreshNodes = false) {
  await editor?.setVariables(variables.value, variableGroups.value, refreshNodes)
  activeTab.value.dirty = true
}

async function addVariable(groupId = 'default', scope: VariableScope = 'execution') {
  if (scope === 'instance' && isFunctionBlueprintTab.value) {
    status.value = '函数蓝图中的变量只能使用局部作用域'
    return
  }
  let index = variables.value.length + 1
  while (variables.value.some(item => item.name === `Variable${index}`)) index++
  const variable: GraphVariable = {
    id: crypto.randomUUID(),
    name: `Variable${index}`,
    type: 'integer',
    defaultValue: 0,
    groupId,
    ...(scope === 'instance' ? { scope: 'instance' as const } : {})
  }
  variables.value.push(variable)
  await syncVariables()
  await selectVariable(variable)
}

async function updateVariable(variable: GraphVariable, previousType?: VariableType) {
  variable.name = variable.name.trim() || 'Variable'
  if (variable.type === 'integer' && !isValidIntegerDefault(variable.defaultValue)) {
    status.value = 'Integer 默认值必须是有效的 64 位整数，不能包含小数或超出 int64 范围'
    return
  }
  if (previousType && previousType !== variable.type) variable.defaultValue = defaultVariableValue(variable.type)
  await syncVariables(true)
}

function previewVariableName(variable: GraphVariable) {
  void editor?.refreshVariableNodePresentation(variable)
}

async function changeVariableType(variable: GraphVariable) {
  variable.defaultValue = defaultVariableValue(variable.type)
  await updateVariable(variable)
}

async function changeVariableScope(variable: GraphVariable, event: Event) {
  const select = event.target as HTMLSelectElement
  const previousScope = variableScope(variable)
  const scope = select.value as VariableScope
  const targetGroupId = matchingVariableGroupId(variableGroups.value, variable.groupId, scope)
  const plan = planVariableGroupDrop(variableGroups.value, variable, targetGroupId, scope, isFunctionBlueprintTab.value)
  if (!await commitVariableGroupDrop(variable, plan)) select.value = previousScope
}

function variableScopeTitle(scope: VariableScope) {
  return scope === 'instance' ? '全局变量' : '局部变量'
}

function variableDropGroupName(groupId: string) {
  return groupId === 'default' ? 'Default' : variableGroups.value.find(group => group.id === groupId)?.name ?? groupId
}

function variableScopeChangeConfirmation(variable: GraphVariable, plan: VariableGroupDropPlan) {
  const sourceScope = variableScope(variable)
  const targetTitle = variableScopeTitle(plan.targetScope)
  const consequence = plan.targetScope === 'instance'
    ? '之后同一蓝图实例的并发执行和后续执行将共享此变量。'
    : '之后每次执行都会从默认值重新初始化，当前实例共享值不会迁移为局部值。'
  const legacyWarning = plan.targetScope === 'instance' && isLegacyGraphPath(activeTab.value?.path || activeTab.value?.title || '')
    ? '\n\n当前文件是 .vgf，旧格式不能保存全局变量；确认后必须另存为 .obp。'
    : ''
  return `将变量“${variable.name}”从${variableScopeTitle(sourceScope)}改为${targetTitle}，并移动到“${variableDropGroupName(plan.targetGroupId)}”。\n\n${consequence}${legacyWarning}`
}

async function commitVariableGroupDrop(variable: GraphVariable, plan: VariableGroupDropPlan) {
  if (plan.kind === 'forbidden') {
    status.value = plan.reason === 'function-instance-scope'
      ? '函数蓝图中的变量只能使用局部作用域'
      : '目标分组与变量作用域不匹配'
    return false
  }
  if (plan.kind === 'none') return false
  if (plan.kind === 'scope-change') {
    if (variable.type === 'integer' && !isValidIntegerDefault(variable.defaultValue)) {
      status.value = 'Integer 默认值必须是有效的 64 位整数，不能包含小数或超出 int64 范围'
      return false
    }
    if (!window.confirm(variableScopeChangeConfirmation(variable, plan))) return false
  }
  const sourceScope = variableScope(variable)
  if (!applyVariableGroupDrop(variable, plan)) return false
  await syncVariables(plan.kind === 'scope-change')
  status.value = plan.kind === 'scope-change'
    ? `已将 ${variable.name} 改为${variableScopeTitle(plan.targetScope)}并移动到 ${variableDropGroupName(plan.targetGroupId)}`
    : `已将 ${variable.name} 移动到 ${variableDropGroupName(plan.targetGroupId)}`
  if (sourceScope === 'execution' && plan.targetScope === 'instance' && isLegacyGraphPath(activeTab.value?.path || activeTab.value?.title || '')) {
    status.value += '；保存时必须另存为 .obp'
  }
  return true
}

async function setVariableArrayDefault(variable: GraphVariable, event: Event) {
  const text = (event.target as HTMLInputElement).value
  variable.defaultValue = text.split(',').map(item => item.trim()).filter(Boolean).map(item => {
    if (/^[+-]?\d+$/.test(item)) return normalizeIntegerInput(item)
    return /^[+-]?(?:\d+\.\d*|\d*\.\d+)(?:[eE][+-]?\d+)?$/.test(item) ? Number(item) : item
  })
  await updateVariable(variable)
}

function setVariableIntegerDefault(variable: GraphVariable, event: Event) {
  variable.defaultValue = normalizeIntegerInput((event.target as HTMLInputElement).value)
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
  const saved = await platform.saveGraph(path, serializeGraphDocument(path, blankDocument(graphName), 2))
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
  document.functionId = newFunctionId()
  document.functionCategory = inferredFunctionCategoryFromPath(path)
  document.functionSignature = emptyFunctionSignature()
  const terminals = functionTerminalNodes(graphName, document.functionSignature, document.functionId)
  document.nodes = terminals.nodes
  document.connections = terminals.connections
  const saved = await platform.saveGraph(path, serializeGraphDocument(path, document, 2))
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

async function addVariableGroup(scope: VariableScope) {
  if (scope === 'instance' && isFunctionBlueprintTab.value) {
    status.value = '函数蓝图不支持全局变量分组'
    return
  }
  const rawName = window.prompt('变量分组名称', '新分组')
  const name = rawName?.trim()
  if (!name) return
  if (variableGroupNameExists(variableGroups.value, name, scope)) {
    status.value = `Variable group already exists: ${name}`
    return
  }
  variableGroups.value.push({ id: crypto.randomUUID(), name, scope })
  await syncVariables()
}

async function renameVariableGroup(group: GraphVariableGroup) {
  if (group.id === 'default') return
  const rawName = window.prompt('重命名变量分组', group.name)
  const name = rawName?.trim()
  if (!name || name === group.name) return
  const scope = variableGroupScope(group) ?? 'execution'
  if (variableGroupNameExists(variableGroups.value, name, scope, group.id)) {
    status.value = `Variable group already exists: ${name}`
    return
  }
  group.name = name
  await syncVariables()
}

async function removeVariableGroup(group: GraphVariableGroup) {
  if (group.id === 'default') return
  const usage = variableGroupUsage(variables.value, group.id)
  const scope = variableGroupScope(group) ?? 'execution'
  const count = scope === 'instance' ? usage.globalCount : usage.localCount
  if (count && !window.confirm(variableGroupRemovalMessage(group.name, scope, count))) return
  moveVariablesToDefaultGroup(variables.value, group.id)
  variableGroups.value = variableGroups.value.filter(item => item.id !== group.id)
  await syncVariables()
}

function startVariableDrag(event: DragEvent, variable: GraphVariable) {
  variableDragId.value = variable.id
  variableDropIndicator.value = null
  event.dataTransfer?.setData('application/x-origin-variable', variable.id)
  event.dataTransfer?.setData('application/x-origin-variable-access', event.altKey ? 'set' : 'get')
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'copyMove'
}

function endVariableDrag() {
  variableDragId.value = ''
  variableDropIndicator.value = null
}

function variableDropLabel(plan: VariableGroupDropPlan) {
  if (plan.kind === 'forbidden') return plan.reason === 'function-instance-scope' ? '函数蓝图不支持全局变量' : '不能移动到此分组'
  if (plan.kind === 'scope-change') return `变更为${variableScopeTitle(plan.targetScope)}并移动到 ${variableDropGroupName(plan.targetGroupId)}`
  if (plan.kind === 'move') return `移动到 ${variableDropGroupName(plan.targetGroupId)}`
  return ''
}

function showVariableGroupDrop(event: DragEvent, key: string, groupId: string, scope: VariableScope) {
  const variable = variables.value.find(item => item.id === variableDragId.value)
  if (!variable) return
  event.preventDefault()
  event.stopPropagation()
  const plan = planVariableGroupDrop(variableGroups.value, variable, groupId, scope, isFunctionBlueprintTab.value)
  const label = variableDropLabel(plan)
  variableDropIndicator.value = label ? { key, plan, label } : null
  if (event.dataTransfer) event.dataTransfer.dropEffect = plan.kind === 'move' || plan.kind === 'scope-change' ? 'move' : 'none'
}

function leaveVariableGroupDrop(event: DragEvent, key: string) {
  const current = event.currentTarget as HTMLElement | null
  const related = event.relatedTarget as Node | null
  if (current && related && current.contains(related)) return
  if (variableDropIndicator.value?.key === key) variableDropIndicator.value = null
}

async function dropVariableIntoGroup(event: DragEvent, key: string, groupId: string, scope: VariableScope) {
  event.preventDefault()
  event.stopPropagation()
  const variable = variables.value.find(item => item.id === variableDragId.value)
  variableDropIndicator.value = null
  if (!variable) return
  const plan = planVariableGroupDrop(variableGroups.value, variable, groupId, scope, isFunctionBlueprintTab.value)
  await commitVariableGroupDrop(variable, plan)
}

function variableDropClass(key: string) {
  return variableDropIndicator.value?.key === key ? `variable-drop-${variableDropIndicator.value.plan.kind}` : ''
}

function variableDropHint(key: string) {
  return variableDropIndicator.value?.key === key ? variableDropIndicator.value.label : undefined
}

async function createVariableNode(variable: GraphVariable, access: 'get' | 'set', position?: { x: number; y: number }) {
  await editor?.addVariableNode(variable, access, position)
}

function touchFunctionSignature() {
  if (isFunctionBlueprintTab.value) activeTab.value.dirty = true
}

function syncFunctionCategoryToGraph() {
  if (!isFunctionBlueprintTab.value) return
  functionCategory.value = activeFunctionCategory()
  if (activeTab.value.document) activeTab.value.document.functionCategory = functionCategory.value
  if (activeTab.value.path) functionCategoryByPath.value = { ...functionCategoryByPath.value, [activeTab.value.path]: functionCategory.value }
  touchFunctionSignature()
}

async function syncFunctionTitleToGraph() {
  if (!isFunctionBlueprintTab.value) return
  functionTitle.value = activeFunctionTitle()
  if (activeTab.value.document) {
    activeTab.value.document.graphName = functionTitle.value
    activeTab.value.document.functionId = activeFunctionId()
    activeTab.value.document.functionCategory = activeFunctionCategory()
  }
  if (activeTab.value.path) functionTitleByPath.value = { ...functionTitleByPath.value, [activeTab.value.path]: functionTitle.value }
  if (activeTab.value.path) functionIdByPath.value = { ...functionIdByPath.value, [activeTab.value.path]: activeFunctionId() }
  if (activeTab.value.path) functionCategoryByPath.value = { ...functionCategoryByPath.value, [activeTab.value.path]: activeFunctionCategory() }
  await syncFunctionSignatureToGraph()
}

function activeFunctionMetadata(role: FunctionNodeMetadata['functionRole']): FunctionNodeMetadata {
  const functionName = activeFunctionTitle()
  return {
    functionRole: role,
    functionId: activeFunctionId(),
    functionName,
    functionSource: 'workspace',
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
	if (type === 'timerhandle' || type === 'timer_handle') return 'timerhandle'
  return 'string'
}

function normalizeDocument(value: any): GraphDocument {
  const sourceVariables = Array.isArray(value.variables) ? value.variables : []
  const groupNormalization = normalizeVariableGroups(value.variableGroups, sourceVariables)
  const variables: GraphVariable[] = sourceVariables.map((variable: any, index: number) => {
    const type = normalizeVariableType(variable?.type)
    const scope: VariableScope = variable?.scope === 'instance' ? 'instance' : 'execution'
    const requestedGroupId = String(variable?.groupId ?? '')
    let defaultValue = variable?.defaultValue ?? variable?.value ?? defaultVariableValue(type)
    if (type === 'integer' && isPreciseJSONInteger(defaultValue)) defaultValue = defaultValue.lexeme
    else if (type === 'float' && isPreciseJSONInteger(defaultValue)) defaultValue = Number(defaultValue.lexeme)
    return {
      id: String(variable?.id || crypto.randomUUID()),
      name: String(variable?.name || `Variable${index + 1}`),
      type,
      defaultValue,
      groupId: groupNormalization.resolveGroupId(requestedGroupId, String(variable?.group ?? 'Default'), scope),
      description: String(variable?.description ?? ''),
      scope: scope === 'instance' ? 'instance' : undefined
    }
  })
  return {
    schemaVersion: 1,
    graphName: String(value.graphName ?? value.graph_name ?? 'Imported Graph'),
    functionId: String(value.functionId ?? '').trim() || undefined,
    functionCategory: String(value.functionCategory ?? '').trim() || undefined,
    nodes: Array.isArray(value.nodes) ? value.nodes : [],
    connections: Array.isArray(value.connections) ? value.connections : [],
    groups: Array.isArray(value.groups) ? value.groups : [],
    comments: Array.isArray(value.comments) ? value.comments : [],
    macroRefs: Array.isArray(value.macroRefs) ? value.macroRefs : [],
    macroId: String(value.macroId ?? '').trim() || undefined,
    variables,
    variableGroups: groupNormalization.groups,
    functionSignature: normalizeFunctionSignature(value.functionSignature),
    view: value.view ?? { x: 0, y: 0, zoom: 1 },
    legacy: value.legacy
  }
}

function isNativeGraphDocument(value: any) {
  return value?.schemaVersion === 1
}

function isLegacyGraphPath(path: string) {
  return /\.vgf$/i.test(path)
}

async function testGraph() {
  if (!editor) return
  await editor.highlightIssueNodes([])
  const document = documentWithFunctionSignature(editor.getDocument(activeTab.value.title, variables.value, variableGroups.value), activeTab.value)
  validationIssues.value = await platform.validateGraph(serializeGraphDocument(activeTab.value.path || activeTab.value.title, document), workspaceRoot.value, activeTab.value.path)
  activeTab.value.saveBlocked = validationIssues.value.some(issue => issue.blocksSave && !issue.target)
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

function issueSeverityLabel(issue: ValidationIssue) {
  return issue.severity === 'error' ? menuText.value.validation.error : menuText.value.validation.warning
}

function issueNodeLabel(issue: ValidationIssue) {
  const ids = issueNodeIds(issue)
  return ids.length ? ids.join(', ') : menuText.value.validation.noNode
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

function validationIssueGroup(issue: ValidationIssue) {
  if (issue.target) return { key: `target:${issue.target}`, label: `运行目标：${issue.target}` }
  const sourcePath = issue.sourcePath?.trim() ?? ''
  const currentPath = activeTab.value?.path?.trim() ?? ''
  const normalize = (value: string) => {
    const normalized = value.replace(/\\/g, '/')
    return platform.isDesktop() ? normalized.toLowerCase() : normalized
  }
  if (sourcePath && (!currentPath || normalize(sourcePath) !== normalize(currentPath))) {
    return { key: `workspace:${normalize(sourcePath)}`, label: `Workspace 依赖：${sourcePath}` }
  }
  return { key: 'current', label: '当前蓝图' }
}

async function loadRecoverySnapshotPrompts() {
  try {
    recoveryQueue = await platform.listRecoverySnapshots()
    showNextRecoverySnapshotPrompt()
  } catch (error) {
    status.value = `Recovery scan failed: ${error instanceof Error ? error.message : String(error)}`
  }
}

function showNextRecoverySnapshotPrompt() {
  recoveryDialog.value = { visible: recoveryQueue.length > 0, snapshot: recoveryQueue[0] ?? null }
}

function keepRecoverySnapshot() {
  recoveryQueue.shift()
  showNextRecoverySnapshotPrompt()
}

async function deleteRecoverySnapshot() {
  const snapshot = recoveryDialog.value.snapshot
  if (!snapshot) return
  try {
    await platform.deleteRecoverySnapshot(snapshot.path)
    recoveryQueue.shift()
    showNextRecoverySnapshotPrompt()
  } catch (error) {
    status.value = `Recovery delete failed: ${error instanceof Error ? error.message : String(error)}`
  }
}

async function restoreRecoverySnapshot() {
  const snapshot = recoveryDialog.value.snapshot
  if (!snapshot) return
  try {
    const raw = await platform.readRecoverySnapshot(snapshot.path)
    const envelope = parseGraphJSON(raw) as { document?: unknown; blockingIssues?: ValidationIssue[] }
    if (!envelope.document || typeof envelope.document !== 'object') throw new Error('Recovery snapshot has no graph document')
    const document = normalizeDocument(envelope.document)
    const sourcePath = snapshot.sourcePath ?? ''
    const sourceTitle = sourcePath.split(/[\\/]/).pop() || 'Untitled'
    const tab: GraphTab = {
      id: crypto.randomUUID(),
      title: `${sourceTitle} (Recovered)`,
      path: sourcePath,
      dirty: true,
      document,
      restoreFatal: true,
      saveBlocked: Boolean(envelope.blockingIssues?.some(issue => issue.blocksSave)),
    }
    tabs.value.push(tab)
    await switchTab(tab.id)
    validationIssues.value = envelope.blockingIssues ?? []
    selectedValidationIssueKey.value = ''
    showLogger.value = validationIssues.value.length > 0
    try {
      await platform.deleteRecoverySnapshot(snapshot.path)
    } catch (error) {
      await platform.logClientError('warning', error instanceof Error ? error.message : String(error), '', 'DeleteRecoverySnapshotAfterRestore')
    }
    recoveryQueue.shift()
    showNextRecoverySnapshotPrompt()
    status.value = `Recovered ${sourceTitle} into a protected dirty tab; save it as a new file after fixing fatal issues`
  } catch (error) {
    status.value = `Recovery failed: ${error instanceof Error ? error.message : String(error)}`
  }
}

async function openGraph(path = '', highlightTypeId = '') {
  const file = await platform.openGraph(path)
  if (!file) return
  const existing = findOpenTab(tabs.value, file.path, platform.isDesktop())
  if (existing) {
    // 宏显示名以磁盘为权威（改名即时落盘）：已打开的宏标签页重读磁盘同步显示名，
    // 避免内存旧文档让模块库/详情面板回退旧名。
    if (isMacroSourcePath(file.path)) {
      try {
        const diskName = String((JSON.parse(file.content) as GraphDocument).graphName ?? '').trim()
        if (diskName) {
          if (existing.document && existing.document.graphName !== diskName) existing.document.graphName = diskName
          macroTitleByPath.value = { ...macroTitleByPath.value, [file.path]: diskName }
          if (existing.id === activeTabId.value) macroTitle.value = diskName
          existing.title = `${diskName}.obpm`
        }
      } catch { /* 磁盘内容异常时保持内存态 */ }
    }
    await switchTab(existing.id)
    status.value = `${existing.title} is already open`
    await highlightReferenceSearchTarget(highlightTypeId)
    return
  }
  let parsed: any
  try { parsed = parseGraphJSON(file.content) } catch { status.value = 'Invalid graph file'; return }
  let document: GraphDocument
  let sourceIssues: ValidationIssue[] = []
  if (isNativeGraphDocument(parsed)) {
    // 宏文件历史版本可能缺 graphName（旧保存逻辑剥离）：归一化的 'Imported Graph' 兜底对宏无意义，
    // 用文件名回填，避免显示名退化。
    if (isMacroSourcePath(file.path)) {
      const macroGraphName = String(parsed.graphName ?? '').trim()
      if (!macroGraphName || macroGraphName === 'Imported Graph') {
        parsed.graphName = (file.path.split(/[\/]/).pop() ?? '宏').replace(/\.obpm$/i, '')
      }
    }
    sourceIssues = await platform.validateGraph(file.content, workspaceRoot.value, file.path)
    document = normalizeDocument(parsed)
  }
  else if (platform.isDesktop()) {
    try { document = normalizeDocument(parseGraphJSON(await platform.migrateLegacyGraph(file.content))) }
    catch (error) { status.value = error instanceof Error ? error.message : 'Legacy graph migration failed'; return }
  } else { status.value = 'Legacy graph migration requires the desktop runtime'; return }
  // 普通蓝图名字=文件名（历史约定）；函数与宏的 graphName 是独立显示名，不能被文件名覆盖。
  if (!isFunctionBlueprintPath(file.path) && !isMacroSourcePath(file.path)) document.graphName = filenameStem(file.path)
  if (isFunctionBlueprintPath(file.path) && !document.functionId) document.functionId = newFunctionId()
  await loadFunctionLibraryTitles(functionLibraryItems.value)
  await refreshDocumentFunctionReferencesOnOpen(document, file.path)
  persistActive()
  const title = file.path.split(/[\\/]/).pop() ?? document.graphName
  const tab: GraphTab = { id: crypto.randomUUID(), title, path: file.path, dirty: false, document }
  tabs.value.push(tab)
  activeTabId.value = tab.id
  selectedVariableId.value = null
  functionSignature.value = normalizeFunctionSignature(document.functionSignature)
  functionTitle.value = isFunctionBlueprintPath(file.path) ? functionTitleFromDocument(document, file.path, title) : ''
  macroTitle.value = isMacroSourcePath(file.path) ? String(document.graphName ?? '').trim() : ''
  if (isMacroSourcePath(file.path) && document.macroId) macroIdByPath.value = { ...macroIdByPath.value, [file.path]: String(document.macroId).trim() }
  if (macroTitle.value) tab.title = `${macroTitle.value}.obpm`
  functionId.value = isFunctionBlueprintPath(file.path) ? functionIdFromDocument(document) : ''
  functionCategory.value = isFunctionBlueprintPath(file.path) ? functionCategoryFromDocument(document, file.path) : ''
  functionDescription.value = isFunctionBlueprintPath(file.path) ? String(document.functionDescription ?? '') : ''
  if (isFunctionBlueprintPath(file.path)) functionTitleByPath.value = { ...functionTitleByPath.value, [file.path]: functionTitle.value }
  if (isFunctionBlueprintPath(file.path) && functionId.value) functionIdByPath.value = { ...functionIdByPath.value, [file.path]: functionId.value }
  if (isFunctionBlueprintPath(file.path)) functionCategoryByPath.value = { ...functionCategoryByPath.value, [file.path]: functionCategory.value }
  if (isFunctionBlueprintPath(file.path) && functionDescription.value) functionDescriptionByPath.value = { ...functionDescriptionByPath.value, [file.path]: functionDescription.value }
  await syncCallableFunctionsToEditor()
  try {
    const report = await editor?.loadDocument(document)
    tab.restoreLoss = hasRestoreLoss(report) ? report : null
    tab.restoreFatal = sourceRequiresProtection(sourceIssues)
  } catch (error) {
    tab.restoreFatal = true
    status.value = `Graph restore failed; source overwrite is disabled: ${error instanceof Error ? error.message : String(error)}`
  }
  if (!tab.restoreFatal && document.legacy?.format === 'vgf') {
    const hiddenCount = document.legacy.hiddenNodes?.length ?? 0
    status.value = `Loaded ${document.nodes.length} visible node(s), ${hiddenCount} hidden undefined node(s)`
  }
  if (tab.restoreLoss) {
    status.value = `Compatibility limited: ${tab.restoreLoss.droppedNodes.length} node(s), ${tab.restoreLoss.droppedConnections.length} connection(s), and ${tab.restoreLoss.alteredNodes.length} normalized node(s); source overwrite is disabled by default`
  }
  if (sourceIssues.length) {
    validationIssues.value = sourceIssues
    selectedValidationIssueKey.value = ''
    showLogger.value = true
    const errors = sourceIssues.filter(issue => issue.severity === 'error').length
    const warnings = sourceIssues.filter(issue => issue.severity === 'warning').length
    status.value = errors
      ? `Source validation found ${errors} error(s) and ${warnings} warning(s); source overwrite is disabled`
      : `Source validation found ${warnings} warning(s)`
  }
  await highlightReferenceSearchTarget(highlightTypeId)
  recentFiles.value = await platform.recentFiles()
}

function requestCompatibilitySaveAction(tab: GraphTab, forceAllowed: boolean) {
  const options = compatibilitySaveOptions({ fatal: Boolean(tab.restoreFatal), hasLoss: hasRestoreLoss(tab.restoreLoss), formatAllowsForce: forceAllowed })
  return new Promise<CompatibilitySaveAction>(resolve => {
    compatibilitySaveDialog.value = {
      visible: true,
      droppedNodes: tab.restoreLoss?.droppedNodes.length ?? 0,
      droppedConnections: tab.restoreLoss?.droppedConnections.length ?? 0,
      alteredNodes: tab.restoreLoss?.alteredNodes.length ?? 0,
      fatal: Boolean(tab.restoreFatal),
      forceAllowed: options.includes('force'),
      resolve
    }
  })
}

function resolveCompatibilitySaveAction(action: CompatibilitySaveAction) {
  const resolve = compatibilitySaveDialog.value.resolve
  compatibilitySaveDialog.value = { visible: false, droppedNodes: 0, droppedConnections: 0, alteredNodes: 0, fatal: false, forceAllowed: false }
  resolve?.(action)
}

async function validateForPersistence(tab: GraphTab, document: GraphDocument) {
  const documentJSON = serializeGraphDocument(tab.path || tab.title, document)
  const issues = await platform.validateGraph(documentJSON, workspaceRoot.value, tab.path)
  const decision = saveGateDecision(issues, projectSettingsContent.value.editor.validateBeforeSave)
  tab.saveBlocked = decision.blocked
  if (tab.id === activeTabId.value) {
    validationIssues.value = issues
    selectedValidationIssueKey.value = ''
  }
  if (!decision.blocked) return true

  tab.dirty = true
  const snapshot = await platform.saveRecoverySnapshot(tab.path, tab.id, documentJSON, JSON.stringify(decision.blockingIssues))
  if (tab.id === activeTabId.value) {
    showLogger.value = true
    const nodeIDs = decision.blockingIssues.flatMap(issueNodeIds)
    await editor?.highlightIssueNodes([...new Set(nodeIDs)])
  }
  const location = snapshot?.path ? ` Recovery snapshot: ${snapshot.path}` : ''
  status.value = `Save blocked by ${decision.blockingIssues.length} fatal core graph issue(s).${location}`
  return false
}

async function clearRecoverySnapshotsAfterSave(tab: GraphTab, previousPath: string, savedPath: string) {
  try {
    await platform.deleteRecoverySnapshots(previousPath, tab.id)
    if (savedPath && savedPath !== previousPath) await platform.deleteRecoverySnapshots(savedPath, tab.id)
  } catch (error) {
    await platform.logClientError('warning', error instanceof Error ? error.message : String(error), '', 'DeleteRecoverySnapshots')
  }
}

async function saveGraph(saveAs: boolean) {
  if (persistenceInFlight) {
    status.value = 'A save operation is already in progress'
    return
  }
  persistenceInFlight = true
  try {
    await saveGraphUnchecked(saveAs)
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error)
    status.value = `${menuText.value.status.saveFailed}: ${detail}`
  } finally {
    persistenceInFlight = false
  }
}

function resetAutoSaveTimer() {
  if (autoSaveTimer) window.clearInterval(autoSaveTimer)
  autoSaveTimer = undefined
  const interval = autoSaveIntervalMs(projectSettingsContent.value.editor.autoSave)
  if (!interval || !platform.isDesktop()) return
  autoSaveTimer = window.setInterval(() => { void autoSaveDirtyTabs() }, interval)
}

function documentForAutoSave(tab: GraphTab) {
  if (tab.id === activeTabId.value && editor) {
    return documentWithFunctionSignature(editor.getDocument(tab.title, variables.value, variableGroups.value), tab)
  }
  return tab.document
}

async function autoSaveDirtyTabs() {
  if (persistenceInFlight || !platform.isDesktop()) return
  persistenceInFlight = true
  let saved = 0
  const failures: string[] = []
  try {
    for (const tab of tabs.value) {
      const document = documentForAutoSave(tab)
      if (!document) continue
      const requiresNativePersistence = documentRequiresNativePersistence(document)
      if (!isAutoSaveEligible({
        dirty: tab.dirty,
        path: tab.path,
        restoreFatal: Boolean(tab.restoreFatal),
        hasRestoreLoss: hasRestoreLoss(tab.restoreLoss),
        legacyRequiresNative: isLegacyGraphPath(tab.path) && requiresNativePersistence,
        saving: false,
      })) continue
      try {
        if (!await validateForPersistence(tab, document)) {
          failures.push(`${tab.title}: blocked by fatal graph validation`)
          continue
        }
        const previousPath = tab.path
        const content = isLegacyGraphPath(tab.path)
          ? await platform.exportLegacyGraph(serializeGraphDocument(tab.path, document))
          : serializeGraphDocument(tab.path, document, 2)
        const path = await platform.saveGraph(tab.path, content)
        if (!path) continue
        tab.path = path
        tab.title = path.split(/[\\/]/).pop() ?? tab.title
        tab.document = document
        tab.dirty = false
        tab.saveBlocked = false
        await clearRecoverySnapshotsAfterSave(tab, previousPath, path)
        saved++
      } catch (error) {
        failures.push(`${tab.title}: ${error instanceof Error ? error.message : String(error)}`)
      }
    }
  } finally {
    persistenceInFlight = false
  }
  if (failures.length) status.value = `Auto-save saved ${saved} graph(s); ${failures.length} failed: ${failures.join('; ')}`
  else if (saved) status.value = `Auto-saved ${saved} graph(s)`
}

async function saveGraphUnchecked(saveAs: boolean) {
  if (!editor) return
  const tab = activeTab.value
  const document = documentWithFunctionSignature(editor.getDocument(tab.title, variables.value, variableGroups.value), tab)
  if (!await validateForPersistence(tab, document)) return
  const requiresNativePersistence = documentRequiresNativePersistence(document)
  let effectiveSaveAs = saveAs
  let forceOriginal = false
  if (!saveAs && tab.path && (tab.restoreFatal || hasRestoreLoss(tab.restoreLoss))) {
    const forceAllowed = !tab.restoreFatal && !(isLegacyGraphPath(tab.path) && requiresNativePersistence)
    const action = await requestCompatibilitySaveAction(tab, forceAllowed)
    const persistenceAction = resolveCompatibilityPersistenceAction(action, { fatal: Boolean(tab.restoreFatal), hasLoss: hasRestoreLoss(tab.restoreLoss), formatAllowsForce: forceAllowed })
    if (persistenceAction === 'cancel') return
    if (persistenceAction === 'recovery-copy') effectiveSaveAs = true
    if (persistenceAction === 'force-source-with-backup') {
      const droppedNodes = tab.restoreLoss?.droppedNodes.length ?? 0
      const droppedConnections = tab.restoreLoss?.droppedConnections.length ?? 0
      const alteredNodes = tab.restoreLoss?.alteredNodes.length ?? 0
      const confirmed = window.confirm(`强制覆盖会永久移除或调整编辑器无法完整恢复的 ${droppedNodes} 个结点、${droppedConnections} 条连线和 ${alteredNodes} 个结点属性。将先创建 ${tab.path}.bak。确认继续？`)
      if (!confirmed) return
      forceOriginal = true
    }
  }
  const forceNativeSaveAs = !effectiveSaveAs && !forceOriginal && isLegacyGraphPath(tab.path) && requiresNativePersistence
  const sourcePath = tab.path || tab.title
  let targetPath = tab.path
  if (!forceOriginal && (effectiveSaveAs || forceNativeSaveAs || !targetPath)) {
    targetPath = await platform.chooseGraphSavePath(tab.path, isFunctionBlueprintPath(sourcePath), requiresNativePersistence)
    if (!targetPath) return
  }
  const preparedSave = prepareGraphSave(sourcePath, targetPath, document)
  const content = preparedSave.exportLegacy
    ? await platform.exportLegacyGraph(preparedSave.documentJSON)
    : preparedSave.documentJSON
  const previousPath = tab.path
  const path = forceOriginal
    ? await platform.forceSaveGraph(preparedSave.path, content)
    : await platform.saveGraph(preparedSave.path, content)
  if (!path) return
  tab.path = path; tab.title = path.split(/[\\/]/).pop() ?? tab.title; tab.document = document; tab.dirty = false; tab.restoreLoss = null; tab.restoreFatal = false; tab.saveBlocked = false
  await clearRecoverySnapshotsAfterSave(tab, previousPath, path)
  if (isFunctionBlueprintPath(path)) {
    functionTitle.value = activeFunctionTitle()
    functionTitleByPath.value = { ...functionTitleByPath.value, [path]: functionTitle.value }
    functionId.value = activeFunctionId()
    functionIdByPath.value = { ...functionIdByPath.value, [path]: functionId.value }
    functionCategory.value = activeFunctionCategory()
    functionCategoryByPath.value = { ...functionCategoryByPath.value, [path]: functionCategory.value }
    await syncOpenFunctionReferences(activeFunctionMetadata('call'))
  }
  recentFiles.value = await platform.recentFiles(); status.value = forceOriginal ? `Saved ${tab.title}; backup created at ${path}.bak` : `Saved ${tab.title}`
  // 宏保存后热更传播：活动引用图立即刷新镜像与宏名；磁盘引用图按引用取最新（无需改写）。
  if (/\.obpm$/i.test(path)) void propagateMacroUpdate(path)
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
  const path = await platform.chooseWorkspace()
  if (!path) return
  if (!isSameWorkspacePath(path, workspaceRoot.value) && !(await closeAllGraphTabsForWorkspaceSwitch())) return
  await loadWorkspace(path)
}

function isSameWorkspacePath(left: string, right: string) {
  if (!left || !right) return false
  return left.replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase() === right.replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase()
}

// 切换工程目录会替换整份节点定义，旧蓝图里的节点类型可能在新目录下不存在，
// 因此默认关闭所有已打开的蓝图；有未保存修改时先让用户确认。
async function closeAllGraphTabsForWorkspaceSwitch() {
  const dirtyTabs = tabs.value.filter(tab => tab.dirty)
  if (dirtyTabs.length) {
    const names = dirtyTabs.map(tab => tab.title).join('、')
    if (!window.confirm(`切换工程目录会关闭所有已打开的蓝图，以下蓝图有未保存的修改：\n${names}\n\n确定继续？`)) return false
  }
  tabs.value = []
  activeTabId.value = ''
  selectedNode.value = null
  await newGraph()
  return true
}

async function refreshWorkspace() {
  if (!workspaceRoot.value) return
  await loadWorkspace(workspaceRoot.value, false)
  status.value = 'Workspace refreshed'
}

async function refreshNodeLibrary() {
  const nodeLoadStatus = await loadRuntimeNodeLibrary()
  void loadConfigTables()
  status.value = nodeLoadStatus || `Node library refreshed (${nodeLibrary.value.length} node template(s))`
}

const configTables = ref<ConfigTable[]>([])

async function loadConfigTables() {
  if (!workspaceRoot.value) {
    configTables.value = []
    setConfigTables([])
    return
  }
  try {
    const tables = await platform.loadConfigTables(workspaceRoot.value, projectSettingsContent.value.configTables)
    configTables.value = tables
    setConfigTables(tables)
    const missing = tables.filter(table => table.missing).length
    status.value = configTableStatusLine(tables) + (missing > 0 ? `，${missing} 个数据集未找到来源文件（配置已保留）` : '')
  } catch (error) {
    configTables.value = []
    setConfigTables([])
    status.value = `配置表加载失败：${error instanceof Error ? error.message : String(error)}`
  }
}

const configTableImportDialog = ref(false)
const configTableSaveFeedback = ref('')
const configTableScan = ref<ConfigTable[]>([])
const configTableImportDraft = ref<ProjectSettings['configTables']>({ directories: ['configs'], defaults: { idMatchRow: 1, nameMatchRow: 1, idKeyword: 'id', nameKeywords: ['name', '名称'] }, datasets: [] })
// 批量导入只做“新增”：勾选尚未导入的表，保存时为它们各建一个默认数据集。
// 已有数据集的表不提供勾选（删除/改名在“数据集管理”里做，避免误删）。
const configTablePendingPicks = ref<Set<string>>(new Set())

async function refreshConfigTableScan() {
  if (!workspaceRoot.value) {
    configTableScan.value = []
    return
  }
  configTableScan.value = await platform.scanConfigTables(workspaceRoot.value, projectSettingsContent.value.configTables)
}

const configTableDirectoriesDialog = ref(false)
const configTableDirectoriesDraft = ref<string[]>([])

function openConfigTableDirectoriesDialog() {
  configTableDirectoriesDraft.value = [...projectSettingsContent.value.configTables.directories]
  if (!configTableDirectoriesDraft.value.length) configTableDirectoriesDraft.value = ['configs']
  configTableDirectoriesDialog.value = true
}

function addConfigTableDirectory() {
  configTableDirectoriesDraft.value.push('')
}

async function browseConfigTableDirectory(index: number) {
  if (!workspaceRoot.value) return
  try {
    const picked = await platform.chooseConfigTableDirectory(workspaceRoot.value)
    if (!picked) return
    configTableDirectoriesDraft.value[index] = relativeWorkspaceDirectory(picked)
  } catch (error) {
    status.value = `选择目录失败：${error instanceof Error ? error.message : String(error)}`
  }
}

async function saveConfigTableDirectories() {
  const directories = configTableDirectoriesDraft.value.map(item => item.trim()).filter(Boolean)
  if (!directories.length) {
    status.value = '至少保留一个表元目录'
    return
  }
  projectSettingsContent.value = {
    ...projectSettingsContent.value,
    configTables: normalizeConfigTablesSettings({ ...projectSettingsContent.value.configTables, directories }, projectSettingsContent.value.configTables)
  }
  configTableDirectoriesDialog.value = false
  await saveProjectSettings()
  await loadConfigTables()
  if (configTableImportDialog.value) await refreshConfigTableScan()
  status.value = `表元目录已保存（${directories.length} 个），可用“导入配置表”勾选要导入的表`
}

function relativeWorkspaceDirectory(absolute: string) {
  const root = String(workspaceRoot.value ?? '').replace(/\\/g, '/').replace(/\/+$/, '')
  const target = String(absolute ?? '').replace(/\\/g, '/').replace(/\/+$/, '')
  if (root && target.toLowerCase().startsWith(`${root.toLowerCase()}/`)) return target.slice(root.length + 1)
  return target || '.'
}

async function openConfigTableImportDialog() {
  configTableSaveFeedback.value = ''
  configTableListView.value = 'all'
  configTableImportDraft.value = JSON.parse(JSON.stringify(projectSettingsContent.value.configTables)) as ProjectSettings['configTables']
  configTablePendingPicks.value = new Set()
  configTablePendingDrafts.value = {}
  configTablePendingExpanded.value = new Set()
  configTablePreview.value = {}
  configTableSearch.value = ''
  configTableImportDialog.value = true
  await refreshConfigTableScan()
}

const configTableSearch = ref('')
const configTableListView = ref<'all' | 'imported' | 'pending'>('all')

type ConfigTableListView = 'all' | 'imported' | 'pending'

function setConfigTableListView(view: ConfigTableListView) {
  configTableListView.value = view
}

// 数据集与物理表按“文件名 + sheet 名”对应。
function datasetsForScanTable(table: ConfigTable) {
  return configTableImportDraft.value.datasets.filter(dataset =>
    dataset.file === table.file && (dataset.sheet ?? '') === (table.sheet ?? ''))
}

function hasDatasetForTable(table: ConfigTable) {
  return datasetsForScanTable(table).length > 0
}

// 勾选的表可展开“识别设置”单独配置表头识别与筛选（不配置则沿用通用默认，保存后仍可在数据集管理里改）。
const configTablePendingDrafts = ref<Record<string, ConfigTableDatasetSetting>>({})
const configTablePendingExpanded = ref<Set<string>>(new Set())

function pendingDatasetDraft(table: ConfigTable): ConfigTableDatasetSetting {
  const existing = configTablePendingDrafts.value[table.key]
  if (existing) return existing
  const created: ConfigTableDatasetSetting = {
    key: '', name: '', file: table.file, sheet: table.sheet ?? '',
    idMatchRow: 0, nameMatchRow: 0, idKeyword: '', nameKeyword: '',
    filter: normalizeConfigTableFilter(undefined)
  }
  configTablePendingDrafts.value = { ...configTablePendingDrafts.value, [table.key]: created }
  return created
}

function togglePendingPick(table: ConfigTable, checked: boolean) {
  const picks = new Set(configTablePendingPicks.value)
  if (checked) {
    picks.add(table.key)
    pendingDatasetDraft(table)
  } else {
    picks.delete(table.key)
    const expanded = new Set(configTablePendingExpanded.value)
    expanded.delete(table.key)
    configTablePendingExpanded.value = expanded
  }
  configTablePendingPicks.value = picks
}

async function togglePendingExpanded(table: ConfigTable) {
  const expanded = new Set(configTablePendingExpanded.value)
  if (expanded.has(table.key)) {
    expanded.delete(table.key)
  } else {
    pendingDatasetDraft(table)
    expanded.add(table.key)
  }
  configTablePendingExpanded.value = expanded
  await loadPendingTablePreview(table)
}

// 全选/全不选只作用于当前搜索结果里尚未导入的表（已导入的表不可勾选）。
function setAllPendingPicks(imported: boolean) {
  const picks = new Set(configTablePendingPicks.value)
  for (const table of filteredConfigTableScan()) {
    if (hasDatasetForTable(table)) continue
    if (imported) {
      picks.add(table.key)
      pendingDatasetDraft(table)
    } else {
      picks.delete(table.key)
    }
  }
  configTablePendingPicks.value = picks
}

// 预览勾选表按其识别设置解析的全量数据（用临时数据集 key 组合草稿，避免与已有数据集冲突）。
async function loadPendingTablePreview(table: ConfigTable) {
  const pick = configTablePendingDrafts.value[table.key]
  if (!pick || !workspaceRoot.value || configTablePreviewLoading.value.has(table.key)) return
  const loading = new Set(configTablePreviewLoading.value)
  loading.add(table.key)
  configTablePreviewLoading.value = loading
  try {
    const previewKey = `${table.key}__pick`
    const composed = JSON.parse(JSON.stringify({
      ...configTableImportDraft.value,
      datasets: [...configTableImportDraft.value.datasets, { ...pick, key: previewKey }]
    })) as ProjectSettings['configTables']
    const result = await platform.previewConfigTable(workspaceRoot.value, composed, previewKey, pick.filter.extraKeyword, pick.filter.extraMatchRow)
    configTablePreview.value = { ...configTablePreview.value, [table.key]: result?.entries ?? [] }
  } finally {
    const done = new Set(configTablePreviewLoading.value)
    done.delete(table.key)
    configTablePreviewLoading.value = done
  }
}

// 勾选表预览：全量条目按该表草稿筛选即时套用（与数据集管理的预览语义一致）。
function pendingPreviewEntries(table: ConfigTable) {
  const pick = configTablePendingDrafts.value[table.key]
  const entries = configTablePreview.value[table.key] ?? []
  if (!pick) return { matched: 0, total: 0, shown: [] as ConfigTableEntry[] }
  const filter = pick.filter
  let filtered: ConfigTableEntry[]
  if (filter.mode === 'range') {
    filtered = filter.rangeMin && filter.rangeMax ? filterConfigEntries(entries, `${filter.rangeMin}-${filter.rangeMax}`) : entries
  } else if (filter.mode === 'compare') {
    filtered = filter.compareValue ? filterConfigEntries(entries, `${filter.compareOp}${filter.compareValue}`) : entries
  } else {
    filtered = filterConfigEntries(entries, filter.text, { columns: filter.column })
  }
  return { matched: filtered.length, total: entries.length, shown: filtered.slice(0, 30) }
}

// 表太多时的模糊搜索（表键/文件名/sheet 名包含、不区分大小写）+ 视图筛选（全部/已导入/未导入）。
function filteredConfigTableScan() {
  const query = configTableSearch.value.trim().toLowerCase()
  return configTableScan.value.filter(table => {
    const imported = hasDatasetForTable(table)
    if (configTableListView.value === 'imported' && !imported) return false
    if (configTableListView.value === 'pending' && imported) return false
    if (!query) return true
    return table.key.toLowerCase().includes(query)
      || table.file.toLowerCase().includes(query)
      || (table.sheet ?? '').toLowerCase().includes(query)
  })
}

function pendingConfigTableCount() {
  return configTableScan.value.filter(table => !hasDatasetForTable(table)).length
}

// 数据集清单里存在、但当前表元目录中找不到来源文件的表（文件被移动/改名，或他人工程目录不同）。
// 配置原样保留、绝不自动删除；换到包含对应文件的目录后自动恢复。
function missingImportedConfigTables(): ConfigTable[] {
  return configTableImportDraft.value.datasets
    .filter(dataset => dataset.key && !configTableScan.value.some(table =>
      table.file === dataset.file && (table.sheet ?? '') === (dataset.sheet ?? '')))
    .map(dataset => ({
      key: dataset.key,
      name: dataset.name,
      file: dataset.file,
      sheet: dataset.sheet,
      rowCount: 0,
      idColumn: '',
      nameColumn: '',
      imported: true,
      missing: true,
      warning: '未找到来源文件（配置已保留，可在“数据集管理”中删除）',
      entries: []
    }))
}

function displayConfigTableScan() {
  return [...filteredConfigTableScan(), ...missingImportedConfigTables().filter(table => {
    if (configTableListView.value === 'pending') return false
    const query = configTableSearch.value.trim().toLowerCase()
    if (!query) return true
    return table.key.toLowerCase().includes(query) || table.file.toLowerCase().includes(query)
  })]
}

// 列表行摘要：识别结果 + 该表已派生的数据集数。
function configTableSummary(table: ConfigTable) {
  if (table.warning) return `⚠ ${table.warning}`
  const base = `${table.rowCount} 行 · id 列 ${table.idColumn} · 名称列 ${table.nameColumn}`
  const datasets = datasetsForScanTable(table).length
  return datasets > 1 ? `${base} · 已有 ${datasets} 个数据集` : base
}

// 数据集管理的预览数据：key 为数据集 key，条目为来源表全量数据（不应用筛选，
// 筛选在界面上按当前编辑中的 dataset.filter 即时套用，便于调整后看到完整命中情况）。
const configTablePreview = ref<Record<string, ConfigTableEntry[]>>({})
const configTablePreviewLoading = ref<Set<string>>(new Set())

// 按草稿数据集规则重新解析来源表并加载全量条目；extraKeyword/extraMatchRow 来自该数据集的筛选列设置。
async function loadConfigTablePreview(key: string, draft: ProjectSettings['configTables']) {
  if (!workspaceRoot.value || configTablePreviewLoading.value.has(key)) return
  const dataset = draft.datasets.find(item => item.key === key)
  if (!dataset) return
  configTablePreviewLoading.value = new Set([...configTablePreviewLoading.value, key])
  try {
    const draftCopy = JSON.parse(JSON.stringify(draft)) as ProjectSettings['configTables']
    const table = await platform.previewConfigTable(workspaceRoot.value, draftCopy, key, dataset.filter.extraKeyword, dataset.filter.extraMatchRow)
    configTablePreview.value = { ...configTablePreview.value, [key]: table?.entries ?? [] }
  } finally {
    const loading = new Set(configTablePreviewLoading.value)
    loading.delete(key)
    configTablePreviewLoading.value = loading
  }
}

// 给新建数据集生成不冲突的稳定 key：沿用表键；冲突时追加 _2、_3…
function uniqueDatasetKey(base: string) {
  const used = new Set<string>([
    ...configTableImportDraft.value.datasets.map(dataset => dataset.key),
    ...configTableManageDraft.value.datasets.map(dataset => dataset.key)
  ])
  let candidate = base || 'dataset'
  let index = 2
  while (used.has(candidate)) candidate = `${base}_${index++}`
  return candidate
}

async function saveConfigTableImport() {
  // 为每个勾选的未导入表创建数据集；展开“识别设置”配置过的表头识别/筛选随数据集一起保存。
  let added = 0
  for (const key of configTablePendingPicks.value) {
    const table = configTableScan.value.find(item => item.key === key)
    if (!table || hasDatasetForTable(table)) continue
    const pick = configTablePendingDrafts.value[key]
    configTableImportDraft.value.datasets.push({
      ...(pick ?? {
        key: '', name: '', file: table.file, sheet: table.sheet ?? '',
        idMatchRow: 0, nameMatchRow: 0, idKeyword: '', nameKeyword: '',
        filter: normalizeConfigTableFilter(undefined)
      }),
      key: uniqueDatasetKey(table.key),
      name: table.key,
      file: table.file,
      sheet: table.sheet ?? ''
    })
    added++
  }
  configTablePendingPicks.value = new Set()
  configTablePendingDrafts.value = {}
  configTablePendingExpanded.value = new Set()
  configTablePreview.value = {}
  const normalized = normalizeConfigTablesSettings(configTableImportDraft.value, projectSettingsContent.value.configTables)
  projectSettingsContent.value = { ...projectSettingsContent.value, configTables: normalized }
  configTableImportDraft.value = JSON.parse(JSON.stringify(normalized)) as ProjectSettings['configTables']
  await saveProjectSettings()
  await loadConfigTables()
  await refreshConfigTableScan()
  const datasets = configTableImportDraft.value.datasets.length
  const entries = configTables.value.reduce((total, table) => total + table.entries.length, 0)
  const missing = configTables.value.filter(table => table.missing).length
  configTableSaveFeedback.value = added
    ? `新增 ${added} 个数据集，共 ${datasets} 个（${entries} 条 id·名称${missing ? `，${missing} 个未找到来源文件` : ''}）`
    : `未勾选新表；当前共 ${datasets} 个数据集`
  status.value = `配置表：${datasets} 个数据集，共 ${entries} 条 id·名称`
}

// ── 数据集管理 ────────────────────────────────────────────────────────────────
// 列表（左）+ 详情（右）：改名、表头识别、筛选、预览、新增/复制/删除。
// 一表多集的入口是“复制”：复制当前数据集后改显示名、改筛选即可切出同表的另一段枚举。
const configTableManageDialog = ref(false)
const configTableManageDraft = ref<ProjectSettings['configTables']>({ directories: ['configs'], defaults: { idMatchRow: 1, nameMatchRow: 1, idKeyword: 'id', nameKeywords: ['name', '名称'] }, datasets: [] })
const configTableManageSelected = ref('')
const configTableManageSearch = ref('')
const configTableManageView = ref<'all' | 'missing'>('all')
const configTableManageAddKey = ref('')
const configTableManageFeedback = ref('')
const configTableManageError = ref('')
// 打开/保存时的数据集快照：用于标记“哪些数据集改了还没保存”。
const configTableManageSnapshot = ref('')

// 未保存统计：draft 与快照逐项对比（新增/修改的数据集 + 已删除的数量）。
const manageDirtyInfo = computed(() => {
  let saved: ConfigTableDatasetSetting[] = []
  try { saved = JSON.parse(configTableManageSnapshot.value) as ConfigTableDatasetSetting[] } catch { saved = [] }
  const savedByKey = new Map(saved.map(dataset => [dataset.key, JSON.stringify(dataset)]))
  const draft = configTableManageDraft.value.datasets
  const dirtyKeys = draft.filter(dataset => savedByKey.get(dataset.key) !== JSON.stringify(dataset)).map(dataset => dataset.key)
  const removed = saved.filter(dataset => !draft.some(item => item.key === dataset.key)).length
  return { dirtyKeys, removed, total: dirtyKeys.length + (removed > 0 ? 1 : 0) }
})

function manageDatasetDirty(key: string) {
  return manageDirtyInfo.value.dirtyKeys.includes(key)
}

async function openConfigTableManageDialog() {
  configTableManageFeedback.value = ''
  configTableManageError.value = ''
  configTableManageSearch.value = ''
  configTableManageView.value = 'all'
  configTableManageAddKey.value = ''
  const normalized = normalizeConfigTablesSettings(projectSettingsContent.value.configTables, projectSettingsContent.value.configTables)
  configTableManageDraft.value = JSON.parse(JSON.stringify(normalized)) as ProjectSettings['configTables']
  configTableManageSnapshot.value = JSON.stringify(configTableManageDraft.value.datasets)
  configTablePreview.value = {}
  configTableManageSelected.value = configTableManageDraft.value.datasets[0]?.key ?? ''
  configTableManageDialog.value = true
  if (configTableManageSelected.value) void loadConfigTablePreview(configTableManageSelected.value, configTableManageDraft.value)
  await refreshConfigTableScan()
}

// 显示名校验：非空且两两不重复（trim 后比较）；返回错误描述，通过返回空串。
function validateManageDatasetNames(): string {
  const seen = new Map<string, string>()
  for (const dataset of configTableManageDraft.value.datasets) {
    const name = dataset.name.trim()
    if (!name) return `数据集 ${dataset.key} 的显示名为空，请填写后再保存`
    const prior = seen.get(name)
    if (prior) return `显示名「${name}」重复：${prior} 与 ${dataset.key} 都在用它，显示名必须唯一`
    seen.set(name, dataset.key)
  }
  return ''
}

// 有未保存修改时关闭需确认，避免切走丢失整批修改。
function closeConfigTableManage() {
  const dirty = manageDirtyInfo.value.total
  if (dirty > 0 && !window.confirm(`数据集管理有 ${dirty} 处修改尚未保存。\n\n确定不保存直接关闭？`)) return
  configTableManageDialog.value = false
}

function selectedManageDataset() {
  return configTableManageDraft.value.datasets.find(dataset => dataset.key === configTableManageSelected.value)
}

// 模板绑定统一走这个 computed（v-model 直接绑定函数调用结果在部分编译路径下会丢 setter，导致输入无效）。
const manageSelected = computed(() => configTableManageDraft.value.datasets.find(dataset => dataset.key === configTableManageSelected.value))

// 来源表重新绑定：下拉值用扫描表 key（file+sheet 唯一）；当前来源不在扫描结果里（文件缺失）时补一个占位项。
function manageSourceValue() {
  const dataset = manageSelected.value
  if (!dataset) return ''
  const scan = configTableScan.value.find(table => table.file === dataset.file && (table.sheet ?? '') === (dataset.sheet ?? ''))
  return scan?.key ?? '__missing__'
}

function setManageSource(event: Event) {
  const dataset = manageSelected.value
  const value = (event.target as HTMLSelectElement).value
  if (!dataset || !value || value === '__missing__') return
  const table = configTableScan.value.find(item => item.key === value)
  if (!table) return
  dataset.file = table.file
  dataset.sheet = table.sheet ?? ''
  void loadConfigTablePreview(dataset.key, configTableManageDraft.value)
}

// 左侧列表：搜索（显示名/key/来源文件）+ 全部/缺失视图（缺失来自已加载索引的 missing 标记）。
function configTableManageDatasetList() {
  const query = configTableManageSearch.value.trim().toLowerCase()
  return configTableManageDraft.value.datasets.filter(dataset => {
    if (configTableManageView.value === 'missing') {
      const loaded = configTables.value.find(table => table.key === dataset.key)
      if (!loaded?.missing) return false
    }
    if (!query) return true
    return dataset.name.toLowerCase().includes(query)
      || dataset.key.toLowerCase().includes(query)
      || dataset.file.toLowerCase().includes(query)
  })
}

// 列表行的状态摘要：已加载条数 / 未保存 / 缺失警告（来自已加载索引，保存后才刷新）。
function manageDatasetMeta(key: string) {
  const loaded = configTables.value.find(table => table.key === key)
  if (loaded?.missing) return `⚠ ${loaded.warning ?? '未找到来源文件'}`
  if (loaded) return `${loaded.entries.length} 条`
  return '未保存/未加载'
}

function selectManageDataset(key: string) {
  configTableManageSelected.value = key
  configTableManageError.value = ''
  void loadConfigTablePreview(key, configTableManageDraft.value)
}

function reloadManagePreview() {
  const dataset = selectedManageDataset()
  if (dataset) void loadConfigTablePreview(dataset.key, configTableManageDraft.value)
}

// 详情区预览：全量条目按当前编辑中的 dataset.filter 即时套用（与加载内存时的筛选语义一致）。
function managePreviewEntries() {
  const dataset = selectedManageDataset()
  if (!dataset) return { matched: 0, total: 0, shown: [] as ConfigTableEntry[] }
  const entries = configTablePreview.value[dataset.key] ?? []
  const filter = dataset.filter
  let filtered: ConfigTableEntry[]
  if (filter.mode === 'range') {
    filtered = filter.rangeMin && filter.rangeMax ? filterConfigEntries(entries, `${filter.rangeMin}-${filter.rangeMax}`) : entries
  } else if (filter.mode === 'compare') {
    filtered = filter.compareValue ? filterConfigEntries(entries, `${filter.compareOp}${filter.compareValue}`) : entries
  } else {
    filtered = filterConfigEntries(entries, filter.text, { columns: filter.column })
  }
  return { matched: filtered.length, total: entries.length, shown: filtered.slice(0, 30) }
}

// 新增候选：表元目录扫描到的物理表（含未导入的；同一表可多次新建数据集）。
function manageAddCandidates() {
  return configTableScan.value
}

function addManageDatasetFromScan(scanKey: string) {
  const table = configTableScan.value.find(item => item.key === scanKey)
  if (!table) return
  configTableManageDraft.value.datasets.push({
    key: uniqueDatasetKey(table.key),
    name: table.key,
    file: table.file,
    sheet: table.sheet ?? '',
    idMatchRow: 0,
    nameMatchRow: 0,
    idKeyword: '',
    nameKeyword: '',
    filter: normalizeConfigTableFilter(undefined)
  })
  configTableManageAddKey.value = ''
  selectManageDataset(configTableManageDraft.value.datasets[configTableManageDraft.value.datasets.length - 1].key)
}

// 一表多集核心操作：复制当前数据集（同来源、同识别规则），改显示名和筛选后另存一段。
function duplicateManageDataset() {
  const dataset = selectedManageDataset()
  if (!dataset) return
  const copy = JSON.parse(JSON.stringify(dataset)) as ConfigTableDatasetSetting
  copy.key = uniqueDatasetKey(dataset.key)
  copy.name = `${dataset.name} 副本`
  configTableManageDraft.value.datasets.push(copy)
  selectManageDataset(copy.key)
}

function removeManageDataset() {
  const dataset = selectedManageDataset()
  if (!dataset) return
  const datasets = configTableManageDraft.value.datasets.filter(item => item.key !== dataset.key)
  configTableManageDraft.value = { ...configTableManageDraft.value, datasets }
  configTableManageSelected.value = datasets[0]?.key ?? ''
  if (configTableManageSelected.value) selectManageDataset(configTableManageSelected.value)
}

async function saveConfigTableManage() {
  // 保存前校验显示名：非空、不重复；不通过时定位到出问题的数据集并中止保存。
  const nameError = validateManageDatasetNames()
  if (nameError) {
    configTableManageError.value = nameError
    configTableManageFeedback.value = ''
    const offender = configTableManageDraft.value.datasets.find(dataset => !dataset.name.trim())
      ?? configTableManageDraft.value.datasets.find(dataset =>
        configTableManageDraft.value.datasets.filter(item => item.name.trim() === dataset.name.trim()).length > 1)
    if (offender) configTableManageSelected.value = offender.key
    return
  }
  configTableManageError.value = ''
  const normalized = normalizeConfigTablesSettings(configTableManageDraft.value, projectSettingsContent.value.configTables)
  projectSettingsContent.value = { ...projectSettingsContent.value, configTables: normalized }
  configTableManageDraft.value = JSON.parse(JSON.stringify(normalized)) as ProjectSettings['configTables']
  configTableManageSnapshot.value = JSON.stringify(configTableManageDraft.value.datasets)
  if (!configTableManageDraft.value.datasets.some(dataset => dataset.key === configTableManageSelected.value)) {
    configTableManageSelected.value = configTableManageDraft.value.datasets[0]?.key ?? ''
  }
  await saveProjectSettings()
  await loadConfigTables()
  await refreshConfigTableScan()
  const datasets = configTableManageDraft.value.datasets.length
  const entries = configTables.value.reduce((total, table) => total + table.entries.length, 0)
  const missing = configTables.value.filter(table => table.missing).length
  configTableManageFeedback.value = `已保存 ${datasets} 个数据集（${entries} 条 id·名称${missing ? `，${missing} 个未找到来源文件` : ''}）`
  status.value = `配置表：${datasets} 个数据集，共 ${entries} 条 id·名称`
}

async function clearRecentFiles() {
  await platform.clearRecentFiles()
  recentFiles.value = []
  status.value = 'Recent graph list cleared'
}

async function quitApplication() {
  await handleCloseRequest()
}
async function loadWorkspace(path: string, refreshNodeSchemas = true) {
  const token = ++workspaceLoadToken
  workspaceRoot.value = path
  expandedWorkspacePaths.value = new Set()
  workspaceTree.value = []
  await loadProjectSettings(path)
  if (token !== workspaceLoadToken) return
  void loadConfigTables()
  const nodeLoadStatus = refreshNodeSchemas ? await loadRuntimeNodeLibrary(path) : ''
  if (token !== workspaceLoadToken) return
  workspaceTree.value = await loadWorkspaceTree(path)
  if (token !== workspaceLoadToken) return
  void hydrateWorkspaceTree(workspaceTree.value, 1, token)
  if (projectSettingsContent.value.explorer.revealActiveFile) void revealActiveWorkspaceFile(false)
  if (nodeLoadStatus) status.value = nodeLoadStatus
}

async function loadWorkspaceTree(path: string, depth = 0): Promise<WorkspaceTreeNode[]> {
  if (depth > 8) return []
  const entries = await platform.listWorkspace(path)
  return entries.map(entry => ({ ...entry, children: [], loaded: !entry.isDir, loading: false }))
}

function mergeWorkspaceChildren(previous: WorkspaceTreeNode[], next: WorkspaceTreeNode[]) {
  const previousByPath = new Map(previous.map(node => [node.path, node]))
  return next.map(node => {
    const old = previousByPath.get(node.path)
    return old && old.isDir === node.isDir ? { ...node, children: old.children, loaded: old.loaded, loading: false } : node
  })
}

async function ensureWorkspaceChildren(node: WorkspaceTreeNode, depth: number, force = false) {
  if (!node.isDir || node.loading || (node.loaded && !force)) return
  node.loading = true
  try {
    const children = await loadWorkspaceTree(node.path, depth)
    node.children = force ? mergeWorkspaceChildren(node.children, children) : children
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

watch([() => projectSettingsContent.value.editor.autoSave, workspaceRoot], () => {
  resetAutoSaveTimer()
})

watch(activeWorkspacePath, path => {
  if (!path) { clearWorkspaceSelection(); return }
  if (projectSettingsContent.value.explorer.revealActiveFile) void revealActiveWorkspaceFile(false)
})

watch([showTools, showRight, showLogger], () => {
  void saveProjectSettings()
})

watch(functionLibraryItems, items => {
  void loadFunctionLibraryTitles(items)
  void loadMacroTitles(macroModuleItems.value)
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

function functionResourceId(node: WorkspaceEntry) {
  const opened = tabs.value.find(tab => tab.path === node.path)
  const openedId = functionIdFromDocument(opened?.document)
  return openedId || functionIdByPath.value[node.path] || ''
}

function functionResourceCategory(node: WorkspaceEntry) {
  const opened = tabs.value.find(tab => tab.path === node.path)
  if (opened?.id === activeTabId.value && isFunctionBlueprintPath(opened.path || opened.title)) return currentFunctionCategory()
  const openedCategory = String(opened?.document?.functionCategory ?? '').trim()
  return openedCategory || functionCategoryByPath.value[node.path] || inferredFunctionCategoryFromPath(node.path)
}

function collectFunctionLibraryItems(nodes: WorkspaceTreeNode[]) {
  const items: FunctionLibraryItem[] = []
  const visit = (entry: WorkspaceTreeNode) => {
    if (!entry.isDir && isFunctionResource(entry)) {
      const id = functionResourceId(entry)
      items.push({
        id: id || encodeURIComponent(entry.path).replace(/%/g, '_'),
        functionId: id,
        name: functionResourceTitle(entry),
        category: functionResourceCategory(entry),
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
    if (!item.path || (functionTitleByPath.value[item.path] && functionIdByPath.value[item.path] && functionCategoryByPath.value[item.path]) || loadingFunctionTitles.has(item.path)) continue
    const opened = tabs.value.find(tab => tab.path === item.path)
    if (opened?.document) {
      const title = String(opened.document.graphName ?? '').trim()
      const id = functionIdFromDocument(opened.document)
      const category = functionCategoryFromDocument(opened.document, item.path)
      const description = String(opened.document.functionDescription ?? '').trim()
      if (title) functionTitleByPath.value = { ...functionTitleByPath.value, [item.path]: title }
      if (id) functionIdByPath.value = { ...functionIdByPath.value, [item.path]: id }
      functionCategoryByPath.value = { ...functionCategoryByPath.value, [item.path]: category }
      functionDescriptionByPath.value = { ...functionDescriptionByPath.value, [item.path]: description }
      continue
    }
    loadingFunctionTitles.add(item.path)
    try {
      const file = await platform.openGraph(item.path)
      if (!file) continue
      const parsed = parseGraphJSON(file.content) as Partial<GraphDocument>
      const title = String(parsed.graphName ?? '').trim()
      const id = String(parsed.functionId ?? '').trim()
      const category = functionCategoryFromDocument(parsed, item.path)
      const description = String(parsed.functionDescription ?? '').trim()
      if (title) functionTitleByPath.value = { ...functionTitleByPath.value, [item.path]: title }
      if (id) functionIdByPath.value = { ...functionIdByPath.value, [item.path]: id }
      functionCategoryByPath.value = { ...functionCategoryByPath.value, [item.path]: category }
      functionDescriptionByPath.value = { ...functionDescriptionByPath.value, [item.path]: description }
    } catch {
      // Function title loading is best-effort; fall back to the file name.
    } finally {
      loadingFunctionTitles.delete(item.path)
    }
  }
}

async function toggleWorkspaceNode(node: WorkspaceTreeNode) {
  selectedWorkspacePath.value = node.path
  if (!node.isDir) {
    void saveProjectSettings()
    return
  }
  const next = new Set(expandedWorkspacePaths.value)
  if (next.has(node.path)) next.delete(node.path); else {
    next.add(node.path)
    await ensureWorkspaceChildren(node, workspaceNodeDepth(node.path) + 1, true)
  }
  expandedWorkspacePaths.value = next
  void saveProjectSettings()
}

function workspaceEntryClass(node: WorkspaceTreeNode) {
  return {
    selected: selectedWorkspacePath.value === node.path,
    active: activeWorkspacePath.value === node.path,
    folder: node.isDir
  }
}

function collapseWorkspaceTree() {
  expandedWorkspacePaths.value = new Set()
  void saveProjectSettings()
  status.value = 'Workspace tree collapsed'
}

function workspaceParentPaths(path: string) {
  const root = workspaceRoot.value.replace(/[\\/]+$/, '')
  if (!root || !path.startsWith(root)) return []
  const relative = path.slice(root.length).replace(/^[\\/]+/, '')
  const parts = relative.split(/[\\/]/).filter(Boolean)
  const parents: string[] = []
  let current = root
  for (let index = 0; index < Math.max(0, parts.length - 1); index++) {
    current = `${current}${current.includes('\\') ? '\\' : '/'}${parts[index]}`
    parents.push(current)
  }
  return parents
}

function workspacePathInRoot(path: string) {
  const root = workspaceRoot.value.replace(/[\\/]+$/, '')
  return Boolean(root) && (path === root || path.startsWith(`${root}\\`) || path.startsWith(`${root}/`))
}

function clearWorkspaceSelection() {
  if (!selectedWorkspacePath.value) return
  selectedWorkspacePath.value = ''
  void saveProjectSettings()
}

async function revealActiveWorkspaceFile(notify = true) {
  return revealWorkspaceFile(activeWorkspacePath.value, notify)
}

async function revealWorkspaceFile(path: string, notify = true) {
  if (!path) {
    clearWorkspaceSelection()
    if (notify) status.value = '当前标签页还没有保存到文件'
    return
  }
  if (!workspaceRoot.value || !workspacePathInRoot(path)) {
    clearWorkspaceSelection()
    if (notify) status.value = '当前文件不在已打开的工程目录中'
    return
  }
  workspaceSearch.value = ''
  const next = new Set(expandedWorkspacePaths.value)
  for (const parent of workspaceParentPaths(path)) {
    next.add(parent)
    const node = findWorkspaceNodeByPath(parent)
    if (node?.isDir) await ensureWorkspaceChildren(node, workspaceNodeDepth(node.path) + 1, true)
  }
  expandedWorkspacePaths.value = next
  if (!findWorkspaceNodeByPath(path)) {
    clearWorkspaceSelection()
    if (notify) status.value = '文件浏览器中未找到当前蓝图文件'
    return
  }
  selectedWorkspacePath.value = path
  await nextTick()
  document.querySelector('.workspace-entry.selected, .workspace-entry.active')?.scrollIntoView({ block: 'nearest' })
  if (notify) status.value = '已定位当前蓝图文件'
  void saveProjectSettings()
}

function selectedWorkspaceRevealPath() {
  const node = selectedWorkspacePath.value ? findWorkspaceNodeByPath(selectedWorkspacePath.value) : null
  return node && !node.isDir ? node.path : ''
}

function currentRevealPath() {
  return selectedWorkspaceRevealPath() || activeWorkspacePath.value
}

async function revealCurrentFileInFolder() {
  const path = currentRevealPath()
  if (!path) {
    clearWorkspaceSelection()
    status.value = '当前没有可定位的蓝图文件'
    return
  }
  void revealWorkspaceFile(path, false)
  await revealFileInFolder(path)
}

function findWorkspaceNodeByPath(path: string, nodes = workspaceTree.value): WorkspaceTreeNode | null {
  for (const node of nodes) {
    if (node.path === path) return node
    const found = findWorkspaceNodeByPath(path, node.children)
    if (found) return found
  }
  return null
}

function pruneWorkspaceCaches() {
  const nextExpanded = new Set<string>()
  for (const path of expandedWorkspacePaths.value) if (findWorkspaceNodeByPath(path)?.isDir) nextExpanded.add(path)
  expandedWorkspacePaths.value = nextExpanded
  if (selectedWorkspacePath.value && !findWorkspaceNodeByPath(selectedWorkspacePath.value)) selectedWorkspacePath.value = ''
}

async function refreshWorkspaceVisibleDirectories(silent = true) {
  if (!workspaceRoot.value || workspaceRefreshInFlight) return
  const token = workspaceLoadToken
  workspaceRefreshInFlight = true
  try {
    const rootChildren = await loadWorkspaceTree(workspaceRoot.value)
    if (token !== workspaceLoadToken) return
    workspaceTree.value = mergeWorkspaceChildren(workspaceTree.value, rootChildren)
    pruneWorkspaceCaches()
    for (const path of expandedWorkspacePaths.value) {
      if (token !== workspaceLoadToken) return
      const node = findWorkspaceNodeByPath(path)
      if (node?.isDir) await ensureWorkspaceChildren(node, workspaceNodeDepth(node.path) + 1, true)
    }
    pruneWorkspaceCaches()
  } catch (error) {
    if (!silent) status.value = `Workspace refresh failed: ${error instanceof Error ? error.message : String(error)}`
  } finally {
    workspaceRefreshInFlight = false
  }
}

function refreshWorkspaceOnFocus() {
  void refreshWorkspaceVisibleDirectories()
}

async function refreshWorkspaceDirectory(path: string) {
  const node = findWorkspaceNodeByPath(path)
  if (!node?.isDir) return
  await ensureWorkspaceChildren(node, workspaceNodeDepth(node.path) + 1, true)
  const next = new Set(expandedWorkspacePaths.value)
  next.add(node.path)
  expandedWorkspacePaths.value = next
  status.value = `Refreshed ${node.name}`
}

async function workspaceOpen(item: WorkspaceTreeNode) {
  selectedWorkspacePath.value = item.path
  if (item.isDir) await toggleWorkspaceNode(item); else await openGraph(item.path)
}

// 宏用专属扩展名 .obpm 标识（格式与 .obp 完全一致，纯身份标签），便于模块库归类与文件识别。
function isMacroSourcePath(path: string) {
  return /\.obpm$/i.test(path)
}

// 插入宏引用（v2 引用语义）：图里只落 macroId + 边界连线，节点/连线/变量一律不复制；
// 宏内容以 .obpm 源文件为准，编辑器展开只读镜像，编译期由引擎内联。
async function insertMacroByPath(path: string, position?: { x: number; y: number }) {
  try {
    // 宏不能引用它自身（会构成循环引用，引擎编译会报 cyclic）；编辑宏请直接改当前文件。
    if (activeTab.value && (activeTab.value.path === path || activeTab.value.title === (path.split(/[\\/]/).pop() ?? path))) {
      status.value = '不能将宏引用到它自身：当前正在编辑该宏文件'
      return
    }
    const payload = await macroPayloadByPath(path)
    if (!payload) {
      status.value = `宏必须是原生蓝图文档：${path}`
      return
    }
    // 宏身份在创建/保存时分配；缺 macroId 的宏（手改文档等）不可被引用。
    const macroId = payload.macroId
    if (!macroId) {
      status.value = `宏缺少宏 ID（macroId）：请打开 ${path.split(/[\\/]/).pop() ?? path} 保存一次后再引用`
      return
    }
    if (activeTab.value?.document?.macroId && activeTab.value.document.macroId === macroId) {
      status.value = '不能将宏引用到它自身：当前正在编辑该宏文件'
      return
    }
    const inserted = await editor?.insertMacroRef({
      macroId,
      // pathHint 仅运行时（详情面板显示/打开源宏），不持久化——引用只落宏 ID。
      pathHint: path,
      label: payload.label,
      snapshot: payload.snapshot,
      variables: payload.variables,
      clientPosition: position ?? visibleCanvasInsertPosition()
    })
    if (inserted) status.value = `已引用宏：${payload.label}（引用语义，修改宏后所有引用蓝图自动更新）`
  } catch (error) {
    status.value = `引用宏失败：${error instanceof Error ? error.message : String(error)}`
  }
}

async function insertMacroFromContextMenu() {
  const path = fileContextMenu.value.path
  fileContextMenu.value.visible = false
  if (!path || !isMacroSourcePath(path)) return
  await insertMacroByPath(path)
}

async function createMacroAtDirectory(directory: string) {
  const rawName = window.prompt('宏名称（建议 宏_业务_行为）', '宏_')
  if (!rawName) return
  const name = sanitizeFunctionFileName(rawName)
  const path = joinWorkspacePath(directory, `${name}.obpm`)
  const document = blankDocument(name)
  // 创建即分配宏身份（macroId）：引用图的稳定锚点，改名/移动文件都不受影响。
  document.macroId = crypto.randomUUID()
  macroIdByPath.value = { ...macroIdByPath.value, [path]: document.macroId }
  const saved = await platform.saveGraph(path, serializeGraphDocument(path, document, 2))
  if (!saved) return
  await refreshWorkspaceAfterFileCreate(saved)
  status.value = `已创建宏 ${name}`
}

async function createMacroInFileContext() {
  const directory = fileContextMenu.value.path
  fileContextMenu.value.visible = false
  if (!directory || !fileContextMenu.value.isDir) return
  await createMacroAtDirectory(directory)
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
  const minFileWidth = fileBrowserMinWidth
  const minToolsWidth = leftToolsMinWidth

  const move = (next: PointerEvent) => {
    const maxFileWidth = Math.min(fileBrowserMaxWidth, Math.max(minFileWidth, (totalWidth - minToolsWidth) * 2))
    const fileWidth = Math.min(maxFileWidth, Math.max(minFileWidth, startFileWidth + next.clientX - startX))
    fileBrowserWidth.value = Math.round(fileWidth)
    leftToolsWidth.value = Math.max(minToolsWidth, Math.round(totalWidth - fileWidth))
  }
  const up = () => {
    localStorage.setItem('origin-blueprint-file-browser-width', String(fileBrowserWidth.value))
    localStorage.setItem('origin-blueprint-left-tools-width', String(leftToolsWidth.value))
    void saveProjectSettings()
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
    leftToolsWidth.value = Math.min(leftToolsMaxWidth, Math.max(leftToolsMinWidth, Math.round(startWidth + next.clientX - startX)))
  }
  const up = () => {
    localStorage.setItem('origin-blueprint-left-tools-width', String(leftToolsWidth.value))
    void saveProjectSettings()
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', up)
}

type ImageExportBounds = { x: number; y: number; width: number; height: number }
const exportImagePadding = 64

function setImportantStyle(element: HTMLElement | SVGElement, property: string, value: string) {
  const previousValue = element.style.getPropertyValue(property)
  const previousPriority = element.style.getPropertyPriority(property)
  element.style.setProperty(property, value, 'important')
  return () => {
    if (previousValue) element.style.setProperty(property, previousValue, previousPriority)
    else element.style.removeProperty(property)
  }
}

function prepareConnectionDomForImageExport(root: HTMLElement) {
  const restore: Array<() => void> = []
  root.querySelectorAll<SVGPathElement>('.connection-hit-area').forEach(path => {
    const previousPath = path.getAttribute('d')
    path.setAttribute('d', '')
    restore.push(() => {
      if (previousPath === null) path.removeAttribute('d')
      else path.setAttribute('d', previousPath)
    })
    restore.push(setImportantStyle(path, 'display', 'none'))
    restore.push(setImportantStyle(path, 'stroke', 'none'))
    restore.push(setImportantStyle(path, 'stroke-width', '0'))
  })
  root.querySelectorAll<SVGSVGElement>('.blueprint-connection').forEach(svg => {
    const line = svg.querySelector<SVGPathElement>('.connection-line')
    if (!line) return
    const color = window.getComputedStyle(svg).getPropertyValue('--connection-color').trim()
      || window.getComputedStyle(line).stroke
      || '#f2f2f2'
    const width = svg.classList.contains('socket-exec') ? '2.5px' : '1.55px'
    restore.push(setImportantStyle(line, 'fill', 'none'))
    restore.push(setImportantStyle(line, 'filter', 'none'))
    restore.push(setImportantStyle(line, 'stroke', color))
    restore.push(setImportantStyle(line, 'stroke-width', width))
    restore.push(setImportantStyle(line, 'vector-effect', 'non-scaling-stroke'))
  })
  return () => restore.reverse().forEach(callback => callback())
}

function relativeElementBounds(element: Element, root: DOMRect): ImageExportBounds | null {
  const rect = element.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) return null
  return { x: rect.left - root.left, y: rect.top - root.top, width: rect.width, height: rect.height }
}

function mergeExportBounds(items: ImageExportBounds[]) {
  const left = Math.min(...items.map(item => item.x))
  const top = Math.min(...items.map(item => item.y))
  const right = Math.max(...items.map(item => item.x + item.width))
  const bottom = Math.max(...items.map(item => item.y + item.height))
  return { x: left, y: top, width: right - left, height: bottom - top }
}

function intersectsBounds(a: ImageExportBounds, b: ImageExportBounds) {
  return a.x < b.x + b.width && a.x + a.width > b.x && a.y < b.y + b.height && a.y + a.height > b.y
}

function paddedExportBounds(bounds: ImageExportBounds) {
  const x = Math.floor(bounds.x - exportImagePadding)
  const y = Math.floor(bounds.y - exportImagePadding)
  const right = Math.ceil(bounds.x + bounds.width + exportImagePadding)
  const bottom = Math.ceil(bounds.y + bounds.height + exportImagePadding)
  return { x, y, width: Math.max(1, right - x), height: Math.max(1, bottom - y) }
}

function graphDirectoryForExport() {
  const path = activeTab.value?.path || activeWorkspacePath.value || workspaceRoot.value
  if (!path) return ''
  if (path === workspaceRoot.value) return path
  const index = Math.max(path.lastIndexOf('\\'), path.lastIndexOf('/'))
  return index > 0 ? path.slice(0, index) : workspaceRoot.value
}

function exportImageBounds(selected: boolean): ImageExportBounds | null {
  if (!canvas.value) return null
  const root = canvas.value.getBoundingClientRect()
  const selectedNodes = Array.from(canvas.value.querySelectorAll('.blueprint-node.selected'))
  const nodeElements = selected && selectedNodes.length ? selectedNodes : Array.from(canvas.value.querySelectorAll('.blueprint-node'))
  const groupElements = selected ? Array.from(canvas.value.querySelectorAll('.node-group.selected')) : Array.from(canvas.value.querySelectorAll('.node-group'))
  const primary = [...nodeElements, ...groupElements].flatMap(element => {
    const bounds = relativeElementBounds(element, root)
    return bounds ? [bounds] : []
  })
  const primaryBounds = primary.length ? mergeExportBounds(primary) : null
  const connectionElements = Array.from(canvas.value.querySelectorAll('.blueprint-connection')).flatMap(element => {
    const bounds = relativeElementBounds(element, root)
    if (!bounds) return []
    return !selected || !primaryBounds || intersectsBounds(bounds, primaryBounds) ? [bounds] : []
  })
  const bounds = [...primary, ...connectionElements]
  return bounds.length ? paddedExportBounds(mergeExportBounds(bounds)) : null
}

async function exportImage(selected: boolean) {
  if (!canvas.value) return
  status.value = selected ? 'Preparing selected image export...' : 'Preparing graph image export...'
  await nextTick()
  const path = await platform.chooseExportPNGPath(graphDirectoryForExport())
  if (!path) { status.value = 'Export cancelled'; return }
  if (selected) await editor?.fitSelected(); else await editor?.resetView()
  await nextTick(); await new Promise(resolve => setTimeout(resolve, 120))
  const bounds = exportImageBounds(selected)
  const pixelRatio = projectSettingsContent.value.export.imageScale
  canvas.value.classList.add('exporting-image')
  const restoreConnectionDom = prepareConnectionDomForImageExport(canvas.value)
  try {
    const data = await toPng(canvas.value, {
      backgroundColor: '#202020',
      pixelRatio,
      cacheBust: true,
      width: bounds?.width,
      height: bounds?.height,
      style: bounds ? {
        width: `${canvas.value.getBoundingClientRect().width}px`,
        height: `${canvas.value.getBoundingClientRect().height}px`,
        transform: `translate(${-bounds.x}px, ${-bounds.y}px)`,
        transformOrigin: 'top left'
      } : undefined
    })
    const saved = await platform.savePNG(path, data)
    status.value = saved ? `Exported ${saved}` : 'Export cancelled'
  } finally {
    restoreConnectionDom()
    canvas.value.classList.remove('exporting-image')
  }
}

async function addNodeAt(typeId: string, position?: { x: number; y: number }) {
  try {
    await editor?.addNode(typeId, position ?? visibleCanvasInsertPosition(), { allowEntryNodes: !isFunctionBlueprintTab.value })
  } catch (error) {
    status.value = error instanceof Error ? error.message : String(error)
    showCanvasToast(status.value, position ?? visibleCanvasInsertPosition())
  }
}

function showCanvasToast(message: string, clientPosition?: { x: number; y: number }) {
  const rect = canvas.value?.getBoundingClientRect()
  const x = rect && clientPosition ? Math.max(12, Math.min(rect.width - 12, clientPosition.x - rect.left)) : (rect?.width ?? 100) / 2
  const y = rect && clientPosition ? Math.max(42, Math.min(rect.height - 42, clientPosition.y - rect.top - 44)) : 58
  canvasToast.value = { visible: true, message, x, y }
  if (canvasToastTimer) window.clearTimeout(canvasToastTimer)
  canvasToastTimer = window.setTimeout(() => {
    canvasToast.value.visible = false
    canvasToastTimer = undefined
  }, 1900)
}

async function loadFunctionSignatureForModuleItem(item: ModuleLibraryItem) {
  if (item.functionSource !== 'workspace' || !item.path) return normalizeFunctionSignature(functionSignature.value)
  const opened = tabs.value.find(tab => tab.path === item.path)
  if (opened?.document) return normalizeFunctionSignature(opened.document.functionSignature)
  try {
    const file = await platform.openGraph(item.path)
    if (!file) return emptyFunctionSignature()
    const parsed = parseGraphJSON(file.content) as Partial<GraphDocument>
    return normalizeFunctionSignature(parsed.functionSignature)
  } catch (error) {
    status.value = `读取函数签名失败: ${error instanceof Error ? error.message : String(error)}`
    return emptyFunctionSignature()
  }
}

async function functionMetadataForModuleItem(item: ModuleLibraryItem): Promise<FunctionNodeMetadata> {
  const source = item.functionSource ?? 'current'
  const id = source === 'workspace' ? item.functionItem?.functionId : item.functionItem?.id
  if (!id) throw new Error('函数信息尚未加载完成')
  return {
    functionRole: 'call',
    functionId: id,
    functionName: item.title,
    functionSource: source,
    functionSignature: source === 'current' ? normalizeFunctionSignature(functionSignature.value) : await loadFunctionSignatureForModuleItem(item),
    functionDescription: source === 'workspace' ? functionDescriptionByPath.value[item.path ?? ''] || undefined : undefined
  }
}

async function syncCallableFunctionsToEditor() {
  if (!editor) return
  const metadata: FunctionNodeMetadata[] = []
  for (const item of functionModuleItems.value) {
    if (isFunctionBlueprintTab.value && item.functionItem?.functionId === activeFunctionId()) continue
    try {
      metadata.push(await functionMetadataForModuleItem(item))
    } catch {
      // Workspace hydration can expose an item before its function ID is available.
    }
  }
  await editor.setCallableFunctions(metadata)
}

function isSelfFunctionReference(item: ModuleLibraryItem) {
  if (!item.functionPlaceholder || !isFunctionBlueprintTab.value) return false
  return Boolean(item.functionItem?.functionId && item.functionItem.functionId === activeFunctionId())
}

async function addModuleItemAt(item: ModuleLibraryItem, position?: { x: number; y: number }) {
  if (item.macroPlaceholder && item.path) {
    await insertMacroByPath(item.path, position)
    return
  }
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
	if (item.id === 'origin.timer.set-by-function') await syncCallableFunctionsToEditor()
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

function openModuleNodeMenu(event: MouseEvent, node: ModuleLibraryItem) {
  moduleNodeMenu.value = { visible: true, x: event.clientX, y: event.clientY, node }
}

function openModuleItemMenu(event: MouseEvent, item: ModuleLibraryItem) {
  moduleNodeMenu.value = { visible: true, x: event.clientX, y: event.clientY, node: item }
}

// 宏更新传播（v2 引用模型）：宏保存/改名后，引用图不落宏内容，磁盘引用文件天然是最新的，
// 无需读改写；只需刷新活动标签页的宏镜像（热更），其余标签页切换时由加载展开自动取最新。
async function propagateMacroUpdate(path: string) {
  macroPayloadCache.delete(path)
  const payload = await macroPayloadByPath(path)
  if (!payload?.macroId) return
  macroTitleByPath.value = { ...macroTitleByPath.value, [path]: payload.label }
  const opened = tabs.value.find(tab => tab.path === path)
  if (opened?.document) { opened.document.graphName = payload.label; opened.title = `${payload.label}.obpm` }
  if (opened?.id === activeTabId.value) macroTitle.value = payload.label
  let refreshed = 0
  if (editor && activeTab.value?.document?.macroRefs?.some(ref => ref.macroId === payload.macroId)) {
    refreshed = await editor.refreshMacroRef(payload.macroId, payload) ? 1 : 0
  }
  if (refreshed) status.value = `宏已热更到当前蓝图：${payload.label}（其他引用图打开时自动生效）`
}

async function renameMacroFromModuleMenu() {
  const item = moduleNodeMenu.value.node
  closeModuleNodeMenu()
  if (!item?.path) return
  const filename = item.path.split(/[\\\/]/).pop() ?? item.path
  const name = window.prompt('重命名宏（显示名，不影响文件名）', item.title || filename.replace(/\.obpm$/i, ''))?.trim()
  if (!name || name === item.title) return
  try {
    const file = await platform.openGraph(item.path)
    if (!file?.content) return
    const document = JSON.parse(file.content) as GraphDocument
    if (!isNativeGraphDocument(document)) return
    document.graphName = name
    const saved = await platform.saveGraph(item.path, serializeGraphDocument(item.path, document, 2))
    if (!saved) return
    await propagateMacroUpdate(item.path)
    status.value = `宏已重命名为 ${name}（引用图打开时自动显示新名）`
  } catch (error) {
    status.value = `重命名宏失败：${error instanceof Error ? error.message : String(error)}`
  }
}

async function insertMacroFromModuleMenu() {
  const item = moduleNodeMenu.value.node
  closeModuleNodeMenu()
  if (item?.path) await insertMacroByPath(item.path)
}

async function editMacroFromModuleMenu() {
  const item = moduleNodeMenu.value.node
  closeModuleNodeMenu()
  if (item?.path) await openGraph(item.path)
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
  if (!node || node.functionPlaceholder) return
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

function functionReferenceSearchKey(node: ModuleLibraryItem) {
  return `${functionReferenceSearchPrefix}${node.functionItem?.functionId || node.title}`
}

function isFunctionReferenceSearchKey(value: string) {
  return value.startsWith(functionReferenceSearchPrefix)
}

function functionReferenceHighlightKey(value: string) {
  return value.slice(functionReferenceSearchPrefix.length)
}

async function findModuleFunctionReferences(node = moduleNodeMenu.value.node) {
  closeModuleNodeMenu()
  if (!node?.functionPlaceholder) return
  if (!workspaceRoot.value) {
    status.value = '请选择工程目录'
    return
  }
  const typeId = functionReferenceSearchKey(node)
  nodeReferenceSearch.value = { visible: true, loading: true, nodeTitle: node.title, typeId, results: [] }
  try {
    const results = await platform.findNodeReferences(workspaceRoot.value, typeId)
    nodeReferenceSearch.value = { visible: true, loading: false, nodeTitle: node.title, typeId, results }
    status.value = `找到 ${results.length} 个引用蓝图`
  } catch (error) {
    nodeReferenceSearch.value.loading = false
    status.value = error instanceof Error ? error.message : String(error)
  }
}

async function openFunctionModuleItem(item = moduleNodeMenu.value.node) {
  closeModuleNodeMenu()
  if (!item?.functionPlaceholder) return
  if (!item.path) {
    status.value = '函数文件尚未保存'
    return
  }
  await openGraph(item.path)
}

async function highlightReferenceSearchTarget(searchKey: string) {
  if (!searchKey) return
  const count = isFunctionReferenceSearchKey(searchKey)
    ? await editor?.highlightFunctionReferences(functionReferenceHighlightKey(searchKey)) ?? 0
    : await editor?.highlightNodesByType(searchKey) ?? 0
  status.value = count ? `已高亮 ${count} 个引用结点` : '该蓝图中未找到引用结点'
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
    void saveProjectSettings()
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
    void saveProjectSettings()
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

async function refreshFileContextDirectory() {
  const directory = fileContextMenu.value.path
  fileContextMenu.value.visible = false
  if (!directory || !fileContextMenu.value.isDir) return
  await refreshWorkspaceDirectory(directory)
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
    void saveProjectSettings()
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
    void saveProjectSettings()
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
  <main class="application-shell" :class="applicationClasses" @pointerdown="closeModuleNodeMenu">
    <header class="menu-bar">
      <div class="menu-items">
        <div class="menu-root"><button @click.stop="toggleMenu('file')">{{ menuText.menu.file.title }}</button><div v-if="activeMenu === 'file'" class="dropdown-menu">
          <button @click="run(newGraph)">{{ menuText.menu.file.newGraph }} <kbd>Ctrl+N</kbd></button>
          <button v-if="platform.isDesktop()" @click="run(() => platform.newWindow())">{{ menuText.menu.file.newWindow }} <kbd>Ctrl+Shift+N</kbd></button>
          <div class="menu-separator"></div>
          <button @click="run(() => openGraph())">{{ menuText.menu.file.open }} <kbd>Ctrl+O</kbd></button>
          <button v-if="platform.isDesktop()" @click="run(chooseWorkspace)">{{ menuText.menu.file.openWorkspace }}</button>
          <template v-if="platform.isDesktop()">
            <div v-if="recentFiles.length" class="menu-subtitle">{{ menuText.menu.file.recent }}</div>
            <button v-for="file in recentFiles" :key="file" class="recent-item" @click="run(() => openGraph(file))">{{ file.split(/[\\/]/).pop() }}</button>
            <button :disabled="!recentFiles.length" @click="run(clearRecentFiles)">{{ menuText.menu.file.clearRecent }}</button>
          </template>
          <div class="menu-separator"></div>
          <button @click="run(() => saveGraph(false))">{{ menuText.menu.file.save }} <kbd>Ctrl+S</kbd></button>
          <button @click="run(() => saveGraph(true))">{{ menuText.menu.file.saveAs }} <kbd>Ctrl+Shift+S</kbd></button>
          <button @click="run(saveAll)">{{ menuText.menu.file.saveAll }} <kbd>Ctrl+Alt+S</kbd></button>
          <div class="menu-separator"></div>
          <button v-if="platform.isDesktop()" :disabled="!workspaceRoot" @click="run(refreshWorkspace)">{{ menuText.menu.file.refreshWorkspace }}</button>
          <button @click="run(refreshNodeLibrary)">{{ menuText.menu.file.refreshNodeLibrary }}</button>
          <div class="menu-separator"></div>
          <button @click="run(() => exportImage(true))">{{ menuText.menu.file.exportSelectedImage }} <kbd>Ctrl+Alt+R</kbd></button>
          <button @click="run(() => exportImage(false))">{{ menuText.menu.file.exportGraphImage }} <kbd>Ctrl+Shift+R</kbd></button>
          <template v-if="platform.isDesktop()"><div class="menu-separator"></div><button @click="run(quitApplication)">{{ menuText.menu.file.quit }} <kbd>Alt+F4</kbd></button></template>
        </div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('configTables')">{{ menuText.menu.configTables.title }}</button><div v-if="activeMenu === 'configTables'" class="dropdown-menu">
          <button :disabled="!workspaceRoot" @click="openConfigTableManageDialog(); activeMenu = null">{{ menuText.menu.configTables.manageDatasets }}</button>
          <button :disabled="!workspaceRoot" @click="openConfigTableImportDialog(); activeMenu = null">{{ menuText.menu.configTables.importTables }}</button>
          <button :disabled="!workspaceRoot" @click="openConfigTableDirectoriesDialog(); activeMenu = null">{{ menuText.menu.configTables.setRootDirectory }}</button>
          <button :disabled="!workspaceRoot" @click="run(loadConfigTables)">{{ menuText.menu.configTables.refresh }}</button>
        </div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('edit')">{{ menuText.menu.edit.title }}</button><div v-if="activeMenu === 'edit'" class="dropdown-menu">
          <button @click="run(() => editor?.undo())">{{ menuText.menu.edit.undo }} <kbd>Ctrl+Z</kbd></button><button @click="run(() => editor?.redo())">{{ menuText.menu.edit.redo }} <kbd>Ctrl+Y</kbd></button><div class="menu-separator"></div>
          <button @click="run(() => editor?.cut())">{{ menuText.menu.edit.cut }} <kbd>Ctrl+X</kbd></button><button @click="run(() => editor?.copy())">{{ menuText.menu.edit.copy }} <kbd>Ctrl+C</kbd></button><button @click="run(() => editor?.paste())">{{ menuText.menu.edit.paste }} <kbd>Ctrl+V</kbd></button><button @click="run(() => editor?.deleteSelected())">{{ menuText.menu.edit.delete }} <kbd>Delete</kbd></button>
          <button @click="run(() => editor?.toggleGroupSelected())">{{ menuText.menu.edit.group }} <kbd>Ctrl+G</kbd></button><div class="menu-separator"></div>
          <button @click="run(() => editor?.selectAll())">{{ menuText.menu.edit.selectAll }} <kbd>Ctrl+A</kbd></button><button @click="run(() => editor?.deselectAll())">{{ menuText.menu.edit.deselectAll }} <kbd>Ctrl+D</kbd></button>
        </div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('align')">{{ menuText.menu.align.title }}</button><div v-if="activeMenu === 'align'" class="dropdown-menu">
          <button @click="run(() => editor?.align('vertical-center'))">{{ menuText.menu.align.verticalCenter }} <kbd>V</kbd></button><button @click="run(() => editor?.align('horizontal-center'))">{{ menuText.menu.align.horizontalCenter }} <kbd>H</kbd></button>
          <button @click="run(() => editor?.align('vertical-distribute'))">{{ menuText.menu.align.verticalDistribute }} <kbd>Shift+V</kbd></button><button @click="run(() => editor?.align('horizontal-distribute'))">{{ menuText.menu.align.horizontalDistribute }} <kbd>Shift+H</kbd></button>
          <button @click="run(() => editor?.align('left'))">{{ menuText.menu.align.left }} <kbd>Shift+L</kbd></button><button @click="run(() => editor?.align('right'))">{{ menuText.menu.align.right }} <kbd>Shift+R</kbd></button><button @click="run(() => editor?.align('top'))">{{ menuText.menu.align.top }} <kbd>Shift+T</kbd></button><button @click="run(() => editor?.align('bottom'))">{{ menuText.menu.align.bottom }} <kbd>Shift+B</kbd></button>
        </div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('view')">{{ menuText.menu.view.title }}</button><div v-if="activeMenu === 'view'" class="dropdown-menu"><button @click="showLogger = !showLogger">{{ showLogger ? '✓ ' : '' }}{{ menuText.menu.view.showTestResults }} <kbd>Alt+Shift+B</kbd></button><button @click="showTools = !showTools">{{ showTools ? '✓ ' : '' }}{{ menuText.menu.view.showLeftSidebar }} <kbd>Alt+Shift+L</kbd></button><button @click="showRight = !showRight">{{ showRight ? '✓ ' : '' }}{{ menuText.menu.view.showModuleLibrary }} <kbd>Alt+Shift+R</kbd></button><div class="menu-separator"></div><div class="menu-subtitle">{{ menuText.menu.view.language }}</div><button @click="setLocale('zh-CN')">{{ currentLocale === 'zh-CN' ? '✓ ' : '' }}{{ menuText.menu.view.chinese }}</button><button @click="setLocale('en-US')">{{ currentLocale === 'en-US' ? '✓ ' : '' }}{{ menuText.menu.view.english }}</button><div class="menu-separator"></div><button @click="showSettings = true; activeMenu = null">{{ menuText.menu.view.settings }}</button></div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('blueprint')">{{ menuText.menu.blueprint.title }}</button><div v-if="activeMenu === 'blueprint'" class="dropdown-menu"><button @click="run(testGraph)">{{ menuText.menu.blueprint.validate }} <kbd>F5</kbd></button></div></div>
        <div class="menu-root"><button @click.stop="toggleMenu('help')">{{ menuText.menu.help.title }}</button><div v-if="activeMenu === 'help'" class="dropdown-menu"><button @click="showShortcuts = true; activeMenu = null">{{ menuText.menu.help.shortcuts }}</button><button @click="showAbout = true; activeMenu = null">{{ menuText.menu.help.about }}</button></div></div>
      </div>
    </header>

    <section class="workspace" :style="workspaceStyle">
      <aside class="sidebar sidebar-file-browser">
        <div class="panel workspace-panel">
          <div class="workspace-actions"><button :title="menuText.menu.file.openWorkspace" @click="chooseWorkspace">⌘</button><button :title="menuText.menu.file.refreshWorkspace" :disabled="!workspaceRoot" @click="refreshWorkspace">↻</button><button :title="`${menuText.menu.file.revealActiveFile} Ctrl+Shift+Q`" :disabled="!activeWorkspacePath" @click="revealActiveWorkspaceFile()">◎</button><button :title="menuText.menu.file.collapseWorkspace" :disabled="!expandedWorkspacePaths.size" @click="collapseWorkspaceTree">▴</button></div>
          <div class="panel-title"><span class="chevron">⌄</span> 文件浏览器<button class="panel-action" @click="chooseWorkspace">…</button></div>
          <div class="workspace-search"><input v-model="workspaceSearch" placeholder="搜索文件..." /></div>
          <div class="workspace-tree">
            <button v-for="row in visibleWorkspaceNodes" :key="row.node.path" class="workspace-entry" :class="workspaceEntryClass(row.node)" :style="{ paddingLeft: workspaceIndent(row.depth) }" :title="row.node.path" @click="toggleWorkspaceNode(row.node)" @contextmenu.stop.prevent="openFileContextMenu($event, row.node)" @dblclick="!row.node.isDir && workspaceOpen(row.node)">
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
        <div class="panel grow variable-panel" :style="variablePanelStyle"><div class="panel-title collapsible" :title="variablePanelCollapsed ? '展开变量面板' : '折叠变量面板'" @click="variablePanelCollapsed = !variablePanelCollapsed"><span class="chevron" :class="{ closed: variablePanelCollapsed }">⌄</span> 变量</div>
          <section v-for="scopeEntry in variableScopeSections" :key="scopeEntry.scope" class="variable-scope-section" :class="`scope-${scopeEntry.scope}`">
            <header class="variable-scope-header" :class="variableDropClass(`scope-${scopeEntry.scope}-default`)" :data-drop-hint="variableDropHint(`scope-${scopeEntry.scope}-default`)" @dragenter="showVariableGroupDrop($event, `scope-${scopeEntry.scope}-default`, 'default', scopeEntry.scope)" @dragover="showVariableGroupDrop($event, `scope-${scopeEntry.scope}-default`, 'default', scopeEntry.scope)" @dragleave="leaveVariableGroupDrop($event, `scope-${scopeEntry.scope}-default`)" @drop="dropVariableIntoGroup($event, `scope-${scopeEntry.scope}-default`, 'default', scopeEntry.scope)">
              <span class="variable-scope-icon">{{ scopeEntry.scope === 'instance' ? 'G' : 'L' }}</span>
              <strong class="variable-scope-title">{{ scopeEntry.title }}</strong>
              <span class="variable-scope-count">{{ scopeEntry.variables.length }}</span>
            </header>
            <div class="variable-scope-actions">
              <button :disabled="scopeEntry.scope === 'instance' && isFunctionBlueprintTab" @click="addVariable('default', scopeEntry.scope)">＋ 变量</button>
              <button :disabled="scopeEntry.scope === 'instance' && isFunctionBlueprintTab" @click="addVariableGroup(scopeEntry.scope)">▣＋ 分组</button>
            </div>
            <div v-if="scopeEntry.scope === 'instance' && isFunctionBlueprintTab" class="variable-scope-notice">函数蓝图不支持全局变量</div>
            <template v-if="scopeEntry.scope !== 'instance' || !isFunctionBlueprintTab || scopeEntry.variables.length">
              <section v-for="entry in scopeEntry.groups" :key="`${scopeEntry.scope}-${entry.group.id}`" class="variable-group" :class="[{ 'variable-group-flat': !scopeEntry.hasCustomGroups }, variableDropClass(`group-${scopeEntry.scope}-${entry.group.id}`)]" :data-drop-hint="variableDropHint(`group-${scopeEntry.scope}-${entry.group.id}`)" @dragenter="showVariableGroupDrop($event, `group-${scopeEntry.scope}-${entry.group.id}`, entry.group.id, scopeEntry.scope)" @dragover="showVariableGroupDrop($event, `group-${scopeEntry.scope}-${entry.group.id}`, entry.group.id, scopeEntry.scope)" @dragleave="leaveVariableGroupDrop($event, `group-${scopeEntry.scope}-${entry.group.id}`)" @drop="dropVariableIntoGroup($event, `group-${scopeEntry.scope}-${entry.group.id}`, entry.group.id, scopeEntry.scope)">
                <div v-if="scopeEntry.hasCustomGroups" class="variable-group-header">
                  <span class="variable-group-name">{{ entry.group.name }}</span><small>{{ entry.variables.length }}</small>
                  <button :title="`在此组添加${scopeEntry.title}`" :disabled="scopeEntry.scope === 'instance' && isFunctionBlueprintTab" @click="addVariable(entry.group.id, scopeEntry.scope)">＋</button>
                  <button v-if="entry.group.id !== 'default'" title="重命名分组" @click="renameVariableGroup(entry.group)">✎</button>
                  <button v-if="entry.group.id !== 'default'" title="删除分组" @click="removeVariableGroup(entry.group)">×</button>
                </div>
                <div class="variable-group-list">
                  <div v-for="variable in entry.variables" :key="variable.id" class="variable-row" :class="{ selected: selectedVariableId === variable.id, 'variable-instance': scopeEntry.scope === 'instance' }" :style="socketStyle(variable.type)" :title="`${variable.name} · ${variable.type} · ${scopeEntry.title}`" draggable="true" @click="selectVariable(variable)" @dragstart="startVariableDrag($event, variable)" @dragend="endVariableDrag">
                    <div class="variable-heading"><span class="variable-type-dot"></span><span class="variable-name">{{ variable.name }}</span><span class="variable-kind">{{ variable.type }}</span><button title="Get" @click.stop="createVariableNode(variable, 'get')">G</button><button title="Set" @click.stop="createVariableNode(variable, 'set')">S</button><button title="Delete" @click.stop="removeVariable(variable)">×</button></div>
                  </div>
                  <button v-if="!entry.variables.length && entry.group.id !== 'default'" class="empty-variable-group" :disabled="scopeEntry.scope === 'instance' && isFunctionBlueprintTab" @click="addVariable(entry.group.id, scopeEntry.scope)">＋ 添加{{ scopeEntry.title }}</button>
                </div>
              </section>
            </template>
          </section>
        </div>
        <div class="panel-height-splitter" @pointerdown="beginVariablePanelHeightResize"></div>
        <div class="panel grow detail-panel sidebar-detail-panel" :style="detailPanelStyle">
          <div class="panel-title collapsible" :title="detailPanelCollapsed ? '展开详情面板' : '折叠详情面板'" @click="detailPanelCollapsed = !detailPanelCollapsed"><span class="chevron" :class="{ closed: detailPanelCollapsed }">⌄</span> 详情</div>
          <div v-if="isFunctionBlueprintTab && !selectedNode && !selectedVariable" class="node-detail function-signature-editor">
            <label>{{ menuText.detail.functionTitle }}<input v-model="functionTitle" :placeholder="menuText.detail.functionTitlePlaceholder" :title="menuText.detail.functionTitleLockedHint" @change="syncFunctionTitleToGraph" /></label>
            <label>{{ menuText.detail.functionCategory }}
              <div class="function-category-combo" @focusin="openFunctionCategoryOptions" @focusout="closeFunctionCategoryOptions">
                <input v-model="functionCategory" :placeholder="menuText.detail.functionCategoryPlaceholder" @input="openFunctionCategoryOptions" @change="syncFunctionCategoryToGraph" />
                <button type="button" title="选择函数类型" @click="functionCategoryDropdownOpen = !functionCategoryDropdownOpen">▾</button>
                <div v-if="functionCategoryDropdownOpen" class="function-category-options">
                  <button v-for="option in functionCategoryOptions" :key="option" type="button" class="function-category-option" :class="{ selected: normalizeFunctionCategory(functionCategory) === option }" @pointerdown.prevent @click="selectFunctionCategory(option)">{{ option }}</button>
                </div>
              </div>
            </label>
            <div class="detail-section-title">函数签名</div>
            <div class="function-terminal-actions">
              <button v-if="metrics.functionEntries === 0" :title="menuText.detail.restoreFunctionEntryHint" @click="addFunctionEntryNodeToGraph">{{ menuText.detail.restoreFunctionEntry }}</button>
              <button :title="menuText.detail.addFunctionReturnHint" @click="addFunctionReturnNodeToGraph">{{ menuText.detail.addFunctionReturn }}</button>
            </div>
            <section class="signature-port-section">
              <header><span>输入参数</span><small>{{ functionSignature.inputs.length }}</small></header>
              <div v-for="port in functionSignature.inputs" :key="port.id" class="signature-port-row">
                <input v-model="port.name" placeholder="参数名" @change="syncFunctionSignatureToGraph" />
                <select v-model="port.type" @change="syncFunctionSignatureToGraph"><option v-for="option in functionSignatureTypeOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select>
                <button title="删除参数" @click="removeFunctionSignaturePort('inputs', port)">×</button>
              </div>
              <button class="add-signature-port" @click="addFunctionSignaturePort('inputs')"><span aria-hidden="true">＋</span>{{ menuText.detail.addInputParameter }}</button>
            </section>
            <section class="signature-port-section">
              <header><span>输出参数</span><small>{{ functionSignature.outputs.length }}</small></header>
              <div v-for="port in functionSignature.outputs" :key="port.id" class="signature-port-row">
                <input v-model="port.name" placeholder="参数名" @change="syncFunctionSignatureToGraph" />
                <select v-model="port.type" @change="syncFunctionSignatureToGraph"><option v-for="option in functionSignatureTypeOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select>
                <button title="删除参数" @click="removeFunctionSignaturePort('outputs', port)">×</button>
              </div>
              <button class="add-signature-port" @click="addFunctionSignaturePort('outputs')"><span aria-hidden="true">＋</span>{{ menuText.detail.addOutputParameter }}</button>
            </section>
          </div>
          <div v-else-if="selectedMacroRefInfo && !selectedNode && !selectedVariable" class="node-detail macro-instance-detail">
            <div class="detail-section-title">宏引用</div>
            <template v-if="!selectedMacroRefInfo.missing">
              <label>宏名<input :value="selectedMacroRefInfo.label" disabled /></label>
              <label>宏 ID<input :value="selectedMacroRefInfo.macroId" disabled :title="selectedMacroRefInfo.macroId" /></label>
              <label>源宏文件（实时解析）<input :value="workspaceRelativeHint(selectedMacroRefInfo.pathHint, workspaceRoot) || selectedMacroRefInfo.pathHint || '（本次未解析到）'" disabled :title="selectedMacroRefInfo.pathHint" /></label>
              <small class="variable-scope-hint">这是对源宏的引用（{{ selectedMacroRefInfo.nodeCount }} 个镜像节点）：引用只保存宏 ID 与外框/边界，宏名和文件路径都实时从宏源解析——改名/移动文件不影响引用。源宏保存后此图立即热更，未打开的图切换标签页时自动取最新。宏在图中不可修改、不可拆分，仅支持整体移动（拖动宏框）与整体删除（选中宏框按 Delete）。</small>
            </template>
            <template v-else>
              <label>宏 ID<input :value="selectedMacroRefInfo.macroId" disabled :title="selectedMacroRefInfo.macroId" /></label>
              <small class="variable-scope-hint" style="color:#f0a95c">⚠ 引用失效：工作区里找不到这个 macroId 对应的宏文件（可能已删除、移动到工作区外，或 macroId 不匹配）。画布上显示「宏：未找到」框，编译/校验会报错。可删除此宏框移除引用，或把宏文件恢复到工作区后重新打开本图。</small>
            </template>
            <button class="apply-properties" @click="openMacroSourceFromRef">打开源宏</button>
          </div>
          <div v-else-if="isMacroBlueprintTab && !selectedNode && !selectedVariable" class="node-detail macro-detail">
            <div class="detail-section-title">宏</div>
            <label>宏名<input v-model="macroTitle" placeholder="模块库中显示的名字（保存后生效）" @change="syncMacroTitleToGraph" /></label>
            <label>宏 ID<input :value="activeTab?.document?.macroId || macroIdByPath[activeTab?.path ?? ''] || ''" disabled :title="activeTab?.document?.macroId" /></label>
            <label>宏文件<input :value="activeTab?.path" disabled /></label>
            <small class="variable-scope-hint">宏名仅用于显示（模块库与引用图宏框标签），文件名可独立修改；宏 ID 是引用图的稳定锚点。其他蓝图引用此宏（不复制内容），保存后所有引用图自动更新。</small>
          </div>
          <div v-else-if="selectedVariable" class="node-detail variable-detail"><div class="detail-section-title">变量属性</div><label>Variable ID<input :value="selectedVariable.id" disabled /></label><label>名称<input v-model="selectedVariable.name" @input="previewVariableName(selectedVariable)" /></label><label>作用域<select :value="variableScope(selectedVariable)" @change="changeVariableScope(selectedVariable, $event)"><option value="execution">局部（每次执行重置）</option><option value="instance" :disabled="isFunctionBlueprintTab">全局（同一蓝图实例共享）</option></select></label><small v-if="variableScope(selectedVariable) === 'instance'" class="variable-scope-hint">并发执行会共享当前值；单次读取和写入线程安全。</small><label>类型<select v-model="selectedVariable.type" @change="changeVariableType(selectedVariable)"><option value="boolean">Boolean</option><option value="integer">Integer</option><option value="float">Float</option><option value="string">String</option><option value="array">Array</option><option value="timerhandle">Timer Handle</option></select></label><label>分组<select v-model="selectedVariable.groupId"><option v-for="group in variableGroupsForScope(variableGroups, variableScope(selectedVariable))" :key="group.id" :value="group.id">{{ group.name }}</option></select></label><label>说明<textarea v-model="selectedVariable.description" rows="4" placeholder="变量用途和约束"></textarea></label><label v-if="selectedVariable.type !== 'timerhandle'">默认值<input v-if="selectedVariable.type === 'boolean'" v-model="selectedVariable.defaultValue" type="checkbox" /><input v-else-if="selectedVariable.type === 'string'" v-model="selectedVariable.defaultValue" type="text" /><input v-else-if="selectedVariable.type === 'array'" :value="Array.isArray(selectedVariable.defaultValue) ? selectedVariable.defaultValue.join(', ') : ''" placeholder="1, 2, text" @change="setVariableArrayDefault(selectedVariable, $event)" /><input v-else-if="selectedVariable.type === 'integer'" :value="selectedVariable.defaultValue" type="text" inputmode="numeric" @input="setVariableIntegerDefault(selectedVariable, $event)" /><input v-else v-model.number="selectedVariable.defaultValue" type="number" step="any" /></label><button class="apply-properties" @click="updateVariable(selectedVariable)">应用变量属性</button><button class="delete-properties" @click="removeVariable(selectedVariable)">删除变量</button></div>
          <div v-else-if="selectedNode" class="node-detail"><label>Node ID<input :value="selectedNode.id" disabled /></label><label>Type<input :value="selectedNode.typeId" disabled /></label><label>Title<input :value="selectedNode.label" readonly /></label><label v-if="selectedNode.description">说明<textarea :value="selectedNode.description" rows="4" readonly></textarea></label></div>
          <div v-else class="empty-detail">选择节点或变量以查看属性</div>
        </div>
      </aside>
      <div v-show="showTools" class="left-tools-splitter" @pointerdown="beginLeftToolsResize"></div>

      <section class="editor-column">
         <div class="tab-strip-wrap">
           <button class="tab-scroll-arrow left" @click="scrollTabStrip(-1)">◀</button>
           <div ref="tabStrip" class="tab-strip" @wheel.prevent="(e: WheelEvent) => { const s = e.currentTarget as HTMLElement; s.scrollLeft += e.deltaY; }">
             <div v-for="(tab, idx) in tabs" :key="tab.id" class="graph-tab" :class="{ active: tab.id === activeTabId, 'drag-over': tabDragOverIndex === idx, 'save-blocked': tab.saveBlocked }" draggable="true" @click="switchTab(tab.id)" @dragstart="onTabDragStart($event, idx)" @dragover="onTabDragOver($event, idx)" @dragleave="onTabDragLeave" @drop="onTabDrop($event, idx)" @dragend="onTabDragEnd"><span class="tab-mark"></span>{{ tab.title }}<span v-if="tab.saveBlocked" class="save-blocked-mark" title="Fatal graph issue blocks saving">⛔</span><span v-if="tab.dirty" class="dirty-mark">●</span><button class="tab-close" @click="closeTab(tab.id, $event)">×</button></div>
             <button class="new-tab" @click="newGraph">＋</button>
           </div>
           <button class="tab-scroll-arrow right" @click="scrollTabStrip(1)">▶</button>
         </div>
        <div class="canvas-wrap" @contextmenu.prevent @dragenter="allowNodeDrop" @dragover="allowNodeDrop" @drop.prevent="dropNode"><div ref="canvas" class="rete-canvas"></div><div v-if="canvasToast.visible" class="canvas-toast" :style="{ left: `${canvasToast.x}px`, top: `${canvasToast.y}px` }">{{ canvasToast.message }}</div><div class="canvas-toolbar"><button :title="menuText.toolbar.addComment" @click="editor?.commentAroundSelection()">🗒＋</button><button :title="menuText.toolbar.resetView" @click="editor?.resetView()">⌂</button></div><div v-if="canvasSearchVisible" class="canvas-search"><input ref="canvasSearchRef" v-model="canvasSearchQuery" type="text" placeholder="搜索画布内容...（节点/注释/参数值）" @keydown.esc.stop="closeCanvasSearch" @keydown.enter.prevent="canvasSearchResults[0] && jumpToCanvasSearchResult(canvasSearchResults[0].nodeId)" /><button class="canvas-search-close" @click="closeCanvasSearch">×</button><div v-if="canvasSearchResults.length" class="canvas-search-results"><button v-for="result in canvasSearchResults" :key="result.nodeId" @click="jumpToCanvasSearchResult(result.nodeId)"><span class="canvas-search-title">{{ result.title }}</span><span class="canvas-search-detail">{{ result.detail }}</span></button></div></div><div class="canvas-hint">{{ menuText.canvas.hint }}</div></div>
        <div v-show="showLogger" class="logger-panel bottom-panel" :class="{ collapsed: testPanelCollapsed }" :style="testPanelStyle">
          <div class="bottom-panel-resizer" @pointerdown="beginTestPanelResize"></div>
          <div class="bottom-panel-title">
            <strong class="bottom-panel-target">{{ menuText.validation.title }}</strong>
            <small>{{ validationIssueCountLabel }}</small>
            <button class="bottom-panel-action" :title="menuText.validation.rerunTitle" @click="testGraph">{{ menuText.toolbar.test }}</button>
            <button class="bottom-panel-tool-button" :title="testPanelCollapsed ? menuText.validation.expandTitle : menuText.validation.collapseTitle" @click="toggleTestPanel">{{ testPanelCollapsed ? '▴' : '▾' }}</button>
            <button class="bottom-panel-tool-button close" :title="menuText.validation.closeTitle" @click="showLogger = false">×</button>
          </div>
          <div v-show="!testPanelCollapsed" class="logger-results">
            <div v-if="!validationIssues.length" class="logger-line">没有发现蓝图问题。</div>
            <section v-for="group in groupedValidationIssues" :key="group.key" class="logger-issue-group">
              <header>{{ group.label }}</header>
              <button v-for="item in group.items" :key="validationIssueKey(item.issue, item.index)" class="logger-issue" :class="[item.issue.severity, { selected: selectedValidationIssueKey === validationIssueKey(item.issue, item.index) }]" @click="queueSelectIssue(item.issue, item.index)" @dblclick.stop.prevent="highlightIssue(item.issue, item.index)">
                <strong>{{ issueSeverityLabel(item.issue) }}</strong>
                <span class="logger-issue-message">{{ item.issue.message }}</span>
                <span v-if="item.issue.blocksSave" class="logger-issue-blocks-save">禁止保存</span>
                <span class="logger-issue-meta"><b>{{ menuText.validation.nodes }}</b>{{ issueNodeLabel(item.issue) }}</span>
                <small><b>{{ menuText.validation.code }}</b>{{ item.issue.code }}</small>
              </button>
            </section>
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
          <div class="panel-title"><span class="chevron">⌄</span> {{ menuText.module.title }}</div>
          <div class="search-box">⌕ <input v-model="moduleSearch" :placeholder="menuText.module.searchPlaceholder" /></div>
          <div class="module-list">
            <section v-for="[category, items] in categories" :key="category" class="module-category-section" :class="{ open: isModuleCategoryExpanded(category), 'function-module-category': isFunctionModuleCategory(items) }">
              <button class="module-category" :aria-expanded="isModuleCategoryExpanded(category)" @click="toggleModuleCategory(category)">
                <span class="module-arrow">{{ isModuleCategoryExpanded(category) ? '⌄' : '›' }}</span>
                <span class="module-category-icon" :class="isFunctionModuleCategory(items) ? 'function-icon' : 'node-icon'">{{ isFunctionModuleCategory(items) ? 'ƒ' : '' }}</span>
                <span class="module-category-name" v-html="renderModuleSearchText(displayModuleCategoryName(category))"></span>
                <small>{{ items.length }}</small>
              </button>
              <div v-if="isModuleCategoryExpanded(category)" class="module-items">
                <button v-for="item in items" :key="item.id" class="module-item" :class="{ 'function-placeholder': item.functionPlaceholder }" :title="item.path || item.title" @click="selectFunctionLibraryItem(item)" @pointerdown.stop="beginModuleItemPointerDrag($event, item)" @contextmenu.stop.prevent="openModuleItemMenu($event, item)" @dblclick="addModuleItemAt(item)"><span class="module-item-icon">{{ item.functionPlaceholder ? 'ƒ' : '◇' }}</span><span class="module-item-title" v-html="renderModuleSearchText(item.title)"></span><small v-if="item.functionPlaceholder">{{ item.functionSource === 'workspace' ? menuText.module.workspaceFunctionLibrary : menuText.module.currentBlueprintFunctions }}</small></button>
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
      <template v-if="moduleNodeMenu.node?.macroPlaceholder">
        <button @click="insertMacroFromModuleMenu()">插入到当前图</button>
        <button @click="editMacroFromModuleMenu()">编辑宏</button>
        <button @click="renameMacroFromModuleMenu()">重命名宏</button>
      </template>
      <button v-else-if="moduleNodeMenu.node?.functionPlaceholder" @click="openFunctionModuleItem()">编辑函数</button>
      <button v-if="!moduleNodeMenu.node?.macroPlaceholder && moduleNodeMenu.node?.functionPlaceholder && moduleNodeMenu.node.functionSource === 'workspace'" @click="moduleNodeMenu.node && openFunctionAnnotationDialog(moduleNodeMenu.node)">编辑函数注解</button>
      <button v-if="!moduleNodeMenu.node?.macroPlaceholder && moduleNodeMenu.node?.functionPlaceholder" @click="findModuleFunctionReferences()">查找所有引用</button>
      <template v-else>
        <button @click="findModuleNodeReferences()">查找所有引用</button>
        <button @click="moduleNodeMenu.node && openNodeAnnotationDialog(moduleNodeMenu.node.id)">编辑节点注解</button>
      </template>
    </div>
    <div v-if="fileContextMenu.visible" class="file-context-menu" :style="{ left: `${fileContextMenu.x}px`, top: `${fileContextMenu.y}px` }" @pointerdown.stop>
      <button v-if="!fileContextMenu.isDir" @click="openFileContextGraph">{{ fileContextMenu.isFunction ? '打开函数' : '打开蓝图' }}</button>
      <button v-if="!fileContextMenu.isDir && isMacroSourcePath(fileContextMenu.path)" @click="insertMacroFromContextMenu">作为宏插入当前图</button>
      <button v-if="fileContextMenu.isDir" @click="refreshFileContextDirectory">刷新目录</button>
      <button v-if="fileContextMenu.isDir" @click="createBlueprintInFileContext">新建蓝图</button>
      <button v-if="fileContextMenu.isDir" @click="createFunctionInFileContext">新建函数</button>
      <button v-if="fileContextMenu.isDir" @click="createMacroInFileContext">新建宏</button>
      <button @click="revealFileContextInFolder">在资源管理器中定位</button>
    </div>
    <div v-if="showSettings" class="settings-backdrop" @pointerdown.self="showSettings = false">
      <section class="settings-dialog">
        <header><strong>{{ menuText.settings.title }}</strong><button @click="showSettings = false">×</button></header>
        <div class="settings-body">
          <label><span>{{ menuText.settings.language }}</span><select :value="currentLocale" @change="setLocale(($event.target as HTMLSelectElement).value as LocaleId)"><option value="zh-CN">中文</option><option value="en-US">English</option></select></label>
          <label><span>{{ menuText.settings.uiScale }}</span><select :value="projectSettingsContent.appearance.uiScale" @change="updateProjectSettings(settings => { settings.appearance.uiScale = ($event.target as HTMLSelectElement).value as UiScale })"><option value="small">{{ menuText.settings.small }}</option><option value="normal">{{ menuText.settings.normal }}</option><option value="large">{{ menuText.settings.large }}</option></select></label>
          <label><span>{{ menuText.settings.moduleScale }}</span><select :value="projectSettingsContent.appearance.moduleScale" @change="updateProjectSettings(settings => { settings.appearance.moduleScale = ($event.target as HTMLSelectElement).value as UiScale })"><option value="small">{{ menuText.settings.small }}</option><option value="normal">{{ menuText.settings.normal }}</option><option value="large">{{ menuText.settings.large }}</option></select></label>
          <label><span>{{ menuText.settings.nodeScale }}</span><select :value="projectSettingsContent.appearance.nodeScale" @change="updateProjectSettings(settings => { settings.appearance.nodeScale = ($event.target as HTMLSelectElement).value as NodeScale })"><option value="normal">{{ menuText.settings.normal }}</option><option value="large">{{ menuText.settings.large }}</option></select></label>
          <label><span>{{ menuText.settings.imageExportScale }}</span><select :value="projectSettingsContent.export.imageScale" @change="updateProjectSettings(settings => { settings.export.imageScale = Number(($event.target as HTMLSelectElement).value) as ImageExportScale })"><option :value="1">1x</option><option :value="2">2x</option><option :value="4">4x</option></select></label>
          <label class="settings-check"><input type="checkbox" :checked="projectSettingsContent.export.showGrid" @change="updateProjectSettings(settings => { settings.export.showGrid = ($event.target as HTMLInputElement).checked })" /><span>{{ menuText.settings.showGrid }}</span></label>
          <label class="settings-check"><input type="checkbox" :checked="projectSettingsContent.explorer.revealActiveFile" @change="updateProjectSettings(settings => { settings.explorer.revealActiveFile = ($event.target as HTMLInputElement).checked })" /><span>{{ menuText.settings.revealActiveFile }}</span></label>
          <label class="settings-check"><input type="checkbox" :checked="projectSettingsContent.editor.validateBeforeSave" @change="updateProjectSettings(settings => { settings.editor.validateBeforeSave = ($event.target as HTMLInputElement).checked })" /><span>{{ menuText.settings.validateBeforeSave }}</span></label>
          <label class="settings-check"><input type="checkbox" :checked="updateState.autoCheck" @change="setAutoCheckUpdates(($event.target as HTMLInputElement).checked)" /><span>{{ menuText.settings.autoCheckUpdates }}</span></label>
          <button class="settings-action" :disabled="updateState.checking" @click="checkForUpdates(true)">{{ updateState.checking ? menuText.update.checking : menuText.settings.checkUpdatesNow }}</button>
        </div>
        <footer><small>{{ projectSettingsPath || 'originblueprint.project' }}</small><button @click="showSettings = false">{{ menuText.settings.close }}</button></footer>
      </section>
    </div>
    <div v-if="updateState.visible" class="update-backdrop" @pointerdown.self="closeUpdateDialog">
      <section class="update-dialog">
        <header><strong>{{ menuText.update.title }}</strong><button @click="closeUpdateDialog">×</button></header>
        <p>{{ menuText.update.available.replace('{version}', updateState.latestVersion) }}</p>
        <dl><dt>{{ menuText.update.currentVersion }}</dt><dd>{{ updateState.currentVersion }}</dd><dt>{{ menuText.update.latestVersion }}</dt><dd>{{ updateState.latestVersion }}</dd></dl>
        <pre v-if="updateState.notes">{{ updateState.notes }}</pre>
        <footer><button @click="closeUpdateDialog">{{ menuText.update.remindLater }}</button><button class="primary" @click="openUpdateRelease">{{ menuText.update.openRelease }}</button></footer>
      </section>
    </div>
    <div v-if="recoveryDialog.visible && recoveryDialog.snapshot" class="unsaved-close-backdrop">
      <section class="unsaved-close-dialog">
        <header>发现蓝图恢复快照</header>
        <p>{{ recoveryDialog.snapshot.sourcePath || '未命名蓝图' }}</p>
        <p>创建时间：{{ recoveryDialog.snapshot.createdAt }}</p>
        <p>恢复会打开一个受保护的未保存标签，不会覆盖原文件。</p>
        <footer>
          <button class="primary" @click="restoreRecoverySnapshot">恢复</button>
          <button @click="keepRecoverySnapshot">保留</button>
          <button @click="deleteRecoverySnapshot">删除</button>
        </footer>
      </section>
    </div>
    <div v-if="compatibilitySaveDialog.visible" class="unsaved-close-backdrop">
      <section class="unsaved-close-dialog">
        <header>蓝图存在兼容性丢失风险</header>
        <p v-if="compatibilitySaveDialog.fatal">蓝图恢复未完整完成。为保护原文件，只能另存为恢复副本。</p>
        <p v-else>编辑器无法完整恢复 {{ compatibilitySaveDialog.droppedNodes }} 个结点、{{ compatibilitySaveDialog.droppedConnections }} 条连线和 {{ compatibilitySaveDialog.alteredNodes }} 个结点属性。默认另存为恢复副本，不会改动原文件。</p>
        <footer>
          <button class="primary" @click="resolveCompatibilitySaveAction('copy')">另存恢复副本</button>
          <button v-if="compatibilitySaveDialog.forceAllowed" @click="resolveCompatibilitySaveAction('force')">强制覆盖原文件</button>
          <button @click="resolveCompatibilitySaveAction('cancel')">取消</button>
        </footer>
      </section>
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
    <div v-if="configTableDirectoriesDialog" class="about-backdrop"><section class="about-dialog config-directory-dialog">
      <header @pointerdown="beginDialogDrag"><strong>设置表元目录</strong><button @click="configTableDirectoriesDialog = false">×</button></header>
      <div class="node-detail annotation-detail">
        <div class="detail-section-title">表元目录（所有可导入的配置表只从这些目录获取）</div>
        <div v-for="(directory, index) in configTableDirectoriesDraft" :key="index" class="config-dir-row">
          <input v-model="configTableDirectoriesDraft[index]" placeholder="如 configs" />
          <button class="dialog-button ghost small" title="浏览选择目录" @click="browseConfigTableDirectory(index)">…</button>
          <button class="dialog-button ghost small danger" title="移除目录" @click="configTableDirectoriesDraft.splice(index, 1)">×</button>
        </div>
        <button class="dialog-button ghost small config-add-dir" @click="addConfigTableDirectory">＋ 添加目录</button>
        <small class="variable-scope-hint">工程内目录保存为相对路径（随 .obproj 共享）；至少保留一个目录。</small>
      </div>
      <footer class="dialog-footer-actions"><button class="dialog-button primary" @click="saveConfigTableDirectories">保存</button><button class="dialog-button ghost" @click="configTableDirectoriesDialog = false">取消</button></footer>
    </section></div>
    <div v-if="configTableImportDialog" class="about-backdrop"><section class="about-dialog config-table-dialog">
      <header @pointerdown="beginDialogDrag"><strong>批量导入配置表</strong><button @click="configTableImportDialog = false">×</button></header>
      <div class="node-detail annotation-detail">
        <div class="detail-section-title">通用默认表头识别规则（新导入的表按此识别）</div>
        <div class="config-defaults-grid">
          <label>id 表头所在行<input v-model.number="configTableImportDraft.defaults.idMatchRow" type="number" min="1" /></label>
          <label>名称表头所在行<input v-model.number="configTableImportDraft.defaults.nameMatchRow" type="number" min="1" /></label>
          <label>id 表头列所含关键字<input v-model="configTableImportDraft.defaults.idKeyword" placeholder="表头包含的字，如 id" /></label>
          <label>名称表头列所含关键字<input :value="configTableImportDraft.defaults.nameKeywords.join(', ')" placeholder="如 name, 名称（逗号分隔，任一命中）" @change="configTableImportDraft.defaults.nameKeywords = ($event.target as HTMLInputElement).value.split(/[,，]/).map(item => item.trim()).filter(Boolean)" /></label>
        </div>

        <div class="detail-section-title config-table-list-title">
          <span>发现的表（已有数据集 {{ configTableImportDraft.datasets.length }} / 共 {{ configTableScan.length }}）</span>
          <span class="config-table-bulk">
            <span class="config-preview-mode">
              <button type="button" class="dialog-button small" :class="configTableListView === 'all' ? 'primary' : 'ghost'" @click="setConfigTableListView('all')">全部</button>
              <button type="button" class="dialog-button small" :class="configTableListView === 'imported' ? 'primary' : 'ghost'" @click="setConfigTableListView('imported')">已导入</button>
              <button type="button" class="dialog-button small" :class="configTableListView === 'pending' ? 'primary' : 'ghost'" @click="setConfigTableListView('pending')">未导入 {{ pendingConfigTableCount() }}</button>
            </span>
            <input v-model="configTableSearch" class="config-table-search" placeholder="🔍 搜索表名" />
            <button class="dialog-button ghost small" @click="setAllPendingPicks(true)">全选</button>
            <button class="dialog-button ghost small" @click="setAllPendingPicks(false)">全不选</button>
          </span>
        </div>
        <div class="config-table-scroll">
          <div v-if="!displayConfigTableScan().length" class="empty-panel">{{ configTableSearch ? '没有匹配的表' : configTableListView === 'imported' ? '还没有任何数据集；切换到“全部”或“未导入”勾选要导入的表。' : configTableListView === 'pending' ? '表元目录中的表都已建立数据集。' : '表元目录中没有 xlsx/csv 文件；可在“设置表元目录”中换目录或放入文件后重新打开本对话框。' }}</div>
          <div v-for="table in displayConfigTableScan()" :key="table.key" class="config-table-row" :class="{ warned: table.warning }">
            <div class="config-table-head">
              <label class="config-table-check" :title="hasDatasetForTable(table) ? '已有数据集的表在这里只读；删除/改名/筛选请用“配置表 → 数据集管理”' : ''"><input type="checkbox" :disabled="hasDatasetForTable(table)" :checked="hasDatasetForTable(table) || configTablePendingPicks.has(table.key)" @change="togglePendingPick(table, ($event.target as HTMLInputElement).checked)" /></label>
              <strong class="config-table-key">{{ table.key }}</strong>
              <small class="config-table-summary">{{ configTableSummary(table) }}</small>
              <button v-if="configTablePendingPicks.has(table.key) && !hasDatasetForTable(table)" class="dialog-button small" :class="configTablePendingExpanded.has(table.key) ? 'primary' : 'ghost'" @click="togglePendingExpanded(table)">识别设置</button>
              <small v-if="hasDatasetForTable(table) && !table.missing" class="config-table-imported-hint">已导入（在“数据集管理”中编辑）</small>
            </div>
            <div v-if="configTablePendingExpanded.has(table.key) && configTablePendingDrafts[table.key]" class="config-table-override">
              <div class="dataset-field-grid dataset-rule-grid">
                <label title="在第几行里找 id 表头">id 表头所在行<input v-model.number="configTablePendingDrafts[table.key]!.idMatchRow" type="number" min="0" placeholder="默认" @change="loadPendingTablePreview(table)" /></label>
                <label title="该行的哪一列算 id 列：表头包含该字">id 表头关键字<input v-model="configTablePendingDrafts[table.key]!.idKeyword" placeholder="默认" @change="loadPendingTablePreview(table)" /></label>
                <label title="在第几行里找名称表头">名称表头所在行<input v-model.number="configTablePendingDrafts[table.key]!.nameMatchRow" type="number" min="0" placeholder="默认" @change="loadPendingTablePreview(table)" /></label>
                <label title="该行的哪一列算名称列：表头包含该字，逗号分隔多个">名称表头关键字<input v-model="configTablePendingDrafts[table.key]!.nameKeyword" placeholder="默认，逗号分隔" @change="loadPendingTablePreview(table)" /></label>
              </div>
              <div class="dataset-filter-row">
                <span class="config-preview-mode">
                  <button type="button" class="dialog-button small" :class="configTablePendingDrafts[table.key]!.filter.mode === 'text' ? 'primary' : 'ghost'" @click="configTablePendingDrafts[table.key]!.filter.mode = 'text'">内容筛选</button>
                  <button type="button" class="dialog-button small" :class="configTablePendingDrafts[table.key]!.filter.mode === 'range' ? 'primary' : 'ghost'" @click="configTablePendingDrafts[table.key]!.filter.mode = 'range'">ID范围</button>
                  <button type="button" class="dialog-button small" :class="configTablePendingDrafts[table.key]!.filter.mode === 'compare' ? 'primary' : 'ghost'" @click="configTablePendingDrafts[table.key]!.filter.mode = 'compare'">ID比较</button>
                </span>
                <template v-if="configTablePendingDrafts[table.key]!.filter.mode === 'text'">
                  <input type="text" class="dataset-filter-text" v-model="configTablePendingDrafts[table.key]!.filter.text" placeholder="筛选内容（该列包含的文字）" />
                  <select v-model="configTablePendingDrafts[table.key]!.filter.column">
                    <option value="all">全部列</option>
                    <option value="id">仅 id 列</option>
                    <option value="name">仅名称列</option>
                    <option value="custom">指定列…</option>
                  </select>
                </template>
                <template v-else-if="configTablePendingDrafts[table.key]!.filter.mode === 'range'">
                  <input type="text" class="dataset-filter-number" v-model="configTablePendingDrafts[table.key]!.filter.rangeMin" placeholder="起始 ID" />
                  <span class="config-preview-sep">至</span>
                  <input type="text" class="dataset-filter-number" v-model="configTablePendingDrafts[table.key]!.filter.rangeMax" placeholder="结束 ID" />
                </template>
                <template v-else>
                  <select v-model="configTablePendingDrafts[table.key]!.filter.compareOp">
                    <option>&gt;</option>
                    <option>&gt;=</option>
                    <option>&lt;</option>
                    <option>&lt;=</option>
                  </select>
                  <input type="text" class="dataset-filter-number" v-model="configTablePendingDrafts[table.key]!.filter.compareValue" placeholder="数值" />
                </template>
                <small>{{ configTablePreviewLoading.has(table.key) ? '加载中…' : `匹配 ${pendingPreviewEntries(table).matched}/${pendingPreviewEntries(table).total} 条` }}</small>
              </div>
              <div v-if="configTablePendingDrafts[table.key]!.filter.mode === 'text' && configTablePendingDrafts[table.key]!.filter.column === 'custom'" class="dataset-filter-row dataset-filter-extra">
                <span class="config-preview-sep">筛选列定位</span>
                <label>表头所在行<input type="number" min="0" v-model.number="configTablePendingDrafts[table.key]!.filter.extraMatchRow" placeholder="默认" @change="loadPendingTablePreview(table)" /></label>
                <label>表头关键字<input type="text" v-model="configTablePendingDrafts[table.key]!.filter.extraKeyword" placeholder="如 备注、EnumName" @change="loadPendingTablePreview(table)" /></label>
              </div>
              <div class="config-preview-list">
                <div v-for="(entry, entryIndex) in pendingPreviewEntries(table).shown" :key="entryIndex + ':' + entry.id" class="config-preview-row"><span class="config-preview-id">{{ entry.id }}</span><span class="config-preview-name">{{ entry.name }}</span><span v-if="entry.extra" class="config-preview-extra">{{ entry.extra }}</span></div>
                <div v-if="!pendingPreviewEntries(table).matched && !configTablePreviewLoading.has(table.key) && (table.key in configTablePreview)" class="config-preview-empty">无匹配数据</div>
              </div>
            </div>
          </div>
        </div>
        <div class="config-table-hint">勾选未导入的表后可展开<b>识别设置</b>，单独配置该表的表头行号/关键字与筛选（留空沿用上面的通用默认），保存时随数据集一起生效。改名、把一张表拆成多个数据集（同一枚举表切出多段）请用“配置表 → 数据集管理”的“复制”。</div>
      </div>
      <footer class="dialog-footer-actions config-table-footer">
        <span v-if="configTableSaveFeedback" class="config-save-feedback">✓ {{ configTableSaveFeedback }}</span>
        <span class="dialog-footer-spacer"></span>
        <button class="dialog-button primary" @click="saveConfigTableImport">导入勾选（新增 {{ configTablePendingPicks.size }} 个数据集）</button>
        <button class="dialog-button ghost" @click="configTableImportDialog = false">关闭</button>
      </footer>
      <div class="dialog-resize" title="拖动缩放窗口" @pointerdown.stop.prevent="beginDialogResize"></div>
    </section></div>
    <div v-if="configTableManageDialog" class="about-backdrop"><section class="about-dialog config-table-dialog config-manage-dialog">
      <header @pointerdown="beginDialogDrag"><strong>数据集管理</strong><button @click="closeConfigTableManage">×</button></header>
      <div class="config-manage-body">
        <aside class="config-dataset-list">
          <div class="config-list-toolbar">
            <input v-model="configTableManageSearch" class="config-table-search" placeholder="🔍 搜索数据集" />
            <span class="config-preview-mode">
              <button type="button" class="dialog-button small" :class="configTableManageView === 'all' ? 'primary' : 'ghost'" @click="configTableManageView = 'all'">全部</button>
              <button type="button" class="dialog-button small" :class="configTableManageView === 'missing' ? 'primary' : 'ghost'" @click="configTableManageView = 'missing'">缺失</button>
            </span>
          </div>
          <div class="config-dataset-items">
            <button v-for="dataset in configTableManageDatasetList()" :key="dataset.key" type="button" class="config-dataset-item" :class="{ active: dataset.key === configTableManageSelected, warned: configTables.some(item => item.key === dataset.key && item.missing) }" @click="selectManageDataset(dataset.key)">
              <strong class="config-dataset-name">{{ dataset.name }}<span v-if="manageDatasetDirty(dataset.key)" class="config-dataset-dirty" title="有未保存的修改">●</span></strong>
              <small class="config-dataset-source">{{ dataset.file }}{{ dataset.sheet ? ' · ' + dataset.sheet : '' }}</small>
              <small class="config-dataset-meta">{{ manageDatasetMeta(dataset.key) }}</small>
            </button>
            <div v-if="!configTableManageDatasetList().length" class="empty-panel">还没有数据集；用下方“从表新增”，或先到“批量导入”勾选表。</div>
          </div>
          <div class="config-dataset-actions">
            <select v-model="configTableManageAddKey" title="从表元目录的表新建一个数据集" @change="configTableManageAddKey && addManageDatasetFromScan(configTableManageAddKey)">
              <option value="">＋ 从表新增…</option>
              <option v-for="table in manageAddCandidates()" :key="table.key" :value="table.key">{{ table.key }}（{{ table.rowCount }} 行）</option>
            </select>
            <button class="dialog-button ghost small" :disabled="!manageSelected" title="复制当前数据集：改显示名和筛选后即可切出同一张表的另一段数据" @click="duplicateManageDataset">复制</button>
            <button class="dialog-button ghost small danger" :disabled="!manageSelected" @click="removeManageDataset">删除</button>
          </div>
        </aside>
        <div v-if="manageSelected" class="config-dataset-detail">
          <div class="dataset-field-grid dataset-info-grid">
            <label>显示名<input v-model="manageSelected.name" placeholder="如 行为限制枚举" /></label>
            <label title="节点 ref 的绑定值（nodes/*.json 与 .obpf 保存它），创建后不可改">数据集 ID<input :value="manageSelected.key" disabled /></label>
            <label title="数据来源的物理表，可重新选择（文件改名/移动后在这里重绑）">来源表<select :value="manageSourceValue()" @change="setManageSource($event)">
              <option v-if="manageSourceValue() === '__missing__'" value="__missing__">{{ manageSelected.file }}{{ manageSelected.sheet ? ' · ' + manageSelected.sheet : '' }}（未找到）</option>
              <option v-for="table in manageAddCandidates()" :key="table.key" :value="table.key">{{ table.file }}{{ table.sheet ? ' · ' + table.sheet : '' }}</option>
            </select></label>
          </div>
          <div class="detail-section-title"><span>表头识别</span><small class="dataset-section-note">0 或留空 = 沿用通用默认</small></div>
          <div class="dataset-field-grid dataset-rule-grid">
            <label title="在第几行里找 id 表头">id 表头所在行<input v-model.number="manageSelected.idMatchRow" type="number" min="0" placeholder="默认" @change="reloadManagePreview()" /></label>
            <label title="该行的哪一列算 id 列：表头包含该字">id 表头关键字<input v-model="manageSelected.idKeyword" placeholder="默认" @change="reloadManagePreview()" /></label>
            <label title="在第几行里找名称表头">名称表头所在行<input v-model.number="manageSelected.nameMatchRow" type="number" min="0" placeholder="默认" @change="reloadManagePreview()" /></label>
            <label title="该行的哪一列算名称列：表头包含该字，逗号分隔多个">名称表头关键字<input v-model="manageSelected.nameKeyword" placeholder="默认，逗号分隔" @change="reloadManagePreview()" /></label>
          </div>
          <div class="detail-section-title"><span>筛选</span><small class="dataset-section-note">保存后加载内存时生效；同表拆多段用“复制”</small></div>
          <div class="dataset-filter-row">
            <span class="config-preview-mode">
              <button type="button" class="dialog-button small" :class="manageSelected.filter.mode === 'text' ? 'primary' : 'ghost'" @click="manageSelected.filter.mode = 'text'">内容筛选</button>
              <button type="button" class="dialog-button small" :class="manageSelected.filter.mode === 'range' ? 'primary' : 'ghost'" @click="manageSelected.filter.mode = 'range'">ID范围</button>
              <button type="button" class="dialog-button small" :class="manageSelected.filter.mode === 'compare' ? 'primary' : 'ghost'" @click="manageSelected.filter.mode = 'compare'">ID比较</button>
            </span>
            <template v-if="manageSelected.filter.mode === 'text'">
              <input type="text" class="dataset-filter-text" v-model="manageSelected.filter.text" placeholder="筛选内容（该列包含的文字）" />
              <select v-model="manageSelected.filter.column">
                <option value="all">全部列</option>
                <option value="id">仅 id 列</option>
                <option value="name">仅名称列</option>
                <option value="custom">指定列…</option>
              </select>
            </template>
            <template v-else-if="manageSelected.filter.mode === 'range'">
              <input type="text" class="dataset-filter-number" v-model="manageSelected.filter.rangeMin" placeholder="起始 ID" />
              <span class="config-preview-sep">至</span>
              <input type="text" class="dataset-filter-number" v-model="manageSelected.filter.rangeMax" placeholder="结束 ID" />
            </template>
            <template v-else>
              <select v-model="manageSelected.filter.compareOp">
                <option>&gt;</option>
                <option>&gt;=</option>
                <option>&lt;</option>
                <option>&lt;=</option>
              </select>
              <input type="text" class="dataset-filter-number" v-model="manageSelected.filter.compareValue" placeholder="数值" />
            </template>
          </div>
          <div v-if="manageSelected.filter.mode === 'text' && manageSelected.filter.column === 'custom'" class="dataset-filter-row dataset-filter-extra">
            <span class="config-preview-sep">筛选列定位</span>
            <label>表头所在行<input type="number" min="0" v-model.number="manageSelected.filter.extraMatchRow" placeholder="默认" @change="reloadManagePreview()" /></label>
            <label>表头关键字<input type="text" v-model="manageSelected.filter.extraKeyword" placeholder="如 备注、EnumName" @change="reloadManagePreview()" /></label>
          </div>
          <div class="detail-section-title dataset-preview-title"><span>数据预览</span><small class="dataset-section-note">{{ configTablePreviewLoading.has(manageSelected.key) ? '加载中…' : `匹配 ${managePreviewEntries().matched}/${managePreviewEntries().total} 条 · 保存后按筛选加载` }}</small></div>
          <div class="config-preview-list dataset-preview-list">
            <div v-for="(entry, entryIndex) in managePreviewEntries().shown" :key="entryIndex + ':' + entry.id" class="config-preview-row"><span class="config-preview-id">{{ entry.id }}</span><span class="config-preview-name">{{ entry.name }}</span><span v-if="entry.extra" class="config-preview-extra">{{ entry.extra }}</span></div>
            <div v-if="!managePreviewEntries().matched && !configTablePreviewLoading.has(manageSelected.key) && (manageSelected.key in configTablePreview)" class="config-preview-empty">无匹配数据</div>
          </div>
        </div>
        <div v-else class="empty-panel">选择左侧数据集查看详情，或“从表新增”。</div>
      </div>
      <footer class="dialog-footer-actions config-table-footer">
        <span v-if="configTableManageError" class="config-save-error">✗ {{ configTableManageError }}</span>
        <span v-else-if="configTableManageFeedback" class="config-save-feedback">✓ {{ configTableManageFeedback }}</span>
        <span class="dialog-footer-spacer"></span>
        <button class="dialog-button primary" @click="saveConfigTableManage">保存（{{ configTableManageDraft.datasets.length }} 个数据集{{ manageDirtyInfo.total ? `，${manageDirtyInfo.total} 处未保存` : '' }}）</button>
        <button class="dialog-button ghost" @click="closeConfigTableManage">关闭</button>
      </footer>
      <div class="dialog-resize" title="拖动缩放窗口" @pointerdown.stop.prevent="beginDialogResize"></div>
    </section></div>
    <div v-if="nodeAnnotationDialog?.visible" class="about-backdrop" @pointerdown.self="closeNodeAnnotationDialog"><section class="about-dialog node-annotation-dialog">
      <header @pointerdown="beginDialogDrag"><strong>{{ nodeAnnotationDialog.functionPath ? '编辑函数注解' : '编辑节点注解' }} — {{ nodeAnnotationDialog.title }}</strong><button @click="closeNodeAnnotationDialog">×</button></header>
      <div class="node-detail annotation-detail">
        <div class="detail-section-title">说明</div>
        <textarea v-model="nodeAnnotationDraft.description" rows="4" placeholder="用途说明"></textarea>
        <template v-if="nodeAnnotationDialog.functionPath">
          <template v-if="nodeAnnotationDraft.functionParams.some(param => param.direction === 'input')">
            <div class="detail-section-title">输入参数说明</div>
            <template v-for="param in nodeAnnotationDraft.functionParams.filter(item => item.direction === 'input')" :key="param.id">
              <div class="annotation-port-group">
                <label>{{ param.name }}<input v-model="param.tip" placeholder="悬停该参数端口时显示" /></label>
                <label v-if="param.type === 'integer'" class="annotation-ref-row">{{ param.name }} 关联数据集
                  <select v-model="param.ref">
                    <option value="">（无）</option>
                    <option v-for="option in importedConfigTableOptions" :key="option.key" :value="option.key">{{ option.name }}（{{ option.entries }} 条）</option>
                  </select>
                </label>
              </div>
            </template>
          </template>
          <template v-if="nodeAnnotationDraft.functionParams.some(param => param.direction === 'output')">
            <div class="detail-section-title">输出参数说明</div>
            <template v-for="param in nodeAnnotationDraft.functionParams.filter(item => item.direction === 'output')" :key="param.id">
              <div class="annotation-port-group">
                <label>{{ param.name }}<input v-model="param.tip" placeholder="悬停该参数端口时显示" /></label>
                <label v-if="param.type === 'integer'" class="annotation-ref-row">{{ param.name }} 关联数据集
                  <select v-model="param.ref">
                    <option value="">（无）</option>
                    <option v-for="option in importedConfigTableOptions" :key="option.key" :value="option.key">{{ option.name }}（{{ option.entries }} 条）</option>
                  </select>
                </label>
              </div>
            </template>
          </template>
          <small class="variable-scope-hint">保存到函数蓝图文件（.obpf），悬停函数节点和参数端口可见。</small>
        </template>
        <template v-else>
          <template v-if="labeledInputPorts(nodeDefinitionById(nodeAnnotationDialog.typeId ?? '')?.inputPorts).length">
            <div class="detail-section-title">输入口提示</div>
            <template v-for="port in labeledInputPorts(nodeDefinitionById(nodeAnnotationDialog.typeId ?? '')?.inputPorts)" :key="port.key">
              <div class="annotation-port-group">
                <label>{{ port.label }}<input v-model="nodeAnnotationDraft.tips[port.key]" placeholder="悬停该输入口时显示" /></label>
                <label v-if="port.type === 'integer'" class="annotation-ref-row">{{ port.label }} 关联数据集
                  <select v-model="nodeAnnotationDraft.refs[port.key]">
                    <option value="">（无）</option>
                    <option v-for="option in importedConfigTableOptions" :key="option.key" :value="option.key">{{ option.name }}（{{ option.entries }} 条）</option>
                  </select>
                </label>
              </div>
            </template>
          </template>
          <small class="variable-scope-hint">保存后写入 nodes/*.json 节点定义，对所有蓝图生效。</small>
        </template>
      </div>
      <footer class="dialog-footer-actions"><button class="dialog-button primary" @click="applyNodeAnnotationsFromDialog">保存</button><button class="dialog-button ghost" @click="closeNodeAnnotationDialog">取消</button></footer>
    </section></div>
    <div v-if="showShortcuts" class="about-backdrop" @pointerdown.self="showShortcuts = false"><section class="about-dialog shortcut-dialog"><header @pointerdown="beginDialogDrag"><strong>{{ menuText.shortcuts.title }}</strong><button @click="showShortcuts = false">×</button></header><p>{{ menuText.shortcuts.intro }}</p><dl><dt>{{ menuText.shortcuts.fileTitle }}</dt><dd>{{ menuText.shortcuts.fileBody }}</dd><dt>{{ menuText.shortcuts.canvasTitle }}</dt><dd>{{ menuText.shortcuts.canvasBody }}</dd><dt>{{ menuText.shortcuts.selectionTitle }}</dt><dd>{{ menuText.shortcuts.selectionBody }}</dd><dt>{{ menuText.shortcuts.groupTitle }}</dt><dd>{{ menuText.shortcuts.groupBody }}</dd><dt>{{ menuText.shortcuts.validateTitle }}</dt><dd>{{ menuText.shortcuts.validateBody }}</dd><dt>{{ menuText.shortcuts.exportTitle }}</dt><dd>{{ menuText.shortcuts.exportBody }}</dd></dl><footer><button @click="showShortcuts = false">{{ menuText.shortcuts.close }}</button></footer></section></div>
    <div v-if="showAbout" class="about-backdrop" @pointerdown.self="showAbout = false"><section class="about-dialog"><header @pointerdown="beginDialogDrag"><strong>{{ menuText.about.title }}</strong><button @click="showAbout = false">×</button></header><p>{{ menuText.about.description }}</p><dl><dt>{{ menuText.about.version }}</dt><dd>{{ appVersion }}</dd><dt>{{ menuText.about.runtime }}</dt><dd>Go + Wails v2 / Vue 3 / Rete.js</dd></dl><footer><button :disabled="updateState.checking" @click="checkForUpdates(true)">{{ updateState.checking ? menuText.update.checking : menuText.about.checkUpdates }}</button><button @click="showAbout = false">{{ menuText.about.close }}</button></footer></section></div>
  </main>
</template>
