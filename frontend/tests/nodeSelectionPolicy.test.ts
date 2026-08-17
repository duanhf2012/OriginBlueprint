import { describe, expect, it } from 'vitest'
import { nodeSelectionPointerIntent, shouldCollapsePreservedSelection } from '../src/editor/nodeSelectionPolicy'

describe('node selection pointer intent', () => {
  it('uses the current pointer modifier instead of sticky keyboard state', () => {
    expect(nodeSelectionPointerIntent(['first'], 'second', false)).toEqual({ accumulate: false, preservedIds: [] })
    expect(nodeSelectionPointerIntent(['first'], 'second', true)).toEqual({ accumulate: true, preservedIds: [] })
  })

  it('temporarily preserves a selected group only when pressing one of its members without Ctrl', () => {
    expect(nodeSelectionPointerIntent(['first', 'second'], 'second', false)).toEqual({
      accumulate: false,
      preservedIds: ['first', 'second']
    })
    expect(nodeSelectionPointerIntent(['first', 'second'], 'second', true).preservedIds).toEqual([])
    expect(nodeSelectionPointerIntent(['first', 'second'], 'third', false).preservedIds).toEqual([])
  })
})

describe('preserved multi-selection completion', () => {
  it('collapses a plain click to the picked node', () => {
    expect(shouldCollapsePreservedSelection(['first', 'second'], 'second', false)).toBe(true)
  })

  it('keeps the group selected when the pointer actually dragged it', () => {
    expect(shouldCollapsePreservedSelection(['first', 'second'], 'second', true)).toBe(false)
  })
})
