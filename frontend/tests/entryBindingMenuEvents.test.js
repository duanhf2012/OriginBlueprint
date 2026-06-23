import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const __dirname = dirname(fileURLToPath(import.meta.url))
const editorSource = readFileSync(resolve(__dirname, '../src/editor/createEditor.ts'), 'utf8')
const cssSource = readFileSync(resolve(__dirname, '../src/style.css'), 'utf8')

assert(cssSource.includes('.entry-binding-menu') && cssSource.includes('overflow: auto'), 'entry binding menu must remain scrollable')
assert(editorSource.includes("entryBindingMenu.addEventListener('wheel'"), 'entry binding menu must handle wheel events')
assert(editorSource.includes('event.stopPropagation()'), 'entry binding menu wheel handler must stop propagation to the canvas')

console.log('entryBindingMenuEvents tests passed')
