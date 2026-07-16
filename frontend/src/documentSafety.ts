import type { RestoreLossReport } from './editor/document'

export type CompatibilitySaveAction = 'copy' | 'force' | 'cancel'
export type CompatibilityPersistenceAction = 'cancel' | 'recovery-copy' | 'force-source-with-backup'

export interface CompatibilitySavePolicyInput {
  fatal: boolean
  hasLoss: boolean
  formatAllowsForce: boolean
}

export function graphPathKey(path: string, caseInsensitive: boolean) {
  const normalized = path.replace(/\\/g, '/')
  return caseInsensitive ? normalized.toLowerCase() : normalized
}

export function findOpenTab<T extends { path: string }>(tabs: readonly T[], path: string, caseInsensitive: boolean) {
  const key = graphPathKey(path, caseInsensitive)
  return tabs.find(tab => Boolean(tab.path) && graphPathKey(tab.path, caseInsensitive) === key)
}

export function hasRestoreLoss(report?: RestoreLossReport | null) {
  return Boolean(report && (report.droppedNodes.length > 0 || report.droppedConnections.length > 0 || report.alteredNodes.length > 0))
}

export function compatibilitySaveOptions(input: CompatibilitySavePolicyInput): readonly CompatibilitySaveAction[] {
  if (!input.fatal && !input.hasLoss) return []
  return !input.fatal && input.hasLoss && input.formatAllowsForce
    ? ['copy', 'force', 'cancel']
    : ['copy', 'cancel']
}

export function resolveCompatibilitySaveAction(action: CompatibilitySaveAction, input: CompatibilitySavePolicyInput): CompatibilityPersistenceAction {
  if (action === 'cancel') return 'cancel'
  if (action === 'copy') return 'recovery-copy'
  return !input.fatal && input.hasLoss && input.formatAllowsForce ? 'force-source-with-backup' : 'cancel'
}
