<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { toPng } from 'html-to-image'
import { createBlueprintEditor, type BlueprintEditorHandle, type EditorMetrics, type FunctionSignature, type FunctionSignaturePort, type GraphDocument, type GraphVariable, type GraphVariableGroup, type SelectedNodeInfo, type ValidationIssue, type VariableType } from './editor/createEditor'
import type { FunctionNodeMetadata, NodeSnapshot, RestoreLossReport } from './editor/document'
import { getNodeDefinitions, registerNodeSchemas, type NodeDefinition } from './editor/nodeRegistry'
import { menuLocales, normalizeLocale, type LocaleId } from './i18n'
import { platform, type NodeReferenceResult, type RecoverySnapshotResult, type WorkspaceEntry } from './platform'
import { compatibilitySaveOptions, findOpenTab, hasRestoreLoss, resolveCompatibilitySaveAction as resolveCompatibilityPersistenceAction, sourceRequiresProtection, type CompatibilitySaveAction } from './documentSafety'
import { autoSaveIntervalMs, isAutoSaveEligible, type AutoSaveMode } from './autoSavePolicy'
import { saveGateDecision } from './saveGate'

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
interface ModuleLibraryItem extends NodeDefinition { functionPlaceholder?: boolean; functionSource?: FunctionLibraryItem['source']; functionItem?: FunctionLibraryItem; path?: string }
type UiScale = 'small' | 'normal' | 'large'
type NodeScale = 'normal' | 'large'
type ImageExportScale = 1 | 2 | 4
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
const metrics = ref<EditorMetrics>({ nodes: 0, connections: 0 })
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
const fileBrowserWidth = ref(savedPanelWidth('origin-blueprint-file-browser-width', 210))
const leftToolsWidth = ref(savedPanelWidth('origin-blueprint-left-tools-width', 210, 160, 520))
const rightSidebarWidth = ref(savedPanelWidth('origin-blueprint-right-sidebar-width', 230, 160, 460))
const variablePanelHeight = ref(savedPanelSize('origin-blueprint-variable-panel-height', 300, 130, 520))
const showTools = ref(true)
const showRight = ref(true)
const showLogger = ref(false)
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
const validationIssues = ref<ValidationIssue[]>([])
const selectedValidationIssueKey = ref('')
const unsavedCloseDialog = ref<{ visible: boolean; names: string[]; resolve?: (action: UnsavedCloseAction) => void }>({ visible: false, names: [] })
const compatibilitySaveDialog = ref<{ visible: boolean; droppedNodes: number; droppedConnections: number; alteredNodes: number; fatal: boolean; forceAllowed: boolean; resolve?: (action: CompatibilitySaveAction) => void }>({ visible: false, droppedNodes: 0, droppedConnections: 0, alteredNodes: 0, fatal: false, forceAllowed: false })
const recoveryDialog = ref<{ visible: boolean; snapshot: RecoverySnapshotResult | null }>({ visible: false, snapshot: null })
let recoveryQueue: RecoverySnapshotResult[] = []
let untitledCount = 1
const tabDragIndex = ref(-1)
const tabDragOverIndex = ref(-1)
let editor: BlueprintEditorHandle | null = null
let unsubscribeCloseRequest = () => {}
let closingApplication = false
let nodePointerDrag: { item: ModuleLibraryItem; startX: number; startY: number; lastX: number; lastY: number; moved: boolean } | null = null
let removeNodePointerListeners = () => {}
let workspaceLoadToken = 0
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
const groupedVariables = computed(() => variableGroups.value.map(group => ({
  group,
  variables: variables.value.filter(variable => variable.groupId === group.id)
})))
const functionLibraryItems = computed(() => collectFunctionLibraryItems(workspaceTree.value))
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
const filteredModuleItems = computed(() => nodeLibrary.value.filter(item => !isFunctionBlueprintTab.value || !item.ordinaryEntry))
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
    onDirty() { if (activeTab.value) { activeTab.value.dirty = true; activeTab.value.saveBlocked = false } },
    onFunctionSignature(value) {
      if (isFunctionBlueprintTab.value) functionSignature.value = normalizeFunctionSignature(value)
    },
    onVariables(value) { variables.value = value },
    onVariableGroups(value) { variableGroups.value = value.length ? value : [{ id: 'default', name: 'Default' }] },
    onSelection(value) {
      selectedNode.value = value ? { ...value, values: { ...value.values } } : null
      if (value) selectedVariableId.value = null
    },
    canAddEntryNodes() { return !isFunctionBlueprintTab.value },
    locale() { return currentLocale.value }
  })
  await editor.newDocument()
  if (nodeLoadStatus) status.value = nodeLoadStatus
  recentFiles.value = await platform.recentFiles()
  const initialWorkspace = await platform.currentWorkingDirectory()
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
    export: { imageScale: 2, showGrid: true }
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
        files: clampNumber(panels.files, fallback.layout.panels.files, 140, 360),
        tools: clampNumber(panels.tools, fallback.layout.panels.tools, 160, 520),
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
    }
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

