import type { GraphDocument } from './editor/document'

export function isFunctionBlueprintPath(path: string) {
  return path.toLowerCase().endsWith('.obpf')
}

export function filenameStem(path: string) {
  const filename = path.split(/[\\/]/).pop() ?? ''
  const extensionIndex = filename.lastIndexOf('.')
  return extensionIndex > 0 ? filename.slice(0, extensionIndex) : filename
}

export function persistedGraphDocument(path: string, document: GraphDocument) {
  if (isFunctionBlueprintPath(path)) return { ...document }

  const { graphName: _graphName, ...persisted } = document
  return persisted
}

export function serializeGraphDocument(path: string, document: GraphDocument, indentation?: number) {
  return JSON.stringify(persistedGraphDocument(path, document), null, indentation)
}
