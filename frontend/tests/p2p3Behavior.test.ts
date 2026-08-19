import { describe, expect, it } from 'vitest'
import { sourceRequiresProtection } from '../src/documentSafety'
import { autoSaveIntervalMs, isAutoSaveEligible } from '../src/autoSavePolicy'
import { pushBoundedHistory } from '../src/editor/history'
import { saveGateDecision } from '../src/saveGate'
import { variableScope } from '../src/editor/document'
import { createVariableNode } from '../src/editor/nodeRegistry'
import { applyVariableGroupDrop, matchingVariableGroupId, moveVariablesToDefaultGroup, normalizeVariableGroups, planVariableGroupDrop, variableGroupNameExists, variableGroupRemovalMessage, variableGroupUsage, variableGroupsForScope } from '../src/editor/variableGroups'
import { isValidIntegerDefault } from '../src/editor/valueValidation'

describe('integer default validation', () => {
  it('accepts safe integers and rejects fractional or unsafe values', () => {
    expect(isValidIntegerDefault(0)).toBe(true)
    expect(isValidIntegerDefault(-42)).toBe(true)
    expect(isValidIntegerDefault(Number.MAX_SAFE_INTEGER)).toBe(true)
    expect(isValidIntegerDefault(1.5)).toBe(false)
    expect(isValidIntegerDefault(Number.MAX_SAFE_INTEGER + 1)).toBe(false)
    expect(isValidIntegerDefault('42')).toBe(true)
    expect(isValidIntegerDefault('9223372036854775807')).toBe(true)
    expect(isValidIntegerDefault('9223372036854775808')).toBe(false)
  })
})

