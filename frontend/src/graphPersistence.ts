import { variableScope, type FunctionSignature, type GraphDocument } from './editor/document'
import { stringifyGraphJSON } from './graphJSON'

export interface FunctionPersistenceMetadata {
  graphName: string
  functionId: string
  functionCategory: string
  functionSignature: FunctionSignature
  functionDescription?: string
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

export function defaultGraphSaveFilename(suggestedPath: string, functionBlueprint: boolean, requiresNative: boolean) {
  const filename = (suggestedPath.split(/[\\/]/).pop() ?? '').trim()
  if (!filename || filename === '.') return functionBlueprint ? 'Untitled.obpf' : 'Untitled.obp'
  if (requiresNative && !functionBlueprint && filename.toLowerCase().endsWith('.vgf')) return `${filename.slice(0, filename.length - 4)}.obp`
  return filename
}

export function documentRequiresNativePersistence(document: GraphDocument) {
  const nodes = document.nodes ?? []
  const signature = document.functionSignature ?? { inputs: [], outputs: [] }
  return nodes.some(node => String(node.typeId ?? '').startsWith('origin.function.') || node.typeId === 'origin.timer.set-by-function')
    || nodes.some(node => node.typeId === 'origin.flow.delay' || String(node.typeId ?? '').startsWith('origin.timer.'))
    || (document.variables ?? []).some(variable => variable.type === 'timerhandle' || variableScope(variable) === 'instance')
    || (signature.inputs?.length ?? 0) > 0
    || (signature.outputs?.length ?? 0) > 0
    // 宏引用（macroRefs）是原生文档字段，legacy .vgf 导出无法承载，引用图必须存 .obp。
    || (document.macroRefs?.length ?? 0) > 0
}

function isMacroBlueprintPath(path: string) {
  return /\.obpm$/i.test(path)
}

export function persistedGraphDocument(path: string, document: GraphDocument) {
  if (isFunctionBlueprintPath(path)) return { ...document }
  // 宏显示名存于 graphName（与文件名分离，模块库/插入注释框用），与函数一样必须持久化；
  // 普通蓝图的名字即文件名，继续剥离以保持文档精简。
  if (isMacroBlueprintPath(path)) return { ...document }

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
  if (metadata.functionDescription?.trim()) document.functionDescription = metadata.functionDescription.trim()
  else delete document.functionDescription
  return document
}

export function prepareGraphSave(sourcePath: string, targetPath: string, document: GraphDocument) {
  const sourceIsFunction = isFunctionBlueprintPath(sourcePath)
  const targetIsFunction = isFunctionBlueprintPath(targetPath)
  if (sourceIsFunction && !targetIsFunction) throw new Error('Function blueprints must be saved as .obpf files')
  if (!sourceIsFunction && targetIsFunction) throw new Error('Ordinary blueprints cannot be saved as .obpf files')

  const exportLegacy = targetPath.toLowerCase().endsWith('.vgf')
  // 宏引用（macroRefs）无法用 legacy .vgf 表达：另存为 .vgf 会静默丢掉全部引用关系，必须拒绝。
  if (exportLegacy && (document.macroRefs?.length ?? 0) > 0) {
    throw new Error('引用了宏的蓝图必须保存为 .obp（legacy .vgf 无法保存宏引用）')
  }
  return {
    path: targetPath,
    documentJSON: serializeGraphDocument(targetPath, document, exportLegacy ? undefined : 2),
    exportLegacy,
  }
}
