import { describe, expect, it } from 'vitest'
import { sourceRequiresProtection } from '../src/documentSafety'
import { autoSaveIntervalMs, isAutoSaveEligible } from '../src/autoSavePolicy'

describe('raw source validation protection', () => {
  it('protects a source when validation found an error', () => {
    expect(sourceRequiresProtection([
      { severity: 'warning' },
      { severity: 'error' },
    ])).toBe(true)
  })

  it('does not protect a source for warnings alone', () => {
    expect(sourceRequiresProtection([{ severity: 'warning' }])).toBe(false)
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
