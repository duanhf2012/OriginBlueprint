import { pathIntersectsRect, type Rect, type SampledPath } from '../src/editor/selectionGeometry'
import { describe, it } from 'vitest'

function assert(value: unknown, message: string) {
  if (!value) throw new Error(message)
}

function sampledPath(points: Array<{ x: number; y: number }>): SampledPath {
  return {
    getTotalLength: () => points.length - 1,
    getPointAtLength: (offset: number) => points[Math.max(0, Math.min(points.length - 1, Math.round(offset)))]
  }
}

const selection: Rect = { left: 40, top: 20, right: 80, bottom: 60 }

describe('selection geometry', () => {
it('distinguishes paths that intersect the selection rectangle', () => {
assert(pathIntersectsRect(sampledPath([
  { x: 0, y: 10 },
  { x: 30, y: 20 },
  { x: 50, y: 40 },
  { x: 90, y: 50 }
]), selection), 'selects a connection when sampled curve points enter the rectangle')

assert(!pathIntersectsRect(sampledPath([
  { x: 0, y: 80 },
  { x: 30, y: 90 },
  { x: 90, y: 95 }
]), selection), 'ignores a connection when all sampled points are outside the rectangle')
})
})
