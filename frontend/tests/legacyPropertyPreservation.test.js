import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(resolve(__dirname, '../src/editor/createEditor.ts'), 'utf8')

function findFunctionBody(source, functionName) {
  const start = source.indexOf(`async function ${functionName}`)
  if (start < 0) {
    const start2 = source.indexOf(`function ${functionName}`)
    if (start2 < 0) throw new Error(`function ${functionName} not found`)
    return source.slice(start2, source.indexOf('}', start2 + 2000))
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

assert(restoreBody.includes("if (item.properties?.legacyClass)"), 'restore must preserve legacyClass for custom runtime nodes')
assert(restoreBody.includes('node.legacyInputs = item.properties.legacyInputs'), 'restore must copy legacyInputs from document properties')
assert(restoreBody.includes('node.legacyOutputs = item.properties.legacyOutputs'), 'restore must copy legacyOutputs from document properties')

assert(pasteBody.includes("if (item.properties?.legacyClass)"), 'paste must preserve legacyClass for custom runtime nodes')
assert(pasteBody.includes('node.legacyInputs = item.properties.legacyInputs'), 'paste must copy legacyInputs from clipboard properties')
assert(pasteBody.includes('node.legacyOutputs = item.properties.legacyOutputs'), 'paste must copy legacyOutputs from clipboard properties')

const snapshotBody = findFunctionBody(source, 'snapshot')
assert(snapshotBody.includes('legacyInputs: node.legacyInputs'), 'snapshot must serialize legacyInputs')
assert(snapshotBody.includes('legacyOutputs: node.legacyOutputs'), 'snapshot must serialize legacyOutputs')

console.log('legacyPropertyPreservation tests passed')
