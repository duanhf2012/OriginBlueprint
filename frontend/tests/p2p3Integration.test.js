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
assert(app.includes('await validateForPersistence(tab, document)') && app.includes('isAutoSaveEligible'), 'autosave must validate through the shared core save gate and apply compatibility-safe eligibility')
assert(control.includes('origin-control-edit-start') && control.includes('origin-control-edit-commit'), 'inline scalar and array controls must expose edit transaction boundaries')
assert(node.includes('beginControlEdit') && node.includes('origin-dynamic-branch-change'), 'dynamic branch values must join control edit transactions')
assert(editor.includes("addEventListener('origin-control-edit-start'") && editor.includes("addEventListener('origin-control-edit-commit'"), 'the editor must record control edit transaction boundaries')
assert(!editor.includes('undoStack.push(') && !editor.includes('redoStack.push('), 'all history writes must pass through the bounded history helper')
assert(history.includes('editorHistoryLimit = 100'), 'editor history must remain capped at 100 snapshots')
assert(app.includes("title: '局部变量'") && app.includes("title: '全局变量'"), 'the variable panel must expose separate local and global sections')
assert(app.includes("addVariable('default', scopeEntry.scope)") && app.includes("addVariable(entry.group.id, scopeEntry.scope)"), 'each variable scope section must have an explicit add path')
assert(app.includes("scope === 'instance' && isFunctionBlueprintTab.value"), 'function blueprints must reject creating global variables')

console.log('p2p3Integration tests passed')
