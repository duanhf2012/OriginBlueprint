import type { NodeSchema } from './editor/nodeRegistry'
import { parseNodeSchemaDocument } from './editor/runtimeNodeSchemas'

export interface FileResult { path: string; content: string }
export interface ProjectSettingsResult { path: string; content: string }
export interface WorkspaceEntry { name: string; path: string; isDir: boolean }
export interface NodeReferenceResult { name: string; path: string; count: number }
export interface ValidationIssue { severity: 'error' | 'warning'; code: string; message: string; nodeId?: string; nodeIds?: string[]; sourcePath?: string; blocksSave?: boolean; blocksRun?: boolean; target?: string }
export interface RecoverySnapshotResult { path: string; sourcePath?: string; tabId?: string; createdAt: string }
export interface NodeSchemaLoadResult { nodes: NodeSchema[]; errors: Array<{ path: string; message: string }>; documentCount: number }
interface NodeSchemaDocument { path: string; content: string }
interface NodeSchemaDocumentLoadResult { documents: NodeSchemaDocument[]; errors: Array<{ path: string; message: string }> }
type RawNodeSchemaDocumentLoadResult = NodeSchemaDocumentLoadResult & {
  Documents?: NodeSchemaDocument[]
  Errors?: Array<{ path: string; message: string }>
}
type DesktopApp = {
  OpenGraph(path: string): Promise<FileResult>
  SaveGraph(path: string, content: string): Promise<string>
  ForceSaveGraph(path: string, content: string): Promise<string>
  CurrentWorkingDirectory(): Promise<string>
  ChooseWorkspace(): Promise<string>
  LoadProjectSettings(root: string): Promise<ProjectSettingsResult>
  SaveProjectSettings(root: string, content: string): Promise<string>
  ChooseDataFile(mode: string): Promise<string>
  NewWindow(): Promise<void>
  ClearRecentFiles(): Promise<void>
  Quit(): Promise<void>
  ListWorkspace(path: string): Promise<WorkspaceEntry[]>
  FindNodeReferences(root: string, typeId: string): Promise<NodeReferenceResult[]>
  RevealInFolder(path: string): Promise<void>
  ExportPNG(dataURL: string): Promise<string>
  ChooseExportPNGPath(defaultDirectory: string): Promise<string>
  SavePNG(path: string, dataURL: string): Promise<string>
  OpenExternalURL(url: string): Promise<void>
  GetRecentFiles(): Promise<string[]>
  ValidateGraph(content: string): Promise<ValidationIssue[]>
  ValidateGraphForWorkspace(content: string, workspaceRoot: string, sourcePath: string): Promise<ValidationIssue[]>
  SaveRecoverySnapshot(sourcePath: string, tabID: string, documentJSON: string, issuesJSON: string): Promise<RecoverySnapshotResult>
  ListRecoverySnapshots(): Promise<RecoverySnapshotResult[]>
  ReadRecoverySnapshot(path: string): Promise<string>
  DeleteRecoverySnapshot(path: string): Promise<void>
  DeleteRecoverySnapshots(sourcePath: string, tabID: string): Promise<void>
  MigrateLegacyGraph(content: string): Promise<string>
  ExportLegacyGraph(content: string): Promise<string>
  LoadNodeSchemaDocuments(): Promise<RawNodeSchemaDocumentLoadResult>
  LogClientError(level: string, message: string, stack: string, context: string): Promise<void>
}

type WailsRuntime = {
  EventsOnMultiple?: (name: string, callback: (...data: unknown[]) => void, count: number) => () => void
}

function desktop(): DesktopApp | undefined {
  return (window as unknown as { go?: { main?: { App?: DesktopApp } } }).go?.main?.App
}

async function logDesktopError(level: string, message: string, stack: string, context: string) {
  try { await desktop()?.LogClientError(level, message, stack, context) } catch { /* logging must never break user actions */ }
}

function describeError(error: unknown) {
  if (error instanceof Error) return { message: error.message, stack: error.stack ?? '' }
  return { message: String(error), stack: '' }
}

