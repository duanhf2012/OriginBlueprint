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

for (const name of ['Delay', 'CreateTimer', 'ClearTimerByKey']) {
  assert(names.has(name), `Event.json must register ${name}`)
}
for (const name of ['SetTimerByFunction', 'ClearTimer', 'PauseTimer', 'UnpauseTimer', 'IsTimerActive', 'IsTimerPaused', 'IsTimerValid', 'GetTimerRemaining', 'GetTimerElapsed']) {
  assert(!names.has(name), `retired timer node ${name} must not remain in the node library`)
}
assert(!entranceNodes.some(node => /timer/i.test(node.name)), 'a generic Timer event entrance must not be required')

const createTimer = eventNodes.find(node => node.name === 'CreateTimer')
assert(createTimer.inputs.map(port => port.port_id).join(',') === '0,1,2,3,4', 'CreateTimer inputs must keep stable port IDs')
assert(createTimer.outputs[0].type === 'exec' && createTimer.outputs[1].type === 'callback', 'Created and OnTriggered must use different control port types')
assert(createTimer.outputs[1].name_en === 'On Triggered', 'callback output must be clearly named')
assert(createTimer.inputs.some(port => port.data_type === 'String' && /Key/.test(port.name_en)), 'CreateTimer must expose a literal Timer Key')

const registry = source('editor/nodeRegistry.ts')
const runtimeSchemas = source('editor/runtimeNodeSchemas.ts')
const editor = source('editor/createEditor.ts')
const socketView = source('editor/BlueprintSocket.vue')
const connectionView = source('editor/BlueprintConnection.vue')
const theme = source('editor/socketTheme.ts')
const app = source('App.vue')

assert(runtimeSchemas.includes("'callback'"), 'runtime schemas must preserve callback ports')
assert(runtimeSchemas.includes('CreateTimer') && runtimeSchemas.includes('ClearTimerByKey'), 'legacy node schemas must map the new timer classes')
assert(registry.includes("callback: new ClassicPreset.Socket('callback')"), 'registry must expose a distinct callback socket')
assert(theme.includes('callback:') && theme.includes('#ff9f2f'), 'callback socket must use its own color')
assert(socketView.includes('isCallback') && socketView.includes('<circle'), 'callback socket must render a distinct diamond/dot shape')
assert(connectionView.includes('socket-callback') && connectionView.includes('stroke-dasharray'), 'callback connection must have distinct styling')
assert(editor.includes("types.source === 'callback' && types.target === 'exec'"), 'callback outputs must connect to ordinary exec inputs')
assert(editor.includes('Timer Key must be entered directly'), 'Timer Key must reject dynamic data connections')
assert(editor.includes('createRetiredTimerNode') && editor.includes("legacyModule: properties?.legacyModule || 'retired-timer'"), 'retired native timer nodes must restore as visible legacy placeholders')
assert(app.includes("String(node.typeId ?? '').startsWith('origin.timer.')"), 'timer graphs must use native persistence')

console.log('timerNodes tests passed')
