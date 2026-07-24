export const editorHistoryLimit = 100

export function pushBoundedHistory<T>(stack: T[], value: T, limit = editorHistoryLimit) {
  const effectiveLimit = Math.max(1, Math.trunc(limit))
  stack.push(value)
  if (stack.length > effectiveLimit) stack.splice(0, stack.length - effectiveLimit)
}
