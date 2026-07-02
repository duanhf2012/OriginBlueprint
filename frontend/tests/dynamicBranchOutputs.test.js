import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = name => readFileSync(resolve(__dirname, `../src/${name}`), 'utf8')

const createEditor = source('editor/createEditor.ts')
const nodeRegistry = source('editor/nodeRegistry.ts')
const runtimeNodeSchemas = source('editor/runtimeNodeSchemas.ts')
const types = source('editor/types.ts')

assert(runtimeNodeSchemas.includes("id === 'origin.flow.equal-switch-new'") || runtimeNodeSchemas.includes('dynamicBranch'), 'equal-switch-new must keep dynamic branch metadata')
assert(runtimeNodeSchemas.includes("EqualSwitch: { typeId: 'origin.flow.equal-switch', inputs: ['exec', 'value', 'cases'], outputs: ['otherwise', 'case0', 'case1', 'case2', 'case3', 'case4'] }"), 'legacy EqualSwitch must keep case key mapping for old port ids')
assert(runtimeNodeSchemas.includes("hiddenOutputKeys: ['case0']"), 'legacy EqualSwitch must hide the historical case0 placeholder')
assert(types.includes('outputTemplate?:'), 'dynamicBranch must describe generated outputs with outputTemplate')
assert(!nodeRegistry.includes('dynamicBranchWithOutputLimit'), 'node registry must not cap dynamic branches to declared output placeholders')
assert(!nodeRegistry.includes('declaredDynamicBranchOutputs'), 'node registry must not count declared case output placeholders')
assert(!nodeRegistry.includes('Math.min(branch.maxBranches, declared)'), 'dynamicBranch.maxBranches must be the single dynamic branch limit')
assert(createEditor.includes('syncDynamicBranchOutputs'), 'editor must synchronize dynamic branch output ports when branch count changes')
assert(createEditor.includes('node.addOutput(outputKey'), 'dynamic branch synchronization may re-add declared output ports after shrinking')
assert(createEditor.includes('node.removeOutput(key)'), 'dynamic branch synchronization must remove overflow output ports')
assert(createEditor.includes('syncDynamicBranchOutputs(node, detail.count)'), 'dynamic branch change handler must sync output ports from the new count')
assert(createEditor.includes('syncDynamicBranchOutputs(node, dynamicBranchValueCount(node))'), 'restored or edited dynamic branch nodes must sync outputs from control values')
assert(createEditor.includes('config.outputTemplate'), 'dynamic branch synchronization must use outputTemplate for generated output sockets')

console.log('dynamicBranchOutputs tests passed')
