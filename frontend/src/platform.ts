import type { NodeSchema } from './editor/nodeRegistry'
import { parseNodeSchemaDocument } from './editor/runtimeNodeSchemas'

export interface FileResult { path: string; content: string }
export interface WorkspaceEntry { name: string; path: string; isDir: boolean }
export interface ValidationIssue { severity: 'error' | 'warning'; code: string; message: string; nodeId?: string }
export interface ExecutionLog { level: 'debug' | 'info' | 'warning' | 'error'; message: string; nodeId?: string }
export interface NodeSchemaLoadResult { nodes: NodeSchema[]; errors: Array<{ path: string; message: string }>; documentCount: number }
interface NodeSchemaDocument { path: string; content: string }
interface NodeSchemaDocumentLoadResult { documents: NodeSchemaDocument[]; errors: Array<{ path: string; message: string }> }
type RawNodeSchemaDocumentLoadResult = NodeSchemaDocumentLoadResult & {
  Documents?: NodeSchemaDocument[]
  Errors?: Array<{ path: string; message: string }>
}
export interface ExecutionEvent {
  sessionId: string
  type: 'started' | 'progress' | 'completed' | 'failed' | 'cancelled'
  message?: string
  states?: Array<{ nodeId: string; state: 'idle' | 'running' | 'completed' | 'error' }>
  logs?: ExecutionLog[]
  results?: unknown[]
  variables?: Record<string, unknown>
}

type DesktopApp = {
  OpenGraph(path: string): Promise<FileResult>
  SaveGraph(path: string, content: string): Promise<string>
  CurrentWorkingDirectory(): Promise<string>
  ChooseWorkspace(): Promise<string>
  ChooseDataFile(mode: string): Promise<string>
  NewWindow(): Promise<void>
  ClearRecentFiles(): Promise<void>
  Quit(): Promise<void>
  ListWorkspace(path: string): Promise<WorkspaceEntry[]>
  ExportPNG(dataURL: string): Promise<string>
  GetRecentFiles(): Promise<string[]>
  ValidateGraph(content: string): Promise<ValidationIssue[]>
  StartGraph(content: string): Promise<string>
  StopGraph(sessionId: string): Promise<boolean>
  MigrateLegacyGraph(content: string): Promise<string>
  ExportLegacyGraph(content: string): Promise<string>
  LoadNodeSchemaDocuments(): Promise<RawNodeSchemaDocumentLoadResult>
}

function desktop(): DesktopApp | undefined {
  return (window as unknown as { go?: { main?: { App?: DesktopApp } } }).go?.main?.App
}

function download(name: string, content: string, type: string) {
  const anchor = document.createElement('a')
  anchor.href = URL.createObjectURL(new Blob([content], { type }))
  anchor.download = name
  anchor.click()
  URL.revokeObjectURL(anchor.href)
}

function parseNodeSchemaDocuments(documents: NodeSchemaDocument[], errors: Array<{ path: string; message: string }> = []): NodeSchemaLoadResult {
  const result: NodeSchemaLoadResult = { nodes: [], errors: [...errors], documentCount: documents.length }
  const byId = new Map<string, NodeSchema>()
  for (const document of documents) {
    try {
      for (const node of parseNodeSchemaDocument(JSON.parse(document.content))) {
        if (node.id) byId.set(node.id, node)
      }
    } catch (error) {
      result.errors.push({ path: document.path, message: error instanceof Error ? error.message : String(error) })
    }
  }
  result.nodes = Array.from(byId.values()).sort((a, b) => a.id.localeCompare(b.id))
  return result
}

function normalizeNodeSchemaDocumentLoadResult(value: RawNodeSchemaDocumentLoadResult | undefined): NodeSchemaDocumentLoadResult {
  return {
    documents: value?.documents ?? value?.Documents ?? [],
    errors: value?.errors ?? value?.Errors ?? []
  }
}

