import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const __dirname = dirname(fileURLToPath(import.meta.url))
const socketTheme = readFileSync(resolve(__dirname, '../src/editor/socketTheme.ts'), 'utf8')
const booleanTheme = socketTheme.match(/boolean:\s*\{([^}]*)\}/)?.[1] ?? ''
const color = booleanTheme.match(/color:\s*'([^']+)'/)?.[1]
const fill = booleanTheme.match(/fill:\s*'([^']+)'/)?.[1] ?? color

assert(Boolean(color), 'boolean sockets must define a visible red color')
assert(fill === color, 'connected boolean sockets must use their red socket color as the solid fill')

console.log('socketVisuals tests passed')
