export interface Point {
  x: number
  y: number
}

export interface Rect {
  left: number
  top: number
  right: number
  bottom: number
}

export interface SampledPath {
  getTotalLength(): number
  getPointAtLength(offset: number): Point
}

export function normalizeRect(rect: Rect): Rect {
  return {
    left: Math.min(rect.left, rect.right),
    top: Math.min(rect.top, rect.bottom),
    right: Math.max(rect.left, rect.right),
    bottom: Math.max(rect.top, rect.bottom)
  }
}

export function rectsIntersect(a: Rect, b: Rect): boolean {
  const first = normalizeRect(a)
  const second = normalizeRect(b)
  return first.right >= second.left
    && first.left <= second.right
    && first.bottom >= second.top
    && first.top <= second.bottom
}

export function pointInRect(point: Point, rect: Rect, padding = 0): boolean {
  const normalized = normalizeRect(rect)
  return point.x >= normalized.left - padding
    && point.x <= normalized.right + padding
    && point.y >= normalized.top - padding
    && point.y <= normalized.bottom + padding
}

export function pathIntersectsRect(path: SampledPath, rect: Rect, sampleStep = 4, padding = 2): boolean {
  const length = path.getTotalLength()
  if (!Number.isFinite(length) || length < 0) return false
  const sampleCount = Math.max(2, Math.ceil(length / Math.max(1, sampleStep)))
  for (let index = 0; index <= sampleCount; index++) {
    const offset = length * index / sampleCount
    if (pointInRect(path.getPointAtLength(offset), rect, padding)) return true
  }
  return false
}
