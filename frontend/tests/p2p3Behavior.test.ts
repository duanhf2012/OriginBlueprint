import { describe, expect, it } from 'vitest'
import { sourceRequiresProtection } from '../src/documentSafety'
import { autoSaveIntervalMs, isAutoSaveEligible } from '../src/autoSavePolicy'
import { pushBoundedHistory } from '../src/editor/history'
import { saveGateDecision } from '../src/saveGate'
import { variableScope } from '../src/editor/document'
import { createVariableNode } from '../src/editor/nodeRegistry'
import { moveVariablesToDefaultGroup, normalizeVariableGroups, variableGroupNameExists, variableGroupRemovalMessage, variableGroupUsage } from '../src/editor/variableGroups'

describe('variable scopes', () => {
  it('defaults omitted scope to execution for existing .obp files', () => {
    expect(variableScope({})).toBe('execution')
  })

  it('marks instance variable nodes explicitly', () => {
    const node = createVariableNode({ id: 'shared', name: 'Shared', type: 'integer', defaultValue: 0, groupId: 'default', scope: 'instance' }, 'get')
    expect(node.variableScope).toBe('instance')
    expect(node.subtitle).toContain('全局')
  })

  it('reports local and global group usage separately before deletion', () => {
    const usage = variableGroupUsage([
      { id: 'local', name: 'Local', type: 'integer', defaultValue: 0, groupId: 'shared' },
      { id: 'global', name: 'Global', type: 'integer', defaultValue: 0, groupId: 'shared', scope: 'instance' },
      { id: 'other', name: 'Other', type: 'integer', defaultValue: 0, groupId: 'default' },
    ], 'shared')
    expect(usage).toEqual({ localCount: 1, globalCount: 1, totalCount: 2 })
    expect(variableGroupRemovalMessage('Shared', usage)).toContain('局部变量 1 个、全局变量 1 个')
  })

  it('preserves case-conflicting native groups for validation instead of silently dropping one', () => {
    const groups = normalizeVariableGroups([
      { id: 'combat-a', name: 'Combat' },
      { id: 'combat-b', name: 'combat' },
    ], [], () => 'generated')
    expect(groups.map(group => group.id)).toEqual(['default', 'combat-a', 'combat-b'])
  })

  it('checks group names case-insensitively and moves both scopes on confirmed deletion', () => {
    expect(variableGroupNameExists([{ id: 'combat', name: 'Combat' }], ' combat ')).toBe(true)
    const variables = [
      { id: 'local', name: 'Local', type: 'integer' as const, defaultValue: 0, groupId: 'shared' },
      { id: 'global', name: 'Global', type: 'integer' as const, defaultValue: 0, groupId: 'shared', scope: 'instance' as const },
    ]
    moveVariablesToDefaultGroup(variables, 'shared')
    expect(variables.map(variable => variable.groupId)).toEqual(['default', 'default'])
  })
})

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
