import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = name => readFileSync(resolve(__dirname, `../src/${name}`), 'utf8')

const app = source('App.vue')
const documentSource = source('editor/document.ts')
const nodeRegistry = source('editor/nodeRegistry.ts')
const createEditor = source('editor/createEditor.ts')

assert(documentSource.includes("export type FunctionNodeRole = 'call' | 'entry' | 'return'"), 'document metadata must describe function node roles')
assert(documentSource.includes('functionSignature?: FunctionSignature'), 'node properties must persist the function signature snapshot')
assert(documentSource.includes('functionPath?: string'), 'node properties must persist workspace function paths')

assert(nodeRegistry.includes('export function createFunctionCallNode'), 'node registry must create function call nodes')
assert(nodeRegistry.includes('export function createFunctionEntryNode'), 'node registry must create function entry nodes')
assert(nodeRegistry.includes('export function createFunctionReturnNode'), 'node registry must create function return nodes')
assert(nodeRegistry.includes("result.addInput('exec'"), 'function call and return nodes must expose exec inputs')
assert(nodeRegistry.includes("result.addOutput('exec'"), 'function call and entry nodes must expose exec outputs')

assert(createEditor.includes('addFunctionCallNode(spec'), 'editor handle must expose addFunctionCallNode')
assert(createEditor.includes('addFunctionEntryNode(spec'), 'editor handle must expose addFunctionEntryNode')
assert(createEditor.includes('addFunctionReturnNode(spec'), 'editor handle must expose addFunctionReturnNode')
assert(createEditor.includes('syncFunctionSignature(spec'), 'editor handle must expose function signature synchronization')
assert(createEditor.includes("typeId.startsWith('origin.function.')"), 'restore must recognize persisted function nodes')
assert(createEditor.includes("typeof item.typeId === 'string'"), 'restore must not crash on legacy nodes without typeId')
assert(createEditor.includes('createRestoredNode(item, typeId'), 'restore must use a shared node restoration helper')
assert(createEditor.includes('item.properties?.legacyClass'), 'restore must fall back to legacy placeholders for unregistered migrated nodes')
assert(createEditor.includes('cloneFunctionSignatureFromProperties'), 'restore must safely clone empty function signature objects from legacy documents')
assert(!createEditor.includes('properties.functionSignature.inputs.map'), 'restore must not assume functionSignature.inputs exists')
assert(createEditor.includes('functionSignature: node.functionSignature'), 'snapshot must serialize function signatures')
assert(createEditor.includes('functionPropertiesForSnapshot(node)'), 'copy and snapshot must share function metadata serialization')

assert(app.includes('beginModuleItemPointerDrag($event, item)'), 'module library must route function drag through function-aware handler')
assert(app.includes('addModuleItemAt(item'), 'module library must route function double-click through function-aware creation')
assert(app.includes('syncFunctionSignatureToGraph'), 'signature edits must sync function entry and return nodes')
assert(app.includes('@change=\"syncFunctionSignatureToGraph\"'), 'signature editor changes must trigger graph synchronization')
assert(app.includes('loadFunctionSignatureForModuleItem'), 'workspace function call nodes must load signatures from .obpf files')
assert(app.includes('platform.openGraph(item.path)'), 'workspace function signatures must be read from the function file')
assert(app.includes('isNativeGraphDocument(parsed)'), 'openGraph must distinguish native documents from legacy-shaped files')
assert(app.includes('existing.document = document'), 'reopening an already-open graph must refresh stale tab contents')
assert(!app.includes('!item.functionPlaceholder && beginNodePointerDrag'), 'function placeholders must not disable pointer drag')
assert(!app.includes('!item.functionPlaceholder && addNodeAt'), 'function placeholders must not disable double-click creation')

console.log('functionNodes tests passed')
