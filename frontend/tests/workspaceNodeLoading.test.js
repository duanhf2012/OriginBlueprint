import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src')
const source = relative => fs.readFileSync(path.join(root, relative), 'utf8')

function assert(value, message) {
  if (!value) throw new Error(message)
}

const app = source('App.vue')
const platform = source('platform.ts')

assert(platform.includes('LoadNodeSchemaDocumentsForWorkspace(workspaceRoot: string)'), 'desktop contract must expose workspace-aware node loading')
assert(platform.includes("async loadNodeSchemas(workspaceRoot = '')"), 'platform node loading must accept the active workspace')
assert(platform.includes("withDesktopLogging('LoadNodeSchemaDocumentsForWorkspace'"), 'desktop node loading must call the workspace-aware backend method')
assert(app.includes('result = await platform.loadNodeSchemas(requestedWorkspace)'), 'the node library must pass the requested workspace to the platform')
assert(app.includes('const token = ++nodeSchemaLoadToken') && app.match(/token !== nodeSchemaLoadToken/g)?.length >= 2, 'stale node schema loads must be discarded')
assert(app.includes('await loadWorkspace(workspaceRoot.value, false)'), 'refreshing the file tree must not reload node schemas')
assert(app.includes('const nodeLoadStatus = refreshNodeSchemas ? await loadRuntimeNodeLibrary(path) :'), 'opening or switching workspace must reload its node schemas')

const periodicRefresh = app.match(/async function refreshWorkspaceVisibleDirectories\([^)]*\)[\s\S]*?\r?\n}\r?\n/)?.[0] ?? ''
assert(periodicRefresh && !periodicRefresh.includes('loadRuntimeNodeLibrary'), 'periodic workspace polling must not reload node schemas')

console.log('workspace node loading tests passed')
