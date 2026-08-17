export const functionEntryTypeId = 'origin.function.entry'
export const functionReturnTypeId = 'origin.function.return'

export interface FunctionTerminalNodeIdentity {
  id: string
  typeId?: string
}

export interface FunctionTerminalDeletionPlan {
  deletableIds: string[]
  protectedEntryIds: string[]
  protectedReturnIds: string[]
}

function protectedSelectedTerminalIds(
  allNodes: readonly FunctionTerminalNodeIdentity[],
  selectedIds: ReadonlySet<string>,
  typeId: string
) {
  const terminals = allNodes.filter(node => node.typeId === typeId)
  const selected = terminals.filter(node => selectedIds.has(node.id))
  const unselectedCount = terminals.length - selected.length
  const protectedCount = Math.max(0, 1 - unselectedCount)
  return selected.slice(0, protectedCount).map(node => node.id)
}

export function planFunctionTerminalDeletion(
  allNodes: readonly FunctionTerminalNodeIdentity[],
  selectedNodes: readonly FunctionTerminalNodeIdentity[]
): FunctionTerminalDeletionPlan {
  const selectedIds = new Set(selectedNodes.map(node => node.id))
  const protectedEntryIds = protectedSelectedTerminalIds(allNodes, selectedIds, functionEntryTypeId)
  const protectedReturnIds = protectedSelectedTerminalIds(allNodes, selectedIds, functionReturnTypeId)
  const protectedIds = new Set([...protectedEntryIds, ...protectedReturnIds])
  return {
    deletableIds: selectedNodes.filter(node => !protectedIds.has(node.id)).map(node => node.id),
    protectedEntryIds,
    protectedReturnIds
  }
}

export function isCopyableFunctionNode(node: FunctionTerminalNodeIdentity) {
  return node.typeId !== functionEntryTypeId
}

export function isPasteableFunctionNode(node: FunctionTerminalNodeIdentity) {
  return node.typeId !== functionEntryTypeId
}
