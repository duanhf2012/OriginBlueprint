import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(resolve(__dirname, '../src/editor/createEditor.ts'), 'utf8')

function findFunctionBody(source, functionName) {
  let start = source.indexOf(`async function ${functionName}(`)
  if (start < 0) {
    start = source.indexOf(`function ${functionName}(`)
    if (start < 0) throw new Error(`function ${functionName} not found`)
  }
  const braceStart = source.indexOf('{', start)
  let depth = 0
  for (let i = braceStart; i < source.length; i++) {
    if (source[i] === '{') depth++
    if (source[i] === '}') depth--
    if (depth === 0) return source.slice(start, i + 1)
  }
  throw new Error(`could not find end of function ${functionName}`)
}

const restoreBody = findFunctionBody(source, 'restore')
const pasteBody = findFunctionBody(source, 'paste')
const applyPropertiesBody = findFunctionBody(source, 'applyNodeProperties')

assert(restoreBody.includes('applyNodeProperties(node, item.properties)'), 'restore must apply persisted node properties')
assert(pasteBody.includes('applyNodeProperties(node, item.properties)'), 'paste must apply clipboard node properties')

assert(applyPropertiesBody.includes('node.legacyClass = properties?.legacyClass'), 'node property restoration must preserve legacyClass')
assert(applyPropertiesBody.includes('node.legacyInputs = properties?.legacyInputs?.map'), 'node property restoration must clone legacyInputs')
assert(applyPropertiesBody.includes('node.legacyOutputs = properties?.legacyOutputs?.map'), 'node property restoration must clone legacyOutputs')

const snapshotBody = findFunctionBody(source, 'snapshot')
assert(snapshotBody.includes('legacyInputs: legacyInputsForSnapshot(node)'), 'snapshot must serialize legacyInputs')
assert(snapshotBody.includes('legacyOutputs: legacyOutputsForSnapshot(node)'), 'snapshot must serialize legacyOutputs')

console.log('legacyPropertyPreservation tests passed')
