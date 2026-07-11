import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const source = name => readFileSync(resolve(root, 'frontend/src', name), 'utf8')
const eventNodes = JSON.parse(readFileSync(resolve(root, 'nodes/Event.json'), 'utf8'))
const entranceNodes = JSON.parse(readFileSync(resolve(root, 'nodes/Entrance.json'), 'utf8'))
const names = new Set(eventNodes.map(node => node.name))

for (const name of [
  'Delay',
  'SetTimerByFunction',
  'ClearTimer',
  'PauseTimer',
  'UnpauseTimer',
  'IsTimerActive',
  'IsTimerPaused',
  'IsTimerValid',
  'GetTimerRemaining',
  'GetTimerElapsed',
]) {
  assert(names.has(name), `Event.json must register ${name}`)
}
assert(!names.has('CreateTimer') && !names.has('CloseTimer'), 'old timer nodes must stay removed')
assert(!entranceNodes.some(node => /timer/i.test(node.name)), 'old Timer event entrance must stay removed')

const registry = source('editor/nodeRegistry.ts')
const runtimeSchemas = source('editor/runtimeNodeSchemas.ts')
const editor = source('editor/createEditor.ts')
const nodeView = source('editor/BlueprintNode.vue')
const documentSource = source('editor/document.ts')
const app = source('App.vue')

assert(runtimeSchemas.includes("case 'timerhandle':") && runtimeSchemas.includes("return 'timerhandle'"), 'runtime schemas must map TimerHandle ports')
assert(registry.includes("timerhandle: new ClassicPreset.Socket('timerhandle')"), 'registry must expose a distinct TimerHandle socket')
assert(registry.includes('createSetTimerByFunctionNode'), 'registry must create dynamic timer function nodes')
assert(registry.includes('applyTimerFunctionMetadata'), 'timer function selection must rebuild callback argument ports')
assert(editor.includes("node.typeId === 'origin.timer.set-by-function'"), 'editor must restore and manage timer function nodes')
assert(editor.includes("node.typeId === 'origin.function.call' || node.typeId === 'origin.timer.set-by-function'"), 'reference highlighting must include timer function references')
assert(nodeView.includes('data.functionOptions') && nodeView.includes('data.functionSelectorLabel'), 'timer function nodes must render the function selector')
assert(nodeView.includes('filteredTimerFunctions') && nodeView.includes('type="search"'), 'timer function selector must filter a searchable option list')
assert(nodeView.includes("event.key === 'Enter'") && nodeView.includes("event.key === 'Escape'"), 'timer function selector must support keyboard selection and dismissal')
assert(nodeView.includes('functionReferenceMissing') && nodeView.includes('functionMissingLabel'), 'missing timer function references must remain visible on the node')
assert(documentSource.includes("'timerhandle'"), 'GraphDocument variables must support TimerHandle')
assert(app.includes("String(node.typeId ?? '').startsWith('origin.timer.')"), 'timer nodes must force native persistence')
assert(app.includes("variable.type === 'timerhandle'"), 'TimerHandle variables must force native persistence')
assert(app.includes('forceNativeSaveAs'), 'native timer graphs opened from .vgf must use Save As instead of overwriting legacy files')
assert(editor.includes("callbacks.onDirty()"), 'undo and redo must mark restored graph state dirty')
assert(editor.includes("outputs.set('timerHandle', 'timerhandle')"), 'timer signature sync must preserve static TimerHandle output connections')

console.log('timerNodes tests passed')
