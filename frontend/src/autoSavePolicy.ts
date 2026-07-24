export type AutoSaveMode = 'off' | '1m' | '3m' | '5m'

export interface AutoSaveCandidate {
  dirty: boolean
  path: string
  restoreFatal: boolean
  hasRestoreLoss: boolean
  legacyRequiresNative: boolean
  saving: boolean
}

export function autoSaveIntervalMs(mode: AutoSaveMode) {
  switch (mode) {
    case '1m': return 60_000
    case '3m': return 180_000
    case '5m': return 300_000
    default: return 0
  }
}

export function isAutoSaveEligible(candidate: AutoSaveCandidate) {
  return candidate.dirty
    && Boolean(candidate.path)
    && !candidate.restoreFatal
    && !candidate.hasRestoreLoss
    && !candidate.legacyRequiresNative
    && !candidate.saving
}