describe('variable scopes', () => {
  it('defaults omitted scope to execution for existing .obp files', () => {
    expect(variableScope({})).toBe('execution')
  })

  it('marks instance variable nodes explicitly', () => {
    const node = createVariableNode({ id: 'shared', name: 'Shared', type: 'integer', defaultValue: 0, groupId: 'default', scope: 'instance' }, 'get')
    expect(node.variableScope).toBe('instance')
    expect(node.subtitle).toContain('全局')
  })

  it('reports usage separately and describes deletion within one scope', () => {
    const usage = variableGroupUsage([
      { id: 'local', name: 'Local', type: 'integer', defaultValue: 0, groupId: 'shared' },
      { id: 'global', name: 'Global', type: 'integer', defaultValue: 0, groupId: 'shared', scope: 'instance' },
      { id: 'other', name: 'Other', type: 'integer', defaultValue: 0, groupId: 'default' },
    ], 'shared')
    expect(usage).toEqual({ localCount: 1, globalCount: 1, totalCount: 2 })
    expect(variableGroupRemovalMessage('Shared', 'execution', usage.localCount)).toContain('1 个局部变量')
    expect(variableGroupRemovalMessage('Shared', 'instance', usage.globalCount)).toContain('1 个全局变量')
  })

  it('keeps case-conflicting legacy groups for core validation instead of silently dropping one', () => {
    const normalized = normalizeVariableGroups([
      { id: 'combat-a', name: 'Combat' },
      { id: 'combat-b', name: 'combat' },
    ], [], () => 'generated')
    expect(normalized.groups.map(group => group.id)).toEqual(['default', 'combat-a', 'combat-b'])
    expect(normalized.groups.slice(1).map(group => group.scope)).toEqual(['execution', 'execution'])
  })

  it('checks names case-insensitively only within the selected scope', () => {
    const groups = [
      { id: 'local-combat', name: 'Combat', scope: 'execution' as const },
      { id: 'global-shared', name: 'Shared', scope: 'instance' as const },
    ]
    expect(variableGroupNameExists(groups, ' combat ', 'execution')).toBe(true)
    expect(variableGroupNameExists(groups, ' combat ', 'instance')).toBe(false)
    expect(variableGroupNameExists(groups, ' shared ', 'instance')).toBe(true)
    expect(variableGroupNames(variableGroupsForScope(groups, 'execution'))).toEqual(['Combat'])
    expect(variableGroupNames(variableGroupsForScope(groups, 'instance'))).toEqual(['Shared'])
  })

  it('moves variables to Default when their scoped group is deleted', () => {
    const variables = [
      { id: 'local', name: 'Local', type: 'integer' as const, defaultValue: 0, groupId: 'shared' },
      { id: 'global', name: 'Global', type: 'integer' as const, defaultValue: 0, groupId: 'shared', scope: 'instance' as const },
    ]
    moveVariablesToDefaultGroup(variables, 'shared')
    expect(variables.map(variable => variable.groupId)).toEqual(['default', 'default'])
  })

  it('infers one scope for old groups used by only local or only global variables', () => {
    const local = normalizeVariableGroups([{ id: 'combat', name: 'Combat' }], [
      { groupId: 'combat' },
    ], () => 'generated-local')
    const global = normalizeVariableGroups([{ id: 'shared', name: 'Shared' }], [
      { groupId: 'shared', scope: 'instance' },
    ], () => 'generated-global')
    const empty = normalizeVariableGroups([{ id: 'empty', name: 'Empty' }], [], () => 'generated-empty')

    expect(local.groups[1].scope).toBe('execution')
    expect(global.groups[1].scope).toBe('instance')
    expect(empty.groups[1].scope).toBe('execution')
    expect(local.resolveGroupId('combat', '', 'execution')).toBe('combat')
    expect(global.resolveGroupId('shared', '', 'instance')).toBe('shared')
  })

  it('splits a mixed old group into same-named local and global groups', () => {
    const normalized = normalizeVariableGroups([{ id: 'shared', name: 'Shared' }], [
      { groupId: 'shared' },
      { groupId: 'shared', scope: 'instance' },
    ], () => 'shared-global')

    expect(normalized.groups).toEqual([
      { id: 'default', name: 'Default' },
      { id: 'shared', name: 'Shared', collapsed: false, scope: 'execution' },
      { id: 'shared-global', name: 'Shared', collapsed: false, scope: 'instance' },
    ])
    expect(normalized.resolveGroupId('shared', '', 'execution')).toBe('shared')
    expect(normalized.resolveGroupId('shared', '', 'instance')).toBe('shared-global')
  })

  it('migrates legacy named groups to local even when a global group has the same name', () => {
    const normalized = normalizeVariableGroups([
      { id: 'global-combat', name: 'Combat', scope: 'instance' },
    ], [{ group: 'Combat' }], () => 'legacy-combat')

    expect(normalized.resolveGroupId('', 'Combat', 'execution')).toBe('legacy-combat')
    expect(normalized.resolveGroupId('', 'Combat', 'instance')).toBe('global-combat')
  })

  it('maps scope changes to a same-named target group or Default', () => {
    const groups = [
      { id: 'local-combat', name: 'Combat', scope: 'execution' as const },
      { id: 'global-combat', name: 'combat', scope: 'instance' as const },
      { id: 'local-only', name: 'Local Only', scope: 'execution' as const },
    ]
    expect(matchingVariableGroupId(groups, 'local-combat', 'instance')).toBe('global-combat')
    expect(matchingVariableGroupId(groups, 'local-only', 'instance')).toBe('default')
  })

  it('moves variables between groups without changing their scope or identity', () => {
    const groups = [
      { id: 'local-a', name: 'A', scope: 'execution' as const },
      { id: 'local-b', name: 'B', scope: 'execution' as const },
    ]
    const variable = { id: 'stable-id', name: 'Value', type: 'integer' as const, defaultValue: 0, groupId: 'local-a' }
    const variables = [
      { id: 'before', name: 'Before', type: 'integer' as const, defaultValue: 0, groupId: 'local-a' },
      variable,
      { id: 'after', name: 'After', type: 'integer' as const, defaultValue: 0, groupId: 'local-b' },
    ]
    const plan = planVariableGroupDrop(groups, variable, 'local-b', 'execution', false)
    expect(plan.kind).toBe('move')
    expect(applyVariableGroupDrop(variable, plan)).toBe(true)
    expect(variable).toMatchObject({ id: 'stable-id', groupId: 'local-b' })
    expect(variable).not.toHaveProperty('scope')
    expect(variables.map(item => item.id)).toEqual(['before', 'stable-id', 'after'])
  })

  it('requires a scope-change plan for cross-scope drops and applies target scope atomically', () => {
    const groups = [
      { id: 'local', name: 'State', scope: 'execution' as const },
      { id: 'global', name: 'State', scope: 'instance' as const },
    ]
    const variable = { id: 'stable-id', name: 'Value', type: 'integer' as const, defaultValue: 0, groupId: 'local' }
    const toGlobal = planVariableGroupDrop(groups, variable, 'global', 'instance', false)
    expect(toGlobal.kind).toBe('scope-change')
    applyVariableGroupDrop(variable, toGlobal)
    expect(variable).toMatchObject({ id: 'stable-id', groupId: 'global', scope: 'instance' })

    const toLocal = planVariableGroupDrop(groups, variable, 'default', 'execution', false)
    expect(toLocal.kind).toBe('scope-change')
    applyVariableGroupDrop(variable, toLocal)
    expect(variable).toMatchObject({ id: 'stable-id', groupId: 'default' })
    expect(variable).not.toHaveProperty('scope')
  })

  it('rejects function-global and mismatched group drop targets without mutation', () => {
    const groups = [{ id: 'local', name: 'Local', scope: 'execution' as const }]
    const variable = { id: 'stable-id', name: 'Value', type: 'integer' as const, defaultValue: 0, groupId: 'local' }
    const functionPlan = planVariableGroupDrop(groups, variable, 'default', 'instance', true)
    expect(functionPlan).toMatchObject({ kind: 'forbidden', reason: 'function-instance-scope' })
    expect(applyVariableGroupDrop(variable, functionPlan)).toBe(false)

    const invalidPlan = planVariableGroupDrop(groups, variable, 'local', 'instance', false)
    expect(invalidPlan).toMatchObject({ kind: 'forbidden', reason: 'invalid-target-group' })
    expect(variable).toMatchObject({ id: 'stable-id', groupId: 'local' })
    expect(variable).not.toHaveProperty('scope')
  })
})

