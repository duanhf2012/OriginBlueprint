import { describe, expect, it } from 'vitest'
import { execOutputReplacementIds, inputAllowsMultipleConnections } from '../src/editor/connectionPolicy'

describe('execution connection policy', () => {
  const connections = [
    { id: 'first', source: 'entry', sourceOutput: 'exec' },
    { id: 'legacy-fanout', source: 'entry', sourceOutput: 'exec' },
    { id: 'other-output', source: 'entry', sourceOutput: 'failed' },
    { id: 'other-node', source: 'branch', sourceOutput: 'exec' }
  ]

  it('replaces every existing connection from the same exec output', () => {
    expect(execOutputReplacementIds({ source: 'entry', sourceOutput: 'exec' }, connections, 'exec'))
      .toEqual(['first', 'legacy-fanout'])
  })

  it('does not replace connections from other outputs or nodes', () => {
    expect(execOutputReplacementIds({ source: 'entry', sourceOutput: 'failed' }, connections, 'exec'))
      .toEqual(['other-output'])
  })

  it('does not apply the single-cast rule to data outputs', () => {
    expect(execOutputReplacementIds({ source: 'entry', sourceOutput: 'exec' }, connections, 'integer'))
      .toEqual([])
  })

  it('allows multiple control-flow predecessors on an exec input', () => {
    expect(inputAllowsMultipleConnections('exec')).toBe(true)
    expect(inputAllowsMultipleConnections('Exec')).toBe(true)
  })

  it('keeps data inputs single-producer', () => {
    expect(inputAllowsMultipleConnections('integer')).toBe(false)
    expect(inputAllowsMultipleConnections('string')).toBe(false)
  })
})
