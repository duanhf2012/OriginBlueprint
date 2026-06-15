export interface FileResult { path: string; content: string }
export interface WorkspaceEntry { name: string; path: string; isDir: boolean }
export interface ValidationIssue { severity: 'error' | 'warning'; code: string; message: string; nodeId?: string }
export interface ExecutionLog { level: 'debug' | 'info' | 'warning' | 'error'; message: string; nodeId?: string }
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