async function loadRuntimeNodeLibrary() {
  let result
  try {
    result = await platform.loadNodeSchemas()
  } catch (error) {
    return `Node library load failed: ${error instanceof Error ? error.message : String(error)}`
  }
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
  tabs.value.push(tab); activeTabId.value = tab.id; selectedVariableId.value = null; functionSignature.value = emptyFunctionSignature(); functionTitle.value = ''; functionId.value = ''; functionCategory.value = ''; await editor?.newDocument()
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
  functionId.value = isFunctionBlueprintPath(tab.path || tab.title) ? functionIdFromDocument(tab.document) : ''
  functionCategory.value = isFunctionBlueprintPath(tab.path || tab.title) ? functionCategoryFromDocument(tab.document, tab.path || tab.title) : ''
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
    functionId.value = isFunctionBlueprintPath(tabs.value[0].path || tabs.value[0].title) ? functionIdFromDocument(tabs.value[0].document) : ''
    functionCategory.value = isFunctionBlueprintPath(tabs.value[0].path || tabs.value[0].title) ? functionCategoryFromDocument(tabs.value[0].document, tabs.value[0].path || tabs.value[0].title) : ''
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
  return { schemaVersion: 1, graphName: name, nodes: [], connections: [], groups: [], variables: [], variableGroups: [{ id: 'default', name: 'Default' }], view: { x: 0, y: 0, zoom: 1 } }
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
  if (isFunctionBlueprintPath(tab.path || tab.title)) {
    document.graphName = activeFunctionTitle()
    document.functionId = activeFunctionId()
    document.functionCategory = activeFunctionCategory()
    document.functionSignature = normalizeFunctionSignature(functionSignature.value)
  }
  return document
}

function hasFunctionNodes(document: GraphDocument) {
  return (document.nodes ?? []).some(node => String(node.typeId ?? '').startsWith('origin.function.') || node.typeId === 'origin.timer.set-by-function')
}

