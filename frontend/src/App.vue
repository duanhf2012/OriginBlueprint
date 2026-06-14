<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { toPng } from 'html-to-image'
import { createBlueprintEditor, type BlueprintEditorHandle, type EditorMetrics, type GraphDocument } from './editor/createEditor'
import { createNode, nodeDefinitions } from './editor/nodeRegistry'
import { platform, type WorkspaceEntry } from './platform'

interface GraphTab { id: string; title: string; path: string; dirty: boolean; document: GraphDocument | null }

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
const workspaceEntries = ref<WorkspaceEntry[]>([])
const showLeft = ref(true)
const showRight = ref(true)
const showLogger = ref(false)
let untitledCount = 1
let editor: BlueprintEditorHandle | null = null

const activeTab = computed(() => tabs.value.find(tab => tab.id === activeTabId.value)!)
const categories = computed(() => {
  const result = new Map<string, typeof nodeDefinitions>()
  for (const definition of nodeDefinitions) {
    const items = result.get(definition.category) ?? []; items.push(definition); result.set(definition.category, items)
  }
  return Array.from(result.entries())
})
const filteredDefinitions = computed(() => {
  const search = contextMenu.value.search.trim().toLowerCase()
  return search ? nodeDefinitions.filter(item => `${item.title} ${item.category}`.toLowerCase().includes(search)) : nodeDefinitions
})

