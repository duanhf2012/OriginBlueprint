import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = name => readFileSync(resolve(__dirname, `../src/${name}`), 'utf8')

const documentSource = source('editor/document.ts')
const nodeRegistry = source('editor/nodeRegistry.ts')
const runtimeNodeSchemas = source('editor/runtimeNodeSchemas.ts')
const socketTheme = source('editor/socketTheme.ts')

for (const removed of ["'file'", "'table'", "'dictionary'"]) {
  assert(!documentSource.includes(removed), `GraphDocument variable types must not include ${removed}`)
  assert(!nodeRegistry.includes(removed), `node registry sockets must not include ${removed}`)
  assert(!runtimeNodeSchemas.includes(removed), `runtime node schema conversion must not include ${removed}`)
}

for (const removed of ['origin.io.', 'origin.table.', 'origin.dictionary.', 'foreach-table-row', 'DataFrame', 'Dict']) {
  assert(!runtimeNodeSchemas.includes(removed), `runtime node schema conversion must not include ${removed}`)
}

assert(!socketTheme.includes('file:'), 'socket theme must not define file colors')
assert(!socketTheme.includes('table:'), 'socket theme must not define table colors')
assert(!socketTheme.includes('dictionary:'), 'socket theme must not define dictionary colors')

console.log('removedDataTypes tests passed')
