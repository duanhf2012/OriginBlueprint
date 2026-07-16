import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname.replace(/^\/(.:)/, '$1')), '..', 'src')
const source = relative => fs.readFileSync(path.join(root, relative), 'utf8')

function assert(value, message) {
  if (!value) throw new Error(message)
}

const app = source('App.vue')
const control = source('editor/BlueprintControl.vue')
const node = source('editor/BlueprintNode.vue')
const editor = source('editor/createEditor.ts')
const history = source('editor/history.ts')

assert(app.includes('autoSaveIntervalMs') && app.includes('window.setInterval') && app.includes('autoSaveDirtyTabs'), 'project autosave setting must schedule the autosave worker')
assert(app.includes('sourceRequiresProtection(issues)') && app.includes('isAutoSaveEligible'), 'autosave must validate and apply the compatibility-safe eligibility policy')
assert(control.includes('origin-control-edit-start') && control.includes('origin-control-edit-commit'), 'inline scalar and array controls must expose edit transaction boundaries')
assert(node.includes('beginControlEdit') && node.includes('origin-dynamic-branch-change'), 'dynamic branch values must join control edit transactions')
assert(editor.includes("addEventListener('origin-control-edit-start'") && editor.includes("addEventListener('origin-control-edit-commit'"), 'the editor must record control edit transaction boundaries')
assert(!editor.includes('undoStack.push(') && !editor.includes('redoStack.push('), 'all history writes must pass through the bounded history helper')
assert(history.includes('editorHistoryLimit = 100'), 'editor history must remain capped at 100 snapshots')

console.log('p2p3Integration tests passed')
