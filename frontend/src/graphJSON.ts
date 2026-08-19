const minSafeInteger = BigInt(Number.MIN_SAFE_INTEGER)
const maxSafeInteger = BigInt(Number.MAX_SAFE_INTEGER)
const jsonNumberPattern = /^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/

// JSON itself can carry int64 literals exactly, but JSON.parse converts every
// number to a JavaScript Number. Quote only unsafe integer tokens before parsing
// so old numeric documents stay readable without losing their original value.
export function preserveUnsafeJSONIntegers(source: string) {
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
              result += JSON.stringify(integer.toString())
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
  return JSON.parse(preserveUnsafeJSONIntegers(source))
}
