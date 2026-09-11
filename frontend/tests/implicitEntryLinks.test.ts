import {
  describeEntryBinding,
  entryBindingBadgeLabel,
  entryBindingCandidateGroups,
  entryBindingLabel,
  entryBindingMenuPosition,
  entryBindingTitle,
  entrySourceColorAssignments,
  entrySourcePalette,
  isEntryNode,
  isEntryOutputConnection,
  socketsCompatible,
  type EntryBindingConnection,
  type EntryBindingNode
} from '../src/editor/implicitEntryLinks'
import { describe, it } from 'vitest'

function assert(value: unknown, message: string) {
  if (!value) throw new Error(message)
}

const entry: EntryBindingNode = {
  id: 'entry',
  typeId: 'origin.event.entry-two-integers',
  label: 'Skill Entry',
  outputs: {
    exec: { label: '', socket: 'exec' },
    objectId: { label: 'ObjectId', socket: 'integer' },
    param1: { label: 'Param1', socket: 'integer' }
  }
}

const target: EntryBindingNode = {
  id: 'target',
  typeId: 'origin.action.use-target',
  label: 'Use Target',
  inputs: {
    targetId: { label: 'TargetId', socket: 'integer' },
    name: { label: 'Name', socket: 'string' }
  },
  outputs: {
    exec: { label: '', socket: 'exec' }
  }
}

const legacyEntry: EntryBindingNode = {
  id: 'legacy-entry',
  typeId: 'origin.legacy.entrance-monster-choice-skill',
  legacyClass: 'Entrance_MonsterChoiceSkill_40300',
  label: 'Monster skill entry',
  outputs: {
    exec: { label: '', socket: 'exec' },
    monsterObjectId: { label: 'MonsterObjectId', socket: 'integer' },
    anyValue: { label: 'AnyValue', socket: 'any' }
  }
}

const customEntry: EntryBindingNode = {
  ...legacyEntry,
  id: 'custom-entry',
  typeId: 'origin.custom.entrance-monster-choice-skill-40300',
  legacyClass: undefined,
  label: 'Monster attack entry(extra)'
}

const functionEntry: EntryBindingNode = {
  id: 'function-entry',
  typeId: 'origin.function.entry',
  label: 'Calculate Entry',
  outputs: {
    exec: { label: '', socket: 'exec' },
    input_value: { label: 'Value', socket: 'integer' },
    input_name: { label: 'Name', socket: 'string' }
  }
}

const functionReturn: EntryBindingNode = {
  id: 'function-return',
  typeId: 'origin.function.return',
  label: 'Calculate Return',
  inputs: {
    exec: { label: '', socket: 'exec' },
    output_result: { label: 'Result', socket: 'integer' }
  }
}

const activityEntry: EntryBindingNode = {
  id: 'activity-entry',
  kind: 'event',
  typeId: 'origin.custom.entrance-activity-200000',
  label: 'Activity event',
  outputs: {
    exec: { label: '', socket: 'exec' },
    mapInstanceId: { label: 'Map instance ID', socket: 'integer' }
  }
}

const playerEnterMapEntry: EntryBindingNode = {
  id: 'player-enter-map-entry',
  kind: 'event',
  typeId: 'origin.custom.entrance-player-enter-map-200001',
  label: 'Player enters map',
  outputs: {
    exec: { label: '', socket: 'exec' },
    mapInstanceId: { label: 'Map instance ID', socket: 'integer' }
  }
}

const groupCountChangedEntry: EntryBindingNode = {
  id: 'group-count-changed-entry',
  kind: 'event',
  typeId: 'origin.custom.on-group-key-count-changed-200002',
  legacyClass: 'OnGroupKeyCountChanged_200002',
  label: 'Group count changed',
  outputs: {
    exec: { label: '', socket: 'exec' },
    mapInstanceId: { label: 'Map instance ID', socket: 'integer' }
  }
}

const connection: EntryBindingConnection = {
  source: 'entry',
  sourceOutput: 'objectId',
  target: 'target',
  targetInput: 'targetId'
}

