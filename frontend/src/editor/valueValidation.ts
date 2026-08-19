const minInt64 = BigInt('-9223372036854775808')
const maxInt64 = BigInt('9223372036854775807')
const minSafeInteger = BigInt(Number.MIN_SAFE_INTEGER)
const maxSafeInteger = BigInt(Number.MAX_SAFE_INTEGER)
const decimalIntegerPattern = /^[+-]?\d+$/

export function normalizeIntegerInput(value: string): number | string {
  const text = value.trim()
  if (!decimalIntegerPattern.test(text)) return value
  try {
    const integer = BigInt(text)
    if (integer < minInt64 || integer > maxInt64) return value
    if (integer >= minSafeInteger && integer <= maxSafeInteger) return Number(integer)
    return integer.toString()
  } catch {
    return value
  }
}

export function isValidIntegerDefault(value: unknown) {
  if (typeof value === 'number') return Number.isSafeInteger(value)
  if (typeof value !== 'string' || !decimalIntegerPattern.test(value)) return false
  try {
    const integer = BigInt(value)
    return integer >= minInt64 && integer <= maxInt64
  } catch {
    return false
  }
}