function documentRequiresNativePersistence(document: GraphDocument) {
  const signature = normalizeFunctionSignature(document.functionSignature)
  const hasNativeTimeNodes = (document.nodes ?? []).some(node =>
    node.typeId === 'origin.flow.delay' || String(node.typeId ?? '').startsWith('origin.timer.'),
  )
  const hasTimerHandleVariables = (document.variables ?? []).some(variable => variable.type === 'timerhandle')
  return hasFunctionNodes(document)
    || hasNativeTimeNodes
    || hasTimerHandleVariables
    || signature.inputs.length > 0
    || signature.outputs.length > 0
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
  document.functionId = newFunctionId()
  document.functionCategory = inferredFunctionCategoryFromPath(path)
  document.functionSignature = emptyFunctionSignature()
  const terminals = functionTerminalNodes(graphName, document.functionSignature, document.functionId)
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
    functionId: String(value.functionId ?? '').trim() || undefined,
    functionCategory: String(value.functionCategory ?? '').trim() || undefined,
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
  return value?.schemaVersion === 1
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
  validationIssues.value = await platform.validateGraph(JSON.stringify(document), workspaceRoot.value, activeTab.value.path)
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
    const envelope = JSON.parse(raw) as { document?: unknown; blockingIssues?: ValidationIssue[] }
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
    await switchTab(existing.id)
    status.value = `${existing.title} is already open`
    await highlightReferenceSearchTarget(highlightTypeId)
    return
  }
  let parsed: any
  try { parsed = JSON.parse(file.content) } catch { status.value = 'Invalid graph file'; return }
  let document: GraphDocument
  let sourceIssues: ValidationIssue[] = []
  if (isNativeGraphDocument(parsed)) {
    sourceIssues = await platform.validateGraph(file.content, workspaceRoot.value, file.path)
    document = normalizeDocument(parsed)
  }
  else if (platform.isDesktop()) {
    try { document = normalizeDocument(JSON.parse(await platform.migrateLegacyGraph(file.content))) }
    catch (error) { status.value = error instanceof Error ? error.message : 'Legacy graph migration failed'; return }
  } else { status.value = 'Legacy graph migration requires the desktop runtime'; return }
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
  functionId.value = isFunctionBlueprintPath(file.path) ? functionIdFromDocument(document) : ''
  functionCategory.value = isFunctionBlueprintPath(file.path) ? functionCategoryFromDocument(document, file.path) : ''
  if (isFunctionBlueprintPath(file.path)) functionTitleByPath.value = { ...functionTitleByPath.value, [file.path]: functionTitle.value }
  if (isFunctionBlueprintPath(file.path) && functionId.value) functionIdByPath.value = { ...functionIdByPath.value, [file.path]: functionId.value }
  if (isFunctionBlueprintPath(file.path)) functionCategoryByPath.value = { ...functionCategoryByPath.value, [file.path]: functionCategory.value }
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
  const documentJSON = JSON.stringify(document)
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
          ? await platform.exportLegacyGraph(JSON.stringify(document))
          : JSON.stringify(document, null, 2)
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
  const shouldSaveLegacy = !effectiveSaveAs && !forceNativeSaveAs && isLegacyGraphPath(tab.path)
  const content = shouldSaveLegacy ? await platform.exportLegacyGraph(JSON.stringify(document)) : JSON.stringify(document, null, 2)
  const previousPath = tab.path
  const path = forceOriginal
    ? await platform.forceSaveGraph(tab.path, content)
    : await platform.saveGraph(effectiveSaveAs || forceNativeSaveAs ? '' : tab.path, content)
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

async function refreshWorkspace() {
  if (!workspaceRoot.value) return
  await loadWorkspace(workspaceRoot.value)
  status.value = 'Workspace refreshed'
}