describe('implicit entry links', () => {
it('detects, groups and describes compatible entry bindings', () => {
assert(isEntryOutputConnection(connection, id => id === 'entry' ? entry : target), 'detects data connections sourced from an entry node')
assert(isEntryNode(legacyEntry), 'detects legacy Entrance_* nodes as entry nodes')
assert(isEntryNode(customEntry), 'detects schema-generated origin.custom.entrance-* nodes as entry nodes')
assert(isEntryNode(functionEntry), 'detects function Entry nodes as entry nodes')
assert(!isEntryNode(functionReturn), 'does not mistake function Return nodes for entry nodes')
assert(isEntryOutputConnection({ ...connection, source: 'legacy-entry', sourceOutput: 'monsterObjectId' }, id => id === 'legacy-entry' ? legacyEntry : target), 'detects data connections sourced from legacy entry nodes')
assert(isEntryOutputConnection({ ...connection, source: 'custom-entry', sourceOutput: 'monsterObjectId' }, id => id === 'custom-entry' ? customEntry : target), 'detects data connections sourced from schema-generated entry nodes')
assert(socketsCompatible('any', 'integer'), 'allows wildcard source sockets for entry bindings')
assert(isEntryOutputConnection({ ...connection, source: 'legacy-entry', sourceOutput: 'anyValue' }, id => id === 'legacy-entry' ? legacyEntry : target), 'allows wildcard entry outputs to bind typed inputs')
assert(!isEntryOutputConnection({ ...connection, sourceOutput: 'exec' }, id => id === 'entry' ? entry : target), 'does not hide exec connections from entry nodes')
assert(!isEntryOutputConnection({ ...connection, targetInput: 'name' }, id => id === 'entry' ? entry : target), 'does not hide mismatched socket connections')

const candidateGroups = entryBindingCandidateGroups('target', 'targetId', [target, entry, legacyEntry, customEntry])
assert(candidateGroups.length === 3, 'groups compatible entry outputs by source entry node')
assert(candidateGroups[0].sourceNodeId === 'entry', 'keeps source entry order in candidate groups')
assert(candidateGroups[0].candidates.map(item => item.sourceOutput).join(',') === 'objectId,param1', 'filters exec outputs out of candidate groups')
assert(candidateGroups[1].candidates.some(item => item.sourceOutput === 'anyValue'), 'keeps wildcard outputs in candidate groups')
assert(entryBindingCandidateGroups('target', 'name', [target, entry]).length === 0, 'filters out groups without compatible outputs')

const functionCandidateGroups = entryBindingCandidateGroups('target', 'targetId', [target, functionEntry, functionReturn])
assert(functionCandidateGroups.length === 1, 'offers function Entry parameters in the shared binding menu')
assert(functionCandidateGroups[0].sourceNodeId === 'function-entry', 'uses the function Entry as the binding source')
assert(functionCandidateGroups[0].candidates.map(item => item.sourceOutput).join(',') === 'input_value', 'filters function Entry parameters by socket type and excludes exec')

const threeEntryGroups = entryBindingCandidateGroups('target', 'targetId', [
  target,
  activityEntry,
  playerEnterMapEntry,
  groupCountChangedEntry
])
assert(threeEntryGroups.length === 3, 'offers matching parameters from all three event entry nodes')
assert(threeEntryGroups.map(group => group.sourceNodeId).join(',') === 'activity-entry,player-enter-map-entry,group-count-changed-entry', 'keeps all event entry groups in canvas order')

const functionBinding = describeEntryBinding({
  source: 'function-entry',
  sourceOutput: 'input_value',
  target: 'target',
  targetInput: 'targetId'
}, id => id === 'function-entry' ? functionEntry : target)
assert(functionBinding?.sourceOutputLabel === 'Value', 'describes a function Entry binding for the collapsed input badge')
assert(isEntryOutputConnection({
  source: 'function-entry',
  sourceOutput: 'input_value',
  target: 'target',
  targetInput: 'targetId'
}, id => id === 'function-entry' ? functionEntry : target), 'collapses function Entry parameter links like ordinary entry bindings')

const binding = describeEntryBinding(connection, id => id === 'entry' ? entry : target)
assert(binding?.sourceNodeId === 'entry', 'describes the source entry node')
assert(binding?.sourceOutput === 'objectId', 'describes the source output key')
assert(binding?.targetNodeId === 'target', 'describes the target node')
assert(binding?.targetInput === 'targetId', 'describes the target input key')
assert(binding?.label === 'ObjectId', 'uses only the field name for the visible input badge')
assert(entryBindingLabel(binding) === 'ObjectId', 'formats a compact field-only badge label')
assert(entryBindingBadgeLabel(binding) === 'ObjectId', 'formats the visible field-only badge text')
assert(entryBindingTitle(binding) === 'Skill Entry/ObjectId', 'formats the tooltip without an entry prefix')
})

it('keeps the full binding menu inside the canvas near its bottom edge', () => {
  const position = entryBindingMenuPosition(780, 790, { left: 0, top: 0, width: 912, height: 812 }, 286, 340)
  assert(position.left === 620, 'clamps the menu to the canvas right edge using its measured width')
  assert(position.top === 466, 'clamps the menu to the canvas bottom edge using its measured height')
  assert(position.left + 286 <= 906 && position.top + 340 <= 806, 'leaves the requested margin around the whole menu')
})

it('assigns a fixed 50-color palette by stable entry node ID order', () => {
  assert(entrySourcePalette.length === 50, 'provides exactly 50 built-in entry colors')
  assert(new Set(entrySourcePalette).size === 50, 'does not repeat colors within the built-in palette')

  const assignments = entrySourceColorAssignments([
    { ...activityEntry, id: 'entry-c' },
    target,
    { ...playerEnterMapEntry, id: 'entry-a' },
    { ...groupCountChangedEntry, id: 'entry-b' }
  ])
  assert(assignments.size === 3, 'assigns colors only to entry nodes')
  assert(assignments.get('entry-a') === entrySourcePalette[0], 'assigns the first palette color to the lowest entry ID')
  assert(assignments.get('entry-b') === entrySourcePalette[1], 'assigns the second palette color to the next entry ID')
  assert(assignments.get('entry-c') === entrySourcePalette[2], 'assigns colors independently of canvas enumeration order')
})
})
