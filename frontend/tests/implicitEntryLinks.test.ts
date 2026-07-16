import {
  describeEntryBinding,
  entryBindingBadgeLabel,
  entryBindingCandidateGroups,
  entryBindingLabel,
  entryBindingTitle,
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
})
