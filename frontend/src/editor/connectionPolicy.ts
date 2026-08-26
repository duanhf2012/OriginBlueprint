import { normalizeSocketName } from './socketTheme'

export interface ConnectionEndpoint {
  id: string
  source: string
  sourceOutput: string
}

export function execOutputReplacementIds(
  candidate: Pick<ConnectionEndpoint, 'source' | 'sourceOutput'>,
  existing: ConnectionEndpoint[],
  sourceSocketName?: string
) {
  if (normalizeSocketName(sourceSocketName) !== 'exec') return []
  return existing
    .filter(item => item.source === candidate.source && String(item.sourceOutput) === String(candidate.sourceOutput))
    .map(item => item.id)
}
