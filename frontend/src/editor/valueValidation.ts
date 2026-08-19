export function isValidIntegerDefault(value: unknown) {
  return typeof value === 'number' && Number.isSafeInteger(value)
}
