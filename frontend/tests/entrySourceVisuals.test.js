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
assert(nodeRegistry.includes('entryColorKeyForSchema') && nodeRegistry.includes('entrySourceColor(entryColorKey)'), 'entry source color must be derived from a stable schema key')
assert(nodeRegistry.includes('result.entrySourceKey = spec.functionId') && nodeRegistry.includes('result.entrySourceColor = entrySourceColor(spec.functionId)'), 'function Entry diamonds and binding badges must share a stable function source color')
assert(types.includes('entrySourceColor?: string'), 'entry source color must be a runtime-only node field')
assert(!types.includes('entrySourceColor') || !source('editor/document.ts').includes('entrySourceColor'), 'entry source color must not be persisted in graph documents')
assert(implicitEntryLinks.includes('export const entrySourcePalette') && implicitEntryLinks.includes('assignEntrySourcePalette'), 'entry source colors must come from the curated 50-color palette assigned by stable sort order')
assert(nodeRegistry.includes('assignEntrySourcePalette(allNodeDefinitions'), 'node registration must batch-assign palette colors so entrances stay pairwise distinct')
assert(implicitEntryLinks.includes('hashEntryColor(key)'), 'keys outside the palette batch must keep the stable hash fallback color')
assert(implicitEntryLinks.includes('hslToHex') && implicitEntryLinks.includes('hash % 360'), 'hash fallback colors must be generated from the full source hash')
assert(implicitEntryLinks.includes('0x7feb352d') && implicitEntryLinks.includes('0x846ca68b'), 'entry source hash must be avalanche-mixed so similar entrance names do not cluster into similar colors')

assert(implicitEntryLinks.includes('entrySourceColor'), 'entry bindings must carry the source color to target inputs')
assert(implicitEntryLinks.includes('export function looksLikeEntryName'), 'the entry-name heuristic must be shared for custom entrance detection')
assert(nodeRegistry.includes('entryColorKeyForSchema'), 'custom workspace entrances (e.g. Chinese “…入口”) must get a stable identity color')
assert(nodeRegistry.includes('looksLikeEntryName(schema.sourceName) || looksLikeEntryName(schema.title)'), 'custom entrance color detection must consider both schema name and title')
assert(nodeRegistry.includes('} else if (entryColorKey) {'), 'custom entrance colors must not set entrySourceKey or change entry add/duplicate policies')
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