async function refreshNodeLibrary() {
  const nodeLoadStatus = await loadRuntimeNodeLibrary()
  status.value = nodeLoadStatus || `Node library refreshed (${nodeLibrary.value.length} node template(s))`
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
  await loadProjectSettings(path)
  workspaceTree.value = await loadWorkspaceTree(path)
  void hydrateWorkspaceTree(workspaceTree.value, 1, token)
  if (projectSettingsContent.value.explorer.revealActiveFile) void revealActiveWorkspaceFile()
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
      if (title) functionTitleByPath.value = { ...functionTitleByPath.value, [item.path]: title }
      if (id) functionIdByPath.value = { ...functionIdByPath.value, [item.path]: id }
      functionCategoryByPath.value = { ...functionCategoryByPath.value, [item.path]: category }
      continue
    }
    loadingFunctionTitles.add(item.path)
    try {
      const file = await platform.openGraph(item.path)
      if (!file) continue
      const parsed = JSON.parse(file.content) as Partial<GraphDocument>
      const title = String(parsed.graphName ?? '').trim()
      const id = String(parsed.functionId ?? '').trim()
      const category = functionCategoryFromDocument(parsed, item.path)
      if (title) functionTitleByPath.value = { ...functionTitleByPath.value, [item.path]: title }
      if (id) functionIdByPath.value = { ...functionIdByPath.value, [item.path]: id }
      functionCategoryByPath.value = { ...functionCategoryByPath.value, [item.path]: category }
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
    leftToolsWidth.value = Math.min(520, Math.max(160, Math.round(startWidth + next.clientX - startX)))
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
    const parsed = JSON.parse(file.content) as Partial<GraphDocument>
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
    functionSignature: source === 'current' ? normalizeFunctionSignature(functionSignature.value) : await loadFunctionSignatureForModuleItem(item)
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
            <label>{{ menuText.detail.functionTitle }}<input v-model="functionTitle" :placeholder="menuText.detail.functionTitlePlaceholder" :title="menuText.detail.functionTitleLockedHint" readonly @change="syncFunctionTitleToGraph" /></label>
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
            <div class="function-terminal-actions"><button @click="addFunctionEntryNodeToGraph">＋ 入口参数</button><button @click="addFunctionReturnNodeToGraph">＋ 出口参数</button></div>
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
          <div v-else-if="selectedVariable" class="node-detail variable-detail"><div class="detail-section-title">变量属性</div><label>Variable ID<input :value="selectedVariable.id" disabled /></label><label>名称<input v-model="selectedVariable.name" /></label><label>类型<select v-model="selectedVariable.type" @change="changeVariableType(selectedVariable)"><option value="boolean">Boolean</option><option value="integer">Integer</option><option value="float">Float</option><option value="string">String</option><option value="array">Array</option><option value="timerhandle">Timer Handle</option></select></label><label>分组<select v-model="selectedVariable.groupId"><option v-for="group in variableGroups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label><label>说明<textarea v-model="selectedVariable.description" rows="4" placeholder="变量用途和约束"></textarea></label><label v-if="selectedVariable.type !== 'timerhandle'">默认值<input v-if="selectedVariable.type === 'boolean'" v-model="selectedVariable.defaultValue" type="checkbox" /><input v-else-if="selectedVariable.type === 'string'" v-model="selectedVariable.defaultValue" type="text" /><input v-else-if="selectedVariable.type === 'array'" :value="Array.isArray(selectedVariable.defaultValue) ? selectedVariable.defaultValue.join(', ') : ''" placeholder="1, 2, text" @change="setVariableArrayDefault(selectedVariable, $event)" /><input v-else v-model.number="selectedVariable.defaultValue" type="number" /></label><button class="apply-properties" @click="updateVariable(selectedVariable)">应用变量属性</button><button class="delete-properties" @click="removeVariable(selectedVariable)">删除变量</button></div>
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
        <div class="canvas-wrap" @contextmenu.prevent @dragenter="allowNodeDrop" @dragover="allowNodeDrop" @drop.prevent="dropNode"><div ref="canvas" class="rete-canvas"></div><div v-if="canvasToast.visible" class="canvas-toast" :style="{ left: `${canvasToast.x}px`, top: `${canvasToast.y}px` }">{{ canvasToast.message }}</div><div class="canvas-toolbar"><button title="Select">⌖</button><button title="Reset view" @click="editor?.resetView()">⌂</button></div><div class="canvas-hint">{{ menuText.canvas.hint }}</div></div>
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
            <button v-for="(issue, index) in validationIssues" :key="validationIssueKey(issue, index)" class="logger-issue" :class="[issue.severity, { selected: selectedValidationIssueKey === validationIssueKey(issue, index) }]" @click="queueSelectIssue(issue, index)" @dblclick.stop.prevent="highlightIssue(issue, index)">
              <strong>{{ issueSeverityLabel(issue) }}</strong>
              <span class="logger-issue-message">{{ issue.message }}</span>
              <span v-if="issue.blocksSave" class="logger-issue-blocks-save">禁止保存</span>
              <span class="logger-issue-meta"><b>{{ menuText.validation.nodes }}</b>{{ issueNodeLabel(issue) }}</span>
              <small><b>{{ menuText.validation.code }}</b>{{ issue.code }}</small>
            </button>
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
      <button v-if="moduleNodeMenu.node?.functionPlaceholder" @click="openFunctionModuleItem()">打开函数</button>
      <button v-if="moduleNodeMenu.node?.functionPlaceholder" @click="findModuleFunctionReferences()">查找所有引用</button>
      <button v-else @click="findModuleNodeReferences()">查找所有引用</button>
    </div>
    <div v-if="fileContextMenu.visible" class="file-context-menu" :style="{ left: `${fileContextMenu.x}px`, top: `${fileContextMenu.y}px` }" @pointerdown.stop>
      <button v-if="!fileContextMenu.isDir" @click="openFileContextGraph">{{ fileContextMenu.isFunction ? '打开函数' : '打开蓝图' }}</button>
      <button v-if="fileContextMenu.isDir" @click="refreshFileContextDirectory">刷新目录</button>
      <button v-if="fileContextMenu.isDir" @click="createBlueprintInFileContext">新建蓝图</button>
      <button v-if="fileContextMenu.isDir" @click="createFunctionInFileContext">新建函数</button>
      <button @click="revealFileContextInFolder">在资源管理器中定位</button>
    </div>
    <div v-if="showSettings" class="settings-backdrop" @click.self="showSettings = false">
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
    <div v-if="updateState.visible" class="update-backdrop" @click.self="closeUpdateDialog">
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
    <div v-if="showShortcuts" class="about-backdrop" @click.self="showShortcuts = false"><section class="about-dialog shortcut-dialog"><header><strong>{{ menuText.shortcuts.title }}</strong><button @click="showShortcuts = false">×</button></header><p>{{ menuText.shortcuts.intro }}</p><dl><dt>{{ menuText.shortcuts.fileTitle }}</dt><dd>{{ menuText.shortcuts.fileBody }}</dd><dt>{{ menuText.shortcuts.canvasTitle }}</dt><dd>{{ menuText.shortcuts.canvasBody }}</dd><dt>{{ menuText.shortcuts.selectionTitle }}</dt><dd>{{ menuText.shortcuts.selectionBody }}</dd><dt>{{ menuText.shortcuts.groupTitle }}</dt><dd>{{ menuText.shortcuts.groupBody }}</dd><dt>{{ menuText.shortcuts.validateTitle }}</dt><dd>{{ menuText.shortcuts.validateBody }}</dd><dt>{{ menuText.shortcuts.exportTitle }}</dt><dd>{{ menuText.shortcuts.exportBody }}</dd></dl><footer><button @click="showShortcuts = false">{{ menuText.shortcuts.close }}</button></footer></section></div>
    <div v-if="showAbout" class="about-backdrop" @click.self="showAbout = false"><section class="about-dialog"><header><strong>{{ menuText.about.title }}</strong><button @click="showAbout = false">×</button></header><p>{{ menuText.about.description }}</p><dl><dt>{{ menuText.about.version }}</dt><dd>{{ appVersion }}</dd><dt>{{ menuText.about.runtime }}</dt><dd>Go + Wails v2 / Vue 3 / Rete.js</dd></dl><footer><button :disabled="updateState.checking" @click="checkForUpdates(true)">{{ updateState.checking ? menuText.update.checking : menuText.about.checkUpdates }}</button><button @click="showAbout = false">{{ menuText.about.close }}</button></footer></section></div>
  </main>
</template>
