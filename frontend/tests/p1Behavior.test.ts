import { describe, expect, it } from 'vitest'
import type { RestoreLossReport } from '../src/editor/document'
import {
  compatibilitySaveOptions,
  findOpenTab,
  graphPathKey,
  hasRestoreLoss,
  resolveCompatibilitySaveAction,
} from '../src/documentSafety'
import type { GraphDocument, GraphSnapshot, NodeSnapshot, RestoreAlteredNode } from '../src/editor/document'
import { buildRestorePlan, normalizeDynamicOutputCount, type PreparedRestoreNode } from '../src/editor/restorePlan'

const emptyReport = (): RestoreLossReport => ({ droppedNodes: [], droppedConnections: [], alteredNodes: [] })

describe('document open safety', () => {
  it('finds the existing Windows tab without mutating dirty state or document identity', () => {
    const originalDocument = { graphName: 'Unsaved edits' }
    const dirtyTab = { path: 'E:\\Graphs\\Test.obp', dirty: true, document: originalDocument }

    const found = findOpenTab([dirtyTab], 'e:/graphs/test.obp', true)

    expect(found).toBe(dirtyTab)
    expect(dirtyTab.document).toBe(originalDocument)
    expect(dirtyTab.dirty).toBe(true)
  })

  it('keeps case-sensitive browser paths distinct', () => {
    expect(graphPathKey('Graphs/Test.obp', false)).not.toBe(graphPathKey('graphs/test.obp', false))
  })
})

describe('compatibility save policy', () => {
  it('recognizes dropped and altered content as restore loss', () => {
    const dropped = emptyReport()
    dropped.droppedNodes.push({ id: 'unknown', typeId: 'custom.node', reason: 'unknown-node-type' })
    const altered = emptyReport()
    altered.alteredNodes.push({ id: 'sequence', typeId: 'origin.flow.sequence', reason: 'invalid-dynamic-output-count', originalValue: 300, restoredValue: 256 })

    expect(hasRestoreLoss(emptyReport())).toBe(false)
    expect(hasRestoreLoss(dropped)).toBe(true)
    expect(hasRestoreLoss(altered)).toBe(true)
  })

  it('offers copy, force, and cancel only for a non-fatal force-compatible loss', () => {
    expect(compatibilitySaveOptions({ fatal: false, hasLoss: true, formatAllowsForce: true })).toEqual(['copy', 'force', 'cancel'])
    expect(compatibilitySaveOptions({ fatal: false, hasLoss: true, formatAllowsForce: false })).toEqual(['copy', 'cancel'])
    expect(compatibilitySaveOptions({ fatal: true, hasLoss: true, formatAllowsForce: true })).toEqual(['copy', 'cancel'])
  })

  it('maps user intent to a persistence action and rejects unsafe force requests', () => {
    const safeForce = { fatal: false, hasLoss: true, formatAllowsForce: true }
    expect(resolveCompatibilitySaveAction('copy', safeForce)).toBe('recovery-copy')
    expect(resolveCompatibilitySaveAction('cancel', safeForce)).toBe('cancel')
    expect(resolveCompatibilitySaveAction('force', safeForce)).toBe('force-source-with-backup')
    expect(resolveCompatibilitySaveAction('force', { ...safeForce, fatal: true })).toBe('cancel')
    expect(resolveCompatibilitySaveAction('force', { ...safeForce, formatAllowsForce: false })).toBe('cancel')
  })
})

type FakeNode = { id: string }

function node(id: string, typeId: string, dynamicOutputCount?: number): NodeSnapshot {
  return {
    id,
    typeId,
    position: { x: 0, y: 0 },
    values: {},
    properties: dynamicOutputCount === undefined ? undefined : { dynamicOutputCount },
  }
}