onMounted(async () => {
  if (!canvas.value) return
  editor = await createBlueprintEditor(canvas.value, {
    onZoom(value) { zoomLabel.value = `${Math.round(value * 100)}%` },
    onStatus(value) { status.value = value },
    onMetrics(value) { metrics.value = value },
    onDirty() { if (activeTab.value) activeTab.value.dirty = true }
  })
  await editor.newDocument()
  recentFiles.value = await platform.recentFiles()
  const savedWorkspace = localStorage.getItem('origin-blueprint-workspace') ?? ''
  if (savedWorkspace) await loadWorkspace(savedWorkspace)
  window.addEventListener('keydown', onKeyDown)
  window.addEventListener('pointerdown', closeFloatingMenus)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown); window.removeEventListener('pointerdown', closeFloatingMenus); editor?.destroy()
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
  if (ctrl && key === 'n') run(newGraph, event)
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
function persistActive() { if (editor && activeTab.value) activeTab.value.document = editor.getDocument(activeTab.value.title) }

async function newGraph() {
  persistActive(); untitledCount++
  const tab: GraphTab = { id: crypto.randomUUID(), title: `Untitled-${untitledCount} Graph`, path: '', dirty: false, document: null }
  tabs.value.push(tab); activeTabId.value = tab.id; await editor?.newDocument()
}

async function switchTab(id: string) {
  if (id === activeTabId.value) return
  persistActive(); activeTabId.value = id
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
  if (wasActive) { activeTabId.value = tabs.value[0].id; await editor?.loadDocument(tabs.value[0].document ?? blankDocument(tabs.value[0].title)) }
}

function blankDocument(name: string): GraphDocument {
  return { schemaVersion: 1, graphName: name, nodes: [], connections: [], groups: [], variables: [], view: { x: 0, y: 0, zoom: 1 } }
}

function migrateLegacy(value: any): GraphDocument {
  const typeMap: Record<string, string> = {
    BeginNode: 'origin.event.begin', ForLoop: 'origin.flow.for-loop', BranchNode: 'origin.flow.branch',
    PrintNode: 'origin.action.print', 'int -> str': 'origin.cast.integer-string'
  }
  const nodes = (value.nodes ?? []).flatMap((item: any) => {
    const typeId = typeMap[item.class]
    if (!typeId) return []
    const template = createNode(typeId)
    const inputKeys = Object.keys(template.inputs)
    const values: Record<string, unknown> = {}
    for (const [index, data] of Object.entries(item.port_defaultv ?? {})) if (inputKeys[Number(index)]) values[inputKeys[Number(index)]] = data
    return [{ id: item.id, typeId, position: { x: item.pos?.[0] ?? 0, y: item.pos?.[1] ?? 0 }, values }]
  })
  const nodeMap = new Map(nodes.map((node: any) => [node.id, node]))
  const connections = (value.edges ?? []).flatMap((edge: any) => {
    const sourceInfo: any = nodeMap.get(edge.source_node_id), targetInfo: any = nodeMap.get(edge.des_node_id)
    if (!sourceInfo || !targetInfo) return []
    const source = createNode(sourceInfo.typeId), target = createNode(targetInfo.typeId)
    const sourceOutput = Object.keys(source.outputs)[edge.source_port_index], targetInput = Object.keys(target.inputs)[edge.des_port_index]
    return sourceOutput && targetInput ? [{ source: sourceInfo.id, sourceOutput, target: targetInfo.id, targetInput }] : []
  })
  return { schemaVersion: 1, graphName: value.graph_name || 'Imported Graph', nodes, connections, groups: [], variables: value.variables ?? [], view: { x: 0, y: 0, zoom: 1 } }
}

async function openGraph(path = '') {
  const file = await platform.openGraph(path)
  if (!file) return
  let parsed: any
  try { parsed = JSON.parse(file.content) } catch { status.value = 'Invalid graph file'; return }
  const document: GraphDocument = parsed.schemaVersion === 1 ? parsed : migrateLegacy(parsed)
  persistActive()
  const existing = tabs.value.find(tab => tab.path === file.path)
  if (existing) return switchTab(existing.id)
  const title = file.path.split(/[\\/]/).pop() ?? document.graphName
  const tab: GraphTab = { id: crypto.randomUUID(), title, path: file.path, dirty: false, document }
  tabs.value.push(tab); activeTabId.value = tab.id; await editor?.loadDocument(document)
  recentFiles.value = await platform.recentFiles()
}

async function saveGraph(saveAs: boolean) {
  if (!editor) return
  const tab = activeTab.value
  const document = editor.getDocument(tab.title)
  const path = await platform.saveGraph(saveAs ? '' : tab.path, JSON.stringify(document, null, 2))
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
async function loadWorkspace(path: string) {
  workspaceRoot.value = path; workspaceEntries.value = await platform.listWorkspace(path); localStorage.setItem('origin-blueprint-workspace', path)
}
async function workspaceOpen(item: WorkspaceEntry) { if (item.isDir) await loadWorkspace(item.path); else await openGraph(item.path) }

async function exportImage(selected: boolean) {
  if (!canvas.value) return
  if (selected) await editor?.fitSelected(); else await editor?.resetView()
  await nextTick(); await new Promise(resolve => setTimeout(resolve, 120))
  const data = await toPng(canvas.value, { backgroundColor: '#202020', pixelRatio: 2, cacheBust: true })
  const path = await platform.exportPNG(data); status.value = path ? `Exported ${path}` : 'Export cancelled'
}

function startNodeDrag(event: DragEvent, typeId: string) { event.dataTransfer?.setData('application/x-origin-node', typeId); if (event.dataTransfer) event.dataTransfer.effectAllowed = 'copy' }
function dropNode(event: DragEvent) { const typeId = event.dataTransfer?.getData('application/x-origin-node'); if (typeId) void editor?.addNode(typeId, { x: event.clientX, y: event.clientY }) }
function openContextMenu(event: MouseEvent) {
  if ((event.target as HTMLElement).closest('.blueprint-node, input, .node-group')) return
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  contextMenu.value = { visible: true, x: event.clientX - rect.left, y: event.clientY - rect.top, clientX: event.clientX, clientY: event.clientY, search: '' }
}
function createFromContext(typeId: string) { void editor?.addNode(typeId, { x: contextMenu.value.clientX, y: contextMenu.value.clientY }); contextMenu.value.visible = false }
</script>

<template>
  <main class="application-shell" :class="{ 'left-hidden': !showLeft, 'right-hidden': !showRight }">
    <header class="menu-bar">
      <div class="menu-items">
        <div class="menu-root"><button @click.stop="toggleMenu('file')">File</button><div v-if="activeMenu === 'file'" class="dropdown-menu">
          <button @click="run(newGraph)">New Graph <kbd>Ctrl+N</kbd></button><button @click="run(() => openGraph())">Open <kbd>Ctrl+O</kbd></button>
          <div v-if="recentFiles.length" class="menu-subtitle">Recent</div><button v-for="file in recentFiles" :key="file" class="recent-item" @click="run(() => openGraph(file))">{{ file.split(/[\\/]/).pop() }}</button>
          <div class="menu-separator"></div><button @click="run(chooseWorkspace)">Set Workspace Path</button><button @click="run(() => saveGraph(false))">Save <kbd>Ctrl+S</kbd></button><button @click="run(() => saveGraph(true))">Save As <kbd>Ctrl+Shift+S</kbd></button><button @click="run(saveAll)">Save All <kbd>Ctrl+Alt+S</kbd></button>
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
        <button>Run</button><button>Help</button>
      </div><div class="window-title">Origin Blueprint</div>
    </header>

    <section class="workspace">
      <aside v-show="showLeft" class="sidebar sidebar-left">
        <div class="panel workspace-panel"><div class="panel-title"><span class="chevron">⌄</span> 文件浏览器<button class="panel-action" @click="chooseWorkspace">…</button></div><div class="workspace-root" :title="workspaceRoot">{{ workspaceRoot || 'No workspace selected' }}</div><button v-for="item in workspaceEntries" :key="item.path" class="workspace-entry" @dblclick="workspaceOpen(item)"><span>{{ item.isDir ? '▸' : '◇' }}</span>{{ item.name }}</button></div>
        <div class="panel"><div class="panel-title"><span class="chevron">⌄</span> 函数</div><div class="tree-row"><span class="folder-dot blue"></span>Default</div></div>
        <div class="panel grow"><div class="panel-title"><span class="chevron">⌄</span> 变量</div><div class="tree-row"><span class="folder-dot green"></span>Default</div><button class="add-variable">＋ 添加变量</button></div>
      </aside>

      <section class="editor-column">
        <div class="tab-strip"><div v-for="tab in tabs" :key="tab.id" class="graph-tab" :class="{ active: tab.id === activeTabId }" @click="switchTab(tab.id)"><span class="tab-mark"></span>{{ tab.title }}<span v-if="tab.dirty" class="dirty-mark">●</span><button class="tab-close" @click="closeTab(tab.id, $event)">×</button></div><button class="new-tab" @click="newGraph">＋</button></div>
        <div class="canvas-wrap" @contextmenu.prevent="openContextMenu" @dragover.prevent @drop.prevent="dropNode"><div ref="canvas" class="rete-canvas"></div><div class="canvas-toolbar"><button title="Select">⌖</button><button title="Reset view" @click="editor?.resetView()">⌂</button></div><div class="canvas-hint">Middle drag: pan&nbsp;&nbsp; Wheel: zoom&nbsp;&nbsp; Left drag: select&nbsp;&nbsp; Ctrl: multi-select</div>
          <div v-if="contextMenu.visible" class="node-context-menu" :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }" @pointerdown.stop><input v-model="contextMenu.search" autofocus placeholder="Search nodes..." /><button v-for="item in filteredDefinitions" :key="item.id" @click="createFromContext(item.id)"><span>{{ item.title }}</span><small>{{ item.category }}</small></button></div>
        </div>
        <div v-show="showLogger" class="logger-panel"><div class="logger-title">Logger</div><div class="logger-line">OriginBlueprint editor ready.</div></div>
        <footer class="status-bar"><span>{{ status }}</span><span>Nodes {{ metrics.nodes }} · Connections {{ metrics.connections }}</span><button @click="editor?.resetView()">{{ zoomLabel }}</button></footer>
      </section>

      <aside v-show="showRight" class="sidebar sidebar-right"><div class="panel module-panel"><div class="panel-title"><span class="chevron">⌄</span> 模块库</div><div class="search-box">⌕ <span>搜索模块...</span></div><div class="module-list"><section v-for="[category, items] in categories" :key="category"><div class="module-category"><span>⌄</span>{{ category }}</div><button v-for="item in items" :key="item.id" class="module-item" draggable="true" @dragstart="startNodeDrag($event, item.id)" @dblclick="editor?.addNode(item.id)">{{ item.title }}</button></section></div></div><div class="panel grow"><div class="panel-title"><span class="chevron">⌄</span> 详情</div><div class="empty-detail">选择节点以查看属性</div></div></aside>
    </section>
  </main>
</template>
