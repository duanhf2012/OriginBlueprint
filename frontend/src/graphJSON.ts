const minSafeInteger = BigInt(Number.MIN_SAFE_INTEGER)
const maxSafeInteger = BigInt(Number.MAX_SAFE_INTEGER)
const jsonNumberPattern = /^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/
const integerLexemePattern = /^-?(?:0|[1-9]\d*)$/
const preciseJSONIntegerBrand = Symbol.for('origin-blueprint.precise-json-integer')

let markerNonce = 0

function uniqueMarker(label: string) {
  markerNonce++
  return `\uE000origin-blueprint-${label}-${Date.now().toString(36)}-${markerNonce.toString(36)}-${Math.random().toString(36).slice(2)}\uE001`
}

// Represents an unsafe integer that was a JSON number in the source file.
// Keeping this distinction in memory lets opaque legacy and Any fields retain
// their numeric JSON type while known Integer controls can still edit the text.
export class PreciseJSONInteger {
  readonly lexeme: string

  constructor(lexeme: string) {
    if (!integerLexemePattern.test(lexeme)) throw new TypeError('precise JSON integer must be a canonical decimal integer')
    this.lexeme = lexeme
    Object.defineProperty(this, preciseJSONIntegerBrand, { value: true })
    Object.freeze(this)
  }

  toString() {
    return this.lexeme
  }
}

export function isPreciseJSONInteger(value: unknown): value is PreciseJSONInteger {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Record<PropertyKey, unknown>
  return candidate[preciseJSONIntegerBrand] === true && typeof candidate.lexeme === 'string'
}

function markUnsafeJSONIntegers(source: string, markerKey: string) {
  let result = ''
  let index = 0
  while (index < source.length) {
    const char = source[index]
    if (char === '"') {
      const start = index++
      while (index < source.length) {
        if (source[index] === '\\') {
          index += 2
          continue
        }
        if (source[index++] === '"') break
      }
      result += source.slice(start, index)
      continue
    }
    if (char === '-' || (char >= '0' && char <= '9')) {
      const match = jsonNumberPattern.exec(source.slice(index))
      if (match) {
        const token = match[0]
        if (!/[.eE]/.test(token)) {
          try {
            const integer = BigInt(token)
            if (integer < minSafeInteger || integer > maxSafeInteger) {
              result += `{${JSON.stringify(markerKey)}:${JSON.stringify(integer.toString())}}`
              index += token.length
              continue
            }
          } catch {
            // Let JSON.parse report malformed input below.
          }
        }
        result += token
        index += token.length
        continue
      }
    }
    result += char
    index++
  }
  return result
}

export function parseGraphJSON(source: string): unknown {
  const markerKey = uniqueMarker('parse')
  return JSON.parse(markUnsafeJSONIntegers(source, markerKey), (_key, value) => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return value
    const keys = Object.keys(value)
    if (keys.length !== 1 || keys[0] !== markerKey || typeof value[markerKey] !== 'string') return value
    return new PreciseJSONInteger(value[markerKey])
  })
}

export function stringifyGraphJSON(value: unknown, indentation?: number) {
  const prefix = uniqueMarker('serialize')
  const integers: Array<{ sentinel: string; lexeme: string }> = []
  const serialized = JSON.stringify(value, (_key, current) => {
    if (!isPreciseJSONInteger(current)) return current
    const sentinel = `${prefix}${integers.length}`
    integers.push({ sentinel, lexeme: current.lexeme })
    return sentinel
  }, indentation)
  if (serialized === undefined) throw new TypeError('graph JSON value is not serializable')
  let result = serialized
  for (const integer of integers) {
    result = result.split(JSON.stringify(integer.sentinel)).join(integer.lexeme)
  }
  return result
}

export function cloneGraphJSONValue<T>(value: T): T {
  if (isPreciseJSONInteger(value)) return new PreciseJSONInteger(value.lexeme) as T
  if (Array.isArray(value)) return value.map(item => cloneGraphJSONValue(item)) as T
  if (value && typeof value === 'object') {
    const result: Record<string, unknown> = {}
    for (const [key, item] of Object.entries(value)) result[key] = cloneGraphJSONValue(item)
    return result as T
  }
  return value
}