function variableGroupNames(groups: Array<{ name: string }>) {
  return groups.map(group => group.name)
}

describe('raw source validation protection', () => {
  it('protects a source when validation found an error', () => {
    expect(sourceRequiresProtection([
      { severity: 'warning' },
      { severity: 'error', blocksSave: true },
    ])).toBe(true)
  })

  it('does not protect a source for warnings alone', () => {
    expect(sourceRequiresProtection([{ severity: 'warning' }])).toBe(false)
  })

  it('does not protect a source from runtime-target or nonblocking core errors', () => {
    expect(sourceRequiresProtection([{ severity: 'error', target: 'target.go', blocksRun: true }])).toBe(false)
    expect(sourceRequiresProtection([{ severity: 'error', code: 'flow.unreachable-node' }])).toBe(false)
  })
})

describe('autosave policy', () => {
  it('maps every supported setting to an exact interval', () => {
    expect(autoSaveIntervalMs('off')).toBe(0)
    expect(autoSaveIntervalMs('1m')).toBe(60_000)
    expect(autoSaveIntervalMs('3m')).toBe(180_000)
    expect(autoSaveIntervalMs('5m')).toBe(300_000)
  })

  it('only accepts dirty, named, compatibility-safe and idle tabs', () => {
    const safe = { dirty: true, path: 'graph.obp', restoreFatal: false, hasRestoreLoss: false, legacyRequiresNative: false, saving: false }
    expect(isAutoSaveEligible(safe)).toBe(true)
    expect(isAutoSaveEligible({ ...safe, dirty: false })).toBe(false)
    expect(isAutoSaveEligible({ ...safe, path: '' })).toBe(false)
    expect(isAutoSaveEligible({ ...safe, restoreFatal: true })).toBe(false)
    expect(isAutoSaveEligible({ ...safe, hasRestoreLoss: true })).toBe(false)
    expect(isAutoSaveEligible({ ...safe, legacyRequiresNative: true })).toBe(false)
    expect(isAutoSaveEligible({ ...safe, saving: true })).toBe(false)
  })
})

describe('editor history policy', () => {
  it('keeps only the newest 100 snapshots', () => {
    const history: number[] = []
    for (let index = 0; index < 125; index++) pushBoundedHistory(history, index)
    expect(history).toHaveLength(100)
    expect(history[0]).toBe(25)
    expect(history[99]).toBe(124)
  })

  it('supports a smaller explicit cap for focused tests', () => {
    const history: string[] = []
    pushBoundedHistory(history, 'a', 2)
    pushBoundedHistory(history, 'b', 2)
    pushBoundedHistory(history, 'c', 2)
    expect(history).toEqual(['b', 'c'])
  })
})

describe('core graph save gate', () => {
  it('blocks core blockers but never target-only errors', () => {
    expect(saveGateDecision([{ severity: 'error', code: 'flow.exec-cycle', message: 'cycle', blocksSave: true }], false).blocked).toBe(true)
    expect(saveGateDecision([{ severity: 'error', code: 'engine.compile', message: 'unsupported', target: 'target.go', blocksRun: true }], false).blocked).toBe(false)
    expect(saveGateDecision([{ severity: 'error', code: 'flow.unreachable-node', message: 'unreachable' }], false).blocked).toBe(false)
    expect(saveGateDecision([{ severity: 'error', code: 'flow.unreachable-node', message: 'unreachable' }], true).blocked).toBe(true)
    expect(saveGateDecision([{ severity: 'warning', code: 'flow.possible-cycle', message: 'possible' }], true).blocked).toBe(false)
  })
})
