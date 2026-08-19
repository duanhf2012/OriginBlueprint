import { variableScope, type FunctionSignature, type GraphDocument } from './editor/document'
import { stringifyGraphJSON } from './graphJSON'

export interface FunctionPersistenceMetadata {
  graphName: string
  functionId: string
  functionCategory: string
  functionSignature: FunctionSignature
}

export function isFunctionBlueprintPath(path: string) {
  return path.toLowerCase().endsWith('.obpf')
}

export function filenameStem(path: string) {
  const filename = path.split(/[\\/]/).pop() ?? ''
  const extensionIndex = filename.lastIndexOf('.')
  return extensionIndex > 0 ? filename.slice(0, extensionIndex) : filename
}

export function completeGraphSavePath(path: string, functionBlueprint: boolean, requiresNative: boolean) {
  const filename = path.split(/[\\/]/).pop() ?? ''
  if (filename.lastIndexOf('.') > 0) return path
  const extension = functionBlueprint ? '.obpf' : requiresNative ? '.obp' : '.vgf'
  return `${path}${extension}`
}

export function documentRequiresNativePersistence(document: GraphDocument) {
  const nodes = document.nodes ?? []
  const signature = document.functionSignature ?? { inputs: [], outputs: [] }
  return nodes.some(node => String(node.typeId ?? '').startsWith('origin.function.') || node.typeId === 'origin.timer.set-by-function')
    || nodes.some(node => node.typeId === 'origin.flow.delay' || String(node.typeId ?? '').startsWith('origin.timer.'))
    || (document.variables ?? []).some(variable => variable.type === 'timerhandle' || variableScope(variable) === 'instance')
    || (signature.inputs?.length ?? 0) > 0
    || (signature.outputs?.length ?? 0) > 0
}

export function persistedGraphDocument(path: string, document: GraphDocument) {
  if (isFunctionBlueprintPath(path)) return { ...document }

  const { graphName: _graphName, ...persisted } = document
  return persisted
}

export function serializeGraphDocument(path: string, document: GraphDocument, indentation?: number) {
  return stringifyGraphJSON(persistedGraphDocument(path, document), indentation)
}

export function applyFunctionPersistenceMetadata(path: string, document: GraphDocument, metadata: FunctionPersistenceMetadata) {
  if (!isFunctionBlueprintPath(path)) return document

  document.graphName = metadata.graphName
  document.functionId = metadata.functionId
  document.functionCategory = metadata.functionCategory
  document.functionSignature = metadata.functionSignature
  return document
}

export function prepareGraphSave(sourcePath: string, targetPath: string, document: GraphDocument) {
  const sourceIsFunction = isFunctionBlueprintPath(sourcePath)
  const targetIsFunction = isFunctionBlueprintPath(targetPath)
  if (sourceIsFunction && !targetIsFunction) throw new Error('Function blueprints must be saved as .obpf files')
  if (!sourceIsFunction && targetIsFunction) throw new Error('Ordinary blueprints cannot be saved as .obpf files')

  const exportLegacy = targetPath.toLowerCase().endsWith('.vgf')
  return {
    path: targetPath,
    documentJSON: serializeGraphDocument(targetPath, document, exportLegacy ? undefined : 2),
    exportLegacy,
  }
}