async function withDesktopLogging<T>(context: string, action: () => Promise<T>): Promise<T> {
  try {
    return await action()
  } catch (error) {
    const detail = describeError(error)
    await logDesktopError('error', detail.message, detail.stack, context)
    throw error
  }
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
  async logClientError(level: string, message: string, stack = '', context = 'frontend') {
    await logDesktopError(level, message, stack, context)
  },
  async openGraph(path = ''): Promise<FileResult | null> {
    if (desktop()) return withDesktopLogging('OpenGraph', () => desktop()!.OpenGraph(path))
    return new Promise(resolve => {
      const input = document.createElement('input')
      input.type = 'file'; input.accept = '.obp,.vgf,.obpf,.json'
      input.onchange = async () => {
        const file = input.files?.[0]
        resolve(file ? { path: file.name, content: await file.text() } : null)
      }
      input.click()
    })
  },
  async saveGraph(path: string, content: string) {
    if (desktop()) return withDesktopLogging('SaveGraph', () => desktop()!.SaveGraph(path, content))
    download(path || 'Untitled.obp', content, 'application/json')
    return path || 'Untitled.obp'
  },
  async forceSaveGraph(path: string, content: string) {
    if (!desktop()) throw new Error('Force overwrite is only available in the desktop application')
    return withDesktopLogging('ForceSaveGraph', () => desktop()!.ForceSaveGraph(path, content))
  },
  async currentWorkingDirectory() { return desktop() ? withDesktopLogging('CurrentWorkingDirectory', () => desktop()!.CurrentWorkingDirectory()) : '' },
  async chooseWorkspace() { return desktop() ? withDesktopLogging('ChooseWorkspace', () => desktop()!.ChooseWorkspace()) : '' },
  async loadProjectSettings(root: string): Promise<ProjectSettingsResult | null> {
    return desktop() ? withDesktopLogging('LoadProjectSettings', () => desktop()!.LoadProjectSettings(root)) : null
  },
  async saveProjectSettings(root: string, content: string) {
    if (desktop()) return withDesktopLogging('SaveProjectSettings', () => desktop()!.SaveProjectSettings(root, content))
    localStorage.setItem(`origin-blueprint-project:${root || 'browser'}`, content)
    return root ? `${root}/originblueprint.project` : 'originblueprint.project'
  },
  async chooseDataFile(mode: 'open' | 'save') {
    if (desktop()) return withDesktopLogging('ChooseDataFile', () => desktop()!.ChooseDataFile(mode))
    if (mode === 'save') return window.prompt('Output file name', 'output.csv') ?? ''
    return new Promise<string>(resolve => {
      const input = document.createElement('input')
      input.type = 'file'
      input.onchange = () => resolve(input.files?.[0]?.name ?? '')
      input.click()
    })
  },
  async newWindow() {
    if (desktop()) return withDesktopLogging('NewWindow', () => desktop()!.NewWindow())
    window.open(window.location.href, '_blank', 'noopener')
  },
  async clearRecentFiles() {
    if (desktop()) await withDesktopLogging('ClearRecentFiles', () => desktop()!.ClearRecentFiles())
  },
  async quit() {
    if (desktop()) await withDesktopLogging('Quit', () => desktop()!.Quit())
    else window.close()
  },
  async listWorkspace(path: string) { return desktop() ? withDesktopLogging('ListWorkspace', () => desktop()!.ListWorkspace(path)) : [] },
  async findNodeReferences(root: string, typeId: string) { return desktop() ? withDesktopLogging('FindNodeReferences', () => desktop()!.FindNodeReferences(root, typeId)) : [] },
  async revealInFolder(path: string) { if (desktop()) await withDesktopLogging('RevealInFolder', () => desktop()!.RevealInFolder(path)) },
  async openExternalURL(url: string) {
    if (desktop()) return withDesktopLogging('OpenExternalURL', () => desktop()!.OpenExternalURL(url))
    window.open(url, '_blank', 'noopener')
  },
  async recentFiles() { return desktop() ? withDesktopLogging('GetRecentFiles', () => desktop()!.GetRecentFiles()) : [] },
  async validateGraph(content: string, workspaceRoot = '', sourcePath = ''): Promise<ValidationIssue[]> {
    if (desktop()) return withDesktopLogging('ValidateGraphForWorkspace', () => desktop()!.ValidateGraphForWorkspace(content, workspaceRoot, sourcePath))
    try {
      JSON.parse(content)
      return [{ severity: 'warning', code: 'engine.unavailable', message: 'Go engine compilation validation is unavailable in browser mode' }]
    } catch {
      return [{ severity: 'error', code: 'document.invalid-json', message: 'Invalid graph document' }]
    }
  },
  async migrateLegacyGraph(content: string) { return desktop() ? withDesktopLogging('MigrateLegacyGraph', () => desktop()!.MigrateLegacyGraph(content)) : '' },
  async exportLegacyGraph(content: string) { return desktop() ? withDesktopLogging('ExportLegacyGraph', () => desktop()!.ExportLegacyGraph(content)) : content },
  async loadNodeSchemas(): Promise<NodeSchemaLoadResult> {
    const result = normalizeNodeSchemaDocumentLoadResult(desktop() ? await withDesktopLogging('LoadNodeSchemaDocuments', () => desktop()!.LoadNodeSchemaDocuments()) : await loadBrowserNodeSchemaDocuments())
    return parseNodeSchemaDocuments(result.documents, result.errors)
  },
  onCloseRequest(callback: () => void) {
    const runtime = (window as unknown as { runtime?: WailsRuntime }).runtime
    return runtime?.EventsOnMultiple?.('origin:before-close', callback, -1) ?? (() => {})
  },
  async exportPNG(dataURL: string) {
    if (desktop()) return withDesktopLogging('ExportPNG', () => desktop()!.ExportPNG(dataURL))
    const anchor = document.createElement('a'); anchor.href = dataURL; anchor.download = 'OriginBlueprint.png'; anchor.click()
    return anchor.download
  },
  async saveRecoverySnapshot(sourcePath: string, tabID: string, documentJSON: string, issuesJSON: string): Promise<RecoverySnapshotResult | null> {
    return desktop() ? withDesktopLogging('SaveRecoverySnapshot', () => desktop()!.SaveRecoverySnapshot(sourcePath, tabID, documentJSON, issuesJSON)) : null
  },
  async listRecoverySnapshots(): Promise<RecoverySnapshotResult[]> {
    return desktop() ? withDesktopLogging('ListRecoverySnapshots', () => desktop()!.ListRecoverySnapshots()) : []
  },
  async readRecoverySnapshot(path: string): Promise<string> {
    return desktop() ? withDesktopLogging('ReadRecoverySnapshot', () => desktop()!.ReadRecoverySnapshot(path)) : ''
  },
  async deleteRecoverySnapshot(path: string): Promise<void> {
    if (desktop()) await withDesktopLogging('DeleteRecoverySnapshot', () => desktop()!.DeleteRecoverySnapshot(path))
  },
  async deleteRecoverySnapshots(sourcePath: string, tabID: string): Promise<void> {
    if (desktop()) await withDesktopLogging('DeleteRecoverySnapshots', () => desktop()!.DeleteRecoverySnapshots(sourcePath, tabID))
  },
  async chooseExportPNGPath(defaultDirectory = '') {
    if (desktop()) return withDesktopLogging('ChooseExportPNGPath', () => desktop()!.ChooseExportPNGPath(defaultDirectory))
    return 'OriginBlueprint.png'
  },
  async savePNG(path: string, dataURL: string) {
    if (desktop()) return withDesktopLogging('SavePNG', () => desktop()!.SavePNG(path, dataURL))
    const anchor = document.createElement('a'); anchor.href = dataURL; anchor.download = path || 'OriginBlueprint.png'; anchor.click()
    return anchor.download
  }
}
