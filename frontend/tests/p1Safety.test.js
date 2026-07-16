import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const documentSource = fs.readFileSync(path.join(root, 'src/editor/document.ts'), 'utf8')
const editorSource = fs.readFileSync(path.join(root, 'src/editor/createEditor.ts'), 'utf8')
const platformSource = fs.readFileSync(path.join(root, 'src/platform.ts'), 'utf8')

assert(documentSource.includes('export interface RestoreLossReport'), 'restore must return a typed loss report')
assert(editorSource.includes('loadDocument(document: GraphDocument): Promise<RestoreLossReport>'), 'editor load must expose restore loss to the application')
assert(editorSource.includes('finally {') && editorSource.includes('restoring = false'), 'restore must always leave restoring mode after failure')
assert(platformSource.includes('ForceSaveGraph(path: string, content: string)'), 'desktop bridge must expose force save with backup')
assert(platformSource.includes("withDesktopLogging('ForceSaveGraph'"), 'force save must use the backend API')

assert(editorSource.includes('const maxDynamicSequenceOutputs = 256'), 'editor sequence output limit must match the engine')
assert(!editorSource.includes('Math.min(12'), 'editor must not truncate sequence outputs above 12')

console.log('P1 safety tests passed')
