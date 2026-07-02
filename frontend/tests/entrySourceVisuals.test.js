import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = name => readFileSync(resolve(__dirname, `../src/${name}`), 'utf8')

const app = source('App.vue')
const blueprintNode = source('editor/BlueprintNode.vue')
const createEditor = source('editor/createEditor.ts')
const implicitEntryLinks = source('editor/implicitEntryLinks.ts')
const nodeRegistry = source('editor/nodeRegistry.ts')
const runtimeNodeSchemas = source('editor/runtimeNodeSchemas.ts')
const types = source('editor/types.ts')

assert(runtimeNodeSchemas.includes('sourceName: name'), 'legacy JSON node name must be preserved as runtime schema sourceName')
assert(nodeRegistry.includes('sourceName?: string'), 'node definitions must keep the schema sourceName at runtime')
assert(nodeRegistry.includes('entrySourceColor(schema.sourceName'), 'entry source color must be derived from schema sourceName')
assert(types.includes('entrySourceColor?: string'), 'entry source color must be a runtime-only node field')
assert(!types.includes('entrySourceColor') || !source('editor/document.ts').includes('entrySourceColor'), 'entry source color must not be persisted in graph documents')

assert(implicitEntryLinks.includes('entrySourceColor'), 'entry bindings must carry the source color to target inputs')
assert(blueprintNode.includes('--entry-source-color'), 'node rendering must expose the entry source color as a CSS variable')
assert(blueprintNode.includes('entry-binding-badge') && blueprintNode.includes('entrySourceColor'), 'entry binding badges must use the source color')

assert(createEditor.includes('isDuplicateEntryNode'), 'editor must detect duplicate ordinary entry nodes')
assert(createEditor.includes('该入口节点已存在，不能重复添加'), 'duplicate ordinary entry insertion must show a clear Chinese error')
assert(createEditor.includes('allowEntryNodes'), 'editor addNode must support blocking ordinary entry nodes for function blueprints')
assert(app.includes('allowEntryNodes: !isFunctionBlueprintTab.value'), 'function blueprints must block ordinary entry nodes')
assert(app.includes('filteredModuleItems'), 'function blueprints must hide ordinary entry nodes from the module list')
assert(app.includes('canvasToast') && app.includes('showCanvasToast'), 'add-node failures must show an in-canvas toast')
assert(app.includes('class="canvas-toast"'), 'canvas toast must render inside the editor canvas area')
assert(app.includes('showCanvasToast(status.value, position'), 'add-node errors must use the attempted insertion position for the toast')

console.log('entrySourceVisuals tests passed')
