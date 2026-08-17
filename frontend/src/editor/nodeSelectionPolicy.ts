export interface NodeSelectionPointerIntent {
  accumulate: boolean
  preservedIds: string[]
}

export function nodeSelectionPointerIntent(
  selectedIds: readonly string[],
  pickedId: string,
  additive: boolean
): NodeSelectionPointerIntent {
  return {
    accumulate: additive,
    preservedIds: !additive && pickedId && selectedIds.length > 1 && selectedIds.includes(pickedId)
      ? [...selectedIds]
      : []
  }
}

export function shouldCollapsePreservedSelection(
  preservedIds: readonly string[],
  pickedId: string,
  moved: boolean
) {
  return !moved && preservedIds.length > 1 && preservedIds.includes(pickedId)
}
