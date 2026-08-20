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
const style = source('style.css')

assert(app.includes('autoSaveIntervalMs') && app.includes('window.setInterval') && app.includes('autoSaveDirtyTabs'), 'project autosave setting must schedule the autosave worker')
assert(app.includes('await validateForPersistence(tab, document)') && app.includes('isAutoSaveEligible'), 'autosave must validate through the shared core save gate and apply compatibility-safe eligibility')
assert(control.includes('origin-control-edit-start') && control.includes('origin-control-edit-commit'), 'inline scalar and array controls must expose edit transaction boundaries')
assert(node.includes('beginControlEdit') && node.includes('origin-dynamic-branch-change'), 'dynamic branch values must join control edit transactions')
assert(editor.includes("addEventListener('origin-control-edit-start'") && editor.includes("addEventListener('origin-control-edit-commit'"), 'the editor must record control edit transaction boundaries')
assert(!editor.includes('undoStack.push(') && !editor.includes('redoStack.push('), 'all history writes must pass through the bounded history helper')
assert(history.includes('editorHistoryLimit = 100'), 'editor history must remain capped at 100 snapshots')
assert(app.includes("title: '局部变量'") && app.includes("title: '全局变量'"), 'the variable panel must expose separate local and global sections')
assert(!app.includes('scopeEntry.description'), 'variable scope headers must remain single-line at narrow panel widths')
assert(app.includes("addVariable('default', scopeEntry.scope)") && app.includes("addVariable(entry.group.id, scopeEntry.scope)"), 'each variable scope section must have an explicit add path')
assert(app.includes("scope === 'instance' && isFunctionBlueprintTab.value"), 'function blueprints must reject creating global variables')
assert(!app.includes('variable-group-manager') && !app.includes('variableGroupManagerEntries'), 'the ambiguous shared group manager must not return')
assert(app.includes('addVariableGroup(scopeEntry.scope)') && app.includes('variableGroupsForScope(variableGroups.value, section.scope)'), 'each scope section must create and render only its own groups')
assert(app.includes('matchingVariableGroupId(variableGroups.value, variable.groupId, scope)'), 'changing variable scope must map to a same-named target group or Default')
assert(app.includes('planVariableGroupDrop') && app.includes('dropVariableIntoGroup') && app.includes('variableDropClass') && style.includes('.variable-group.variable-drop-scope-change'), 'variable groups must accept explicit same-scope and cross-scope drop plans')
assert(app.includes("effectAllowed = 'copyMove'") && app.includes("dropEffect = plan.kind === 'move' || plan.kind === 'scope-change' ? 'move' : 'none'"), 'variable drags must remain copyable to the canvas while group drops use move semantics')
assert(app.includes('window.confirm(variableScopeChangeConfirmation(variable, plan))'), 'cross-scope variable drops must require explicit confirmation')
assert(app.includes("保存时必须另存为 .obp") && app.includes("reason === 'function-instance-scope'"), 'cross-scope drops must retain .vgf and function-blueprint compatibility guards')
assert(app.includes(':style="socketStyle(variable.type)"') && style.includes('color: var(--socket-label-color, var(--socket-color, #ddd))'), 'variable names must reuse the canonical socket type colors')
assert(!app.includes('<span class="variable-scope">'), 'variable rows must not repeat the scope already conveyed by their local or global section')
assert(app.includes("savedPanelWidth('origin-blueprint-left-tools-width', leftToolsDefaultWidth, leftToolsMinWidth, leftToolsMaxWidth)") && app.includes('tools: clampNumber(panels.tools, fallback.layout.panels.tools, leftToolsMinWidth, leftToolsMaxWidth)') && style.includes('var(--left-tools-width, 280px)'), 'the variable and detail sidebar must migrate saved layouts to a wider readable range')
assert(app.includes('setVariableIntegerDefault') && app.includes('inputmode="numeric"'), 'integer variable defaults must use precision-preserving text controls')
assert(app.includes('isValidIntegerDefault(variable.defaultValue)'), 'integer variable defaults must be rejected before syncing')
assert(!app.includes('toggleVariableGroup'), 'variable group collapse must not couple local and global scope presentation')

console.log('p2p3Integration tests passed')