async function loadBrowserNodeSchemaDocuments(): Promise<NodeSchemaDocumentLoadResult> {
  const result: NodeSchemaDocumentLoadResult = { documents: [], errors: [] }
  let files: string[] = []
  try {
    const response = await fetch('/nodes/manifest.json', { cache: 'no-store' })
    if (!response.ok) return result
    files = await response.json()
  } catch (error) {
    result.errors.push({ path: '/nodes/manifest.json', message: error instanceof Error ? error.message : String(error) })
    return result
  }

  for (const file of files) {
    const path = `/nodes/${file}`
    try {
      const response = await fetch(path, { cache: 'no-store' })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      result.documents.push({ path, content: await response.text() })
    } catch (error) {
      result.errors.push({ path, message: error instanceof Error ? error.message : String(error) })
    }
  }
  return result
}

export const platform = {
  isDesktop: () => Boolean(desktop()),
  async openGraph(path = ''): Promise<FileResult | null> {
    if (desktop()) return desktop()!.OpenGraph(path)
    return new Promise(resolve => {
      const input = document.createElement('input')
      input.type = 'file'; input.accept = '.obp,.vgf,.json'
      input.onchange = async () => {
        const file = input.files?.[0]
        resolve(file ? { path: file.name, content: await file.text() } : null)
      }
      input.click()
    })
  },
  async saveGraph(path: string, content: string) {
    if (desktop()) return desktop()!.SaveGraph(path, content)
    download(path || 'Untitled.obp', content, 'application/json')
    return path || 'Untitled.obp'
  },
  async currentWorkingDirectory() { return desktop() ? desktop()!.CurrentWorkingDirectory() : '' },
  async chooseWorkspace() { return desktop() ? desktop()!.ChooseWorkspace() : '' },
  async chooseDataFile(mode: 'open' | 'save') {
    if (desktop()) return desktop()!.ChooseDataFile(mode)
    if (mode === 'save') return window.prompt('Output file name', 'output.csv') ?? ''
    return new Promise<string>(resolve => {
      const input = document.createElement('input')
      input.type = 'file'
      input.onchange = () => resolve(input.files?.[0]?.name ?? '')
      input.click()
    })
  },
  async newWindow() {
    if (desktop()) return desktop()!.NewWindow()
    window.open(window.location.href, '_blank', 'noopener')
  },
  async clearRecentFiles() {
    if (desktop()) await desktop()!.ClearRecentFiles()
  },
  async quit() {
    if (desktop()) await desktop()!.Quit()
    else window.close()
  },
  async listWorkspace(path: string) { return desktop() ? desktop()!.ListWorkspace(path) : [] },
  async recentFiles() { return desktop() ? desktop()!.GetRecentFiles() : [] },
  async validateGraph(content: string): Promise<ValidationIssue[]> {
    if (desktop()) return desktop()!.ValidateGraph(content)
    try { JSON.parse(content); return [] } catch { return [{ severity: 'error', code: 'document.invalid-json', message: 'Invalid graph document' }] }
  },
  async startGraph(content: string) {
    if (desktop()) return desktop()!.StartGraph(content)
    throw new Error('Graph execution requires the desktop runtime')
  },
  async stopGraph(sessionId: string) { return desktop() ? desktop()!.StopGraph(sessionId) : false },
  async migrateLegacyGraph(content: string) { return desktop() ? desktop()!.MigrateLegacyGraph(content) : '' },
  async exportLegacyGraph(content: string) { return desktop() ? desktop()!.ExportLegacyGraph(content) : content },
  async loadNodeSchemas(): Promise<NodeSchemaLoadResult> {
    const result = normalizeNodeSchemaDocumentLoadResult(desktop() ? await desktop()!.LoadNodeSchemaDocuments() : await loadBrowserNodeSchemaDocuments())
    return parseNodeSchemaDocuments(result.documents, result.errors)
  },
  onExecution(callback: (event: ExecutionEvent) => void) {
    const runtime = (window as unknown as { runtime?: { EventsOnMultiple?: (name: string, callback: (event: ExecutionEvent) => void, count: number) => () => void } }).runtime
    return runtime?.EventsOnMultiple?.('origin:execution', callback, -1) ?? (() => {})
  },
  async exportPNG(dataURL: string) {
    if (desktop()) return desktop()!.ExportPNG(dataURL)
    const anchor = document.createElement('a'); anchor.href = dataURL; anchor.download = 'OriginBlueprint.png'; anchor.click()
    return anchor.download
  }
}
