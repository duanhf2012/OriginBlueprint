export interface FileResult { path: string; content: string }
export interface WorkspaceEntry { name: string; path: string; isDir: boolean }

type DesktopApp = {
  OpenGraph(path: string): Promise<FileResult>
  SaveGraph(path: string, content: string): Promise<string>
  ChooseWorkspace(): Promise<string>
  ListWorkspace(path: string): Promise<WorkspaceEntry[]>
  ExportPNG(dataURL: string): Promise<string>
  GetRecentFiles(): Promise<string[]>
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
  async listWorkspace(path: string) { return desktop() ? desktop()!.ListWorkspace(path) : [] },
  async recentFiles() { return desktop() ? desktop()!.GetRecentFiles() : [] },
  async exportPNG(dataURL: string) {
    if (desktop()) return desktop()!.ExportPNG(dataURL)
    const anchor = document.createElement('a'); anchor.href = dataURL; anchor.download = 'OriginBlueprint.png'; anchor.click()
    return anchor.download
  }
}
