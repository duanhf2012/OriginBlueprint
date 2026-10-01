import type { GraphDocument } from './document'

// 纯文档级宏实例同步：用源宏当前内容替换目标文档中的实例（节点换新 ID、变量同名合并、
// 边界连线按源顺序重接、框标签刷新、实例记录更新）。与编辑器内 syncMacroInstance 同一算法，
// 供未打开的标签页与磁盘文件的自动传播使用；无 DOM 依赖，可单元测试。
export function syncMacroInstanceInDocument(document: GraphDocument, source: string, macro: GraphDocument, label: string): boolean {
  const instance = (document.macroInstances ?? []).find(item => item.source === source)
  if (!instance) return false
  const oldIds = new Set(instance.nodeIds)
  const indexById = new Map(instance.nodeIds.map((id, index) => [id, index]))
  type Boundary = { externalNode: string; externalPort: string; instanceIndex: number; instancePort: string; intoInstance: boolean }
  const boundaries: Boundary[] = []
  for (const connection of document.connections ?? []) {
    const sourceInside = oldIds.has(connection.source)
    const targetInside = oldIds.has(connection.target)
    if (sourceInside === targetInside) continue
    if (sourceInside) boundaries.push({ externalNode: connection.target, externalPort: String(connection.targetInput), instanceIndex: indexById.get(connection.source) ?? -1, instancePort: String(connection.sourceOutput), intoInstance: false })
    else boundaries.push({ externalNode: connection.source, externalPort: String(connection.sourceOutput), instanceIndex: indexById.get(connection.target) ?? -1, instancePort: String(connection.targetInput), intoInstance: true })
  }
  const variableRemap = new Map<string, string>()
  for (const variable of macro.variables ?? []) {
    const existing = (document.variables ?? []).find(item => item.name === variable.name && item.type === variable.type)
    if (existing) { variableRemap.set(variable.id, existing.id); continue }
    const freshId = variable.id && !(document.variables ?? []).some(item => item.id === variable.id) ? variable.id : crypto.randomUUID()
    variableRemap.set(variable.id, freshId)
    document.variables = [...(document.variables ?? []), { ...variable, id: freshId }]
  }
  const idBySourceNode = new Map<string, string>()
  const newNodes: GraphDocument['nodes'] = []
  for (const node of macro.nodes ?? []) {
    const freshId = crypto.randomUUID()
    idBySourceNode.set(node.id, freshId)
    const properties = { ...(node.properties ?? {}) }
    if (properties.variableId) properties.variableId = variableRemap.get(properties.variableId) ?? properties.variableId
    newNodes.push({ ...node, id: freshId, properties })
  }
  const externalConnections = (document.connections ?? []).filter(connection => !oldIds.has(connection.source) && !oldIds.has(connection.target))
  const rebuilt: GraphDocument['connections'] = [...externalConnections]
  for (const connection of macro.connections ?? []) {
    const source = idBySourceNode.get(connection.source)
    const target = idBySourceNode.get(connection.target)
    if (source && target) rebuilt.push({ source, sourceOutput: connection.sourceOutput, target, targetInput: connection.targetInput })
  }
  const sourceOrder = [...idBySourceNode.keys()]
  for (const boundary of boundaries) {
    const newNodeId = idBySourceNode.get(sourceOrder[boundary.instanceIndex] ?? '')
    if (!newNodeId) continue
    rebuilt.push(boundary.intoInstance
      ? { source: boundary.externalNode, sourceOutput: boundary.externalPort, target: newNodeId, targetInput: boundary.instancePort }
      : { source: newNodeId, sourceOutput: boundary.instancePort, target: boundary.externalNode, targetInput: boundary.externalPort })
  }
  document.nodes = [...(document.nodes ?? []).filter(node => !oldIds.has(node.id)), ...newNodes]
  document.connections = rebuilt
  instance.nodeIds = [...idBySourceNode.values()]
  const comment = (document.comments ?? []).find(item => item.id === instance.commentId)
  if (comment) comment.text = `宏：${label}`
  return true
}