function fakePreparer(knownTypes: readonly string[]) {
  return (snapshot: NodeSnapshot, typeId: string): PreparedRestoreNode<FakeNode> | null => {
    if (!knownTypes.includes(typeId)) return null
    const alteredNodes: RestoreAlteredNode[] = []
    const count = typeId === 'origin.flow.sequence'
      ? normalizeDynamicOutputCount(snapshot.properties?.dynamicOutputCount ?? 3)
      : 0
    const requested = snapshot.properties?.dynamicOutputCount
    if (typeId === 'origin.flow.sequence' && requested !== undefined && requested !== 0 && requested !== count) {
      alteredNodes.push({ id: snapshot.id, typeId, reason: 'invalid-dynamic-output-count', originalValue: requested, restoredValue: count })
    }
    return {
      snapshot,
      node: { id: snapshot.id },
      inputKeys: typeId === 'origin.flow.sequence' ? ['exec'] : ['in'],
      outputKeys: typeId === 'origin.flow.sequence' ? Array.from({ length: count }, (_, index) => `then${index}`) : ['out'],
      alteredNodes,
    }
  }
}

describe('restore planning', () => {
  it('reports unknown nodes and connections that depend on missing endpoints', () => {
    const snapshot: GraphSnapshot = {
      nodes: [node('known', 'known.node'), node('unknown', 'custom.node')],
      connections: [{ source: 'unknown', sourceOutput: 'out', target: 'known', targetInput: 'in' }],
      groups: [],
    }

    const plan = buildRestorePlan(snapshot, fakePreparer(['known.node']))

    expect(plan.nodes.map(item => item.snapshot.id)).toEqual(['known'])
    expect(plan.report.droppedNodes).toEqual([{ id: 'unknown', typeId: 'custom.node', reason: 'unknown-node-type' }])
    expect(plan.report.droppedConnections[0].reason).toBe('missing-endpoint')
  })

  it('reports missing source and target ports separately', () => {
    const snapshot: GraphSnapshot = {
      nodes: [node('source', 'known.node'), node('target', 'known.node')],
      connections: [
        { source: 'source', sourceOutput: 'missing', target: 'target', targetInput: 'in' },
        { source: 'source', sourceOutput: 'out', target: 'target', targetInput: 'missing' },
      ],
      groups: [],
    }

    const plan = buildRestorePlan(snapshot, fakePreparer(['known.node']))

    expect(plan.report.droppedConnections.map(item => item.reason)).toEqual(['missing-source-port', 'missing-target-port'])
    expect(plan.connections).toEqual([])
  })

  it('does not treat opaque legacy state as visual restore loss', () => {
    const document: GraphDocument = {
      schemaVersion: 1,
      graphName: 'Legacy',
      nodes: [],
      connections: [],
      groups: [],
      variables: [],
      variableGroups: [],
      view: { x: 0, y: 0, zoom: 1 },
      legacy: { format: 'vgf', hiddenNodes: [{ id: 'hidden', class: 'Unknown', module: 'legacy', pos: [0, 0], port_defaultv: {} }] },
    }

    expect(buildRestorePlan(document, fakePreparer([])).report).toEqual(emptyReport())
  })

  it('normalizes unsafe sequence counts while preserving the 0 compatibility default', () => {
    expect(normalizeDynamicOutputCount(0)).toBe(3)
    expect(normalizeDynamicOutputCount(-1)).toBe(1)
    expect(normalizeDynamicOutputCount(Number.NaN)).toBe(3)
    expect(normalizeDynamicOutputCount(257)).toBe(256)
    expect(normalizeDynamicOutputCount(256)).toBe(256)
  })

  it('preserves valid then12 and then255 connections in the restore plan', () => {
    const snapshot: GraphSnapshot = {
      nodes: [node('sequence', 'origin.flow.sequence', 256), node('target-a', 'known.node'), node('target-b', 'known.node')],
      connections: [
        { source: 'sequence', sourceOutput: 'then12', target: 'target-a', targetInput: 'in' },
        { source: 'sequence', sourceOutput: 'then255', target: 'target-b', targetInput: 'in' },
      ],
      groups: [],
    }

    const plan = buildRestorePlan(snapshot, fakePreparer(['origin.flow.sequence', 'known.node']))

    expect(plan.report).toEqual(emptyReport())
    expect(plan.connections).toEqual(snapshot.connections)
  })
})
