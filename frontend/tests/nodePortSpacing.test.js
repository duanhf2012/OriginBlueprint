import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

function assert(value, message) {
  if (!value) throw new Error(message)
}

function ruleBody(source, selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`(?:^|\\n)${escaped}\\s*\\{([^}]*)\\}`))
  return match?.[1] ?? ''
}

function pxValue(body, property) {
  const match = body.match(new RegExp(`${property}\\s*:\\s*(\\d+)px`))
  return match ? Number(match[1]) : 0
}

function propertyValue(body, property) {
  const match = body.match(new RegExp(`${property}\\s*:\\s*([^;]+)`))
  return match?.[1].trim() ?? ''
}

const __dirname = dirname(fileURLToPath(import.meta.url))
const component = readFileSync(resolve(__dirname, '../src/editor/BlueprintNode.vue'), 'utf8')

const portRow = ruleBody(component, '.port-row')
const entryBindingPortRow = ruleBody(component, '.blueprint-node.has-entry-binding .port-row')
const entryBindingNode = ruleBody(component, '.blueprint-node.has-entry-binding')
const controlRef = ruleBody(component, '.control-ref')
const inputPort = ruleBody(component, '.input-port')
const entryBindingBadge = ruleBody(component, '.entry-binding-badge')

assert(propertyValue(portRow, 'column-gap') === '0', 'port rows should let the middle spacer own the dynamic gap')
assert(propertyValue(entryBindingPortRow, 'column-gap') === '0', 'entry binding rows should let the middle spacer own the dynamic gap')
assert(pxValue(controlRef, 'margin-right') >= 18, 'input controls need at least 18px clearance before output ports')
assert(propertyValue(inputPort, 'overflow') !== 'hidden', 'input ports must not hide input labels')
assert(propertyValue(entryBindingBadge, 'min-width') === '0', 'entry binding badges must be allowed to shrink')
assert(propertyValue(entryBindingBadge, 'flex') === '0 1 auto', 'entry binding badges should size to their content')
assert(component.includes('const nodeWidth = computed'), 'node width must be content-aware')
assert(component.includes('function estimateTextWidth'), 'node width must estimate visible text widths')
assert(component.includes('function inputContentWidth'), 'node width must include input labels and controls')
assert(component.includes('function outputContentWidth'), 'node width must include output labels')
assert(component.includes('DEFAULT_CONTROL_WIDTH = 62'), 'default input controls should be estimated near their rendered width')
assert(component.includes('DEFAULT_LABEL_MIN_WIDTH = 28'), 'short port labels should not force oversized nodes')
assert(component.includes('ENTRY_BADGE_MIN_WIDTH = 56'), 'short entry binding badges should not force oversized nodes')
assert(component.includes('BRANCH_ACTION_WIDTH = 96'), 'dynamic branch rows need enough width for + Item controls')
assert(component.includes('width: `${nodeWidth.value}px`') && component.includes(':style="nodeStyle"'), 'node template must use computed content-aware width')
assert(component.includes('inputEntryBindingTitle'), 'entry binding badge title must be different from badge text')
assert(component.includes('entryBindingBadgeLabel'), 'entry binding badge text must prefer the field name')
assert(!component.includes('max-width: 150px'), 'input labels should not be capped at 150px')
assert(propertyValue(portRow, 'grid-template-columns') === 'max-content minmax(6px, 1fr) max-content', 'port rows should keep outputs right-aligned with a compact flexible middle gap')
assert(propertyValue(entryBindingPortRow, 'grid-template-columns') === 'max-content minmax(6px, 1fr) max-content', 'entry binding rows should keep outputs right-aligned with a compact flexible middle gap')
assert(!propertyValue(portRow, 'grid-template-columns').startsWith('minmax(0, 1fr)'), 'port rows should not split wide nodes into equal input/output columns')
assert(!propertyValue(entryBindingPortRow, 'grid-template-columns').includes('1.45fr'), 'entry binding rows should not reserve oversized middle whitespace')
assert(!propertyValue(entryBindingNode, 'min-width'), 'entry binding nodes must rely on content-aware width instead of a fixed oversized minimum')
assert(component.includes('class="port-spacer middle-spacer"'), 'port rows need a middle spacer column')
assert(component.includes('PORT_COLUMN_GAP = 6'), 'node width estimation should use the compact middle gap')

console.log('nodePortSpacing tests passed')
